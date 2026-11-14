package main

import (
	"bytes"
	"strings"
	"testing"
)

// testMigrations is defaultMigrations seen from the package directory,
// where go test runs.
const testMigrations = "../" + defaultMigrations

func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, testMigrations); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"state_5.sqlite at state_5.sqlite schema=59 max-known=59",
		`thread 019a0000-0000-7000-8000-00000000000a "Fix the build" source=vscode archived=false pinned=true tokens=1234 history=paginated sandbox=workspace-write`,
		"source=subagent archived=true",
		"active threads in /work/app: 1",
		`as conversation: "Fix the build" dir=/work/app originator=Codex Desktop branch=main`,
		"unknown thread: ErrNotFound",
		"status=closed nickname=helper role=worker",
		`project p1 "App" roots=[/work/app /work/app-docs]`,
		`turn t2 status=failed duration=10s error={"message":"boom"}`,
		"page 2 item i3 type=userMessage known=true ordinal=10",
		"legacy thread turns: 0",
		`goal "ship it" status=budget_limited used=1200 budget=1000 time=1m30s`,
		"queued q1 order=1",
		"memory job stage1 status=done retries-left=3",
		`WARN codex_core "slow tool call"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Contains(got, "other thread") {
		t.Error("log filter returned another thread's line")
	}
}
