package gemini

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/internal/hookjson"
)

// File modes for a settings.json the Writer creates; homes are private.
const (
	settingsPerm os.FileMode = 0o600
	homeDirPerm  os.FileMode = 0o700
	tmpSuffix                = ".agentio-tmp"
)

// Writer is the Disk · Write door. Its only target is the "hooks" key of
// a home's user settings.json (RemoveHooks, EnsureHook and the neutral
// HookInstaller). Every method backs the file up first and returns the
// agentio.Backup. Edits splice into the file's bytes (internal/hookjson),
// so comments, other keys and other tools' hooks keep their exact bytes.
type Writer struct{ a *Agent }

// Writer returns the write door. cost: Pure.
func (a *Agent) Writer() *Writer { return &Writer{a: a} }

// now is Host.Now, the only clock; a Host without one reads as the zero
// time (backup names still stay unique through their -N suffix).
func (a *Agent) now() time.Time {
	if a.host.Now == nil {
		return time.Time{}
	}
	return a.host.Now()
}

// hookFileOptions: Gemini CLI reads settings.json through
// strip-json-comments, so comments are allowed and kept.
func hookFileOptions() hookjson.Options { return hookjson.Options{Comments: true} }

// readSettingsBytes reads a settings file; a missing file is empty.
// cost: Disk · Read.
func readSettingsBytes(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gemini: read %s: %w", path, err)
	}
	return b, nil
}

// applyHookEntries appends one command group per entry the file at path
// does not hold yet (re-read now, so an entry that appeared since the
// plan is not added twice); nothing missing writes nothing (wrote=false).
// cost: Disk · Write.
func (w *Writer) applyHookEntries(path string, entries []agentio.HookEntry) (wrote bool, b agentio.Backup, err error) {
	data, err := readSettingsBytes(path)
	if err != nil {
		return false, b, err
	}
	_, missing, err := hookjson.Split(data, entries, hookFileOptions())
	if err != nil {
		return false, b, fmt.Errorf("gemini: %s: not rewriting it: %w", path, err)
	}
	if len(missing) == 0 {
		return false, b, nil
	}
	out, err := hookjson.AppendEntries(data, missing, hookFileOptions())
	if err != nil {
		return false, b, fmt.Errorf("gemini: %s: not rewriting it: %w", path, err)
	}
	b, err = w.writeSettingsFile(path, out)
	return err == nil, b, err
}

// EnsureHook appends a command hook for ev to h's user settings.json
// unless one with the same command is already there (then nothing is
// written: added=false, empty Backup). cost: Disk · Write.
func (w *Writer) EnsureHook(h Home, ev HookEvent, command string) (added bool, b agentio.Backup, err error) {
	if command == "" {
		return false, b, fmt.Errorf("%w: gemini %s: empty command", agentio.ErrInvalidHook, ev)
	}
	return w.applyHookEntries(h.UserSettingsPath(), []agentio.HookEntry{{Kind: HookKindOf(ev), Event: string(ev), Command: command}})
}

// RemoveHooks removes every command hook of h's user settings.json whose
// command match reports true; a matcher group left empty is removed, then
// an event left empty, then a "hooks" key left empty. Every other byte is
// kept; zero removed writes nothing. cost: Disk · Write.
func (w *Writer) RemoveHooks(h Home, match func(command string) bool) (int, agentio.Backup, error) {
	path := h.UserSettingsPath()
	data, err := readSettingsBytes(path)
	if err != nil || data == nil {
		return 0, agentio.Backup{}, err
	}
	out, n, err := hookjson.RemoveHandlers(data, hookjson.DropCommands(match), hookFileOptions())
	if err != nil {
		return 0, agentio.Backup{}, fmt.Errorf("gemini: %s: not rewriting it: %w", path, err)
	}
	if n == 0 {
		return 0, agentio.Backup{}, nil
	}
	b, err := w.writeSettingsFile(path, out)
	return n, b, err
}

// writeSettingsFile backs path up, then writes data atomically (temp +
// rename), keeping the file's mode (settingsPerm for a new file).
func (w *Writer) writeSettingsFile(path string, data []byte) (agentio.Backup, error) {
	b, err := agentio.BackupFile(path, w.a.now())
	if err != nil {
		return b, err
	}
	if err := os.MkdirAll(filepath.Dir(path), homeDirPerm); err != nil {
		return b, err
	}
	perm := settingsPerm
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	tmp := path + tmpSuffix
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return b, err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // the rename error is the one reported
		return b, err
	}
	return b, nil
}
