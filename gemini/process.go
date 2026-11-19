package gemini

import (
	"slices"
	"strings"

	"github.com/ubgo/agentio"
)

// managementCommands are the subcommands that manage config and exit
// (gemini --help, 0.68.0): not a conversation.
func managementCommands() []string {
	return []string{"mcp", "extensions", "extension", "skills", "skill", "hooks", "hook", "gemma", "models"}
}

// ClassifyProcess recognises a Gemini CLI process from Spec().Process: the
// standalone binary or shim named gemini (gemini.cmd, gemini.exe), or
// node running the npm shim as invoked (.../bin/gemini) or a path inside
// @google/gemini-cli (the bundle, a Homebrew libexec). Kind is exec for a
// headless run (-p/--prompt), launcher for a management subcommand (mcp,
// extensions, ...), cli otherwise.
//
// Every session is two processes: the parent relaunches itself as a child
// (with GEMINI_CLI_NO_RELAUNCH=true and, when it raises the heap,
// --max-old-space-size). Both match; a caller counting sessions keeps the
// child (the row whose parent is also a match; RelaunchChild). cost: Pure.
func (a *Agent) ClassifyProcess(p agentio.Process) (agentio.ProcessInfo, bool) {
	args, ok := geminiArgs(p)
	if !ok {
		return agentio.ProcessInfo{}, false
	}
	kind := agentio.ProcessCLI
	switch {
	case len(args) > 0 && slices.Contains(managementCommands(), args[0]):
		kind = agentio.ProcessLauncher
	case slices.Contains(args, argPromptShort) || slices.Contains(args, flagPrompt) || hasPrefixArg(args, flagPrompt+"="):
		kind = agentio.ProcessExec
	}
	return agentio.ProcessInfo{Agent: ID, Kind: kind, Session: kind != agentio.ProcessLauncher}, true
}

// geminiArgs returns the CLI's own args (after the script or binary) when
// p is Gemini CLI. cost: Pure.
func geminiArgs(p agentio.Process) ([]string, bool) {
	m := Spec().Process
	if !m.Match(p) || len(p.Argv) == 0 {
		return nil, false
	}
	if i, ok := m.ScriptIndex(p); ok && !m.MatchesBasename(p) {
		return p.Argv[i+1:], true
	}
	return p.Argv[1:], true
}

// hasPrefixArg reports an arg starting with prefix. cost: Pure.
func hasPrefixArg(args []string, prefix string) bool {
	return slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, prefix) })
}
