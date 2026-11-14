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
		return linuxProcessesAt(procRoot)
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

// reasonWindowsCwd is why Cwd is unsupported on windows.
const reasonWindowsCwd = "windows exposes another process's working directory only in its memory (PEB)"

// Linux /proc layout.
const (
	procRoot = "/proc"
	// procStatPPIDField and procStatStartField are 0-based indexes into
	// the fields after the ")" that ends comm in /proc/{pid}/stat
	// (proc(5): field 4 ppid, field 22 starttime).
	procStatPPIDField  = 1
	procStatStartField = 19
	// clockTicks is USER_HZ, 100 on every linux ABI Go supports.
	clockTicks = 100
	procBtime  = "btime "
)

// linuxProcessesAt reads every <root>/{pid} (root is /proc; a test passes a
// fixture tree). Rows that vanish mid-read or are unreadable are skipped:
// the table changes while it is read.
func linuxProcessesAt(root string) ([]Process, error) {
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
		if i := bytes.LastIndexByte(stat, ')'); i >= 0 {
			f := strings.Fields(string(stat[i+1:]))
			if len(f) > procStatStartField {
				p.PPID, _ = strconv.Atoi(f[procStatPPIDField])
				if ticks, err := strconv.ParseInt(f[procStatStartField], 10, 64); err == nil {
					p.Start = boot.Add(time.Duration(ticks) * time.Second / clockTicks)
				}
			}
		}
		if cmdline, err := os.ReadFile(filepath.Join(dir, "cmdline")); err == nil {
			for _, a := range bytes.Split(bytes.TrimRight(cmdline, "\x00"), []byte{0}) {
				if len(a) > 0 {
					p.Argv = append(p.Argv, string(a))
				}
			}
		}
		if exe, err := os.Readlink(filepath.Join(dir, "exe")); err == nil {
			p.Exe = exe
		} else if len(p.Argv) > 0 {
			p.Exe = p.Argv[0]
		}
		out = append(out, p)
	}
	return out, nil
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
)

// darwinProcesses runs one `ps -axww -o pid=,ppid=,lstart=,command=`.
func darwinProcesses(ctx context.Context) ([]Process, error) {
	cmd := exec.CommandContext(ctx, psBin, "-axww", "-o", "pid=,ppid=,lstart=,command=")
	cmd.Env = []string{envLocaleC}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("agentio: ps: %w", err)
	}
	return parseDarwinPS(out, time.Local), nil
}

// parseDarwinPS parses `ps -axww -o pid=,ppid=,lstart=,command=` output
// (lstart in the C locale, read in loc). Pure.
func parseDarwinPS(out []byte, loc *time.Location) []Process {
	var procs []Process
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2+psLstartFields+1 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		p := Process{PID: pid, PPID: ppid, Argv: f[2+psLstartFields:]}
		if t, err := time.ParseInLocation(psLstartLayout, strings.Join(f[2:2+psLstartFields], " "), loc); err == nil {
			p.Start = t
		}
		p.Exe = p.Argv[0]
		procs = append(procs, p)
	}
	return procs
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
	psWin32Command = "Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId,CreationDate,ExecutablePath,CommandLine | ConvertTo-Json -Compress"
	psTimeout      = 20 * time.Second
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
// with the Windows rules (splitWindowsCommandLine); Exe is ExecutablePath,
// else Argv[0]. Pure.
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
			p.Argv = splitWindowsCommandLine(*r.CommandLine)
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
