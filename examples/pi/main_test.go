package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// testFixture is the fixture user home relative to this package dir.
const testFixture = "../" + defaultFixture

// TestRun runs every section on a fixture copy and checks one line of each.
func TestRun(t *testing.T) {
	// The fake install is an npm bin symlink to dist/bundle/cli.js with an
	// exec bit; on Windows npm writes pi.cmd instead.
	exampletest.RequirePOSIXLayout(t)
	h, err := setup(t.TempDir(), testFixture)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), &out, h); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(strings.ReplaceAll(out.String(), h.UserHome, "~"))
	for _, want := range []string{
		"install: ~/npm/bin/pi -> ~/npm/lib/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js method=npm version=1.5.2 tested=1.5.2..1.5.2\n",
		"home: personal default=true dir=~/.pi/agent\n",
		`session 0190f0e0 "my name" cwd=/home/fixture/proj model=localmock/mock-1 v3`,
		"  turns: 4 user, 4 assistant\n",
		"  usage: 4 messages, 480 input and 16 output tokens, $0.0000\n",
		"unknown=[compaction hideThinkingBlock] warnings=0\n",
		`round trip: {"compaction":{"enabled":false},"defaultModel":"claude-sonnet","defaultProvider":"anthropic","hideThinkingBlock":true}`,
		"default model: anthropic/claude-sonnet\n",
		`new: pi -- "fix the failing tests"`,
		`read-only headless: pi --mode json --tools read,grep,find,ls --no-mcp --no-extensions -- "review src/"`,
		"token login: pi ANTHROPIC_API_KEY=[redacted] OPENAI_API_KEY kept=true\n",
		"ask: agentio: unsupported: pi: autonomy ask: pi has no permission modes; it always runs without prompts\n",
		"version-tested  ok    1.5.2 is tested\n",
		"login           ok    env OPENAI_API_KEY\n",
		"lacks usage: pi has no plan or quota",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
	if strings.Contains(got, exampleKey) {
		t.Error("the example key was printed")
	}
}
