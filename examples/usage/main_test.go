package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestRun runs every step against the fake API and fixture copies and
// checks one line per guide step.
func TestRun(t *testing.T) {
	srv := fakeAPI()
	defer srv.Close()
	client := srv.Client()
	h, err := setup(t.TempDir(), client)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), &out, h, srv.URL); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"claude personal: max 6% source=cache age=387h21m0s",
		"claude work: no cached usage",
		"codex personal: max 12% source=transcript",
		"claude: max 42% billing=subscription source=server",
		"codex: login expired; run the vendor CLI once to refresh it",
		`claude: plan="Max (20x)" status="active" billing-type="stripe_subscription"`,
		`codex: plan="Plus"`,
		"claude browser             claude auth login --claudeai",
		"claude device              unsupported",
		"codex device              codex login --device-auth",
		"token env: [CLAUDE_CODE_OAUTH_TOKEN=[redacted]]",
		"token on argv: false",
		"codex: no token login",
		"claude oauth token: max 37% source=probe",
		"claude api key: billed per token, not probed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Contains(got, sampleOAuthToken) {
		t.Error("a token was printed")
	}
}
