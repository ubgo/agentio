package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Rollout file naming: sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl.
const (
	rolloutPrefix = "rollout-"
	rolloutExt    = ".jsonl"
	// titleMaxRunes bounds a title derived from the first user message.
	titleMaxRunes = 80
)

// Conversation is one Codex rollout with its title from session_index.
type Conversation struct {
	ID            string // uuid v7
	Home          Home
	Path          string // rollout file
	Dir           string // session_meta.cwd
	Title         string // session_index thread_name (last wins), else the first user message (head read, truncated)
	Created       time.Time
	Updated       time.Time // file mtime
	CLIVersion    agentio.Version
	Source        SessionSource
	Originator    string // "codex-tui" | "Codex Desktop" | "codex_exec" …
	ModelProvider string
	Git           *GitInfo
	Archived      bool // under archived_sessions/
}

// Neutral projects onto agentio.Conversation. Pure.
func (c Conversation) Neutral() agentio.Conversation {
	return agentio.Conversation{Agent: ID, Home: c.Home.Neutral(), ID: c.ID, Title: c.Title, Dir: c.Dir, Path: c.Path, Created: c.Created, Updated: c.Updated}
}

// ConversationQuery filters Conversations. Zero values do not filter.
type ConversationQuery struct {
	Dir             string
	Limit           int
	IncludeArchived bool
	Since           time.Time
	Sources         []SessionSource
}

// Title is one session_index.jsonl entry (last line per id wins).
type Title struct {
	ID        string    `json:"id"`
	Name      string    `json:"thread_name"`
	UpdatedAt time.Time `json:"-"`
}

type titleWire struct {
	ID         string `json:"id"`
	ThreadName string `json:"thread_name"`
	UpdatedAt  string `json:"updated_at"`
}

// Titles reads session_index.jsonl; a missing file is an empty map. Disk · Read.
func (a *Agent) Titles(h Home) (map[string]Title, error) {
	return readTitles(a.Paths(h).SessionIndex)
}

func readTitles(path string) (map[string]Title, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]Title{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("codex: open %s: %w", fileSessionIndex, err)
	}
	// Read-only file: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	out := map[string]Title{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, readChunk), maxLineBytes)
	for sc.Scan() {
		var w titleWire
		if json.Unmarshal(sc.Bytes(), &w) != nil || w.ID == "" {
			continue
		}
		out[w.ID] = Title{ID: w.ID, Name: w.ThreadName, UpdatedAt: parseRFC3339(w.UpdatedAt)}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("codex: read %s: %w", fileSessionIndex, err)
	}
	return out, nil
}

type rolloutFile struct {
	path     string
	mod      time.Time
	archived bool
}

// walkRollouts lists rollout files under root, newest mtime first.
func walkRollouts(root string, archived bool) ([]rolloutFile, error) {
	var out []rolloutFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipDir
			}
			return nil // unreadable subtree: skip, don't fail the listing
		}
		if d.IsDir() || !isRolloutName(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, rolloutFile{path: path, mod: info.ModTime(), archived: archived})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("codex: walk %s: %w", root, err)
	}
	return out, nil
}

func isRolloutName(name string) bool {
	return strings.HasPrefix(name, rolloutPrefix) && strings.HasSuffix(name, rolloutExt)
}

