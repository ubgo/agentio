package claude

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/ubgo/agentio"
)

// Resume flags whose value names the conversation a process runs:
// `--resume <id>`, `-r <id>` and `--session-id <id>` (a new session with a
// chosen id). `--resume` also takes a search term, so only a UUID counts.
const flagResumeShort = "-r"

// resumeFlags lists the argv flags that name a conversation. Pure.
func resumeFlags() []string { return []string{flagResume, flagResumeShort, flagSessionID} }

// stopGrace is how long a Claude session gets after SIGTERM: it writes its
// transcript line by line, so the wait only has to cover the turn's last
// write and the exit (agentop's measured kill grace).
const stopGrace = 4 * time.Second

// refuseMCPServe is the stop refusal for `claude mcp serve`.
const refuseMCPServe = "claude mcp serve is an MCP server for another client, not a conversation"

// ProcessSession joins a Claude process row to what Claude recorded:
//   - exact: <home>/sessions/<pid>.json, read directly by pid in the home
//     the process's CLAUDE_CONFIG_DIR names (every discovered home when
//     its environment is unreadable), and only when the file's startedAt
//     is within the pid-reuse tolerance of the row's start time (a file
//     left by an earlier process with the same pid is skipped; a row or
//     file without a start time gives JoinInferred);
//   - argv: `--resume <id>`, `-r <id>`, `--session-id <id>` (UUIDs only),
//     with the home from the process env when readable, else the home
//     whose projects/ holds that transcript, else the default home;
//   - otherwise the home alone (JoinUnknown) when the env names it.
//
// Status maps waiting to SessionWaiting; LastActivity is the newer of
// statusUpdatedAt and updatedAt (argv joins: the transcript's mtime).
// found is false for a row that is not a Claude session process.
// cost: Disk · Read (one small file per candidate home, a projects glob
// for argv joins) + Process · Read (the start env; the cwd for argv
// joins).
func (a *Agent) ProcessSession(ctx context.Context, p agentio.Process) (agentio.ProcessSession, bool, error) {
	info, ok := ClassifyProcess(p)
	if !ok || !info.Session {
		return agentio.ProcessSession{}, false, nil
	}
	envHome, envErr := a.core.ProcessHome(ctx, p.PID)
	var candidates []Home
	if envErr == nil {
		candidates = []Home{wrap(envHome)}
	} else {
		hs, err := a.Homes()
		if err != nil {
			return agentio.ProcessSession{}, false, err
		}
		candidates = hs
	}
	for _, h := range candidates {
		if s, ok := a.pidSession(h, p); ok {
			return s, true, nil
		}
	}
	base := agentio.ProcessSession{Agent: ID, PID: p.PID, Dir: a.processCwd(ctx, p.PID)}
	if envErr == nil {
		base.Home = envHome
	}
	if id, ok := agentio.ArgValue(p.Argv, resumeFlags()...); ok && agentio.IsUUID(id) {
		base.ConversationID = id
		base.Confidence = agentio.JoinArgv
		home, path := a.transcriptHome(candidates, envErr == nil, id)
		base.Home = home.Neutral()
		if path != "" {
			base.Path = path
			if fi, err := os.Stat(path); err == nil {
				base.LastActivity = fi.ModTime()
			}
		}
		return base, true, nil
	}
	if envErr == nil {
		return base, true, nil
	}
	return agentio.ProcessSession{}, false, nil
}

