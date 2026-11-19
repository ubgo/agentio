package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// TitleSource names where a conversation title came from.
type TitleSource string

// Title sources, preferred first.
const (
	TitleCustom     TitleSource = "custom-title"
	TitleAI         TitleSource = "ai-title"
	TitleLastPrompt TitleSource = "last-prompt"
)

// Conversation is one resumable conversation: the neutral fields plus
// Claude extras read from the transcript head and tail.
type Conversation struct {
	agentio.Conversation
	TitleSource TitleSource
	Size        int64
	// Version is the Claude Code version that wrote the first record.
	Version     string
	GitBranch   string
	Owner       *SessionOwner
	ContinuedIn string
	Name        string
}

// Neutral returns the shared conversation. cost: Pure.
func (c Conversation) Neutral() agentio.Conversation { return c.Conversation }

// headInfo is what the transcript head yields.
type headInfo struct {
	cwd, version, branch string
	created              time.Time
}

// readHead scans the first 256 KiB for the first record with a cwd.
func readHead(path string) (headInfo, error) {
	var hi headInfo
	err := headLines(path, headScanBytes, func(line []byte) bool {
		var r Record
		if json.Unmarshal(line, &r) != nil {
			return true
		}
		if hi.created.IsZero() && r.Timestamp != nil {
			hi.created = *r.Timestamp
		}
		if r.Cwd != "" {
			hi.cwd, hi.version, hi.branch = r.Cwd, r.Version, r.GitBranch
			return false
		}
		return true
	})
	return hi, err
}

// tailInfo is what the last 256 KiB yields.
type tailInfo struct {
	title       string
	source      TitleSource
	owner       *SessionOwner
	continuedIn string
	name        string
}

// Cheap prefilter marks: the quoted type value must appear in the line
// before it is decoded (matches JSON written with or without spaces).
func tailMarks() [][]byte {
	return [][]byte{
		[]byte(`"` + string(RecordCustomTitle) + `"`), []byte(`"` + string(RecordAITitle) + `"`),
		[]byte(`"` + string(RecordLastPrompt) + `"`), []byte(`"` + string(RecordSessionOwner) + `"`),
		[]byte(`"` + string(RecordContinuedIn) + `"`), []byte(`"` + string(RecordAgentName) + `"`),
	}
}

func readTailInfo(path string) (tailInfo, error) {
	buf, err := readTail(path, tailScanBytes)
	if err != nil {
		return tailInfo{}, err
	}
	var ti tailInfo
	var custom, ai, last string
	marks := tailMarks()
	for _, line := range splitLines(buf) {
		if !slices.ContainsFunc(marks, func(m []byte) bool { return bytes.Contains(line, m) }) {
			continue
		}
		r, err := ParseRecord(line)
		if err != nil {
			continue
		}
		switch r.Type {
		case RecordCustomTitle, RecordAITitle:
			if t, err := r.AsTitle(); err == nil && t.Title != "" {
				if r.Type == RecordCustomTitle {
					custom = t.Title
				} else {
					ai = t.Title
				}
			}
		case RecordLastPrompt:
			if lp, err := r.AsLastPrompt(); err == nil && lp.Text != "" {
				last = lp.Text
			}
		case RecordSessionOwner:
			if o, err := r.AsSessionOwner(); err == nil {
				ti.owner = &o
			}
		case RecordContinuedIn:
			if id, err := r.AsContinuedIn(); err == nil {
				ti.continuedIn = id
			}
		case RecordAgentName:
			if n, err := r.AsAgentName(); err == nil {
				ti.name = n
			}
		}
	}
	switch {
	case custom != "":
		ti.title, ti.source = custom, TitleCustom
	case ai != "":
		ti.title, ti.source = ai, TitleAI
	case last != "":
		ti.title, ti.source = last, TitleLastPrompt
	}
	return ti, nil
}

