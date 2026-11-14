package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/khanakia/agentcli"
)

// StreamType is the `type` of one `--output-format stream-json` line (open
// set: a value this version does not know is kept, IsKnown false, and the
// line stays reachable through StreamMessage.Raw).
//
// Source: the recorded 2.1.325 run in testdata/stream/2.1.325 (OD-11,
// closed 2026-11-14) for system, stream_event, assistant, rate_limit_event
// and result; user is named by the docs (`headless`) and seen in transcripts
// but did not occur in the recorded run (no tool call).
type StreamType string

// Stream line types.
const (
	StreamTypeSystem    StreamType = "system"
	StreamTypeAssistant StreamType = "assistant"
	StreamTypeUser      StreamType = "user"
	StreamTypeResult    StreamType = "result"
	StreamTypeEvent     StreamType = "stream_event"
	StreamTypeRateLimit StreamType = "rate_limit_event"
)

// IsKnown reports a typed stream line type. cost: Pure.
func (t StreamType) IsKnown() bool {
	switch t {
	case StreamTypeSystem, StreamTypeAssistant, StreamTypeUser, StreamTypeResult, StreamTypeEvent, StreamTypeRateLimit:
		return true
	}
	return false
}

// StreamSubtype is the `subtype` of a system or result line (open set).
type StreamSubtype string

// System and result subtypes. init, status, commands_changed and success
// are recorded (2.1.325); api_retry and task_notification come from the
// docs table (headless page); the four result error subtypes are named in
// the docs (agent-sdk) and appear verbatim in the 2.1.325 binary, but no
// error result was recorded, so their field set is assumed to be the
// success one.
const (
	SubtypeInit             StreamSubtype = "init"
	SubtypeStatus           StreamSubtype = "status"
	SubtypeCommandsChanged  StreamSubtype = "commands_changed"
	SubtypeAPIRetry         StreamSubtype = "api_retry"
	SubtypeTaskNotification StreamSubtype = "task_notification"

	SubtypeSuccess                      StreamSubtype = "success"
	SubtypeErrorMaxTurns                StreamSubtype = "error_max_turns"
	SubtypeErrorDuringExecution         StreamSubtype = "error_during_execution"
	SubtypeErrorMaxBudgetUSD            StreamSubtype = "error_max_budget_usd"
	SubtypeErrorMaxStructuredOutRetries StreamSubtype = "error_max_structured_output_retries"
)

// IsKnown reports a typed subtype. cost: Pure.
func (s StreamSubtype) IsKnown() bool {
	switch s {
	case SubtypeInit, SubtypeStatus, SubtypeCommandsChanged, SubtypeAPIRetry, SubtypeTaskNotification,
		SubtypeSuccess, SubtypeErrorMaxTurns, SubtypeErrorDuringExecution, SubtypeErrorMaxBudgetUSD, SubtypeErrorMaxStructuredOutRetries:
		return true
	}
	return false
}

// IsResultError reports one of the result error subtypes. cost: Pure.
func (s StreamSubtype) IsResultError() bool {
	switch s {
	case SubtypeErrorMaxTurns, SubtypeErrorDuringExecution, SubtypeErrorMaxBudgetUSD, SubtypeErrorMaxStructuredOutRetries:
		return true
	}
	return false
}

// APIEventType is the `event.type` of a stream_event line: the raw Messages
// API stream event Claude Code forwards with --include-partial-messages.
// All six are recorded (2.1.325). Open set.
type APIEventType string

// API stream event types.
const (
	APIEventMessageStart      APIEventType = "message_start"
	APIEventContentBlockStart APIEventType = "content_block_start"
	APIEventContentBlockDelta APIEventType = "content_block_delta"
	APIEventContentBlockStop  APIEventType = "content_block_stop"
	APIEventMessageDelta      APIEventType = "message_delta"
	APIEventMessageStop       APIEventType = "message_stop"
)

// IsKnown reports a recorded API event type. cost: Pure.
func (t APIEventType) IsKnown() bool {
	switch t {
	case APIEventMessageStart, APIEventContentBlockStart, APIEventContentBlockDelta, APIEventContentBlockStop, APIEventMessageDelta, APIEventMessageStop:
		return true
	}
	return false
}

// DeltaType is a content_block_delta's `delta.type` (open set). Only
// text_delta is recorded; tool input and thinking deltas keep their type
// here and their payload in StreamDelta.Unknown.
type DeltaType string

// DeltaText is a text block's partial text.
const DeltaText DeltaType = "text_delta"

// streamFile names the stream in decode warnings and errors.
const streamFile = "stream-json"

