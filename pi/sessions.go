package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Session file format (PKG/docs/session-format.md, session-manager.js).
const (
	// SessionVersion is the header version pi 1.5.2 writes
	// (CURRENT_SESSION_VERSION, session-manager.js:11); older files are
	// migrated by pi on load, newer ones are read with a warning-free best
	// effort because the header and message shapes are additive.
	SessionVersion = 3
	// sessionDirMark wraps an encoded cwd: --<cwd>--.
	sessionDirMark = "--"
	// fileIDSep separates the timestamp from the id in a file name.
	fileIDSep = "_"
)

// Entry types read here (PKG/docs/session-format.md:62-286).
const (
	entrySession     = "session"
	entryMessage     = "message"
	entrySessionInfo = "session_info"
	entryModelChange = "model_change"
)

// Message roles read here (PKG/docs/message-types.md).
const (
	roleUser      = "user"
	roleAssistant = "assistant"
	contentText   = "text"
)

// EncodeSessionDir is the folder name pi keeps a cwd's sessions under:
// "--" + cwd without its leading "/" or "\", with every "/", "\" and ":"
// replaced by "-", + "--" (getDefaultSessionDirPath, session-manager.js:
// 291-296). Nothing else is replaced, and the cwd is not resolved through
// symlinks. "/work/proj" is "--work-proj--"; "C:\work\x" is
// "--C--work-x--". Pure; dir must already be absolute.
func EncodeSessionDir(dir string) string {
	s := dir
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, `\`) {
		s = s[1:]
	}
	s = strings.NewReplacer("/", "-", `\`, "-", ":", "-").Replace(s)
	return sessionDirMark + s + sessionDirMark
}

// SessionStore is where h keeps sessions: the default tree
// <agent-dir>/sessions/--<cwd>--/, or a custom flat dir (every cwd in one
// folder, filtered by the header cwd) from PI_CODING_AGENT_SESSION_DIR or
// settings sessionDir (main.js:599-602; --session-dir is per launch).
type SessionStore struct {
	Root string
	Flat bool
}

// SessionStore resolves h's session storage. A relative custom dir is an
// error: pi resolves it against its own working directory, which a reader
// cannot know. Disk · Read (settings.json).
func (a *Agent) SessionStore(h Home) (SessionStore, error) {
	custom, _ := a.host.Getenv(SessionDirEnv)
	if custom == "" {
		s, _, _, err := a.ReadSettings(h)
		if err != nil {
			return SessionStore{}, err
		}
		custom = s.SessionDir.Or("")
	}
	if custom == "" {
		return SessionStore{Root: a.Paths(h).Sessions}, nil
	}
	custom = a.expandTilde(custom)
	if !filepath.IsAbs(custom) {
		return SessionStore{}, fmt.Errorf("pi: session dir %q is relative; pi resolves it against its own working directory", custom)
	}
	return SessionStore{Root: custom, Flat: true}, nil
}

// expandTilde expands a leading ~ against Host.UserHome, as pi's
// normalizePath does. Pure.
func (a *Agent) expandTilde(p string) string {
	if p == "~" {
		return a.host.UserHome
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(a.host.UserHome, rest)
	}
	if rest, ok := strings.CutPrefix(p, `~\`); ok {
		return filepath.Join(a.host.UserHome, rest)
	}
	return p
}

// SessionHeader is a session file's first line.
type SessionHeader struct {
	Type      string    `json:"type"`
	Version   int       `json:"version"`
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Cwd       string    `json:"cwd"`
	// ParentSession is set for a fork or clone (the source file).
	ParentSession string `json:"parentSession,omitempty"`
}

// Session is one session file with what its head and tail say.
type Session struct {
	Home Home
	SessionHeader
	// Path is the .jsonl file; pi's --session accepts it as is.
	Path string
	// Updated is the file's modification time (pi's --continue orders by
	// it, session-manager.js:1366-1374).
	Updated time.Time
	// Name is the latest session_info name ("" when never named); pi shows
	// it instead of the first message.
	Name string
	// FirstPrompt and LastPrompt are user message texts from the head and
	// tail windows ("" when outside them).
	FirstPrompt, LastPrompt string
	// Model is "provider/model" of the last assistant message or model
	// change in the tail window.
	Model string
}

// Title is Name, else the first prompt, as pi's picker shows it. Pure.
func (s Session) Title() string {
	if s.Name != "" {
		return s.Name
	}
	return s.FirstPrompt
}

// entryWire is the part of a session line read here.
type entryWire struct {
	Type     string       `json:"type"`
	Name     string       `json:"name"`
	Provider string       `json:"provider"`
	ModelID  string       `json:"modelId"`
	Message  *messageWire `json:"message"`
}

// messageWire is a message entry's payload (message-types.md:67-145).
type messageWire struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Provider   string          `json:"provider"`
	Model      string          `json:"model"`
	StopReason string          `json:"stopReason"`
	Timestamp  millis          `json:"timestamp"`
	Usage      *UsageEntry     `json:"usage"`
}

// UsageEntry is an assistant message's usage (message-types.md:67-90).
type UsageEntry struct {
	Input       int64     `json:"input"`
	Output      int64     `json:"output"`
	CacheRead   int64     `json:"cacheRead"`
	CacheWrite  int64     `json:"cacheWrite"`
	Reasoning   int64     `json:"reasoning"`
	TotalTokens int64     `json:"totalTokens"`
	Cost        UsageCost `json:"cost"`
}

// UsageCost is the cost of one message in USD, from pi's model catalog
// unless TotalFromProvider.
type UsageCost struct {
	Input             float64 `json:"input"`
	Output            float64 `json:"output"`
	CacheRead         float64 `json:"cacheRead"`
	CacheWrite        float64 `json:"cacheWrite"`
	Total             float64 `json:"total"`
	TotalFromProvider bool    `json:"totalFromProvider"`
}

// text joins a message's text parts; content is a string or a list of
// parts (message-types.md). Pure.
func (m messageWire) text() string {
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(m.Content, &parts) != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		if p.Type == contentText {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// sessionFiles lists the .jsonl files under store for dir ("" = every
// folder). Disk · Read (directory listings only).
func sessionFiles(store SessionStore, dir string) ([]string, error) {
	var dirs []string
	switch {
	case store.Flat:
		dirs = []string{store.Root}
	case dir != "":
		dirs = []string{filepath.Join(store.Root, EncodeSessionDir(dir))}
	default:
		entries, err := os.ReadDir(store.Root)
		if isNotExist(err) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("pi: list %s: %w", store.Root, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, filepath.Join(store.Root, e.Name()))
			}
		}
	}
	var out []string
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if isNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("pi: list %s: %w", d, err)
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), sessionExt) {
				out = append(out, filepath.Join(d, e.Name()))
			}
		}
	}
	return out, nil
}

// ReadSession reads one session file: the header (first line), the head
// window for the first prompt and the tail window for name, last prompt
// and model. Never the whole transcript. Disk · Read.
func (a *Agent) ReadSession(h Home, path string) (Session, error) {
	line, err := firstLine(path)
	if err != nil {
		return Session{}, fmt.Errorf("pi: read %s: %w", path, err)
	}
	var hdr SessionHeader
	if err := json.Unmarshal(line, &hdr); err != nil || hdr.Type != entrySession {
		return Session{}, fmt.Errorf("pi: %s: first line is not a session header", path)
	}
	s := Session{Home: h, SessionHeader: hdr, Path: path}
	if fi, err := os.Stat(path); err == nil {
		s.Updated = fi.ModTime().UTC()
	}
	head, err := readHead(path, tailBytes)
	if err != nil {
		return Session{}, fmt.Errorf("pi: read %s: %w", path, err)
	}
	for _, l := range splitLines(head) {
		var e entryWire
		if json.Unmarshal(l, &e) == nil && e.Type == entryMessage && e.Message != nil && e.Message.Role == roleUser {
			s.FirstPrompt = e.Message.text()
			break
		}
	}
	tail, err := readTail(path, tailBytes)
	if err != nil {
		return Session{}, fmt.Errorf("pi: read %s: %w", path, err)
	}
	for _, l := range splitLines(tail) {
		var e entryWire
		if json.Unmarshal(l, &e) != nil {
			continue
		}
		switch e.Type {
		case entrySessionInfo:
			s.Name = e.Name
		case entryModelChange:
			s.Model = e.Provider + "/" + e.ModelID
		case entryMessage:
			if e.Message == nil {
				continue
			}
			switch e.Message.Role {
			case roleUser:
				s.LastPrompt = e.Message.text()
			case roleAssistant:
				if e.Message.Model != "" {
					s.Model = e.Message.Provider + "/" + e.Message.Model
				}
			}
		}
	}
	return s, nil
}

// readHead reads at most n bytes from the start of path, dropping a
// trailing partial line. Disk · Read.
func readHead(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	buf, err := io.ReadAll(io.LimitReader(f, n))
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) == n {
		if i := strings.LastIndexByte(string(buf), '\n'); i >= 0 {
			buf = buf[:i+1]
		}
	}
	return buf, nil
}

// Sessions lists h's sessions, newest first by modification time. q.Dir
// narrows to one cwd (its encoded folder, or the header cwd in a flat
// custom dir); q.Since and q.Limit apply after sorting. A file whose first
// line is not a session header is skipped (pi writes the header first,
// session-manager.js:786-815). Disk · Read.
func (a *Agent) Sessions(h Home, q agentio.ConversationQuery) ([]Session, error) {
	store, err := a.SessionStore(h)
	if err != nil {
		return nil, err
	}
	dir, err := absDir(q.Dir)
	if err != nil {
		return nil, err
	}
	files, err := sessionFiles(store, dir)
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, f := range files {
		s, err := a.ReadSession(h, f)
		if err != nil {
			continue
		}
		if store.Flat && dir != "" && filepath.Clean(s.Cwd) != dir {
			continue
		}
		if !q.Since.IsZero() && s.Updated.Before(q.Since) {
			continue
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

// FindSession finds a session by its exact id across every folder: the
// file name ends in _<id>.jsonl (session-manager.js:697-715), confirmed by
// the header. ErrNotFound when absent. Disk · Read.
func (a *Agent) FindSession(h Home, id string) (Session, error) {
	store, err := a.SessionStore(h)
	if err != nil {
		return Session{}, err
	}
	files, err := sessionFiles(store, "")
	if err != nil {
		return Session{}, err
	}
	for _, f := range files {
		if !strings.HasSuffix(filepath.Base(f), fileIDSep+id+sessionExt) {
			continue
		}
		if s, err := a.ReadSession(h, f); err == nil && s.ID == id {
			return s, nil
		}
	}
	return Session{}, fmt.Errorf("pi: session %s: %w", id, agentio.ErrNotFound)
}

// conversation projects a Session onto the neutral type.
func conversation(s Session) agentio.Conversation {
	return agentio.Conversation{
		Agent: ID, Home: s.Home.Neutral(), ID: s.ID, Title: s.Title(), Dir: s.Cwd,
		Path: s.Path, Created: s.Timestamp, Updated: s.Updated,
	}
}

// Conversations is the neutral listing. Disk · Read.
func (a *Agent) Conversations(_ context.Context, h Home, q agentio.ConversationQuery) ([]agentio.Conversation, error) {
	ss, err := a.Sessions(h, q)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.Conversation, 0, len(ss))
	for _, s := range ss {
		out = append(out, conversation(s))
	}
	return out, nil
}

// HasHistory reports whether dir has at least one session file in h. It
// lists the folder only, never reads a transcript. Disk · Read.
func (a *Agent) HasHistory(h Home, dir string) (bool, error) {
	store, err := a.SessionStore(h)
	if err != nil {
		return false, err
	}
	abs, err := absDir(dir)
	if err != nil {
		return false, err
	}
	if store.Flat {
		ss, err := a.Sessions(h, agentio.ConversationQuery{Dir: abs, Limit: 1})
		return len(ss) > 0, err
	}
	files, err := sessionFiles(store, abs)
	return len(files) > 0, err
}

// Turns reads user and assistant messages of a session from offset from
// to the last complete line, calling fn per message; it returns the offset
// after the last complete line read. Tokens come from an assistant
// message's usage. Disk · Read.
func (a *Agent) Turns(h Home, id string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	s, err := a.FindSession(h, id)
	if err != nil {
		return from, err
	}
	f, err := os.Open(s.Path)
	if err != nil {
		return from, fmt.Errorf("pi: open %s: %w", s.Path, err)
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	if _, err := f.Seek(int64(from), io.SeekStart); err != nil {
		return from, err
	}
	r := bufio.NewReaderSize(f, scanStartBytes)
	off := from
	for {
		line, err := r.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return off, nil // a partial trailing line is left for the next read
		}
		if err != nil {
			return off, err
		}
		off += agentio.Offset(len(line))
		t, ok := turnOf(line, s)
		if !ok {
			continue
		}
		if err := fn(t); err != nil {
			return off, err
		}
	}
}

// turnOf converts a message line to a neutral turn. Pure.
func turnOf(line []byte, s Session) (agentio.Turn, bool) {
	var e entryWire
	if json.Unmarshal(line, &e) != nil || e.Type != entryMessage || e.Message == nil {
		return agentio.Turn{}, false
	}
	m := e.Message
	t := agentio.Turn{ConversationID: s.ID, Dir: s.Cwd, Time: m.Timestamp.Time(), Text: m.text(), StopReason: m.StopReason, Raw: agentio.Raw(append([]byte(nil), line...))}
	switch m.Role {
	case roleUser:
		t.Role = agentio.RoleUser
	case roleAssistant:
		t.Role = agentio.RoleAssistant
		if m.Model != "" {
			t.Model = m.Provider + "/" + m.Model
		}
		if u := m.Usage; u != nil {
			t.Tokens = &agentio.TokenCounts{Input: u.Input, CachedInput: u.CacheRead, CacheWrite: u.CacheWrite, Output: u.Output, Reasoning: u.Reasoning}
		}
	default:
		return agentio.Turn{}, false
	}
	return t, true
}

// SessionUsage is the token and cost total of a session's assistant
// messages. pi has no plan or quota; this is the only usage it records.
type SessionUsage struct {
	Messages int
	Tokens   agentio.TokenCounts
	// CostUSD sums each message's cost.total.
	CostUSD float64
}

// SessionUsage sums every assistant message's usage in the file (all
// branches: pi keeps them in one file). It reads the whole file line by
// line, so call it for one session, not in a listing loop. Disk · Read.
func (a *Agent) SessionUsage(h Home, id string) (SessionUsage, error) {
	var u SessionUsage
	_, err := a.Turns(h, id, 0, func(t agentio.Turn) error {
		if t.Role != agentio.RoleAssistant || t.Tokens == nil {
			return nil
		}
		u.Messages++
		u.Tokens = u.Tokens.Add(*t.Tokens)
		var e entryWire
		if json.Unmarshal(t.Raw, &e) == nil && e.Message != nil && e.Message.Usage != nil {
			u.CostUSD += e.Message.Usage.Cost.Total
		}
		return nil
	})
	return u, err
}
