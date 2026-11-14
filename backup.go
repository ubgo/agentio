package agentio

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Backup is what a Write call saved before changing anything (invariant
// 11). Every Write door returns one; an empty Backup means there was
// nothing to save (the target did not exist).
type Backup struct {
	Entries []BackupEntry
}

// BackupEntry is one saved original.
type BackupEntry struct {
	// Original is the path that was about to change.
	Original string
	// Copy is where its bytes were saved.
	Copy string
}

// IsEmpty reports nothing saved. Pure.
func (b Backup) IsEmpty() bool { return len(b.Entries) == 0 }

// Merge appends o's entries. Pure.
func (b Backup) Merge(o Backup) Backup {
	return Backup{Entries: append(append([]BackupEntry{}, b.Entries...), o.Entries...)}
}

// Backup naming.
const (
	// backupInfix separates the original name from the stamp.
	backupInfix = ".agentio-bak-"
	// backupStamp is the UTC stamp layout: sortable, no ':' (Windows-safe).
	backupStamp = "20060102T150405Z"
	// maxBackupSuffix bounds the "-N" search for a free backup name.
	maxBackupSuffix = 1000
	// backupDirPerm is used for directories created for backups and moves;
	// agent homes are private to the user.
	backupDirPerm os.FileMode = 0o700
)

// BackupPath returns `<path>.agentio-bak-<UTC stamp>`, adding "-2", "-3",
// … until the name is free, so a backup never overwrites another backup.
// Disk · Read.
func BackupPath(path string, now time.Time) (string, error) {
	base := path + backupInfix + now.UTC().Format(backupStamp)
	for i := 1; i <= maxBackupSuffix; i++ {
		p := base
		if i > 1 {
			p = base + "-" + strconv.Itoa(i)
		}
		if _, err := os.Lstat(p); errors.Is(err, fs.ErrNotExist) {
			return p, nil
		} else if err != nil {
			return "", fmt.Errorf("agentio: backup name %s: %w", p, err)
		}
	}
	return "", fmt.Errorf("agentio: no free backup name for %s", path)
}

// BackupFile copies path (file or directory tree) to BackupPath(path, now)
// before a write. A missing path returns an empty Backup and nil: there is
// nothing to save. Disk · Write; call it only from a Writer door.
func BackupFile(path string, now time.Time) (Backup, error) {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return Backup{}, nil
	} else if err != nil {
		return Backup{}, fmt.Errorf("agentio: backup %s: %w", path, err)
	}
	dst, err := BackupPath(path, now)
	if err != nil {
		return Backup{}, err
	}
	if err := CopyTree(path, dst); err != nil {
		return Backup{}, fmt.Errorf("agentio: backup %s: %w", path, err)
	}
	return Backup{Entries: []BackupEntry{{Original: path, Copy: dst}}}, nil
}

// CopyTree copies a file or directory tree from src to dst, preserving
// file modes; symlinks are copied as links, not followed. dst must not
// exist. Disk · Write.
func CopyTree(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("agentio: copy: %s already exists", dst)
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|backupDirPerm)
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			if err := os.MkdirAll(filepath.Dir(target), backupDirPerm); err != nil {
				return err
			}
			return copyFile(path, target, info.Mode().Perm())
		}
	})
}

// copyFile copies one regular file; O_EXCL so nothing is overwritten.
func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	// Read-only handle: a close error cannot lose data.
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close() // the copy error is the one worth reporting
		return err
	}
	return out.Close() // a failed close can mean a failed write: report it
}
