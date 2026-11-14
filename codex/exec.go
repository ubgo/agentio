package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/khanakia/agentcli"
)

// ExecEventType is a `codex exec --json` line's `type`
// (exec/src/exec_events.rs at rust-v0.162.1). Open set.
type ExecEventType string

// ExecEventType values.
const (
	ExecThreadStarted ExecEventType = "thread.started"
	ExecTurnStarted   ExecEventType = "turn.started"
	ExecTurnCompleted ExecEventType = "turn.completed"
	ExecTurnFailed    ExecEventType = "turn.failed"
	ExecItemStarted   ExecEventType = "item.started"
	ExecItemUpdated   ExecEventType = "item.updated"
	ExecItemCompleted ExecEventType = "item.completed"
	ExecErrorEvent    ExecEventType = "error"
)

// IsKnown reports a known event type. Pure.
func (t ExecEventType) IsKnown() bool {
	switch t {
	case ExecThreadStarted, ExecTurnStarted, ExecTurnCompleted, ExecTurnFailed, ExecItemStarted,
		ExecItemUpdated, ExecItemCompleted, ExecErrorEvent:
		return true
	}
	return false
}

// ExecItemType is an exec item's snake_case `type`.
type ExecItemType string

// ExecItemType values.
const (
	ExecAgentMessage     ExecItemType = "agent_message"
	ExecReasoning        ExecItemType = "reasoning"
	ExecCommandExecution ExecItemType = "command_execution"
	ExecFileChange       ExecItemType = "file_change"
	ExecMCPToolCall      ExecItemType = "mcp_tool_call"
	ExecCollabToolCall   ExecItemType = "collab_tool_call"
	ExecWebSearch        ExecItemType = "web_search"
	ExecTodoList         ExecItemType = "todo_list"
	ExecItemError        ExecItemType = "error"
)

// IsKnown reports a known item type. Pure.
func (t ExecItemType) IsKnown() bool {
	switch t {
	case ExecAgentMessage, ExecReasoning, ExecCommandExecution, ExecFileChange, ExecMCPToolCall,
		ExecCollabToolCall, ExecWebSearch, ExecTodoList, ExecItemError:
		return true
	}
	return false
}

// ExecError is turn.failed.error / error.message.
type ExecError struct {
	Message string `json:"message"`
}

// TextItem is agent_message / reasoning.
type TextItem struct {
	Text string `json:"text"`
}

