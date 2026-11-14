package codex

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestWindowsFixtureHome runs Codex's Windows layout on any machine: the
// npm global .cmd shim in %APPDATA%\npm is found through PATHEXT and leads
// to the package version, UpdateCommand resolves npm.cmd the same way, a
// BOM+CRLF config.toml decodes, and the default home is %USERPROFILE%\.codex
// with CODEX_HOME removed in any spelling.
func TestWindowsFixtureHome(t *testing.T) {
	dir := t.TempDir()
	agenttest.CopyFixture(t, filepath.Join("testdata", "windows", "userhome"), dir)
	npmDir := filepath.Join(dir, "AppData", "Roaming", "npm")
	h := agenttest.NewHost(t, agenttest.WithUserHome(dir), agenttest.WithSystem(agentio.OSWindows, ""),
		agenttest.WithEnv("Path="+npmDir, "PATHEXT=.COM;.EXE;.BAT;.CMD", "codex_home=C:\\stale"))
	a := New(h)
	in, err := a.Install()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(in.Path) != "codex.cmd" || in.Method != agentio.InstallNPM || in.Version.String() != "0.162.1" || in.PackageRoot != filepath.Join(npmDir, "node_modules", "@openai", "codex") {
		t.Fatalf("install %+v", in)
	}
	up, err := a.UpdateCommand()
	if err != nil || up.Path != filepath.Join(npmDir, "npm.cmd") || !slices.Equal(up.Args, []string{"install", "-g", npmPkg}) {
		t.Fatalf("update %s %v %v", up.Path, up.Args, err)
	}
	def := a.DefaultHome()
	if def.Dir() != filepath.Join(dir, ".codex") {
		t.Fatalf("default home %s", def.Dir())
	}
	cfg, err := a.ConfigFile(def)
	if err != nil || cfg.Model.V != "o3" {
		t.Fatalf("BOM+CRLF config.toml: %q %v", cfg.Model.V, err)
	}
	if tr, ok := cfg.TrustOf(`C:\Users\user\proj`); !ok || tr != TrustTrusted {
		t.Fatalf("trust of a drive-letter project: %v %v", tr, ok)
	}
	cmd, err := a.New(def.Target(), LaunchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range cmd.Env {
		if n, _, _ := strings.Cut(kv, "="); n == "codex_home" || n == EnvHome {
			t.Fatalf("default home kept %s", kv)
		}
	}
}

// TestInstallLayouts covers the package-manager layouts install detection
// knows, each built as it lies on disk (symlink, shim script or plain
// binary), and the update command each one gets.
func TestInstallLayouts(t *testing.T) {
	const pkgJSON = `{"version":"0.162.1"}`
	for _, tc := range []struct {
		name    string
		goos    string
		build   func(t *testing.T, home string) (binDir string)
		method  agentio.InstallMethod
		version string
		update  []string
	}{
		{"nvm symlink", agentio.OSLinux, func(t *testing.T, home string) string {
			lib := filepath.Join(home, ".nvm", "versions", "node", "v22.0.0", "lib", "node_modules", "@openai", "codex")
			agenttest.WriteFile(t, lib, "package.json", pkgJSON)
			agenttest.WriteFile(t, lib, "bin/codex.js", "#!/usr/bin/env node\n")
			bin := filepath.Join(home, ".nvm", "versions", "node", "v22.0.0", "bin")
			symlinkExec(t, filepath.Join(lib, "bin", "codex.js"), filepath.Join(bin, Binary))
			writeExec(t, filepath.Join(bin, "npm"))
			return bin
		}, agentio.InstallNPM, "0.162.1", []string{"install", "-g", npmPkg}},
		{"pnpm sh shim", agentio.OSLinux, func(t *testing.T, home string) string {
			bin := filepath.Join(home, ".local", "share", "pnpm")
			agenttest.WriteFile(t, filepath.Join(bin, "global", "5", "node_modules", "@openai", "codex"), "package.json", pkgJSON)
			agenttest.WriteFile(t, bin, Binary, "#!/bin/sh\nbasedir=$(dirname \"$0\")\nexec node \"$basedir/global/5/node_modules/@openai/codex/bin/codex.js\" \"$@\"\n")
			chmodExec(t, filepath.Join(bin, Binary))
			writeExec(t, filepath.Join(bin, "pnpm"))
			return bin
		}, agentio.InstallPNPM, "0.162.1", []string{"add", "-g", npmPkg}},
		{"bun on windows", agentio.OSWindows, func(t *testing.T, home string) string {
			bin := filepath.Join(home, ".bun", "bin")
			agenttest.WriteFile(t, filepath.Join(home, ".bun", "install", "global", "node_modules", "@openai", "codex"), "package.json", pkgJSON)
			agenttest.WriteFile(t, bin, "codex.exe", "MZ\x00\x00")
			agenttest.WriteFile(t, bin, "bun.exe", "MZ\x00\x00")
			return bin
		}, agentio.InstallBun, "0.162.1", []string{"install", "-g", npmPkg}},
		{"volta", agentio.OSLinux, func(t *testing.T, home string) string {
			bin := filepath.Join(home, ".volta", "bin")
			agenttest.WriteFile(t, filepath.Join(home, ".volta", "tools", "image", "packages", "@openai", "codex", "lib", "node_modules", "@openai", "codex"), "package.json", pkgJSON)
			writeExec(t, filepath.Join(bin, "volta-shim"))
			symlinkExec(t, filepath.Join(bin, "volta-shim"), filepath.Join(bin, Binary))
			writeExec(t, filepath.Join(bin, "volta"))
			return bin
		}, agentio.InstallVolta, "0.162.1", []string{"install", npmPkg}},
		{"homebrew formula", agentio.OSLinux, func(t *testing.T, home string) string {
			cellar := filepath.Join(home, "linuxbrew", "Cellar", "codex", "0.162.1", "bin")
			writeExec(t, filepath.Join(cellar, Binary))
			bin := filepath.Join(home, "linuxbrew", "bin")
			symlinkExec(t, filepath.Join(cellar, Binary), filepath.Join(bin, Binary))
			writeExec(t, filepath.Join(bin, "brew"))
			return bin
		}, agentio.InstallBrew, "0.162.1", []string{"upgrade", Binary}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			bin := tc.build(t, home)
			a := New(agenttest.NewHost(t, agenttest.WithUserHome(home), agenttest.WithSystem(tc.goos, ""),
				agenttest.WithEnv("PATH="+bin, "PATHEXT=.EXE;.CMD")))
			in, err := a.Install()
			if err != nil {
				t.Fatal(err)
			}
			if in.Method != tc.method || in.Version.String() != tc.version {
				t.Fatalf("install %s %s (%+v)", in.Method, in.Version, in)
			}
			up, err := a.UpdateCommand()
			if err != nil || !slices.Equal(up.Args, tc.update) || !filepath.IsAbs(up.Path) {
				t.Fatalf("update %s %v %v", up.Path, up.Args, err)
			}
		})
	}
}

// writeExec writes an executable placeholder.
func writeExec(t *testing.T, p string) {
	t.Helper()
	agenttest.WriteFile(t, filepath.Dir(p), filepath.Base(p), "#!/bin/sh\n")
	chmodExec(t, p)
}

// chmodExec sets the exec bit.
func chmodExec(t *testing.T, p string) {
	t.Helper()
	if err := os.Chmod(p, 0o700); err != nil {
		t.Fatal(err)
	}
}

// symlinkExec links to -> from (from is created executable when missing).
func symlinkExec(t *testing.T, from, to string) {
	t.Helper()
	if _, err := os.Stat(from); err != nil {
		writeExec(t, from)
	}
	chmodExec(t, from)
	if err := os.MkdirAll(filepath.Dir(to), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(from, to); err != nil {
		t.Fatal(err)
	}
}
