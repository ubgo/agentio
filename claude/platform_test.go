package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestEncodePathMatchesUpstream: the history key is Claude Code 2.1.325's
// own rule ($H in the shipped bundle): every UTF-16 code unit that is not
// an ASCII letter or digit becomes "-", and a key longer than 200 units is
// cut to 200 and suffixed with "-" plus base 36 of |Java-style hash| of the
// original path. The want values were produced by running that JS function
// under node, so the Windows drive key, non-ASCII and long paths are the
// upstream results, not a guess.
func TestEncodePathMatchesUpstream(t *testing.T) {
	t.Parallel()
	long := "/work/" + strings.Repeat("a", 250)
	for _, tc := range []struct{ in, want string }{
		{"/work/proj12", "-work-proj12"},
		{`C:\Users\x\proj`, "C--Users-x-proj"},
		{"/home/u/café", "-home-u-caf-"},
		{"/home/u/😀x", "-home-u---x"},
		{"/a_b.c d", "-a-b-c-d"},
		{long, "-work-" + strings.Repeat("a", 194) + "-58fult"},
	} {
		if got := EncodePath(tc.in); got != tc.want {
			t.Errorf("EncodePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestLocalSettingsPathPerOS: settings.local.json sits at the repo root on
// Linux and macOS, but stays in the start dir on Windows and when the repo
// root is the user's home (Claude Code settings docs).
func TestLocalSettingsPathPerOS(t *testing.T) {
	t.Parallel()
	p := Project{Dir: "/r/sub", RepoRoot: "/r"}
	for _, tc := range []struct {
		goos, userHome, want string
	}{
		{agentio.OSLinux, "/home/u", "/r"},
		{agentio.OSDarwin, "/home/u", "/r"},
		{agentio.OSWindows, "/home/u", "/r/sub"},
		{agentio.OSLinux, "/r", "/r/sub"},
	} {
		a := New(agentio.Host{GOOS: tc.goos, UserHome: tc.userHome})
		want := filepath.Join(tc.want, dotClaudeDir, localSettingsFile)
		if got := a.ProjectPaths(a.DefaultHome(), p).LocalSettings; got != want {
			t.Errorf("%s home=%s: LocalSettings = %s, want %s", tc.goos, tc.userHome, got, want)
		}
	}
}

// TestKeychainOnlyOnDarwin: Claude Code keeps the login in the keychain on
// macOS only; on a linux or windows Host the library reads
// .credentials.json and never asks Host.Secrets (no secret-tool run).
func TestKeychainOnlyOnDarwin(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		goos     string
		keychain bool
	}{{agentio.OSDarwin, true}, {agentio.OSLinux, false}, {agentio.OSWindows, false}} {
		secrets := agenttest.NewFakeSecrets()
		a, dir := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithSecrets(secrets), agenttest.WithNow(agenttest.FixedNow), agenttest.WithSystem(tc.goos, ""))
		if err := os.Remove(filepath.Join(dir, ".claude", credentialsFile)); err != nil {
			t.Fatal(err)
		}
		h := a.DefaultHome()
		secrets.Put(KeychainServices(h.Dir(), true)[0], "user", `{"claudeAiOauth":{"accessToken":"kc","expiresAt":1893456000000}}`)
		_, err := a.Credential(h)
		if got := err == nil; got != tc.keychain {
			t.Errorf("%s: keychain credential found=%v, want %v (%v)", tc.goos, got, tc.keychain, err)
		}
	}
}

// windowsAgent copies the Windows fixture user home (%USERPROFILE%: an npm
// global install as %APPDATA%\npm\claude.cmd, a BOM+CRLF settings.json, a
// history folder keyed by C:\Users\user\proj) and builds an agent with a
// windows Host: "Path" spelled as Windows does, PATHEXT, no exec bits.
func windowsAgent(t *testing.T) (*Agent, string) {
	t.Helper()
	dir := t.TempDir()
	agenttest.CopyFixture(t, filepath.Join("testdata", "windows", "userhome"), dir)
	npmDir := filepath.Join(dir, "AppData", "Roaming", "npm")
	h := agenttest.NewHost(t, agenttest.WithUserHome(dir), agenttest.WithSystem(agentio.OSWindows, ""),
		agenttest.WithEnv("Path="+npmDir, "PATHEXT=.COM;.EXE;.BAT;.CMD", "USERPROFILE="+dir))
	h.User = "user"
	return New(h), dir
}

// TestWindowsFixtureHome runs the Windows layout on any machine: PATHEXT
// finds the npm .cmd shim, the shim leads to the package and its version,
// a BOM+CRLF settings.json decodes, and a conversation recorded under a
// drive-letter path is found by that path.
func TestWindowsFixtureHome(t *testing.T) {
	t.Parallel()
	a, dir := windowsAgent(t)
	in, err := a.Install()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(in.Path) != "claude.cmd" || in.Method != InstallNPMGlobal || in.Version.String() != "2.1.325" {
		t.Fatalf("install %+v", in.Install)
	}
	up, err := a.UpdateCommand()
	if err != nil || up.Path != in.Path {
		t.Fatalf("update %v %v", up.Path, err)
	}
	us, err := a.UserSettings(a.DefaultHome())
	if err != nil || us.Settings.Model != "opus" {
		t.Fatalf("BOM+CRLF settings.json: %+v %v", us.Settings, err)
	}
	if got := a.DefaultHome().Dir(); got != filepath.Join(dir, ".claude") {
		t.Fatalf("default home %s", got)
	}
	convs, err := a.Conversations(context.Background(), a.DefaultHome(), agentio.ConversationQuery{Dir: `C:\Users\user\proj`})
	if err != nil || len(convs) != 1 || convs[0].Title != "Windows fixture" {
		t.Fatalf("conversations %+v %v", convs, err)
	}
}

// TestInstallLayouts covers Claude's install layouts per OS, each built as
// it lies on disk.
func TestInstallLayouts(t *testing.T) {
	t.Parallel()
	const pkgJSON = `{"version":"2.1.325"}`
	exec := func(t *testing.T, p string) {
		t.Helper()
		agenttest.WriteFile(t, filepath.Dir(p), filepath.Base(p), "#!/bin/sh\n")
		if err := os.Chmod(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name    string
		goos    string
		build   func(t *testing.T, home string) string
		method  agentio.InstallMethod
		version string
	}{
		{"native symlink (linux, macOS)", agentio.OSLinux, func(t *testing.T, home string) string {
			ver := filepath.Join(home, ".local", "share", "claude", "versions", "2.1.325")
			exec(t, ver)
			bin := filepath.Join(home, ".local", "bin")
			if err := os.MkdirAll(bin, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(ver, filepath.Join(bin, Binary)); err != nil {
				t.Fatal(err)
			}
			return bin
		}, agentio.InstallNative, "2.1.325"},
		{"native copied exe (windows)", agentio.OSWindows, func(t *testing.T, home string) string {
			agenttest.WriteFile(t, filepath.Join(home, ".local", "share", "claude", "versions"), "2.1.323.exe", "MZ")
			agenttest.WriteFile(t, filepath.Join(home, ".local", "share", "claude", "versions"), "2.1.325.exe", "MZ")
			bin := filepath.Join(home, ".local", "bin")
			agenttest.WriteFile(t, bin, "claude.exe", "MZ")
			return bin
		}, agentio.InstallNative, "2.1.325"},
		{"pnpm sh shim", agentio.OSLinux, func(t *testing.T, home string) string {
			bin := filepath.Join(home, ".local", "share", "pnpm")
			agenttest.WriteFile(t, filepath.Join(bin, "global", "5", "node_modules", "@anthropic-ai", "claude-code"), "package.json", pkgJSON)
			agenttest.WriteFile(t, bin, Binary, "#!/bin/sh\nexec node \"$basedir/global/5/node_modules/@anthropic-ai/claude-code/cli.js\" \"$@\"\n")
			if err := os.Chmod(filepath.Join(bin, Binary), 0o700); err != nil {
				t.Fatal(err)
			}
			return bin
		}, agentio.InstallPNPM, "2.1.325"},
		{"bun", agentio.OSDarwin, func(t *testing.T, home string) string {
			bin := filepath.Join(home, ".bun", "bin")
			agenttest.WriteFile(t, filepath.Join(home, ".bun", "install", "global", "node_modules", "@anthropic-ai", "claude-code"), "package.json", pkgJSON)
			exec(t, filepath.Join(bin, Binary))
			return bin
		}, agentio.InstallBun, "2.1.325"},
		{"volta (windows layout)", agentio.OSWindows, func(t *testing.T, home string) string {
			vh := filepath.Join(home, "AppData", "Local", "Volta")
			agenttest.WriteFile(t, filepath.Join(vh, "tools", "image", "packages", "@anthropic-ai", "claude-code", "node_modules", "@anthropic-ai", "claude-code"), "package.json", pkgJSON)
			bin := filepath.Join(vh, "bin")
			agenttest.WriteFile(t, bin, "claude.exe", "MZ")
			return bin
		}, agentio.InstallVolta, "2.1.325"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			home := t.TempDir()
			bin := tc.build(t, home)
			a := New(agenttest.NewHost(t, agenttest.WithUserHome(home), agenttest.WithSystem(tc.goos, ""), agenttest.WithEnv("PATH="+bin, "PATHEXT=.EXE;.CMD")))
			in, err := a.Install()
			if err != nil {
				t.Fatal(err)
			}
			if in.Method != tc.method || in.Version.String() != tc.version {
				t.Fatalf("install %s %s (%+v)", in.Method, in.Version, in.Install)
			}
		})
	}
}

// TestManagedRegistry reads the Windows policy values with reg.exe (HKLM,
// then HKCU); an absent key is skipped, a non-windows Host reads nothing.
func TestManagedRegistry(t *testing.T) {
	t.Parallel()
	reg := filepath.Join("/sysroot", "Windows", "System32", "reg.exe")
	key := func(k string) string {
		return agentio.NewCommand(reg, []string{regQuery, k, regValueFlag, regValueName}, nil, "").String()
	}
	fx := &agenttest.FakeExec{
		Results: map[string]agentio.ProcessResult{
			key(regKeyHKLM): {Stdout: []byte("\r\nHKEY_LOCAL_MACHINE\\SOFTWARE\\Policies\\ClaudeCode\r\n    Settings    REG_SZ    {\"model\": \"opus\", \"cleanupPeriodDays\": 7}\r\n\r\n")},
		},
		Default: agentio.ProcessResult{ExitCode: 1, Stderr: []byte("ERROR: The system was unable to find the specified registry key or value.")},
	}
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithExec(fx), agenttest.WithSystem(agentio.OSWindows, "/sysroot"))
	layers, err := a.Runner().ManagedRegistry(context.Background())
	if err != nil || len(layers) != 1 || layers[0].Source.Kind != SourceRegistryHKLM || layers[0].Settings.Model != "opus" {
		t.Fatalf("%+v %v", layers, err)
	}
	if v, ok := parseRegValue([]byte("    Settings    REG_DWORD    0x1\n"), regValueName); ok {
		t.Fatalf("a non-string value is not settings JSON: %q", v)
	}
	lin, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithExec(fx), agenttest.WithSystem(agentio.OSLinux, "/"))
	if got, err := lin.Runner().ManagedRegistry(context.Background()); got != nil || err != nil {
		t.Fatalf("linux host read the registry: %v %v", got, err)
	}
}
