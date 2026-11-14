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
		`0199e1a0-0000-7000-8000-000000000001 "List the files" dir=/work/proj source=cli cli=0.162.1 originator=codex-tui`,
		"in /work/other: 1",
		"unknown id: ErrConversationNotFound",
		"has history in /work/proj: true",
		`turn assistant "There is one file: a.txt" model=gpt-5.5 output=80 reasoning=20`,
		"0 new turns, offset unchanged=true",
		`"and the sizes" session=0199e1a0-0000-7000-8000-000000000001`,
		"validate gpt-6-luna/xhigh unsupported: true",
		"parsed models=1 warnings=1",
		"turn_context model=gpt-5.5 approval=on-request sandbox=workspace-write",
		"records=14 last kind=brand_new_record_type known=false",
		"tail brand_new_record_type",
		"tokens input=1200 cached=1000 output=80 reasoning=20 total=1280",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	// Newest first: the October conversation is listed before the March one.
	if strings.Index(got, "000000000001 ") > strings.Index(got, "000000000002 ") {
		t.Error("conversations are not newest first")
	}
}
