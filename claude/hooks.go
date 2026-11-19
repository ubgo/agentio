package claude

import (
	"encoding/json"
	"io"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// HookEvent names a hook event (docs `hooks`, 33 events). Unknown names are
// kept; IsKnown reports them.
type HookEvent string

// The documented hook events.
const (
	HookSessionStart        HookEvent = "SessionStart"
	HookSetup               HookEvent = "Setup"
	HookUserPromptSubmit    HookEvent = "UserPromptSubmit"
	HookUserPromptExpansion HookEvent = "UserPromptExpansion"
	HookPreToolUse          HookEvent = "PreToolUse"
	HookPermissionRequest   HookEvent = "PermissionRequest"
	HookPermissionDenied    HookEvent = "PermissionDenied"
	HookPostToolUse         HookEvent = "PostToolUse"
	HookPostToolUseFailure  HookEvent = "PostToolUseFailure"
	HookPostToolBatch       HookEvent = "PostToolBatch"
	HookNotification        HookEvent = "Notification"
	HookMessageDisplay      HookEvent = "MessageDisplay"
	HookSubagentStart       HookEvent = "SubagentStart"
	HookSubagentStop        HookEvent = "SubagentStop"
	HookTaskCreated         HookEvent = "TaskCreated"
	HookTaskCompleted       HookEvent = "TaskCompleted"
	HookStop                HookEvent = "Stop"
	HookStopFailure         HookEvent = "StopFailure"
	HookTeammateIdle        HookEvent = "TeammateIdle"
	HookInstructionsLoaded  HookEvent = "InstructionsLoaded"
	HookConfigChange        HookEvent = "ConfigChange"
	HookCwdChanged          HookEvent = "CwdChanged"
	HookDirectoryAdded      HookEvent = "DirectoryAdded"
	HookFileChanged         HookEvent = "FileChanged"
	HookWorktreeCreate      HookEvent = "WorktreeCreate"
	HookWorktreeRemove      HookEvent = "WorktreeRemove"
	HookPreCompact          HookEvent = "PreCompact"
	HookPostCompact         HookEvent = "PostCompact"
	HookPreModelSwitch      HookEvent = "PreModelSwitch"
	HookPostModelSwitch     HookEvent = "PostModelSwitch"
	HookElicitation         HookEvent = "Elicitation"
	HookElicitationResult   HookEvent = "ElicitationResult"
	HookSessionEnd          HookEvent = "SessionEnd"
)

// HookEvents lists every documented event. cost: Pure.
func HookEvents() []HookEvent {
	return []HookEvent{
		HookSessionStart, HookSetup, HookUserPromptSubmit, HookUserPromptExpansion, HookPreToolUse, HookPermissionRequest,
		HookPermissionDenied, HookPostToolUse, HookPostToolUseFailure, HookPostToolBatch, HookNotification, HookMessageDisplay,
		HookSubagentStart, HookSubagentStop, HookTaskCreated, HookTaskCompleted, HookStop, HookStopFailure, HookTeammateIdle,
		HookInstructionsLoaded, HookConfigChange, HookCwdChanged, HookDirectoryAdded, HookFileChanged, HookWorktreeCreate,
		HookWorktreeRemove, HookPreCompact, HookPostCompact, HookPreModelSwitch, HookPostModelSwitch, HookElicitation,
		HookElicitationResult, HookSessionEnd,
	}
}

// IsKnown reports a documented event. cost: Pure.
func (e HookEvent) IsKnown() bool { return slices.Contains(HookEvents(), e) }

// SupportsMatcher reports whether the event filters on a matcher (tool
// events and the few with a subject per docs). cost: Pure.
func (e HookEvent) SupportsMatcher() bool {
	switch e {
	case HookPreToolUse, HookPostToolUse, HookPostToolUseFailure, HookPermissionRequest, HookPermissionDenied,
		HookNotification, HookSessionStart, HookSessionEnd, HookPreCompact, HookPostCompact, HookSubagentStart, HookSubagentStop,
		HookElicitation, HookElicitationResult, HookConfigChange, HookFileChanged, HookInstructionsLoaded:
		return true
	}
	return false
}

// Hooks is the `hooks` object of one settings layer.
type Hooks map[HookEvent][]HookMatcher

// HookMatcher is one matcher group.
type HookMatcher struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []HookHandler `json:"hooks"`
}

// matchAll are the matcher spellings that match every subject (docs).
const (
	matchAllStar = "*"
	matcherSep   = "|"
)

// Matches applies the docs rules: "" or "*" match all; a plain name list
// "A|B" matches exactly; anything else is a regex. cost: Pure.
func (m HookMatcher) Matches(subject string) bool {
	if m.Matcher == "" || m.Matcher == matchAllStar {
		return true
	}
	if isPlainMatcher(m.Matcher) {
		return slices.Contains(strings.Split(m.Matcher, matcherSep), subject)
	}
	re, err := regexp.Compile("^(?:" + m.Matcher + ")$")
	return err == nil && re.MatchString(subject)
}

