package gemini

import (
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ubgo/agentio"
)

// joinPath joins host path elements. cost: Pure.
func joinPath(elem ...string) string { return filepath.Join(elem...) }

// hostGOOS is the running OS, for a Host without GOOS. cost: Pure.
func hostGOOS() string { return runtime.GOOS }

// samePath compares two host paths after cleaning; case-insensitively on
// Windows, whose filesystems fold case. cost: Pure.
func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == agentio.OSWindows {
		return strings.EqualFold(a, b)
	}
	return a == b
}
