// Package exampletest holds what the example tests share so their checks
// mean the same on macOS, Linux and Windows. The examples print paths
// relative to a temp home as "~/..."; their README output and test lines
// are written with forward slashes.
package exampletest

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// goosWindows is runtime.GOOS on Windows.
const goosWindows = "windows"

// homePathPattern matches a printed path relative to the temp home
// ("~\x\y" on Windows). Only these tokens are rewritten, so escapes such
// as \n or \" in quoted output are left alone.
const homePathPattern = `~\\[^\s"'()\[\],]*`

// Output returns example output with the home-relative paths written with
// forward slashes on Windows (filepath joins with '\' there), so a test
// line such as "~/.claude/settings.json" matches on every OS. It is the
// identity elsewhere. Pure.
func Output(s string) string {
	if runtime.GOOS != goosWindows {
		return s
	}
	return normalizeHomePaths(s)
}

// normalizeHomePaths rewrites '\' to '/' inside home-relative path tokens
// only. Pure.
func normalizeHomePaths(s string) string {
	return regexp.MustCompile(homePathPattern).ReplaceAllStringFunc(s, func(p string) string { return strings.ReplaceAll(p, `\`, "/") })
}

// RequirePOSIXLayout skips the calling test on Windows. It is for an
// example that builds a POSIX install on disk (a binary named without an
// extension, an exec bit, a symlink into a versions dir or node_modules)
// and prints that layout: on Windows the same binary is claude.exe or
// codex.cmd, which the library's own Windows fixture tests
// (claude and codex TestWindowsFixtureHome, TestInstallLayouts) cover.
func RequirePOSIXLayout(t testing.TB) {
	t.Helper()
	if runtime.GOOS == goosWindows {
		t.Skip("this example builds and prints a POSIX install layout; the Windows layout is tested in claude and codex TestWindowsFixtureHome")
	}
}

// RequireSymlinks skips the calling test where the OS refuses to create a
// symbolic link (Windows without Developer Mode or elevation): the example
// links shared home files and would stop with *agentio.SymlinkPrivilegeError.
func RequireSymlinks(t testing.TB) {
	t.Helper()
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "target"), filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks are not available here (%v); the example links shared home files", err)
	}
}
