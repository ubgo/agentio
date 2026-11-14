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
		`session 0199e1a0-0000-7000-8000-000000000001 alive=true pid=0 dir="/work/proj"`,
		`session 01a1d83a-e1cb-70d1-bee7-7aebab05bc2b alive=false pid=0 dir=""`,
		"01a1d83a-e1cb-70d1-bee7-7aebab05bc2b IsLive=false lock file exists=true",
		`ParseThreadLockName(".coordination.lock") = "", false`,
		`process pid=101 kind=cli cwd="/work/proj"`,
		"process pid=103 kind=launcher",
		"classify pid=104 codex=true kind=app-server session=false",
		"classify pid=105 codex=false",
		`chat process "npm run dev" pid=4242`,
		"daemon socket present: false",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}
