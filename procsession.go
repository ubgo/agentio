package agentio

import (
	"context"
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
	for _, a := range agents {
		c, ok := ProcessClassifierOf(a)
		if !ok {
			continue
		}
		info, ok := c.ClassifyProcess(p)
		if !ok {
			continue
		}
		if !info.Session {
			return ProcessSession{}, false, nil
		}
		r, ok := ProcessSessionsOf(a)
		if !ok {
			return ProcessSession{}, false, nil
		}
		return r.ProcessSession(ctx, p)
	}
	return ProcessSession{}, false, nil
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