// StreamMessage is one line of `--output-format stream-json`: the envelope
// every line shares. The typed body is reached through the AsX accessors;
// Raw always holds the full line, so an unknown type is never an error.
type StreamMessage struct {
	Type            StreamType    `json:"type"`
	Subtype         StreamSubtype `json:"subtype,omitempty"`
	SessionID       string        `json:"session_id,omitempty"`
	UUID            string        `json:"uuid,omitempty"`
	ParentToolUseID *string       `json:"parent_tool_use_id,omitempty"`
	Raw             agentcli.Raw  `json:"-"`
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

// Neutral maps onto the shared run event: system/init → Started, assistant
// and stream_event → Message (stream_event carries the partial text of a
// text_delta), user with tool results → Tool, result → Done (Error when
// is_error or an error subtype), everything else Other. Raw is always the
// line; a body that fails to decode leaves Text/Tokens empty. cost: Pure.
func (m StreamMessage) Neutral() agentcli.RunEvent {
	ev := agentcli.RunEvent{Type: agentcli.RunEventOther, ConversationID: m.SessionID, Raw: m.Raw}
	switch m.Type {
	case StreamTypeSystem:
		if m.Subtype == SubtypeInit {
			ev.Type = agentcli.RunEventStarted
		}
	case StreamTypeAssistant:
		ev.Type = agentcli.RunEventMessage
		if a, err := m.AsAssistant(); err == nil {
			ev.Text = a.Message.Content.Text()
		}
	case StreamTypeEvent:
		ev.Type = agentcli.RunEventMessage
		if e, err := m.AsEvent(); err == nil && e.Event.Delta != nil && e.Event.Delta.Type == DeltaText {
			ev.Text = e.Event.Delta.Text
		}
	case StreamTypeUser:
		if u, err := m.AsUser(); err == nil {
			for _, b := range u.Message.Content {
				if b.Type == BlockToolResult {
					ev.Type = agentcli.RunEventTool
				}
			}
		}
	case StreamTypeResult:
		ev.Type = agentcli.RunEventDone
		if m.Subtype.IsResultError() {
			ev.Type = agentcli.RunEventError
		}
		if r, err := m.AsResult(); err == nil {
			ev.Text = r.Result
			if r.IsError {
				ev.Type = agentcli.RunEventError
			}
			t := r.Usage.Neutral()
			ev.Tokens = &t
		}
	}
	return ev
}

// ErrWrongStreamMessage means an AsX accessor was called on another type.
var ErrWrongStreamMessage = errors.New("claude: wrong stream message type")

// streamBody lists the wire structs the generic decoder fills. Why a union
// and not `any`: the archtest bans `any`, and the list doubles as the index
// of every typed stream body.
type streamBody interface {
	wireInit | wireStatus | wireCommandsChanged | wireStreamEvent | wireStreamAssistant | wireStreamUser | wireRateLimitEvent | wireResult | APIRetry | TaskNotification
}

// decodeBody checks the envelope against t (and sub when non-empty) and
// decodes the line into W, returning the top-level keys W does not model.
func decodeBody[W streamBody](m StreamMessage, t StreamType, sub StreamSubtype) (W, agentcli.Unknown, error) {
	var w W
	if m.Type != t || (sub != "" && m.Subtype != sub) {
		return w, nil, fmt.Errorf("%w: %s/%s", ErrWrongStreamMessage, m.Type, m.Subtype)
	}
	unknown, _, err := decodeLenient(streamFile, m.Raw, reflect.TypeFor[W](), func(d []byte) error { w = *new(W); return json.Unmarshal(d, &w) })
	return w, unknown, err
}

// dropEnvelope removes the keys StreamMessage itself models, so a body's
// Unknown never repeats them. The key list comes from StreamMessage's tags.
func dropEnvelope(u agentcli.Unknown) agentcli.Unknown {
	for k := range jsonKeys(reflect.TypeFor[StreamMessage]()) {
		delete(u, k)
	}
	if len(u) == 0 {
		return nil
	}
	return u
}

// msDurationPtr converts an optional millisecond count; nil stays nil.
func msDurationPtr(ms *int64) *time.Duration {
	if ms == nil {
		return nil
	}
	d := time.Duration(*ms) * time.Millisecond
	return &d
}

// ---- system/init ----

// StreamMCPServer is one init `mcp_servers` entry.
type StreamMCPServer struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "connected" recorded; docs also list failed/pending states
	Source string `json:"source"` // "claudeai" recorded (claude.ai connector); open set
}

// StreamPlugin is one init `plugins` entry.
type StreamPlugin struct {
	Name   string `json:"name"`
	Path   string `json:"path"`   // "builtin" for bundled plugins, else a directory
	Source string `json:"source"` // "<name>@<marketplace>"
}

