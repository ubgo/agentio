package main

import (
	"bytes"
	"strings"
	"testing"
)

// testUserHome is the fixture user home relative to this package dir.
const testUserHome = "../../claude/testdata/homes/2.1.325/userhome"

func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, testUserHome); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		`new: claude --verbose --model sonnet --add-dir /work/shared-lib -- "fix the failing tests"` + "\n",
		"new: env CLAUDE_CONFIG_DIR=~/.claude-work\n",
		"new: env -ANTHROPIC_API_KEY\n",
		"new: warnings=0\n",
		"resume: claude --verbose --resume f85efbdb-fc2f-1e8e-7748-9f915edc8e2d\n",
		"default: env -CLAUDE_CONFIG_DIR\n",
		"default: env -ANTHROPIC_API_KEY\n",
		"default: warning version-unknown: installed Claude Code version unknown; flags not checked\n",
		"autonomy full: refused=true\n",
		"autonomy full allowed: claude --verbose --dangerously-skip-permissions\n",
		"home rule for default: -CLAUDE_CONFIG_DIR\n",
		"change: op=set-secret CLAUDE_CODE_OAUTH_TOKEN=[redacted]\n",
		"redacted env: [DEBUG=0 CLAUDE_CONFIG_DIR=~/.claude-work CLAUDE_CODE_OAUTH_TOKEN=[redacted]]\n",
		"token: %s=[redacted] %v=[redacted] %q=[redacted] %#v=[redacted]\n",
		"token: slog=level=INFO msg=login token=[redacted]\n",
		"token launch: argv holds token=false\n",
		"token launch: env -ANTHROPIC_API_KEY\n",
		`personal: json={"email":"[redacted]","org":"","mcp_auth":"[redacted]"}` + "\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
	// No token value may reach the output.
	if strings.Contains(got, exampleToken) || strings.Contains(got, "sk-from-the-shell") {
		t.Errorf("output leaks a secret:\n%s", got)
	}
}
