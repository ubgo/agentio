package agentio

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// writeTestFile writes a file and its dirs.
func writeTestFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestInstallFromPath: the version comes from install metadata in every
// layout, without running anything (G13).
func TestInstallFromPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	gem := filepath.Join(root, "npm", "lib", "node_modules", "@google", "gemini-cli")
	writeTestFile(t, filepath.Join(gem, "package.json"), `{"name":"@google/gemini-cli","version":"0.68.0"}`)
	writeTestFile(t, filepath.Join(gem, "bundle", "gemini.js"), "#!/usr/bin/env node\n")
	pnpm := filepath.Join(root, "pnpm", "global", "5", "node_modules", "@earendil-works", "pi-coding-agent")
	writeTestFile(t, filepath.Join(pnpm, "package.json"), "\xEF\xBB\xBF"+`{"name":"@earendil-works/pi-coding-agent","version":"1.5.2"}`)
	writeTestFile(t, filepath.Join(pnpm, "dist", "bundle", "cli.js"), "")
	cellar := filepath.Join(root, "Cellar", "gemini-cli", "0.67.1_1", "bin", "gemini")
	writeTestFile(t, cellar, "")
	native := filepath.Join(root, ".local", "share", "claude", "versions", "2.1.325")
	writeTestFile(t, native, "")
	other := filepath.Join(root, "opt", "tool", "bin", "tool")
	writeTestFile(t, other, "")
	wrongPkg := filepath.Join(root, "x", "node_modules", "other")
	writeTestFile(t, filepath.Join(wrongPkg, "package.json"), `{"name":"other","version":"9.9.9"}`)
	writeTestFile(t, filepath.Join(wrongPkg, "cli.js"), "")
	packages := []string{"@google/gemini-cli", "@earendil-works/pi-coding-agent"}
	for _, tc := range []struct {
		name, file string
		method     InstallMethod
		version    string
	}{
		{"npm", filepath.Join(gem, "bundle", "gemini.js"), InstallNPM, "0.68.0"},
		{"pnpm with BOM", filepath.Join(pnpm, "dist", "bundle", "cli.js"), InstallPNPM, "1.5.2"},
		{"brew cellar revision", cellar, InstallBrew, "0.67.1"},
		{"native versions dir", native, InstallNative, "2.1.325"},
		{"unknown", other, InstallUnknown, ""},
		{"another package", filepath.Join(wrongPkg, "cli.js"), InstallUnknown, ""},
	} {
		in, err := InstallFromPath([]string{tc.file, "/shadow/tool"}, packages)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		got := ""
		if !in.Version.IsZero() {
			got = in.Version.String()
		}
		if in.Method != tc.method || got != tc.version || !slices.Equal(in.Shadowed, []string{"/shadow/tool"}) {
			t.Errorf("%s: %+v, want %s %s", tc.name, in, tc.method, tc.version)
		}
	}
	if _, err := InstallFromPath(nil, packages); err == nil {
		t.Error("no hit must be ErrNotInstalled")
	}
}

// TestLinuxEnvAt reads a fake /proc environ (NUL-separated).
func TestLinuxEnvAt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "42", "environ"), "PATH=/bin\x00AGX_RUN_ID=r1\x00")
	env, err := linuxEnvAt(root, 42)
	if err != nil || !slices.Equal(env, []string{"PATH=/bin", "AGX_RUN_ID=r1"}) {
		t.Fatalf("env = %v %v", env, err)
	}
	if _, err := linuxEnvAt(root, 43); err == nil {
		t.Error("missing pid must fail")
	}
}

// TestEnvFields keeps NAME=value fields from `ps -E` output.
func TestEnvFields(t *testing.T) {
	t.Parallel()
	got := envFields("/usr/local/bin/claude --resume x PATH=/bin AGX_RUN_ID=r1 =bad\n")
	if !slices.Equal(got, []string{"PATH=/bin", "AGX_RUN_ID=r1"}) {
		t.Fatalf("fields = %v", got)
	}
}