// StreamInit is a system/init line: the session's effective setup, sent
// once before the first request. Recorded at 2.1.325.
type StreamInit struct {
	SessionID, UUID   string
	ClaudeCodeVersion string
	Model             string
	PermissionMode    PermissionMode
	// CWD and OriginalCWD differ when the session changed directory; both
	// absolute.
	CWD, OriginalCWD      string
	AdditionalDirectories []string
	Tools                 []string // tool names, MCP tools as mcp__<server>__<tool>
	MCPServers            []StreamMCPServer
	SlashCommands         []string
	// TerminalSlashCommands are commands only the interactive terminal runs
	// (doctor, color, …); a headless caller cannot send them.
	TerminalSlashCommands []string
	Agents                []string // subagent names
	Skills                []string
	Plugins               []StreamPlugin
	// Capabilities are protocol capability tokens (interrupt_receipt_v1, …)
	// an SDK host can rely on; open set.
	Capabilities []string
	APIKeySource string // "none" under an OAuth login; open set
	OutputStyle  string
	// MemoryPaths maps a memory kind ("auto" recorded) to its directory.
	MemoryPaths             map[string]string
	MessagingSocketPath     string
	FastModeState           string // "off" recorded
	FastModeDisabledReason  string // "sdk_opt_in_required" recorded
	AnalyticsDisabled       bool
	ProductFeedbackDisabled bool
	PerTurnEffortActive     bool
	UltrathinkAvailable     bool
	UltrathinkTypedRaises   bool
	ViewMode                string
	Unknown                 agentcli.Unknown
}

type wireInit struct {
	CWD                     string            `json:"cwd"`
	SessionID               string            `json:"session_id"`
	Tools                   []string          `json:"tools"`
	MCPServers              []StreamMCPServer `json:"mcp_servers"`
	Model                   string            `json:"model"`
	PermissionMode          PermissionMode    `json:"permissionMode"`
	SlashCommands           []string          `json:"slash_commands"`
	TerminalSlashCommands   []string          `json:"terminal_slash_commands"`
	APIKeySource            string            `json:"apiKeySource"`
	ClaudeCodeVersion       string            `json:"claude_code_version"`
	OutputStyle             string            `json:"output_style"`
	Agents                  []string          `json:"agents"`
	Skills                  []string          `json:"skills"`
	Plugins                 []StreamPlugin    `json:"plugins"`
	Capabilities            []string          `json:"capabilities"`
	AnalyticsDisabled       bool              `json:"analytics_disabled"`
	ProductFeedbackDisabled bool              `json:"product_feedback_disabled"`
	UUID                    string            `json:"uuid"`
	MemoryPaths             map[string]string `json:"memory_paths"`
	OriginalCWD             string            `json:"original_cwd"`
	AdditionalDirectories   []string          `json:"additional_directories"`
	MessagingSocketPath     string            `json:"messaging_socket_path"`
	FastModeState           string            `json:"fast_mode_state"`
	FastModeDisabledReason  string            `json:"fast_mode_disabled_reason"`
	PerTurnEffortActive     bool              `json:"per_turn_effort_active"`
	UltrathinkAvailable     bool              `json:"ultrathink_available"`
	UltrathinkTypedRaises   bool              `json:"ultrathink_typed_raises"`
	ViewMode                string            `json:"view_mode"`
	Type                    StreamType        `json:"type"`
	Subtype                 StreamSubtype     `json:"subtype"`
}

// AsInit decodes a system/init line. cost: Pure.
func (m StreamMessage) AsInit() (StreamInit, error) {
	w, unknown, err := decodeBody[wireInit](m, StreamTypeSystem, SubtypeInit)
	if err != nil {
		return StreamInit{}, err
	}
	return StreamInit{
		SessionID: w.SessionID, UUID: w.UUID, ClaudeCodeVersion: w.ClaudeCodeVersion, Model: w.Model, PermissionMode: w.PermissionMode,
		CWD: w.CWD, OriginalCWD: w.OriginalCWD, AdditionalDirectories: w.AdditionalDirectories,
		Tools: w.Tools, MCPServers: w.MCPServers, SlashCommands: w.SlashCommands, TerminalSlashCommands: w.TerminalSlashCommands,
		Agents: w.Agents, Skills: w.Skills, Plugins: w.Plugins, Capabilities: w.Capabilities,
		APIKeySource: w.APIKeySource, OutputStyle: w.OutputStyle, MemoryPaths: w.MemoryPaths, MessagingSocketPath: w.MessagingSocketPath,
		FastModeState: w.FastModeState, FastModeDisabledReason: w.FastModeDisabledReason,
		AnalyticsDisabled: w.AnalyticsDisabled, ProductFeedbackDisabled: w.ProductFeedbackDisabled,
		PerTurnEffortActive: w.PerTurnEffortActive, UltrathinkAvailable: w.UltrathinkAvailable, UltrathinkTypedRaises: w.UltrathinkTypedRaises,
		ViewMode: w.ViewMode, Unknown: dropEnvelope(unknown),
	}, nil
}

// ---- system/status ----

