package claude

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/internal/hookjson"
)

// Hook edits splice into the settings file's bytes (internal/hookjson)
// instead of re-marshalling it: settings.json is shared by every tool that
// installs a hook or a status line, and a rewrite through a map would
// reorder its keys and reformat it on every install.

// hookFileOptions: Claude Code reads settings.json as plain JSON.
func hookFileOptions() hookjson.Options { return hookjson.Options{} }

// appendMatcher splices m into data as the last group of ev. Pure.
func appendMatcher(data []byte, ev HookEvent, m HookMatcher) ([]byte, error) {
	group, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return hookjson.AppendGroup(data, string(ev), group, hookFileOptions())
}

// EnsureHook is AddHook made idempotent: handlers of m that ev already has
// in the scope's settings file (a command handler with the same command,
// or any other handler with the same JSON) are left out, and when none is
// left nothing is written (added=false, empty Backup). A second run with
// the same arguments is therefore a no-op. cost: Disk · Write.
func (w *Writer) EnsureHook(h Home, p *Project, s SettingsScope, ev HookEvent, m HookMatcher) (added bool, b agentio.Backup, err error) {
	path, err := w.a.settingsPath(h, p, s)
	if err != nil {
		return false, b, err
	}
	data, _, err := readFileBounded(path, maxJSONFileBytes)
	if err != nil {
		return false, b, err
	}
	have, err := hookjson.Handlers(data, hookFileOptions())
	if err != nil {
		return false, b, fmt.Errorf("claude: %s: not rewriting it: %w", path, err)
	}
	var missing []HookHandler
	for _, hh := range m.Hooks {
		if !hasHandler(have, ev, hh) {
			missing = append(missing, hh)
		}
	}
	if len(missing) == 0 {
		return false, b, nil
	}
	m.Hooks = missing
	out, err := appendMatcher(data, ev, m)
	if err != nil {
		return false, b, fmt.Errorf("claude: %s: not rewriting it: %w", path, err)
	}
	b, err = w.writeSettingsBytes(path, out)
	return err == nil, b, err
}

// hasHandler reports whether have holds hh under ev: a command handler
// matches on its command alone (another tool's timeout does not make it a
// different hook), any other handler on its whole JSON. Pure.
func hasHandler(have []hookjson.Handler, ev HookEvent, hh HookHandler) bool {
	want, err := json.Marshal(hh)
	if err != nil {
		return false
	}
	for _, h := range have {
		if h.Event != string(ev) {
			continue
		}
		if hh.Type == HookTypeCommand {
			if h.IsCommand && h.Command == hh.Command {
				return true
			}
			continue
		}
		var got HookHandler
		if json.Unmarshal(h.Raw, &got) != nil {
			continue
		}
		if b, err := json.Marshal(got); err == nil && bytes.Equal(b, want) {
			return true
		}
	}
	return false
}

// removeHookHandlers removes the handlers drop selects from the settings
// file at path; zero removed writes nothing. cost: Disk · Write.
func (w *Writer) removeHookHandlers(path string, drop func(hookjson.Handler) bool) (int, agentio.Backup, error) {
	data, found, err := readFileBounded(path, maxJSONFileBytes)
	if err != nil || !found {
		return 0, agentio.Backup{}, err
	}
	out, n, err := hookjson.RemoveHandlers(data, drop, hookFileOptions())
	if err != nil {
		return 0, agentio.Backup{}, fmt.Errorf("claude: %s: not rewriting it: %w", path, err)
	}
	if n == 0 {
		return 0, agentio.Backup{}, nil
	}
	b, err := w.writeSettingsBytes(path, out)
	return n, b, err
}

// applyHookEntries appends one command group per entry the file at path
// does not hold yet (re-read now, so an entry that appeared since the
// plan is not added twice); nothing missing writes nothing.
// cost: Disk · Write.
func (w *Writer) applyHookEntries(path string, entries []agentio.HookEntry) (agentio.Backup, error) {
	data, _, err := readFileBounded(path, maxJSONFileBytes)
	if err != nil {
		return agentio.Backup{}, err
	}
	_, missing, err := hookjson.Split(data, entries, hookFileOptions())
	if err != nil {
		return agentio.Backup{}, fmt.Errorf("claude: %s: not rewriting it: %w", path, err)
	}
	if len(missing) == 0 {
		return agentio.Backup{}, nil
	}
	out, err := hookjson.AppendEntries(data, missing, hookFileOptions())
	if err != nil {
		return agentio.Backup{}, fmt.Errorf("claude: %s: not rewriting it: %w", path, err)
	}
	return w.writeSettingsBytes(path, out)
}
