package agentio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

// osProcesses reads the process table.
type osProcesses struct{}

// Processes is Process · Read: /proc read directly on linux (no `ps`, so
// minimal and Alpine/musl images work), one `ps` run on darwin, one
// PowerShell Get-CimInstance Win32_Process run on windows (stdlib only: no
// cgo, no x/sys). Other OSes return ErrUnsupported with the reason.
func (osProcesses) Processes(ctx context.Context) ([]Process, error) {
	switch runtime.GOOS {
	case goosLinux:
		return linuxProcessesAt(procRoot, os.Getpagesize())
	case goosDarwin:
		return darwinProcesses(ctx)
	case goosWindows:
		return windowsProcesses(ctx)
	}
	return nil, UnsupportedError("", "process table", "no reader for "+runtime.GOOS)
}

// Cwd is Process · Read: /proc/{pid}/cwd on linux, `lsof` on darwin. On
// windows it is ErrUnsupported: another process's working directory is only
// in its PEB, which needs reading that process's memory (no stdlib or
// Win32_Process field exposes it). Live sessions still match on windows
// through the session files, which record the cwd themselves.
func (osProcesses) Cwd(ctx context.Context, pid int) (string, error) {
	switch runtime.GOOS {
	case goosLinux:
		return os.Readlink(filepath.Join(procRoot, strconv.Itoa(pid), "cwd"))
	case goosDarwin:
		return darwinCwd(ctx, pid)
	case goosWindows:
		return "", UnsupportedError("", "process cwd", reasonWindowsCwd)
	}
	return "", UnsupportedError("", "process cwd", "no reader for "+runtime.GOOS)
}

// Env is Process · Read: /proc/{pid}/environ on linux (NUL-separated),
// `ps -E` on darwin (space-split, mixed with the command line; see
// ProcessEnvReader). Windows keeps another process's environment only in
// its memory (PEB), so there it is ErrUnsupported.
func (osProcesses) Env(ctx context.Context, pid int) ([]string, error) {
	switch runtime.GOOS {
	case goosLinux:
		return linuxEnvAt(procRoot, pid)
	case goosDarwin:
		return darwinEnv(ctx, pid)
	case goosWindows:
		return nil, UnsupportedError("", "process env", reasonWindowsEnv)
	}
	return nil, UnsupportedError("", "process env", "no reader for "+runtime.GOOS)
}

// reasonWindowsEnv is why Env is unsupported on windows.
const reasonWindowsEnv = "windows exposes another process's environment only in its memory (PEB)"

