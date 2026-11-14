package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// RecordType is a transcript record type (open set; unknown types decode).
type RecordType string

// Record types observed in tails of ~150 transcripts (CLAUDE_API §18).
const (
	RecordUser                RecordType = "user"
	RecordAssistant           RecordType = "assistant"
	RecordAttachment          RecordType = "attachment"
	RecordSystem              RecordType = "system"
	RecordAPIRequest          RecordType = "api-request"
	RecordAPIRequestBlob      RecordType = "api-request-blob"
	RecordAtisLatch           RecordType = "atis-latch"
	RecordLastPrompt          RecordType = "last-prompt"
	RecordPermissionMode      RecordType = "permission-mode"
	RecordMode                RecordType = "mode"
	RecordAITitle             RecordType = "ai-title"
	RecordCustomTitle         RecordType = "custom-title"
	RecordBridgeSession       RecordType = "bridge-session"
	RecordCostState           RecordType = "cost-state"
	RecordSessionOwner        RecordType = "session-owner"
	RecordFileHistorySnapshot RecordType = "file-history-snapshot"
	RecordFileHistoryDelta    RecordType = "file-history-delta"
	RecordQueueOperation      RecordType = "queue-operation"
	RecordToolsUsed           RecordType = "tools-used"
	RecordHistorySuppression  RecordType = "history-suppression"
	RecordPRLink              RecordType = "pr-link"
	RecordAgentName           RecordType = "agent-name"
	RecordContinuedIn         RecordType = "continued-in"
	RecordAPIRequestShape     RecordType = "api-request-shape"
)

// RecordTypes lists the recorded types. cost: Pure.
func RecordTypes() []RecordType {
	return []RecordType{RecordUser, RecordAssistant, RecordAttachment, RecordSystem, RecordAPIRequest, RecordAPIRequestBlob,
		RecordAtisLatch, RecordLastPrompt, RecordPermissionMode, RecordMode, RecordAITitle, RecordCustomTitle, RecordBridgeSession,
		RecordCostState, RecordSessionOwner, RecordFileHistorySnapshot, RecordFileHistoryDelta, RecordQueueOperation, RecordToolsUsed,
		RecordHistorySuppression, RecordPRLink, RecordAgentName, RecordContinuedIn, RecordAPIRequestShape}
}

// IsKnown reports a recorded type. cost: Pure.
func (t RecordType) IsKnown() bool {
	for _, k := range RecordTypes() {
		if k == t {
			return true
		}
	}
	return false
}

// isInternal reports the types that may carry system prompts and are never
// decoded (OD-8).
func (t RecordType) isInternal() bool {
	return t == RecordAPIRequest || t == RecordAPIRequestBlob || t == RecordAtisLatch
}

// ErrInternalRecord means a typed accessor was asked to decode an internal
// record (api-request, api-request-blob, atis-latch); use InternalRecords.
var ErrInternalRecord = errors.New("claude: internal transcript record is never decoded")

// ErrWrongRecordType means an AsX accessor was called on another type.
var ErrWrongRecordType = errors.New("claude: wrong transcript record type")

// Record is the envelope of one transcript line; the payload stays in Raw
// and typed accessors decode it.
type Record struct {
	Type        RecordType  `json:"type"`
	Subtype     string      `json:"subtype,omitempty"`
	UUID        string      `json:"uuid,omitempty"`
	ParentUUID  string      `json:"parentUuid,omitempty"`
	SessionID   string      `json:"sessionId,omitempty"`
	Timestamp   *time.Time  `json:"timestamp,omitempty"`
	Cwd         string      `json:"cwd,omitempty"`
	GitBranch   string      `json:"gitBranch,omitempty"`
	Version     string      `json:"version,omitempty"`
	Entrypoint  string      `json:"entrypoint,omitempty"`
	IsSidechain bool        `json:"isSidechain,omitempty"`
	Raw         agentio.Raw `json:"-"`
}

