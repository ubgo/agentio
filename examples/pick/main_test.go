package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// TestRun runs every step against the fake usage API and fixture copies
// and checks the decision, the skips, the launch and the custom policy.
func TestRun(t *testing.T) {
	srv := fakeAPI()
	defer srv.Close()
	h, err := setup(t.TempDir(), srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), &out, h, srv.URL); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(strings.ReplaceAll(out.String(), h.UserHome, "~"))
	for _, want := range []string{
		"auto → claude:work (max 9%) over claude:personal (max 77%)\n",
		"skipped codex:personal (usage-error): codex: access token expired",
		`launch: claude -- "fix the failing tests" (CLAUDE_CONFIG_DIR=~/.claude-work)`,
		"auto → claude:personal (max 6%, cached 16d ago) over codex:personal (max 12%, cached 30d ago)\n",
		"skipped claude:work (usage-error): no cached usage on disk\n",
		"auto [reserve] → claude:work (reserve 9)\n",
		"skipped claude:personal (policy): pick: refused by policy: a window is at 77%, the reserve starts at 50%\n",
		"no candidate: true\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
	if strings.Contains(got, workToken) {
		t.Error("the placeholder token reached the output")
	}
}