// linuxEnvAt reads root/{pid}/environ. Disk · Read.
func linuxEnvAt(root string, pid int) ([]string, error) {
	b, err := os.ReadFile(filepath.Join(root, strconv.Itoa(pid), "environ"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, kv := range bytes.Split(b, []byte{0}) {
		if len(kv) > 0 {
			out = append(out, string(kv))
		}
	}
	return out, nil
}

// darwinEnv runs `ps -E -ww -o command= -p pid` and returns the
// space-split fields that look like NAME=value. Process · Read.
func darwinEnv(ctx context.Context, pid int) ([]string, error) {
	cmd := exec.CommandContext(ctx, psBin, "-E", "-ww", "-o", "command=", "-p", strconv.Itoa(pid))
	cmd.Env = []string{envLocaleC}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("agentio: ps -E: %w", err)
	}
	return darwinEnvFrom(pid, string(out))
}

// darwinEnvFrom turns `ps -E` output into the environment. darwin prints
// no environment at all for a process it will not expose (a protected
// platform binary, a hardened process): an empty result is "unreadable",
// never "an environment without the variable", or ProcessAlive would
// call a running process ended and a cleaner would delete its files.
// Every real process has at least one variable. Pure.
func darwinEnvFrom(pid int, out string) ([]string, error) {
	env := envFields(out)
	if len(env) == 0 {
		return nil, fmt.Errorf("%w: ps -E shows no environment for pid %d (darwin hides it for protected processes)", ErrUnsupported, pid)
	}
	return env, nil
}

// envFields keeps the space-separated fields shaped NAME=value. Pure.
func envFields(s string) []string {
	var out []string
	for _, f := range strings.Fields(s) {
		if i := strings.IndexByte(f, '='); i > 0 {
			out = append(out, f)
		}
	}
	return out
}

// reasonWindowsCwd is why Cwd is unsupported on windows.
const reasonWindowsCwd = "windows exposes another process's working directory only in its memory (PEB)"

// Linux /proc layout.
const (
	procRoot = "/proc"
	// procStat*Field are 0-based indexes into the fields after the ")"
	// that ends comm in /proc/{pid}/stat (proc(5) numbers fields from 1
	// and comm is field 2, so index = field - 3): field 4 ppid, 14 utime,
	// 15 stime, 22 starttime, 24 rss (pages).
	procStatPPIDField  = 1
	procStatUTimeField = 11
	procStatSTimeField = 12
	procStatStartField = 19
	procStatRSSField   = 21
	// clockTicks is USER_HZ, 100 on every linux ABI Go supports.
	clockTicks = 100
	procBtime  = "btime "
	// procStdinFD is the fd whose link names the controlling terminal:
	// stat's tty_nr is a device number that needs a device map to name.
	procStdinFD = "fd/0"
	devPrefix   = "/dev/"
)

// ttyLinkPrefixes are the terminal devices a stdin link may point at
// (pseudo terminals and virtual consoles); /dev/null, pipes and sockets
// are not terminals. Pure.
func ttyLinkPrefixes() []string { return []string{"/dev/pts/", "/dev/tty"} }

// linuxProcessesAt reads every <root>/{pid} (root is /proc; a test passes a
// fixture tree; pageSize converts stat's rss pages to bytes). Rows that
// vanish mid-read or are unreadable are skipped: the table changes while
// it is read. Each row is stat, cmdline and two links (exe, fd/0); an
// unreadable fd/0 (another user's process) leaves TTY "". Disk · Read.
func linuxProcessesAt(root string, pageSize int) ([]Process, error) {
	boot, err := linuxBootTime(root)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("agentio: read %s: %w", root, err)
	}
	var out []Process
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		dir := filepath.Join(root, e.Name())
		stat, err := os.ReadFile(filepath.Join(dir, "stat"))
		if err != nil {
			continue
		}
		p := Process{PID: pid}
		applyLinuxStat(&p, stat, boot, pageSize)
		if cmdline, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			raw := bytes.TrimRight(cmdline, "\x00")
			p.CommandLine = string(bytes.ReplaceAll(raw, []byte{0}, []byte{' '}))
			for _, a := range bytes.Split(raw, []byte{0}) {
				if len(a) > 0 {
					p.Argv = append(p.Argv, string(a))
				}
			}
		}
		p.TTY = linuxTTY(filepath.Join(dir, procStdinFD))
		if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
			p.Exe = exe
		} else if len(p.Argv) > 0 {
			p.Exe = p.Argv[0]
		}
		out = append(out, p)
	}
	return out, nil
}

// applyLinuxStat fills PPID, Start, CPUTime and RSS from one
// /proc/{pid}/stat body. comm (field 2) may hold spaces and parentheses,
// so the fields are counted after its last ")". Pure.
func applyLinuxStat(p *Process, stat []byte, boot time.Time, pageSize int) {
	i := bytes.LastIndexByte(stat, ')')
	if i < 0 {
		return
	}
	f := strings.Fields(string(stat[i+1:]))
	if len(f) <= procStatStartField {
		return
	}
	p.PPID, _ = strconv.Atoi(f[procStatPPIDField])
	if ticks, err := strconv.ParseInt(f[procStatStartField], 10, 64); err == nil {
		p.Start = boot.Add(ticksDuration(ticks))
	}
	utime, uerr := strconv.ParseInt(f[procStatUTimeField], 10, 64)
	stime, serr := strconv.ParseInt(f[procStatSTimeField], 10, 64)
	if uerr == nil && serr == nil {
		p.CPUTime = ticksDuration(utime + stime)
	}
	if len(f) > procStatRSSField {
		if pages, err := strconv.ParseUint(f[procStatRSSField], 10, 64); err == nil && pageSize > 0 {
			p.RSS = pages * uint64(pageSize)
		}
	}
}

// ticksDuration converts USER_HZ clock ticks to a duration. Pure.
func ticksDuration(ticks int64) time.Duration {
	return time.Duration(ticks) * time.Second / clockTicks
}

