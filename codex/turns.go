package codex

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
)

// resumeContextBytes bounds the window read before a resume cursor for
// the last turn_context (model and cwd), so a read that starts mid-file
// still stamps its turns. A turn_context opens every turn, so the window
// only misses it inside one turn longer than this.
const resumeContextBytes = 256 << 10

// rolloutIDLen is the length of the uuid at the end of a rollout file name
// (rollout-<ts>-<uuid>.jsonl).
const rolloutIDLen = 36

// TurnsAt reads the turns of the rollout at path from cursor from, calling
// fn once per user or agent message, and returns the cursor to pass back.
// Both the 0.15x+ item_completed items and the older user_message /
// agent_message events are read; when a file has both, item_completed wins
// so a message is not reported twice.
//
//   - Turn.ID is the item id (item_completed); "" for the older events,
//     which carry none.
//   - An agent message whose Phase is interim (commentary) is emitted at
//     once with Interim set and no tokens: a viewer shows it, a caller that
//     stores answers skips it.
//   - An answer (final_answer, or no phase in older files) waits for its
//     token_count, which Codex writes a few lines later. token_count
//     attaches last_token_usage and releases it; task_complete,
//     turn_aborted, task_started, turn_context, session_meta, compacted or
//     the next message release it without tokens. response_item,
//     token_usage_record, world_state and unknown records do not, because
//     Codex writes them between the answer and its token_count.
//   - At the end of the file a waiting answer is not emitted, and the
//     returned cursor is the offset of the line that started it, so the
//     next call reads it again with its tokens. A session that crashed
//     right after its answer and never writes another line keeps that last
//     answer held back.
//
// A read that starts past 0 takes the folder and CLI version from the
// session_meta head and the model from the last turn_context within
// resumeContextBytes before the cursor. ConversationID comes from
// session_meta. Disk · Read (streaming, bounded head and window reads).
func (a *Agent) TurnsAt(path string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	return turnsFromPath(path, "", from, fn, nil)
}

// TurnsAtReporting is TurnsAt that also reports each skipped line to bad
// (nil = silent): a line that is not valid JSON (ErrInvalidLine) never
// stops the read, as in Claude's reader; the turns after it are read.
// Disk · Read.
func (a *Agent) TurnsAtReporting(path string, from agentio.Offset, fn func(agentio.Turn) error, bad func(agentio.BadLine)) (agentio.Offset, error) {
	return turnsFromPath(path, "", from, fn, bad)
}

// turnState is what earlier records say about the turns that follow.
type turnState struct {
	id, dir, model, version string
}

// seedState fills st for a read that starts at from > 0: session_meta
// from the head, the last turn_context before from in a bounded window.
// Disk · Read (bounded).
func seedState(path string, from agentio.Offset, st *turnState) {
	if m, ok := readHeadMeta(path); ok {
		if st.id == "" {
			st.id = m.ID
		}
		st.dir, st.version = m.Cwd, m.CLIVersion
	}
	if tc, ok := lastTurnContextBefore(path, from); ok {
		st.model = tc.Model
		if tc.Cwd != "" {
			st.dir = tc.Cwd
		}
	}
}

// lastTurnContextBefore decodes the complete lines in the window before
// from and returns the last turn_context. Disk · Read (bounded).
func lastTurnContextBefore(path string, from agentio.Offset) (TurnContext, bool) {
	start := max(int64(from)-resumeContextBytes, 0)
	b, err := readRange(path, start, int64(from))
	if err != nil {
		return TurnContext{}, false
	}
	if start > 0 {
		i := bytes.IndexByte(b, newline)
		if i < 0 {
			return TurnContext{}, false
		}
		b = b[i+1:] // the first line is cut by the window
	}
	var last *TurnContext
	for _, line := range bytes.Split(b, []byte{newline}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if rec, err := ParseRecord(line); err == nil && rec.TurnContext != nil {
			last = rec.TurnContext
		}
	}
	if last == nil {
		return TurnContext{}, false
	}
	return *last, true
}