// StreamStatus is a system/status line: "requesting" recorded (sent before
// each API request); the docs add "compacting" and a null status that
// clears it, plus permissionMode on a mode change.
type StreamStatus struct {
	// Status is "" for a JSON null (status cleared).
	Status         string
	PermissionMode PermissionMode // "" when the line does not carry it
	Unknown        agentcli.Unknown
}

type wireStatus struct {
	Status         *string        `json:"status"`
	PermissionMode PermissionMode `json:"permissionMode"`
	Type           StreamType     `json:"type"`
	Subtype        StreamSubtype  `json:"subtype"`
	SessionID      string         `json:"session_id"`
	UUID           string         `json:"uuid"`
}

// AsStatus decodes a system/status line. cost: Pure.
func (m StreamMessage) AsStatus() (StreamStatus, error) {
	w, unknown, err := decodeBody[wireStatus](m, StreamTypeSystem, SubtypeStatus)
	if err != nil {
		return StreamStatus{}, err
	}
	s := StreamStatus{PermissionMode: w.PermissionMode, Unknown: dropEnvelope(unknown)}
	if w.Status != nil {
		s.Status = *w.Status
	}
	return s, nil
}

// ---- system/commands_changed ----

// StreamCommand is one slash command a session offers.
type StreamCommand struct {
	Name         string `json:"name"`
	Description  string `json:"description"` // skills end with " (user)" / " (project)" / " (plugin)"
	ArgumentHint string `json:"argumentHint"`
}

type wireCommandsChanged struct {
	Commands  []StreamCommand `json:"commands"`
	Type      StreamType      `json:"type"`
	Subtype   StreamSubtype   `json:"subtype"`
	SessionID string          `json:"session_id"`
	UUID      string          `json:"uuid"`
}

// StreamCommandsChanged is a system/commands_changed line: the full list of
// slash commands after skills and plugins loaded (recorded 2.1.325).
type StreamCommandsChanged struct {
	Commands []StreamCommand
	Unknown  agentcli.Unknown
}

// AsCommandsChanged decodes a system/commands_changed line. cost: Pure.
func (m StreamMessage) AsCommandsChanged() (StreamCommandsChanged, error) {
	w, unknown, err := decodeBody[wireCommandsChanged](m, StreamTypeSystem, SubtypeCommandsChanged)
	if err != nil {
		return StreamCommandsChanged{}, err
	}
	return StreamCommandsChanged{Commands: w.Commands, Unknown: dropEnvelope(unknown)}, nil
}

// ---- stream_event ----

// StreamAPIMessage is the API message in message_start and in an assistant
// line. Recorded keys only; container, stop_details, input_transformations,
// diagnostics and context_management (all null or empty in the sample) are
// kept in Unknown.
type StreamAPIMessage struct {
	ID           string
	Model        string
	Role         string
	Content      Content
	StopReason   string // "" while streaming (null)
	StopSequence string
	Usage        *MessageUsage
	Unknown      agentcli.Unknown
}

type wireAPIMessage struct {
	ID           string        `json:"id"`
	Type         string        `json:"type"`
	Model        string        `json:"model"`
	Role         string        `json:"role"`
	Content      Content       `json:"content"`
	StopReason   *string       `json:"stop_reason"`
	StopSequence *string       `json:"stop_sequence"`
	Usage        *MessageUsage `json:"usage"`
}

// UnmarshalJSON decodes the message, keeping unmodelled keys. cost: Pure.
func (a *StreamAPIMessage) UnmarshalJSON(data []byte) error {
	var w wireAPIMessage
	unknown, _, err := decodeLenient(streamFile, data, reflect.TypeFor[wireAPIMessage](), func(d []byte) error { w = wireAPIMessage{}; return json.Unmarshal(d, &w) })
	if err != nil {
		return err
	}
	*a = StreamAPIMessage{ID: w.ID, Model: w.Model, Role: w.Role, Content: w.Content, Usage: w.Usage, Unknown: unknown}
	if w.StopReason != nil {
		a.StopReason = *w.StopReason
	}
	if w.StopSequence != nil {
		a.StopSequence = *w.StopSequence
	}
	return nil
}

// StreamDelta is the `delta` of content_block_delta (Type, Text) or of
// message_delta (StopReason, StopSequence). Other delta kinds keep their
// payload in Unknown.
type StreamDelta struct {
	Type         DeltaType
	Text         string
	StopReason   string
	StopSequence string
	Unknown      agentcli.Unknown
}

type wireDelta struct {
	Type         DeltaType `json:"type"`
	Text         string    `json:"text"`
	StopReason   *string   `json:"stop_reason"`
	StopSequence *string   `json:"stop_sequence"`
}

