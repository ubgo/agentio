package opencode

import (
	"context"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
)

// Subcommands by what the process does (opencode --help, 1.18.30).
// serverCommands serve many sessions over HTTP or ACP, attach is a client
// of such a server, and the rest manage config or data and exit.
func serverCommands() []string { return []string{"serve", "web", "acp"} }

// attachCommand is the TUI client of a running server.
const attachCommand = "attach"

// managementCommands are the subcommands that are not a conversation.
func managementCommands() []string {
	return []string{
		"completion", "mcp", "debug", "providers", "auth", "agent", "upgrade", "uninstall", "models",
		"stats", "export", "import", "github", "session", "plugin", "plug", "db",
	}
}

// ClassifyProcess recognises an opencode process. The native binary (every
// install method ends in one) is argv[0] opencode or opencode.exe:
//   - no subcommand (the TUI, optionally with a project dir) is cli, and
//     `opencode pr <n>` (checks out a PR, then runs the TUI) too;
//   - `opencode run` is exec;
//   - serve, web and acp are app-server, attach is a bridge to a server:
//     none holds one conversation, so none is a session;
//   - management subcommands (mcp, providers, session, export, db, ...)
//     are launcher, not a session.
//
// An npm install started through node or bun (`node .../bin/opencode`) is
// the opencode-ai launcher script that starts the native binary as its
// child, so it is a launcher, never the session. cost: Pure.
func (a *Agent) ClassifyProcess(p agentio.Process) (agentio.ProcessInfo, bool) {
	if len(p.Argv) == 0 {
		return agentio.ProcessInfo{}, false
	}
	if !isOpencodeBase(p.Argv[0]) && !isOpencodeBase(p.Exe) {
		// The Spec matcher finds an interpreter running the npm launcher
		// script (ScriptBasenames); the native binary is checked here with
		// both separators, so a Windows path classifies on any host.
		if Spec().Process.Match(p) {
			return agentio.ProcessInfo{Agent: ID, Kind: agentio.ProcessLauncher}, true
		}
		return agentio.ProcessInfo{}, false
	}
	kind := agentio.ProcessCLI
	switch sub := subcommand(p.Argv[1:]); {
	case sub == cmdRun:
		kind = agentio.ProcessExec
	case slices.Contains(serverCommands(), sub):
		kind = agentio.ProcessAppServer
	case sub == attachCommand:
		kind = agentio.ProcessBridge
	case slices.Contains(managementCommands(), sub):
		kind = agentio.ProcessLauncher
	}
	return agentio.ProcessInfo{Agent: ID, Kind: kind, Session: kind == agentio.ProcessCLI || kind == agentio.ProcessExec}, true
}

// subcommand is the first non-flag argument; "" when there is none. A
// flag value (`--model x`) is not a subcommand: flags that take a value
// are skipped with it. A project dir positional is returned too and simply
// matches no subcommand. cost: Pure.
func subcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
		if valueFlag(arg) {
			i++
		}
	}
	return ""
}

// valueFlags are the global and TUI flags that take a separate value
// (opencode --help, 1.18.30).
func valueFlags() []string {
	return []string{"--log-level", "--port", "--hostname", "--mdns-domain", "--cors", flagModel, "-m", flagSession, flagSessionS, flagPrompt, "--agent", "--replay-limit"}
}

// valueFlag reports a flag whose value is the next argument (not given as
// --flag=value). cost: Pure.
func valueFlag(arg string) bool { return slices.Contains(valueFlags(), arg) }

// isOpencodeBase reports a path whose base name is the opencode binary
// (opencode, opencode.exe). cost: Pure.
func isOpencodeBase(p string) bool {
	b := strings.ToLower(path.Base(strings.ReplaceAll(p, `\`, "/")))
	return b == Binary || b == Binary+exeSuffix
}

// argvSession reads the session id a command line names: --session <id>,
// -s <id> or --session=<id>. fork is true when --fork is present, which
// makes opencode continue in a new copy of that session. cost: Pure.
func argvSession(args []string) (id string, fork bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == flagFork:
			fork = true
		case (arg == flagSession || arg == flagSessionS) && i+1 < len(args):
			id = args[i+1]
			i++
		case strings.HasPrefix(arg, flagSession+"="):
			id = strings.TrimPrefix(arg, flagSession+"=")
		}
	}
	return id, fork
}

// ProcessSession joins an opencode session process (the TUI or `run`) to
// what its command line and start environment say:
//   - Home is the data dir the process's own XDG_DATA_HOME selects, read
//     through agentio.ProcessEnvReader (linux, darwin); where the
//     environment is unreadable (windows) it is the data dir the Host env
//     selects, an assumption that holds unless the process was started
//     with another XDG_DATA_HOME;
//   - ConversationID comes from --session/-s (Confidence JoinArgv) unless
//     --fork is given, which makes a new session; --continue and a new
//     session name nothing (JoinUnknown: the id is then only in
//     opencode.db, which opencode/store reads);
//   - Dir is the legacy session's directory, else the process cwd
//     (Process · Read) when Host.Processes can tell; Path and Version are
//     set when the session predates the database (a legacy session file).
//
// found is false for a process that is not a session (a launcher, a
// server). cost: Pure, plus Process · Read (env, cwd) and Disk · Read
// (legacy lookup).
func (a *Agent) ProcessSession(ctx context.Context, p agentio.Process) (agentio.ProcessSession, bool, error) {
	info, ok := a.ClassifyProcess(p)
	if !ok || !info.Session {
		return agentio.ProcessSession{}, false, nil
	}
	h := a.processHome(ctx, p.PID)
	out := agentio.ProcessSession{Agent: ID, PID: p.PID, Home: h.Neutral(), Confidence: agentio.JoinUnknown}
	if id, fork := argvSession(p.Argv[1:]); id != "" && !fork {
		out.ConversationID, out.Confidence = id, agentio.JoinArgv
		if s, err := a.Session(h, id); err == nil {
			out.Path, out.Version, out.Dir = s.Path, s.Version, s.Directory
		}
	}
	if out.Dir == "" && a.host.Processes != nil {
		if cwd, err := a.host.Processes.Cwd(ctx, p.PID); err == nil {
			out.Dir = cwd
		}
	}
	return out, true, nil
}

// processHome is the data dir pid's start environment selects; the Host
// default when that environment is unreadable. Process · Read.
func (a *Agent) processHome(ctx context.Context, pid int) Home {
	r, ok := a.host.Processes.(agentio.ProcessEnvReader)
	if !ok {
		return a.DefaultHome()
	}
	env, err := r.Env(ctx, pid)
	if err != nil {
		return a.DefaultHome()
	}
	base := filepath.Join(a.host.UserHome, defaultDataBase)
	for _, kv := range env {
		if v, found := strings.CutPrefix(kv, XDGDataHomeEnv+"="); found && filepath.IsAbs(v) {
			base = filepath.Clean(v)
		}
	}
	return a.HomeAt(filepath.Join(base, appDir))
}
