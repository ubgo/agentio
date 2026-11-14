package agentio

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// PlanHomeLinks decides which of names to link from def into h without
// changing anything. A name missing from def is left out (nothing to
// share); a target already present (Lstat, so a broken symlink counts) is
// planned with Exists true and is never overwritten. Both homes must
// belong to one agent. Disk · Read.
func PlanHomeLinks(def, h Home, names []string) ([]HomeLink, error) {
	if def.Agent() != h.Agent() {
		return nil, fmt.Errorf("%w: link %s into %s", ErrForeignHome, def, h)
	}
	out := make([]HomeLink, 0, len(names))
	for _, n := range names {
		l := HomeLink{Agent: h.Agent(), Name: n, From: def.Path(n), To: h.Path(n)}
		if _, err := os.Stat(l.From); err != nil {
			continue
		}
		if _, err := os.Lstat(l.To); !errors.Is(err, fs.ErrNotExist) {
			l.Exists = true
		}
		out = append(out, l)
	}
	return out, nil
}

// CheckLinksOwner returns ErrForeignHome when any link was planned for
// another agent's homes; every agent's ApplyHomeLinks calls it first, as
// ApplyMove checks MovePlan.Agent. Pure.
func CheckLinksOwner(agent ID, links []HomeLink) error {
	for _, l := range links {
		if l.Agent != agent {
			return fmt.Errorf("%w: %s link %s", ErrForeignHome, l.Agent, l.Name)
		}
	}
	return nil
}

// ApplyHomeLinks makes every link that does not exist yet. os.Symlink
// refuses an existing target, so a file that appeared after planning is
// never replaced; nothing is overwritten, so there is nothing to back up.
// Disk · Write; reached only through the HomeLinker door.
func ApplyHomeLinks(links []HomeLink) error {
	for _, l := range links {
		if l.Exists {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(l.To), backupDirPerm); err != nil {
			return fmt.Errorf("agentio: link %s: %w", l.Name, err)
		}
		if err := os.Symlink(l.From, l.To); err != nil {
			if isSymlinkPrivilegeError(err) {
				return &SymlinkPrivilegeError{Link: l.To, Err: err}
			}
			return fmt.Errorf("agentio: link %s: %w", l.Name, err)
		}
	}
	return nil
}

// errorPrivilegeNotHeld is Windows ERROR_PRIVILEGE_NOT_HELD (1314), what
// CreateSymbolicLink returns when the user may not create symlinks: neither
// Developer Mode is on nor the process is elevated.
const errorPrivilegeNotHeld syscall.Errno = 1314

// reasonSymlinkPrivilege is the fix a caller can show.
const reasonSymlinkPrivilege = "creating a symlink on Windows needs Developer Mode (Settings > System > For developers) or an elevated process"

// SymlinkPrivilegeError is ApplyHomeLinks refused by Windows because the
// user may not create symbolic links. It matches ErrUnsupported (the
// links cannot be made on this host as configured); links made before it
// stay, nothing is overwritten. There is no copy fallback on purpose: a
// copied settings.json would silently stop following the default home.
type SymlinkPrivilegeError struct {
	Link string
	Err  error
}

// Error names the link and the fix. Pure.
func (e *SymlinkPrivilegeError) Error() string {
	return "agentio: link " + e.Link + ": " + reasonSymlinkPrivilege
}

// Is matches ErrUnsupported. Pure.
func (e *SymlinkPrivilegeError) Is(target error) bool { return target == ErrUnsupported }

// Unwrap is the os error. Pure.
func (e *SymlinkPrivilegeError) Unwrap() error { return e.Err }

// isSymlinkPrivilegeError reports ERROR_PRIVILEGE_NOT_HELD inside err. The
// errno is compared by number, so the check compiles and is testable on
// every OS. Pure.
func isSymlinkPrivilegeError(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == errorPrivilegeNotHeld
}
