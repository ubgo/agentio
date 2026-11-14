package agentio

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
		l := HomeLink{Name: n, From: def.Path(n), To: h.Path(n)}
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
			return fmt.Errorf("agentio: link %s: %w", l.Name, err)
		}
	}
	return nil
}
