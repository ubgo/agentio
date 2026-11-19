package pi

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Process-session constants.
const (
	// flagContinueShort is -c, the short spelling of --continue.
	flagContinueShort = "-c"
	// sessionStartTolerance absorbs darwin's whole-second process start
	// times when a session header's timestamp is compared with it.
	sessionStartTolerance = 2 * time.Second
)

// ProcessSession joins a pi process row to its session file. pi's process
// title overwrites argv on linux and macOS, so argv is read only where it
// survives (windows rows run node with the package's cli.js):
//   - argv: `--session <path>` (a .jsonl file) or `--session <id>` /
//     `--session-id <id>`, in the home the process's PI_CODING_AGENT_DIR
//     names (every discovered home when the env is unreadable); JoinArgv;
//   - `--continue` / `-c`: the cwd's session written most recently;
//     JoinInferred;
//   - inferred: the newest session of the cwd whose header timestamp is
//     at or after the process start; JoinInferred, a guess never to act
//     on;
//   - otherwise the home alone (JoinUnknown) when the env names it.
//
// The session store follows PI_CODING_AGENT_SESSION_DIR from the process
// env when readable, else from Host.Env and settings (SessionStore).
// LastActivity is the session file's mtime. cost: Process · Read (start
// env, cwd) + Disk · Read (settings, the cwd's session folder: a head and
// tail per file).
func (a *Agent) ProcessSession(ctx context.Context, p agentio.Process) (agentio.ProcessSession, bool, error) {
	if _, ok := a.ClassifyProcess(p); !ok {
		return agentio.ProcessSession{}, false, nil
	}
	envHome, envErr := a.core.ProcessHome(ctx, p.PID)
	var homes []Home
	if envErr == nil {
		homes = []Home{wrap(envHome)}
	} else {
		hs, err := a.Homes()
		if err != nil {
			return agentio.ProcessSession{}, false, err
		}
		homes = hs
	}
	env := a.processEnv(ctx, p.PID)
	base := agentio.ProcessSession{Agent: ID, PID: p.PID, Dir: a.processCwd(ctx, p)}
	if envErr == nil {
		base.Home = envHome
	}
	args := scriptArgs(p)
	if v, ok := agentio.ArgValue(append([]string{""}, args...), flagSession, flagSessionID); ok && v != "" {
		base.Confidence = agentio.JoinArgv
		for _, h := range homes {
			if s, ok := a.sessionByArg(h, env, v); ok {
				return withSession(base, s), true, nil
			}
		}
		if !strings.HasSuffix(v, sessionExt) {
			base.ConversationID = v
		}
		if base.Home.IsZero() {
			base.Home = a.DefaultHome().Neutral()
		}
		return base, true, nil
	}
	if base.Dir != "" {
		var pick func(Session) bool
		switch {
		case slices.Contains(args, flagContinue) || slices.Contains(args, flagContinueShort):
			pick = func(Session) bool { return true }
		case !p.Start.IsZero():
			floor := p.Start.Add(-sessionStartTolerance)
			pick = func(s Session) bool { return !s.Timestamp.Before(floor) }
		}
		if pick != nil {
			for _, h := range homes {
				if s, ok := a.newestSession(h, env, base.Dir, pick); ok {
					out := withSession(base, s)
					out.Confidence = agentio.JoinInferred
					return out, true, nil
				}
			}
		}
	}
	if envErr == nil {
		return base, true, nil
	}
	return agentio.ProcessSession{}, false, nil
}

// withSession fills ps from a session file. cost: Pure.
func withSession(ps agentio.ProcessSession, s Session) agentio.ProcessSession {
	ps.Home, ps.ConversationID, ps.Path, ps.LastActivity = s.Home.Neutral(), s.ID, s.Path, s.Updated
	if s.Cwd != "" {
		ps.Dir = s.Cwd
	}
	return ps
}

