package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// testUserHome is the fixture user home relative to this package dir.
const testUserHome = "../../claude/testdata/homes/2.1.325/userhome"

func TestRun(t *testing.T) {
	exampletest.RequireSymlinks(t)
	var out bytes.Buffer
	if err := run(&out, testUserHome); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"backup: ~/.claude/settings.json -> ~/.claude/settings.json.agentio-bak-20261114T120000Z\n",
		"backup: ~/.claude/settings.json -> ~/.claude/settings.json.agentio-bak-20261114T120000Z-2\n",
		"backup of a missing file: empty=true\n",
		// The project dir is a host path: the plan cleans it with the host's
		// filepath, so on Windows it prints as \work\proj12. The Claude
		// history key (-work-proj12) is the same on every OS.
		"plan: agent=claude dir=" + filepath.FromSlash(movedDir) + " conversations=[f85efbdb-fc2f-1e8e-7748-9f915edc8e2d]\n",
		"plan: blockers=0 left=0 cleanup=2\n",
		"apply: backed up ~/.claude/projects/-work-proj12/f85efbdb-fc2f-1e8e-7748-9f915edc8e2d.jsonl -> ~/.claude.agentio-bak-20261114T120000Z/projects/-work-proj12/f85efbdb-fc2f-1e8e-7748-9f915edc8e2d.jsonl\n",
		"apply: ~/.claude-work/projects/-work-proj12/f85efbdb-fc2f-1e8e-7748-9f915edc8e2d.jsonl present=true\n",
		"already exists in the target home\n",
		"blocked: apply refused=true\n",
		"link plan: settings.json exists=true\n",
		"linked: ~/.claude-work/CLAUDE.md -> ~/.claude/CLAUDE.md\n",
		"link plan again: CLAUDE.md exists=true\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
