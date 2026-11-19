package agentio

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Host is the machine the library looks at: user home, base environment,
// clock, HTTP client, keychain, process table and the way to run a program
// (SPEC §5.1). Nothing in the library reads $HOME, os.Environ, the keychain,
// the clock or the network except through a Host, so a test points every
// input at a fixture and two homes can be inspected side by side.
// HostFromOS builds the real one; agenttest.NewHost builds a fixture one.
type Host struct {
	// UserHome is the user's home directory (absolute).
	UserHome string
	// User is the OS login name. Claude keychain lookups name it as the
	// account first (invariant 3): stale same-service entries exist.
	User string
	// Env is the base environment ("K=V"); PATH lookups use it, never a shell.
	Env []string
	// Now is the clock.
	Now func() time.Time
	// HTTP is used for usage, subscription and probe calls.
	HTTP *http.Client
	// Secrets is the keychain / keyring, or a fake.
	Secrets SecretStore
	// Processes lists running processes (live sessions, process matching).
	Processes ProcessLister
	// Exec runs a program; reached only from Runner doors.
	Exec ProcessRunner
	// GOOS is the operating system whose system locations apply ("linux",
	// "darwin", "windows"), so a vendor never branches on runtime.GOOS and a
	// test can inspect another OS's layout. "" = no system locations.
	GOOS string
	// SystemRoot is the root that system and managed locations are resolved
	// under: "/" on unix and the system drive root on Windows from
	// HostFromOS, a temp dir in a test that models /etc. "" (the fixture
	// default) = the host has no system locations, so managed settings,
	// managed CLAUDE.md and MCP files and /etc/codex layers are never read
	// and fixture output cannot depend on the test machine's /etc.
	SystemRoot string
}

// SystemPath returns rel (a slash-separated path relative to the system
// root, such as "etc/claude-code") under SystemRoot; false when the host
// has no SystemRoot or no GOOS. Pure.
func (h Host) SystemPath(rel string) (string, bool) {
	if h.SystemRoot == "" || h.GOOS == "" {
		return "", false
	}
	return filepath.Join(h.SystemRoot, filepath.FromSlash(rel)), true
}

// Getenv returns the value of key in Host.Env and whether it is present.
// An entry "K=" is present with value "". Pure.
// Names compare case-insensitively when Host.GOOS is windows (EnvLookupFor).
func (h Host) Getenv(key string) (string, bool) { return EnvLookupFor(h.hostOS(), h.Env, key) }

// Process is one row of the process table. Start guards against pid reuse:
// a pid with another start time is another process.
type Process struct {
	PID, PPID int
	Start     time.Time
	// Exe is the executable path when known (linux /proc/{pid}/exe), else argv[0].
	Exe string
	// Argv is the command line. On darwin it is split on spaces from `ps`
	// output, so an argument containing a space is split too.
	Argv []string
	// CommandLine is the command line as one string, unsplit: the `ps`
	// command column on darwin, the NUL-separated cmdline joined with
	// spaces on linux, Win32_Process CommandLine on windows. Exclusions
	// and substring matches test it, because darwin's space-split Argv
	// cuts a path like "Application Support/Claude" in two. "" = unknown.
	CommandLine string
	// RSS is the resident set size in bytes; 0 = unknown.
	RSS uint64
	// TTY is the controlling terminal without "/dev/" ("ttys004",
	// "pts/3"); "" = none or unknown (always "" on windows).
	TTY string
	// CPUTime is the user plus system CPU time used so far; 0 = unknown.
	// A CPU percentage needs two samples, which is the caller's job.
	CPUTime time.Duration
	// Cwd is the working directory when the caller already looked it up
	// (ProcessLister.Cwd); Host.Processes leaves it "" because reading it
	// costs a run per process on darwin. Readers that need a cwd
	// (ProcessSessionReader) use it instead of a second lookup. "" = not
	// looked up.
	Cwd string
}

// CommandString is CommandLine, or Argv joined with spaces when the
// table gave no raw line. Pure.
func (p Process) CommandString() string {
	if p.CommandLine != "" {
		return p.CommandLine
	}
	return strings.Join(p.Argv, " ")
}

// ProcessLister reads the process table.
type ProcessLister interface {
	// Processes is Process · Read: one process-table read.
	Processes(ctx context.Context) ([]Process, error)
	// Cwd is Process · Read: the working directory of pid.
	Cwd(ctx context.Context, pid int) (string, error)
}

// ProcessEnvReader is an optional extension of ProcessLister: the start
// environment of a process, so a caller can tell its own launch (one that
// set AGX_RUN_ID=<id>) from an unrelated process that reused the pid.
// HostFromOS's lister implements it on linux (/proc/<pid>/environ) and
// darwin (`ps -E`); windows keeps another process's environment only in its
// memory, so there it is ErrUnsupported.
type ProcessEnvReader interface {
	// Env is Process · Read: pid's start environment as NAME=value
	// entries. On darwin the entries are split on spaces from `ps -E`
	// output, mixed with the command line, so match a whole NAME=value
	// entry, never parse values with spaces.
	Env(ctx context.Context, pid int) ([]string, error)
}

// ProcessEnvBatchReader is an optional extension of ProcessLister: the start
// environments of many processes in one read, for joining a whole process
// table to sessions (FindProcessSessions) without a run per process.
// HostFromOS's lister implements it on linux (one /proc read per pid, no
// process run) and darwin (one `ps -E` run for every pid).
type ProcessEnvBatchReader interface {
	// EnvBatch is Process · Read: the start environment of each pid whose
	// environment is readable, same entry format as Env. A pid that has
	// ended or whose environment is hidden (a protected darwin process)
	// is missing from the map; the error is for a read that failed as a
	// whole (windows: ErrUnsupported).
	EnvBatch(ctx context.Context, pids []int) (map[int][]string, error)
}

// SecretStore reads the keychain or keyring.
type SecretStore interface {
	// Get is Process · Read. account "" is a service-only lookup, used only
	// as the documented fallback after the named-account lookup (invariant
	// 3). found is false (with a nil error) when no entry exists. Never
	// writes, never lists values.
	Get(ctx context.Context, service, account string) (Token, bool, error)
}

// ProcessRunner runs a program once and collects its output.
type ProcessRunner interface {
	// Run is Process · Read; reached only from Runner doors. A non-zero exit
	// is reported in ProcessResult.ExitCode with a nil error; the error is
	// for "could not run at all" (missing binary, context cancelled).
	Run(ctx context.Context, c Command, stdin io.Reader) (ProcessResult, error)
}

// ProcessResult is a finished run.
type ProcessResult struct {
	Stdout, Stderr []byte
	ExitCode       int
}

// envSep separates key and value in an environment entry.
const envSep = "="

// EnvLookup returns the last value of key in env (the one a child process
// sees on most platforms) and whether it is present. Pure.
func EnvLookup(env []string, key string) (string, bool) {
	prefix := key + envSep
	val, found := "", false
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, prefix); ok {
			val, found = v, true
		}
	}
	return val, found
}
