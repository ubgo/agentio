package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// repoRoot is the repository root seen from this package's directory.
const repoRoot = "../.."

// snippet:start host
// driftHost copies both fixture homes into a temp user home and returns a
// fixture Host over the copy: FailExec and FailingHTTP (the agenttest
// defaults) fail the test if the drift check ever runs a binary or calls
// the network, and the copy lets a test edit files without touching the
// checked-in fixtures.
func driftHost(t *testing.T) agentio.Host {
	t.Helper()
	home := t.TempDir()
	agenttest.CopyFixture(t, filepath.Join(repoRoot, claudeFixture), home)
	agenttest.CopyFixture(t, filepath.Join(repoRoot, codexFixture), filepath.Join(home, ".codex"))
	return agenttest.NewHost(t,
		agenttest.WithUserHome(home),
		agenttest.WithEnv("HOME="+home, "PATH="+filepath.Join(home, "bin")),
	)
}

// snippet:end host

// runDrift runs the check on h and returns the report text.
func runDrift(t *testing.T, h agentio.Host, opt options) string {
	t.Helper()
	var b strings.Builder
	if err := run(context.Background(), h, opt, &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// wantLines fails the test for each line missing from out.
func wantLines(t *testing.T, out string, lines ...string) {
	t.Helper()
	for _, l := range lines {
		if !strings.Contains(out, l) {
			t.Errorf("report missing %q", l)
		}
	}
}

// TestFixtures checks the header and a few findings of the 2.1.325
// fixture homes: no binary on PATH, both Claude homes and the Codex home
// read, and unknown keys the fixtures carry on purpose.
func TestFixtures(t *testing.T) {
	out := runDrift(t, driftHost(t), options{})
	wantLines(t, out,
		"claude: install not found (agentio: binary not found on PATH: claude)",
		"claude home personal\n  transcripts read 2, record versions 2.1.325\n",
		"claude home work\n",
		"codex: install not found (agentio: binary not found on PATH: codex)",
		"codex home personal\n",
		"claude\tpersonal\tunknown-key\tskills\t[].Unknown.license\t(x1)\n",
		"claude\tpersonal\tunknown-key\ttranscript assistant\tUsage.Unknown.speed\t(x17)\n",
		"claude\twork\tunknown-key\tsessions\t[].Unknown.nameSince\t(x2)\n",
	)
}

// TestFixturesFlagHost checks the Host main builds for -fixtures, which
// reads the checked-in fixtures in place.
func TestFixturesFlagHost(t *testing.T) {
	h, err := fixtureHost(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	out := runDrift(t, h, options{})
	wantLines(t, out, "codex home 0.162.1\n", "246 distinct findings, 0 already in the baseline\n")
}

// snippet:start newkey
// TestNewKeyAndEnum adds a settings key and a permission mode the library
// has never seen to the copied home. The check reports both: the key from
// Settings.Unknown, the mode from PermissionMode.IsKnown.
func TestNewKeyAndEnum(t *testing.T) {
	h := driftHost(t)
	agenttest.WriteFile(t, h.UserHome, ".claude/settings.json",
		`{"model": "opus", "futureKey": true, "permissions": {"defaultMode": "futureMode"}}`)
	out := runDrift(t, h, options{})
	wantLines(t, out,
		"claude\tpersonal\tunknown-key\tsettings\tSettings.Unknown.futureKey\t(x1)\n",
		"claude\tpersonal\tunknown-enum\tsettings\tSettings.Permissions.DefaultMode PermissionMode=\"futureMode\"\t(x1)\n",
	)
}

// snippet:end newkey

// snippet:start baseline
// TestBaseline saves one run and feeds it back with -baseline: every
// finding is already known, so the second report lists none of them and
// only the summary line counts them as suppressed.
func TestBaseline(t *testing.T) {
	h := driftHost(t)
	first := runDrift(t, h, options{})
	base := filepath.Join(t.TempDir(), "last.tsv")
	if err := os.WriteFile(base, []byte(first), 0o600); err != nil {
		t.Fatal(err)
	}
	second := runDrift(t, h, options{basePath: base})
	if strings.Contains(second, "\tunknown-key\t") {
		t.Fatalf("baseline did not suppress known findings:\n%s", second)
	}
	wantLines(t, second, "246 distinct findings, 246 already in the baseline\n")
}

// snippet:end baseline

// snippet:start help
// TestClaudeHelp feeds a saved `claude --help` text: a flag the feature
// table names is quiet, a flag it does not name is a candidate row.
func TestClaudeHelp(t *testing.T) {
	help := filepath.Join(t.TempDir(), "help.txt")
	text := "Options:\n  --worktree [name]   Create a git worktree\n  --brand-new-flag    Something new\n"
	if err := os.WriteFile(help, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	out := runDrift(t, driftHost(t), options{helpPath: help})
	wantLines(t, out, "claude\t-\tflag-not-in-table\t--help\t--brand-new-flag\t(x1)\n")
	if strings.Contains(out, "\t--worktree\t") {
		t.Fatal("--worktree is in features.tsv and must not be reported")
	}
}

// snippet:end help