// readConversation builds one listing entry; ok is false when the head has
// no cwd (a transcript that never ran a turn cannot be resumed in a known
// folder, and listing it would make it the "newest" one).
func (a *Agent) readConversation(h Home, path string, info os.FileInfo) (Conversation, bool) {
	hi, err := readHead(path)
	if err != nil || hi.cwd == "" {
		return Conversation{}, false
	}
	ti, err := readTailInfo(path)
	if err != nil {
		return Conversation{}, false
	}
	return Conversation{
		Conversation: agentio.Conversation{
			Agent: ID, Home: h.Neutral(), ID: strings.TrimSuffix(filepath.Base(path), transcriptExt),
			Title: ti.title, Dir: hi.cwd, Path: path, Created: hi.created, Updated: info.ModTime(),
		},
		TitleSource: ti.source, Size: info.Size(), Version: hi.version, GitBranch: hi.branch,
		Owner: ti.owner, ContinuedIn: ti.continuedIn, Name: ti.name,
	}, true
}

type candidate struct {
	path string
	info os.FileInfo
}

// transcriptCandidates lists <id>.jsonl files in the given project dirs.
// Set-aside files (<id>.orphaned-…jsonl, .superseded-…) are skipped.
func transcriptCandidates(dirs []string, since time.Time) []candidate {
	var out []candidate
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue // a missing history folder = no conversations there
		}
		for _, e := range entries {
			n := e.Name()
			if e.IsDir() || !strings.HasSuffix(n, transcriptExt) || strings.Contains(strings.TrimSuffix(n, transcriptExt), ".") {
				continue
			}
			info, err := e.Info()
			if err != nil || (!since.IsZero() && info.ModTime().Before(since)) {
				continue
			}
			out = append(out, candidate{path: filepath.Join(d, n), info: info})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].info.ModTime().After(out[j].info.ModTime()) })
	return out
}

// Conversations lists resumable conversations in h, newest first. With
// q.Dir only that folder's history dir is read (EncodePath lookup, as
// Claude Code does) and Dir is q.Dir, because the cwd recorded in old
// transcripts is stale after a history move (invariant 4). Each transcript
// is read at its head (cwd) and last 256 KiB (title) only (invariant 5); an
// unreadable transcript is skipped. cost: Disk · Read.
func (a *Agent) Conversations(ctx context.Context, h Home, q agentio.ConversationQuery) ([]Conversation, error) {
	var dirs []string
	if q.Dir != "" {
		dirs = []string{a.HistoryDir(h, q.Dir)}
	} else {
		root := filepath.Join(h.Dir(), projectsDir)
		entries, err := os.ReadDir(root)
		if isNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("claude: list projects: %w", err)
		}
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(root, e.Name()))
			}
		}
	}
	var out []Conversation
	for _, c := range transcriptCandidates(dirs, q.Since) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
		conv, ok := a.readConversation(h, c.path, c.info)
		if !ok {
			continue
		}
		if q.Dir != "" {
			conv.Dir = filepath.Clean(q.Dir)
		}
		out = append(out, conv)
	}
	return out, nil
}

// Conversation finds one conversation by id in any project dir of h.
// ErrNotFound when absent. cost: Disk · Read.
func (a *Agent) Conversation(h Home, id string) (Conversation, error) {
	p, err := a.findTranscript(h, id)
	if err != nil {
		return Conversation{}, err
	}
	info, err := os.Stat(p)
	if err != nil {
		return Conversation{}, fmt.Errorf("claude: stat transcript: %w", err)
	}
	c, ok := a.readConversation(h, p, info)
	if !ok {
		return Conversation{}, fmt.Errorf("%w: conversation %s has no turn yet", agentio.ErrNotFound, id)
	}
	return c, nil
}

// HasHistory reports whether dir has a history folder in h: one stat, no
// transcript read. cost: Disk · Read.
func (a *Agent) HasHistory(h Home, dir string) (bool, error) {
	info, err := os.Stat(a.HistoryDir(h, dir))
	if isNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claude: stat history: %w", err)
	}
	return info.IsDir(), nil
}

// Turns reads user and assistant turns of a conversation from an offset to
// the last complete line, calling fn per turn, and returns the new offset.
// Tool-result user lines and assistant lines without text or usage are
// skipped (they are not turns a person typed or read). Turn.ID is the
// record uuid. Claude Code writes an assistant record with its usage on
// the same line, so no turn waits for a later line. cost: Disk · Read.
func (a *Agent) Turns(h Home, id string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	p, err := a.findTranscript(h, id)
	if err != nil {
		return from, err
	}
	return TurnsFile(p, from, fn)
}

