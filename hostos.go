package agentio

// hostos.go is the ONLY file that touches the real machine: os.Getenv,
// os.Environ, os.UserHomeDir, exec.Command and time.Now appear here and
// nowhere else in the module (archtest 5). Everything else reaches the
// machine through the Host this file builds.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// OS names this file branches on.
const (
	goosDarwin  = "darwin"
	goosLinux   = "linux"
	goosWindows = "windows"
)

// HostFromOS builds the real Host: $HOME, os.Environ(), the real clock, a
// default HTTP client with a timeout, the macOS keychain (not-found
// elsewhere), the process table (/proc on linux, `ps` on darwin), an
// exec-based runner, runtime.GOOS and the OS system root. It is the one
// place that fills GOOS and SystemRoot, so only a real host reads /etc.
// Disk · Read (resolves the home dir only).
func HostFromOS() (Host, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Host{}, fmt.Errorf("agentio: user home: %w", err)
	}
	return Host{
		UserHome:   home,
		User:       osAccount(),
		Env:        os.Environ(),
		Now:        time.Now,
		HTTP:       &http.Client{Timeout: osHTTPTimeout},
		Secrets:    osSecrets{},
		Processes:  osProcesses{},
		Exec:       osRunner{},
		GOOS:       runtime.GOOS,
		SystemRoot: osSystemRoot(),
	}, nil
}

