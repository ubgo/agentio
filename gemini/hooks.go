package gemini

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/ubgo/agentio"
)

// Hooks: the configuration shape (settings.json "hooks", the same
// event -> matcher groups -> command handlers shape as Claude Code; handler
// keys type, command, name, timeout in ms, description) and the stdin
// payloads come from the docs and source shipped in the @google/gemini-cli
// 0.68.0 package (docs/hooks/reference.md, HookEventName,
// createBaseInput, fire*Event). The payloads are UNVERIFIED: no Gemini CLI
// hook run has been recorded here; every other key is kept in Unknown and
// the whole payload in Raw.

// HookEvent names a Gemini CLI hook event. Unknown names are kept;
// IsKnown reports them.
type HookEvent string

// The documented hook events (HookEventName at 0.68.0).
const (
	HookBeforeTool          HookEvent = "BeforeTool"
	HookAfterTool           HookEvent = "AfterTool"
	HookBeforeAgent         HookEvent = "BeforeAgent"
	HookNotification        HookEvent = "Notification"
	HookAfterAgent          HookEvent = "AfterAgent"
	HookSessionStart        HookEvent = "SessionStart"
	HookSessionEnd          HookEvent = "SessionEnd"
	HookPreCompress         HookEvent = "PreCompress"
	HookBeforeModel         HookEvent = "BeforeModel"
	HookAfterModel          HookEvent = "AfterModel"
	HookBeforeToolSelection HookEvent = "BeforeToolSelection"
)

// HookEvents lists every documented event. cost: Pure.
func HookEvents() []HookEvent {
	return []HookEvent{HookBeforeTool, HookAfterTool, HookBeforeAgent, HookNotification, HookAfterAgent, HookSessionStart,
		HookSessionEnd, HookPreCompress, HookBeforeModel, HookAfterModel, HookBeforeToolSelection}
}

// IsKnown reports a documented event. cost: Pure.
func (e HookEvent) IsKnown() bool { return slices.Contains(HookEvents(), e) }

// maxHookPayload caps a hook payload read (an AfterModel payload carries
// a whole model request).
const maxHookPayload = 16 << 20

// HookInput is the JSON a Gemini CLI hook command receives on stdin
// (unverified: typed from the docs and source, never from a recorded run).
type HookInput struct {
	// Base fields every event sends.
	SessionID      string    `json:"session_id"`
	TranscriptPath string    `json:"transcript_path"`
	Cwd            string    `json:"cwd"`
	HookEventName  HookEvent `json:"hook_event_name"`
	// Timestamp is the ISO 8601 time of the run; zero when absent or not
	// RFC 3339 (the text stays in Raw).
	Timestamp time.Time `json:"-"`
	// Prompt is the user's prompt (BeforeAgent, AfterAgent).
	Prompt string `json:"prompt,omitempty"`
	// PromptResponse is the final answer of the turn (AfterAgent).
	PromptResponse string `json:"prompt_response,omitempty"`
	// StopHookActive: AfterAgent runs inside a retry it caused.
	StopHookActive bool `json:"stop_hook_active,omitempty"`
	// Source is startup, resume or clear (SessionStart).
	Source string `json:"source,omitempty"`
	// Reason is exit, clear, logout, prompt_input_exit or other (SessionEnd).
	Reason string `json:"reason,omitempty"`
	// Unknown holds every other key (tool, model and notification fields).
	Unknown agentio.Unknown `json:"-"`
	// Raw is the whole payload as received.
	Raw agentio.Raw `json:"-"`
}

// hookInputKeys are the keys HookInput types.
func hookInputKeys() []string {
	return []string{"session_id", "transcript_path", "cwd", "hook_event_name", "timestamp", "prompt", "prompt_response",
		"stop_hook_active", "source", "reason"}
}

// DecodeHookInput decodes a hook's stdin (unverified shape, see above).
// cost: Pure (reads r).
func DecodeHookInput(r io.Reader) (HookInput, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxHookPayload))
	if err != nil {
		return HookInput{}, fmt.Errorf("gemini: read hook input: %w", err)
	}
	var w struct {
		HookInput
		Timestamp string `json:"timestamp"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return HookInput{}, fmt.Errorf("gemini: decode hook input: %w", err)
	}
	in := w.HookInput
	if ts, err := time.Parse(time.RFC3339Nano, w.Timestamp); err == nil {
		in.Timestamp = ts
	}
	in.Unknown, _ = agentio.ExtractUnknown(b, hookInputKeys())
	in.Raw = agentio.Raw(b)
	return in, nil
}

// hookKindEvents maps each neutral kind to its Gemini CLI event. Pure.
func hookKindEvents() map[agentio.HookKind]HookEvent {
	return map[agentio.HookKind]HookEvent{
		agentio.HookPromptSubmit: HookBeforeAgent,
		agentio.HookTurnEnd:      HookAfterAgent,
		agentio.HookSessionStart: HookSessionStart,
		agentio.HookSessionEnd:   HookSessionEnd,
	}
}

// HookEventFor is the Gemini CLI event of a neutral kind. Pure.
func HookEventFor(k agentio.HookKind) (HookEvent, bool) {
	ev, ok := hookKindEvents()[k]
	return ev, ok
}

// HookKindOf is the neutral kind of a Gemini CLI event;
// agentio.HookUnknown for an event with none. Pure.
func HookKindOf(ev HookEvent) agentio.HookKind {
	for k, e := range hookKindEvents() {
		if e == ev {
			return k
		}
	}
	return agentio.HookUnknown
}

// Call maps the input onto the neutral HookCall; PromptResponse is the
// turn's last assistant message. Pure.
func (in HookInput) Call() agentio.HookCall {
	return agentio.HookCall{
		Agent:                ID,
		Kind:                 HookKindOf(in.HookEventName),
		Event:                string(in.HookEventName),
		ConversationID:       in.SessionID,
		Dir:                  in.Cwd,
		TranscriptPath:       in.TranscriptPath,
		Prompt:               in.Prompt,
		LastAssistantMessage: in.PromptResponse,
		Raw:                  in.Raw,
	}
}
