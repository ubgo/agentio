package main

import (
	"bytes"
	"strings"
	"testing"
)

// testFixture is the fixture user home relative to this package dir.
const testFixture = "../../claude/testdata/homes/2.1.325/userhome"

// TestRun runs the example on the fixture and checks one line per section.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, testFixture); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"home ~/.claude name=personal default=true",
		"home ~/.claude-work name=work default=false",
		"HomeFromEnv: CLAUDE_CONFIG_DIR is absent, so the default home is in use",
		"path=~/.local/bin/claude real=~/.local/share/claude/versions/2.1.325 method=native version=2.1.325",
		"last update 2026-11-12T22:39:35Z: 2.1.323 -> 2.1.325 outcome=success",
		"update command: ~/.local/bin/claude update",
		"EncodePath(\"/work/proj12\") = -work-proj12",
		"project present=true file=~/project/.claude/settings.json model=\"\" unknown keys=[futureKey]",
		"model=\"sonnet\" from local",
		"FIXTURE_FLAG=[redacted] from=local secret=false",
		"account email=[redacted] org type=claude_max billing=stripe_subscription",
		"dir=/work/proj2 trusted=false last session cost=$6.07 models=2",
		"backup ~/.claude/backups/.claude.json.backup.1794000000000",
		"default mode=bypassPermissions set by user",
		"\"Bash(git *\": claude: invalid permission rule \"Bash(git *\"",
		"restriction allow_remote_control allowed=false",
		"stamp kind=org identity=[redacted] confirmed=2026-11-01T10:00:00Z",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Contains(out.String(), "user@example.com") {
		t.Error("a personal value was printed")
	}
}
