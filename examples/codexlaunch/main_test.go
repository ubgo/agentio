package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		`new          codex -c model_reasoning_effort="high" -m gpt-5.5 -s workspace-write -a on-request --add-dir /work/shared "fix the build"  [warning version-unknown]`,
		"resume       codex resume 0199e1a0-0000-7000-8000-000000000001\n",
		"resume last  codex resume --last --all",
		"other home   codex   [CODEX_HOME=work-codex]",
		"resume without an id: ErrNoSessionID",
		"bypass without Target.AllowDangerous: ErrDangerousNotAllowed",
		"profile work: ErrProfileConflict",
		`exec         codex exec -s read-only -c approval_policy="never" --json -o /tmp/last.txt --skip-git-repo-check -`,
		`review       codex exec review --base main --title "Release review" --json`,
		"delete       codex delete --force 0199e1a0-0000-7000-8000-000000000001",
		"feature      codex features enable hooks",
		"feature=--no-daemon installed=0.150.0 since=0.156.0 neutral match=true",
		"event item.completed command \"ls\" exit=0",
		"event future.event known=false",
		"run event done reasoning=1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	// The default home must never carry CODEX_HOME.
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "new ") && strings.Contains(line, "CODEX_HOME") {
			t.Errorf("default home sets CODEX_HOME: %s", line)
		}
	}
}
