package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// testFixture is the fixture user home relative to this package dir.
const testFixture = "../../claude/testdata/homes/2.1.325/userhome"

// TestRun runs the demo on the fixture and checks one line per section.
func TestRun(t *testing.T) {
	exampletest.RequireSymlinks(t)
	var out bytes.Buffer
	if err := run(&out, testFixture); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`user    PreToolUse        matcher="Bash"               handlers=4`,
		"documented events=33, 17 take a matcher",
		`matcher "Write|Edit|MultiEdit" tool NotebookEdit         -> false`,
		"event=PreToolUse tool=Bash mode=manual effort=high unknown=[new_field]",
		`"permissionDecision":"deny"`,
		"setting: type=command command=/usr/local/bin/my-statusline",
		"prints: [Opus] src $0.42 +12 -3",
		"user model=sonnet tui=\"\" theme=dark hooks kept=10 events",
		`invalid JSON value refused: claude: settings value for "model" is not valid JSON`,
		"Stop matcher groups now=2",
		"second run removed=0 backup empty=true",
		"moved=true now at ~/.claude/projects/-work-proj12-renamed",
		"destination exists: ErrHistoryExists=true",
		"plan: 1 conversation(s), 1 item(s), blockers=[]",
		"plan ~/.claude-work/settings.json -> ~/.claude/settings.json exists=true",
		"work CLAUDE.md -> ~/.claude/CLAUDE.md",
	} {
		if !strings.Contains(exampletest.Output(out.String()), want) {
			t.Errorf("output lacks %q", want)
		}
	}
}

// TestHookAllowsSafeCommands checks that the hook stays silent for a
// command it does not block, so Claude Code's own permission flow runs.
func TestHookAllowsSafeCommands(t *testing.T) {
	in := `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"go test ./..."}}`
	var out bytes.Buffer
	if err := hookMain(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Fatalf("hook replied %q", out.String())
	}
}

// TestStatusLineWithoutCost checks the line before the first request.
func TestStatusLineWithoutCost(t *testing.T) {
	in := `{"model":{"display_name":"Sonnet"},"workspace":{"current_dir":"/work/app"}}`
	var out bytes.Buffer
	if err := statusLineMain(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "[Sonnet] app\n" {
		t.Fatalf("line = %q", out.String())
	}
}

// TestInputsMatchTestdata keeps the sample input constants equal to the
// testdata files the README pipes into the hook and statusline modes.
func TestInputsMatchTestdata(t *testing.T) {
	for file, want := range map[string]string{
		"testdata/pretooluse.json": preToolUseInput,
		"testdata/statusline.json": statusLineInput,
	} {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.TrimSpace(string(body)); got != want {
			t.Errorf("%s differs from its constant", file)
		}
	}
}
