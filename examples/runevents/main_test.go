package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio/examples/internal/exampletest"
)

// TestRun decodes the recorded Claude stream and the Codex sample and
// checks the events and token counts.
func TestRun(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(filepath.Join(root, claudeStream))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out bytes.Buffer
	if err := run(&out, f, strings.NewReader(codexStream)); err != nil {
		t.Fatal(err)
	}
	got := exampletest.Output(out.String())
	for _, want := range []string{
		`launch: claude -p --output-format stream-json --verbose -- "list the files"`,
		"started conversation 00000000-0000-4000-8000-000000000001",
		`message "ok"`,
		"done tokens in=2 cached=11370 out=4 reasoning=0",
		`launch: codex exec --json "list the files"`,
		`message "I will list the files."`,
		"done tokens in=1200 cached=1000 out=80 reasoning=16",
		"  tool    x1",
		"  other   x2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}
