package agentio

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// shim.go reads package-manager launcher shims. npm on Windows, and pnpm on
// every OS, do not symlink a global CLI into PATH: they write a small
// script (cmd-shim) that runs the package's entry file. The script names
// that file relative to its own directory, so reading it is the only way
// to find the package (and its package.json version) from the PATH hit.
//
// Shapes (cmd-shim, which npm and pnpm both use):
//   - .cmd:  "%dp0%\node_modules\@openai\codex\bin\codex.js" %*
//     (older shims write %~dp0\ instead of %dp0%\)
//   - .ps1:  "$basedir/node_modules/@openai/codex/bin/codex.js" $args
//   - sh:    exec node "$basedir/global/5/node_modules/@openai/codex/bin/codex.js" "$@"

// shimMaxBytes bounds the read: a shim is a few hundred bytes, a binary
// launcher is megabytes and is never a shim.
const shimMaxBytes = 16 << 10

// shimDirTokens are the shim spellings of "the directory of this script".
func shimDirTokens() []string { return []string{`%dp0%\`, `%~dp0\`, `$basedir/`, `$basedir\`} }

// ShimTarget reads the launcher script at p and returns the file it runs,
// resolved against p's directory with OS separators, and whether p is such
// a shim. A binary (a NUL byte in the first bytes), an unreadable file or a
// script naming no shim path gives ok false. Disk · Read (one bounded read).
func ShimTarget(p string) (string, bool) {
	f, err := os.Open(p)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, shimMaxBytes))
	if err != nil || bytes.IndexByte(b, 0) >= 0 {
		return "", false
	}
	rel, ok := shimRel(string(b))
	if !ok {
		return "", false
	}
	return filepath.Join(filepath.Dir(p), filepath.FromSlash(rel)), true
}

// shimRel returns the shim-relative path the script runs, with forward
// slashes: of every path after a shim dir token (up to the closing quote),
// the first that goes into node_modules. The npm .cmd shim also names
// "%dp0%\node.exe" (a node beside the shim) before the package entry, so
// the first token alone is not the answer. Pure.
func shimRel(script string) (string, bool) {
	for _, tok := range shimDirTokens() {
		rest := script
		for {
			i := strings.Index(rest, tok)
			if i < 0 {
				break
			}
			rest = rest[i+len(tok):]
			end := strings.IndexAny(rest, "\"' \r\n")
			if end < 0 {
				end = len(rest)
			}
			rel := strings.ReplaceAll(rest[:end], `\`, "/")
			if strings.Contains(rel, shimPackageDir) && !strings.ContainsAny(rel, "%$") {
				return rel, true
			}
		}
	}
	return "", false
}

// shimPackageDir marks the package path a shim runs.
const shimPackageDir = "node_modules/"
