package codex

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Process-session bounds and argv words.
const (
	// flagCdLong is the long spelling of -C (`codex --cd DIR`): the
	// session's cwd when the process was started elsewhere.
	flagCdLong = "--cd"
	// inferBefore and inferAfter bound the date folders an inferred join
	// scans around the process start: rollouts live in
	// sessions/YYYY/MM/DD named by local date, so a day before covers a
	// zone difference, and Codex writes the rollout at the first turn,
	// which can come a while after the process started. A process idle
	// longer than inferAfter before its first prompt is not joined.
	inferBefore = 24 * time.Hour
	inferAfter  = 48 * time.Hour
	day         = 24 * time.Hour
	// rolloutDayLayout is the sessions/ date folder path.
	rolloutDayLayout = "2006/01/02"
	// refuseAppServer and refuseMCPServer are the stop refusals for the
	// daemons a TUI or another client attaches to.
	refuseAppServer = "codex app-server serves many threads and the TUI reattaches to it; stop the sessions, not the daemon"
	refuseMCPServer = "codex mcp-server is an MCP server for another client, not a conversation"
	// refuseLauncherNoChild is the refusal for an npm launcher whose
	// native child is missing from the table.
	refuseLauncherNoChild = "npm launcher: its native codex child holds the session and is not in the process table"
)

// ProcessSession joins a Codex process row to its rollout:
//   - argv: `codex resume <id>` or `codex exec resume <id>` (a UUID), the
//     rollout found by name in the home the process's CODEX_HOME names
//     (every discovered home when the env is unreadable); JoinArgv;
//   - inferred: the rollout whose session_meta cwd equals the process cwd
//     (or its -C/--cd dir) and whose creation is the closest at or after
//     the process start, from the date folders inferBefore before to
//     inferAfter after the start (session_meta heads only); JoinInferred,
//     a guess never to act on;
//   - otherwise the home alone (JoinUnknown) when the env names it.
//
// LastActivity is the rollout's mtime, Version its cli_version, Entrypoint
// its originator. The npm launcher, app-server and mcp-server are not
// session processes (found false). cost: Disk · Read (a name walk of the
// home's sessions for argv joins; a few date-folder listings and heads for
// inferred joins) + Process · Read (start env and cwd).
func (a *Agent) ProcessSession(ctx context.Context, p agentio.Process) (agentio.ProcessSession, bool, error) {
	info, ok := a.ClassifyProcess(p)
	if !ok || !info.Session {
		return agentio.ProcessSession{}, false, nil
	}
	envHome, envErr := a.core.ProcessHome(ctx, p.PID)
	var homes []Home
	if envErr == nil {
		homes = []Home{{h: envHome}}
	} else {
		hs, err := a.Homes()
		if err != nil {
			return agentio.ProcessSession{}, false, err
		}
		homes = hs
	}
	base := agentio.ProcessSession{Agent: ID, PID: p.PID, Dir: a.sessionCwd(ctx, p)}
	if envErr == nil {
		base.Home = envHome
	}
	if id, ok := resumeID(p.Argv); ok {
		base.ConversationID, base.Confidence = id, agentio.JoinArgv
		for _, h := range homes {
			if c, err := a.Conversation(h, id); err == nil {
				return withRollout(base, h, c.Path, c.Dir, c.CLIVersion.String(), c.Originator, c.Updated), true, nil
			}
		}
		if base.Home.IsZero() {
			base.Home = a.DefaultHome().Neutral()
		}
		return base, true, nil
	}
	if !p.Start.IsZero() && base.Dir != "" {
		var best inferredRollout
		for _, h := range homes {
			if r, ok := a.inferRollout(h, base.Dir, p.Start); ok && (best.path == "" || r.created.Before(best.created)) {
				best = r
			}
		}
		if best.path != "" {
			base.ConversationID, base.Confidence = best.meta.ID, agentio.JoinInferred
			return withRollout(base, best.home, best.path, best.meta.Cwd, best.meta.CLIVersion, best.meta.Originator, best.mod), true, nil
		}
	}
	if envErr == nil {
		return base, true, nil
	}
	return agentio.ProcessSession{}, false, nil
}

// withRollout fills s from a rollout of h. cost: Pure.
func withRollout(s agentio.ProcessSession, h Home, path, dir, version, originator string, mod time.Time) agentio.ProcessSession {
	s.Home, s.Path, s.LastActivity, s.Version, s.Entrypoint = h.Neutral(), path, mod, version, originator
	if dir != "" {
		s.Dir = dir
	}
	return s
}

// resumeID returns the conversation id after the `resume` subcommand
// (`codex resume <id>`, `codex exec resume <id>`): the first argument
// after it that is not a flag, when it is a UUID (`resume --last` and a
// thread name are not ids). cost: Pure.
func resumeID(argv []string) (string, bool) {
	i := slices.Index(argv[min(1, len(argv)):], subResume)
	if i < 0 {
		return "", false
	}
	for _, arg := range argv[i+2:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg, agentio.IsUUID(arg)
	}
	return "", false
}