// ParseRecord decodes one transcript line; any valid JSON object decodes,
// whatever its type. cost: Pure.
func ParseRecord(line []byte) (Record, error) {
	var r Record
	if err := json.Unmarshal(line, &r); err != nil {
		return Record{}, fmt.Errorf("claude: transcript line: %w", err)
	}
	r.Raw = agentio.Raw(bytes.Clone(line))
	return r, nil
}

func (r Record) as(t RecordType, dst func([]byte) error) error {
	if r.Type.isInternal() {
		return ErrInternalRecord
	}
	if r.Type != t {
		return fmt.Errorf("%w: %s is not %s", ErrWrongRecordType, r.Type, t)
	}
	return dst(r.Raw)
}

// BlockType is a content block type (open set).
type BlockType string

// Content block types.
const (
	BlockText       BlockType = "text"
	BlockThinking   BlockType = "thinking"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
	BlockImage      BlockType = "image"
)

// IsKnown reports a recorded block type. cost: Pure.
func (b BlockType) IsKnown() bool {
	switch b {
	case BlockText, BlockThinking, BlockToolUse, BlockToolResult, BlockImage:
		return true
	}
	return false
}

// ContentBlock is one message content block.
type ContentBlock struct {
	Type      BlockType
	Text      string
	ToolUseID string
	ToolName  string
	Input     agentio.Raw
	IsError   bool
	Unknown   agentio.Unknown
}

type wireBlock struct {
	Type      BlockType       `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	ToolUseID string          `json:"tool_use_id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	IsError   bool            `json:"is_error"`
}

// UnmarshalJSON decodes a block, keeping unmodelled keys (tool_result
// content, signatures, image sources). cost: Pure.
func (b *ContentBlock) UnmarshalJSON(data []byte) error {
	var w wireBlock
	unknown, _, err := decodeLenient("content", data, reflect.TypeFor[wireBlock](), func(d []byte) error { w = wireBlock{}; return json.Unmarshal(d, &w) })
	if err != nil {
		return err
	}
	*b = ContentBlock{Type: w.Type, Text: w.Text, ToolUseID: w.ToolUseID, ToolName: w.Name, Input: rawOrNil(w.Input), IsError: w.IsError, Unknown: unknown}
	if w.Type == BlockThinking {
		b.Text = w.Thinking
	}
	if w.Type == BlockToolUse {
		b.ToolUseID = w.ID
	}
	return nil
}

// Content is a message content: a plain string or a block array.
type Content []ContentBlock

// UnmarshalJSON accepts both shapes. cost: Pure.
func (c *Content) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*c = Content{{Type: BlockText, Text: s}}
		return nil
	}
	var blocks []ContentBlock
	if err := json.Unmarshal(data, &blocks); err != nil {
		return err
	}
	*c = blocks
	return nil
}

