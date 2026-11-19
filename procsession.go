package agentio

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ProcessSession is what an agent knows about one of its running
// processes: which home and conversation it serves, how sure that join is,
// and the activity the agent itself recorded. Every field but Agent, PID
// and Confidence may be zero: an agent fills what its files hold.
type ProcessSession struct {
	Agent ID
	// PID is the process the session was read for.
	PID int
	// Home is the home the process runs in; zero when it cannot be told
	// (another process's environment is unreadable on windows).
	Home Home
	// ConversationID is the conversation the process serves; "" when
	// unknown (a new session that has not written anything yet).
	ConversationID string
	// Dir is the working directory the agent recorded, else the process
	// cwd; "" when neither is readable.
	Dir string
	// Path is the transcript file when known (TurnFilesOf reads it).
	Path string
	// Version is the agent version the process runs, when recorded.
	Version string
	// Kind is the vendor's kind of session (interactive, background).
	Kind SessionKind
	// Entrypoint is how the agent says it was started, as it records it
	// (Claude "cli", "sdk-ts"; Codex originator "codex_cli_rs",
	// "Codex Desktop"); "" when not recorded. Which terminal or tool
	// started it is the caller's question (see Ancestors).
	Entrypoint string
	// Status is the activity the agent recorded; SessionStatusUnknown
	// when it records none (every agent but Claude).
	Status SessionStatus
	// StatusSince is when Status last changed; zero when unknown.
	StatusSince time.Time
	// LastActivity is the agent's own last-activity time: Claude's
	// statusUpdatedAt / updatedAt, else the transcript's mtime. It moves
	// while a turn streams with a quiet screen and stays put while a
	// spinner redraws, which terminal I/O time cannot tell. Zero = unknown.
	LastActivity time.Time
	// Confidence says how the process was tied to the conversation; only
	// JoinExact is evidence for acting on the process.
	Confidence JoinConfidence
}

// ProcessSessionReader joins a process row to what the agent recorded
// about it (an optional capability; ProcessSessionsOf).
type ProcessSessionReader interface {
	// ProcessSession is Disk · Read, plus Process · Read for the process
	// cwd or start environment where the agent needs them. found is false
	// (with a nil error) when p is not this agent's session process or
	// nothing ties it to a home; an error is for a read that failed.
	ProcessSession(ctx context.Context, p Process) (ProcessSession, bool, error)
}

// FindProcessSession classifies p against agents (first match wins, as
// ClassifyProcess) and asks that agent's ProcessSessionReader. found is
// false when no agent claims p, the agent has no reader, or the reader
// finds nothing. Disk · Read + Process · Read.
func FindProcessSession(ctx context.Context, agents []Agent, p Process) (ProcessSession, bool, error) {
	i, ok := sessionAgent(agents, p)
	if !ok {
		return ProcessSession{}, false, nil
	}
	r, ok := ProcessSessionsOf(agents[i])
	if !ok {
		return ProcessSession{}, false, nil
	}
	return r.ProcessSession(ctx, p)
}

// ProcessSessionBatchReader is the batch form of ProcessSessionReader,
// for joining a whole process table at once: the start environments are
// read once for every row (ProcessEnvBatchReader) instead of once per row.
// An agent that implements it also implements ProcessSessionReader, with
// the same results row by row (ProcessSessionBatchOf).
type ProcessSessionBatchReader interface {
	// ProcessSessions is ProcessSession for every row of ps: the result
	// holds, by pid, each row a session was found for; rows that are not
	// this agent's session processes are skipped. The error joins the
	// rows whose read failed (the other rows are still in the map).
	// Disk · Read + Process · Read (one environment read for all rows,
	// plus a cwd lookup for a row without Process.Cwd that needs one).
	ProcessSessions(ctx context.Context, ps []Process) (map[int]ProcessSession, error)
}

// FindProcessSessions is FindProcessSession for a whole process table:
// each row is classified against agents (first match wins), the session
// rows are grouped per agent, and each group is read with one
// ProcessSessionBatchReader call when the agent has one, else row by row.
// The result holds, by pid, every row a session was found for. The error
// joins the failed reads; the rows read before and after are still
// returned. Set Process.Cwd on rows whose cwd the caller already looked
// up, so no reader looks it up again. Disk · Read + Process · Read.
func FindProcessSessions(ctx context.Context, agents []Agent, ps []Process) (map[int]ProcessSession, error) {
	groups := make([][]Process, len(agents))
	for _, p := range ps {
		if i, ok := sessionAgent(agents, p); ok {
			groups[i] = append(groups[i], p)
		}
	}
	out := map[int]ProcessSession{}
	var errs []error
	for i, rows := range groups {
		if len(rows) == 0 {
			continue
		}
		if b, ok := ProcessSessionBatchOf(agents[i]); ok {
			found, err := b.ProcessSessions(ctx, rows)
			for pid, s := range found {
				out[pid] = s
			}
			if err != nil {
				errs = append(errs, err)
			}
			continue
		}
		r, ok := ProcessSessionsOf(agents[i])
		if !ok {
			continue
		}
		for _, p := range rows {
			s, found, err := r.ProcessSession(ctx, p)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if found {
				out[p.PID] = s
			}
		}
	}
	return out, errors.Join(errs...)
}

// sessionAgent returns the index of the first agent that claims p as a
// session process (FindProcessSession's rule); false when the first
// claimant calls it a non-session kind or no agent claims it. Pure.
func sessionAgent(agents []Agent, p Process) (int, bool) {
	for i, a := range agents {
		c, ok := ProcessClassifierOf(a)
		if !ok {
			continue
		}
		info, ok := c.ClassifyProcess(p)
		if !ok {
			continue
		}
		return i, info.Session
	}
	return 0, false
}

