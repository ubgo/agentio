package gemini

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
	"unicode/utf8"

	"github.com/ubgo/agentio"
)

// SessionKind is the metadata kind of a chat file. Vendor-owned open set.
type SessionKind string

// SessionKind values (core chatRecordingService).
const (
	SessionMain     SessionKind = "main"
	SessionSubagent SessionKind = "subagent"
)

// Record types of a chat JSONL file (core chatRecordingService).
const (
	msgTypeUser   = "user"
	msgTypeGemini = "gemini"
	// slashPrefix and helpPrefix mark command lines the CLI skips when it
	// picks a title from the first user message.
	slashPrefix = "/"
	helpPrefix  = "?"
)

// Session is one chat file's facts, read from its head and tail only.
type Session struct {
	// ID is the full session id: what `gemini --resume <id>` accepts. The
	// resume is per project: launch with Dir set to Dir.
	ID string
	// ProjectHash is sha256 of the project dir (ProjectHash).
	ProjectHash string
	Kind        SessionKind
	// Slug is the tmp/<slug> dir the file lives in.
	Slug string
	// Dir is the project dir from tmp/<slug>/.project_root; "" when the
	// marker is missing.
	Dir string
	// Title is the newest $set summary, else the first user message that
	// is not a /command or ?help line (the CLI's own rule), truncated.
	Title string
	// Path is the chat file.
	Path string
	// Legacy is true for a pre-JSONL single-object .json chat.
	Legacy           bool
	Start, Updated   time.Time
	hasUserOrGemini  bool
	firstUserMessage string
	summary          string
}

// Neutral projects a session onto agentio.Conversation. cost: Pure.
func (s Session) Neutral(h Home) agentio.Conversation {
	return agentio.Conversation{
		Agent: ID, Home: h.Neutral(), ID: s.ID, Title: s.Title, Dir: s.Dir,
		Path: s.Path, Created: s.Start, Updated: s.Updated,
	}
}

// metaLine is the first line of a JSONL chat, and the top of a legacy
// .json chat (which also carries "messages").
type metaLine struct {
	SessionID   string          `json:"sessionId"`
	ProjectHash string          `json:"projectHash"`
	StartTime   string          `json:"startTime"`
	LastUpdated string          `json:"lastUpdated"`
	Kind        SessionKind     `json:"kind"`
	Summary     string          `json:"summary"`
	Messages    json.RawMessage `json:"messages"`
}

// recordLine is any later JSONL line: a message, or a $set / $rewindTo /
// $patch edit.
type recordLine struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content"`
	Set     *metaLine       `json:"$set"`
}

// part is one element of a content list ({"text": ...}).
type part struct {
	Text string `json:"text"`
}

// contentText is the text of a Gemini PartListUnion: a string or a list
// of parts. cost: Pure.
func contentText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var parts []part
	if err := json.Unmarshal(raw, &parts); err != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Text)
	}
	return b.String()
}

// parseTime reads the CLI's ISO timestamps; zero when absent or invalid.
// cost: Pure.
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// ReadSession reads a chat file's head (metadata and the first user
// message) and tail (the newest $set), never the whole JSONL file. A
// legacy .json chat is decoded as a stream. cost: Disk · Read.
func ReadSession(path string) (Session, error) {
	s := Session{Path: path, Legacy: strings.HasSuffix(path, legacySessionFileExt)}
	f, err := os.Open(path)
	if err != nil {
		return s, fmt.Errorf("gemini: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if s.Legacy {
		err = readLegacy(f, &s)
	} else {
		err = readJSONL(f, &s)
	}
	if err != nil {
		return s, fmt.Errorf("gemini: %s: %w", path, err)
	}
	s.Title = s.summary
	if s.Title == "" {
		s.Title = truncateRunes(s.firstUserMessage, titleRunes)
	}
	return s, nil
}

// readJSONL fills s from the head and tail of a JSONL chat. Disk · Read.
func readJSONL(f *os.File, s *Session) error {
	head := make([]byte, headLimit)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return err
	}
	lines := bytes.Split(head[:n], []byte{'\n'})
	if n == headLimit && len(lines) > 1 {
		lines = lines[:len(lines)-1] // the last line may be cut
	}
	for i, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if i == 0 {
			var m metaLine
			if err := json.Unmarshal(line, &m); err != nil {
				return fmt.Errorf("metadata line: %w", err)
			}
			applyMeta(s, m)
			continue
		}
		applyRecord(s, line)
	}
	if fi, err := f.Stat(); err == nil && fi.Size() > int64(n) {
		return readTail(f, fi.Size(), s)
	}
	return nil
}

// readTail scans the last tailLimit bytes for $set edits and message
// records, so Updated and the summary are the newest. Disk · Read.
func readTail(f *os.File, size int64, s *Session) error {
	off := max(size-tailLimit, 0)
	if _, err := f.Seek(off, io.SeekStart); err != nil {
		return err
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxScanLine)
	first := off > 0
	for sc.Scan() {
		if first {
			first = false // a partial line
			continue
		}
		applyRecord(s, sc.Bytes())
	}
	return sc.Err()
}

