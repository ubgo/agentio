package pi

import (
	"path/filepath"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestWindowsNpmInstall runs the layout npm writes on Windows, on any
// machine: %APPDATA%\npm\pi.cmd (a cmd-shim, no symlink) beside
// node_modules\@earendil-works\pi-coding-agent. A Windows host finds the
// shim through PATHEXT, and the shim leads to the package and its version.
func TestWindowsNpmInstall(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	npmDir := filepath.Join(dir, "AppData", "Roaming", "npm")
	pkg := filepath.Join(npmDir, "node_modules", "@earendil-works", "pi-coding-agent")
	agenttest.WriteFile(t, pkg, "package.json", `{"name":"@earendil-works/pi-coding-agent","version":"1.5.2"}`)
	agenttest.WriteFile(t, pkg, "dist/bundle/cli.js", "#!/usr/bin/env node\n")
	agenttest.WriteFile(t, npmDir, "pi.cmd", "@ECHO off\r\n\"%dp0%\\node_modules\\@earendil-works\\pi-coding-agent\\dist\\bundle\\cli.js\" %*\r\n")
	h := agenttest.NewHost(t, agenttest.WithUserHome(dir), agenttest.WithSystem(agentio.OSWindows, ""),
		agenttest.WithEnv("Path="+npmDir, "PATHEXT=.COM;.EXE;.BAT;.CMD", "USERPROFILE="+dir))
	in, err := New(h).Install()
	if err != nil || filepath.Base(in.Path) != "pi.cmd" || in.Version.String() != "1.5.2" || in.Method != agentio.InstallNPM {
		t.Fatalf("install %+v %v", in, err)
	}
}