// readRange returns the bytes [start, end) of path. Disk · Read (bounded).
func readRange(path string, start, end int64) ([]byte, error) {
	r, err := openRolloutAt(path, agentio.Offset(start))
	if err != nil {
		return nil, err
	}
	// Read-only file: a close error cannot lose data.
	defer func() { _ = r.Close() }()
	return io.ReadAll(io.LimitReader(r.f, end-start))
}

// turnsFromPath is TurnsAt with a known conversation id ("" = take it from
// session_meta). A line that is not valid JSON is skipped and reported to
// bad (when set); a read error or an over-long line still stops the read.
func turnsFromPath(path, id string, from agentio.Offset, fn func(agentio.Turn) error, bad func(agentio.BadLine)) (agentio.Offset, error) {
	tr := &turnRead{st: turnState{id: id}, fn: fn}
	if from > 0 {
		seedState(path, from, &tr.st)
	}
	r, err := openRolloutAt(path, from)
	if err != nil {
		return from, err
	}
	// Read-only file: a close error cannot lose data.
	defer func() { _ = r.Close() }()
	for {
		at := r.Offset()
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			return tr.cursor(r.Offset()), nil
		}
		if errors.Is(err, ErrInvalidLine) {
			if bad != nil {
				bad(agentio.BadLine{Path: path, Offset: at, Err: err})
			}
			continue
		}
		if err != nil {
			return tr.cursor(r.Offset()), err
		}
		if err := tr.apply(rec, at); err != nil {
			return tr.cursor(r.Offset()), err
		}
	}
}

// turnRead is one TurnsAt pass: the state earlier records set and the
// answer held for its token count.
type turnRead struct {
	st       turnState
	fn       func(agentio.Turn) error
	held     *agentio.Turn
	heldAt   agentio.Offset
	sawItems bool
}

// cursor is the offset to return: never past a held answer, so the next
// call starts at its line. Pure.
func (tr *turnRead) cursor(read agentio.Offset) agentio.Offset {
	if tr.held != nil {
		return tr.heldAt
	}
	return read
}

// release emits the held answer, if any.
func (tr *turnRead) release() error {
	if tr.held == nil {
		return nil
	}
	t := *tr.held
	tr.held = nil
	return tr.fn(t)
}

// message emits a user turn or an interim agent message at once and holds
// an answer (started at offset at) for its token count.
func (tr *turnRead) message(t agentio.Turn, at agentio.Offset) error {
	if err := tr.release(); err != nil {
		return err
	}
	if t.Role == agentio.RoleUser || t.Interim {
		return tr.fn(t)
	}
	tr.held, tr.heldAt = &t, at
	return nil
}

// apply folds one record (starting at offset at) into the read: state
// updates, messages, token counts and the records that release a held
// answer.
func (tr *turnRead) apply(rec Record, at agentio.Offset) error {
	st := &tr.st
	switch {
	case rec.SessionMeta != nil:
		if st.id == "" {
			st.id = rec.SessionMeta.ID
		}
		st.dir, st.version = rec.SessionMeta.Cwd, rec.SessionMeta.CLIVersion
		return tr.release()
	case rec.TurnContext != nil:
		st.model = rec.TurnContext.Model
		if rec.TurnContext.Cwd != "" {
			st.dir = rec.TurnContext.Cwd
		}
		return tr.release()
	case rec.Compacted != nil:
		return tr.release()
	case rec.Event == nil:
		return nil // response_item, token_usage_record, world_state, unknown kinds
	}
	ev := rec.Event
	base := agentio.Turn{Time: rec.Time, ConversationID: st.id, Dir: st.dir, Model: st.model, AgentVersion: st.version, Raw: rec.Raw}
	switch {
	case ev.Type == EventItemCompleted && ev.Item != nil:
		role, ok := itemRole(ev.Item.Type)
		if !ok {
			return nil
		}
		tr.sawItems = true
		base.ID, base.Role, base.Text = ev.Item.ID, role, ev.Item.Text()
		base.Interim = role == agentio.RoleAssistant && ev.Item.Phase.Interim()
		return tr.message(base, at)
	case ev.Type == EventUserMessage && !tr.sawItems:
		base.Role, base.Text = agentio.RoleUser, ev.Message
		return tr.message(base, at)
	case ev.Type == EventAgentMessage && !tr.sawItems:
		base.Role, base.Text, base.Interim = agentio.RoleAssistant, ev.Message, ev.Phase.Interim()
		return tr.message(base, at)
	case ev.Type == EventTokenCount:
		if ev.Info == nil || ev.Info.LastTokenUsage == nil || tr.held == nil {
			return nil // an early token_count without info pairs with nothing
		}
		n := ev.Info.LastTokenUsage.Neutral()
		tr.held.Tokens = &n
		return tr.release()
	case ev.Type == EventTaskComplete, ev.Type == EventTurnAborted, ev.Type == EventTaskStarted:
		return tr.release()
	}
	return nil
}

