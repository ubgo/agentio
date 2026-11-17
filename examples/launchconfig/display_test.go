package main

import (
	"strings"
	"testing"

	"github.com/ubgo/agentio"
)

// TestDisplayCommandWindowsPath: a Windows run dir (backslashes, which
// Command.String quotes and escapes) still renders as "<run dir>/…", the
// failure of platforms run 44085141162. The separator is the build OS's,
// so the path is built with it.
func TestDisplayCommandWindowsPath(t *testing.T) {
	dir := `C:\Users\RUNNER~1\AppData\Local\Temp\TestRun123\001`
	sep := string([]rune{'\\'})
	cmd := agentio.Command{Path: "claude", Args: []string{"--settings", dir + sep + "settings-x.json"}}
	got := displayCommandSep(cmd, dir, sep)
	if !strings.Contains(got, "--settings <run dir>/settings-x.json") {
		t.Fatalf("got %q", got)
	}
}