// linuxTTY names the terminal fdLink (a /proc/{pid}/fd/0 link) points at,
// without "/dev/" ("pts/3", "tty1"); "" when the link is unreadable or
// is not a terminal. Disk · Read.
func linuxTTY(fdLink string) string {
	target, err := os.Readlink(fdLink)
	if err != nil {
		return ""
	}
	for _, pre := range ttyLinkPrefixes() {
		if strings.HasPrefix(target, pre) {
			return strings.TrimPrefix(target, devPrefix)
		}
	}
	return ""
}

// linuxBootTime reads btime from <root>/stat.
func linuxBootTime(root string) (time.Time, error) {
	f, err := os.Open(filepath.Join(root, "stat"))
	if err != nil {
		return time.Time{}, fmt.Errorf("agentio: boot time: %w", err)
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), procBtime); ok {
			secs, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return time.Time{}, fmt.Errorf("agentio: boot time: %w", err)
			}
			return time.Unix(secs, 0), nil
		}
	}
	return time.Time{}, fmt.Errorf("agentio: boot time: no btime in %s/stat", root)
}

// darwin `ps` constants. lstart is printed as five fields in the C locale
// ("Fri Nov 13 19:38:43 2026"), so the environment pins LC_ALL=C.
const (
	psBin          = "/bin/ps"
	psLstartLayout = "Mon Jan _2 15:04:05 2006"
	psLstartFields = 5
	lsofBin        = "/usr/sbin/lsof"
	lsofNamePrefix = "n"
	envLocaleC     = "LC_ALL=C"
	// psColumns is the one table read: everything a row needs, with the
	// unsplit command last so it can hold spaces.
	psColumns = "pid=,ppid=,lstart=,rss=,tty=,time=,command="
	// psFixedFields is the count of whitespace-separated fields before
	// the command: pid, ppid, five lstart words, rss, tty, time.
	psFixedFields = 2 + psLstartFields + 3
	psRSSField    = 2 + psLstartFields
	psTTYField    = psRSSField + 1
	psTimeField   = psTTYField + 1
	// psRSSUnit is the unit of ps's rss column (KiB).
	psRSSUnit = 1024
)

// psNoTTY are the tty column values that mean "no controlling terminal"
// (darwin prints "??", procps "?", some BSDs "-"). Pure.
func psNoTTY() []string { return []string{"??", "?", "-"} }

// darwinProcesses runs one `ps -axww -o pid=,ppid=,lstart=,rss=,tty=,time=,command=`.
// Process · Read.
func darwinProcesses(ctx context.Context) ([]Process, error) {
	cmd := exec.CommandContext(ctx, psBin, "-axww", "-o", psColumns)
	cmd.Env = []string{envLocaleC}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("agentio: ps: %w", err)
	}
	return parseDarwinPS(out, time.Local), nil
}

// parseDarwinPS parses `ps -axww -o pid=,ppid=,lstart=,rss=,tty=,time=,command=`
// output (lstart in the C locale, read in loc). CommandLine is the rest of
// the line after the fixed columns, cut from the line itself so runs of
// spaces inside it survive; Argv is that rest split on whitespace. A row
// whose rss, tty or time column does not parse keeps the zero value for
// it. Pure.
func parseDarwinPS(out []byte, loc *time.Location) []Process {
	var procs []Process
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		f := strings.Fields(line)
		if len(f) < psFixedFields+1 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		p := Process{PID: pid, PPID: ppid, Argv: f[psFixedFields:], CommandLine: skipFields(line, psFixedFields)}
		if t, err := time.ParseInLocation(psLstartLayout, strings.Join(f[2:2+psLstartFields], " "), loc); err == nil {
			p.Start = t
		}
		if kib, err := strconv.ParseUint(f[psRSSField], 10, 64); err == nil {
			p.RSS = kib * psRSSUnit
		}
		if tty := f[psTTYField]; !slices.Contains(psNoTTY(), tty) {
			p.TTY = tty
		}
		if d, ok := parsePSTime(f[psTimeField]); ok {
			p.CPUTime = d
		}
		p.Exe = p.Argv[0]
		procs = append(procs, p)
	}
	return procs
}

// skipFields returns line after its first n whitespace-separated fields,
// with the whitespace before the rest removed and the rest untouched. Pure.
func skipFields(line string, n int) string {
	rest := line
	for range n {
		rest = strings.TrimLeft(rest, " \t")
		i := strings.IndexAny(rest, " \t")
		if i < 0 {
			return ""
		}
		rest = rest[i:]
	}
	return strings.TrimLeft(rest, " \t")
}

