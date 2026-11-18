package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// TestRun runs the whole walk on the fixture copy and checks one line of
// every guide step, so a library change that alters an answer fails here.
func TestRun(t *testing.T) {
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := setup(tmp)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), &out, m); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"default: ~/.gemini name=personal\n",
		"name=gemini-work launches with GEMINI_CLI_HOME=~",
		"install: gemini",
		"method=npm version=0.68.0\n",
		"user auth=oauth-personal approval=auto_edit unknown keys=[skills ui]\n",
		"effective model=gemini-2.5-flash (workspace wins) approval=auto_edit -> autonomy accept-edits\n",
		"project slug=demo (Slugify of the base name: my-project)\n",
		`  22222222  "Changelog entry for the parser fix" legacy=false updated 2026-11-02T09:31Z`,
		`  44444444  "explain the build script"           legacy=true  updated 2026-10-20T12:10Z`,
		"resume: gemini --resume 22222222-2222-4222-8222-222222222222 (in <project>)\n",
		"mcp disabled-one    stdio scope=user    enabled=false env=[]\n",
		"mcp fixture-stdio   stdio scope=user    enabled=-     env=[FIXTURE_TOKEN]\n",
		"skill old-skill  scope=user    enabled=false\n",
		"skill ext-skill  scope=plugin  enabled=true extension=demo-ext\n",
		"auth=oauth-personal logged-in=true credential=file email=[redacted] expires=2026-11-14\n",
		`launch: gemini --prompt-interactive "explain the parser"`,
		"launch: gemini --approval-mode plan --model gemini-2.5-flash\n",
		`launch: gemini --approval-mode yolo --output-format stream-json --include-directories ../lib --prompt "fix the failing tests"`,
		"token launch sets GEMINI_API_KEY (value redacted in output: true)\n",
		"full without AllowDangerous: agentio: full autonomy needs explicit consent\n",
		"login-commander: Gemini CLI has no login subcommand",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
	for _, leak := range []string{"fixture-secret", "fixture-access-token", "user@example.com", "example-key"} {
		if strings.Contains(got, leak) {
			t.Errorf("output leaks %q", leak)
		}
	}
}
