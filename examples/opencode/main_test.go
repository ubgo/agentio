package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// Fixture paths relative to this package dir.
const (
	testFixture = "../" + defaultFixture
	testSchema  = "../" + defaultSchema
)

// TestRun runs every section on a fixture copy and checks one line of each.
func TestRun(t *testing.T) {
	// The fake install is an npm bin symlink to bin/opencode with an exec
	// bit; on Windows npm writes opencode.cmd instead.
	exampletest.RequirePOSIXLayout(t)
	h, err := setup(t.TempDir(), testFixture, testSchema)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), &out, h); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(strings.ReplaceAll(out.String(), h.UserHome, "~"))
	for _, want := range []string{
		"install: ~/npm/bin/opencode -> ~/npm/lib/node_modules/opencode-ai/bin/opencode method=npm version=1.18.30 tested=1.18.30..1.18.30\n",
		"home: personal default=true dir=~/.local/share/opencode\n",
		"  database=~/.local/share/opencode/opencode.db config=~/.config/opencode\n",
		`session ses_0f0000000001ffeFixtureAAAA "Fix the login form" dir=/home/alice/proj v0.14.7`,
		`  msg_0f0000000002AAAAAAAAAAAA02 assistant "I fixed the login form." in=1200 cached=1000 out=300 reasoning=120`,
		"  cursor=4, re-read from it: cursor=4\n",
		`session ses_0f0000000100ffeExampleDB01 "Add a health check" dir=/work/app v1.18.30 agent=build cost=$0.04 in=2300 out=410`,
		`  msg_e2 assistant interim=true model=openai/gpt-5 "Reading the router first."`,
		"  cursor=3 (msg_e4 is still streaming)\n",
		`new: opencode --model openai/gpt-5 --prompt "fix the failing tests"`,
		"resume: opencode --session ses_0f0000000001ffeFixtureAAAA\n",
		`headless full: opencode run --format json --auto "run the tests"`,
		"ask: agentio: unsupported: opencode: autonomy ask:",
		"pid 104: opencode=true kind=launcher session=false\n",
		"stop 101: targets=[101] signal=terminate grace=4s resume=opencode --session ses_0f0000000001ffeFixtureAAAA\n",
		"stop 103: refused: not a conversation process: app-server\n",
		"version-tested  ok    1.18.30 is tested\n",
		"lacks hooks: opencode has no shell-command hooks",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