// TurnsFile is Turns over a transcript path, for callers that hold a path
// (a file watcher, a hook's transcript_path) rather than a home and id; the
// neutral TurnFileReader.TurnsAt. A line that is not JSON is skipped.
// cost: Disk · Read.
func TurnsFile(path string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	return TurnsFileReporting(path, from, fn, nil)
}

// TurnsFileReporting is TurnsFile that also reports each skipped non-JSON
// line to bad (nil = silent); the neutral BadLineReporter. cost: Disk ·
// Read.
func TurnsFileReporting(path string, from agentio.Offset, fn func(agentio.Turn) error, bad func(agentio.BadLine)) (agentio.Offset, error) {
	r, err := openTranscriptFile(path, from)
	if err != nil {
		return from, err
	}
	if bad != nil {
		r.onBad = func(at agentio.Offset, err error) { bad(agentio.BadLine{Path: path, Offset: at, Err: err}) }
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = r.Close() }()
	for {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			return r.Offset(), nil
		}
		if err != nil {
			return r.Offset(), err
		}
		t, ok := turnOf(rec)
		if !ok {
			continue
		}
		if err := fn(t); err != nil {
			return r.Offset(), err
		}
	}
}

// turnOf normalizes a user or assistant record.
func turnOf(rec Record) (agentio.Turn, bool) {
	t := agentio.Turn{ID: rec.UUID, ConversationID: rec.SessionID, Dir: rec.Cwd, AgentVersion: rec.Version, Raw: rec.Raw}
	if rec.Timestamp != nil {
		t.Time = *rec.Timestamp
	}
	switch rec.Type {
	case RecordUser:
		u, err := rec.AsUser()
		if err != nil || u.IsToolResult || !u.Content.HasText() {
			return t, false
		}
		t.Role, t.Text = agentio.RoleUser, u.Content.Text()
		return t, true
	case RecordAssistant:
		ar, err := rec.AsAssistant()
		if err != nil || (!ar.Content.HasText() && ar.Usage == nil) {
			return t, false
		}
		t.Role, t.Text, t.Model, t.StopReason = agentio.RoleAssistant, ar.Content.Text(), ar.Model, ar.StopReason
		if ar.Usage != nil {
			n := ar.Usage.Neutral()
			t.Tokens = &n
		}
		return t, true
	}
	return t, false
}

// Per-conversation data Claude Code keeps outside the project folder,
// keyed by conversation id (agx move.go, verified 2.1.281).
func perConversationDirs() []string { return []string{fileHistoryDir, sessionEnvDir} }

// ErrAmbiguousID means a MoveRequest.OnlyIDs prefix matches more than one
// conversation.
var ErrAmbiguousID = errors.New("claude: ambiguous conversation prefix")

// matchID accepts a full id or a unique prefix; an exact id always wins.
func matchID(ids []string, want string) (string, int) {
	var found string
	n := 0
	for _, id := range ids {
		if id == want {
			return id, 1
		}
		if strings.HasPrefix(id, want) {
			found = id
			n++
		}
	}
	if n != 1 {
		return "", n
	}
	return found, n
}

