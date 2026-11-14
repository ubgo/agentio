package agentio

// osrules.go holds the rules that differ by operating system and that the
// library must apply for Host.GOOS, never for the OS the program was built
// for: environment names are case-insensitive on Windows, PATH entries are
// separated by ';' there and a bare command name is tried with every
// PATHEXT extension, and Windows has no executable bit. Every function takes
// the GOOS explicitly (Host methods pass Host.GOOS), so a test on Linux can
// prove the Windows rule and the other way round.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// OS names a Host can report in GOOS (runtime.GOOS spellings).
const (
	OSLinux   = "linux"
	OSDarwin  = "darwin"
	OSWindows = "windows"
)

// Env and PATH constants.
const (
	// envPathExt lists the extensions Windows tries for a bare command name.
	envPathExt = "PATHEXT"
	// defaultPathExt is cmd.exe's PATHEXT when the variable is unset; the
	// first four are the ones a vendor CLI install ever uses.
	defaultPathExt = ".COM;.EXE;.BAT;.CMD"
	// windowsListSep and unixListSep separate PATH entries.
	windowsListSep = ";"
	unixListSep    = ":"
)

// envFolds reports whether environment names compare case-insensitively on
// goos: Windows treats Path and PATH as one variable, every other OS does
// not.
func envFolds(goos string) bool { return goos == OSWindows }

