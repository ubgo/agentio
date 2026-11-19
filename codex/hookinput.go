package codex

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/ubgo/agentio"
)

// Hook payloads: UNVERIFIED. No Codex hook run has been recorded on this
// machine yet. The typed fields below come from the vendor's own JSON
// schemas for command hooks, embedded in the Codex 0.162.1 binary
// ("user-prompt-submit.command.input", "stop.command.input",
// "session-start.command.input", "session-end.command.input" and the
// tool / compact / subagent ones); every key outside them is kept in
// Unknown and the whole payload in Raw. The notify payload is still
// untyped beyond its type key.

// hook payload limits.
const maxHookPayload = 16 << 20

// HookInput is a hook's stdin payload (unverified: typed from the vendor
// schemas, never from a recorded run).
type HookInput struct {
	// Event is the event name: hook_event_name, or the first non-empty of
	// the older candidate keys "event" and "type" for a payload without it.
	Event HookEvent `json:"-"`
	// SessionID is the thread (conversation) id.
	SessionID string `json:"session_id"`
	// TranscriptPath is the rollout file; the schema allows null ("").
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	HookEventName  string `json:"hook_event_name"`
	Model          string `json:"model,omitempty"`
	// PermissionMode is one of default, acceptEdits, plan, dontAsk,
	// bypassPermissions (schema enum), kept as sent.
	PermissionMode string `json:"permission_mode,omitempty"`
	// TurnID is the active turn (a Codex extension, per the schema).
	TurnID string `json:"turn_id,omitempty"`
	// Prompt is the submitted prompt (UserPromptSubmit).
	Prompt string `json:"prompt,omitempty"`
	// LastAssistantMessage is the finished answer (Stop, SubagentStop);
	// the schema allows null ("").
	LastAssistantMessage string `json:"last_assistant_message,omitempty"`
	StopHookActive       bool   `json:"stop_hook_active,omitempty"`
	// Source is startup, resume, clear, compact or fork (SessionStart).
	Source string `json:"source,omitempty"`
	// Reason is why a session ended (SessionEnd; the schema allows only
	// "other").
	Reason    string `json:"reason,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
	// Unknown holds every key the schemas above do not name.
	Unknown agentio.Unknown `json:"-"`
	// Raw is the whole payload as received.
	Raw agentio.Raw `json:"-"`
}

// hookEventKeys are the event-name keys tried in order: the schema's
// hook_event_name, then the candidates read before the schema was known.
func hookEventKeys() []string { return []string{"hook_event_name", "event", "type"} }

// hookInputKeys are the keys HookInput types (schema property names).
func hookInputKeys() []string {
	return []string{"session_id", "transcript_path", "cwd", "hook_event_name", "model", "permission_mode", "turn_id",
		"prompt", "last_assistant_message", "stop_hook_active", "source", "reason", "agent_id", "agent_type"}
}

// hookInputWire takes the nullable strings as pointers.
type hookInputWire struct {
	SessionID            string  `json:"session_id"`
	TranscriptPath       *string `json:"transcript_path"`
	Cwd                  string  `json:"cwd"`
	HookEventName        string  `json:"hook_event_name"`
	Model                string  `json:"model"`
	PermissionMode       string  `json:"permission_mode"`
	TurnID               string  `json:"turn_id"`
	Prompt               string  `json:"prompt"`
	LastAssistantMessage *string `json:"last_assistant_message"`
	StopHookActive       bool    `json:"stop_hook_active"`
	Source               string  `json:"source"`
	Reason               string  `json:"reason"`
	AgentID              string  `json:"agent_id"`
	AgentType            string  `json:"agent_type"`
}

// DecodeHookInput reads a hook payload (unverified shape, see above). A
// field of an unexpected type fails the decode; Raw is still the input.
// Pure (reads r).
func DecodeHookInput(r io.Reader) (HookInput, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxHookPayload))
	if err != nil {
		return HookInput{}, fmt.Errorf("codex: read hook input: %w", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		return HookInput{}, fmt.Errorf("codex: decode hook input: %w", err)
	}
	in := HookInput{Raw: agentio.Raw(b)}
	var w hookInputWire
	if err := json.Unmarshal(b, &w); err != nil {
		return in, fmt.Errorf("codex: decode hook input: %w", err)
	}
	in.SessionID, in.Cwd, in.HookEventName, in.Model = w.SessionID, w.Cwd, w.HookEventName, w.Model
	in.PermissionMode, in.TurnID, in.Prompt, in.StopHookActive = w.PermissionMode, w.TurnID, w.Prompt, w.StopHookActive
	in.Source, in.Reason, in.AgentID, in.AgentType = w.Source, w.Reason, w.AgentID, w.AgentType
	if w.TranscriptPath != nil {
		in.TranscriptPath = *w.TranscriptPath
	}
	if w.LastAssistantMessage != nil {
		in.LastAssistantMessage = *w.LastAssistantMessage
	}
	in.Unknown, _ = agentio.ExtractUnknown(b, hookInputKeys())
	for _, k := range hookEventKeys() {
		var s string
		if v, ok := obj[k]; ok && json.Unmarshal(v, &s) == nil && s != "" {
			in.Event = HookEvent(s)
			break
		}
	}
	return in, nil
}

// NotifyPayload is the JSON `notify` appends as its last argv element.
type NotifyPayload struct {
	Type string
	Raw  agentio.Raw
}

// DecodeNotify decodes a notify argument. Pure.
func DecodeNotify(arg string) (NotifyPayload, error) {
	var w struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(arg), &w); err != nil {
		return NotifyPayload{}, fmt.Errorf("codex: decode notify payload: %w", err)
	}
	return NotifyPayload{Type: w.Type, Raw: agentio.Raw(arg)}, nil
}
