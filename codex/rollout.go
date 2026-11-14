package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// RecordKind is a rollout line's top-level `type`. Open set: a new kind is
// kept with its Raw line, never an error (SPEC §5.8).
type RecordKind string

// RecordKind values seen in rollouts of cli 0.117 … 0.162.
const (
	KindSessionMeta      RecordKind = "session_meta"
	KindResponseItem     RecordKind = "response_item"
	KindEventMsg         RecordKind = "event_msg"
	KindTurnContext      RecordKind = "turn_context"
	KindCompacted        RecordKind = "compacted"
	KindWorldState       RecordKind = "world_state"
	KindTokenUsageRecord RecordKind = "token_usage_record"
)

// IsKnown reports a modelled record kind. Pure.
func (k RecordKind) IsKnown() bool {
	switch k {
	case KindSessionMeta, KindResponseItem, KindEventMsg, KindTurnContext, KindCompacted, KindWorldState, KindTokenUsageRecord:
		return true
	}
	return false
}

// EventType is event_msg payload.type.
type EventType string

// EventType values seen (older user_message/agent_message and 0.15x+
// item_completed are both decoded).
const (
	EventItemCompleted         EventType = "item_completed"
	EventTokenCount            EventType = "token_count"
	EventTaskStarted           EventType = "task_started"
	EventTaskComplete          EventType = "task_complete"
	EventTurnAborted           EventType = "turn_aborted"
	EventThreadSettingsApplied EventType = "thread_settings_applied"
	EventUserMessage           EventType = "user_message"
	EventAgentMessage          EventType = "agent_message"
)

// IsKnown reports a modelled event type. Pure.
func (t EventType) IsKnown() bool {
	switch t {
	case EventItemCompleted, EventTokenCount, EventTaskStarted, EventTaskComplete, EventTurnAborted,
		EventThreadSettingsApplied, EventUserMessage, EventAgentMessage:
		return true
	}
	return false
}

// ResponseType is response_item payload.type.
type ResponseType string

// ResponseType values seen.
const (
	ResponseMessage              ResponseType = "message"
	ResponseReasoning            ResponseType = "reasoning"
	ResponseFunctionCall         ResponseType = "function_call"
	ResponseFunctionCallOutput   ResponseType = "function_call_output"
	ResponseCustomToolCall       ResponseType = "custom_tool_call"
	ResponseCustomToolCallOutput ResponseType = "custom_tool_call_output"
	ResponseWebSearchCall        ResponseType = "web_search_call"
)

// IsKnown reports a modelled response type. Pure.
func (t ResponseType) IsKnown() bool {
	switch t {
	case ResponseMessage, ResponseReasoning, ResponseFunctionCall, ResponseFunctionCallOutput,
		ResponseCustomToolCall, ResponseCustomToolCallOutput, ResponseWebSearchCall:
		return true
	}
	return false
}

// ItemType is the `type` of an item_completed item (TurnItem upstream).
type ItemType string

// ItemType values.
const (
	ItemUserMessage       ItemType = "UserMessage"
	ItemAgentMessage      ItemType = "AgentMessage"
	ItemReasoning         ItemType = "Reasoning"
	ItemCommandExecution  ItemType = "CommandExecution"
	ItemFileChange        ItemType = "FileChange"
	ItemMcpToolCall       ItemType = "McpToolCall"
	ItemWebSearch         ItemType = "WebSearch"
	ItemContextCompaction ItemType = "ContextCompaction"
	ItemExtension         ItemType = "Extension"
	ItemPlan              ItemType = "Plan"
)

// IsKnown reports a modelled item type. Pure.
func (t ItemType) IsKnown() bool {
	switch t {
	case ItemUserMessage, ItemAgentMessage, ItemReasoning, ItemCommandExecution, ItemFileChange,
		ItemMcpToolCall, ItemWebSearch, ItemContextCompaction, ItemExtension, ItemPlan:
		return true
	}
	return false
}

// Message roles in response_item/message.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleDeveloper = "developer"
)

// Record is one rollout line. Exactly one of the typed payload pointers is
// set for a known kind; Raw always holds the original line.
type Record struct {
	Time    time.Time
	Ordinal int64
	Kind    RecordKind

	SessionMeta *SessionMeta
	Response    *ResponseItem
	Event       *EventMsg
	TurnContext *TurnContext
	Compacted   *Compacted
	TokenUsage  *TokenUsageRecord
	WorldState  agentio.Raw // large, rarely needed; kept raw

	Raw agentio.Raw
}

