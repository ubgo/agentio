package main

import (
	"strings"
	"testing"
)

// TestRun runs the example and checks the lines the compat docs quote.
func TestRun(t *testing.T) {
	var b strings.Builder
	if err := run(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"claude tested 2.1.323..2.1.330",
		"codex  tested 0.162.1..0.162.1",
		`check "2.1.330" -> in-range`,
		`check "2.1.400" -> newer (warning untested-version)`,
		`check "2.1.40"  -> older (warning untested-version)`,
		`check ""        -> unknown (warning version-unknown)`,
		"claude --worktree                   since 2.1.49   Claude Code CHANGELOG.md 2.1.49",
		`--worktree in "2.1.40" : false`,
		`--worktree in ""       : true`,
		"claude launch: claude: --worktree needs Claude Code >= 2.1.49 (installed 2.1.40)",
		"is ErrFeatureUnavailable=true",
		"claude 2.1.400: built, untested warning=true",
		"codex launch: codex 0.150.0: --no-daemon needs codex >= 0.156.0",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
}
