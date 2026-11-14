package main

import (
	"bytes"
	"strings"
	"testing"
)

// testFixture is defaultFixture seen from the package directory, where
// go test runs.
const testFixture = "../" + defaultFixture

func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, testFixture); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"id=codex name=Codex binary=codex home-env=CODEX_HOME default-home=.codex",
		"default home keeps CODEX_HOME in the child env: false",
		"installation id: [redacted]",
		"path=~/npm/bin/codex method=npm version=0.162.1",
		"update: ~/npm/bin/npm install -g @openai/codex",
		"latest seen=0.156.1 checked=2026-09-24 dismissed=false",
		"login status: logged-in=true method=apikey",
		`feature unified_exec stage="under development" enabled=false`,
		"v2 profile work=~/.codex/work.config.toml",
		"model=gpt-5.6-terra effort=medium (known=true) warnings=0",
		"sandbox=brand-new-mode known=false",
		"effective model from project",
		"effective model_reasoning_effort from user-profile",
		"effective model_verbosity from session",
		"profile work kind=v2 table=false file=~/.codex/work.config.toml",
		`-c model="gpt-5.5"`,
		`-c sandbox_mode="read-only"`,
		"rejected: no '='",
		"mcp node_repl transport=stdio layer=user enabled-set=false env-keys=[CODEX_HOME SECRET_TOKEN]",
		"SECRET_TOKEN prints as [redacted]",
		"codex mcp add files -- /opt/mcp/files --root /work",
		"env values refused: true",
		"instructions ~/work/proj/AGENTS.md scope=project kind=AGENTS.md bytes=16 truncated=false",
		`skill pr-review scope=plugin enabled=true plugin="github"`,
		"plugin github@openai-curated version=26.928.31416 enabled=true",
		"state /home/test/.codex/hooks.json:stop:0:0 enabled-set=true enabled=false",
		"rule [npm test] -> allow (~/work/proj/.codex/rules/a.rules:1)",
		"issue ~/.codex/rules/default.rules:4 statement is not a prefix_rule call",
		"source cli subagent",
		"warn  profile-conflict",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	// The fixture's fake MCP env value must never be printed.
	if strings.Contains(got, "fake-secret-value") {
		t.Error("an MCP env value was printed")
	}
}
