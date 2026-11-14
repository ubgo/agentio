package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// TestRun checks the pid-reuse guard, the Codex lock answer, the process
// kinds and that FindLiveSession never matches a Codex pid.
func TestRun(t *testing.T) {
	h, err := setup(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), &out, h); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		`claude personal pid=4242 conv=6d50c0ec dir=/work/proj4 kind="interactive" status=busy alive=true`,
		`claude personal pid=4243 conv=3e03d92d dir=/work/proj6 kind="interactive" status=idle alive=false`,
		`codex personal pid=0 conv=0199e1a0 dir=/work/proj kind="" status=unknown alive=true`,
		"pid 5001: claude mcp-server session=false",
		"pid 5002 /Applications/Claude.app/Contents/MacOS/Claude: not an agent",
		"pid 6000: codex launcher session=false",
		"pid 6001: codex exec session=true",
		"pid 4342: claude home work conversation 78d3d87c-4440-06f3-ceba-625a49fad9ec in /work/proj15",
		"pid 6001: no live session found",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q", want)
		}
	}
}
