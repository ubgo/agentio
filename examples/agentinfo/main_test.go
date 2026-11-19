package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio/examples/internal/exampletest"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// Fixture homes, relative to this package directory.
const (
	claudeFixture = "../../claude/testdata/homes/2.1.325/userhome"
	codexFixture  = "../../codex/testdata/homes/0.162.1"
)

// fixtureHost copies both fixture homes into a temp user home (Codex as
// ~/.codex) and returns a Host whose HTTP client and process runner fail
// the test on use, with one live Claude session in the process table.
func fixtureHost(t *testing.T) (agentio.Host, string) {
	t.Helper()
	home := t.TempDir()
	agenttest.CopyFixture(t, claudeFixture, home)
	agenttest.CopyFixture(t, codexFixture, filepath.Join(home, ".codex"))
	procs := &agenttest.FakeProcesses{List: []agentio.Process{
		{PID: 4242, Exe: "/opt/claude/bin/claude", Argv: []string{"claude"}, Start: time.UnixMilli(1794597025793)},
	}}
	h := agenttest.NewHost(t,
		agenttest.WithUserHome(home),
		agenttest.WithEnv("HOME="+home, "PATH=/nonexistent-bin"),
		agenttest.WithProcesses(procs),
	)
	h.User = "user"
	return h, home
}

// TestRunText runs the default report (no --runner, no --network) on the
// fixture homes and checks lines from every section it prints.
func TestRunText(t *testing.T) {
	h, home := fixtureHost(t)
	var out bytes.Buffer
	if err := run(context.Background(), h, options{dir: filepath.Join(home, "project")}, &out); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(strings.ReplaceAll(out.String(), home, "~"))
	for _, want := range []string{
		"=== Claude Code (claude) ===",
		"install:   not installed (agentio: binary not found on PATH: claude)",
		"--- home ~/.claude (default)  name=personal",
		"--- home ~/.claude-work  name=work",
		"defaults:  model=sonnet autonomy=full",
		"fixture-stdio        stdio  user     fixture-mcp serve env=[FIXTURE_TOKEN]  (~/.claude.json)",
		"identity:  logged-in=true email=[redacted] org=[redacted] plan=Max (20x) billing=subscription credential=file expires=2026-11-13 20:56 expired=false",
		"pid 4242 6d50c0ec-647e-18aa-84c7-04170288be89 interactive alive=true status=busy",
		"=== Codex (codex) ===",
		"--- home ~/.codex (default)  name=personal",
		"profile:   work                 v2     ~/.codex/work.config.toml",
		"skills (3): squad[user] imagegen[system] github:pr-review[plugin]",
		"cached usage (transcript, as of 2026-10-14 10:00, 30d2h old):",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}

// TestRunJSON checks the typed report: homes, identity redaction, cached
// usage, the live session and the classified process.
func TestRunJSON(t *testing.T) {
	h, home := fixtureHost(t)
	var out bytes.Buffer
	if err := run(context.Background(), h, options{json: true, dir: filepath.Join(home, "project")}, &out); err != nil {
		t.Fatal(err)
	}
	var rep Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "user@example.com") {
		t.Error("email printed unredacted")
	}
	if len(rep.Agents) != 5 || rep.Agents[0].ID != "claude" || rep.Agents[1].ID != "codex" || rep.Agents[2].ID != "gemini" || rep.Agents[3].ID != "pi" || rep.Agents[4].ID != "opencode" {
		t.Fatalf("agents %+v", rep.Agents)
	}
	cl := rep.Agents[0]
	if len(cl.Homes) != 2 || cl.Homes[0].Identity == nil || !cl.Homes[0].Identity.LoggedIn || cl.Homes[0].CachedUsage == nil {
		t.Fatalf("claude homes %+v", cl.Homes)
	}
	if len(cl.Processes) != 1 || cl.Processes[0].PID != 4242 || !cl.Processes[0].Session {
		t.Fatalf("claude processes %+v", cl.Processes)
	}
}

// TestFixturesFlag runs `agentinfo -fixtures` as main does: the repository
// fixture homes read in place, with no real process table, keychain,
// network or binary, so the platform workflow and the distro images can
// run it on any OS.
func TestFixturesFlag(t *testing.T) {
	var out bytes.Buffer
	if err := runOS(context.Background(), options{fixtures: true}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"=== Claude Code (claude) ===", "=== Codex (codex) ===", "install:   not installed"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q", want)
		}
	}
}
