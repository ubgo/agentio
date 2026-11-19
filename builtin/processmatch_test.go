package builtin_test

import (
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/builtin"
)

// darwinRow builds a process row the way the darwin reader does from one
// `ps` command column: Argv split on whitespace (a path with a space is
// cut in two), Exe = argv[0], CommandLine the raw string. Pure.
func darwinRow(command string) agentio.Process {
	argv := strings.Fields(command)
	p := agentio.Process{PID: 100, PPID: 1, Argv: argv, CommandLine: command}
	if len(argv) > 0 {
		p.Exe = argv[0]
	}
	return p
}

// TestProcessMatchParity runs agentop's TestClassify table and real `ps`
// command columns (users and paths replaced) through every built-in
// classifier: each row is claimed by the agent named, or by none ("").
// Session says whether the claimed row is a conversation process.
func TestProcessMatchParity(t *testing.T) {
	t.Parallel()
	agents, err := builtin.New(agenttest.NewHost(t))
	if err != nil {
		t.Fatal(err)
	}
	all := agents.All()
	for _, tc := range []struct {
		name    string
		command string
		want    agentio.ID
		session bool
	}{
		// agentop internal/agent TestClassify.
		{"claude launcher", "/Users/x/.local/bin/claude --resume abc", "claude", true},
		{"claude bin path infix", "/usr/bin/node /opt/bin/claude --foo", "claude", true},
		{"claude desktop app excluded", "/Applications/Claude.app/Contents/MacOS/Claude", "", false},
		{"claude-in-chrome excluded", "/x/claude-in-chrome/server", "", false},
		{"codex launcher", "/opt/homebrew/bin/codex", "codex", true},
		{"codex via node pkg path", "node /opt/homebrew/lib/node_modules/@openai/codex/bin/codex.js", "codex", false},
		{"unrelated process", "/usr/bin/vim main.go", "", false},
		{"empty", "", "", false},

		// Real rows from a macOS machine.
		{"runbay claude session", "claude --dangerously-skip-permissions --chrome --remote-control --resume 0f79d084-6061-439e-b571-3668de33ed34", "claude", true},
		{"claude resume first", "claude --resume 5db26401-1f22-49eb-ab3f-447f44409c34 --dangerously-skip-permissions", "claude", true},
		{"claude mcp serve", "claude mcp serve", "claude", false},
		{"desktop chrome native host", "/Applications/Claude.app/Contents/Helpers/chrome-native-host chrome-extension://abcdefghijklmnop/", "", false},
		{"desktop renderer helper", "/Applications/Claude.app/Contents/Frameworks/Claude Helper (Renderer).app/Contents/MacOS/Claude Helper (Renderer) --type=renderer", "", false},
		{"desktop bundled claude code", "/Users/alice/Library/Application Support/Claude/claude-code/2.1.300/claude --output-format stream-json", "", false},
		{"claude-tunnel is another tool", "/Library/Frameworks/Python.framework/Versions/3.13/Resources/Python.app/Contents/MacOS/Python /Users/alice/.local/bin/claude-tunnel serve", "", false},
		{"runbay app", "/Applications/Runbay.app/Contents/MacOS/runbay", "", false},
		{"runbay daemon", "/Applications/Runbay.app/Contents/MacOS/runbay-daemon serve", "", false},
		{"runbay pty daemon", "/Users/alice/.runbay/bin/runbay-ptyd-0.1.0 ptyd --daemon", "", false},
		{"cmux", "/Applications/cmux.app/Contents/MacOS/cmux", "", false},
		{"emdash", "/Applications/Emdash.app/Contents/MacOS/Emdash", "", false},
		{"claude shell snapshot", "/bin/zsh -c source /Users/alice/.claude/shell-snapshots/snapshot-zsh-1.sh 2>/dev/null || true", "", false},
		{"node runs claude npm cli", "node /Users/alice/.npm-global/lib/node_modules/@anthropic-ai/claude-code/cli.js --resume abc", "claude", true},
		{"bun runs claude", "bun /Users/alice/.bun/bin/claude", "claude", true},
		{"npm installs claude", "node /opt/homebrew/bin/npm install -g @anthropic-ai/claude-code", "", false},
		{"node runs an unrelated cli.js", "node /Users/alice/src/app/cli.js", "", false},
		{"npm installs codex", "node /opt/homebrew/bin/npm install -g @openai/codex", "", false},
		{"codex native child", "/opt/homebrew/lib/node_modules/@openai/codex/node_modules/@openai/codex-darwin-arm64/vendor/aarch64-apple-darwin/codex/codex resume 0199a213-81c0-7800-8aa1-bbab2a035a53", "codex", true},
		{"codex app-server", "codex app-server", "codex", false},
		{"gemini relaunch child", "node --max-old-space-size=8192 /opt/homebrew/bin/gemini --resume latest", "gemini", true},
		{"gemini bundle", "node /opt/homebrew/lib/node_modules/@google/gemini-cli/bundle/gemini.js", "gemini", true},
		{"gemini management", "gemini mcp list", "gemini", false},
		{"pi title row", "pi", "pi", true},
		{"pi digits program", "/usr/bin/pi 1000", "", false},
		{"pip is not pi", "/usr/bin/pip install x", "", false},
		{"pi as an argument", "/usr/bin/vim pi", "", false},
		{"pi npm cli", "node /Users/alice/.npm-global/lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js --session abc", "pi", true},
	} {
		info, ok := agentio.ClassifyProcess(all, darwinRow(tc.command))
		got := agentio.ID("")
		if ok {
			got = info.Agent
		}
		if got != tc.want || (ok && info.Session != tc.session) {
			t.Errorf("%s: %q = %q session %v, want %q session %v", tc.name, tc.command, got, info.Session, tc.want, tc.session)
		}
	}
}

// TestProcessMatchWindowsRows checks rows read from Win32_Process: the
// native binary with an .exe name and an npm install run by node.exe
// with backslash paths.
func TestProcessMatchWindowsRows(t *testing.T) {
	t.Parallel()
	agents, err := builtin.New(agenttest.NewHost(t))
	if err != nil {
		t.Fatal(err)
	}
	all := agents.All()
	for _, tc := range []struct {
		argv []string
		want agentio.ID
	}{
		{[]string{`C:\Users\alice\.local\bin\claude.exe`, "--resume", "abc"}, "claude"},
		{[]string{`C:\Program Files\nodejs\node.exe`, `C:\Users\alice\AppData\Roaming\npm\node_modules\@anthropic-ai\claude-code\cli.js`}, "claude"},
		{[]string{`C:\Program Files\nodejs\node.exe`, `C:\Users\alice\AppData\Roaming\npm\node_modules\@openai\codex\bin\codex.js`}, "codex"},
		{[]string{`C:\Program Files\nodejs\node.exe`, `C:\Users\alice\AppData\Roaming\npm\node_modules\@earendil-works\pi-coding-agent\dist\bundle\cli.js`}, "pi"},
		{[]string{`C:\Program Files\nodejs\node.exe`, `C:\Users\alice\AppData\Roaming\npm\node_modules\npm\bin\npm-cli.js`, "install", "-g", "@openai/codex"}, ""},
	} {
		p := agentio.Process{PID: 5, Argv: tc.argv, Exe: tc.argv[0], CommandLine: strings.Join(tc.argv, " ")}
		info, ok := agentio.ClassifyProcess(all, p)
		got := agentio.ID("")
		if ok {
			got = info.Agent
		}
		if got != tc.want {
			t.Errorf("%q = %q, want %q", tc.argv, got, tc.want)
		}
	}
}
