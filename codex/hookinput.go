package codex

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/ubgo/agentio"
)

// Hook and notify payloads have NO recorded sample yet, and the library
// never invents a vendor shape: only the event / type name and the raw
// bytes are kept until one Codex hook run and one notify call are recorded.

// hook payload limits.
const maxHookPayload = 16 << 20

// HookInput is a hook's stdin payload.
type HookInput struct {
	Event HookEvent
	Raw   agentio.Raw
}

// hookEventKeys are the candidate event-name keys; which one Codex uses is
// unrecorded, so the Claude-compatible name is tried first.
func hookEventKeys() []string { return []string{"hook_event_name", "event", "type"} }

// DecodeHookInput reads a hook payload. Pure (reads r).
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