// Text joins the text blocks with "\n". cost: Pure.
func (c Content) Text() string {
	var parts []string
	for _, b := range c {
		if b.Type == BlockText && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// HasText reports a non-empty text block. cost: Pure.
func (c Content) HasText() bool { return c.Text() != "" }

// MessageUsage is an assistant message's usage block.
type MessageUsage struct {
	Input         int64           `json:"input_tokens"`
	Output        int64           `json:"output_tokens"`
	CacheRead     int64           `json:"cache_read_input_tokens"`
	CacheCreation int64           `json:"cache_creation_input_tokens"`
	ServiceTier   string          `json:"service_tier"`
	Unknown       agentio.Unknown `json:"-"`
}

// usageFile names a usage block in decode warnings.
const usageFile = "usage"

// UnmarshalJSON decodes the counts and keeps every other key (server tool
// use, iterations, cache_creation split, speed, …) in Unknown, so the same
// type serves transcripts and stream-json. cost: Pure.
func (u *MessageUsage) UnmarshalJSON(data []byte) error {
	type plain MessageUsage // no methods: avoids recursing into this one
	var p plain
	unknown, _, err := decodeLenient(usageFile, data, reflect.TypeFor[plain](), func(d []byte) error { p = plain{}; return json.Unmarshal(d, &p) })
	if err != nil {
		return err
	}
	*u = MessageUsage(p)
	u.Unknown = unknown
	return nil
}

// Neutral maps onto the shared counts. cost: Pure.
func (u MessageUsage) Neutral() agentio.TokenCounts {
	return agentio.TokenCounts{Input: u.Input, CachedInput: u.CacheRead, CacheWrite: u.CacheCreation, Output: u.Output}
}

// AssistantRecord is a decoded assistant line.
type AssistantRecord struct {
	Record
	MessageID, RequestID, Model, RequestedModel, StopReason string
	Effort                                                  Effort
	Content                                                 Content
	Usage                                                   *MessageUsage
}

type wireAssistant struct {
	RequestID      string `json:"requestId"`
	RequestedModel string `json:"requestedModel"`
	Effort         Effort `json:"effort"`
	Message        struct {
		ID         string          `json:"id"`
		Model      string          `json:"model"`
		Content    Content         `json:"content"`
		StopReason string          `json:"stop_reason"`
		Usage      json.RawMessage `json:"usage"`
	} `json:"message"`
}

// AsAssistant decodes an assistant line. cost: Pure.
func (r Record) AsAssistant() (AssistantRecord, error) {
	var w wireAssistant
	if err := r.as(RecordAssistant, func(b []byte) error { return json.Unmarshal(b, &w) }); err != nil {
		return AssistantRecord{}, err
	}
	out := AssistantRecord{Record: r, MessageID: w.Message.ID, RequestID: w.RequestID, Model: w.Message.Model,
		RequestedModel: w.RequestedModel, StopReason: w.Message.StopReason, Effort: w.Effort, Content: w.Message.Content}
	if len(w.Message.Usage) > 0 && string(w.Message.Usage) != jsonNull {
		var mu MessageUsage
		if err := json.Unmarshal(w.Message.Usage, &mu); err == nil {
			out.Usage = &mu
		}
	}
	return out, nil
}

// UserRecord is a decoded user line.
type UserRecord struct {
	Record
	PromptID       string
	PermissionMode PermissionMode
	Content        Content
	// IsToolResult is true when every block is a tool_result (a tool turn,
	// not a typed prompt).
	IsToolResult bool
}

type wireUser struct {
	PromptID       string         `json:"promptId"`
	PermissionMode PermissionMode `json:"permissionMode"`
	Message        struct {
		Content Content `json:"content"`
	} `json:"message"`
}

// AsUser decodes a user line. cost: Pure.
func (r Record) AsUser() (UserRecord, error) {
	var w wireUser
	if err := r.as(RecordUser, func(b []byte) error { return json.Unmarshal(b, &w) }); err != nil {
		return UserRecord{}, err
	}
	out := UserRecord{Record: r, PromptID: w.PromptID, PermissionMode: w.PermissionMode, Content: w.Message.Content}
	out.IsToolResult = len(out.Content) > 0
	for _, b := range out.Content {
		if b.Type != BlockToolResult {
			out.IsToolResult = false
		}
	}
	return out, nil
}

// Title is an ai-title or custom-title record.
type Title struct {
	Type  RecordType
	Title string
}

// AsTitle decodes ai-title / custom-title. cost: Pure.
func (r Record) AsTitle() (Title, error) {
	var w struct {
		AITitle     string `json:"aiTitle"`
		CustomTitle string `json:"customTitle"`
	}
	if r.Type != RecordAITitle && r.Type != RecordCustomTitle {
		return Title{}, fmt.Errorf("%w: %s is not a title", ErrWrongRecordType, r.Type)
	}
	if err := json.Unmarshal(r.Raw, &w); err != nil {
		return Title{}, err
	}
	t := Title{Type: r.Type, Title: w.AITitle}
	if r.Type == RecordCustomTitle {
		t.Title = w.CustomTitle
	}
	return t, nil
}

// LastPrompt is a last-prompt record.
type LastPrompt struct {
	Text     string `json:"lastPrompt"`
	LeafUUID string `json:"leafUuid"`
}

// AsLastPrompt decodes last-prompt. cost: Pure.
func (r Record) AsLastPrompt() (LastPrompt, error) {
	var v LastPrompt
	err := r.as(RecordLastPrompt, func(b []byte) error { return json.Unmarshal(b, &v) })
	return v, err
}

// CostState is a cost-state record: running totals of a session.
type CostState struct {
	StartTime           time.Time
	TotalCostUSD        float64
	TotalDuration       time.Duration
	TotalAPIDuration    time.Duration
	TotalToolDuration   time.Duration
	TotalLinesAdded     int
	TotalLinesRemoved   int
	HasUnknownModelCost bool
	ModelUsage          map[string]ModelUsage
}

type wireCostState struct {
	StartTime           millis                    `json:"startTime"`
	TotalCostUSD        float64                   `json:"totalCostUSD"`
	TotalDuration       float64                   `json:"totalDuration"`
	TotalAPIDuration    float64                   `json:"totalAPIDuration"`
	TotalToolDuration   float64                   `json:"totalToolDuration"`
	TotalLinesAdded     int                       `json:"totalLinesAdded"`
	TotalLinesRemoved   int                       `json:"totalLinesRemoved"`
	HasUnknownModelCost bool                      `json:"hasUnknownModelCost"`
	ModelUsage          map[string]wireModelUsage `json:"modelUsage"`
}

// AsCostState decodes cost-state. cost: Pure.
func (r Record) AsCostState() (CostState, error) {
	var w wireCostState
	if err := r.as(RecordCostState, func(b []byte) error { return json.Unmarshal(b, &w) }); err != nil {
		return CostState{}, err
	}
	return CostState{StartTime: w.StartTime.Time(), TotalCostUSD: w.TotalCostUSD, TotalDuration: msDuration(w.TotalDuration),
		TotalAPIDuration: msDuration(w.TotalAPIDuration), TotalToolDuration: msDuration(w.TotalToolDuration),
		TotalLinesAdded: w.TotalLinesAdded, TotalLinesRemoved: w.TotalLinesRemoved, HasUnknownModelCost: w.HasUnknownModelCost,
		ModelUsage: modelUsageMap(w.ModelUsage)}, nil
}

// PRLink is a pr-link record.
type PRLink struct {
	Number     int        `json:"prNumber"`
	Repository string     `json:"prRepository"`
	URL        string     `json:"prUrl"`
	Timestamp  *time.Time `json:"timestamp"`
}

// AsPRLink decodes pr-link. cost: Pure.
func (r Record) AsPRLink() (PRLink, error) {
	var v PRLink
	err := r.as(RecordPRLink, func(b []byte) error { return json.Unmarshal(b, &v) })
	return v, err
}

// SessionOwner is a session-owner record (account ids are Personal).
type SessionOwner struct {
	AccountUUID      agentio.Personal `json:"accountUuid"`
	OrganizationUUID agentio.Personal `json:"organizationUuid"`
}

// AsSessionOwner decodes session-owner. cost: Pure.
func (r Record) AsSessionOwner() (SessionOwner, error) {
	var v SessionOwner
	err := r.as(RecordSessionOwner, func(b []byte) error { return json.Unmarshal(b, &v) })
	return v, err
}

// AsContinuedIn returns the session a continued-in record points to. cost: Pure.
func (r Record) AsContinuedIn() (string, error) {
	var v struct {
		ID string `json:"continuedInSessionId"`
	}
	err := r.as(RecordContinuedIn, func(b []byte) error { return json.Unmarshal(b, &v) })
	return v.ID, err
}

// AsAgentName returns an agent-name record's name. cost: Pure.
func (r Record) AsAgentName() (string, error) {
	var v struct {
		Name string `json:"agentName"`
	}
	err := r.as(RecordAgentName, func(b []byte) error { return json.Unmarshal(b, &v) })
	return v.Name, err
}

// AsPermissionMode returns a permission-mode record's mode. cost: Pure.
func (r Record) AsPermissionMode() (PermissionMode, error) {
	var v struct {
		Mode PermissionMode `json:"permissionMode"`
	}
	err := r.as(RecordPermissionMode, func(b []byte) error { return json.Unmarshal(b, &v) })
	return v.Mode, err
}

// TranscriptReader streams records from a transcript, caller-driven. It
// never advances past an incomplete trailing line, so a live file can be
// re-opened later from Offset() (G4).
type TranscriptReader struct {
	f      *os.File
	r      *bufio.Reader
	offset agentio.Offset
}

// openTranscriptFile opens path at from.
func openTranscriptFile(path string, from agentio.Offset) (*TranscriptReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("claude: open transcript: %w", err)
	}
	if from > 0 {
		if _, err := f.Seek(int64(from), io.SeekStart); err != nil {
			_ = f.Close() // the seek error is the one reported
			return nil, fmt.Errorf("claude: seek transcript: %w", err)
		}
	}
	return &TranscriptReader{f: f, r: bufio.NewReaderSize(f, scanStartBytes), offset: from}, nil
}

// Next returns the next complete record; io.EOF at the end or before a
// partial trailing line. Blank and non-JSON lines are skipped. A line over
// 16 MiB is an error. cost: Disk · Read.
func (t *TranscriptReader) Next() (Record, error) {
	for {
		line, err := t.readLine()
		if err != nil {
			return Record{}, err
		}
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		rec, perr := ParseRecord(line)
		if perr != nil {
			continue
		}
		return rec, nil
	}
}

// readLine reads one complete line and advances the offset past it.
func (t *TranscriptReader) readLine() ([]byte, error) {
	var buf []byte
	for {
		chunk, err := t.r.ReadSlice('\n')
		buf = append(buf, chunk...)
		if len(buf) > maxLineBytes {
			return nil, fmt.Errorf("claude: transcript line over %d bytes", maxLineBytes)
		}
		switch {
		case err == nil:
			t.offset += agentio.Offset(len(buf))
			return buf, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			// A partial trailing line is left for the next open (live file).
			return nil, io.EOF
		default:
			return nil, err
		}
	}
}

// Offset is the byte offset just after the last complete line read. cost: Pure.
func (t *TranscriptReader) Offset() agentio.Offset { return t.offset }

// Close releases the file. cost: Pure.
func (t *TranscriptReader) Close() error { return t.f.Close() }

// OpenTranscriptFile opens a transcript by path at an offset (ailog live
// capture holds a path from the hook input). cost: Disk · Read.
func (a *Agent) OpenTranscriptFile(path string, from agentio.Offset) (*TranscriptReader, error) {
	return openTranscriptFile(path, from)
}

// TranscriptAt maps a transcript path under <home>/projects/<key>/<id>.jsonl
// to its home and session id; ErrForeignHome when the path is not inside a
// Claude projects dir. cost: Pure.
func (a *Agent) TranscriptAt(path string) (Home, string, error) {
	clean := filepath.Clean(path)
	if filepath.Ext(clean) != transcriptExt {
		return Home{}, "", fmt.Errorf("%w: %s is not a transcript", agentio.ErrForeignHome, path)
	}
	projDir := filepath.Dir(filepath.Dir(clean))
	if filepath.Base(projDir) != projectsDir {
		return Home{}, "", fmt.Errorf("%w: %s is not under a Claude projects dir", agentio.ErrForeignHome, path)
	}
	return a.HomeAt(filepath.Dir(projDir)), strings.TrimSuffix(filepath.Base(clean), transcriptExt), nil
}

// findTranscript locates <id>.jsonl in any project dir of h.
func (a *Agent) findTranscript(h Home, id string) (string, error) {
	if id == "" || strings.ContainsAny(id, `/\`) {
		return "", fmt.Errorf("%w: conversation %q", agentio.ErrNotFound, id)
	}
	matches, err := filepath.Glob(filepath.Join(h.Dir(), projectsDir, "*", id+transcriptExt))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("%w: conversation %s in %s", agentio.ErrNotFound, id, h.Dir())
	}
	return matches[0], nil
}

// OpenTranscript opens a conversation's transcript from the start.
// cost: Disk · Read, streaming.
func (a *Agent) OpenTranscript(h Home, id string) (*TranscriptReader, error) {
	p, err := a.findTranscript(h, id)
	if err != nil {
		return nil, err
	}
	return openTranscriptFile(p, 0)
}

// TranscriptTailFile decodes the records in the last n bytes of path.
// cost: Disk · Read (bounded).
func (a *Agent) TranscriptTailFile(path string, n int64) ([]Record, error) {
	buf, err := readTail(path, n)
	if err != nil {
		return nil, fmt.Errorf("claude: tail transcript: %w", err)
	}
	var out []Record
	for _, line := range splitLines(buf) {
		if r, err := ParseRecord(line); err == nil {
			out = append(out, r)
		}
	}
	return out, nil
}

// TranscriptTail decodes the records in the last n bytes of a conversation.
// cost: Disk · Read (bounded).
func (a *Agent) TranscriptTail(h Home, id string, n int64) ([]Record, error) {
	p, err := a.findTranscript(h, id)
	if err != nil {
		return nil, err
	}
	return a.TranscriptTailFile(p, n)
}

// LastAssistant returns the newest assistant record with text in the
// last 256 KiB (tool-only turns are skipped). cost: Disk · Read (tail only).
func (a *Agent) LastAssistant(h Home, id string) (AssistantRecord, bool, error) {
	recs, err := a.TranscriptTail(h, id, tailScanBytes)
	if err != nil {
		return AssistantRecord{}, false, err
	}
	for i := len(recs) - 1; i >= 0; i-- {
		if recs[i].Type != RecordAssistant {
			continue
		}
		ar, err := recs[i].AsAssistant()
		if err == nil && ar.Content.HasText() {
			return ar, true, nil
		}
	}
	return AssistantRecord{}, false, nil
}

// SessionCost returns the last cost-state record in the tail; nil when
// none. cost: Disk · Read (tail).
func (a *Agent) SessionCost(h Home, id string) (*CostState, error) {
	recs, err := a.TranscriptTail(h, id, tailScanBytes)
	if err != nil {
		return nil, err
	}
	for i := len(recs) - 1; i >= 0; i-- {
		if recs[i].Type == RecordCostState {
			c, err := recs[i].AsCostState()
			if err != nil {
				return nil, err
			}
			return &c, nil
		}
	}
	return nil, nil
}

// PRLinks returns the pr-link records in the tail. cost: Disk · Read (tail).
func (a *Agent) PRLinks(h Home, id string) ([]PRLink, error) {
	recs, err := a.TranscriptTail(h, id, tailScanBytes)
	if err != nil {
		return nil, err
	}
	var out []PRLink
	for _, r := range recs {
		if r.Type == RecordPRLink {
			if l, err := r.AsPRLink(); err == nil {
				out = append(out, l)
			}
		}
	}
	return out, nil
}

// InternalRecords returns api-request, api-request-blob and atis-latch
// lines from the tail as raw bytes, never decoded: they may carry system
// prompts (OD-8). cost: Disk · Read (tail).
func (a *Agent) InternalRecords(h Home, id string) ([]agentio.Raw, error) {
	recs, err := a.TranscriptTail(h, id, tailScanBytes)
	if err != nil {
		return nil, err
	}
	var out []agentio.Raw
	for _, r := range recs {
		if r.Type.isInternal() {
			out = append(out, r.Raw)
		}
	}
	return out, nil
}
