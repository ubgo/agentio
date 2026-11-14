package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// testFixture is the fixture user home relative to this package dir.
const testFixture = "../" + defaultFixture

func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), &out, testFixture, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"stats v5 computed 2026-10-26: 141 sessions, 93795 messages since 2026-10-11",
		"  busiest hour: 17:00 (23 sessions started)",
		`  3f474440  "Fixture title 166" (ai-title) dir /work/proj13, v2.1.325, updated 2026-11-13T12:00:00Z`,
		"  history for /work/never-used: false",
		"turns: 1 user, 11 assistant, offset 47179",
		"  turns after offset 47179: 0",
		"move plan personal -> work for /work/proj13: 1 conversations, 1 items, 0 blockers, 0 left",
		`  prefix 3f47: claude: ambiguous conversation prefix "3f47" matches 2 conversations`,
		"  internal record: claude: internal transcript record is never decoded",
		`tail: 54 records, title "Fixture title 166" (ai-title), last prompt "fixture last prompt"`,
		"cost: $4.21 over 1h0m0s (API 25m0s), +120/-30 lines, 1 models",
		"pr: example/project #42 https://github.com/example/project/pull/42",
		`paste #1 (text, hash true): "a long pasted log line"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}

// TestFixtureUntouched checks the appends went to the copy only.
func TestFixtureUntouched(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(testFixture, ".claude", "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("Pasted text")) {
		t.Fatal("the checked-in fixture was modified")
	}
}
