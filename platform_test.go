package agentio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The tests in this file prove each OS difference on any machine: the OS
// is a parameter (Host.GOOS or a goos argument), never the OS the test
// runs on.

// TestEnvNamesFoldOnWindows: env names are case-insensitive on Windows only,
// for lookups and for edits (a PATH change also drops an inherited Path).
func TestEnvNamesFoldOnWindows(t *testing.T) {
	t.Parallel()
	env := []string{"Path=C:\\bin", "HOME=/h"}
	if v, ok := EnvLookupFor(OSWindows, env, "PATH"); !ok || v != `C:\bin` {
		t.Errorf("windows PATH lookup = %q %v", v, ok)
	}
	if _, ok := EnvLookupFor(OSLinux, env, "PATH"); ok {
		t.Error("linux lookup must be case-sensitive")
	}
	got := ApplyEnvFor(OSWindows, env, EnvSet("PATH", `D:\x`))
	if !slices.Equal(got, []string{"HOME=/h", `PATH=D:\x`}) {
		t.Errorf("windows edit = %v", got)
	}
	got = ApplyEnvFor(OSLinux, env, EnvSet("PATH", "/x"))
	if !slices.Equal(got, []string{`Path=C:\bin`, "HOME=/h", "PATH=/x"}) {
		t.Errorf("linux edit = %v", got)
	}
	h := Host{GOOS: OSWindows, Env: []string{"codex_home=C:\\c"}}
	if v, ok := h.Getenv("CODEX_HOME"); !ok || v != `C:\c` {
		t.Errorf("Host.Getenv on windows = %q %v", v, ok)
	}
	// The default home rule removes the variable in every spelling.
	cmd := h.NewCommand("codex", nil, "", EnvUnset("CODEX_HOME"))
	if len(cmd.Env) != 0 {
		t.Errorf("windows unset left %v", cmd.Env)
	}
}

// TestLookPathPerOS: Windows splits PATH on ';', tries PATHEXT extensions
// for a bare name (codex.cmd from npm, claude.exe from the native
// installer) and needs no exec bit; unix needs the exec bit and splits on
// ':'. The rules run on a fake stat, so a case means the same on every OS
// (a Windows filesystem cannot store exec bits); Host.LookPath then runs
// the Windows rule on real files.
func TestLookPathPerOS(t *testing.T) {
	t.Parallel()
	a, b := filepath.FromSlash("/pa"), filepath.FromSlash("/pb")
	files := map[string]os.FileMode{
		filepath.Join(a, "codex.cmd"):  0o644,
		filepath.Join(b, "claude.exe"): 0o644,
		filepath.Join(b, "codex"):      0o755,
		filepath.Join(a, "claude"):     0o644, // no exec bit: never a unix hit
	}
	base, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stat := func(p string) (os.FileInfo, error) {
		if m, ok := files[p]; ok {
			return fakeFileInfo{FileInfo: base, mode: m}, nil
		}
		return nil, os.ErrNotExist
	}
	for _, tc := range []struct {
		goos, list, ext, name string
		want                  []string
	}{
		{OSWindows, a + ";" + b, "", "codex", []string{filepath.Join(a, "codex.cmd")}},
		{OSWindows, a + ";" + b, ".EXE;.CMD", "claude", []string{filepath.Join(b, "claude.exe")}},
		{OSLinux, a + ":" + b, "", "codex", []string{filepath.Join(b, "codex")}},
		{OSLinux, a + ":" + b, "", "claude", nil},
		{OSDarwin, a + ":" + b, "", "codex", []string{filepath.Join(b, "codex")}},
	} {
		if got := lookPathAll(tc.goos, tc.list, tc.ext, tc.name, stat); !slices.Equal(got, tc.want) {
			t.Errorf("%s %s = %v, want %v", tc.goos, tc.name, got, tc.want)
		}
	}
	real := t.TempDir()
	shim := filepath.Join(real, "codex.cmd")
	if err := os.WriteFile(shim, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := Host{GOOS: OSWindows, Env: []string{"Path=" + real, "PATHEXT=.CMD"}}
	if p, err := h.LookPath("codex"); err != nil || p != shim {
		t.Errorf("Host.LookPath windows = %q %v", p, err)
	}
	if _, err := h.LookPath("nothere"); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("missing = %v", err)
	}
	if !windowsAbs(`C:\Users\x`) || !windowsAbs(`\\srv\share`) || windowsAbs(`relative\dir`) {
		t.Error("windowsAbs")
	}
}

