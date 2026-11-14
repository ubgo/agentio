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

	"github.com/khanakia/agentcli"
)

// argv words that refine a Codex process's kind.
const (
	nodeBinary   = "node"
	launcherFile = "codex.js"
	argDaemon    = "daemon"
	argProxy     = "proxy"
)

// LiveProcess is one running Codex process from Host.Processes.
type LiveProcess struct {
	agentcli.Process
	Kind agentcli.ProcessKind
	// Home is "" unless argv names it; CODEX_HOME in another process's env
	// is not readable on macOS (SPEC §5.1).
	Home string
	Cwd  string // Host.Processes.Cwd; "" when not readable
}

// ClassifyProcess reports whether p is a Codex process and of what kind:
// the npm launcher (`node …/codex.js`), `exec`, `app-server` (daemon or
// proxy), `mcp-server`, else the interactive CLI. Session is true for cli
// and exec (conversation processes). Pure.
func (a *Agent) ClassifyProcess(p agentcli.Process) (agentcli.ProcessInfo, bool) {
	if !Spec().Process.Match(p) {
		return agentcli.ProcessInfo{}, false
	}
	kind := classifyArgs(p)
	session := kind == agentcli.ProcessCLI || kind == agentcli.ProcessExec
	return agentcli.ProcessInfo{Agent: ID, Kind: kind, Session: session}, true
}

func classifyArgs(p agentcli.Process) agentcli.ProcessKind {
	argv := p.Argv
	base := ""
	if len(argv) > 0 {
		base = filepath.Base(argv[0])
	}
	if base == nodeBinary || (len(argv) > 1 && strings.HasSuffix(argv[1], launcherFile)) {
		return agentcli.ProcessLauncher
	}
	for _, arg := range argv[min(1, len(argv)):] {
		switch arg {
		case subExec:
			return agentcli.ProcessExec
		case subAppServer:
			return agentcli.ProcessAppServer
		case subMCPServer:
			return agentcli.ProcessMCPServer
		}
	}
	return agentcli.ProcessCLI
}

// Processes lists running Codex processes through Host.Processes (one
// process-table read, plus one cwd lookup per match). Never runs codex.
// Process · Read, MayTouchLogin false.
func (a *Agent) Processes(ctx context.Context) ([]LiveProcess, error) {
	if a.host.Processes == nil {
		return nil, agentcli.UnsupportedError(ID, "processes", "Host.Processes is nil")
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

// LiveSessions answers from thread-writer locks; their format has no
// recorded sample (CODEX_API.md §15), so it returns none until one is
// recorded (FindLiveSession then reports found=false). Disk · Read.
func (a *Agent) LiveSessions(h Home) ([]agentcli.LiveSession, error) {
	return nil, nil
}

// IsLive reports whether a thread-writer lock exists for the
// conversation; with no recorded lock format this is false (see
// LiveSessions). Disk · Read.
func (a *Agent) IsLive(h Home, conversationID string) (bool, error) {
	return a.LiveConversation(h, conversationID)
}

// LiveConversation reports whether thread-writer-locks/ holds an entry
// whose name contains the id. The lock format is unrecorded, so only the
// name is matched. Disk · Read.
func (a *Agent) LiveConversation(h Home, id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	entries, err := os.ReadDir(a.Paths(h).ThreadWriterLocks)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("codex: read %s: %w", dirThreadWriterLocks, err)
	}
	return slices.ContainsFunc(entries, func(e fs.DirEntry) bool { return strings.Contains(e.Name(), id) }), nil
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
	b, err := os.ReadFile(a.Paths(h).ChatProcesses)
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