// UnmarshalJSON decodes the delta, keeping unmodelled keys. cost: Pure.
func (d *StreamDelta) UnmarshalJSON(data []byte) error {
	var w wireDelta
	unknown, _, err := decodeLenient(streamFile, data, reflect.TypeFor[wireDelta](), func(b []byte) error { w = wireDelta{}; return json.Unmarshal(b, &w) })
	if err != nil {
		return err
	}
	*d = StreamDelta{Type: w.Type, Text: w.Text, Unknown: unknown}
	if w.StopReason != nil {
		d.StopReason = *w.StopReason
	}
	if w.StopSequence != nil {
		d.StopSequence = *w.StopSequence
	}
	return nil
}

// APIStreamEvent is the forwarded Messages API event. Which fields are set
// depends on Type: message_start → Message; content_block_start → Index,
// ContentBlock; content_block_delta → Index, Delta; content_block_stop →
// Index; message_delta → Delta, Usage; message_stop → none.
type APIStreamEvent struct {
	Type         APIEventType
	Index        *int
	Message      *StreamAPIMessage
	ContentBlock *ContentBlock
	Delta        *StreamDelta
	Usage        *MessageUsage
	Unknown      agentcli.Unknown
}

type wireAPIStreamEvent struct {
	Type         APIEventType      `json:"type"`
	Index        *int              `json:"index"`
	Message      *StreamAPIMessage `json:"message"`
	ContentBlock *ContentBlock     `json:"content_block"`
	Delta        *StreamDelta      `json:"delta"`
	Usage        *MessageUsage     `json:"usage"`
}

// UnmarshalJSON decodes the event, keeping unmodelled keys. cost: Pure.
func (e *APIStreamEvent) UnmarshalJSON(data []byte) error {
	var w wireAPIStreamEvent
	unknown, _, err := decodeLenient(streamFile, data, reflect.TypeFor[wireAPIStreamEvent](), func(b []byte) error { w = wireAPIStreamEvent{}; return json.Unmarshal(b, &w) })
	if err != nil {
		return err
	}
	*e = APIStreamEvent{Type: w.Type, Index: w.Index, Message: w.Message, ContentBlock: w.ContentBlock, Delta: w.Delta, Usage: w.Usage, Unknown: unknown}
	return nil
}

// StreamEventMessage is a stream_event line (--include-partial-messages).
type StreamEventMessage struct {
	Event        APIStreamEvent
	APIMessageID string
	// TTFT (time to first token) is set on message_start only.
	TTFT *time.Duration
	// ThinkingDisplay is set on message_start only ("updates" recorded).
	ThinkingDisplay string
	Unknown         agentcli.Unknown
}

type wireStreamEvent struct {
	Event           APIStreamEvent `json:"event"`
	APIMessageID    string         `json:"api_message_id"`
	TTFTMS          *int64         `json:"ttft_ms"`
	ThinkingDisplay string         `json:"thinking_display"`
	Type            StreamType     `json:"type"`
	SessionID       string         `json:"session_id"`
	UUID            string         `json:"uuid"`
	ParentToolUseID *string        `json:"parent_tool_use_id"`
}

// AsEvent decodes a stream_event line. cost: Pure.
func (m StreamMessage) AsEvent() (StreamEventMessage, error) {
	w, unknown, err := decodeBody[wireStreamEvent](m, StreamTypeEvent, "")
	if err != nil {
		return StreamEventMessage{}, err
	}
	return StreamEventMessage{Event: w.Event, APIMessageID: w.APIMessageID, TTFT: msDurationPtr(w.TTFTMS), ThinkingDisplay: w.ThinkingDisplay, Unknown: dropEnvelope(unknown)}, nil
}

// ---- assistant / user ----

// StreamAssistantMessage is an assistant line: one complete API message
// (sent after its text blocks streamed, before message_delta).
type StreamAssistantMessage struct {
	Message   StreamAPIMessage
	RequestID string
	Timestamp time.Time
	Unknown   agentcli.Unknown
}

type wireStreamAssistant struct {
	Message         StreamAPIMessage `json:"message"`
	RequestID       string           `json:"request_id"`
	Timestamp       time.Time        `json:"timestamp"`
	Type            StreamType       `json:"type"`
	SessionID       string           `json:"session_id"`
	UUID            string           `json:"uuid"`
	ParentToolUseID *string          `json:"parent_tool_use_id"`
}

// AsAssistant decodes an assistant line. cost: Pure.
func (m StreamMessage) AsAssistant() (StreamAssistantMessage, error) {
	w, unknown, err := decodeBody[wireStreamAssistant](m, StreamTypeAssistant, "")
	if err != nil {
		return StreamAssistantMessage{}, err
	}
	return StreamAssistantMessage{Message: w.Message, RequestID: w.RequestID, Timestamp: w.Timestamp, Unknown: dropEnvelope(unknown)}, nil
}

// StreamUserMessage is a user line (tool results fed back to the model).
// Not in the recorded run: the envelope is from the docs, the message
// content is the transcript user content shape (CLAUDE_API §18).
type StreamUserMessage struct {
	Message struct {
		Role    string  `json:"role"`
		Content Content `json:"content"`
	}
	Unknown agentcli.Unknown
}

