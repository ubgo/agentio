package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// TestRun runs every door on a temp copy and checks the output, the backup
// file on disk and that only Runner calls reached Host.Exec.
func TestRun(t *testing.T) {
	exampletest.RequireSymlinks(t)
	srv := fakeProbeAPI()
	defer srv.Close()
	tmp := t.TempDir()
	exec := &recordingExec{}
	h, err := setup(tmp, exec, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), &out, h, exec, srv.URL); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"claude 2.1.325, codex 0.162.1",
		"aider 0.86.1; `aider --help` exited 1",
		"Current session 64% resets 16:00 UTC",
		"to ~/.claude/settings.json.agentio-bak-20261113T120000Z",
		`theme now "dark"`,
		"~/.claude-new/skills -> ~/.claude/skills (existed false)",
		"replan settings.json exists=true",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if _, err := os.Stat(filepath.Join(tmp, ".claude", "settings.json.agentio-bak-20261113T120000Z")); err != nil {
		t.Errorf("backup not on disk: %v", err)
	}
	want := []string{"claude --version", "codex --version", "aider --version", "aider --help"}
	if calls := exec.Calls(); strings.Join(calls, "|") != strings.Join(want, "|") {
		t.Errorf("exec calls %v, want %v", calls, want)
	}
}