// OS system roots: unix resolves system paths under "/"; Windows under the
// system drive (%SystemDrive%, "C:" when unset) plus a separator.
const (
	unixSystemRoot     = "/"
	envSystemDrive     = "SystemDrive"
	defaultSystemDrive = "C:"
	windowsPathSep     = `\`
)

// osSystemRoot is the root system and managed locations live under.
func osSystemRoot() string {
	if runtime.GOOS != goosWindows {
		return unixSystemRoot
	}
	drive := os.Getenv(envSystemDrive)
	if drive == "" {
		drive = defaultSystemDrive
	}
	return drive + windowsPathSep
}

// osHTTPTimeout bounds one vendor HTTP call; usage endpoints answer in
// well under a second, a hung one must not hang the caller.
const osHTTPTimeout = 30 * time.Second

// osRunner runs commands with os/exec.
type osRunner struct{}

// Run runs c with its own Env and Dir (never the parent's). Process · Read.
func (osRunner) Run(ctx context.Context, c Command, stdin io.Reader) (ProcessResult, error) {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Env = c.Env
	cmd.Dir = c.Dir
	cmd.Stdin = stdin
	cmd.WaitDelay = osWaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	res := ProcessResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && ctx.Err() == nil {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	if err != nil {
		return res, fmt.Errorf("agentio: run %s: %w", filepath.Base(c.Path), err)
	}
	return res, nil
}

// osWaitDelay bounds the wait for output pipes after a killed process: a
// child that inherited stdout would otherwise keep Run blocked.
const osWaitDelay = 200 * time.Millisecond

// osSecrets reads the macOS keychain through `security`.
type osSecrets struct{}

// Keychain lookup constants.
const (
	securityBin = "/usr/bin/security"
	// securityNotFoundExit is `security find-generic-password`'s exit for
	// "The specified item could not be found in the keychain."
	securityNotFoundExit = 44
	// keychainTimeout bounds one lookup: macOS may show an access prompt and
	// an unanswered one must not hang the run.
	keychainTimeout = 10 * time.Second
)

// Get runs `security find-generic-password -s <service> [-a <account>] -w`
// on darwin. Elsewhere it returns ErrUnsupported naming the OS: there is no
// Secret Service (linux) or Credential Manager (windows) reader yet, and a
// caller must be able to tell "no entry" from "cannot look" (a Codex
// keyring login on linux is not a logout). Callers that only use the
// keychain as a second store (Claude) treat ErrUnsupported as no entry.
// Process · Read.
func (osSecrets) Get(ctx context.Context, service, account string) (Token, bool, error) {
	if runtime.GOOS != goosDarwin {
		return Token{}, false, UnsupportedError("", "keychain", "no keychain reader for "+runtime.GOOS)
	}
	ctx, cancel := context.WithTimeout(ctx, keychainTimeout)
	defer cancel()
	args := []string{"find-generic-password", "-s", service}
	if account != "" {
		args = append(args, "-a", account)
	}
	args = append(args, "-w")
	cmd := exec.CommandContext(ctx, securityBin, args...)
	cmd.WaitDelay = osWaitDelay
	out, err := cmd.Output()
	if err == nil {
		return NewToken(string(bytes.TrimSpace(out))), true, nil
	}
	var exitErr *exec.ExitError
	switch {
	case errors.Is(err, exec.ErrNotFound), errors.Is(err, os.ErrNotExist):
		return Token{}, false, nil
	case errors.As(err, &exitErr) && exitErr.ExitCode() == securityNotFoundExit:
		return Token{}, false, nil
	case ctx.Err() != nil:
		return Token{}, false, fmt.Errorf("agentio: keychain lookup timed out after %s (unanswered prompt?): %w", keychainTimeout, ctx.Err())
	}
	// The error text of `security` never holds the secret (it is on stdout).
	return Token{}, false, fmt.Errorf("agentio: keychain lookup %s: %w", service, err)
}

// osAccount returns the OS user name (Claude Code's keychain account):
// os/user first, then $USER.
func osAccount() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv(envUser)
}

// envUser is the fallback source of the user name.
const envUser = "USER"

// osProcesses reads the process table.
type osProcesses struct{}

// Processes is Process · Read: /proc on linux, one `ps` run on darwin.
func (osProcesses) Processes(ctx context.Context) ([]Process, error) {
	switch runtime.GOOS {
	case goosLinux:
		return linuxProcesses()
	case goosDarwin:
		return darwinProcesses(ctx)
	}
	return nil, UnsupportedError("", "process table", "no reader for "+runtime.GOOS)
}

// Cwd is Process · Read: /proc/{pid}/cwd on linux, `lsof` on darwin.
func (osProcesses) Cwd(ctx context.Context, pid int) (string, error) {
	switch runtime.GOOS {
	case goosLinux:
		return os.Readlink(filepath.Join(procRoot, strconv.Itoa(pid), "cwd"))
	case goosDarwin:
		return darwinCwd(ctx, pid)
	}
	return "", UnsupportedError("", "process cwd", "no reader for "+runtime.GOOS)
}

// Linux /proc layout.
const (
	procRoot = "/proc"
	// procStatPPIDField and procStatStartField are 0-based indexes into
	// the fields after the ")" that ends comm in /proc/{pid}/stat
	// (proc(5): field 4 ppid, field 22 starttime).
	procStatPPIDField  = 1
	procStatStartField = 19
	// clockTicks is USER_HZ, 100 on every linux ABI Go supports.
	clockTicks = 100
	procBtime  = "btime "
)

// linuxProcesses reads every /proc/{pid}. Rows that vanish mid-read or
// are unreadable are skipped: the table changes while it is read.
func linuxProcesses() ([]Process, error) {
	boot, err := linuxBootTime()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, fmt.Errorf("agentio: read %s: %w", procRoot, err)
	}
	var out []Process
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		dir := filepath.Join(procRoot, e.Name())
		stat, err := os.ReadFile(filepath.Join(dir, "stat"))
		if err != nil {
			continue
		}
		p := Process{PID: pid}
		if i := bytes.LastIndexByte(stat, ')'); i >= 0 {
			f := strings.Fields(string(stat[i+1:]))
			if len(f) > procStatStartField {
				p.PPID, _ = strconv.Atoi(f[procStatPPIDField])
				if ticks, err := strconv.ParseInt(f[procStatStartField], 10, 64); err == nil {
					p.Start = boot.Add(time.Duration(ticks) * time.Second / clockTicks)
				}
			}
		}
		if cmdline, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			for _, a := range bytes.Split(bytes.TrimRight(cmdline, "\x00"), []byte{0}) {
				if len(a) > 0 {
					p.Argv = append(p.Argv, string(a))
				}
			}
		}
		if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
			p.Exe = exe
		} else if len(p.Argv) > 0 {
			p.Exe = p.Argv[0]
		}
		out = append(out, p)
	}
	return out, nil
}

// linuxBootTime reads btime from /proc/stat.
func linuxBootTime() (time.Time, error) {
	f, err := os.Open(filepath.Join(procRoot, "stat"))
	if err != nil {
		return time.Time{}, fmt.Errorf("agentio: boot time: %w", err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), procBtime); ok {
			secs, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return time.Time{}, fmt.Errorf("agentio: boot time: %w", err)
			}
			return time.Unix(secs, 0), nil
		}
	}
	return time.Time{}, fmt.Errorf("agentio: boot time: no btime in %s/stat", procRoot)
}

// darwin `ps` constants. lstart is printed as five fields in the C locale
// ("Fri Nov 13 19:38:43 2026"), so the environment pins LC_ALL=C.
const (
	psBin          = "/bin/ps"
	psLstartLayout = "Mon Jan _2 15:04:05 2006"
	psLstartFields = 5
	lsofBin        = "/usr/sbin/lsof"
	lsofNamePrefix = "n"
	envLocaleC     = "LC_ALL=C"
)

// darwinProcesses runs one `ps -axww -o pid=,ppid=,lstart=,command=`.
func darwinProcesses(ctx context.Context) ([]Process, error) {
	cmd := exec.CommandContext(ctx, psBin, "-axww", "-o", "pid=,ppid=,lstart=,command=")
	cmd.Env = []string{envLocaleC}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("agentio: ps: %w", err)
	}
	var procs []Process
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2+psLstartFields+1 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		p := Process{PID: pid, PPID: ppid, Argv: f[2+psLstartFields:]}
		if t, err := time.ParseInLocation(psLstartLayout, strings.Join(f[2:2+psLstartFields], " "), time.Local); err == nil {
			p.Start = t
		}
		p.Exe = p.Argv[0]
		procs = append(procs, p)
	}
	return procs, nil
}

// darwinCwd asks lsof for the cwd of pid (`-Fn` prints "n<path>").
func darwinCwd(ctx context.Context, pid int) (string, error) {
	cmd := exec.CommandContext(ctx, lsofBin, "-a", "-d", "cwd", "-p", strconv.Itoa(pid), "-Fn")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("agentio: lsof cwd %d: %w", pid, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, lsofNamePrefix); ok && p != "" {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: cwd of %d", ErrNotFound, pid)
}