// isPlainMatcher reports a matcher made only of letters, digits, '_' and
// '|', which docs treat as an exact-name list rather than a regex.
func isPlainMatcher(s string) bool {
	for i := range len(s) {
		c := s[i]
		if c != '|' && c != '_' && !isASCIIAlnum(c) {
			return false
		}
	}
	return true
}

// HookType is a handler type.
type HookType string

// Documented handler types.
const (
	HookTypeCommand HookType = "command"
	HookTypeHTTP    HookType = "http"
	HookTypeMCPTool HookType = "mcp_tool"
	HookTypePrompt  HookType = "prompt"
	HookTypeAgent   HookType = "agent"
)

// IsKnown reports a documented handler type. cost: Pure.
func (t HookType) IsKnown() bool {
	switch t {
	case HookTypeCommand, HookTypeHTTP, HookTypeMCPTool, HookTypePrompt, HookTypeAgent:
		return true
	}
	return false
}

// HookHandler is one handler. Timeout is seconds on disk.
type HookHandler struct {
	Type                     HookType
	If                       string
	Timeout                  time.Duration
	StatusMessage            string
	Once, Async, AsyncRewake *bool
	OnFailure                string
	Command                  string
	Args                     []string
	Shell                    string
	URL                      string
	Headers                  map[string]string
	AllowedEnvVars           []string
	Server, Tool             string
	Input                    agentio.Raw
	Prompt, Model            string
	Unknown                  agentio.Unknown
}