// fakeFileInfo is a real regular file's FileInfo with the mode replaced,
// for the stat seam.
type fakeFileInfo struct {
	os.FileInfo
	mode os.FileMode
}

// Mode is the faked mode. Pure.
func (f fakeFileInfo) Mode() os.FileMode { return f.mode }

// TestShimTarget reads the three cmd-shim shapes npm and pnpm write.
func TestShimTarget(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"codex.cmd": "@ECHO off\r\nSETLOCAL\r\nSET dp0=%~dp0\r\n\"%_prog%\"  \"%dp0%\\node_modules\\@openai\\codex\\bin\\codex.js\" %*\r\n",
		"codex.ps1": "$basedir=Split-Path $MyInvocation.MyCommand.Definition -Parent\n& \"$basedir/node_modules/@openai/codex/bin/codex.js\" $args\n",
		"codex":     "#!/bin/sh\nbasedir=$(dirname \"$(echo \"$0\" | sed -e 's,\\\\,/,g')\")\nexec node  \"$basedir/global/5/node_modules/@openai/codex/bin/codex.js\" \"$@\"\n",
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		got, ok := ShimTarget(p)
		if !ok || !strings.HasPrefix(got, dir) || filepath.Base(got) != "codex.js" {
			t.Errorf("%s: %q %v", name, got, ok)
		}
	}
	bin := filepath.Join(dir, "codex.exe")
	if err := os.WriteFile(bin, []byte{'M', 'Z', 0, 0}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := ShimTarget(bin); ok {
		t.Error("a binary is not a shim")
	}
}