// pidSession reads h's sessions/<pid>.json for p and applies the
// pid-reuse guard. cost: Disk · Read (one small file).
func (a *Agent) pidSession(h Home, p agentio.Process) (agentio.ProcessSession, bool) {
	path := filepath.Join(h.Dir(), sessionsDir, strconv.Itoa(p.PID)+jsonExt)
	data, found, err := readFileBounded(path, maxResponseBytes)
	if err != nil || !found {
		return agentio.ProcessSession{}, false
	}
	l, err := parseLiveSession(path, data)
	if err != nil || l.PID != p.PID {
		return agentio.ProcessSession{}, false
	}
	conf := agentio.JoinExact
	if p.Start.IsZero() || l.StartedAt.IsZero() {
		conf = agentio.JoinInferred
	} else if d := p.Start.Sub(l.StartedAt); d > liveTolerance || d < -liveTolerance {
		return agentio.ProcessSession{}, false
	}
	s := agentio.ProcessSession{
		Agent: ID, PID: p.PID, Home: h.Neutral(), ConversationID: l.SessionID, Dir: l.Cwd,
		Version: l.Version, Kind: l.Kind, Entrypoint: l.Entrypoint, Status: l.Status.Neutral(),
		StatusSince: l.StatusUpdatedAt, LastActivity: later(l.StatusUpdatedAt, l.UpdatedAt), Confidence: conf,
	}
	if l.SessionID != "" && l.Cwd != "" {
		t := filepath.Join(a.HistoryDir(h, l.Cwd), l.SessionID+transcriptExt)
		if _, err := os.Stat(t); err == nil {
			s.Path = t
		}
	}
	return s, true
}

// transcriptHome picks the home of an argv join: the env home when it was
// readable, else the first candidate whose projects/*/<id>.jsonl exists,
// else the default home. path is the transcript when found. cost: Disk ·
// Read (one projects glob per candidate).
func (a *Agent) transcriptHome(candidates []Home, envKnown bool, id string) (Home, string) {
	for _, h := range candidates {
		matches, err := filepath.Glob(filepath.Join(h.Dir(), projectsDir, "*", id+transcriptExt))
		if err == nil && len(matches) > 0 {
			return h, matches[0]
		}
		if envKnown {
			return h, ""
		}
	}
	return a.DefaultHome(), ""
}

// processCwd is the process's working directory, "" when unreadable.
// cost: Process · Read.
func (a *Agent) processCwd(ctx context.Context, pid int) string {
	if a.host.Processes == nil {
		return ""
	}
	cwd, err := a.host.Processes.Cwd(ctx, pid)
	if err != nil {
		return ""
	}
	return cwd
}

// later returns the later of two times. cost: Pure.
func later(x, y time.Time) time.Time {
	if y.After(x) {
		return y
	}
	return x
}

// StopPlan plans how to stop a Claude process: `claude mcp serve` is
// refused (it serves another client); a session is SIGTERM with a 4 s
// grace (forced on windows, see agentio.Core.BaseStopPlan); a background
// session (Kind bg) is ended with `claude stop <id>` in its home instead,
// because the background supervisor respawns a killed worker, and forcing
// it is marked unsafe. Resume is `claude --resume <id>` when s names the
// conversation. cost: Pure.
func (a *Agent) StopPlan(p agentio.Process, _ []agentio.Process, s agentio.ProcessSession) (agentio.StopPlan, error) {
	info, ok := ClassifyProcess(p)
	if !ok {
		return agentio.StopPlan{}, agentio.NotThisAgentProcess(ID, p)
	}
	plan := a.core.BaseStopPlan(info, p, s)
	if info.Kind == agentio.ProcessMCPServer {
		plan.Refuse = refuseMCPServe
	}
	if plan.Refuse != "" {
		return plan, nil
	}
	plan.Grace = stopGrace
	if s.Kind == agentio.SessionBackground && s.ConversationID != "" {
		h := a.DefaultHome() // no home known: `claude stop` in the default home
		if !s.Home.IsZero() {
			hh, err := a.HomeOf(s.Home)
			if err != nil {
				return agentio.StopPlan{}, fmt.Errorf("claude: stop plan: %w", err)
			}
			h = hh
		}
		cmd := a.command(h, subStop, s.ConversationID)
		plan.Command = &cmd
		plan.Force = false
	}
	return plan, nil
}
