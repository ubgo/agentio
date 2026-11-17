package gemini

import (
	"path/filepath"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestWindowsNpmInstall runs the layout npm writes on Windows, on any
// machine: %APPDATA%\npm\gemini.cmd (a cmd-shim, no symlink) beside
// node_modules\@google\gemini-cli. A Windows host finds the shim through
// PATHEXT, and the shim leads to the package and its version.
func TestWindowsNpmInstall(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	npmDir := filepath.Join(dir, "AppData", "Roaming", "npm")
	pkg := filepath.Join(npmDir, "node_modules", "@google", "gemini-cli")
	agenttest.WriteFile(t, pkg, "package.json", `{"name":"@google/gemini-cli","version":"0.68.0"}`)
	agenttest.WriteFile(t, pkg, "bundle/gemini.js", "#!/usr/bin/env node\n")
	agenttest.WriteFile(t, npmDir, "gemini.cmd", "@ECHO off\r\n\"%dp0%\\node_modules\\@google\\gemini-cli\\bundle\\gemini.js\" %*\r\n")
	h := agenttest.NewHost(t, agenttest.WithUserHome(dir), agenttest.WithSystem(agentio.OSWindows, ""),
		agenttest.WithEnv("Path="+npmDir, "PATHEXT=.COM;.EXE;.BAT;.CMD", "USERPROFILE="+dir))
	in, err := New(h).Install()
	if err != nil || filepath.Base(in.Path) != "gemini.cmd" || in.Version.String() != "0.68.0" || in.Method != agentio.InstallNPM {
		t.Fatalf("install %+v %v", in, err)
	}
}