// PlanMove plans moving every conversation recorded for r.Dir from r.From
// to r.To (both Claude homes): transcripts, per-conversation dirs, file
// checkpoints, env snapshots, and on a full move the folder's auto memory.
// It changes nothing. Blockers: a live Claude session in the folder (either
// home) or a target that already exists. Unknown entries are listed in
// Left. cost: Disk · Read.
func (a *Agent) PlanMove(r agentio.MoveRequest) (agentio.MovePlan, error) {
	from, err := a.HomeOf(r.From)
	if err != nil {
		return agentio.MovePlan{}, err
	}
	to, err := a.HomeOf(r.To)
	if err != nil {
		return agentio.MovePlan{}, err
	}
	plan := agentio.MovePlan{Agent: ID, From: r.From, To: r.To, Dir: filepath.Clean(r.Dir)}
	src, dst := a.HistoryDir(from, plan.Dir), a.HistoryDir(to, plan.Dir)
	entries, err := os.ReadDir(src)
	if isNotExist(err) {
		return plan, nil
	}
	if err != nil {
		return plan, fmt.Errorf("claude: read %s: %w", src, err)
	}
	ids := transcriptIDs(entries)
	if len(r.OnlyIDs) > 0 {
		var picked []string
		for _, want := range r.OnlyIDs {
			id, n := matchID(ids, want)
			switch {
			case n == 0:
				return plan, fmt.Errorf("%w: conversation %q for %s", agentio.ErrNotFound, want, plan.Dir)
			case n > 1:
				return plan, fmt.Errorf("%w %q matches %d conversations", ErrAmbiguousID, want, n)
			}
			picked = append(picked, id)
		}
		ids = picked
	}
	plan.Conversations = ids
	moving := map[string]bool{}
	add := func(f, t string) {
		if !fileExists(f) {
			return
		}
		if fileExists(t) {
			plan.Blockers = append(plan.Blockers, t+" already exists in the target home")
		}
		plan.Items = append(plan.Items, agentio.MoveItem{From: f, To: t})
		moving[f] = true
	}
	for _, id := range ids {
		add(filepath.Join(src, id+transcriptExt), filepath.Join(dst, id+transcriptExt))
		add(filepath.Join(src, id), filepath.Join(dst, id))
		for _, d := range perConversationDirs() {
			add(filepath.Join(from.Dir(), d, id), filepath.Join(to.Dir(), d, id))
		}
	}
	if len(r.OnlyIDs) == 0 {
		if mem, err := os.ReadDir(filepath.Join(src, memoryDir)); err == nil {
			for _, m := range mem {
				add(filepath.Join(src, memoryDir, m.Name()), filepath.Join(dst, memoryDir, m.Name()))
			}
		}
		plan.Cleanup = []string{filepath.Join(src, memoryDir), src}
	}
	for _, e := range entries {
		p := filepath.Join(src, e.Name())
		if !moving[p] && e.Name() != memoryDir {
			plan.Left = append(plan.Left, p)
		}
	}
	plan.Blockers = append(plan.Blockers, a.liveBlockers(plan.Dir, ids, from, to)...)
	return plan, nil
}

// liveBlockers names live sessions in dir or of ids in any of the homes,
// in the text LiveSessionError.Blockers uses.
func (a *Agent) liveBlockers(dir string, ids []string, homes ...Home) []string {
	return (&LiveSessionError{Dir: dir, PIDs: a.livePIDs(dir, ids, homes...)}).Blockers()
}

// livePIDs lists the pids of alive sessions whose cwd is dir or whose
// conversation is one of ids, in any of the homes. A home whose sessions
// cannot be read adds none.
func (a *Agent) livePIDs(dir string, ids []string, homes ...Home) []int {
	var out []int
	for _, hm := range homes {
		live, err := a.LiveSessions(hm)
		if err != nil {
			continue
		}
		for _, s := range live {
			if s.Alive && (filepath.Clean(s.Cwd) == dir || slices.Contains(ids, s.SessionID)) {
				out = append(out, s.PID)
			}
		}
	}
	return out
}

// transcriptIDs are the conversation ids of the transcripts among entries
// of a history folder, sorted.
func transcriptIDs(entries []os.DirEntry) []string {
	var ids []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), transcriptExt) {
			ids = append(ids, strings.TrimSuffix(e.Name(), transcriptExt))
		}
	}
	slices.Sort(ids)
	return ids
}

// LiveSessionError is a move refused because Claude is running in the
// folder (or one of its conversations is live): Writer.MoveHistory returns
// it, and PlanMove lists the same Blockers text. It matches
// agentio.ErrMoveBlocked through errors.Is.
type LiveSessionError struct {
	// Dir is the folder whose history would move.
	Dir string
	// PIDs are the live Claude processes, in session order.
	PIDs []int
}

// Blockers is one line per live process, as in MovePlan.Blockers. Pure.
func (e *LiveSessionError) Blockers() []string {
	out := make([]string, 0, len(e.PIDs))
	for _, pid := range e.PIDs {
		out = append(out, fmt.Sprintf("Claude is running in this folder (pid %d); exit it first", pid))
	}
	return out
}

// Error names the folder and every blocker. Pure.
func (e *LiveSessionError) Error() string {
	return fmt.Sprintf("claude: move %s blocked: %s", e.Dir, strings.Join(e.Blockers(), "; "))
}

// Unwrap is agentio.ErrMoveBlocked. Pure.
func (e *LiveSessionError) Unwrap() error { return agentio.ErrMoveBlocked }
