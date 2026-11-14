package main

import (
	"bytes"
	"runtime"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// TestRun checks one line of every step, so a library change that alters
// an answer fails here.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	wants := []string{
		"  effort                     enum     flag=--effort since=- values=[low medium high xhigh max]",
		"  mcp_configs                list     flag=--mcp-config since=0.2.75 values=[]",
		"effort=max (the --option beat the profile)",
		`refused: agentio: invalid launch option: unknown key "codex_profile"`,
		"ok=true unknown keys (kept, warned): [permissions.futureRule spinnerTipsEnabled]",
		"type error: cleanupPeriodDays expected int",
		"--settings <run dir>",
		`settings {"apiKeyHelper":"/usr/local/bin/agx credential"}`,
		"scrub [CLAUDE_CODE_OAUTH_TOKEN ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN]",
		"codex profile=deep search=true",
		"  -c model=true                                     type-error     expected string, found bool",
		"  -c tui.theme=\"dark\"                               raw",
		"pid 4242: alive (err=<nil>)",
		"pid 5151: ended (err=<nil>)",
	}
	if runtime.GOOS != "windows" {
		// Windows files carry ACLs, not unix permission bits.
		wants = append(wants, "settings file: <run dir>/settings-20261114-120000-ab12.json mode=-rw-------")
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