type wireStreamUser struct {
	Message struct {
		Role    string  `json:"role"`
		Content Content `json:"content"`
	} `json:"message"`
	Type            StreamType `json:"type"`
	SessionID       string     `json:"session_id"`
	UUID            string     `json:"uuid"`
	ParentToolUseID *string    `json:"parent_tool_use_id"`
}

// AsUser decodes a user line. cost: Pure.
func (m StreamMessage) AsUser() (StreamUserMessage, error) {
	w, unknown, err := decodeBody[wireStreamUser](m, StreamTypeUser, "")
	if err != nil {
		return StreamUserMessage{}, err
	}
	u := StreamUserMessage{Unknown: dropEnvelope(unknown)}
	u.Message.Role, u.Message.Content = w.Message.Role, w.Message.Content
	return u, nil
}

// ---- rate_limit_event ----

// RateLimitWindow is one entry of rate_limit_info.unifiedWindows.
type RateLimitWindow struct {
	// Utilization is the used fraction, 0..1 (0.52 recorded), not a percent.
	Utilization float64
	ResetsAt    time.Time
	Status      string // "allowed" recorded
}

type wireRateLimitWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    int64   `json:"resetsAt"`
	Status      string  `json:"status"`
}

// RateLimitInfo is a rate_limit_event's plan-limit state after a request.
// It is the same data the usage endpoint returns, delivered with no extra
// request; ResetsAt values are Unix seconds on the wire.
type RateLimitInfo struct {
	Status                string // "allowed" recorded
	ResetsAt              time.Time
	RateLimitType         string // the binding window: "seven_day" recorded
	OverageStatus         string // "rejected" recorded
	OverageDisabledReason string // "org_level_disabled" recorded
	IsUsingOverage        bool
	// UnifiedWindows is keyed by window name ("seven_day" recorded).
	UnifiedWindows map[string]RateLimitWindow
	Unknown        agentcli.Unknown
}

type wireRateLimitInfo struct {
	Status                string                         `json:"status"`
	ResetsAt              int64                          `json:"resetsAt"`
	RateLimitType         string                         `json:"rateLimitType"`
	OverageStatus         string                         `json:"overageStatus"`
	OverageDisabledReason string                         `json:"overageDisabledReason"`
	IsUsingOverage        bool                           `json:"isUsingOverage"`
	UnifiedWindows        map[string]wireRateLimitWindow `json:"unifiedWindows"`
}

type wireRateLimitEvent struct {
	RateLimitInfo json.RawMessage `json:"rate_limit_info"`
	Type          StreamType      `json:"type"`
	SessionID     string          `json:"session_id"`
	UUID          string          `json:"uuid"`
}

// unixSeconds converts a Unix-seconds wire value; 0 stays the zero time.
func unixSeconds(s int64) time.Time {
	if s <= 0 {
		return time.Time{}
	}
	return time.Unix(s, 0).UTC()
}

// RateLimitEvent is a rate_limit_event line (recorded 2.1.325, sent after
// each request).
type RateLimitEvent struct {
	Info    RateLimitInfo
	Unknown agentcli.Unknown
}

// AsRateLimit decodes a rate_limit_event line. cost: Pure.
func (m StreamMessage) AsRateLimit() (RateLimitEvent, error) {
	w, top, err := decodeBody[wireRateLimitEvent](m, StreamTypeRateLimit, "")
	if err != nil {
		return RateLimitEvent{}, err
	}
	var wi wireRateLimitInfo
	unknown, _, err := decodeLenient(streamFile, w.RateLimitInfo, reflect.TypeFor[wireRateLimitInfo](), func(d []byte) error { wi = wireRateLimitInfo{}; return json.Unmarshal(d, &wi) })
	if err != nil {
		return RateLimitEvent{}, err
	}
	info := RateLimitInfo{
		Status: wi.Status, ResetsAt: unixSeconds(wi.ResetsAt), RateLimitType: wi.RateLimitType,
		OverageStatus: wi.OverageStatus, OverageDisabledReason: wi.OverageDisabledReason, IsUsingOverage: wi.IsUsingOverage, Unknown: unknown,
	}
	if len(wi.UnifiedWindows) > 0 {
		info.UnifiedWindows = make(map[string]RateLimitWindow, len(wi.UnifiedWindows))
		for k, v := range wi.UnifiedWindows {
			info.UnifiedWindows[k] = RateLimitWindow{Utilization: v.Utilization, ResetsAt: unixSeconds(v.ResetsAt), Status: v.Status}
		}
	}
	return RateLimitEvent{Info: info, Unknown: dropEnvelope(top)}, nil
}

// ---- result ----

