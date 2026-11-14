package agentio

// hostos.go is the ONLY file that touches the real machine: os.Getenv,
// os.Environ, os.UserHomeDir, exec.Command and time.Now appear here and
// nowhere else in the module (archtest 5). Everything else reaches the
// machine through the Host this file builds.

import (
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

// Run runs c with its own Env and Dir (never the parent's). A bare
// Command.Path ("codex", as every builder emits) is resolved on the PATH of
// c.Env under this OS's rules (PATHEXT on Windows), never the parent
// process's PATH: exec.Command would search os.Getenv("PATH"), so a Host
// whose Env names another PATH (a temp npm prefix, a profile) would run a
// different binary or none. Not found is ErrNotInstalled. Process · Read.
func (osRunner) Run(ctx context.Context, c Command, stdin io.Reader) (ProcessResult, error) {
	path, err := resolveCommandPath(c)
	if err != nil {
		return ProcessResult{}, err
	}
	cmd := exec.CommandContext(ctx, path, c.Args...)
	cmd.Env = c.Env
	cmd.Dir = c.Dir
	cmd.Stdin = stdin
	cmd.WaitDelay = osWaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
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

// Get reads one secret from the OS store. Process · Read:
//   - darwin: `security find-generic-password -s <service> [-a <account>] -w`;
//   - linux: `secret-tool lookup service <service> [username <account>]`,
//     the Secret Service attributes the Rust keyring crate (Codex's store)
//     writes; ErrUnsupported when secret-tool (libsecret-tools) is not
//     installed or no Secret Service answers (headless, no D-Bus session);
//   - windows and others: ErrUnsupported with the reason (reading a Windows
//     Credential Manager secret needs CredReadW, which the stdlib does not
//     wrap).
//
// A caller must be able to tell "no entry" (found false, nil error) from
// "cannot look" (ErrUnsupported): a Codex keyring login on such a host is
// not a logout. Callers that only use the store as a second source
// (Claude) treat ErrUnsupported as no entry. The secret never appears in
// an error.
func (osSecrets) Get(ctx context.Context, service, account string) (Token, bool, error) {
	switch runtime.GOOS {
	case goosDarwin:
	case goosLinux:
		return secretToolGet(ctx, service, account)
	case goosWindows:
		return Token{}, false, UnsupportedError("", "keyring", reasonWindowsKeyring)
	default:
		return Token{}, false, UnsupportedError("", "keyring", "no keyring reader for "+runtime.GOOS)
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
// os/user first, then $USER (unix) or %USERNAME% (windows).
func osAccount() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	if v := os.Getenv(envUser); v != "" {
		return v
	}
	return os.Getenv(envUserWindows)
}

// envUser and envUserWindows are the fallback sources of the user name.
const (
	envUser        = "USER"
	envUserWindows = "USERNAME"
)

// Linux Secret Service through secret-tool.
const (
	secretToolBin = "secret-tool"
	// secretToolNotFoundExit is secret-tool's exit when lookup finds no
	// item (it prints nothing); a D-Bus failure exits the same but writes
	// the reason to stderr.
	secretToolNotFoundExit = 1
	// reasonWindowsKeyring is why the windows keyring is unsupported.
	reasonWindowsKeyring = "reading a Windows Credential Manager secret needs CredReadW, which the Go standard library does not wrap"
)

// secretToolGet runs one secret-tool lookup.
func secretToolGet(ctx context.Context, service, account string) (Token, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, keychainTimeout)
	defer cancel()
	args := []string{"lookup", "service", service}
	if account != "" {
		args = append(args, "username", account)
	}
	cmd := exec.CommandContext(ctx, secretToolBin, args...)
	cmd.WaitDelay = osWaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	var exitErr *exec.ExitError
	switch {
	case errors.Is(err, exec.ErrNotFound):
		return Token{}, false, UnsupportedError("", "keyring", "secret-tool (libsecret-tools) is not on PATH")
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	case ctx.Err() != nil:
		return Token{}, false, fmt.Errorf("agentio: secret service lookup timed out after %s: %w", keychainTimeout, ctx.Err())
	case err != nil:
		return Token{}, false, fmt.Errorf("agentio: secret-tool: %w", err)
	}
	return secretToolResult(stdout.Bytes(), stderr.Bytes(), code)
}

// secretToolResult maps one secret-tool run: exit 0 is the secret (stdout,
// trailing newline trimmed); exit 1 with an empty stderr is "no entry";
// any other failure is ErrUnsupported carrying the first stderr line (the
// Secret Service is unreachable, for example no D-Bus session). Pure.
func secretToolResult(stdout, stderr []byte, code int) (Token, bool, error) {
	if code == 0 {
		return NewToken(strings.TrimRight(string(stdout), "\r\n")), true, nil
	}
	msg, _, _ := strings.Cut(strings.TrimSpace(string(stderr)), "\n")
	if code == secretToolNotFoundExit && msg == "" {
		return Token{}, false, nil
	}
	return Token{}, false, UnsupportedError("", "keyring", "secret service unavailable: "+msg)
}

// resolveCommandPath returns c.Path when it names a file (absolute, or with
// a directory part), else the first hit for it on c.Env's PATH under this
// OS's rules.
func resolveCommandPath(c Command) (string, error) {
	if filepath.IsAbs(c.Path) || strings.ContainsAny(c.Path, `/\`) {
		return c.Path, nil
	}
	pathList, _ := EnvLookupFor(runtime.GOOS, c.Env, envPath)
	pathExt, _ := EnvLookupFor(runtime.GOOS, c.Env, envPathExt)
	hits := LookPathAllFor(runtime.GOOS, pathList, pathExt, c.Path)
	if len(hits) == 0 {
		return "", fmt.Errorf("agentio: run %s: %w", c.Path, ErrNotInstalled)
	}
	return hits[0], nil
}
