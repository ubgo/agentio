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
	exampletest.RequirePOSIXLayout(t)
	var out bytes.Buffer
	if err := run(context.Background(), &out, testFixture, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"target: home personal, version 2.1.325",
		`launch: claude --model opus --permission-mode plan --add-dir /work/shared -- "-explain the build"`,
		"headless args: -p --output-format stream-json --include-partial-messages --verbose",
		"invalid: claude: invalid launch options: --output-format needs -p",
		"fork in work home: claude --resume 3f474440-903e-5410-8bfb-d537b89f9547 --fork-session --session-id 22222222-0000-4000-8000-000000000002\n  CLAUDE_CONFIG_DIR set: true",
		"unavailable: claude: --worktree needs Claude Code >= 2.1.49 (installed 2.1.40)",
		"  warning version-unknown: installed Claude Code version unknown; flags not checked",
		"  warning untested-version: installed 2.1.400 is outside tested 2.1.323..2.1.330",
		"runner version: 2.1.325",
		"runner plugin demo@example-market v1.2.0 (user), enabled true, installed 2026-11-01",
		`background agent ab12 "fix-tests": pid 4242, state blocked, cwd /work/proj4`,
		"check binary-found         ok    <tmp>/bin/claude",
		"check login-expired        warn  login expired at 2026-11-13T20:56:01Z",
		"check auth-status          ok    auth status: logged in (claude.ai)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