// ResultModelUsage is one model's tally in result.modelUsage (camelCase keys on
// the wire, unlike usage).
type ResultModelUsage struct {
	InputTokens                         int64   `json:"inputTokens"`
	OutputTokens                        int64   `json:"outputTokens"`
	CacheReadInputTokens                int64   `json:"cacheReadInputTokens"`
	CacheCreationInputTokens            int64   `json:"cacheCreationInputTokens"`
	CacheCreationEphemeral5mInputTokens int64   `json:"cacheCreationEphemeral5mInputTokens"`
	CacheCreationEphemeral1hInputTokens int64   `json:"cacheCreationEphemeral1hInputTokens"`
	ThinkingTokens                      int64   `json:"thinkingTokens"`
	WebSearchRequests                   int64   `json:"webSearchRequests"`
	CostUSD                             float64 `json:"costUSD"`
	ContextWindow                       int64   `json:"contextWindow"`
	MaxOutputTokens                     int64   `json:"maxOutputTokens"`
	CanonicalModel                      string  `json:"canonicalModel"`
	Provider                            string  `json:"provider"`  // "firstParty" recorded
	CostBasis                           string  `json:"costBasis"` // "list" recorded
}

// SubagentRequested counts requested subagents by launch mode.
type SubagentRequested struct {
	Background int64 `json:"background"`
	Foreground int64 `json:"foreground"`
	Unset      int64 `json:"unset"`
}

// SubagentKilled counts killed subagents by who killed them.
type SubagentKilled struct {
	Parent int64 `json:"parent"`
	User   int64 `json:"user"`
	System int64 `json:"system"`
}

// SubagentRefused counts refused subagent launches by reason.
type SubagentRefused struct {
	DepthLimit       int64 `json:"depth_limit"`
	ConcurrencyLimit int64 `json:"concurrency_limit"`
	Budget           int64 `json:"budget"`
}

// SubagentStats is result.subagent_stats. ByType was an empty object in the
// recorded run, so its value shape is unknown and kept raw per key.
type SubagentStats struct {
	Spawned             int64                   `json:"spawned"`
	Requested           SubagentRequested       `json:"requested"`
	StartedInBackground int64                   `json:"started_in_background"`
	MaxDepth            int64                   `json:"max_depth"`
	SpawnedBySubagents  int64                   `json:"spawned_by_subagents"`
	Completed           int64                   `json:"completed"`
	Failed              int64                   `json:"failed"`
	Killed              SubagentKilled          `json:"killed"`
	Refused             SubagentRefused         `json:"refused"`
	ByType              map[string]agentcli.Raw `json:"by_type"`
}

// StreamResult is the result line of stream-json and the single document of
// --output-format json (same keys, except ModelUsageSinceLastResult which
// only stream-json sends). Recorded at 2.1.325 (subtype success).
type StreamResult struct {
	Subtype   StreamSubtype
	IsError   bool
	Result    string
	SessionID string
	UUID      string
	NumTurns  int
	// StopReason is the API stop reason ("end_turn"); TerminalReason is
	// Claude Code's own ("completed").
	StopReason     string
	TerminalReason string
	// APIErrorStatus is the HTTP status of a failed API call; nil on success.
	APIErrorStatus *int
	TotalCostUSD   float64
	Usage          MessageUsage
	// ModelUsage is keyed by model id.
	ModelUsage                map[string]ResultModelUsage
	ModelUsageSinceLastResult map[string]ResultModelUsage
	// PermissionDenials was empty in the recorded run; elements stay raw.
	PermissionDenials []agentcli.Raw
	SubagentStats     *SubagentStats
	SafetyStops       int
	QueuedTurnCount   int
	ResultIndex       int
	// Durations: Duration is wall time, APIDuration time inside API calls,
	// TTFT/TTFTStream time to first token, TimeToRequest startup to first
	// request, FirstContentFrame time to first rendered content.
	Duration, APIDuration, TTFT, TTFTStream, TimeToRequest, FirstContentFrame time.Duration
	FastModeState, FastModeDisabledReason                                     string
	// CredentialExpiresAt is when the OAuth access token expires; nil when
	// the line does not carry it (API key login).
	CredentialExpiresAt *time.Time
	Unknown             agentcli.Unknown
	Raw                 agentcli.Raw
}

