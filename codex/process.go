package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// LiveProcess is one running Codex process from Host.Processes.
type LiveProcess struct {
	agentio.Process
	Kind agentio.ProcessKind
	// Home is "" unless argv names it; CODEX_HOME in another process's env
	// is not readable on macOS (SPEC §5.1).
	Home string
	Cwd  string // Host.Processes.Cwd; "" when not readable
}

// ClassifyProcess reports whether p is a Codex process and of what kind:
// the npm launcher (`node …/codex.js`), `exec`, `app-server` (daemon or
// proxy), `mcp-server`, else the interactive CLI. Session is true for cli
// and exec (conversation processes). Pure.
func (a *Agent) ClassifyProcess(p agentio.Process) (agentio.ProcessInfo, bool) {
	if !Spec().Process.Match(p) {
		return agentio.ProcessInfo{}, false
	}
	kind := classifyArgs(p)
	session := kind == agentio.ProcessCLI || kind == agentio.ProcessExec
	return agentio.ProcessInfo{Agent: ID, Kind: kind, Session: session}, true
}

// classifyArgs names the kind of a matched Codex row: an interpreter
// running a script is the npm launcher, else the first subcommand word
// decides. Pure.
func classifyArgs(p agentio.Process) agentio.ProcessKind {
	argv := p.Argv
	if _, ok := Spec().Process.ScriptIndex(p); ok && !Spec().Process.MatchesBasename(p) {
		return agentio.ProcessLauncher
	}
	for _, arg := range argv[min(1, len(argv)):] {
		switch arg {
		case subExec:
			return agentio.ProcessExec
		case subAppServer:
			return agentio.ProcessAppServer
		case subMCPServer:
			return agentio.ProcessMCPServer
		}
	}
	return agentio.ProcessCLI
}

// Processes lists running Codex processes through Host.Processes (one
// process-table read, plus one cwd lookup per match). Never runs codex.
// Process · Read, MayTouchLogin false.
func (a *Agent) Processes(ctx context.Context) ([]LiveProcess, error) {
	if a.host.Processes == nil {
		return nil, agentio.UnsupportedError(ID, "processes", "Host.Processes is nil")
	}
	rows, err := a.host.Processes.Processes(ctx)
	if err != nil {
		return nil, fmt.Errorf("codex: process table: %w", err)
	}
	var out []LiveProcess
	for _, p := range rows {
		info, ok := a.ClassifyProcess(p)
		if !ok {
			continue
		}
		lp := LiveProcess{Process: p, Kind: info.Kind}
		if cwd, err := a.host.Processes.Cwd(ctx, p.PID); err == nil {
			lp.Cwd = cwd
		}
		out = append(out, lp)
	}
	return out, nil
}

// Thread writer locks (upstream rollout/src/writer_lock.rs:17-18, 51):
// <home>/thread-writer-locks/<thread-id>.lock per thread being written,
// beside the permanent .coordination.lock. The files are empty: no pid,
// no owner, no time inside (codex-cli 0.162.1 capture, CODEX_API.md §15).
const (
	threadLockSuffix     = ".lock"
	coordinationLockFile = ".coordination.lock"
	// threadIDLen is the hyphenated UUID text upstream writes with
	// format!("{thread_id}") (protocol/src/thread_id.rs Display).
	threadIDLen = 36
	// lockStartTolerance absorbs the whole-second process start times of
	// darwin `ps` when comparing a process start with a lock's mtime.
	lockStartTolerance = 2 * time.Second
)

// threadIDHyphens are the byte offsets of the '-' in a hyphenated UUID.
func threadIDHyphens() []int { return []int{8, 13, 18, 23} }