// Conversations lists rollouts newest first: a directory walk plus one
// bounded head read per file and one read of session_index.jsonl. Never
// reads a transcript whole. Disk · Read.
func (a *Agent) Conversations(ctx context.Context, h Home, q ConversationQuery) ([]Conversation, error) {
	p := a.Paths(h)
	files, err := walkRollouts(p.Sessions, false)
	if err != nil {
		return nil, err
	}
	if q.IncludeArchived {
		arch, err := walkRollouts(p.ArchivedSessions, true)
		if err != nil {
			return nil, err
		}
		files = append(files, arch...)
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	titles, err := readTitles(p.SessionIndex)
	if err != nil {
		return nil, err
	}
	var out []Conversation
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
		if !q.Since.IsZero() && f.mod.Before(q.Since) {
			break // newest first: everything after is older
		}
		c, ok := conversationFrom(h, f, titles)
		if !ok || !matchQuery(c, q) {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func matchQuery(c Conversation, q ConversationQuery) bool {
	if q.Dir != "" && filepath.Clean(c.Dir) != filepath.Clean(q.Dir) {
		return false
	}
	if len(q.Sources) > 0 {
		for _, s := range q.Sources {
			if s == c.Source {
				return true
			}
		}
		return false
	}
	return true
}

func conversationFrom(h Home, f rolloutFile, titles map[string]Title) (Conversation, bool) {
	m, ok := readHeadMeta(f.path)
	if !ok {
		return Conversation{}, false
	}
	c := Conversation{
		ID: m.ID, Home: h, Path: f.path, Dir: m.Cwd, Created: m.Created(), Updated: f.mod,
		CLIVersion: m.Version(), Source: m.Source, Originator: m.Originator, ModelProvider: m.ModelProvider,
		Git: m.Git, Archived: f.archived,
	}
	if t, ok := titles[m.ID]; ok {
		c.Title = t.Name
	} else {
		c.Title = headTitle(f.path)
	}
	return c, true
}

// headTitle is the first user message within the head bound, cut to
// titleMaxRunes: the fallback title for a conversation never renamed.
func headTitle(path string) string {
	b, err := readHead(path, headScanBytes)
	if err != nil {
		return ""
	}
	lines := bytes.Split(b, []byte{newline})
	if len(b) == headScanBytes && len(lines) > 0 {
		lines = lines[:len(lines)-1] // last line may be cut by the bound
	}
	for _, line := range lines {
		rec, err := ParseRecord(bytes.TrimSpace(line))
		if err != nil || rec.Event == nil {
			continue
		}
		ev := rec.Event
		text := ""
		switch {
		case ev.Type == EventItemCompleted && ev.Item != nil && ev.Item.Type == ItemUserMessage:
			text = ev.Item.Text()
		case ev.Type == EventUserMessage:
			text = ev.Message
		}
		if text = strings.Join(strings.Fields(text), " "); text != "" {
			if r := []rune(text); len(r) > titleMaxRunes {
				return string(r[:titleMaxRunes]) + "…"
			}
			return text
		}
	}
	return ""
}

// readHead returns at most n bytes from the start of path.
func readHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// Read-only file: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, int64(n)))
}

// ErrConversationNotFound means no rollout file names the id.
var ErrConversationNotFound = errors.New("codex: conversation not found")

// Conversation finds one rollout by id: the id is in the file name, so the
// lookup matches names without opening other files. Disk · Read.
func (a *Agent) Conversation(h Home, id string) (Conversation, error) {
	if id == "" {
		return Conversation{}, ErrConversationNotFound
	}
	p := a.Paths(h)
	for _, root := range []struct {
		dir      string
		archived bool
	}{{p.Sessions, false}, {p.ArchivedSessions, true}} {
		files, err := walkRollouts(root.dir, root.archived)
		if err != nil {
			return Conversation{}, err
		}
		for _, f := range files {
			if !strings.Contains(filepath.Base(f.path), id) {
				continue
			}
			titles, err := readTitles(p.SessionIndex)
			if err != nil {
				return Conversation{}, err
			}
			if c, ok := conversationFrom(h, f, titles); ok && c.ID == id {
				return c, nil
			}
		}
	}
	return Conversation{}, fmt.Errorf("%w: %s", ErrConversationNotFound, id)
}

// HasHistory reports whether any rollout's cwd is dir. The cwd index is
// built per call (no global cache). Disk · Read.
func (a *Agent) HasHistory(h Home, dir string) (bool, error) {
	convs, err := a.Conversations(context.Background(), h, ConversationQuery{Dir: dir, Limit: 1})
	if err != nil {
		return false, err
	}
	return len(convs) > 0, nil
}

// MoveUnsupportedReason explains why Codex conversations cannot move
// between homes (invariant 6). Pure.
func (a *Agent) MoveUnsupportedReason() string {
	return "Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home"
}

// CachedUsage returns the last rate-limit snapshot (event_msg/token_count
// with rate_limits) of the newest rollout, from a bounded tail read: usage
// with zero network calls, timestamped by its record. found=false when no
// rollout has a snapshot. Disk · Read (OD-10).
func (a *Agent) CachedUsage(h Home) (Usage, bool, error) {
	files, err := walkRollouts(a.Paths(h).Sessions, false)
	if err != nil {
		return Usage{}, false, err
	}
	sort.SliceStable(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	const maxFilesScanned = 5 // a few newest files: the newest may be a fresh, snapshot-less session
	for i, f := range files {
		if i >= maxFilesScanned {
			break
		}
		recs, err := rolloutTail(f.path, tailReadBytes)
		if err != nil {
			continue
		}
		for j := len(recs) - 1; j >= 0; j-- {
			r := recs[j]
			if r.Event == nil || r.Event.Type != EventTokenCount || r.Event.RateLimits == nil {
				continue
			}
			return usageFromSnapshot(r), true, nil
		}
	}
	return Usage{}, false, nil
}

func usageFromSnapshot(r Record) Usage {
	rl := r.Event.RateLimits
	u := Usage{
		ObservedAt: r.Time, Source: agentio.UsageTranscript,
		Credits: rl.Credits, IndividualLimit: rl.IndividualLimit, SpendControlReached: rl.SpendControlReached,
		LimitReached: rl.RateLimitReachedType != "",
	}
	if rl.PlanType != "" {
		u.Plan = ParsePlan(rl.PlanType)
	}
	var payload struct {
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(r.Raw, &payload) == nil {
		u.Raw = agentio.Raw(payload.Payload)
	}
	for _, pair := range []struct {
		w      *RateLimitWindow
		vendor string
	}{{rl.Primary, vendorPrimaryL}, {rl.Secondary, vendorSecondL}} {
		if pair.w == nil {
			continue
		}
		uw := UsageWindow{Length: time.Duration(pair.w.WindowMinutes) * time.Minute, Percent: pair.w.UsedPercent, LimitID: rl.LimitID, Vendor: pair.vendor}
		if pair.w.ResetsAt > 0 {
			t := time.Unix(pair.w.ResetsAt, 0).UTC()
			uw.ResetsAt = &t
		}
		u.Windows = append(u.Windows, uw)
	}
	return u
}

// Turns reads conversation id from offset from to the last complete line,
// calling fn once per user or agent message, and returns the new offset
// (G4, G6). Both the 0.15x+ item_completed items and the older
// user_message / agent_message events are read; when a file has both,
// item_completed wins so a message is not reported twice. Disk · Read.
func (a *Agent) Turns(h Home, id string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	c, err := a.Conversation(h, id)
	if err != nil {
		return from, err
	}
	return turnsFromPath(c.Path, id, from, fn)
}

func turnsFromPath(path, id string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	r, err := openRolloutAt(path, from)
	if err != nil {
		return from, err
	}
	// Read-only file: a close error cannot lose data.
	defer func() { _ = r.Close() }()
	var (
		dir, model, version string
		pending             *agentio.Turn
		sawItems            bool
	)
	flush := func() error {
		if pending == nil {
			return nil
		}
		t := *pending
		pending = nil
		return fn(t)
	}
	for {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return r.Offset(), err
		}
		switch {
		case rec.SessionMeta != nil:
			dir, version = rec.SessionMeta.Cwd, rec.SessionMeta.CLIVersion
		case rec.TurnContext != nil:
			model = rec.TurnContext.Model
			if rec.TurnContext.Cwd != "" {
				dir = rec.TurnContext.Cwd
			}
		case rec.Event != nil:
			ev := rec.Event
			base := agentio.Turn{Time: rec.Time, ConversationID: id, Dir: dir, Model: model, AgentVersion: version, Raw: rec.Raw}
			switch {
			case ev.Type == EventItemCompleted && ev.Item != nil:
				role, ok := itemRole(ev.Item.Type)
				if !ok {
					continue
				}
				sawItems = true
				if err := flush(); err != nil {
					return r.Offset(), err
				}
				base.Role, base.Text = role, ev.Item.Text()
				pending = &base
			case (ev.Type == EventUserMessage || ev.Type == EventAgentMessage) && !sawItems:
				if err := flush(); err != nil {
					return r.Offset(), err
				}
				base.Role = agentio.RoleUser
				if ev.Type == EventAgentMessage {
					base.Role = agentio.RoleAssistant
				}
				base.Text = ev.Message
				pending = &base
			case ev.Type == EventTokenCount && ev.Info != nil && ev.Info.LastTokenUsage != nil:
				// token counts follow the assistant message of the same turn
				if pending != nil && pending.Role == agentio.RoleAssistant && pending.Tokens == nil {
					n := ev.Info.LastTokenUsage.Neutral()
					pending.Tokens = &n
				}
			}
		}
	}
	if err := flush(); err != nil {
		return r.Offset(), err
	}
	return r.Offset(), nil
}

func itemRole(t ItemType) (agentio.Role, bool) {
	switch t {
	case ItemUserMessage:
		return agentio.RoleUser, true
	case ItemAgentMessage:
		return agentio.RoleAssistant, true
	}
	return 0, false
}
