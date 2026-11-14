package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// testFixture is the fixture user home relative to this package dir.
const testFixture = "../" + defaultFixture

func TestRun(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), &out, testFixture); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"live usage (server, billing subscription), highest window 6%",
		"  Current session    6%  class short resets 2026-10-28T12:59:59Z",
		`wire row: kind weekly_scoped, group weekly, severity normal, active false, label "Fable"`,
		"empty body: claude: usage response has no limits",
		"cached usage (cache), observed 2026-10-28T08:38:37Z",
		"extra usage: enabled false",
		"  Current session   31%  resets 2026-11-13T15:00:00Z",
		"api key: agentio: billed per token: no usage windows",
		"half a header pair: claude: probe response has no usable rate-limit headers",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "sk-ant-") {
		t.Error("output leaks a token value")
	}
}