// envNameEqual compares two variable names under goos's rule.
func envNameEqual(goos, a, b string) bool {
	if envFolds(goos) {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// EnvLookupFor returns the last value of key in env under goos's naming
// rule (case-insensitive on Windows, so "Path" answers a PATH lookup) and
// whether it is present. An entry "K=" is present with value "". Pure.
func EnvLookupFor(goos string, env []string, key string) (string, bool) {
	val, found := "", false
	for _, kv := range env {
		name, v, ok := strings.Cut(kv, envSep)
		if ok && envNameEqual(goos, name, key) {
			val, found = v, true
		}
	}
	return val, found
}

// NewCommandFor is NewCommand with the env built by ApplyEnvFor for goos.
// Pure.
func NewCommandFor(goos, path string, args []string, base []string, dir string, changes ...EnvChange) Command {
	c := NewCommand(path, args, nil, dir, changes...)
	c.Env = ApplyEnvFor(goos, base, changes...)
	return c
}

// PathListSeparatorFor is the PATH entry separator on goos. Pure.
func PathListSeparatorFor(goos string) string {
	if goos == OSWindows {
		return windowsListSep
	}
	return unixListSep
}

// LookPathAllFor returns every PATH hit for name on goos, in order (the
// first is what runs; the rest are shadowed copies):
//   - pathList is split on goos's separator (";" on Windows), and empty or
//     relative entries are skipped (never the current dir);
//   - on Windows a name without an extension is tried with every PATHEXT
//     extension in order (pathExt; "" means the cmd.exe default), so
//     "codex" finds codex.cmd from an npm global install and claude.exe
//     from the native installer, and any regular file counts as runnable
//     (Windows has no executable bit);
//   - elsewhere a hit is a regular file with an exec bit.
//
// An absolute name is checked as is. Disk · Read.
func LookPathAllFor(goos, pathList, pathExt, name string) []string {
	return lookPathAll(goos, pathList, pathExt, name, os.Stat)
}

// statFunc reads a file's metadata; os.Stat outside tests. A test passes a
// fake so it can ask the unix exec-bit rule questions on a filesystem that
// cannot store exec bits (Windows), without weakening the rule.
type statFunc func(string) (os.FileInfo, error)

// lookPathAll is LookPathAllFor with the stat seam.
func lookPathAll(goos, pathList, pathExt, name string, stat statFunc) []string {
	if name == "" {
		return nil
	}
	cands := lookCandidates(goos, pathExt, name)
	if filepath.IsAbs(name) {
		var out []string
		for _, c := range cands {
			if runnableOn(goos, c, stat) {
				out = append(out, c)
			}
		}
		return out
	}
	var out []string
	for _, dir := range strings.Split(pathList, PathListSeparatorFor(goos)) {
		if dir == "" || !isAbsFor(goos, dir) {
			continue
		}
		for _, c := range cands {
			p := filepath.Join(dir, c)
			if runnableOn(goos, p, stat) && !slices.Contains(out, p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// lookCandidates are the file names tried for name: name itself, plus on
// Windows name+ext for each PATHEXT extension when name has none.
func lookCandidates(goos, pathExt, name string) []string {
	if goos != OSWindows || filepath.Ext(name) != "" {
		return []string{name}
	}
	if pathExt == "" {
		pathExt = defaultPathExt
	}
	var out []string
	for _, ext := range strings.Split(pathExt, windowsListSep) {
		if ext = strings.TrimSpace(ext); ext != "" {
			out = append(out, name+strings.ToLower(ext))
		}
	}
	return out
}

// runnableOn reports whether p can be started on goos: a regular file, with
// an exec bit except on Windows.
func runnableOn(goos, p string, stat statFunc) bool {
	fi, err := stat(p)
	if err != nil || !fi.Mode().IsRegular() {
		return false
	}
	return goos == OSWindows || fi.Mode().Perm()&execBits != 0
}

// isAbsFor reports an absolute directory for goos. A Windows path such as
// C:\bin or \\server\share is absolute there even when the program runs
// elsewhere (tests); a "/..." path is accepted on every OS so fixture dirs
// work in a Windows-rule test on Linux.
func isAbsFor(goos, dir string) bool {
	if filepath.IsAbs(dir) || strings.HasPrefix(dir, "/") {
		return true
	}
	if goos != OSWindows {
		return false
	}
	return windowsAbs(dir)
}

// windowsAbs reports a drive-letter path (C:\x, C:/x) or a UNC path.
func windowsAbs(p string) bool {
	if strings.HasPrefix(p, `\\`) {
		return true
	}
	return len(p) >= 3 && isASCIILetter(p[0]) && p[1] == ':' && (p[2] == '\\' || p[2] == '/')
}

func isASCIILetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

// LookPath walks Host.Env PATH for name under Host.GOOS's rules
// (LookPathAllFor), returning the first hit or ErrNotInstalled. Never a
// shell lookup. Disk · Read.
func (h Host) LookPath(name string) (string, error) {
	if all := h.LookPathAll(name); len(all) > 0 {
		return all[0], nil
	}
	return "", fmt.Errorf("%w: %s", ErrNotInstalled, name)
}

// LookPathAll returns every hit for name on Host.Env PATH under Host.GOOS's
// rules (PATHEXT on Windows). Disk · Read.
func (h Host) LookPathAll(name string) []string {
	pathList, _ := h.Getenv(envPath)
	pathExt, _ := h.Getenv(envPathExt)
	return LookPathAllFor(h.hostOS(), pathList, pathExt, name)
}

// NewCommand builds a Command over Host.Env with changes applied under
// Host.GOOS's env naming rule. Pure.
func (h Host) NewCommand(path string, args []string, dir string, changes ...EnvChange) Command {
	return NewCommandFor(h.hostOS(), path, args, h.Env, dir, changes...)
}

// ApplyEnv applies changes to base under Host.GOOS's env naming rule. Pure.
func (h Host) ApplyEnv(base []string, changes ...EnvChange) []string {
	return ApplyEnvFor(h.hostOS(), base, changes...)
}

// hostOS is Host.GOOS, or the OS the program was built for when it is
// unset (a hand-built or fixture host): that keeps the PATH and env rules
// every earlier release applied (os.PathListSeparator), while a Host that
// names an OS gets that OS's rules wherever it runs. runtime.GOOS is a
// build constant, not machine state.
func (h Host) hostOS() string {
	if h.GOOS == "" {
		return runtime.GOOS
	}
	return h.GOOS
}

// utf8BOM is the UTF-8 byte order mark. Notepad and PowerShell on Windows
// write one at the start of a text file; encoding/json and TOML parsers
// reject it.
const utf8BOM = "\xEF\xBB\xBF"

// StripBOM removes a leading UTF-8 byte order mark. Every vendor JSON, TOML
// and JSONL reader calls it on file contents (and on a file's first line),
// so a settings or config file saved on Windows reads the same as one
// written on Linux. CRLF line ends need nothing: JSON treats \r as
// whitespace, the TOML parser and the frontmatter parser accept CRLF.
// Pure.
func StripBOM(b []byte) []byte { return bytes.TrimPrefix(b, []byte(utf8BOM)) }

// windowsVolume is the volume of a Windows path ("C:" of C:\Users\x, or
// \\server\share of a UNC path), parsed by hand so the Windows rule is the
// same on every build OS (filepath.VolumeName is "" off Windows). Pure.
func windowsVolume(p string) string {
	if len(p) >= 2 && isASCIILetter(p[0]) && p[1] == ':' {
		return p[:2]
	}
	if rest, ok := strings.CutPrefix(p, `\\`); ok {
		parts := strings.SplitN(rest, `\`, 3)
		if len(parts) >= 2 {
			return `\\` + parts[0] + `\` + parts[1]
		}
	}
	return ""
}
