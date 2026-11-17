package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestRun pins a few rows and reasons, so a capability change shows here
// and in the docs matrix.
func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"capability          claude  codex   gemini  pi",
		"usage               yes     yes     -       -",
		"conversations       yes     yes     yes     yes",
		"live-credential     yes     -       -       -",
		"launch-config       yes     yes     -       -",
		"gemini launch-config: Gemini CLI has no settings flag",
		"codex live-credential: Codex reads its login",
		"pi usage: ",
		"gemini usage: Gemini CLI quota and tier come from a logged-in Code Assist API call",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
}
