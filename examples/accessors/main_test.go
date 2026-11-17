package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// TestRun runs the whole walk on the fixture machine and checks one line of
// every guide step, so a library change that alters an answer fails here.
func TestRun(t *testing.T) {
	exampletest.RequirePOSIXLayout(t)
	h, err := setup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), &out, h); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"claude personal ~/.claude (cached usage 6%)",
		"codex personal ~/.codex (cached usage 12%)",
		"claude: 27 capabilities",
		"codex: 23 capabilities",
		"claude lacks models: Claude Code keeps no model catalog",
		"codex lacks history-mover: Codex indexes conversations in its own SQLite database",
		"binary-version       process read (may touch login)",
		"token-prober         quota read",
		"claude: /opt/fake/bin/claude -> /opt/fake/.local/share/claude/versions/2.1.325 method=native version=2.1.325",
		"codex: /opt/fake/bin/codex -> /opt/fake/lib/node_modules/@openai/codex/bin/codex.js method=npm version=0.162.1",
		"claude personal: logged-in=true email=[redacted] plan=\"Max (20x)\" billing=subscription credential=file expired=false",
		"codex personal: logged-in=true email=[redacted] plan=\"Plus\" billing=subscription credential=file expired=true",
		"claude trust=unknown model=\"sonnet\" autonomy=full",
		"claude fixture-stdio  stdio scope=user    fixture-mcp env=[FIXTURE_TOKEN]",
		"claude project  project      25 bytes ~/project/CLAUDE.md",
		"codex skill  pr-review      scope=plugin  plugin=\"github\"",
		"codex model gpt-6-luna       visible=true  context=272000",
		"claude work warn  login           no login found in the credentials file or keychain",
		"claude warn 2.0.0 is untested (tested 2.1.323..2.1.325)",
		"aider: no such agent",
		"aider (Aider)",
		"refused: builtin: agent id \"codex\" is already defined: agentio: invalid spec",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Contains(got, "fixture-secret") {
		t.Error("an MCP env value was printed")
	}
}
