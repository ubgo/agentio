package claude

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/internal/hookjson"
)

// Claude Code runs command hooks from the user settings.json of a home.
var _ agentio.HookInstaller = neutral{}

// hookKindReason is why a neutral kind without a Claude event is refused.
const hookKindReason = "Claude Code has no hook event for this kind"

// manualDisableAllHooks is the Manual line for a home whose user settings
// turn every hook off.
const manualDisableAllHooks = "settings.json sets disableAllHooks: true, so Claude Code runs no hooks until it is removed"

// hookKindEvents maps each neutral kind to its Claude event. Pure.
func hookKindEvents() map[agentio.HookKind]HookEvent {
	return map[agentio.HookKind]HookEvent{
		agentio.HookPromptSubmit: HookUserPromptSubmit,
		agentio.HookTurnEnd:      HookStop,
		agentio.HookSessionStart: HookSessionStart,
		agentio.HookSessionEnd:   HookSessionEnd,
	}
}

// HookEventFor is the Claude event of a neutral kind. Pure.
func HookEventFor(k agentio.HookKind) (HookEvent, bool) {
	ev, ok := hookKindEvents()[k]
	return ev, ok
}

// HookKindOf is the neutral kind of a Claude event; agentio.HookUnknown
// for an event with none. Pure.
func HookKindOf(ev HookEvent) agentio.HookKind {
	for k, e := range hookKindEvents() {
		if e == ev {
			return k
		}
	}
	return agentio.HookUnknown
}

// hookFile is the settings file the neutral installer edits. Pure.
func hookFile(h Home) string { return filepath.Join(h.Dir(), settingsFile) }

// HookKinds lists the neutral kinds Claude Code runs a command at. Pure.
func (n neutral) HookKinds() []agentio.HookKind {
	return []agentio.HookKind{agentio.HookPromptSubmit, agentio.HookTurnEnd, agentio.HookSessionStart, agentio.HookSessionEnd}
}

// PlanHooks reads h's user settings.json and reports which of want it
// already holds and which ApplyHooks would append. cost: Disk · Read.
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
	path := hookFile(ch)
	data, _, err := readFileBounded(path, maxJSONFileBytes)
	if err != nil {
		return agentio.HookPlan{}, err
	}
	present, missing, err := hookjson.Split(data, entries, hookFileOptions())
	if err != nil {
		return agentio.HookPlan{}, fmt.Errorf("claude: %s: %w", path, err)
	}
	existing, err := hookjson.Existing(data, hookFileOptions(), func(ev string) agentio.HookKind { return HookKindOf(HookEvent(ev)) })
	if err != nil {
		return agentio.HookPlan{}, fmt.Errorf("claude: %s: %w", path, err)
	}
	p := agentio.HookPlan{Agent: ID, Home: h, File: path, Add: missing, Present: present, Existing: existing}
	if l, err := n.a.UserSettings(ch); err == nil && l.Settings.DisableAllHooks != nil && *l.Settings.DisableAllHooks {
		p.Manual = append(p.Manual, manualDisableAllHooks)
	}
	return p, nil
}

// ApplyHooks appends p.Add to the plan's settings.json after a backup,
// skipping entries the file gained since the plan. cost: Disk · Write.
func (n neutral) ApplyHooks(p agentio.HookPlan) (agentio.Backup, error) {
	ch, err := n.home(p.Home)
	if err != nil {
		return agentio.Backup{}, err
	}
	if err := hookjson.CheckPlan(p, ID, hookFile(ch)); err != nil {
		return agentio.Backup{}, err
	}
	if p.IsNoop() {
		return agentio.Backup{}, nil
	}
	return n.a.Writer().applyHookEntries(p.File, p.Add)
}

// RemoveHooks removes the command hooks of h's user settings.json whose
// command match reports true. cost: Disk · Write.
func (n neutral) RemoveHooks(h agentio.Home, match func(command string) bool) (int, agentio.Backup, error) {
	ch, err := n.home(h)
	if err != nil {
		return 0, agentio.Backup{}, err
	}
	return n.a.Writer().removeHookHandlers(hookFile(ch), hookjson.DropCommands(match))
}

// DecodeHook decodes a Claude Code hook payload (DecodeHookInput) into
// the neutral call. cost: Pure (reads r).
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
		Kind:                 HookKindOf(in.HookEventName),
		Event:                string(in.HookEventName),
		ConversationID:       in.SessionID,
		Dir:                  in.Cwd,
		TranscriptPath:       in.TranscriptPath,
		Prompt:               in.Prompt,
		LastAssistantMessage: in.LastAssistantMessage,
		PermissionMode:       string(in.PermissionMode),
		Model:                in.Model,
		Raw:                  in.Raw,
	}
}
