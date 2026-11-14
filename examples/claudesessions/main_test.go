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

func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), &out, testFixture, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"session pid 4242: fixture-session-0, status busy (neutral busy), alive true, cwd /work/proj4, v2.1.325",
		"session pid 4243: fixture-session-1, status idle (neutral idle), alive false, cwd /work/proj6, v2.1.325",
		"  conversation 6d50c0ec live: true",
		"pid 4243 python3 worker.py: not Claude Code",
		"pid 5001 claude mcp serve: mcp-server, session false",
		"  claude process 4242: cli, cwd /work/proj4",
		`job ab12 "fix-tests": state blocked, needs "approval", 230135 tokens, 1 children, unknown keys 1`,
		"  command: claude attach ab12",
		"daemon: pid 4242 v2.1.325, running true, auth ok",
		"team review: lead lead, 1 members, lead pid 4242",
		"task list review: item 1.json, 27 raw bytes",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
