package gemini

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/ubgo/agentio"
)

// Process-session constants.
const (
	// flagResumeShort is -r, the short spelling of --resume.
	flagResumeShort = "-r"
	// chatStartTolerance absorbs darwin's whole-second process start
	// times when a chat's startTime is compared with the process start.
	chatStartTolerance = 2 * time.Second
)

// ProcessSession joins a Gemini CLI process row to its chat. Only the
// relaunch child is the session (the parent waits for it; found false for
// the parent, told from the process table), and a management subcommand
// is not one.
//   - argv: `--resume <uuid>` / `-r <uuid>`, the chat found in the cwd's
//     project of the home the process's GEMINI_CLI_HOME names (every
//     discovered home when the env is unreadable); JoinArgv;
//   - `--resume latest`, an index or a bare --resume: the chat of the
//     cwd's project written most recently; JoinInferred;
//   - inferred: the newest chat of the cwd's project that started at or
//     after the process; JoinInferred, a guess never to act on;
//   - otherwise the home alone (JoinUnknown) when the env names it.
//
// LastActivity is the chat file's mtime. cost: Process · Read (one
// process-table read, the start env, the cwd) + Disk · Read (projects.json
// and a head and tail of each chat of one project).
func (a *Agent) ProcessSession(ctx context.Context, p agentio.Process) (agentio.ProcessSession, bool, error) {
	info, ok := a.ClassifyProcess(p)
	if !ok || !info.Session {
		return agentio.ProcessSession{}, false, nil
	}
	if _, isParent := a.relaunchChild(p, a.processTable(ctx)); isParent {
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
	base := agentio.ProcessSession{Agent: ID, PID: p.PID, Dir: a.processCwd(ctx, p.PID)}
	if envErr == nil {
		base.Home = envHome
	}
	resume, hasResume := agentio.ArgValue(p.Argv, flagResume, flagResumeShort)
	if !hasResume && hasArg(p.Argv, flagResume, flagResumeShort) {
		hasResume = true // a bare --resume as the last argument: latest
	}
	byID := hasResume && agentio.IsUUID(resume)
	if byID {
		base.ConversationID, base.Confidence = resume, agentio.JoinArgv
	}
	if base.Dir != "" {
		var pick func(chat) bool
		switch {
		case byID:
			pick = func(c chat) bool { return c.s.ID == resume }
		case hasResume:
			pick = func(chat) bool { return true }
		case !p.Start.IsZero():
			floor := p.Start.Add(-chatStartTolerance)
			pick = func(c chat) bool { return !c.s.Start.Before(floor) }
		}
		if pick != nil {
			for _, h := range homes {
				if c, ok := a.newestChat(h, base.Dir, pick); ok {
					base.Home, base.ConversationID, base.Path, base.LastActivity = h.Neutral(), c.s.ID, c.s.Path, c.mod
					if !byID {
						base.Confidence = agentio.JoinInferred
					}
					return base, true, nil
				}
			}
		}
	}
	if byID {
		if base.Home.IsZero() {
			base.Home = a.DefaultHome().Neutral()
		}
		return base, true, nil
	}
	if envErr == nil {
		return base, true, nil
	}
	return agentio.ProcessSession{}, false, nil
}

// hasArg reports whether any of flags is an argument after argv[0].
// cost: Pure.
func hasArg(argv []string, flags ...string) bool {
	for _, a := range argv[min(1, len(argv)):] {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
}

// chat is one main chat file with its mtime.
type chat struct {
	s   Session
	mod time.Time
}

// newestChat returns the most recently modified main chat of dir's
// project in h that pick accepts. cost: Disk · Read (projects.json, the
// chats dir, and a head and tail per chat file).
func (a *Agent) newestChat(h Home, dir string, pick func(chat) bool) (chat, bool) {
	slug, ok, err := a.ProjectSlug(h, dir)
	if err != nil || !ok {
		return chat{}, false
	}
	chats := filepath.Join(h.ProjectTmpDir(slug), chatsDir)
	entries, err := os.ReadDir(chats)
	if err != nil {
		return chat{}, false
	}
	var best chat
	found := false
	for _, e := range entries {
		if e.IsDir() || !isSessionFile(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		s, err := ReadSession(filepath.Join(chats, e.Name()))
		if err != nil || s.ID == "" || s.Kind == SessionSubagent {
			continue
		}
		c := chat{s: s, mod: fi.ModTime()}
		if pick(c) && (!found || c.mod.After(best.mod)) {
			best, found = c, true
		}
	}
	return best, found
}

// processTable reads Host.Processes once; nil when the host has none or
// the read fails. cost: Process · Read.
func (a *Agent) processTable(ctx context.Context) []agentio.Process {
	if a.host.Processes == nil {
		return nil
	}
	rows, err := a.host.Processes.Processes(ctx)
	if err != nil {
		return nil
	}
	return rows
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

// RelaunchChild returns the Gemini CLI child p relaunched itself as: a
// row of table whose parent is p and that is also Gemini CLI. ok is false
// when p has none (p is the child itself, or ran without relaunching).
// cost: Pure.
func (a *Agent) RelaunchChild(p agentio.Process, table []agentio.Process) (agentio.Process, bool) {
	return a.relaunchChild(p, table)
}

// relaunchChild is RelaunchChild. cost: Pure.
func (a *Agent) relaunchChild(p agentio.Process, table []agentio.Process) (agentio.Process, bool) {
	for _, c := range table {
		if c.PPID == p.PID && c.PID != p.PID {
			if _, ok := a.ClassifyProcess(c); ok {
				return c, true
			}
		}
	}
	return agentio.Process{}, false
}

// relaunchParent returns p's parent in table when it is Gemini CLI (p is
// the relaunch child). cost: Pure.
func (a *Agent) relaunchParent(p agentio.Process, table []agentio.Process) (agentio.Process, bool) {
	for _, r := range table {
		if r.PID == p.PPID && r.PID != p.PID {
			if _, ok := a.ClassifyProcess(r); ok {
				return r, true
			}
			return agentio.Process{}, false
		}
	}
	return agentio.Process{}, false
}

// StopPlan plans how to stop a Gemini CLI session: the relaunch child
// (which holds the chat) first, then its relaunch parent, whichever of
// the two p is; a management subcommand is refused. SIGTERM (forced on
// windows), DefaultStopGrace; Resume is `gemini --resume <id>` in the
// project dir when s names the chat. cost: Pure.
func (a *Agent) StopPlan(p agentio.Process, table []agentio.Process, s agentio.ProcessSession) (agentio.StopPlan, error) {
	info, ok := a.ClassifyProcess(p)
	if !ok {
		return agentio.StopPlan{}, agentio.NotThisAgentProcess(ID, p)
	}
	plan := a.core.BaseStopPlan(info, p, s)
	if plan.Refuse != "" {
		return plan, nil
	}
	if child, ok := a.relaunchChild(p, table); ok {
		plan.Targets = []int{child.PID, p.PID}
	} else if parent, ok := a.relaunchParent(p, table); ok {
		plan.Targets = []int{p.PID, parent.PID}
	}
	return plan, nil
}
