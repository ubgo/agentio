package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/ubgo/agentio"
)

// maxRecordBytes caps one legacy JSON file read (a tool part can hold a
// whole command output), so a corrupted multi-GB file never lands in
// memory.
const maxRecordBytes = 32 << 20

// SessionTime is a session's time object.
type SessionTime struct {
	Created Millis `json:"created"`
	Updated Millis `json:"updated"`
}

// Share is a shared session's public link (0.x sessions).
type Share struct {
	URL string `json:"url"`
}

// Session is one legacy session file (storage/session/<projectID>/<id>.json):
// the id, the opencode version that created it, the project, the
// directory it ran in, the title and its times. Keys not modelled are kept
// in Unknown.
type Session struct {
	ID        string      `json:"id"`
	Version   string      `json:"version"`
	ProjectID string      `json:"projectID"`
	Directory string      `json:"directory"`
	Title     string      `json:"title"`
	ParentID  string      `json:"parentID,omitempty"`
	Time      SessionTime `json:"time"`
	Share     *Share      `json:"share,omitempty"`
	// Path is the session file.
	Path string `json:"-"`
	// Unknown are the keys not modelled above, raw.
	Unknown agentio.Unknown `json:"-"`
}

// Neutral projects a session onto agentio.Conversation; Path is the
// session file. cost: Pure.
func (s Session) Neutral(h Home) agentio.Conversation {
	return agentio.Conversation{
		Agent: ID, Home: h.Neutral(), ID: s.ID, Title: s.Title, Dir: s.Directory,
		Path: s.Path, Created: s.Time.Created.Time(), Updated: s.Time.Updated.Time(),
	}
}

// StorageDir is the legacy JSON tree of h (storage/). cost: Pure.
func (h Home) StorageDir() string { return h.Path(storageDir) }

// readBounded reads a whole small file, refusing anything above
// maxRecordBytes. Disk · Read.
func readBounded(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRecordBytes {
		return nil, fmt.Errorf("opencode: %s is larger than %d bytes", path, maxRecordBytes)
	}
	return data, nil
}

// ReadSession reads one legacy session file. cost: Disk · Read.
func ReadSession(path string) (Session, error) {
	data, err := readBounded(path)
	if err != nil {
		return Session{}, fmt.Errorf("opencode: session %s: %w", path, err)
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return Session{}, fmt.Errorf("opencode: session %s: %w", path, err)
	}
	u, err := agentio.ExtractUnknownFor(data, reflect.TypeFor[Session]())
	if err != nil {
		return Session{}, fmt.Errorf("opencode: session %s: %w", path, err)
	}
	s.Path, s.Unknown = path, u
	return s, nil
}

// jsonFiles lists the *.json names in dir, sorted; a missing dir is empty.
// opencode ids are ascending (time-based), so name order is creation
// order. Disk · Read.
func jsonFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), jsonExt) {
			out = append(out, e.Name())
		}
	}
	slices.Sort(out)
	return out, nil
}

// sessionFiles lists every legacy session file of h, project by project.
// Disk · Read.
func sessionFiles(h Home) ([]string, error) {
	root := filepath.Join(h.StorageDir(), sessionDir)
	projects, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opencode: read %s: %w", root, err)
	}
	var out []string
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		dir := filepath.Join(root, p.Name())
		names, err := jsonFiles(dir)
		if err != nil {
			return nil, fmt.Errorf("opencode: read %s: %w", dir, err)
		}
		for _, n := range names {
			out = append(out, filepath.Join(dir, n))
		}
	}
	return out, nil
}

// Sessions lists the legacy sessions of h, newest Updated first (ties by
// id), filtered by q (Dir exact after cleaning, Since, Limit). A session
// file that does not decode is skipped. Sessions created after the 1.2.0
// migration live in opencode.db only (opencode/store). cost: Disk · Read
// (one small file per session).
func (a *Agent) Sessions(ctx context.Context, h Home, q agentio.ConversationQuery) ([]Session, error) {
	files, err := sessionFiles(h)
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s, err := ReadSession(f)
		if err != nil || s.ID == "" {
			continue // a half-written or foreign file is not a session
		}
		if q.Dir != "" && filepath.Clean(s.Directory) != filepath.Clean(q.Dir) {
			continue
		}
		if !q.Since.IsZero() && s.Time.Updated.Time().Before(q.Since) {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Time.Updated != out[j].Time.Updated {
			return out[i].Time.Updated > out[j].Time.Updated
		}
		return out[i].ID < out[j].ID
	})
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