// ps time column separators: "[[dd-]hh:]mm:ss[.cc]". darwin prints
// minutes past 59 without hours ("462:03.95"); procps prints hours and a
// day count ("1-02:03:04").
const (
	psTimeDaySep = "-"
	psTimeSep    = ":"
	psTimeParts  = 3
)

// parsePSTime parses a ps cumulative CPU time: "0:01.23", "12:34.56",
// "462:03.95", "1:02:03", "1-02:03:04". false for anything else. Pure.
func parsePSTime(s string) (time.Duration, bool) {
	var total time.Duration
	if d, rest, ok := strings.Cut(s, psTimeDaySep); ok {
		days, err := strconv.ParseUint(d, 10, 32)
		if err != nil {
			return 0, false
		}
		total = time.Duration(days) * 24 * time.Hour
		s = rest
	}
	parts := strings.Split(s, psTimeSep)
	if len(parts) < 2 || len(parts) > psTimeParts {
		return 0, false
	}
	secs, err := strconv.ParseFloat(parts[len(parts)-1], 64)
	if err != nil || secs < 0 {
		return 0, false
	}
	total += time.Duration(secs * float64(time.Second))
	units := []time.Duration{time.Minute, time.Hour}
	for i, j := len(parts)-2, 0; i >= 0; i, j = i-1, j+1 {
		n, err := strconv.ParseUint(parts[i], 10, 32)
		if err != nil {
			return 0, false
		}
		total += time.Duration(n) * units[j]
	}
	return total, true
}

// darwinCwd asks lsof for the cwd of pid (`-Fn` prints "n<path>").
func darwinCwd(ctx context.Context, pid int) (string, error) {
	cmd := exec.CommandContext(ctx, lsofBin, "-a", "-d", "cwd", "-p", strconv.Itoa(pid), "-Fn")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("agentio: lsof cwd %d: %w", pid, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, lsofNamePrefix); ok && p != "" {
			return p, nil
		}
	}
	return "", fmt.Errorf("%w: cwd of %d", ErrNotFound, pid)
}

// windows process table: one PowerShell run, JSON out. Get-CimInstance
// Win32_Process gives pid, parent pid, start time, image path and the full
// command line; tasklist has no command line or start time.
const (
	powershellBin  = "powershell.exe"
	psWin32Command = "Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId,CreationDate,ExecutablePath,CommandLine,WorkingSetSize,UserModeTime,KernelModeTime | ConvertTo-Json -Compress"
	// win32TimeUnit is the unit of UserModeTime and KernelModeTime.
	win32TimeUnit = 100 * time.Nanosecond
	psTimeout     = 20 * time.Second
	// psDatePrefix and psDateSuffix wrap Windows PowerShell 5.1's JSON
	// DateTime ("\/Date(1794639376159)\/" decodes to "/Date(ms)/").
	psDatePrefix = "/Date("
	psDateSuffix = ")/"
)

// windowsProcesses runs PowerShell once and parses its JSON.
func windowsProcesses(ctx context.Context) ([]Process, error) {
	ctx, cancel := context.WithTimeout(ctx, psTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, powershellBin, "-NoProfile", "-NonInteractive", "-Command", psWin32Command)
	cmd.WaitDelay = osWaitDelay
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, UnsupportedError("", "process table", "powershell.exe is not on PATH")
	}
	if err != nil {
		return nil, fmt.Errorf("agentio: powershell Win32_Process: %w", err)
	}
	return parseWin32Processes(out)
}

// win32Process is one Win32_Process row as ConvertTo-Json writes it.
type win32Process struct {
	ProcessID       int     `json:"ProcessId"`
	ParentProcessID int     `json:"ParentProcessId"`
	CreationDate    psDate  `json:"CreationDate"`
	ExecutablePath  *string `json:"ExecutablePath"`
	CommandLine     *string `json:"CommandLine"`
	// WorkingSetSize is bytes; UserModeTime and KernelModeTime are 100 ns
	// units. All three are null for a process the caller may not inspect.
	WorkingSetSize *uint64 `json:"WorkingSetSize"`
	UserModeTime   *uint64 `json:"UserModeTime"`
	KernelModeTime *uint64 `json:"KernelModeTime"`
}