type wireHookHandler struct {
	Type           HookType          `json:"type"`
	If             string            `json:"if,omitempty"`
	Timeout        *float64          `json:"timeout,omitempty"`
	StatusMessage  string            `json:"statusMessage,omitempty"`
	Once           *bool             `json:"once,omitempty"`
	Async          *bool             `json:"async,omitempty"`
	AsyncRewake    *bool             `json:"asyncRewake,omitempty"`
	OnFailure      string            `json:"onFailure,omitempty"`
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	Shell          string            `json:"shell,omitempty"`
	URL            string            `json:"url,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	AllowedEnvVars []string          `json:"allowedEnvVars,omitempty"`
	Server         string            `json:"server,omitempty"`
	Tool           string            `json:"tool,omitempty"`
	Input          json.RawMessage   `json:"input,omitempty"`
	Prompt         string            `json:"prompt,omitempty"`
	Model          string            `json:"model,omitempty"`
}

// UnmarshalJSON decodes a handler, keeping unmodelled keys. cost: Pure.
func (h *HookHandler) UnmarshalJSON(b []byte) error {
	var w wireHookHandler
	unknown, _, err := decodeLenient("hooks", b, reflect.TypeFor[wireHookHandler](), func(d []byte) error { return json.Unmarshal(d, &w) })
	if err != nil {
		return err
	}
	*h = HookHandler{
		Type: w.Type, If: w.If, StatusMessage: w.StatusMessage, Once: w.Once, Async: w.Async, AsyncRewake: w.AsyncRewake,
		OnFailure: w.OnFailure, Command: w.Command, Args: w.Args, Shell: w.Shell, URL: w.URL, Headers: w.Headers,
		AllowedEnvVars: w.AllowedEnvVars, Server: w.Server, Tool: w.Tool, Input: rawOrNil(w.Input), Prompt: w.Prompt, Model: w.Model,
		Unknown: unknown,
	}
	if w.Timeout != nil {
		h.Timeout = time.Duration(*w.Timeout * float64(time.Second))
	}
	return nil
}

// MarshalJSON writes the on-disk shape back, unknown keys included, so a
// Writer round trip never drops a key. cost: Pure.
func (h HookHandler) MarshalJSON() ([]byte, error) {
	w := wireHookHandler{
		Type: h.Type, If: h.If, StatusMessage: h.StatusMessage, Once: h.Once, Async: h.Async, AsyncRewake: h.AsyncRewake,
		OnFailure: h.OnFailure, Command: h.Command, Args: h.Args, Shell: h.Shell, URL: h.URL, Headers: h.Headers,
		AllowedEnvVars: h.AllowedEnvVars, Server: h.Server, Tool: h.Tool, Input: json.RawMessage(h.Input), Prompt: h.Prompt, Model: h.Model,
	}
	if h.Timeout > 0 {
		s := h.Timeout.Seconds()
		w.Timeout = &s
	}
	return marshalWithUnknown(w, h.Unknown)
}

// marshalWithUnknown encodes a wire struct and merges the kept unknown keys
// back in (known keys win).
func marshalWithUnknown(w json.Marshaler, unknown agentio.Unknown) ([]byte, error) {
	b, err := w.MarshalJSON()
	if err != nil || len(unknown) == 0 {
		return b, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for k, v := range unknown {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return json.Marshal(m)
}

// MarshalJSON lets wireHookHandler satisfy json.Marshaler for marshalWithUnknown.
func (w wireHookHandler) MarshalJSON() ([]byte, error) {
	type plain wireHookHandler
	return json.Marshal(plain(w))
}

// ScopedHook is one handler with where it came from.
type ScopedHook struct {
	Event   HookEvent
	Matcher string
	Handler HookHandler
	Scope   SettingsScope
	Source  Source
}

// BackgroundTask is one entry of a hook input's background_tasks (ailog
// hook.log sample, the only recorded shape).
type BackgroundTask struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Description string `json:"description"`
	Command     string `json:"command"`
}

// HookEffort is the `effort` object of a hook input.
type HookEffort struct {
	Level Effort `json:"level"`
}

// HookInput is the JSON a hook program receives on stdin. Common fields plus
// the per-event ones recorded in ailog's hook log; everything else is
// kept in Unknown.
type HookInput struct {
	SessionID            string           `json:"session_id"`
	PromptID             string           `json:"prompt_id,omitempty"`
	TranscriptPath       string           `json:"transcript_path"`
	Cwd                  string           `json:"cwd"`
	ScratchpadDir        string           `json:"scratchpad_dir,omitempty"`
	PermissionMode       PermissionMode   `json:"permission_mode,omitempty"`
	Effort               *HookEffort      `json:"effort,omitempty"`
	HookEventName        HookEvent        `json:"hook_event_name"`
	AgentID              string           `json:"agent_id,omitempty"`
	AgentType            string           `json:"agent_type,omitempty"`
	Prompt               string           `json:"prompt,omitempty"`
	LastAssistantMessage string           `json:"last_assistant_message,omitempty"`
	StopHookActive       bool             `json:"stop_hook_active,omitempty"`
	BackgroundTasks      []BackgroundTask `json:"background_tasks,omitempty"`
	SessionCrons         []agentio.Raw    `json:"session_crons,omitempty"`
	Source               string           `json:"source,omitempty"`
	Reason               string           `json:"reason,omitempty"`
	ToolName             string           `json:"tool_name,omitempty"`
	ToolInput            agentio.Raw      `json:"tool_input,omitempty"`
	ToolResponse         agentio.Raw      `json:"tool_response,omitempty"`
	ToolUseID            string           `json:"tool_use_id,omitempty"`
	Model                string           `json:"model,omitempty"`
	FromModel            string           `json:"from_model,omitempty"`
	ToModel              string           `json:"to_model,omitempty"`
	Unknown              agentio.Unknown  `json:"-"`
	// Raw is the whole payload as received.
	Raw agentio.Raw `json:"-"`
}

// DecodeHookInput decodes a hook's stdin. cost: Pure.
func DecodeHookInput(r io.Reader) (HookInput, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxLineBytes))
	if err != nil {
		return HookInput{}, err
	}
	var in HookInput
	unknown, _, err := decodeLenient("hook input", b, reflect.TypeFor[HookInput](), func(d []byte) error { return json.Unmarshal(d, &in) })
	if err != nil {
		return HookInput{}, err
	}
	in.Unknown, in.Raw = unknown, agentio.Raw(b)
	return in, nil
}

// HookOutput is what a hook may print on stdout.
type HookOutput struct {
	Continue           *bool               `json:"continue,omitempty"`
	StopReason         string              `json:"stopReason,omitempty"`
	SuppressOutput     *bool               `json:"suppressOutput,omitempty"`
	SystemMessage      string              `json:"systemMessage,omitempty"`
	TerminalSequence   string              `json:"terminalSequence,omitempty"`
	Decision           string              `json:"decision,omitempty"`
	Reason             string              `json:"reason,omitempty"`
	HookSpecificOutput *HookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

// HookSpecificOutput is the per-event part of a hook's output.
type HookSpecificOutput struct {
	HookEventName            HookEvent `json:"hookEventName"`
	PermissionDecision       string    `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string    `json:"permissionDecisionReason,omitempty"`
	AdditionalContext        string    `json:"additionalContext,omitempty"`
	Retry                    *bool     `json:"retry,omitempty"`
}

// Encode writes the output as one JSON document. cost: Pure.
func (o HookOutput) Encode(w io.Writer) error { return json.NewEncoder(w).Encode(o) }

// Hooks lists every hook handler in effect for h (and p when set) with its
// layer: managed, local, project, user. cost: Disk · Read.
func (a *Agent) Hooks(h Home, p *Project) ([]ScopedHook, error) {
	eff, err := a.Settings(h, p, nil)
	if err != nil {
		return nil, err
	}
	var out []ScopedHook
	for _, l := range eff.Layers {
		for _, ev := range sortedEvents(l.Settings.Hooks) {
			for _, m := range l.Settings.Hooks[ev] {
				for _, hh := range m.Hooks {
					out = append(out, ScopedHook{Event: ev, Matcher: m.Matcher, Handler: hh, Scope: l.Scope, Source: l.Source})
				}
			}
		}
	}
	return out, nil
}

func sortedEvents(h Hooks) []HookEvent {
	out := make([]HookEvent, 0, len(h))
	for e := range h {
		out = append(out, e)
	}
	slices.Sort(out)
	return out
}
