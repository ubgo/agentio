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
// (Claude's user home plus Codex as ~/.codex) and adds one background
// Claude session file.
const (
	claudeFixture = "claude/testdata/homes/2.1.325/userhome"
	codexFixture  = "codex/testdata/homes/0.162.1"
	codexHomeDir  = ".codex"
	fixtureUser   = "user"
	dirPerm       = 0o700
	filePerm      = 0o600
	noBinaries    = "/nonexistent-bin"
	mib           = 1 << 20
)

// fixtureNow is the fixed clock.
func fixtureNow() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

// Process table of the example. 4242 is the recorded busy Claude session
// (sessions/4242.json), started from a shell in a terminal; 4500 is a
// background Claude session whose file the example writes; 5001 is
// `claude mcp serve`; 6000 is the Codex npm launcher and 6001 its native
// child resuming a recorded thread; 6100 is a Codex app-server.
const (
	launchdPID        = 1
	terminalPID       = 100
	shellPID          = 200
	claudeBusyPID     = 4242
	claudeBgPID       = 4500
	claudeMCPPID      = 5001
	codexLauncherPID  = 6000
	codexChildPID     = 6001
	codexAppServerPID = 6100
	claudeBusyStartMS = 1794597025793
	claudeBgSessionID = "aaaaaaaa-0000-4000-8000-000000000045"
	codexThreadID     = "0199e1a0-0000-7000-8000-000000000001"
	sessionsDir       = "sessions"
)

// claudeBusyStart is when the busy session's process started (its
// recorded startedAt; darwin ps shows it to the second).
func claudeBusyStart() time.Time { return time.UnixMilli(claudeBusyStartMS).Truncate(time.Second) }

// backgroundSession is a sessions/<pid>.json body for a background
// session, in the shape of the fixture's recorded files (values replaced).
func backgroundSession() string {
	return fmt.Sprintf(`{"pid":%d,"sessionId":%q,"cwd":"/work/bg","startedAt":%d,"version":"2.1.325","kind":"bg","entrypoint":"cli","status":"busy","updatedAt":%d,"statusUpdatedAt":%d}`,
		claudeBgPID, claudeBgSessionID, claudeBusyStartMS, claudeBusyStartMS+60000, claudeBusyStartMS+60000)
}

// processTable is the fixed process table the example host reports, with
// the resource columns a real table read fills.
func processTable() []agentio.Process {
	start := claudeBusyStart()
	return []agentio.Process{
		{PID: launchdPID, Argv: []string{"/sbin/launchd"}, Exe: "/sbin/launchd"},
		{PID: terminalPID, PPID: launchdPID, Argv: []string{"/Applications/Terminal.app/Contents/MacOS/Terminal"}, CommandLine: "/Applications/Terminal.app/Contents/MacOS/Terminal"},
		{PID: shellPID, PPID: terminalPID, Argv: []string{"-zsh"}, TTY: "ttys004"},
		{PID: claudeBusyPID, PPID: shellPID, Start: start, Exe: "claude", Argv: []string{"claude"}, CommandLine: "claude", TTY: "ttys004", RSS: 512 * mib, CPUTime: 72*time.Minute + 6*time.Second},
		{PID: claudeBgPID, PPID: launchdPID, Start: start, Exe: "claude", Argv: []string{"claude", "--bg"}, RSS: 300 * mib},
		{PID: claudeMCPPID, PPID: claudeBusyPID, Start: start, Exe: "claude", Argv: []string{"claude", "mcp", "serve"}, RSS: 90 * mib},
		{PID: codexLauncherPID, PPID: shellPID, Start: start, Exe: "node", Argv: []string{"node", "/usr/lib/node_modules/@openai/codex/bin/codex.js", "resume", codexThreadID}, TTY: "ttys005", RSS: 60 * mib},
		{PID: codexChildPID, PPID: codexLauncherPID, Start: start, Exe: "/usr/lib/node_modules/@openai/codex/vendor/codex/codex", Argv: []string{"/usr/lib/node_modules/@openai/codex/vendor/codex/codex", "resume", codexThreadID}, TTY: "ttys005", RSS: 140 * mib, CPUTime: 3 * time.Second},
		{PID: codexAppServerPID, PPID: launchdPID, Start: start, Exe: "codex", Argv: []string{"codex", "app-server"}, RSS: 80 * mib},
	}
}

// setup copies the fixtures into tmp, writes the background session file
// and returns the Host with the fixed process table. Disk · Write.
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
	bg := filepath.Join(tmp, ".claude", sessionsDir, fmt.Sprintf("%d.json", claudeBgPID))
	if err := os.WriteFile(bg, []byte(backgroundSession()), filePerm); err != nil {
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

// fixedProcesses is a fixed process table whose processes all run in the
// default homes (their start env names no home variable).
type fixedProcesses struct{ list []agentio.Process }

// Processes returns a copy of the table. Pure.
func (f fixedProcesses) Processes(context.Context) ([]agentio.Process, error) {
	return append([]agentio.Process(nil), f.list...), nil
}

// Cwd is unknown for every row in this example. Pure.
func (fixedProcesses) Cwd(context.Context, int) (string, error) {
	return "", fmt.Errorf("example: cwd not recorded")
}

// Env is the same start environment for every process: no home
// variable, so each runs in its agent's default home. Pure.
func (fixedProcesses) Env(context.Context, int) ([]string, error) {
	return []string{"PATH=/usr/bin"}, nil
}

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
