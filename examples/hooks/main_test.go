package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// TestRun runs the example and checks one line per step and agent.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := runMain(&out); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"file ~/.claude/settings.json",
		"file ~/.codex/hooks.json",
		"file ~/.gemini/settings.json",
		"add turn-end -> Stop",
		"add prompt-submit -> BeforeAgent",
		"add turn-end -> AfterAgent",
		"manual: Codex asks you to review new or changed hooks",
		"backup ~/.codex/hooks.json.agentio-bak-20261119T120000Z",
		"decoded prompt-submit (UserPromptSubmit) conv=0b5e3a8e dir=/work/app prompt=\"add a test\" answer=\"\"",
		"decoded turn-end (Stop) conv=019a0000 dir=/work/app prompt=\"\" answer=\"Added the test.\"",
		"decoded turn-end (AfterAgent) conv=7c0e0000",
		"no hook installer: pi has no shell-command hooks",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if n := strings.Count(got, "second install: present=2 noop=true backup empty=true"); n != 3 {
		t.Errorf("idempotent second install shown %d times, want 3", n)
	}
	if n := strings.Count(got, "removed=2 backup=true file restored=true"); n != 3 {
		t.Errorf("byte-for-byte uninstall shown %d times, want 3", n)
	}
}