// SessionMeta is the first record of a rollout. Only id, timestamp and cwd
// are always present; the rest are presence-tracked by zero/nil.
type SessionMeta struct {
	ID                    string          `json:"id"`
	SessionID             string          `json:"session_id,omitempty"`
	ForkedFromID          string          `json:"forked_from_id,omitempty"`
	ParentThreadID        string          `json:"parent_thread_id,omitempty"`
	Timestamp             string          `json:"timestamp"`
	Cwd                   string          `json:"cwd"`
	Originator            string          `json:"originator,omitempty"`
	CLIVersion            string          `json:"cli_version,omitempty"`
	Source                SessionSource   `json:"source,omitempty"`
	ModelProvider         string          `json:"model_provider,omitempty"`
	HistoryMode           string          `json:"history_mode,omitempty"`
	ThreadSource          string          `json:"thread_source,omitempty"`
	AgentNickname         string          `json:"agent_nickname,omitempty"`
	AgentRole             string          `json:"agent_role,omitempty"`
	Git                   *GitInfo        `json:"git,omitempty"`
	RuntimeWorkspaceRoots []string        `json:"runtime_workspace_roots,omitempty"`
	CreatorUserID         string          `json:"creator_user_id,omitempty"`
	CreatorAccountID      string          `json:"creator_account_id,omitempty"`
	Unknown               agentio.Unknown `json:"-"`
}

// Created parses Timestamp; zero when unparsable. Pure.
func (m SessionMeta) Created() time.Time { return parseRFC3339(m.Timestamp) }

// Version parses CLIVersion; zero when absent. Pure.
func (m SessionMeta) Version() agentio.Version {
	v, _ := agentio.ParseVersion(m.CLIVersion)
	return v
}

// ContentItem is one element of a message's content array.
type ContentItem struct {
	Type string `json:"type"` // input_text | output_text | input_image | …
	Text string `json:"text,omitempty"`
}

// ResponseItem is a response_item payload: the model-facing history.
type ResponseItem struct {
	Type      ResponseType  `json:"type"`
	Role      string        `json:"role,omitempty"`
	Content   []ContentItem `json:"content,omitempty"`
	Name      string        `json:"name,omitempty"`
	CallID    string        `json:"call_id,omitempty"`
	Arguments string        `json:"arguments,omitempty"`
	Input     string        `json:"input,omitempty"`
	Output    agentio.Raw   `json:"output,omitempty"`
	Summary   []ContentItem `json:"summary,omitempty"`
	Status    string        `json:"status,omitempty"`
}

// Text joins the text parts of Content. Pure.
func (r ResponseItem) Text() string { return joinText(r.Content) }

