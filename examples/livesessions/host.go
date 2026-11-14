package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ubgo/agentio"
)

// Fixture locations; the example copies them into a temporary user home
// (Claude's user home plus Codex as ~/.codex) because the example adds a
// Codex writer lock.
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

// Recorded fixture sessions: Claude's default home has sessions/4242.json
// (busy) and 4243.json (idle); the work home has 4342 and 4343. The start
// times are the startedAt values in those files, so the pid-reuse guard
// accepts the rows below.
const (
	claudeBusyPID   = 4242
	claudeStalePID  = 4243
	claudeWorkPID   = 4342
	claudeMCPPID    = 5001
	desktopPID      = 5002
	codexExecPID    = 6001
	codexLauncherID = 6000
	editorPID       = 7001
	codexThreadID   = "0199e1a0-0000-7000-8000-000000000001"
	threadLocksDir  = "thread-writer-locks"
	lockSuffix      = ".lock"
)

// Recorded start times (ms since epoch) of the fixture sessions.
const (
	claudeBusyStartMS = 1794597025793
	claudeWorkStartMS = 1794544889729
)

// lockTime is when the Codex writer lock was taken in this example.
func lockTime() time.Time { return time.Date(2026, 11, 13, 11, 30, 0, 0, time.UTC) }

// processTable is the fixed process table the example host reports: two
// live Claude sessions, a Claude MCP server, the Claude desktop app (not
// the CLI), a Codex exec run, its npm launcher and an editor.
func processTable() []agentio.Process {
	return []agentio.Process{
		{PID: claudeBusyPID, Exe: "/opt/claude/bin/claude", Argv: []string{"claude"}, Start: time.UnixMilli(claudeBusyStartMS)},
		{PID: claudeWorkPID, Exe: "/opt/claude/bin/claude", Argv: []string{"claude", "--resume", "78d3d87c"}, Start: time.UnixMilli(claudeWorkStartMS)},
		{PID: claudeMCPPID, PPID: claudeBusyPID, Exe: "/opt/claude/bin/claude", Argv: []string{"claude", "mcp", "serve"}},
		{PID: desktopPID, Exe: "/Applications/Claude.app/Contents/MacOS/Claude", Argv: []string{"/Applications/Claude.app/Contents/MacOS/Claude"}},
		{PID: codexLauncherID, Exe: "/usr/bin/node", Argv: []string{"node", "/usr/lib/node_modules/@openai/codex/bin/codex.js", "exec", "hi"}, Start: lockTime().Add(-time.Minute)},
		{PID: codexExecPID, PPID: codexLauncherID, Exe: "/opt/codex/codex", Argv: []string{"codex", "exec", "hi"}, Start: lockTime().Add(-time.Second)},
		{PID: editorPID, Exe: "/usr/bin/vim", Argv: []string{"vim", "notes.md"}},
	}
}

// setup copies both fixtures into tmp, takes a Codex writer lock for one
// recorded thread (as a running Codex would) and returns the Host with the
// fixed process table. Disk · Write.
func setup(tmp string) (agentio.Host, error) {
	root, err := repoRoot()
	if err != nil {
		return agentio.Host{}, err
	}
	if err := copyTree(filepath.Join(root, claudeFixture), tmp); err != nil {
		return agentio.Host{}, err
	}
	codexHome := filepath.Join(tmp, codexHomeDir)
	if err := copyTree(filepath.Join(root, codexFixture), codexHome); err != nil {
		return agentio.Host{}, err
	}
	lock := filepath.Join(codexHome, threadLocksDir, codexThreadID+lockSuffix)
	if err := os.WriteFile(lock, nil, filePerm); err != nil {
		return agentio.Host{}, err
	}
	if err := os.Chtimes(lock, lockTime(), lockTime()); err != nil {
		return agentio.Host{}, err
	}
	return agentio.Host{
		UserHome:  tmp,
		User:      fixtureUser,
		Env:       []string{"HOME=" + tmp, "PATH=" + noBinaries},
		Now:       fixtureNow,
		Processes: fixedProcesses{list: processTable()},
	}, nil
}

// fixedProcesses is a fixed process table.
type fixedProcesses struct{ list []agentio.Process }

// Processes returns a copy of the table. Pure.
func (f fixedProcesses) Processes(context.Context) ([]agentio.Process, error) {
	return append([]agentio.Process(nil), f.list...), nil
}

// Cwd is unknown for every row in this example. Pure.
func (fixedProcesses) Cwd(context.Context, int) (string, error) {
	return "", fmt.Errorf("example: cwd not recorded")
}
