package pi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
)

// TurnsAt reads the user and assistant messages of the session file at
// path from byte offset from to the last complete line, calling fn per
// message, and returns the offset after the last complete line read (a
// half-written line is left for the next call).
//
//   - Turn.ID is the entry id. pi keeps every branch in one file (each
//     entry names its parentId), so messages of every branch are emitted in
//     file order; a caller that wants one branch follows parentId in Raw
//     from the leaf it shows.
//   - Tokens come from an assistant message's usage, written on the same
//     line, so no message waits for a later line.
//   - ConversationID and Dir come from the session header (first line),
//     read on every call. AgentVersion is "": a session file records its
//     format version, not pi's.
//
// Disk · Read (streaming, plus a bounded header read).
func (a *Agent) TurnsAt(path string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	hdr, err := readHeader(path)
	if errors.Is(err, errHeaderIncomplete) {
		return from, nil // pi is still writing the header: nothing to read yet
	}
	if err != nil {
		return from, err
	}
	return turnsFile(path, hdr, from, fn)
}

// errHeaderIncomplete means the first line has no newline yet.
var errHeaderIncomplete = errors.New("pi: session header not written yet")

// readHeader reads a session file's first complete line as its header.
// errHeaderIncomplete when that line is not finished; ErrNotFound when the
// file is missing or the line is not a session header. Disk · Read
// (bounded by headLimit).
func readHeader(path string) (SessionHeader, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return SessionHeader{}, fmt.Errorf("%w: pi: %w", agentio.ErrNotFound, err)
	}
	if err != nil {
		return SessionHeader{}, fmt.Errorf("pi: read %s: %w", path, err)
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	line, err := bufio.NewReaderSize(io.LimitReader(f, headLimit), scanStartBytes).ReadBytes('\n')
	if errors.Is(err, io.EOF) {
		return SessionHeader{}, errHeaderIncomplete
	}
	if err != nil {
		return SessionHeader{}, fmt.Errorf("pi: read %s: %w", path, err)
	}
	var hdr SessionHeader
	if err := json.Unmarshal(agentio.StripBOM(line), &hdr); err != nil || hdr.Type != entrySession {
		return SessionHeader{}, fmt.Errorf("%w: pi: %s: first line is not a session header", agentio.ErrNotFound, path)
	}
	return hdr, nil
}

// turnsFile streams path from from, stamping turns with hdr. Disk · Read.
func turnsFile(path string, hdr SessionHeader, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	f, err := os.Open(path)
	if err != nil {
		return from, fmt.Errorf("pi: open %s: %w", path, err)
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
		t, ok := turnOf(line, hdr)
		if !ok {
			continue
		}
		if err := fn(t); err != nil {
			return off, err
		}
	}
}

// SessionOwner maps a session file path to its agent dir (home) and
// session id, from the path and the file's header only:
//   - <agent dir>/sessions/--<cwd>--/<file>.jsonl names its agent dir;
//   - a file directly in a custom flat session dir belongs to the home
//     whose store is that dir (PI_CODING_AGENT_SESSION_DIR or settings
//     sessionDir), checked for the PI_CODING_AGENT_DIR home and the
//     default home only, never by listing other dirs.
//
// The id is the header's. ErrNotFound when the file is not a .jsonl whose
// first line is a session header, or no home's store holds it.
// Disk · Read (one bounded header read, settings.json for a flat dir).
func (a *Agent) SessionOwner(path string) (Home, string, error) {
	clean := filepath.Clean(path)
	if filepath.Ext(clean) != sessionExt {
		return Home{}, "", fmt.Errorf("%w: %s is not a pi session file", agentio.ErrNotFound, path)
	}
	hdr, err := readHeader(clean)
	if errors.Is(err, errHeaderIncomplete) {
		return Home{}, "", fmt.Errorf("%w: %w", agentio.ErrNotFound, err)
	}
	if err != nil {
		return Home{}, "", err
	}
	folder := filepath.Dir(clean)
	name := filepath.Base(folder)
	sessions := filepath.Dir(folder)
	if len(name) > 2*len(sessionDirMark) && strings.HasPrefix(name, sessionDirMark) && strings.HasSuffix(name, sessionDirMark) && filepath.Base(sessions) == sessionsDir {
		return a.HomeAt(filepath.Dir(sessions)), hdr.ID, nil
	}
	candidates := []Home{a.DefaultHome()}
	if h, ok := a.HomeFromEnv(); ok {
		candidates = append([]Home{h}, candidates...)
	}
	for _, h := range candidates {
		store, err := a.SessionStore(h)
		if err == nil && store.Flat && filepath.Clean(store.Root) == folder {
			return h, hdr.ID, nil
		}
	}
	return Home{}, "", fmt.Errorf("%w: %s is not in a pi session store", agentio.ErrNotFound, path)
}