// Session finds one legacy session by id. ErrNotFound when absent.
// cost: Disk · Read.
func (a *Agent) Session(h Home, id string) (Session, error) {
	files, err := sessionFiles(h)
	if err != nil {
		return Session{}, err
	}
	for _, f := range files {
		if filepath.Base(f) != id+jsonExt {
			continue
		}
		return ReadSession(f)
	}
	return Session{}, fmt.Errorf("%w: opencode session %s", agentio.ErrNotFound, id)
}

// HasHistory reports whether dir has a legacy session in h. It reads the
// small session files only, never a message. cost: Disk · Read.
func (a *Agent) HasHistory(ctx context.Context, h Home, dir string) (bool, error) {
	ss, err := a.Sessions(ctx, h, agentio.ConversationQuery{Dir: dir, Limit: 1})
	return len(ss) > 0, err
}

// readMessage reads one legacy message file with its parts. Disk · Read.
func readMessage(storage, sessionID, name string) (MessageRecord, error) {
	path := filepath.Join(storage, messageDir, sessionID, name)
	raw, err := readBounded(path)
	if err != nil {
		return MessageRecord{}, fmt.Errorf("opencode: message %s: %w", path, err)
	}
	m, err := DecodeMessage(raw)
	if err != nil {
		return MessageRecord{}, fmt.Errorf("opencode: %s: %w", path, err)
	}
	id := strings.TrimSuffix(name, jsonExt)
	r := MessageRecord{ID: id, SessionID: sessionID, Info: m, Raw: raw}
	pdir := filepath.Join(storage, partDir, id)
	names, err := jsonFiles(pdir)
	if err != nil {
		return MessageRecord{}, fmt.Errorf("opencode: parts %s: %w", pdir, err)
	}
	for _, n := range names {
		data, err := readBounded(filepath.Join(pdir, n))
		if err != nil {
			return MessageRecord{}, fmt.Errorf("opencode: part %s: %w", n, err)
		}
		p, err := DecodePart(data)
		if err != nil {
			return MessageRecord{}, fmt.Errorf("opencode: %s: %w", filepath.Join(pdir, n), err)
		}
		r.Parts = append(r.Parts, p)
	}
	return r, nil
}

// Messages reads every message of a legacy session with its parts, oldest
// first. It reads one file per message and part: call it for one session,
// not in a listing loop. cost: Disk · Read.
func (a *Agent) Messages(h Home, sessionID string) ([]MessageRecord, error) {
	return readMessages(h.StorageDir(), sessionID, 0)
}

