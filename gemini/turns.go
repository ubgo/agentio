package gemini

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
)

// MessageTokens is a gemini message's `tokens` (chatRecordingService
// recordMessageTokens: the response's usageMetadata counts).
type MessageTokens struct {
	Input    int64 `json:"input"`
	Output   int64 `json:"output"`
	Cached   int64 `json:"cached"`
	Thoughts int64 `json:"thoughts"`
	Tool     int64 `json:"tool"`
	Total    int64 `json:"total"`
}

// Neutral maps the counts onto agentio.TokenCounts: Input is input plus
// tool (toolUsePromptTokenCount is prompt input spent on tool use, outside
// promptTokenCount), Cached is CachedInput, Thoughts is Reasoning; Total
// is dropped (derivable). Pure.
func (t MessageTokens) Neutral() agentio.TokenCounts {
	return agentio.TokenCounts{Input: t.Input + t.Tool, CachedInput: t.Cached, Output: t.Output, Reasoning: t.Thoughts}
}

// chatMessage is the part of a message record a turn needs. Patch marks a
// $patch record, which also carries ids but is not a message
// (isMessageRecord in the CLI).
type chatMessage struct {
	ID        string          `json:"id"`
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Content   json.RawMessage `json:"content"`
	Model     string          `json:"model"`
	Tokens    *MessageTokens  `json:"tokens"`
	Patch     json.RawMessage `json:"$patch"`
}

// legacyChat is a pre-JSONL single-object chat.
type legacyChat struct {
	SessionID string            `json:"sessionId"`
	Messages  []json.RawMessage `json:"messages"`
}

// TurnsAt reads the user and model messages of the chat file at path from
// cursor from, calling fn once per message record, and returns the cursor
// to pass back.
//
//   - Turn.ID is the message id. The CLI re-appends a whole message with
//     the same id when it adds tokens or tool calls to it (pushMessage), so
//     a re-appended message is emitted again with the same ID: a caller
//     that stores turns replaces the earlier one by ID.
//   - A user message with text and a gemini message with text or tokens
//     are turns; info, error and warning records are not. Tokens map
//     through MessageTokens.Neutral; Model is the message's model.
//   - $set, $rewindTo and $patch records emit nothing. A $rewindTo (or a
//     $patch removeIds) removes messages the CLI no longer shows, but turns
//     already emitted cannot be taken back through fn: a caller keeps the
//     removed messages, and the messages that follow a rewind carry new
//     ids. A $patch that rewrites a message's content is not re-emitted
//     either (it edits the model history, not what the user saw).
//   - A JSONL chat's cursor is the byte offset after the last complete
//     line; a half-written line is left for the next call. No message waits
//     for a later line, since late tokens arrive as a re-append.
//   - A legacy single-object .json chat has no lines: it is read whole (the
//     only way to read it) and its cursor is the count of messages read.
//     The CLI never appends to one; resuming it rewrites it as .jsonl.
//
// ConversationID is the chat's sessionId; Dir is the project dir from the
// chat's tmp/<slug>/.project_root (the chat itself records the project
// only as a hash), "" when the marker is missing. AgentVersion is "": a
// chat file records no CLI version. Disk · Read (streaming; a bounded first-line
// read for a read that starts past 0; legacy .json read whole).
func (a *Agent) TurnsAt(path string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	dir := chatProjectDir(path)
	withDir := func(t agentio.Turn) error {
		t.Dir = dir
		return fn(t)
	}
	if strings.HasSuffix(path, legacySessionFileExt) {
		return legacyTurns(path, from, withDir)
	}
	return jsonlTurns(path, from, withDir)
}

// maxChatDepth bounds the walk from a chat file up to its tmp/<slug> dir:
// tmp/<slug>/chats/<file> or, for a subagent chat,
// tmp/<slug>/chats/<parent id>/<file>.
const maxChatDepth = 3