// applyMeta copies the metadata fields that are set.
func applyMeta(s *Session, m metaLine) {
	if m.SessionID != "" {
		s.ID = m.SessionID
	}
	if m.ProjectHash != "" {
		s.ProjectHash = m.ProjectHash
	}
	if m.Kind != "" {
		s.Kind = m.Kind
	}
	if t := parseTime(m.StartTime); !t.IsZero() {
		s.Start = t
	}
	if t := parseTime(m.LastUpdated); !t.IsZero() {
		s.Updated = t
	}
	if m.Summary != "" {
		s.summary = m.Summary
	}
}

// applyRecord folds one JSONL record into s; an undecodable line is
// skipped (a later re-append replaces it in the CLI too).
func applyRecord(s *Session, line []byte) {
	var r recordLine
	if json.Unmarshal(line, &r) != nil {
		return
	}
	if r.Set != nil {
		applyMeta(s, *r.Set)
		return
	}
	switch r.Type {
	case msgTypeUser:
		text := strings.TrimSpace(contentText(r.Content))
		if text != "" {
			s.hasUserOrGemini = true
		}
		if s.firstUserMessage == "" && text != "" && !strings.HasPrefix(text, slashPrefix) && !strings.HasPrefix(text, helpPrefix) {
			s.firstUserMessage = text
		}
	case msgTypeGemini:
		s.hasUserOrGemini = true
	}
}

// readLegacy streams a legacy single-object chat: the metadata keys, and
// the messages array one element at a time. Disk · Read.
func readLegacy(r io.Reader, s *Session) error {
	dec := json.NewDecoder(r)
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return fmt.Errorf("legacy chat is not an object: %v", err)
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		if key != "messages" {
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return err
			}
			var m metaLine
			if obj, err := json.Marshal(map[string]json.RawMessage{key: raw}); err == nil && json.Unmarshal(obj, &m) == nil {
				applyMeta(s, m)
			}
			continue
		}
		if tok, err := dec.Token(); err != nil || tok != json.Delim('[') {
			return fmt.Errorf("legacy messages is not a list: %v", err)
		}
		for dec.More() {
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				return err
			}
			applyRecord(s, raw)
		}
		if _, err := dec.Token(); err != nil {
			return err
		}
	}
	return nil
}

// truncateRunes cuts s to n runes on one line. cost: Pure.
func truncateRunes(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// isSessionFile reports a main chat file name: session-*.jsonl or the
// legacy session-*.json (subagent chats live in subdirs). cost: Pure.
func isSessionFile(name string) bool {
	return strings.HasPrefix(name, sessionFilePrefix) &&
		(strings.HasSuffix(name, sessionFileExt) || strings.HasSuffix(name, legacySessionFileExt))
}

// Sessions lists the resumable main chats of h, newest Updated first:
//   - every tmp/<slug>/chats/session-*.jsonl (and legacy .json), or only
//     dir's project when dir is set;
//   - a file without a user or model message is skipped, as is a
//     subagent chat (the CLI's --list-sessions rules);
//   - of two files with one session id, the newer Updated wins.
//
// cost: Disk · Read (a head and a tail per file).
func (a *Agent) Sessions(ctx context.Context, h Home, dir string) ([]Session, error) {
	roots, err := a.projectRoots(h)
	if err != nil {
		return nil, err
	}
	slugs := make([]string, 0, len(roots))
	if dir != "" {
		slug, ok, err := a.ProjectSlug(h, dir)
		if err != nil || !ok {
			return nil, err
		}
		slugs = append(slugs, slug)
	} else {
		for slug := range roots {
			slugs = append(slugs, slug)
		}
		sort.Strings(slugs)
	}
	byID := map[string]Session{}
	for _, slug := range slugs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chats := filepath.Join(h.ProjectTmpDir(slug), chatsDir)
		entries, err := os.ReadDir(chats)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("gemini: read %s: %w", chats, err)
		}
		for _, e := range entries {
			if e.IsDir() || !isSessionFile(e.Name()) {
				continue
			}
			s, err := ReadSession(filepath.Join(chats, e.Name()))
			if err != nil || s.ID == "" || !s.hasUserOrGemini || s.Kind == SessionSubagent {
				continue // unreadable or not resumable: the CLI skips it too
			}
			s.Slug, s.Dir = slug, roots[slug]
			if old, ok := byID[s.ID]; !ok || s.Updated.After(old.Updated) {
				byID[s.ID] = s
			}
		}
	}
	out := make([]Session, 0, len(byID))
	for _, s := range byID {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Updated.Equal(out[j].Updated) {
			return out[i].Updated.After(out[j].Updated)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// Session finds one chat by its full id. ErrNotFound when absent.
// cost: Disk · Read.
func (a *Agent) Session(ctx context.Context, h Home, id string) (Session, error) {
	all, err := a.Sessions(ctx, h, "")
	if err != nil {
		return Session{}, err
	}
	for _, s := range all {
		if s.ID == id {
			return s, nil
		}
	}
	return Session{}, fmt.Errorf("%w: gemini session %s", agentio.ErrNotFound, id)
}

// HasHistory reports whether dir has a chat in h, from the slug registry
// and the chats dir listing; it never opens a chat. cost: Disk · Read.
func (a *Agent) HasHistory(h Home, dir string) (bool, error) {
	slug, ok, err := a.ProjectSlug(h, dir)
	if err != nil || !ok {
		return false, err
	}
	entries, err := os.ReadDir(filepath.Join(h.ProjectTmpDir(slug), chatsDir))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("gemini: history %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() && isSessionFile(e.Name()) {
			return true, nil
		}
	}
	return false, nil
}