// scriptArgs returns pi's own arguments: after the script for a row that
// runs the package's cli.js through node, else after argv[0]. cost: Pure.
func scriptArgs(p agentio.Process) []string {
	m := Spec().Process
	if i, ok := m.ScriptIndex(p); ok && !m.MatchesBasename(p) {
		return p.Argv[i+1:]
	}
	return p.Argv[min(1, len(p.Argv)):]
}

// sessionByArg resolves a --session value in h: a .jsonl path is read
// directly (relative to nothing: pi resolves it against its own cwd, so
// only an absolute path is trusted), an id is found by file name in the
// store. cost: Disk · Read.
func (a *Agent) sessionByArg(h Home, env []string, v string) (Session, bool) {
	if strings.HasSuffix(v, sessionExt) {
		if !filepath.IsAbs(v) {
			return Session{}, false
		}
		s, err := a.ReadSession(h, v)
		return s, err == nil
	}
	store, err := a.processStore(h, env)
	if err != nil {
		return Session{}, false
	}
	files, err := sessionFiles(store, "")
	if err != nil {
		return Session{}, false
	}
	for _, f := range files {
		if strings.HasSuffix(filepath.Base(f), fileIDSep+v+sessionExt) {
			if s, err := a.ReadSession(h, f); err == nil && s.ID == v {
				return s, true
			}
		}
	}
	return Session{}, false
}

// newestSession returns the most recently modified session of dir in h
// that pick accepts. cost: Disk · Read (one folder; a head and tail per
// file).
func (a *Agent) newestSession(h Home, env []string, dir string, pick func(Session) bool) (Session, bool) {
	store, err := a.processStore(h, env)
	if err != nil {
		return Session{}, false
	}
	abs, err := absDir(dir)
	if err != nil {
		return Session{}, false
	}
	files, err := sessionFiles(store, abs)
	if err != nil {
		return Session{}, false
	}
	var best Session
	found := false
	for _, f := range files {
		s, err := a.ReadSession(h, f)
		if err != nil || (store.Flat && filepath.Clean(s.Cwd) != abs) || !pick(s) {
			continue
		}
		if !found || s.Updated.After(best.Updated) {
			best, found = s, true
		}
	}
	return best, found
}

// processStore is h's session store, with PI_CODING_AGENT_SESSION_DIR
// taken from the process env when it sets an absolute one. cost: Disk ·
// Read (settings.json).
func (a *Agent) processStore(h Home, env []string) (SessionStore, error) {
	if v, ok := agentio.EnvLookup(env, SessionDirEnv); ok && v != "" {
		if v = a.expandTilde(v); filepath.IsAbs(v) {
			return SessionStore{Root: v, Flat: true}, nil
		}
	}
	return a.SessionStore(h)
}

// processEnv is pid's start env, nil when unreadable. cost: Process · Read.
func (a *Agent) processEnv(ctx context.Context, pid int) []string {
	r, ok := a.host.Processes.(agentio.ProcessEnvReader)
	if !ok {
		return nil
	}
	env, err := r.Env(ctx, pid)
	if err != nil {
		return nil
	}
	return env
}

// processCwd is the process's working directory: p.Cwd when the caller
// filled it, else one lookup; "" when unreadable. cost: Process · Read
// (none when p.Cwd is set).
func (a *Agent) processCwd(ctx context.Context, p agentio.Process) string {
	cwd, err := a.host.ProcessCwd(ctx, p)
	if err != nil {
		return ""
	}
	return cwd
}

// StopPlan is the base plan for a pi session: SIGTERM (forced on
// windows), DefaultStopGrace, and `pi --session <id>` as Resume when s
// names the session. pi has no daemon or server mode to refuse.
// cost: Pure.
func (a *Agent) StopPlan(p agentio.Process, _ []agentio.Process, s agentio.ProcessSession) (agentio.StopPlan, error) {
	info, ok := a.ClassifyProcess(p)
	if !ok {
		return agentio.StopPlan{}, agentio.NotThisAgentProcess(ID, p)
	}
	return a.core.BaseStopPlan(info, p, s), nil
}
