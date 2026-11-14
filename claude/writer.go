package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ubgo/agentio"
)

// ErrHistoryExists means the destination already has a history folder;
// MoveHistory refuses rather than merging two histories.
var ErrHistoryExists = errors.New("claude: destination already has history")

// File modes for files and dirs the Writer creates; homes are private.
const (
	settingsPerm os.FileMode = 0o600
	dirPerm      os.FileMode = 0o700
	tmpSuffix                = ".agentio-tmp"
	jsonIndent               = "  "
)

// Writer is the Disk · Write door. Every method backs its target up first
// (<file>.agentio-bak-<UTC stamp>, never overwriting a backup) and returns
// the agentio.Backup (invariant 11). It never edits ~/.claude.json or a
// project .mcp.json: Claude Code rewrites those itself, so a library edit
// could race it; those changes are vendor commands as data.
type Writer struct{ a *Agent }

// Writer returns the write door. cost: Pure.
func (a *Agent) Writer() *Writer { return &Writer{a: a} }

// MoveHistory re-keys the history Claude Code recorded for from so it is
// found from to, by renaming <home>/projects/<encode(from)>. Claude looks
// conversations up by the encoded folder, never by the cwd inside old
// lines, so a rename is enough. A folder without history is
// HistoryMove{Moved: false} with a nil error; an existing destination is
// ErrHistoryExists; a live Claude session in from (or of one of its
// conversations) is *LiveSessionError, the blocker PlanMove reports.
// cost: Disk · Write (plus Process · Read through Host.Processes for the
// live check).
func (w *Writer) MoveHistory(h Home, from, to string) (agentio.HistoryMove, error) {
	src, dst := w.a.HistoryDir(h, from), w.a.HistoryDir(h, to)
	entries, err := os.ReadDir(src)
	if isNotExist(err) {
		return agentio.HistoryMove{}, nil
	} else if err != nil {
		return agentio.HistoryMove{}, fmt.Errorf("claude: read history: %w", err)
	}
	if pids := w.a.livePIDs(filepath.Clean(from), transcriptIDs(entries), h); len(pids) > 0 {
		return agentio.HistoryMove{}, &LiveSessionError{Dir: filepath.Clean(from), PIDs: pids}
	}
	if fileExists(dst) {
		return agentio.HistoryMove{}, fmt.Errorf("%w: %s", ErrHistoryExists, dst)
	}
	b, err := agentio.BackupFile(src, w.a.now())
	if err != nil {
		return agentio.HistoryMove{Backup: b}, err
	}
	if err := os.Rename(src, dst); err != nil {
		return agentio.HistoryMove{Backup: b}, fmt.Errorf("claude: move history: %w", err)
	}
	return agentio.HistoryMove{Moved: true, Backup: b}, nil
}

// ApplyMove executes a plan from PlanMove: backs every item up, renames
// each into place, rolls back on failure (agentio.ApplyMovePlan).
// cost: Disk · Write.
func (w *Writer) ApplyMove(p agentio.MovePlan) (agentio.Backup, error) {
	if p.Agent != ID {
		return agentio.Backup{}, fmt.Errorf("%w: %s move plan", agentio.ErrForeignHome, p.Agent)
	}
	return agentio.ApplyMovePlan(p, w.a.now())
}

// ApplyHomeLinks makes the planned symlinks of shared home files (never
// overwrites; nothing to back up). A link planned for another agent is
// agentio.ErrForeignHome and nothing is linked, as in ApplyMove.
// cost: Disk · Write.
func (w *Writer) ApplyHomeLinks(links []agentio.HomeLink) error {
	if err := agentio.CheckLinksOwner(ID, links); err != nil {
		return err
	}
	return agentio.ApplyHomeLinks(links)
}

// readRawSettings reads a settings file as a raw top-level map; a missing
// file is an empty map.
func readRawSettings(path string) (map[string]json.RawMessage, error) {
	data, found, err := readFileBounded(path, maxJSONFileBytes)
	if err != nil {
		return nil, err
	}
	m := map[string]json.RawMessage{}
	if !found || len(data) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("claude: %s is not a JSON object; not rewriting it: %w", path, err)
	}
	return m, nil
}

// writeRawSettings backs path up, then writes m atomically (temp + rename).
func (w *Writer) writeRawSettings(path string, m map[string]json.RawMessage) (agentio.Backup, error) {
	b, err := agentio.BackupFile(path, w.a.now())
	if err != nil {
		return b, err
	}
	data, err := json.MarshalIndent(m, "", jsonIndent)
	if err != nil {
		return b, err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return b, err
	}
	perm := settingsPerm
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	tmp := path + tmpSuffix
	if err := os.WriteFile(tmp, append(data, '\n'), perm); err != nil {
		return b, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // the rename error is the one reported
		return b, err
	}
	return b, nil
}

