package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// TestRun checks the resource columns, the join confidences, each stop
// plan (refusals, the launcher's child, the background stop command), the
// parent walk and the start-time re-check.
func TestRun(t *testing.T) {
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
		`pid 4242 claude cli session=true rss=512MiB tty=ttys004 cpu=1h12m6s cmd="claude"`,
		"pid 4242: claude home personal conversation 6d50c0ec confidence exact status busy",
		"stop: targets [4242], terminate then wait 4s, force true, resume: cd /work/proj4 && claude --resume 6d50c0ec-647e-18aa-84c7-04170288be89",
		"run instead: claude stop aaaaaaaa-0000-4000-8000-000000000045",
		"stop: refused: claude mcp serve is an MCP server for another client, not a conversation",
		"pid 6000: codex launcher, no session of its own",
		"stop: targets [6001 6000], terminate then wait 4s, force true",
		"pid 6001: codex home personal conversation 0199e1a0 confidence argv",
		"stop: refused: codex app-server serves many threads",
		"pid 4242 <- 200 -zsh",
		"pid 4242 <- 100 /Applications/Terminal.app/Contents/MacOS/Terminal",
		": alive, signal",
		": ended, skip",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
