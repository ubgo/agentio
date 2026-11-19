package codex

import (
	"io"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/internal/hookjson"
)

// Codex runs command hooks from <home>/hooks.json (and the [hooks] table of
// config.toml, which agentio keeps read-only).
var _ agentio.HookInstaller = neutral{}

// hookKindReason is why a neutral kind without a Codex event is refused.
const hookKindReason = "Codex has no hook event for this kind"

// Manual lines of a Codex hook plan. Sources: the Codex 0.162.1 binary
// (`codex features list` reports hooks as stable and on by default; the
// TUI says "Hooks need review" and offers "Review hooks" / "Trust all and
// continue" for a new or changed hook, recording trusted_hash in
// config.toml hooks.state).
const (
	featureHooks       = "hooks"
	manualReviewHooks  = "Codex asks you to review new or changed hooks before it runs them: start codex on this home and trust them (Review hooks)"
	manualFeatureHooks = "config.toml sets [features] hooks = false; set it to true for Codex to run hooks (agentio keeps config.toml read-only)"
)

// hookKindEvents maps each neutral kind to its Codex event (all four have
// a command-hook input schema in the 0.162.1 binary). Pure.
func hookKindEvents() map[agentio.HookKind]HookEvent {
	return map[agentio.HookKind]HookEvent{
		agentio.HookPromptSubmit: HookUserPromptSubmit,
		agentio.HookTurnEnd:      HookStop,
		agentio.HookSessionStart: HookSessionStart,
		agentio.HookSessionEnd:   HookSessionEnd,
	}
}

// HookEventFor is the Codex event of a neutral kind. Pure.
func HookEventFor(k agentio.HookKind) (HookEvent, bool) {
	ev, ok := hookKindEvents()[k]
	return ev, ok
}

// HookKindOf is the neutral kind of a Codex event; agentio.HookUnknown for
// an event with none. Pure.
func HookKindOf(ev HookEvent) agentio.HookKind {
	for k, e := range hookKindEvents() {
		if e == ev {
			return k
		}
	}
	return agentio.HookUnknown
}

// HookKinds lists the neutral kinds Codex runs a command at. Pure.
func (n neutral) HookKinds() []agentio.HookKind {
	return []agentio.HookKind{agentio.HookPromptSubmit, agentio.HookTurnEnd, agentio.HookSessionStart, agentio.HookSessionEnd}
}

// PlanHooks reads h's hooks.json (and config.toml for the hooks feature)
// and reports which of want it holds and which ApplyHooks would append.
// cost: Disk · Read.
func (n neutral) PlanHooks(h agentio.Home, want []agentio.HookEntry) (agentio.HookPlan, error) {
	ch, err := n.home(h)
	if err != nil {
		return agentio.HookPlan{}, err
	}
	entries, err := hookjson.Resolve(ID, want, func(k agentio.HookKind) (string, bool) {
		ev, ok := HookEventFor(k)
		return string(ev), ok
	}, hookKindReason)
	if err != nil {
		return agentio.HookPlan{}, err
	}
	path := n.a.Paths(ch).Hooks
	data, err := readHooksBytes(path)
	if err != nil {
		return agentio.HookPlan{}, err
	}
	present, missing, err := hookjson.Split(data, entries, hookFileOptions())
	if err != nil {
		return agentio.HookPlan{}, err
	}
	p := agentio.HookPlan{Agent: ID, Home: h, File: path, Add: missing, Present: present}
	if c, err := n.a.ConfigFile(ch); err == nil {
		if f, ok := c.Features[featureHooks]; ok && !f.Enabled {
			p.Manual = append(p.Manual, manualFeatureHooks)
		}
	}
	if len(missing) > 0 {
		p.Manual = append(p.Manual, manualReviewHooks)
	}
	return p, nil
}

// ApplyHooks appends p.Add to the plan's hooks.json after a backup,
// skipping entries the file gained since the plan. cost: Disk · Write.
func (n neutral) ApplyHooks(p agentio.HookPlan) (agentio.Backup, error) {
	ch, err := n.home(p.Home)
	if err != nil {
		return agentio.Backup{}, err
	}
	if err := hookjson.CheckPlan(p, ID, n.a.Paths(ch).Hooks); err != nil {
		return agentio.Backup{}, err
	}
	if p.IsNoop() {
		return agentio.Backup{}, nil
	}
	_, b, err := n.a.Writer().applyHookEntries(p.File, p.Add)
	return b, err
}

// RemoveHooks removes the command hooks of h's hooks.json whose command
// match reports true (Writer.RemoveHooks). cost: Disk · Write.
func (n neutral) RemoveHooks(h agentio.Home, match func(command string) bool) (int, agentio.Backup, error) {
	ch, err := n.home(h)
	if err != nil {
		return 0, agentio.Backup{}, err
	}
	return n.a.Writer().RemoveHooks(ch, match)
}

// DecodeHook decodes a Codex hook payload (DecodeHookInput; unverified
// shape) into the neutral call. cost: Pure (reads r).
func (n neutral) DecodeHook(r io.Reader) (agentio.HookCall, error) {
	in, err := DecodeHookInput(r)
	if err != nil {
		return agentio.HookCall{}, err
	}
	return in.Call(), nil
}

// Call maps the input onto the neutral HookCall. Pure.
func (in HookInput) Call() agentio.HookCall {
	return agentio.HookCall{
		Agent:                ID,
		Kind:                 HookKindOf(in.Event),
		Event:                string(in.Event),
		ConversationID:       in.SessionID,
		Dir:                  in.Cwd,
		TranscriptPath:       in.TranscriptPath,
		Prompt:               in.Prompt,
		LastAssistantMessage: in.LastAssistantMessage,
		PermissionMode:       in.PermissionMode,
		Model:                in.Model,
		Raw:                  in.Raw,
	}
}