func joinText(items []ContentItem) string {
	var parts []string
	for _, c := range items {
		if c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// EventMsg is an event_msg payload. Fields not used by a given Type stay zero.
type EventMsg struct {
	Type             EventType       `json:"type"`
	ThreadID         string          `json:"thread_id,omitempty"`
	TurnID           string          `json:"turn_id,omitempty"`
	Message          string          `json:"message,omitempty"` // user_message / agent_message (older versions)
	LastAgentMessage string          `json:"last_agent_message,omitempty"`
	DurationMs       int64           `json:"duration_ms,omitempty"`
	Reason           string          `json:"reason,omitempty"` // turn_aborted
	Item             *TurnItem       `json:"item,omitempty"`
	Info             *TokenUsageInfo `json:"info,omitempty"`        // token_count
	RateLimits       *RateLimits     `json:"rate_limits,omitempty"` // token_count
	Error            *EventError     `json:"error,omitempty"`
}

// EventError is task_complete.error.
type EventError struct {
	Message        string `json:"message"`
	CodexErrorInfo string `json:"codex_error_info,omitempty"`
}

// TurnItem is the typed item of item_completed (header + text; the full
// item stays in Raw because its schema is per item type).
type TurnItem struct {
	Type ItemType    `json:"type"`
	ID   string      `json:"id"`
	Raw  agentio.Raw `json:"-"`
	text string
}

// Text is the user or agent message text of the item; "" for other types. Pure.
func (t TurnItem) Text() string { return t.text }

type turnItemWire struct {
	Type    ItemType `json:"type"`
	ID      string   `json:"id"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Text string `json:"text"`
}

// UnmarshalJSON keeps the raw item and extracts message text. Pure.
func (t *TurnItem) UnmarshalJSON(b []byte) error {
	var w turnItemWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	var parts []string
	for _, c := range w.Content {
		if c.Text != "" {
			parts = append(parts, c.Text)
		}
	}
	text := strings.Join(parts, "\n")
	if text == "" {
		text = w.Text
	}
	*t = TurnItem{Type: w.Type, ID: w.ID, Raw: append(agentio.Raw(nil), b...), text: text}
	return nil
}

// TokenCounts is upstream TokenUsage.
type TokenCounts struct {
	Input           int64 `json:"input_tokens"`
	CachedInput     int64 `json:"cached_input_tokens"`
	CacheWriteInput int64 `json:"cache_write_input_tokens"`
	Output          int64 `json:"output_tokens"`
	ReasoningOutput int64 `json:"reasoning_output_tokens"`
	Total           int64 `json:"total_tokens"`
}

// Neutral maps onto agentio.TokenCounts (CacheWriteInput → CacheWrite,
// ReasoningOutput → Reasoning; Total dropped, derivable). Pure.
func (t TokenCounts) Neutral() agentio.TokenCounts {
	return agentio.TokenCounts{Input: t.Input, CachedInput: t.CachedInput, CacheWrite: t.CacheWriteInput, Output: t.Output, Reasoning: t.ReasoningOutput}
}

// TokenUsageInfo is token_count.info.
type TokenUsageInfo struct {
	TotalTokenUsage    *TokenCounts `json:"total_token_usage,omitempty"`
	LastTokenUsage     *TokenCounts `json:"last_token_usage,omitempty"`
	ModelContextWindow int64        `json:"model_context_window,omitempty"`
}

// RateLimits is token_count.rate_limits: the server's snapshot at that moment.
type RateLimits struct {
	LimitID              string           `json:"limit_id,omitempty"`
	LimitName            string           `json:"limit_name,omitempty"`
	Primary              *RateLimitWindow `json:"primary,omitempty"`
	Secondary            *RateLimitWindow `json:"secondary,omitempty"`
	Credits              agentio.Raw      `json:"credits,omitempty"`
	IndividualLimit      agentio.Raw      `json:"individual_limit,omitempty"`
	SpendControlReached  *bool            `json:"spend_control_reached,omitempty"`
	PlanType             string           `json:"plan_type,omitempty"`
	RateLimitReachedType string           `json:"rate_limit_reached_type,omitempty"`
}

// RateLimitWindow is one window of a snapshot.
type RateLimitWindow struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int64   `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
}

// TurnContext is a turn_context payload.
type TurnContext struct {
	TurnID            string           `json:"turn_id,omitempty"`
	Cwd               string           `json:"cwd"`
	ApprovalPolicy    ApprovalPolicy   `json:"-"`
	SandboxPolicy     *SandboxPolicy   `json:"sandbox_policy,omitempty"`
	Model             string           `json:"model"`
	Effort            ReasoningEffort  `json:"effort,omitempty"`
	Summary           ReasoningSummary `json:"summary,omitempty"`
	Timezone          string           `json:"timezone,omitempty"`
	CurrentDate       string           `json:"current_date,omitempty"`
	CollaborationMode agentio.Raw      `json:"collaboration_mode,omitempty"`
	PermissionProfile agentio.Raw      `json:"permission_profile,omitempty"`
}

// Compacted is a compacted payload.
type Compacted struct {
	Message string `json:"message"`
}

// TokenUsageRecord is a token_usage_record payload.
type TokenUsageRecord struct {
	ThreadID         string       `json:"thread_id"`
	TurnID           string       `json:"turn_id"`
	Usage            *TokenCounts `json:"usage,omitempty"`
	TurnTokenUsage   *TokenCounts `json:"turn_token_usage,omitempty"`
	ThreadTokenUsage *TokenCounts `json:"thread_token_usage,omitempty"`
}

type recordWire struct {
	Timestamp string          `json:"timestamp"`
	Ordinal   int64           `json:"ordinal"`
	Type      RecordKind      `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

func sessionMetaKnownKeys() []string {
	return []string{"id", "session_id", "forked_from_id", "parent_thread_id", "timestamp", "cwd", "originator", "cli_version",
		"source", "model_provider", "history_mode", "thread_source", "agent_nickname", "agent_role", "git",
		"runtime_workspace_roots", "creator_user_id", "creator_account_id"}
}

// approvalWire reads approval_policy as a string or a {granular: …} object.
type approvalWire struct {
	ApprovalPolicy json.RawMessage `json:"approval_policy"`
}

// ParseRecord decodes one rollout line. A payload that does not fit its
// kind's model degrades to Raw only (Kind kept), never an error; only a
// line that is not a JSON object fails. Pure.
func ParseRecord(line []byte) (Record, error) {
	var w recordWire
	if err := json.Unmarshal(line, &w); err != nil {
		return Record{}, fmt.Errorf("codex: rollout line: %w", err)
	}
	r := Record{Time: parseRFC3339(w.Timestamp), Ordinal: w.Ordinal, Kind: w.Type, Raw: append(agentio.Raw(nil), line...)}
	p := w.Payload
	switch w.Type {
	case KindSessionMeta:
		var m SessionMeta
		if json.Unmarshal(p, &m) == nil {
			m.Unknown, _ = agentio.ExtractUnknown(p, sessionMetaKnownKeys())
			r.SessionMeta = &m
		}
	case KindResponseItem:
		var v ResponseItem
		if json.Unmarshal(p, &v) == nil {
			r.Response = &v
		}
	case KindEventMsg:
		var v EventMsg
		if json.Unmarshal(p, &v) == nil {
			r.Event = &v
		}
	case KindTurnContext:
		var v TurnContext
		if json.Unmarshal(p, &v) == nil {
			var aw approvalWire
			if json.Unmarshal(p, &aw) == nil {
				v.ApprovalPolicy = decodeApproval(aw.ApprovalPolicy)
			}
			r.TurnContext = &v
		}
	case KindCompacted:
		var v Compacted
		if json.Unmarshal(p, &v) == nil {
			r.Compacted = &v
		}
	case KindTokenUsageRecord:
		var v TokenUsageRecord
		if json.Unmarshal(p, &v) == nil {
			r.TokenUsage = &v
		}
	case KindWorldState:
		r.WorldState = append(agentio.Raw(nil), p...)
	}
	return r, nil
}

func decodeApproval(raw json.RawMessage) ApprovalPolicy {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return ApprovalPolicy(s)
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		for k := range obj {
			return ApprovalPolicy(k)
		}
	}
	return ""
}

func parseRFC3339(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// Rollout reader limits.
const (
	// maxLineBytes bounds one rollout line (base_instructions and
	// world_state lines are large but well under this).
	maxLineBytes  = 16 << 20
	readChunk     = 64 << 10
	newline       = '\n'
	tailReadBytes = 256 << 10
	headScanBytes = 256 << 10
)

// RolloutReader streams records from a rollout file, tracking the byte
// offset just after the last complete line it returned (G4). An incomplete
// trailing line (Codex still writing) is never consumed.
type RolloutReader struct {
	f      *os.File
	br     *bufio.Reader
	offset agentio.Offset
}

// OpenRollout opens path for streaming from the start. Disk · Read.
func (a *Agent) OpenRollout(path string) (*RolloutReader, error) {
	return a.OpenRolloutAt(path, 0)
}

// OpenRolloutAt opens path and resumes after a stored offset. Disk · Read.
func (a *Agent) OpenRolloutAt(path string, from agentio.Offset) (*RolloutReader, error) {
	return openRolloutAt(path, from)
}

func openRolloutAt(path string, from agentio.Offset) (*RolloutReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("codex: open rollout: %w", err)
	}
	if from > 0 {
		if _, err := f.Seek(int64(from), io.SeekStart); err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("codex: seek rollout: %w", err)
		}
	}
	return &RolloutReader{f: f, br: bufio.NewReaderSize(f, readChunk), offset: from}, nil
}

// ErrLineTooLong means a rollout line exceeded the reader's bound.
var ErrLineTooLong = errors.New("codex: rollout line exceeds the size limit")

// Next returns the next complete record; io.EOF at the end (including
// when only an incomplete trailing line remains). Blank lines are skipped.
// Disk · Read.
func (r *RolloutReader) Next() (Record, error) {
	for {
		line, err := r.readLine()
		if err != nil {
			return Record{}, err
		}
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		return ParseRecord(trimmed)
	}
}

// readLine returns one complete line and advances the offset; an
// unterminated tail is left unread (io.EOF).
func (r *RolloutReader) readLine() ([]byte, error) {
	var buf []byte
	for {
		chunk, err := r.br.ReadSlice(newline)
		buf = append(buf, chunk...)
		if len(buf) > maxLineBytes {
			return nil, ErrLineTooLong
		}
		switch {
		case err == nil:
			r.offset += agentio.Offset(len(buf))
			return buf, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			return nil, io.EOF
		default:
			return nil, fmt.Errorf("codex: read rollout: %w", err)
		}
	}
}

// Offset is the byte offset just after the last complete line returned. Pure.
func (r *RolloutReader) Offset() agentio.Offset { return r.offset }

// Close releases the file. Pure (no write).
func (r *RolloutReader) Close() error { return r.f.Close() }

// RolloutTail decodes the complete lines within the last maxBytes of path
// (the first, possibly cut, line is dropped). Never reads the whole file.
// Disk · Read.
func (a *Agent) RolloutTail(path string, maxBytes int64) ([]Record, error) {
	return rolloutTail(path, maxBytes)
}

func rolloutTail(path string, maxBytes int64) ([]Record, error) {
	b, cut, err := readTail(path, maxBytes)
	if err != nil {
		return nil, err
	}
	if cut {
		if i := bytes.IndexByte(b, newline); i >= 0 {
			b = b[i+1:]
		} else {
			return nil, nil
		}
	}
	var out []Record
	for _, line := range bytes.Split(b, []byte{newline}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		rec, err := ParseRecord(line)
		if err != nil {
			continue // a torn last line while Codex writes
		}
		out = append(out, rec)
	}
	return out, nil
}

// readTail returns the last maxBytes of path and whether it cut the start.
func readTail(path string, maxBytes int64) ([]byte, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	// Read-only file: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	size := st.Size()
	start := int64(0)
	if maxBytes > 0 && size > maxBytes {
		start = size - maxBytes
	}
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	return buf, start > 0, nil
}

// readHeadMeta reads a rollout's first line (bounded) as session_meta.
func readHeadMeta(path string) (SessionMeta, bool) {
	f, err := os.Open(path)
	if err != nil {
		return SessionMeta{}, false
	}
	// Read-only file: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	br := bufio.NewReaderSize(io.LimitReader(f, headScanBytes), readChunk)
	line, err := br.ReadSlice(newline)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return SessionMeta{}, false
	}
	if errors.Is(err, bufio.ErrBufferFull) {
		// session_meta with large base_instructions: read the rest of the line.
		rest, rerr := br.ReadBytes(newline)
		if rerr != nil && !errors.Is(rerr, io.EOF) {
			return SessionMeta{}, false
		}
		line = append(append([]byte{}, line...), rest...)
	}
	rec, perr := ParseRecord(bytes.TrimSpace(line))
	if perr != nil || rec.SessionMeta == nil || rec.SessionMeta.ID == "" {
		return SessionMeta{}, false
	}
	return *rec.SessionMeta, true
}

// ConversationTokens returns the last token_usage_record's thread totals
// (or, for older rollouts, the last token_count total), from a tail read.
// Disk · Read.
func (a *Agent) ConversationTokens(path string) (TokenCounts, error) {
	recs, err := rolloutTail(path, tailReadBytes)
	if err != nil {
		return TokenCounts{}, fmt.Errorf("codex: tail rollout: %w", err)
	}
	for i := len(recs) - 1; i >= 0; i-- {
		r := recs[i]
		if r.TokenUsage != nil && r.TokenUsage.ThreadTokenUsage != nil {
			return *r.TokenUsage.ThreadTokenUsage, nil
		}
		if r.Event != nil && r.Event.Type == EventTokenCount && r.Event.Info != nil && r.Event.Info.TotalTokenUsage != nil {
			return *r.Event.Info.TotalTokenUsage, nil
		}
	}
	return TokenCounts{}, nil
}