// readMessages reads the messages of a session from position from on.
// Disk · Read.
func readMessages(storage, sessionID string, from agentio.Offset) ([]MessageRecord, error) {
	dir := filepath.Join(storage, messageDir, sessionID)
	names, err := jsonFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("opencode: read %s: %w", dir, err)
	}
	if int(from) >= len(names) {
		return nil, nil
	}
	out := make([]MessageRecord, 0, len(names)-int(from))
	for _, n := range names[from:] {
		r, err := readMessage(storage, sessionID, n)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// messageCount is the number of message files of a session. Disk · Read.
func messageCount(storage, sessionID string) (agentio.Offset, error) {
	names, err := jsonFiles(filepath.Join(storage, messageDir, sessionID))
	return agentio.Offset(len(names)), err
}

// Turns reads the user and assistant messages of a legacy session from
// cursor from (0 = the start), calling fn per message, and returns the new
// cursor.
//
// The cursor is the number of message files already passed, in file-name
// order (message ids are ascending, so that is creation order). Reading
// stops before the newest message while opencode is still writing it (an
// assistant message without time.completed or an error, a user message
// without parts), so its text and usage are reported once. A cursor past
// the end (messages deleted by a revert) emits nothing and returns the
// current count; a caller that must not miss a message dedups on Turn.ID
// and re-reads from 0. cost: Disk · Read (only the messages past from).
func (a *Agent) Turns(h Home, sessionID string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	s, err := a.Session(h, sessionID)
	if err != nil {
		return from, err
	}
	return turnsFor(h.StorageDir(), s, from, fn)
}

// turnsFor runs the cursor rule over a session of storage. Disk · Read.
func turnsFor(storage string, s Session, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	from = max(from, 0)
	n, err := messageCount(storage, s.ID)
	if err != nil {
		return from, fmt.Errorf("opencode: %w", err)
	}
	if from > n {
		return n, nil
	}
	recs, err := readMessages(storage, s.ID, from)
	if err != nil {
		return from, err
	}
	return EmitTurns(recs, from, TurnContext{Dir: s.Directory, Version: s.Version}, fn)
}

// TranscriptOwner maps a legacy transcript path to its home and session:
// the session file (<home>/storage/session/<projectID>/<id>.json), the
// session's message dir (<home>/storage/message/<id>) or one message file
// in it. ErrNotFound for any other path. cost: Pure (the path alone).
func (a *Agent) TranscriptOwner(path string) (agentio.TranscriptRef, error) {
	home, sid, ok := ownerOf(path)
	if !ok {
		return agentio.TranscriptRef{}, fmt.Errorf("%w: %s is not an opencode transcript", agentio.ErrNotFound, path)
	}
	return agentio.TranscriptRef{Agent: ID, Home: a.HomeAt(home).Neutral(), ConversationID: sid}, nil
}

// ownerOf splits a legacy transcript path into its data dir and session
// id. cost: Pure.
func ownerOf(path string) (home, sessionID string, ok bool) {
	p := filepath.Clean(path)
	if strings.HasSuffix(p, jsonExt) {
		base := strings.TrimSuffix(filepath.Base(p), jsonExt)
		parent := filepath.Dir(p)
		switch {
		case strings.HasPrefix(base, sessionIDPrefix):
			// storage/session/<projectID>/<ses>.json
			sessions := filepath.Dir(parent)
			storage := filepath.Dir(sessions)
			if filepath.Base(sessions) != sessionDir || filepath.Base(storage) != storageDir {
				return "", "", false
			}
			return storageHome(storage), base, true
		case strings.HasPrefix(base, messageIDPrefix):
			// storage/message/<ses>/<msg>.json
			p = parent
		default:
			return "", "", false
		}
	}
	sid := filepath.Base(p)
	msgs := filepath.Dir(p)
	if !strings.HasPrefix(sid, sessionIDPrefix) || filepath.Base(msgs) != messageDir || filepath.Base(filepath.Dir(msgs)) != storageDir {
		return "", "", false
	}
	return storageHome(filepath.Dir(msgs)), sid, true
}

// storageHome is the data dir above a storage dir. cost: Pure.
func storageHome(storage string) string { return filepath.Dir(storage) }

// TurnsAt is Turns over a legacy transcript path the caller holds (see
// TranscriptOwner for the accepted paths), with the same cursor. The
// session file is read for the directory and version; without it (a
// message dir whose session file is gone) turns carry neither.
// cost: Disk · Read.
func (a *Agent) TurnsAt(path string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	home, sid, ok := ownerOf(path)
	if !ok {
		return from, fmt.Errorf("%w: %s is not an opencode transcript", agentio.ErrNotFound, path)
	}
	h := a.HomeAt(home)
	s, err := a.Session(h, sid)
	if errors.Is(err, agentio.ErrNotFound) {
		s, err = Session{ID: sid}, nil
	}
	if err != nil {
		return from, err
	}
	return turnsFor(h.StorageDir(), s, from, fn)
}

// StorageMigration is the number in storage/migration: the last legacy
// storage migration opencode ran (2 on a tree last written by 1.1.x).
// found is false when the file is absent. cost: Disk · Read.
func (h Home) StorageMigration() (n int, found bool, err error) {
	data, err := readBounded(filepath.Join(h.StorageDir(), migrationFile))
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("opencode: %w", err)
	}
	if err := json.Unmarshal(data, &n); err != nil {
		return 0, true, fmt.Errorf("opencode: %s: %w", migrationFile, err)
	}
	return n, true, nil
}
