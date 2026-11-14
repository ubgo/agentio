package main

import (
	"bytes"
	"strings"
	"testing"
)

// Fixture paths relative to this package dir.
const (
	testFixture = "../../claude/testdata/homes/2.1.325/userhome"
	testResult  = "../../claude/testdata/stream/2.1.325/result.json"
)

// TestRun runs the example on the fixture and checks one line per section.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, testFixture, testResult); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`proj-db       scope=project transport=stdio approval="approved" env=map[DB_PASSWORD:[redacted]]`,
		"ParseMCPConfig: docs transport=http oauth client=docs-cli port=8765",
		"fixture-http needs auth since 2026-11-06T21:20:00Z",
		`add:    claude mcp add-json -s user docs`,
		"server with env values refused:",
		"user-rule    ~/.claude/rules/style.md size=44 paths=[src/**/*.go] imports=0",
		"auto MEMORY.md size=40",
		"line 5 raw=~/notes.md",
		"CLAUDE.md imports ~/missing-file.md -> ~/missing-file.md exists=false",
		`plug-skill scope=plugin  plugin="demo-plugin@claude-plugins-official"`,
		"/deploy scope=plugin",
		"/triage scope=project",
		"Split+ParseYAML: tools kind=scalar text=\"Read, Grep\" List=[Read Grep]",
		"(plugin=demo-plugin marketplace=claude-plugins-official) v1.0.0 scope=user enabled=false",
		"blocklist fetched 2026-11-13T10:00:00Z entries=[]",
		"unknown=[homepage]",
		"claude plugin install demo-plugin@claude-plugins-official -s project",
		"reviewer scope=user model=sonnet mode=plan",
		"agent-a1 type=Explore builtin=true depth=1 mode=manual",
		`--agents {"tester":`,
		"Teacher  scope=project keep coding instructions=unset",
		"session=s1 file hash=abc123 version=2 size=18",
		"shell=zsh created=2026-11-06T21:20:00Z",
		"ide=VS Code port=41234 pid=5 workspace=[/work/project] transport=ws unknown=[authToken]",
		"debug session=s1 size=10 latest=true",
		"foreign entry notes-by-another-tool.txt (21 bytes)",
		"spawned=0 completed=0 failed=0 max depth=0",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q", want)
		}
	}
	for _, secret := range []string{"fixture-secret", "hunter2"} {
		if strings.Contains(out.String(), secret) {
			t.Errorf("secret %q printed", secret)
		}
	}
}
