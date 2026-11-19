package codex

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/internal/hookjson"
)

// The one Codex-owned file the Writer edits is <home>/hooks.json: Codex
// reads it but never writes it (hook trust goes to config.toml, which
// stays read-only here). Edits splice into the file's bytes
// (internal/hookjson), so other tools' hooks keep their exact bytes.

// File modes for a hooks.json the Writer creates; homes are private.
const (
	hooksFilePerm os.FileMode = 0o600
	hooksDirPerm  os.FileMode = 0o700
	tmpSuffix                 = ".agentio-tmp"
)

// hookFileOptions: Codex reads hooks.json with serde_json (no comments).
func hookFileOptions() hookjson.Options { return hookjson.Options{} }

// readHooksBytes reads hooks.json; a missing file is empty. Disk · Read.
func readHooksBytes(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("codex: read %s: %w", fileHooks, err)
	}
	return b, nil
}

// applyHookEntries appends one command group per entry the file at path
// does not hold yet (re-read now, so an entry that appeared since the
// plan is not added twice); nothing missing writes nothing (wrote=false).
// cost: Disk · Write.
func (w *Writer) applyHookEntries(path string, entries []agentio.HookEntry) (wrote bool, b agentio.Backup, err error) {
	data, err := readHooksBytes(path)
	if err != nil {
		return false, b, err
	}
	_, missing, err := hookjson.Split(data, entries, hookFileOptions())
	if err != nil {
		return false, b, fmt.Errorf("codex: %s: not rewriting it: %w", path, err)
	}
	if len(missing) == 0 {
		return false, b, nil
	}
	out, err := hookjson.AppendEntries(data, missing, hookFileOptions())
	if err != nil {
		return false, b, fmt.Errorf("codex: %s: not rewriting it: %w", path, err)
	}
	b, err = w.writeHooksFile(path, out)
	return err == nil, b, err
}

// RemoveHooks removes every command hook of <home>/hooks.json whose
// command match reports true; a matcher group left empty is removed, then
// an event left empty, then a "hooks" key left empty. Every other byte is
// kept; zero removed writes nothing. cost: Disk · Write.
func (w *Writer) RemoveHooks(h Home, match func(command string) bool) (int, agentio.Backup, error) {
	path := w.a.Paths(h).Hooks
	data, err := readHooksBytes(path)
	if err != nil || data == nil {
		return 0, agentio.Backup{}, err
	}
	out, n, err := hookjson.RemoveHandlers(data, hookjson.DropCommands(match), hookFileOptions())
	if err != nil {
		return 0, agentio.Backup{}, fmt.Errorf("codex: %s: not rewriting it: %w", path, err)
	}
	if n == 0 {
		return 0, agentio.Backup{}, nil
	}
	b, err := w.writeHooksFile(path, out)
	return n, b, err
}

// EnsureHook appends a command hook for ev to <home>/hooks.json unless one
// with the same command is already there (then nothing is written:
// added=false, empty Backup). cost: Disk · Write.
func (w *Writer) EnsureHook(h Home, ev HookEvent, command string) (added bool, b agentio.Backup, err error) {
	if command == "" {
		return false, b, fmt.Errorf("%w: codex %s: empty command", agentio.ErrInvalidHook, ev)
	}
	return w.applyHookEntries(w.a.Paths(h).Hooks, []agentio.HookEntry{{Kind: HookKindOf(ev), Event: string(ev), Command: command}})
}

// writeHooksFile backs path up, then writes data atomically (temp +
// rename), keeping the file's mode (hooksFilePerm for a new file).
func (w *Writer) writeHooksFile(path string, data []byte) (agentio.Backup, error) {
	b, err := agentio.BackupFile(path, w.a.now())
	if err != nil {
		return b, err
	}
	if err := os.MkdirAll(filepath.Dir(path), hooksDirPerm); err != nil {
		return b, err
	}
	perm := hooksFilePerm
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