type wireResult struct {
	Type                      StreamType                  `json:"type"`
	Subtype                   StreamSubtype               `json:"subtype"`
	IsError                   bool                        `json:"is_error"`
	Result                    string                      `json:"result"`
	SessionID                 string                      `json:"session_id"`
	UUID                      string                      `json:"uuid"`
	NumTurns                  int                         `json:"num_turns"`
	StopReason                *string                     `json:"stop_reason"`
	TerminalReason            string                      `json:"terminal_reason"`
	APIErrorStatus            *int                        `json:"api_error_status"`
	TotalCostUSD              float64                     `json:"total_cost_usd"`
	Usage                     MessageUsage                `json:"usage"`
	ModelUsage                map[string]ResultModelUsage `json:"modelUsage"`
	ModelUsageSinceLastResult map[string]ResultModelUsage `json:"model_usage_since_last_result"`
	PermissionDenials         []agentcli.Raw              `json:"permission_denials"`
	SubagentStats             *SubagentStats              `json:"subagent_stats"`
	SafetyStops               int                         `json:"safety_stops"`
	QueuedTurnCount           int                         `json:"queued_turn_count"`
	ResultIndex               int                         `json:"result_index"`
	DurationMS                int64                       `json:"duration_ms"`
	DurationAPIMS             int64                       `json:"duration_api_ms"`
	TTFTMS                    int64                       `json:"ttft_ms"`
	TTFTStreamMS              int64                       `json:"ttft_stream_ms"`
	TimeToRequestMS           int64                       `json:"time_to_request_ms"`
	FirstContentFrameMS       int64                       `json:"first_content_frame_ms"`
	FastModeState             string                      `json:"fast_mode_state"`
	FastModeDisabledReason    string                      `json:"fast_mode_disabled_reason"`
	CredentialExpiresAt       *time.Time                  `json:"credential_expires_at"`
	ParentToolUseID           *string                     `json:"parent_tool_use_id"`
}

// resultFrom converts the wire result.
func resultFrom(w wireResult, unknown agentcli.Unknown, raw agentcli.Raw) StreamResult {
	r := StreamResult{
		Subtype: w.Subtype, IsError: w.IsError, Result: w.Result, SessionID: w.SessionID, UUID: w.UUID, NumTurns: w.NumTurns,
		TerminalReason: w.TerminalReason, APIErrorStatus: w.APIErrorStatus, TotalCostUSD: w.TotalCostUSD, Usage: w.Usage,
		ModelUsage: w.ModelUsage, ModelUsageSinceLastResult: w.ModelUsageSinceLastResult, PermissionDenials: w.PermissionDenials,
		SubagentStats: w.SubagentStats, SafetyStops: w.SafetyStops, QueuedTurnCount: w.QueuedTurnCount, ResultIndex: w.ResultIndex,
		Duration: time.Duration(w.DurationMS) * time.Millisecond, APIDuration: time.Duration(w.DurationAPIMS) * time.Millisecond, TTFT: time.Duration(w.TTFTMS) * time.Millisecond, TTFTStream: time.Duration(w.TTFTStreamMS) * time.Millisecond,
		TimeToRequest: time.Duration(w.TimeToRequestMS) * time.Millisecond, FirstContentFrame: time.Duration(w.FirstContentFrameMS) * time.Millisecond,
		FastModeState: w.FastModeState, FastModeDisabledReason: w.FastModeDisabledReason, CredentialExpiresAt: w.CredentialExpiresAt,
		Unknown: dropEnvelope(unknown), Raw: raw,
	}
	if w.StopReason != nil {
		r.StopReason = *w.StopReason
	}
	return r
}

// AsResult decodes a result line (any subtype). cost: Pure.
func (m StreamMessage) AsResult() (StreamResult, error) {
	w, unknown, err := decodeBody[wireResult](m, StreamTypeResult, "")
	if err != nil {
		return StreamResult{}, err
	}
	return resultFrom(w, unknown, m.Raw), nil
}

// DecodeJSONResult reads the single document of `--output-format json`
// (the same object as the stream's result line). A document whose type is
// not "result" is ErrWrongStreamMessage. cost: Pure.
func DecodeJSONResult(r io.Reader) (StreamResult, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxLineBytes))
	if err != nil {
		return StreamResult{}, err
	}
	b = bytes.TrimSpace(b)
	var m StreamMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return StreamResult{}, fmt.Errorf("claude: json result: %w", err)
	}
	m.Raw = agentcli.Raw(b)
	return m.AsResult()
}

// ---- docs-only system lines ----

// NoResponse is api_retry's no_response object.
type NoResponse struct {
	WaitedMS    int64 `json:"waited_ms"`
	RetryWaitMS int64 `json:"retry_wait_ms"`
}

// APIRetry is a system/api_retry message (docs fields; not recorded).
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

// TaskNotification is a system/task_notification message (docs fields; not
// recorded).
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

// AsAPIRetry decodes a system/api_retry message. cost: Pure.
func (m StreamMessage) AsAPIRetry() (APIRetry, error) {
	v, _, err := decodeBody[APIRetry](m, StreamTypeSystem, SubtypeAPIRetry)
	return v, err
}

// AsTaskNotification decodes a system/task_notification message. cost: Pure.
func (m StreamMessage) AsTaskNotification() (TaskNotification, error) {
	v, _, err := decodeBody[TaskNotification](m, StreamTypeSystem, SubtypeTaskNotification)
	return v, err
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
