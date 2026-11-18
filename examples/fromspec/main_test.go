package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// TestRun builds the config-only agent in a temp machine and checks every
// step's answer.
func TestRun(t *testing.T) {
	exampletest.RequirePOSIXLayout(t)
	h, err := setup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), &out, h); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	if strings.Contains(got, exampleToken) {
		t.Error("output shows the token value")
	}
	for _, want := range []string{
		"agentio: invalid spec: acme: resume args [--resume] lack {id}",
		"home work default=false ~/.acme-work",
		"supported: [installer binary-version token-login login-commander process-classifier home-linker doctor]",
		"usage: config-only agent: a spec describes L1 only; this needs code",
		"without login args: login commander false (spec has no login args)",
		`personal: acme --model acme-large --message "fix the build" (ACME_HOME set=false )`,
		"work: acme --resume c-42 (ACME_HOME set=true ~/.acme-work)",
		"(unsupported=true)",
		"full autonomy: agentio: full autonomy needs Target.AllowDangerous",
		"kinds: [api-key oauth-token] (detected from the value: false)",
		"api-key: acme env [ACME_API_KEY=[redacted]]",
		"oauth-token: acme env [ACME_OAUTH_TOKEN=[redacted]]",
		"api-key-helper: agentio: unsupported: acme: token login api-key-helper: not described by the agent spec",
		"login: acme login, logout: acme logout",
		"pid 11: match=false",
		"check api-billing-env warn ACME_API_KEY set: launches bill per token",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}
