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
	var out bytes.Buffer
	if err := run(context.Background(), &out, testFixture); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"personal: login in file (~/.claude/.credentials.json)",
		"work: login in keychain (Claude Code-credentials-<hash>)",
		"sk-ant-ort01-E refused: claude: this token cannot run Claude",
		"token env for oauth-token: CLAUDE_CODE_OAUTH_TOKEN=[redacted]",
		"command: claude auth login --console --email user@example.com CLAUDE_CONFIG_DIR set: true",
		`identity: logged in true, plan "Max (20x)", billing subscription, credential file`,
		"profile without organization: claude: profile response has no organization",
		"auth status: logged in true via claude.ai (firstParty), plan tier max_20x, email [redacted]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
	// No token value and no personal value may reach the output.
	for _, secret := range []string{"FIXTURE-NOT-A-TOKEN", "EXAMPLE-NOT-A-TOKEN", "Example organizationName"} {
		if strings.Contains(got, secret) {
			t.Errorf("output leaks %q", secret)
		}
	}
}