// TestParseWin32Processes decodes both PowerShell date shapes, a single
// object, a null command line, and splits the command line like
// CommandLineToArgvW.
func TestParseWin32Processes(t *testing.T) {
	t.Parallel()
	ps51 := []byte("\xEF\xBB\xBF" + `[{"ProcessId":4242,"ParentProcessId":1,"CreationDate":"\/Date(1794639376159)\/","ExecutablePath":"C:\\Users\\u\\AppData\\Roaming\\npm\\node.exe","CommandLine":"\"C:\\Program Files\\nodejs\\node.exe\" C:\\Users\\u\\AppData\\Roaming\\npm\\node_modules\\@openai\\codex\\bin\\codex.js exec \"a b\""},{"ProcessId":4,"ParentProcessId":0,"CreationDate":null,"ExecutablePath":null,"CommandLine":null}]`)
	procs, err := parseWin32Processes(ps51)
	if err != nil || len(procs) != 2 {
		t.Fatalf("%v %v", procs, err)
	}
	p := procs[0]
	if p.PID != 4242 || p.PPID != 1 || !p.Start.Equal(time.UnixMilli(1794639376159)) || len(p.Argv) != 4 || p.Argv[0] != `C:\Program Files\nodejs\node.exe` || p.Argv[3] != "a b" {
		t.Errorf("row = %+v", p)
	}
	ps7 := []byte(`{"ProcessId":7,"ParentProcessId":2,"CreationDate":"2026-11-14T07:31:00.5+00:00","ExecutablePath":"","CommandLine":"claude.exe --resume x"}`)
	one, err := parseWin32Processes(ps7)
	if err != nil || len(one) != 1 || one[0].Exe != "claude.exe" || one[0].Start.IsZero() {
		t.Fatalf("ps7 = %+v %v", one, err)
	}
	for in, want := range map[string][]string{
		`a "b c" d`:         {"a", "b c", "d"},
		`a\\\"b`:            {`a\"b`},
		`"a\\" b`:           {`a\`, "b"},
		`C:\dir\x.exe  arg`: {`C:\dir\x.exe`, "arg"},
		`"say ""hi"""`:      {`say "hi"`},
		``:                  nil,
	} {
		if got := splitWindowsCommandLine(in); !slices.Equal(got, want) {
			t.Errorf("split(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestLinuxProcessesFromProcTree reads a fixture /proc tree the way the
// real reader does (no `ps`, so it works on Alpine and minimal images).
func TestLinuxProcessesFromProcTree(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("stat", "cpu 1 2 3\nbtime 1794600000\n")
	write("4242/stat", "4242 (claude) S 77 4242 4242 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 1000 0 0\n")
	write("4242/cmdline", "claude\x00--resume\x00x\x00")
	write("self/stat", "not a pid dir")
	procs, err := linuxProcessesAt(root)
	if err != nil || len(procs) != 1 {
		t.Fatalf("%+v %v", procs, err)
	}
	p := procs[0]
	if p.PID != 4242 || p.PPID != 77 || !slices.Equal(p.Argv, []string{"claude", "--resume", "x"}) || !p.Start.Equal(time.Unix(1794600000+10, 0)) {
		t.Errorf("proc = %+v", p)
	}
}

// TestParseDarwinPS keeps the macOS reader's shape.
func TestParseDarwinPS(t *testing.T) {
	t.Parallel()
	out := []byte("  4242     1 Fri Nov 13 19:38:43 2026 /Users/u/.local/bin/claude --resume x\n bad line\n")
	procs := parseDarwinPS(out, time.UTC)
	if len(procs) != 1 || procs[0].PID != 4242 || procs[0].Argv[1] != "--resume" || procs[0].Start.Day() != 13 {
		t.Fatalf("%+v", procs)
	}
}

// TestSecretToolResult maps secret-tool exits: secret, no entry, and an
// unreachable Secret Service (ErrUnsupported with the reason, no secret).
func TestSecretToolResult(t *testing.T) {
	t.Parallel()
	tok, found, err := secretToolResult([]byte("s3cr3t\n"), nil, 0)
	if err != nil || !found || tok.Reveal() != "s3cr3t" {
		t.Fatalf("hit: %v %v", found, err)
	}
	if _, found, err := secretToolResult(nil, nil, secretToolNotFoundExit); found || err != nil {
		t.Fatalf("miss: %v %v", found, err)
	}
	_, found, err = secretToolResult(nil, []byte("Cannot autolaunch D-Bus without X11 $DISPLAY\n"), 1)
	if found || !errors.Is(err, ErrUnsupported) {
		t.Fatalf("no dbus: %v %v", found, err)
	}
}

// TestSymlinkPrivilegeError: ERROR_PRIVILEGE_NOT_HELD from os.Symlink (no
// Developer Mode on Windows) becomes a typed error matching ErrUnsupported.
func TestSymlinkPrivilegeError(t *testing.T) {
	t.Parallel()
	err := &os.LinkError{Op: "symlink", Old: "a", New: "b", Err: syscall.Errno(1314)}
	if !isSymlinkPrivilegeError(err) {
		t.Fatal("1314 not recognised")
	}
	se := &SymlinkPrivilegeError{Link: "b", Err: err}
	if !errors.Is(se, ErrUnsupported) || !errors.As(error(se), new(*os.LinkError)) {
		t.Fatal("SymlinkPrivilegeError must match ErrUnsupported and unwrap the os error")
	}
	if isSymlinkPrivilegeError(&os.LinkError{Err: syscall.ENOENT}) {
		t.Fatal("ENOENT is not a privilege error")
	}
}

// TestRunnerResolvesOnCommandEnv: the OS runner resolves a bare
// Command.Path on the Command's own PATH, not the test process's, and a
// name on neither is ErrNotInstalled.
func TestRunnerResolvesOnCommandEnv(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == OSWindows {
		t.Skip("uses a POSIX sh script")
	}
	bin := t.TempDir()
	script := filepath.Join(bin, "agentio-probe-bin")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho probe-ok\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	res, err := osRunner{}.Run(context.Background(), NewCommand("agentio-probe-bin", nil, []string{"PATH=" + bin}, ""), nil)
	if err != nil || strings.TrimSpace(string(res.Stdout)) != "probe-ok" {
		t.Fatalf("%q %v", res.Stdout, err)
	}
	if _, err := (osRunner{}).Run(context.Background(), NewCommand("agentio-probe-bin", nil, []string{"PATH=/nonexistent"}, ""), nil); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("not on the command PATH: %v", err)
	}
}

// TestResolveHomeDirPerOS: a home variable's value is made absolute the way
// the OS would: relative under the user home; on Windows a rooted path
// without a drive lands on the user home's drive.
func TestResolveHomeDirPerOS(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ goos, home, dir, want string }{
		{OSWindows, `C:\Users\u`, `\work\h`, `C:\work\h`},
		{OSWindows, `\\srv\share\u`, `\work\h`, `\\srv\share\work\h`},
		{OSWindows, `C:\Users\u`, `D:\h`, `D:\h`},
		{OSLinux, "/home/u", "/work/h", "/work/h"},
	} {
		if got := resolveHomeDir(tc.goos, tc.home, tc.dir); got != tc.want {
			t.Errorf("%s %q in %q = %q, want %q", tc.goos, tc.dir, tc.home, got, tc.want)
		}
	}
}
