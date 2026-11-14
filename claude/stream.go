package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/khanakia/agentcli"
)

// Stream message types and subtypes (docs `headless`, agent-sdk). OD-11:
// until one real stream is recorded, only the envelope and the docs-backed
// accessors are typed.
const (
	streamTypeSystem     = "system"
	streamTypeAssistant  = "assistant"
	streamTypeUser       = "user"
	streamTypeResult     = "result"
	streamTypeEvent      = "stream_event"
	streamSubtypeInit    = "init"
	streamSubtypeRetry   = "api_retry"
	streamSubtypeTaskNtf = "task_notification"
)

// StreamMessage is one line of `--output-format stream-json`.
type StreamMessage struct {
	Type            string       `json:"type"`
	Subtype         string       `json:"subtype,omitempty"`
	SessionID       string       `json:"session_id,omitempty"`
	UUID            string       `json:"uuid,omitempty"`
	ParentToolUseID *string      `json:"parent_tool_use_id,omitempty"`
	Raw             agentcli.Raw `json:"-"`
}

// StreamDecoder reads stream-json lines a caller feeds it.
type StreamDecoder struct{ sc *bufio.Scanner }

// NewStreamDecoder wraps r (16 MiB max line). cost: Pure.
func NewStreamDecoder(r io.Reader) *StreamDecoder {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, scanStartBytes), maxLineBytes)
	return &StreamDecoder{sc: sc}
}

// Next returns the next message; io.EOF at the end. Blank lines are
// skipped; a non-JSON line is an error. cost: Pure (reads the caller's stream).
func (d *StreamDecoder) Next() (StreamMessage, error) {
	for d.sc.Scan() {
		line := bytes.TrimSpace(d.sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var m StreamMessage
		if err := json.Unmarshal(line, &m); err != nil {
			return StreamMessage{}, fmt.Errorf("claude: stream line: %w", err)
		}
		m.Raw = agentcli.Raw(bytes.Clone(line))
		return m, nil
	}
	if err := d.sc.Err(); err != nil {
		return StreamMessage{}, err
	}
	return StreamMessage{}, io.EOF
}

// streamPayload is the subset of message and result fields Neutral reads.
type streamPayload struct {
	Message *struct {
		Content Content `json:"content"`
	} `json:"message"`
	Result  string        `json:"result"`
	IsError bool          `json:"is_error"`
	Usage   *MessageUsage `json:"usage"`
}

// Neutral maps onto the shared run event: system/init → Started, assistant
// and stream_event → Message, user with tool results → Tool, result → Done
// (Error when is_error), everything else Other. Raw is always the line.
// cost: Pure.
func (m StreamMessage) Neutral() agentcli.RunEvent {
	ev := agentcli.RunEvent{Type: agentcli.RunEventOther, ConversationID: m.SessionID, Raw: m.Raw}
	var p streamPayload
	_ = json.Unmarshal(m.Raw, &p) // a payload shape change leaves Text/Tokens empty; Type and Raw still hold
	switch m.Type {
	case streamTypeSystem:
		if m.Subtype == streamSubtypeInit {
			ev.Type = agentcli.RunEventStarted
		}
	case streamTypeAssistant:
		ev.Type = agentcli.RunEventMessage
		if p.Message != nil {
			ev.Text = p.Message.Content.Text()
		}
	case streamTypeEvent:
		ev.Type = agentcli.RunEventMessage
	case streamTypeUser:
		if p.Message != nil {
			for _, b := range p.Message.Content {
				if b.Type == BlockToolResult {
					ev.Type = agentcli.RunEventTool
				}
			}
		}
	case streamTypeResult:
		ev.Type, ev.Text = agentcli.RunEventDone, p.Result
		if p.IsError {
			ev.Type = agentcli.RunEventError
		}
		if p.Usage != nil {
			t := p.Usage.Neutral()
			ev.Tokens = &t
		}
	}
	return ev
}

// NoResponse is api_retry's no_response object.
type NoResponse struct {
	WaitedMS    int64 `json:"waited_ms"`
	RetryWaitMS int64 `json:"retry_wait_ms"`
}

// APIRetry is a system/api_retry message (docs fields).
type APIRetry struct {
	Attempt      int         `json:"attempt"`
	MaxRetries   int         `json:"max_retries"`
	RetryDelayMS int64       `json:"retry_delay_ms"`
	ErrorStatus  *int        `json:"error_status"`
	NoResponse   *NoResponse `json:"no_response"`
	Error        string      `json:"error"`
	UUID         string      `json:"uuid"`
	SessionID    string      `json:"session_id"`
}

// TaskUsage is a task notification's usage.
type TaskUsage struct {
	TotalTokens int64 `json:"total_tokens"`
	ToolUses    int64 `json:"tool_uses"`
	DurationMS  int64 `json:"duration_ms"`
}

// TaskNotification is a system/task_notification message (docs fields).
type TaskNotification struct {
	TaskID     string     `json:"task_id"`
	RunID      string     `json:"run_id"`
	ToolUseID  string     `json:"tool_use_id"`
	Status     string     `json:"status"`
	Reason     string     `json:"reason"`
	OutputFile string     `json:"output_file"`
	Summary    string     `json:"summary"`
	Usage      *TaskUsage `json:"usage"`
}

// ErrWrongStreamMessage means an AsX accessor was called on another type.
var ErrWrongStreamMessage = errors.New("claude: wrong stream message type")

// AsAPIRetry decodes a system/api_retry message. cost: Pure.
func (m StreamMessage) AsAPIRetry() (APIRetry, error) {
	var v APIRetry
	if m.Type != streamTypeSystem || m.Subtype != streamSubtypeRetry {
		return v, fmt.Errorf("%w: %s/%s", ErrWrongStreamMessage, m.Type, m.Subtype)
	}
	return v, json.Unmarshal(m.Raw, &v)
}

// AsTaskNotification decodes a system/task_notification message. cost: Pure.
func (m StreamMessage) AsTaskNotification() (TaskNotification, error) {
	var v TaskNotification
	if m.Type != streamTypeSystem || m.Subtype != streamSubtypeTaskNtf {
		return v, fmt.Errorf("%w: %s/%s", ErrWrongStreamMessage, m.Type, m.Subtype)
	}
	return v, json.Unmarshal(m.Raw, &v)
}

// DecodeJSONResult reads the single document of `--output-format json`;
// Raw until a run is recorded (OD-11). cost: Pure.
func DecodeJSONResult(r io.Reader) (agentcli.Raw, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxLineBytes))
	if err != nil {
		return nil, err
	}
	if !json.Valid(bytes.TrimSpace(b)) {
		return nil, fmt.Errorf("claude: json result is not valid JSON")
	}
	return agentcli.Raw(bytes.TrimSpace(b)), nil
}

// runDecoder adapts StreamDecoder to the neutral RunDecoder.
type runDecoder struct{ d *StreamDecoder }

var _ agentcli.RunDecoder = runDecoder{}

// Next is Pure.
func (r runDecoder) Next() (agentcli.RunEvent, error) {
	m, err := r.d.Next()
	if err != nil {
		return agentcli.RunEvent{}, err
	}
	return m.Neutral(), nil
}

// newRunDecoder is the neutral RunEvents constructor.
func newRunDecoder(r io.Reader) agentcli.RunDecoder { return runDecoder{d: NewStreamDecoder(r)} }