// itemRole maps a message item type onto a turn side.
func itemRole(t ItemType) (agentio.Role, bool) {
	switch t {
	case ItemUserMessage:
		return agentio.RoleUser, true
	case ItemAgentMessage:
		return agentio.RoleAssistant, true
	}
	return 0, false
}

// RolloutOwner maps a rollout path to the Codex home whose sessions/ or
// archived_sessions/ tree holds it and to its conversation id, from the
// path alone: the uuid at the end of the file name, else the session_meta
// head when the name does not end in one. It never lists other homes.
// ErrNotFound when the name is not a rollout's, no sessions/ or
// archived_sessions/ folder is above it, or the folder above holds no
// Codex home marker. Disk · Read (marker stat, at most one head read).
func (a *Agent) RolloutOwner(path string) (Home, string, error) {
	clean := filepath.Clean(path)
	if !isRolloutName(filepath.Base(clean)) {
		return Home{}, "", fmt.Errorf("%w: %s is not a Codex rollout", agentio.ErrNotFound, path)
	}
	root, ok := rolloutRoot(clean)
	if !ok {
		return Home{}, "", fmt.Errorf("%w: %s is not under a Codex sessions dir", agentio.ErrNotFound, path)
	}
	h, err := a.HomeAt(filepath.Dir(root))
	if err != nil {
		return Home{}, "", err
	}
	if id, ok := rolloutNameID(filepath.Base(clean)); ok {
		return h, id, nil
	}
	m, ok := readHeadMeta(clean)
	if !ok {
		return Home{}, "", fmt.Errorf("%w: %s has no session_meta", agentio.ErrNotFound, path)
	}
	return h, m.ID, nil
}

// maxRolloutDepth is how many folders RolloutOwner climbs looking for the
// sessions root: sessions/YYYY/MM/DD/ is three below it.
const maxRolloutDepth = 4

// rolloutRoot finds the sessions/ or archived_sessions/ folder above a
// rollout path. Pure.
func rolloutRoot(path string) (string, bool) {
	d := filepath.Dir(path)
	for range maxRolloutDepth {
		switch filepath.Base(d) {
		case dirSessions, dirArchivedSessions:
			return d, true
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return "", false
}

// rolloutNameID is the uuid at the end of a rollout file name; false when
// the name does not end in one. Pure.
func rolloutNameID(name string) (string, bool) {
	stem := strings.TrimSuffix(name, rolloutExt)
	if len(stem) < rolloutIDLen {
		return "", false
	}
	id := stem[len(stem)-rolloutIDLen:]
	if !isUUIDShape(id) {
		return "", false
	}
	return id, true
}

// isUUIDShape reports the 8-4-4-4-12 hex layout. Pure.
func isUUIDShape(s string) bool {
	const hex = "0123456789abcdefABCDEF"
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !strings.ContainsRune(hex, c) {
				return false
			}
		}
	}
	return len(s) == rolloutIDLen
}