// ProcessCwd is p.Cwd when the caller filled it, else the lister's Cwd
// lookup. Process · Read (none when p.Cwd is set).
func (h Host) ProcessCwd(ctx context.Context, p Process) (string, error) {
	if p.Cwd != "" {
		return p.Cwd, nil
	}
	if h.Processes == nil {
		return "", UnsupportedError("", "process cwd", "the host has no process lister")
	}
	return h.Processes.Cwd(ctx, p.PID)
}

// Ancestors returns the parent chain of pid in table, nearest parent
// first, at most maxHops rows (<= 0 = no limit). The walk stops at a pid
// missing from the table, at pid 0 or 1, and at a cycle (a pid table read
// while processes change can hold one). Which launcher a chain belongs to
// (a terminal, tmux, an IDE) is the caller's knowledge; agentio only
// walks. Pure.
func Ancestors(table []Process, pid, maxHops int) []Process {
	byPID := make(map[int]Process, len(table))
	for _, p := range table {
		byPID[p.PID] = p
	}
	var out []Process
	seen := map[int]bool{pid: true}
	cur, ok := byPID[pid]
	for ok && (maxHops <= 0 || len(out) < maxHops) {
		parent, found := byPID[cur.PPID]
		if !found || cur.PPID <= initPID || seen[cur.PPID] {
			break
		}
		seen[cur.PPID] = true
		out = append(out, parent)
		cur = parent
	}
	return out
}

// initPID is the first user process (launchd, init, systemd); the chain
// stops below it because every process descends from it.
const initPID = 1

// ProcessHome returns the home the process pid runs in, read from its
// start environment: the home Spec.HomeEnv names there (resolved like
// HomeFromEnv), else the default home, because a vendor CLI started
// without the variable uses its default. The error is set, and the home
// zero, when the environment cannot be read (windows, a protected darwin
// process, a host lister without ProcessEnvReader): then the home is
// unknown, never "the default". Process · Read (one environment read).
func (c *Core) ProcessHome(ctx context.Context, pid int) (Home, error) {
	if c.host.Processes == nil {
		return Home{}, UnsupportedError(c.spec.ID, "process env", "the host has no process lister")
	}
	env, err := c.host.processEnv(ctx, pid)
	if err != nil {
		return Home{}, err
	}
	return c.homeFromProcessEnv(env), nil
}

// ProcessHomes is ProcessHome for many pids with one environment read
// (ProcessEnvBatchReader when the lister has it). The map holds the pids
// whose environment was read; a missing pid's home is unknown, never
// "the default" (it ended, darwin hides its environment, or the read is
// unsupported). The error is set only when nothing could be read (no
// lister or env reader, windows, a batch read that failed). Process ·
// Read (one environment read for all pids).
func (c *Core) ProcessHomes(ctx context.Context, pids []int) (map[int]Home, error) {
	if c.host.Processes == nil {
		return nil, UnsupportedError(c.spec.ID, "process env", "the host has no process lister")
	}
	envs, err := c.host.processEnvs(ctx, pids)
	if err != nil {
		return nil, err
	}
	out := make(map[int]Home, len(envs))
	for pid, env := range envs {
		out[pid] = c.homeFromProcessEnv(env)
	}
	return out, nil
}

// homeFromProcessEnv is the home a start environment selects: the home
// Spec.HomeEnv names (resolved like HomeFromEnv), else the default. Pure.
func (c *Core) homeFromProcessEnv(env []string) Home {
	if c.spec.HomeEnv == "" {
		return c.DefaultHome()
	}
	v, ok := EnvLookupFor(c.host.hostOS(), env, c.spec.HomeEnv)
	if !ok || v == "" {
		return c.DefaultHome()
	}
	if c.spec.HomeEnvNamesParent {
		v = filepath.Join(resolveHomeDir(c.host.hostOS(), c.host.UserHome, v), c.spec.DefaultHome)
	}
	return c.HomeAt(v)
}

// flagValueSep joins a long flag and its value in one argument.
const flagValueSep = "="

// ArgValue returns the value of the first of flags present in argv, in
// either spelling a CLI parser accepts: "--resume id" (the next
// argument) or "--resume=id". A flag that is the last argument has no
// value (false). Only argv after argv[0] is searched. Pure.
func ArgValue(argv []string, flags ...string) (string, bool) {
	for i := 1; i < len(argv); i++ {
		for _, f := range flags {
			if f == "" {
				continue
			}
			if argv[i] == f {
				if i+1 < len(argv) {
					return argv[i+1], true
				}
				return "", false
			}
			if v, ok := strings.CutPrefix(argv[i], f+flagValueSep); ok {
				return v, true
			}
		}
	}
	return "", false
}

// uuidHyphens are the byte offsets of the '-' in a hyphenated UUID.
func uuidHyphens() []int { return []int{8, 13, 18, 23} }

// uuidLen is the length of a hyphenated UUID.
const uuidLen = 36

// IsUUID reports a hyphenated UUID in either case
// ("6d50c0ec-647e-18aa-84c7-04170288be89"): the shape Claude, Codex and
// Gemini conversation ids take, so an argv value such as a search term
// after `claude --resume` is not taken for an id. Pure.
func IsUUID(s string) bool {
	if len(s) != uuidLen {
		return false
	}
	hyphens := uuidHyphens()
	for i := 0; i < len(s); i++ {
		c := s[i]
		if slices.Contains(hyphens, i) {
			if c != '-' {
				return false
			}
			continue
		}
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}