// applyPatch edits a settings file's top-level keys.
func (w *Writer) applyPatch(path string, patch SettingsPatch) (agentio.Backup, error) {
	m, err := readRawSettings(path)
	if err != nil {
		return agentio.Backup{}, err
	}
	for _, k := range patch.Delete {
		delete(m, k)
	}
	for k, v := range patch.Set {
		if !json.Valid(v) {
			return agentio.Backup{}, fmt.Errorf("claude: settings value for %q is not valid JSON", k)
		}
		m[k] = json.RawMessage(v)
	}
	return w.writeRawSettings(path, m)
}

// SetUserSetting edits top-level keys of <home>/settings.json; other keys
// are kept byte for byte. cost: Disk · Write.
func (w *Writer) SetUserSetting(h Home, patch SettingsPatch) (agentio.Backup, error) {
	return w.applyPatch(filepath.Join(h.Dir(), settingsFile), patch)
}

// SetProjectSetting edits .claude/settings.json (local=false) or
// .claude/settings.local.json (local=true) of p. cost: Disk · Write.
func (w *Writer) SetProjectSetting(p Project, local bool, patch SettingsPatch) (agentio.Backup, error) {
	scope := ScopeProject
	if local {
		scope = ScopeLocal
	}
	path, err := w.a.settingsPath(Home{}, &p, scope)
	if err != nil {
		return agentio.Backup{}, err
	}
	return w.applyPatch(path, patch)
}

// readHooks decodes the hooks key of a raw settings map.
func readHooks(m map[string]json.RawMessage) (Hooks, error) {
	hooks := Hooks{}
	if raw, ok := m[keyHooks]; ok && len(raw) > 0 && string(raw) != jsonNull {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, fmt.Errorf("claude: hooks do not decode; not rewriting them: %w", err)
		}
	}
	return hooks, nil
}

// AddHook appends a matcher group for ev to the settings file of scope s
// (user: h; project/local: p, which must be set). Existing hooks and keys
// are kept. cost: Disk · Write.
func (w *Writer) AddHook(h Home, p *Project, s SettingsScope, ev HookEvent, m HookMatcher) (agentio.Backup, error) {
	path, err := w.a.settingsPath(h, p, s)
	if err != nil {
		return agentio.Backup{}, err
	}
	raw, err := readRawSettings(path)
	if err != nil {
		return agentio.Backup{}, err
	}
	hooks, err := readHooks(raw)
	if err != nil {
		return agentio.Backup{}, err
	}
	hooks[ev] = append(hooks[ev], m)
	enc, err := json.Marshal(hooks)
	if err != nil {
		return agentio.Backup{}, err
	}
	raw[keyHooks] = enc
	return w.writeRawSettings(path, raw)
}

// RemoveHooks drops every handler in the scope's settings file for which
// keep returns false; empty matcher groups and events are removed. Nothing
// is written (and no backup made) when nothing matches. cost: Disk · Write.
func (w *Writer) RemoveHooks(h Home, p *Project, s SettingsScope, keep func(HookEvent, HookHandler) bool) (removed int, b agentio.Backup, err error) {
	path, err := w.a.settingsPath(h, p, s)
	if err != nil {
		return 0, b, err
	}
	raw, err := readRawSettings(path)
	if err != nil {
		return 0, b, err
	}
	hooks, err := readHooks(raw)
	if err != nil {
		return 0, b, err
	}
	for ev, groups := range hooks {
		var keptGroups []HookMatcher
		for _, g := range groups {
			var kept []HookHandler
			for _, hh := range g.Hooks {
				if keep(ev, hh) {
					kept = append(kept, hh)
				} else {
					removed++
				}
			}
			if len(kept) > 0 {
				g.Hooks = kept
				keptGroups = append(keptGroups, g)
			}
		}
		if len(keptGroups) == 0 {
			delete(hooks, ev)
		} else {
			hooks[ev] = keptGroups
		}
	}
	if removed == 0 {
		return 0, b, nil
	}
	if len(hooks) == 0 {
		delete(raw, keyHooks)
	} else {
		enc, err := json.Marshal(hooks)
		if err != nil {
			return 0, b, err
		}
		raw[keyHooks] = enc
	}
	b, err = w.writeRawSettings(path, raw)
	return removed, b, err
}
