package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// testFixture is defaultFixture seen from the package directory, where
// go test runs.
const testFixture = "../" + defaultFixture

func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out, testFixture); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		"logged-in=true api-key=false plan=plus (Plus) workspace=false",
		"email=[redacted] account=[redacted] store=file",
		"access token expires 2026-09-21T14:13:20Z",
		"keyring login: logged-in=true store=keyring",
		"keyring error: value is not auth JSON: true",
		"usage plan=plus limit-reached=false source=server",
		"window primary_window length=5h0m0s used=42.5%",
		"subscription plan=plus source=cache",
		"server saw a bearer token: true",
		"cached window primary limit=codex length=168h0m0s used=12%",
		"api-key-stdin      codex login --with-api-key",
		"logout             codex logout",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	// Neither the email nor any part of a fixture token may be printed.
	for _, secret := range []string{"user@example.test", "acct-0000", "eyJ"} {
		if strings.Contains(got, secret) {
			t.Errorf("printed a personal or secret value starting %.4q", secret)
		}
	}
}
