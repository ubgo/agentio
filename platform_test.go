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
// installer), needs no exec bit and accepts rooted entries without a drive
// (\pa); unix needs the exec bit and splits on ':'. The rules run on a fake
// stat keyed by each rule's own path syntax, written literally (never with
// the runner's filepath), so a case means the same on every OS; Host.LookPath
// then runs the Windows rule on real files.
func TestLookPathPerOS(t *testing.T) {
	t.Parallel()
	files := map[string]os.FileMode{
		`\pa\codex.cmd`:    0o644,
		`\pb\claude.exe`:   0o644,
		`C:\pc\gemini.cmd`: 0o644,
		"/pb/codex":        0o755,
		"/pa/claude":       0o644, // no exec bit: never a unix hit
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
		{OSWindows, `\pa;\pb`, "", "codex", []string{`\pa\codex.cmd`}},
		{OSWindows, `\pa;\pb`, ".EXE;.CMD", "claude", []string{`\pb\claude.exe`}},
		{OSWindows, `relative;C:\pc\`, "", "gemini", []string{`C:\pc\gemini.cmd`}},
		{OSLinux, "/pa:/pb", "", "codex", []string{"/pb/codex"}},
		{OSLinux, "/pa:/pb", "", "claude", nil},
		{OSLinux, "relative:/pb", "", "codex", []string{"/pb/codex"}},
		{OSDarwin, "/pa:/pb", "", "codex", []string{"/pb/codex"}},
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
	ps51 := []byte("\xEF\xBB\xBF" + `[{"ProcessId":4242,"ParentProcessId":1,"CreationDate":"\/Date(1794639376159)\/","ExecutablePath":"C:\\Users\\u\\AppData\\Roaming\\npm\\node.exe","CommandLine":"\"C:\\Program Files\\nodejs\\node.exe\" C:\\Users\\u\\AppData\\Roaming\\npm\\node_modules\\@openai\\codex\\bin\\codex.js exec \"a b\"","WorkingSetSize":52428800,"UserModeTime":312500,"KernelModeTime":156250},{"ProcessId":4,"ParentProcessId":0,"CreationDate":null,"ExecutablePath":null,"CommandLine":null,"WorkingSetSize":null,"UserModeTime":null,"KernelModeTime":null}]`)
	procs, err := parseWin32Processes(ps51)
	if err != nil || len(procs) != 2 {
		t.Fatalf("%v %v", procs, err)
	}
	p := procs[0]
	if p.PID != 4242 || p.PPID != 1 || !p.Start.Equal(time.UnixMilli(1794639376159)) || len(p.Argv) != 4 || p.Argv[0] != `C:\Program Files\nodejs\node.exe` || p.Argv[3] != "a b" {
		t.Errorf("row = %+v", p)
	}
	if p.RSS != 52428800 || p.CPUTime != 46875*time.Microsecond {
		t.Errorf("ps51 resources = RSS %d CPU %v", p.RSS, p.CPUTime)
	}
	if p.CommandLine != `"C:\Program Files\nodejs\node.exe" C:\Users\u\AppData\Roaming\npm\node_modules\@openai\codex\bin\codex.js exec "a b"` {
		t.Errorf("raw command line = %q", p.CommandLine)
	}
	if procs[1].RSS != 0 || procs[1].CPUTime != 0 || procs[1].CommandLine != "" {
		t.Errorf("null resource columns = %+v", procs[1])
	}
	ps7 := []byte(`{"ProcessId":7,"ParentProcessId":2,"CreationDate":"2026-11-14T07:31:00.5+00:00","ExecutablePath":"","CommandLine":"claude.exe  --resume x","WorkingSetSize":104857600,"UserModeTime":15000000,"KernelModeTime":5000000}`)
	one, err := parseWin32Processes(ps7)
	if err != nil || len(one) != 1 || one[0].Exe != "claude.exe" || one[0].Start.IsZero() {
		t.Fatalf("ps7 = %+v %v", one, err)
	}
	if q := one[0]; q.RSS != 104857600 || q.CPUTime != 2*time.Second || q.TTY != "" || q.CommandLine != "claude.exe  --resume x" {
		t.Errorf("ps7 resources = RSS %d CPU %v TTY %q line %q", q.RSS, q.CPUTime, q.TTY, q.CommandLine)
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
// real reader does (no `ps`, so it works on Alpine and minimal images):
// start time, CPU time and RSS from stat (comm with spaces and a ")"),
// the raw command line, and the tty from the fd/0 link (a terminal, a
// non-terminal and an unreadable link).
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
	link := func(target, rel string) bool {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		return os.Symlink(target, p) == nil
	}
	// Fields after comm: state ppid pgrp session tty_nr tpgid flags minflt
	// cminflt majflt cmajflt utime(14) stime(15) cutime cstime priority
	// nice threads itrealvalue starttime(22) vsize rss(24).
	write("stat", "cpu 1 2 3\nbtime 1794600000\n")
	write("4242/stat", "4242 (claude) S 77 4242 4242 0 -1 0 0 0 0 0 250 50 0 0 20 0 1 0 1000 123456 300\n")
	write("4242/cmdline", "node\x00/opt/my tools/bin/claude\x00--resume\x00x\x00")
	write("4343/stat", "4343 (odd) name) S 1 4343 4343 0 -1 0 0 0 0 0 1 2 0 0 20 0 1 0 2000 0 7\n")
	write("4343/cmdline", "pi\x00")
	write("self/stat", "not a pid dir")
	symlinks := link("/dev/pts/3", "4242/fd/0") && link("/dev/null", "4343/fd/0")
	procs, err := linuxProcessesAt(root, 4096)
	if err != nil || len(procs) != 2 {
		t.Fatalf("%+v %v", procs, err)
	}
	slices.SortFunc(procs, func(a, b Process) int { return a.PID - b.PID })
	p := procs[0]
	if p.PID != 4242 || p.PPID != 77 || !slices.Equal(p.Argv, []string{"node", "/opt/my tools/bin/claude", "--resume", "x"}) || !p.Start.Equal(time.Unix(1794600000+10, 0)) {
		t.Errorf("proc = %+v", p)
	}
	if p.CommandLine != "node /opt/my tools/bin/claude --resume x" || p.CPUTime != 3*time.Second || p.RSS != 300*4096 {
		t.Errorf("resources = line %q CPU %v RSS %d", p.CommandLine, p.CPUTime, p.RSS)
	}
	q := procs[1]
	if q.PPID != 1 || q.CPUTime != 30*time.Millisecond || q.RSS != 7*4096 || q.TTY != "" || q.CommandLine != "pi" {
		t.Errorf("comm with \")\" = %+v", q)
	}
	if symlinks && p.TTY != "pts/3" {
		t.Errorf("tty = %q, want pts/3", p.TTY)
	}
	if !symlinks && p.TTY != "" {
		t.Errorf("unreadable fd/0: tty = %q, want empty", p.TTY)
	}
}

// TestParseDarwinPS keeps the macOS reader's shape: one row per line with
// rss (KiB), tty ("??" = none), cumulative CPU time and the raw command
// line, whose runs of spaces survive (a path like "Application Support"
// is not cut in two there).
func TestParseDarwinPS(t *testing.T) {
	t.Parallel()
	out := []byte("" +
		"  4242     1 Fri Nov 13 19:38:43 2026  1276560 ttys000   72:06.69 /Users/alice/.local/bin/claude --resume x\n" +
		"  837   640 Wed Nov 18 10:30:41 2026     499312 ??         2:22.87 /Users/alice/Library/Application Support/Claude/helper  --type=renderer\n" +
		" bad line\n" +
		"  9  1 Fri Nov 13 19:38:43 2026 notanumber ?? junk /bin/x\n")
	procs := parseDarwinPS(out, time.UTC)
	if len(procs) != 3 || procs[0].PID != 4242 || procs[0].Argv[1] != "--resume" || procs[0].Start.Day() != 13 {
		t.Fatalf("%+v", procs)
	}
	p := procs[0]
	if p.RSS != 1276560*1024 || p.TTY != "ttys000" || p.CPUTime != 72*time.Minute+6690*time.Millisecond || p.CommandLine != "/Users/alice/.local/bin/claude --resume x" {
		t.Errorf("row = RSS %d TTY %q CPU %v line %q", p.RSS, p.TTY, p.CPUTime, p.CommandLine)
	}
	h := procs[1]
	if h.TTY != "" || h.CommandLine != "/Users/alice/Library/Application Support/Claude/helper  --type=renderer" || h.Argv[0] != "/Users/alice/Library/Application" {
		t.Errorf("helper = TTY %q line %q argv %q", h.TTY, h.CommandLine, h.Argv)
	}
	if bad := procs[2]; bad.RSS != 0 || bad.CPUTime != 0 || bad.CommandLine != "/bin/x" {
		t.Errorf("unparsable columns = %+v", bad)
	}
}

// TestParsePSTime covers every cumulative CPU time shape ps prints.
func TestParsePSTime(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]time.Duration{
		"0:01.23":    1230 * time.Millisecond,
		"12:34.56":   12*time.Minute + 34560*time.Millisecond,
		"462:03.95":  462*time.Minute + 3950*time.Millisecond,
		"1:02:03":    time.Hour + 2*time.Minute + 3*time.Second,
		"1-02:03:04": 26*time.Hour + 3*time.Minute + 4*time.Second,
		"00:00:00":   0,
	} {
		if got, ok := parsePSTime(in); !ok || got != want {
			t.Errorf("parsePSTime(%q) = %v %v, want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "junk", "1", "a:b", "1:2:3:4", "x-1:00", "1:-5"} {
		if _, ok := parsePSTime(in); ok {
			t.Errorf("parsePSTime(%q) accepted", in)
		}
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
		{OSWindows, `C:\Users\u`, `.claude-w`, `C:\Users\u\.claude-w`},
		{OSWindows, `C:\Users\u`, `/work/h`, `C:\work\h`},
		{OSLinux, "/home/u", "/work/h", "/work/h"},
		{OSLinux, "/home/u", ".claude-w", "/home/u/.claude-w"},
		{OSDarwin, "/Users/u", "/work/h", "/work/h"},
	} {
		if got := resolveHomeDir(tc.goos, tc.home, tc.dir); got != tc.want {
			t.Errorf("%s %q in %q = %q, want %q", tc.goos, tc.dir, tc.home, got, tc.want)
		}
	}
}