// ParseThreadLockName returns the thread id of a thread-writer-locks/
// entry name: exactly a lowercase hyphenated UUID followed by ".lock",
// the only form Codex writes. .coordination.lock and every other name
// are rejected. Pure.
func ParseThreadLockName(name string) (string, bool) {
	id, ok := strings.CutSuffix(name, threadLockSuffix)
	if !ok || len(id) != threadIDLen {
		return "", false
	}
	hyphens := threadIDHyphens()
	for i := 0; i < len(id); i++ {
		c := id[i]
		if slices.Contains(hyphens, i) {
			if c != '-' {
				return "", false
			}
			continue
		}
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", false
		}
	}
	return id, true
}

// ThreadLock is one <home>/thread-writer-locks/<thread-id>.lock file.
// Codex holds an exclusive advisory lock (std File::try_lock, flock on
// unix) on it while a process writes the thread and deletes it when the
// writer finishes (writer_lock.rs:43-86, 174-196). A writer that crashes
// leaves the file behind unlocked until the next writer in the home
// sweeps it (writer_lock.rs:45-49, 124-171), so a file alone does not
// prove a live writer.
type ThreadLock struct {
	ThreadID string // conversation id: same UUIDv7 as the rollout file and the threads row
	Path     string
	// ModTime is the file's mtime. The file is created at acquire and never
	// written (opened without truncate, empty), so this is when the writer
	// took the thread.
	ModTime time.Time
}

// ThreadLocks lists the thread lock files of h, skipping
// .coordination.lock, directories and names that are not
// <uuid>.lock. Contents are never read (they are empty). Disk · Read.
func (a *Agent) ThreadLocks(h Home) ([]ThreadLock, error) {
	dir := a.Paths(h).ThreadWriterLocks
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("codex: read %s: %w", dirThreadWriterLocks, err)
	}
	var out []ThreadLock
	for _, e := range entries {
		if e.IsDir() || e.Name() == coordinationLockFile {
			continue
		}
		id, ok := ParseThreadLockName(e.Name())
		if !ok {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // removed between ReadDir and Info: the writer finished
		}
		out = append(out, ThreadLock{ThreadID: id, Path: filepath.Join(dir, e.Name()), ModTime: info.ModTime()})
	}
	return out, nil
}

// writerCandidate reports whether a process could own a thread lock taken
// at lockTime: a Codex process other than the npm launcher (exec, the
// interactive CLI, the app-server daemon the TUI attaches to, mcp-server
// all write threads) that started no later than the lock. Pure.
func (a *Agent) writerCandidate(p agentio.Process, lockTime time.Time) bool {
	info, ok := a.ClassifyProcess(p)
	if !ok || info.Kind == agentio.ProcessLauncher {
		return false
	}
	if p.Start.IsZero() || lockTime.IsZero() {
		return true
	}
	return !p.Start.After(lockTime.Add(lockStartTolerance))
}

// LiveSessions lists h's thread writer locks as live sessions. Upstream
// decides liveness by trying the lock's flock (writer_lock.rs:149-161);
// this library stays read-only and does not take it, because even a
// brief non-blocking probe would make a writer starting at that instant
// fail with "already has an active writer" (writer_lock.rs:65-72). So:
// ConversationID is the file's thread id, StartedAt its mtime, Dir the
// rollout's cwd when the rollout exists (Codex may create it later than
// the lock), Status Unknown, and Alive is true only when the process
// table holds a Codex process (not the npm launcher) that started no
// later than the lock: with none, the file is a crash leftover. PID stays
// 0 because the lock does not say which process holds it (several Codex
// processes, or one app-server daemon writing many threads, are common),
// and a process's CODEX_HOME is not readable (SPEC §5.1), so a process of
// another home also counts; FindLiveSession therefore never matches a
// Codex pid. A failed process-table read leaves every Alive false.
// Disk · Read + one process-table read (Host.Processes).
func (a *Agent) LiveSessions(h Home) ([]agentio.LiveSession, error) {
	locks, err := a.ThreadLocks(h)
	if err != nil || len(locks) == 0 {
		return nil, err
	}
	var procs []agentio.Process
	if a.host.Processes != nil {
		if rows, perr := a.host.Processes.Processes(context.Background()); perr == nil {
			procs = rows
		}
	}
	out := make([]agentio.LiveSession, 0, len(locks))
	for _, l := range locks {
		s := agentio.LiveSession{
			ConversationID: l.ThreadID,
			Home:           h.Neutral(),
			Status:         agentio.SessionStatusUnknown,
			StartedAt:      l.ModTime,
			Alive:          slices.ContainsFunc(procs, func(p agentio.Process) bool { return a.writerCandidate(p, l.ModTime) }),
		}
		if c, cerr := a.Conversation(h, l.ThreadID); cerr == nil {
			s.Dir = c.Dir
		}
		out = append(out, s)
	}
	slices.SortFunc(out, func(x, y agentio.LiveSession) int { return strings.Compare(x.ConversationID, y.ConversationID) })
	return out, nil
}

