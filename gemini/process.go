package gemini

import (
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
)

// managementCommands are the subcommands that manage config and exit
// (gemini --help, 0.68.0): not a conversation.
func managementCommands() []string {
	return []string{"mcp", "extensions", "extension", "skills", "skill", "hooks", "hook", "gemma", "models"}
}

// ClassifyProcess recognises a Gemini CLI process. It runs as node with
// the bin path as argv[1]: a path inside @google/gemini-cli (the bundle,
// a Homebrew libexec) or the npm shim as invoked (.../bin/gemini), or as
// the standalone binary named gemini. Kind is exec for a headless run
// (-p/--prompt), launcher for a management subcommand (mcp, extensions,
// ...), cli otherwise.
//
// Every session is two processes: the parent relaunches itself as a child
// (with GEMINI_CLI_NO_RELAUNCH=true and, when it raises the heap,
// --max-old-space-size). Both match; a caller counting sessions keeps the
// child (the row whose parent is also a match). cost: Pure.
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
	if len(p.Argv) == 0 {
		return nil, false
	}
	if isGeminiBase(p.Argv[0]) {
		return p.Argv[1:], true
	}
	if !strings.HasPrefix(baseName(p.Argv[0]), nodeBase) && !strings.HasPrefix(baseName(p.Exe), nodeBase) {
		return nil, false
	}
	for i, arg := range p.Argv[1:] {
		if strings.HasPrefix(arg, "-") {
			continue // node options such as --max-old-space-size
		}
		if strings.Contains(filepath.ToSlash(arg), argvPackage) || isGeminiBase(arg) {
			return p.Argv[i+2:], true
		}
		return nil, false // another script
	}
	return nil, false
}

// isGeminiBase reports a path whose base name is the gemini binary or shim
// (gemini, gemini.cmd, gemini.exe). cost: Pure.
func isGeminiBase(p string) bool {
	b := strings.ToLower(baseName(p))
	return strings.TrimSuffix(b, path.Ext(b)) == Binary
}

// baseName is the last element of a slash or backslash path. cost: Pure.
func baseName(p string) string {
	return path.Base(strings.ReplaceAll(p, `\`, "/"))
}

// hasPrefixArg reports an arg starting with prefix. cost: Pure.
func hasPrefixArg(args []string, prefix string) bool {
	return slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, prefix) })
}