// psDate is a CreationDate in either PowerShell shape: Windows PowerShell
// 5.1 writes "/Date(<unix ms>)/", PowerShell 7 an ISO 8601 string, and a
// process the caller may not inspect has null.
type psDate struct{ t time.Time }

// UnmarshalJSON accepts both shapes and null. Pure.
func (d *psDate) UnmarshalJSON(b []byte) error {
	var s *string
	if err := json.Unmarshal(b, &s); err != nil || s == nil {
		return err
	}
	if ms, ok := strings.CutPrefix(*s, psDatePrefix); ok {
		ms = strings.TrimSuffix(ms, psDateSuffix)
		n, err := strconv.ParseInt(ms, 10, 64)
		if err != nil {
			return fmt.Errorf("agentio: CreationDate %q: %w", *s, err)
		}
		d.t = time.UnixMilli(n)
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, *s)
	if err != nil {
		return fmt.Errorf("agentio: CreationDate %q: %w", *s, err)
	}
	d.t = t
	return nil
}

// parseWin32Processes decodes ConvertTo-Json output: an array, or a bare
// object when only one process is visible. Argv is split from CommandLine
// with the Windows rules (splitWindowsCommandLine) and CommandLine keeps
// the raw string; Exe is ExecutablePath, else Argv[0]; RSS is the working
// set; CPUTime is user plus kernel time; TTY stays "" (Windows has no
// controlling terminal). Pure.
func parseWin32Processes(out []byte) ([]Process, error) {
	trimmed := bytes.TrimSpace(StripBOM(out))
	if len(trimmed) == 0 {
		return nil, nil
	}
	var rows []win32Process
	if trimmed[0] == '{' {
		var one win32Process
		if err := json.Unmarshal(trimmed, &one); err != nil {
			return nil, fmt.Errorf("agentio: Win32_Process JSON: %w", err)
		}
		rows = []win32Process{one}
	} else if err := json.Unmarshal(trimmed, &rows); err != nil {
		return nil, fmt.Errorf("agentio: Win32_Process JSON: %w", err)
	}
	procs := make([]Process, 0, len(rows))
	for _, r := range rows {
		p := Process{PID: r.ProcessID, PPID: r.ParentProcessID, Start: r.CreationDate.t}
		if r.CommandLine != nil {
			p.CommandLine = *r.CommandLine
			p.Argv = splitWindowsCommandLine(*r.CommandLine)
		}
		if r.WorkingSetSize != nil {
			p.RSS = *r.WorkingSetSize
		}
		if r.UserModeTime != nil || r.KernelModeTime != nil {
			var units uint64
			for _, v := range []*uint64{r.UserModeTime, r.KernelModeTime} {
				if v != nil {
					units += *v
				}
			}
			p.CPUTime = time.Duration(units) * win32TimeUnit
		}
		switch {
		case r.ExecutablePath != nil && *r.ExecutablePath != "":
			p.Exe = *r.ExecutablePath
		case len(p.Argv) > 0:
			p.Exe = p.Argv[0]
		}
		procs = append(procs, p)
	}
	return procs, nil
}

// splitWindowsCommandLine splits a Windows command line the way
// CommandLineToArgvW and the MSVC runtime do: whitespace separates
// arguments outside quotes; 2n backslashes before a quote become n
// backslashes and the quote toggles quoting; 2n+1 backslashes before a
// quote become n backslashes and a literal quote; other backslashes are
// literal; "" inside quotes is a literal quote. Pure.
func splitWindowsCommandLine(s string) []string {
	var args []string
	var cur strings.Builder
	inQuote, have := false, false
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\\':
			n := 0
			for i < len(s) && s[i] == '\\' {
				n++
				i++
			}
			if i < len(s) && s[i] == '"' {
				cur.WriteString(strings.Repeat(`\`, n/2))
				if n%2 == 1 {
					cur.WriteByte('"')
					i++
				}
			} else {
				cur.WriteString(strings.Repeat(`\`, n))
			}
			have = true
			continue
		case c == '"':
			if inQuote && i+1 < len(s) && s[i+1] == '"' {
				cur.WriteByte('"')
				i += 2
				continue
			}
			inQuote = !inQuote
			have = true
		case (c == ' ' || c == '\t') && !inQuote:
			if have {
				args = append(args, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteByte(c)
			have = true
		}
		i++
	}
	if have {
		args = append(args, cur.String())
	}
	return args
}
