package gemini

import (
	"encoding/json"
	"io"
	"slices"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/internal/hookjson"
)

// Gemini CLI runs command hooks from the "hooks" key of settings.json.
var _ agentio.HookInstaller = neutral{}

// hookKindReason is why a neutral kind without a Gemini event is refused.
const hookKindReason = "Gemini CLI has no hook event for this kind"

// Manual lines of a Gemini hook plan, from the 0.68.0 settings reference
// (hooksConfig.enabled, default true; hooksConfig.disabled lists hook
// names, a nameless command hook is named by its command) and the hook
// registry, which loads settings hooks only when the folder is trusted
// (security.folderTrust.enabled, default true).
const (
	manualHooksOff    = "settings.json sets hooksConfig.enabled: false, so Gemini CLI runs no hooks until it is true"
	manualHookOff     = "settings.json lists this command in hooksConfig.disabled, so Gemini CLI skips it: "
	manualFolderTrust = "Gemini CLI runs settings hooks only in folders it trusts while folder trust is on (security.folderTrust.enabled): trust the folders you work in"
)

// hookSettings are the settings keys a hook plan reads; the rest of the
// file is not decoded here.
type hookSettings struct {
	HooksConfig *struct {
		Enabled  *bool    `json:"enabled"`
		Disabled []string `json:"disabled"`
	} `json:"hooksConfig"`
	Security *struct {
		FolderTrust *struct {
			Enabled *bool `json:"enabled"`
		} `json:"folderTrust"`
	} `json:"security"`
}

// hookManual lists the steps the plan's hooks need before they run. A file
// that does not decode yields none (Split has already accepted it). Pure.
func hookManual(data []byte, entries []agentio.HookEntry) []string {
	var s hookSettings
	if len(data) > 0 && json.Unmarshal(StripJSONComments(data), &s) != nil {
		return nil
	}
	var out []string
	if c := s.HooksConfig; c != nil {
		if c.Enabled != nil && !*c.Enabled {
			out = append(out, manualHooksOff)
		}
		for _, e := range entries {
			if slices.Contains(c.Disabled, e.Command) && !slices.Contains(out, manualHookOff+e.Command) {
				out = append(out, manualHookOff+e.Command)
			}
		}
	}
	if sec := s.Security; sec == nil || sec.FolderTrust == nil || sec.FolderTrust.Enabled == nil || *sec.FolderTrust.Enabled {
		out = append(out, manualFolderTrust)
	}
	return out
}

// HookKinds lists the neutral kinds Gemini CLI runs a command at. Pure.
func (n neutral) HookKinds() []agentio.HookKind {
	return []agentio.HookKind{agentio.HookPromptSubmit, agentio.HookTurnEnd, agentio.HookSessionStart, agentio.HookSessionEnd}
}

// PlanHooks reads h's user settings.json and reports which of want it
// holds and which ApplyHooks would append. cost: Disk · Read.
func (n neutral) PlanHooks(h agentio.Home, want []agentio.HookEntry) (agentio.HookPlan, error) {
	gh, err := n.home(h)
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
	path := gh.UserSettingsPath()
	data, err := readSettingsBytes(path)
	if err != nil {
		return agentio.HookPlan{}, err
	}
	present, missing, err := hookjson.Split(data, entries, hookFileOptions())
	if err != nil {
		return agentio.HookPlan{}, err
	}
	existing, err := hookjson.Existing(data, hookFileOptions(), func(ev string) agentio.HookKind { return HookKindOf(HookEvent(ev)) })
	if err != nil {
		return agentio.HookPlan{}, err
	}
	return agentio.HookPlan{Agent: ID, Home: h, File: path, Add: missing, Present: present, Existing: existing, Manual: hookManual(data, entries)}, nil
}

// ApplyHooks appends p.Add to the plan's settings.json after a backup,
// skipping entries the file gained since the plan. cost: Disk · Write.
func (n neutral) ApplyHooks(p agentio.HookPlan) (agentio.Backup, error) {
	gh, err := n.home(p.Home)
	if err != nil {
		return agentio.Backup{}, err
	}
	if err := hookjson.CheckPlan(p, ID, gh.UserSettingsPath()); err != nil {
		return agentio.Backup{}, err
	}
	if p.IsNoop() {
		return agentio.Backup{}, nil
	}
	_, b, err := n.a.Writer().applyHookEntries(p.File, p.Add)
	return b, err
}

// RemoveHooks removes the command hooks of h's user settings.json whose
// command match reports true (Writer.RemoveHooks). cost: Disk · Write.
func (n neutral) RemoveHooks(h agentio.Home, match func(command string) bool) (int, agentio.Backup, error) {
	gh, err := n.home(h)
	if err != nil {
		return 0, agentio.Backup{}, err
	}
	return n.a.Writer().RemoveHooks(gh, match)
}

// DecodeHook decodes a Gemini CLI hook payload (DecodeHookInput;
// unverified shape) into the neutral call. cost: Pure (reads r).
func (n neutral) DecodeHook(r io.Reader) (agentio.HookCall, error) {
	in, err := DecodeHookInput(r)
	if err != nil {
		return agentio.HookCall{}, err
	}
	return in.Call(), nil
}