// IsLive reports whether conversationID has a thread writer lock that
// LiveSessions marks Alive. Disk · Read + one process-table read.
func (a *Agent) IsLive(h Home, conversationID string) (bool, error) {
	if conversationID == "" {
		return false, nil
	}
	all, err := a.LiveSessions(h)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(all, func(s agentio.LiveSession) bool {
		return s.ConversationID == conversationID && s.Alive
	}), nil
}

// LiveConversation reports whether thread-writer-locks/<id>.lock exists,
// without the process-table cross-check: true may be a crash leftover
// (see ThreadLock). Use IsLive for the checked answer. Disk · Read.
func (a *Agent) LiveConversation(h Home, id string) (bool, error) {
	if _, ok := ParseThreadLockName(id + threadLockSuffix); !ok {
		return false, nil
	}
	st, err := os.Lstat(filepath.Join(a.Paths(h).ThreadWriterLocks, id+threadLockSuffix))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("codex: stat %s: %w", dirThreadWriterLocks, err)
	}
	return !st.IsDir(), nil
}

// DaemonSocket reports the app-server socket path when it exists. Disk · Read.
func (a *Agent) DaemonSocket(h Home) (string, bool) {
	p := a.Paths(h).IPCSocket
	if _, err := os.Lstat(p); err != nil {
		return "", false
	}
	return p, true
}

// ChatProcess is one process_manager/chat_processes.json entry: a
// background command a chat started.
type ChatProcess struct {
	ID             string    `json:"id"`
	ProcessID      string    `json:"processId"`
	OSPid          int       `json:"osPid"`
	Command        string    `json:"command"`
	Cwd            string    `json:"cwd"`
	ConversationID string    `json:"conversationId"`
	ChatTitle      string    `json:"chatTitle"`
	TurnID         string    `json:"turnId"`
	ItemID         string    `json:"itemId"`
	StartedAtMs    int64     `json:"startedAtMs"`
	UpdatedAtMs    int64     `json:"updatedAtMs"`
	StartedAt      time.Time `json:"-"`
	UpdatedAt      time.Time `json:"-"`
}

// ChatProcesses reads chat_processes.json. The sample recorded the entry
// keys, not the container: a top-level array, or an object holding one
// array of entries, are both read. Disk · Read.
func (a *Agent) ChatProcesses(h Home) ([]ChatProcess, error) {
	b, err := readFile(a.Paths(h).ChatProcesses)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("codex: read %s: %w", fileChatProcesses, err)
	}
	var list []ChatProcess
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &obj); err != nil {
			return nil, fmt.Errorf("codex: decode %s: %w", fileChatProcesses, err)
		}
		for _, v := range obj {
			var l []ChatProcess
			if json.Unmarshal(v, &l) == nil {
				list = append(list, l...)
			}
		}
	} else if err := json.Unmarshal(trimmed, &list); err != nil {
		return nil, fmt.Errorf("codex: decode %s: %w", fileChatProcesses, err)
	}
	for i := range list {
		list[i].StartedAt = millis(list[i].StartedAtMs)
		list[i].UpdatedAt = millis(list[i].UpdatedAtMs)
	}
	return list, nil
}

func millis(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