// CommandItem is command_execution.
type CommandItem struct {
	Command          string `json:"command"`
	AggregatedOutput string `json:"aggregated_output"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	Status           string `json:"status"` // in_progress | completed | failed | declined
}

// FileChange is one file_change change.
type FileChange struct {
	Path string `json:"path"`
	Kind string `json:"kind"` // add | delete | update
}

// FileChangeItem is file_change.
type FileChangeItem struct {
	Changes []FileChange `json:"changes"`
	Status  string       `json:"status"`
}

// MCPToolCallItem is mcp_tool_call; arguments and results stay raw
// (vendor-defined schemas).
type MCPToolCallItem struct {
	Server    string       `json:"server"`
	Tool      string       `json:"tool"`
	Arguments agentcli.Raw `json:"arguments,omitempty"`
	Result    agentcli.Raw `json:"result,omitempty"`
	Error     *ExecError   `json:"error,omitempty"`
	Status    string       `json:"status"`
}

// CollabItem is collab_tool_call.
type CollabItem struct {
	Tool              string       `json:"tool"` // spawn_agent | send_input | wait | close_agent
	SenderThreadID    string       `json:"sender_thread_id"`
	ReceiverThreadIDs []string     `json:"receiver_thread_ids"`
	Prompt            string       `json:"prompt,omitempty"`
	AgentsStates      agentcli.Raw `json:"agents_states,omitempty"`
	Status            string       `json:"status"`
}

// WebSearchItem is web_search.
type WebSearchItem struct {
	ID      string       `json:"id"`
	Query   string       `json:"query"`
	Action  agentcli.Raw `json:"action,omitempty"`
	Results agentcli.Raw `json:"results,omitempty"`
}

// TodoItem is one todo_list entry.
type TodoItem struct {
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

// TodoListItem is todo_list.
type TodoListItem struct {
	Items []TodoItem `json:"items"`
}

// ExecItem is an item.* payload; exactly one typed pointer is set for a
// known type.
type ExecItem struct {
	ID             string
	Type           ExecItemType
	AgentMessage   *TextItem
	Reasoning      *TextItem
	Command        *CommandItem
	FileChange     *FileChangeItem
	MCPToolCall    *MCPToolCallItem
	CollabToolCall *CollabItem
	WebSearch      *WebSearchItem
	TodoList       *TodoListItem
	Error          *ExecError
}

// ExecEvent is one `exec --json` line.
type ExecEvent struct {
	Type     ExecEventType
	ThreadID string       // thread.started
	Usage    *TokenCounts // turn.completed
	Error    *ExecError   // turn.failed, error
	Item     *ExecItem    // item.*
	Raw      agentcli.Raw // always the full line
}

type execEventWire struct {
	Type     ExecEventType   `json:"type"`
	ThreadID string          `json:"thread_id"`
	Usage    *TokenCounts    `json:"usage"`
	Error    *ExecError      `json:"error"`
	Message  string          `json:"message"`
	Item     json.RawMessage `json:"item"`
}

type execItemHeader struct {
	ID   string       `json:"id"`
	Type ExecItemType `json:"type"`
}

// ParseExecEvent decodes one line. Unknown types keep Type and Raw; a
// known type whose payload does not fit keeps Raw only. Pure.
func ParseExecEvent(line []byte) (ExecEvent, error) {
	var w execEventWire
	if err := json.Unmarshal(line, &w); err != nil {
		return ExecEvent{}, fmt.Errorf("codex: exec event: %w", err)
	}
	ev := ExecEvent{Type: w.Type, ThreadID: w.ThreadID, Usage: w.Usage, Error: w.Error, Raw: append(agentcli.Raw(nil), line...)}
	if w.Type == ExecErrorEvent && ev.Error == nil && w.Message != "" {
		ev.Error = &ExecError{Message: w.Message}
	}
	if len(w.Item) > 0 {
		ev.Item = parseExecItem(w.Item)
	}
	return ev, nil
}

func parseExecItem(raw json.RawMessage) *ExecItem {
	var h execItemHeader
	if json.Unmarshal(raw, &h) != nil {
		return nil
	}
	it := &ExecItem{ID: h.ID, Type: h.Type}
	switch h.Type {
	case ExecAgentMessage:
		it.AgentMessage = decodeInto[TextItem](raw)
	case ExecReasoning:
		it.Reasoning = decodeInto[TextItem](raw)
	case ExecCommandExecution:
		it.Command = decodeInto[CommandItem](raw)
	case ExecFileChange:
		it.FileChange = decodeInto[FileChangeItem](raw)
	case ExecMCPToolCall:
		it.MCPToolCall = decodeInto[MCPToolCallItem](raw)
	case ExecCollabToolCall:
		it.CollabToolCall = decodeInto[CollabItem](raw)
	case ExecWebSearch:
		it.WebSearch = decodeInto[WebSearchItem](raw)
	case ExecTodoList:
		it.TodoList = decodeInto[TodoListItem](raw)
	case ExecItemError:
		it.Error = decodeInto[ExecError](raw)
	}
	return it
}

// decodeInto decodes raw into a fresh T; nil when it does not fit.
func decodeInto[T TextItem | CommandItem | FileChangeItem | MCPToolCallItem | CollabItem | WebSearchItem | TodoListItem | ExecError](raw json.RawMessage) *T {
	var v T
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return &v
}

// ExecDecoder reads `codex exec --json` stdout line by line.
type ExecDecoder struct {
	sc *bufio.Scanner
}

// NewExecDecoder wraps the stdout of a process the caller runs. Pure.
func NewExecDecoder(r io.Reader) *ExecDecoder {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, readChunk), maxLineBytes)
	return &ExecDecoder{sc: sc}
}

// Next returns the next event; io.EOF at the end. Blank and non-JSON lines
// are skipped (exec may print diagnostics). Pure (reads the given reader).
func (d *ExecDecoder) Next() (ExecEvent, error) {
	for d.sc.Scan() {
		line := bytes.TrimSpace(d.sc.Bytes())
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		return ParseExecEvent(line)
	}
	if err := d.sc.Err(); err != nil {
		return ExecEvent{}, fmt.Errorf("codex: exec stream: %w", err)
	}
	return ExecEvent{}, io.EOF
}

// ExecEventVersion is the upstream version the decoder was written
// against. Pure.
func (a *Agent) ExecEventVersion() agentcli.Version { return agentcli.MustVersion(TestedMax) }

// runDecoder is the neutral view over ExecDecoder (L2 RunEvents).
type runDecoder struct{ d *ExecDecoder }

// NewRunDecoder returns the neutral decoder over r. Pure.
func (a *Agent) NewRunDecoder(r io.Reader) agentcli.RunDecoder {
	return runDecoder{d: NewExecDecoder(r)}
}

// Next maps one exec event onto agentcli.RunEvent. Pure.
func (r runDecoder) Next() (agentcli.RunEvent, error) {
	ev, err := r.d.Next()
	if err != nil {
		return agentcli.RunEvent{}, err
	}
	out := agentcli.RunEvent{Type: agentcli.RunEventOther, ConversationID: ev.ThreadID, Raw: ev.Raw}
	switch ev.Type {
	case ExecThreadStarted:
		out.Type = agentcli.RunEventStarted
	case ExecTurnCompleted:
		out.Type = agentcli.RunEventDone
		if ev.Usage != nil {
			n := ev.Usage.Neutral()
			out.Tokens = &n
		}
	case ExecTurnFailed, ExecErrorEvent:
		out.Type = agentcli.RunEventError
		if ev.Error != nil {
			out.Text = ev.Error.Message
		}
	case ExecItemCompleted, ExecItemStarted, ExecItemUpdated:
		if ev.Item == nil {
			break
		}
		switch {
		case ev.Item.AgentMessage != nil:
			out.Type, out.Text = agentcli.RunEventMessage, ev.Item.AgentMessage.Text
		case ev.Item.Command != nil || ev.Item.MCPToolCall != nil || ev.Item.FileChange != nil || ev.Item.WebSearch != nil || ev.Item.CollabToolCall != nil:
			out.Type = agentcli.RunEventTool
		case ev.Item.Error != nil:
			out.Type, out.Text = agentcli.RunEventError, ev.Item.Error.Message
		}
	}
	return out, nil
}