// chatProjectDir is the project dir recorded in the .project_root of the
// tmp/<slug> dir holding the chat at path; "" when none is found within
// maxChatDepth parents. cost: Disk · Read (a small marker file).
func chatProjectDir(path string) string {
	dir := filepath.Dir(path)
	for range maxChatDepth {
		if b, err := os.ReadFile(filepath.Join(dir, projectRootFile)); err == nil {
			return strings.TrimSpace(string(b))
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

// jsonlTurns is TurnsAt over a JSONL chat. Disk · Read.
func jsonlTurns(path string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	f, err := os.Open(path)
	if err != nil {
		return from, fmt.Errorf("gemini: open %s: %w", path, err)
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	var sessionID string
	if from > 0 {
		m, err := firstMeta(f)
		if err != nil {
			return from, fmt.Errorf("gemini: %s: %w", path, err)
		}
		sessionID = m.SessionID
		if _, err := f.Seek(int64(from), io.SeekStart); err != nil {
			return from, fmt.Errorf("gemini: seek %s: %w", path, err)
		}
	}
	br := bufio.NewReaderSize(f, headLimit)
	off := from
	for {
		line, err := readLine(br)
		if errors.Is(err, io.EOF) {
			return off, nil
		}
		if err != nil {
			return off, fmt.Errorf("gemini: read %s: %w", path, err)
		}
		atStart := off == 0
		off += agentio.Offset(len(line))
		line = bytes.TrimSpace(agentio.StripBOM(line))
		if len(line) == 0 {
			continue
		}
		if atStart {
			var m metaLine
			if json.Unmarshal(line, &m) == nil {
				sessionID = m.SessionID
			}
			continue
		}
		t, ok := chatTurn(line, sessionID)
		if !ok {
			continue
		}
		if err := fn(t); err != nil {
			return off, err
		}
	}
}

// readLine returns one complete line with its newline; io.EOF when only a
// partial line (the CLI still writing) or nothing remains. A line over
// maxScanLine is an error. Disk · Read.
func readLine(br *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > maxScanLine {
			return nil, fmt.Errorf("chat line over %d bytes", maxScanLine)
		}
		switch {
		case err == nil:
			return buf, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			return nil, err
		}
	}
}

// firstMeta reads the metadata line at the start of f (bounded by
// maxScanLine). Disk · Read.
func firstMeta(f *os.File) (metaLine, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return metaLine{}, err
	}
	line, err := readLine(bufio.NewReaderSize(io.LimitReader(f, maxScanLine+1), headLimit))
	if err != nil {
		return metaLine{}, fmt.Errorf("metadata line: %w", err)
	}
	var m metaLine
	if err := json.Unmarshal(bytes.TrimSpace(agentio.StripBOM(line)), &m); err != nil {
		return metaLine{}, fmt.Errorf("metadata line: %w", err)
	}
	return m, nil
}

// legacyTurns is TurnsAt over a legacy .json chat; from counts messages.
// Disk · Read (whole file).
func legacyTurns(path string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return from, fmt.Errorf("gemini: read %s: %w", path, err)
	}
	var c legacyChat
	if err := json.Unmarshal(agentio.StripBOM(b), &c); err != nil {
		return from, fmt.Errorf("gemini: %s: %w", path, err)
	}
	n := agentio.Offset(len(c.Messages))
	if from >= n {
		return n, nil
	}
	for i := from; i < n; i++ {
		t, ok := chatTurn(c.Messages[i], c.SessionID)
		if !ok {
			continue
		}
		if err := fn(t); err != nil {
			return i + 1, err
		}
	}
	return n, nil
}

// chatTurn normalizes one message record; false for any other record or
// a message that is not a turn. Pure.
func chatTurn(line []byte, sessionID string) (agentio.Turn, bool) {
	var m chatMessage
	if json.Unmarshal(line, &m) != nil || m.ID == "" || m.Patch != nil {
		return agentio.Turn{}, false
	}
	t := agentio.Turn{ID: m.ID, ConversationID: sessionID, Time: parseTime(m.Timestamp), Text: contentText(m.Content), Raw: agentio.Raw(append([]byte(nil), line...))}
	switch m.Type {
	case msgTypeUser:
		if strings.TrimSpace(t.Text) == "" {
			return agentio.Turn{}, false
		}
		t.Role = agentio.RoleUser
	case msgTypeGemini:
		if strings.TrimSpace(t.Text) == "" && m.Tokens == nil {
			return agentio.Turn{}, false
		}
		t.Role, t.Model = agentio.RoleAssistant, m.Model
		if m.Tokens != nil {
			n := m.Tokens.Neutral()
			t.Tokens = &n
		}
	default:
		return agentio.Turn{}, false
	}
	return t, true
}

// ChatOwner maps a chat file path to its home and session id: the home is
// the folder above tmp/<slug>/chats/ (a subagent chat sits one folder
// deeper, chats/<parent id>/), the id is the sessionId of the metadata
// line, since the file name holds only its first 8 characters. It never
// lists other homes. ErrNotFound when the path is not a chat file under a
// tmp/<slug>/chats/ folder or its first line is not chat metadata.
// Disk · Read (one bounded first-line read).
func (a *Agent) ChatOwner(path string) (Home, string, error) {
	clean := filepath.Clean(path)
	ext := filepath.Ext(clean)
	if ext != sessionFileExt && ext != legacySessionFileExt {
		return Home{}, "", fmt.Errorf("%w: %s is not a Gemini chat", agentio.ErrNotFound, path)
	}
	home, ok := chatHomeDir(clean)
	if !ok {
		return Home{}, "", fmt.Errorf("%w: %s is not under a Gemini tmp/<project>/chats dir", agentio.ErrNotFound, path)
	}
	id, err := chatSessionID(clean, ext == legacySessionFileExt)
	if err != nil {
		return Home{}, "", fmt.Errorf("%w: %s: %w", agentio.ErrNotFound, path, err)
	}
	return a.HomeAt(home), id, nil
}

// chatHomeDir finds <home> in <home>/tmp/<slug>/chats/[<parent>/]<file>.
// Pure.
func chatHomeDir(path string) (string, bool) {
	chats := filepath.Dir(path)
	if filepath.Base(chats) != chatsDir {
		chats = filepath.Dir(chats) // a subagent chat: chats/<parent id>/<agent id>.jsonl
	}
	if filepath.Base(chats) != chatsDir {
		return "", false
	}
	tmp := filepath.Dir(filepath.Dir(chats))
	if filepath.Base(tmp) != tmpDir {
		return "", false
	}
	return filepath.Dir(tmp), true
}

// chatSessionID reads a chat's sessionId: the first line of a JSONL chat;
// a legacy .json chat is decoded as a stream until the key appears.
// Disk · Read (bounded for JSONL).
func chatSessionID(path string, legacy bool) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	var s Session
	if legacy {
		if err := readLegacy(f, &s); err != nil {
			return "", err
		}
	} else {
		m, err := firstMeta(f)
		if err != nil {
			return "", err
		}
		s.ID = m.SessionID
	}
	if s.ID == "" {
		return "", errors.New("no sessionId")
	}
	return s.ID, nil
}
