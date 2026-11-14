package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Fixture locations; the example copies them into a temporary user home
// (Claude's user home plus Codex as ~/.codex) because the move steps
// change files.
const (
	claudeFixture = "claude/testdata/homes/2.1.325/userhome"
	codexFixture  = "codex/testdata/homes/0.162.1"
	codexHomeDir  = ".codex"
	fixtureUser   = "user"
	dirPerm       = 0o700
	filePerm      = 0o600
	noBinaries    = "/nonexistent-bin"
)

// fixtureNow is the fixed clock; it also names the backup files.
func fixtureNow() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

// repoRoot walks up from the working directory to the folder holding the
// fixtures. Disk · Read.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, claudeFixture)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("example: %s not found above the working directory", claudeFixture)
		}
		dir = parent
	}
}

// copyTree copies the files and directories of src into dst. Disk · Write.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, dirPerm)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, filePerm)
	})
}

// setup copies both fixtures into tmp and returns the Host on them, with an
// empty process table (no session is running, so no move is blocked).
// Disk · Write.
func setup(tmp string) (agentio.Host, error) {
	root, err := repoRoot()
	if err != nil {
		return agentio.Host{}, err
	}
	if err := copyTree(filepath.Join(root, claudeFixture), tmp); err != nil {
		return agentio.Host{}, err
	}
	if err := copyTree(filepath.Join(root, codexFixture), filepath.Join(tmp, codexHomeDir)); err != nil {
		return agentio.Host{}, err
	}
	if err := pinTranscriptTimes(tmp); err != nil {
		return agentio.Host{}, err
	}
	return agentio.Host{
		UserHome:  tmp,
		User:      fixtureUser,
		Env:       []string{"HOME=" + tmp, "PATH=" + noBinaries},
		Now:       fixtureNow,
		Processes: noProcesses{},
	}, nil
}

// transcriptTimes pins each copied transcript's mtime (paths relative to
// the temp user home). Conversations are listed newest first by mtime, and
// a copy's mtime is the copy time: two files copied within one coarse
// kernel clock tick (a few ms, easily hit under -race) got the same mtime,
// the newest-first order became a tie, and readTurns sometimes read
// /work/proj12 instead of /work/proj13.
func transcriptTimes() map[string]time.Time {
	at := fixtureNow()
	return map[string]time.Time{
		".claude/projects/-work-proj13/3f474440-903e-5410-8bfb-d537b89f9547.jsonl":                                   at,
		".claude/projects/-work-proj12/f85efbdb-fc2f-1e8e-7748-9f915edc8e2d.jsonl":                                   at.Add(-time.Hour),
		codexHomeDir + "/sessions/2026/10/14/rollout-2026-10-14T10-00-00-0199e1a0-0000-7000-8000-000000000001.jsonl": at,
		codexHomeDir + "/sessions/2026/03/01/rollout-2026-03-01T08-00-00-0199e1a0-0000-7000-8000-000000000002.jsonl": at.Add(-time.Hour),
	}
}

// pinTranscriptTimes applies transcriptTimes under userHome. Disk · Write.
func pinTranscriptTimes(userHome string) error {
	for rel, at := range transcriptTimes() {
		if err := os.Chtimes(filepath.Join(userHome, filepath.FromSlash(rel)), at, at); err != nil {
			return err
		}
	}
	return nil
}

// tidy replaces the temporary user home with ~ in printed paths. Pure.
func tidy(h agentio.Host, s string) string { return strings.ReplaceAll(s, h.UserHome, "~") }

// noProcesses is an empty process table.
type noProcesses struct{}

// Processes returns no rows. Pure.
func (noProcesses) Processes(context.Context) ([]agentio.Process, error) { return nil, nil }

// Cwd knows no process. Pure.
func (noProcesses) Cwd(context.Context, int) (string, error) {
	return "", fmt.Errorf("example: no process table")
}
