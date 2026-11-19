package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// TestRun runs every step on fixture copies and checks one line per guide
// step, including the history move landing on disk.
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
		`claude personal 3f474440-903e-5410-8bfb-d537b89f9547 /work/proj13 "Fixture title 166"`,
		`codex personal 0199e1a0-0000-7000-8000-000000000001 /work/proj "List the files"`,
		"claude: 12 turns, offset 46441, tokens in=24 cached=1186928 out=4198",
		"claude: 0 new turns after offset 46441",
		"codex: 2 turns, offset 4236, tokens in=1200 cached=1000 out=80",
		"claude owner: home=personal conversation=3f474440-903e-5410-8bfb-d537b89f9547",
		"claude: 11 answers, 0 interim, cursor 46441",
		`codex assistant id="i2" interim=false`,
		"codex: 1 answers, 0 interim, cursor 4236",
		`claude 2026-11-13 19:35 dir="/work/proj13" "fixture text 139"`,
		`codex 2026-10-15 10:01 dir="" "and the sizes"`,
		"claude has history in /work/proj13: true",
		"codex has history in /work/proj: true",
		"claude moved /work/proj13 -> /work/proj13-renamed, backup ~/.claude/projects/-work-proj13.agentio-bak-20261113T120000Z",
		"claude plan personal -> work: 1 conversations, 1 items, blockers []",
		"codex cannot move history: Codex indexes conversations",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}