// sessionCwd is the folder the session runs in: the -C/--cd value
// (relative to the process cwd), else the process cwd; "" when neither
// is known. cost: Process · Read.
func (a *Agent) sessionCwd(ctx context.Context, p agentio.Process) string {
	cwd := ""
	if a.host.Processes != nil {
		if d, err := a.host.Processes.Cwd(ctx, p.PID); err == nil {
			cwd = d
		}
	}
	if dir, ok := agentio.ArgValue(p.Argv, flagCd, flagCdLong); ok && dir != "" {
		if filepath.IsAbs(dir) {
			return filepath.Clean(dir)
		}
		if cwd == "" {
			return ""
		}
		return filepath.Join(cwd, dir)
	}
	return cwd
}

// inferredRollout is the best rollout candidate of an inferred join.
type inferredRollout struct {
	home    Home
	path    string
	meta    SessionMeta
	created time.Time
	mod     time.Time
}

// inferRollout finds h's rollout whose session_meta cwd is cwd and whose
// creation is the earliest at or after start (lockStartTolerance early
// allowed, for darwin's whole-second start times), scanning only the
// date folders from start-inferBefore to start+inferAfter (capped at the
// host clock plus a day). cost: Disk · Read (folder listings and one
// bounded head read per rollout in them).
func (a *Agent) inferRollout(h Home, cwd string, start time.Time) (inferredRollout, bool) {
	root := a.Paths(h).Sessions
	end := start.Add(inferAfter)
	if a.host.Now != nil {
		if now := a.host.Now().Add(day); now.Before(end) {
			end = now
		}
	}
	want := filepath.Clean(cwd)
	floor := start.Add(-lockStartTolerance)
	var best inferredRollout
	seen := map[string]bool{}
	for t := start.Add(-inferBefore); !t.After(end.Add(day)); t = t.Add(day) {
		dir := filepath.Join(root, filepath.FromSlash(t.Format(rolloutDayLayout)))
		if seen[dir] {
			continue
		}
		seen[dir] = true
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // no rollouts that day
		}
		for _, e := range entries {
			if e.IsDir() || !isRolloutName(e.Name()) {
				continue
			}
			path := filepath.Join(dir, e.Name())
			m, ok := readHeadMeta(path)
			if !ok || filepath.Clean(m.Cwd) != want {
				continue
			}
			created := m.Created()
			if created.IsZero() || created.Before(floor) || (best.path != "" && !created.Before(best.created)) {
				continue
			}
			r := inferredRollout{home: h, path: path, meta: m, created: created}
			if fi, err := e.Info(); err == nil {
				r.mod = fi.ModTime()
			}
			best = r
		}
	}
	return best, best.path != ""
}

// StopPlan plans how to stop a Codex process: app-server and mcp-server
// are refused (they serve many threads or another client); the npm
// launcher targets its native codex child from table first, then itself
// (the child holds the session; the launcher exits with it), and is
// refused when no child is in the table; the CLI and exec are the base
// plan (SIGTERM, forced on windows). Resume is `codex resume <id>` when s
// names the conversation. cost: Pure.
func (a *Agent) StopPlan(p agentio.Process, table []agentio.Process, s agentio.ProcessSession) (agentio.StopPlan, error) {
	info, ok := a.ClassifyProcess(p)
	if !ok {
		return agentio.StopPlan{}, agentio.NotThisAgentProcess(ID, p)
	}
	switch info.Kind {
	case agentio.ProcessAppServer:
		return agentio.StopPlan{Agent: ID, Refuse: refuseAppServer}, nil
	case agentio.ProcessMCPServer:
		return agentio.StopPlan{Agent: ID, Refuse: refuseMCPServer}, nil
	case agentio.ProcessLauncher:
		child, ok := a.nativeChild(p, table)
		if !ok {
			return agentio.StopPlan{Agent: ID, Refuse: refuseLauncherNoChild}, nil
		}
		plan := a.core.BaseStopPlan(agentio.ProcessInfo{Agent: ID, Kind: agentio.ProcessCLI, Session: true}, child, s)
		plan.Targets = []int{child.PID, p.PID}
		return plan, nil
	}
	return a.core.BaseStopPlan(info, p, s), nil
}

// nativeChild finds the launcher's native Codex child in table: a row
// whose parent is the launcher and that classifies as Codex but not as
// another launcher. cost: Pure.
func (a *Agent) nativeChild(launcher agentio.Process, table []agentio.Process) (agentio.Process, bool) {
	for _, c := range table {
		if c.PPID != launcher.PID || c.PID == launcher.PID {
			continue
		}
		if info, ok := a.ClassifyProcess(c); ok && info.Kind != agentio.ProcessLauncher {
			return c, true
		}
	}
	return agentio.Process{}, false
}
