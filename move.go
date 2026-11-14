package agentio

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// MoveRequest asks to plan moving one folder's conversations from one home
// to another home of the same agent.
type MoveRequest struct {
	From, To Home
	// Dir is the working folder whose conversations move (absolute, clean).
	Dir string
	// OnlyIDs limits the move to these conversation ids; empty = all.
	OnlyIDs []string
}

// MoveItem is one filesystem entry (file or directory) to rename.
type MoveItem struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// MovePlan is everything a move will do, computed without changing
// anything, so it can be shown, checked (Blockers) and then applied.
type MovePlan struct {
	Agent ID     `json:"agent"`
	From  Home   `json:"from"`
	To    Home   `json:"to"`
	Dir   string `json:"dir"`
	// Conversations are the ids that will move, newest first.
	Conversations []string   `json:"conversations"`
	Items         []MoveItem `json:"items"`
	// Blockers abort the move before a single file is touched (an agent
	// running in the folder, an id already in the target).
	Blockers []string `json:"blockers,omitempty"`
	// Left lists entries found for the folder that are not moved, so
	// nothing is silently dropped.
	Left []string `json:"left,omitempty"`
	// Cleanup lists source dirs removed after a successful move only if
	// empty, innermost first, so a fully moved folder leaves no empty
	// history that would still look like "has history".
	Cleanup []string `json:"cleanup,omitempty"`
}

// ApplyMovePlan executes a plan the same way for every agent: back up every
// source item into one sibling dir `<from home>.agentio-bak-<stamp>/`
// (paths relative to the source home), then rename each item into place.
// If a rename fails, the items already moved are renamed back, so a failed
// move leaves both homes as they were. Renames need both homes on one
// filesystem; a cross-device target fails and triggers the rollback.
// Disk · Write; call it only from a Writer door.
func ApplyMovePlan(plan MovePlan, now time.Time) (Backup, error) {
	if len(plan.Blockers) > 0 {
		return Backup{}, fmt.Errorf("%w: %v", ErrMoveBlocked, plan.Blockers)
	}
	if plan.From.Agent() != plan.To.Agent() {
		return Backup{}, fmt.Errorf("%w: move from %s to %s", ErrForeignHome, plan.From.Agent(), plan.To.Agent())
	}
	backup, err := backupMoveItems(plan, now)
	if err != nil {
		return Backup{}, err
	}
	var done []MoveItem
	for _, it := range plan.Items {
		err := os.MkdirAll(filepath.Dir(it.To), backupDirPerm)
		if err == nil {
			err = os.Rename(it.From, it.To)
		}
		if err != nil {
			if rbErr := rollbackMove(done); rbErr != nil {
				return backup, fmt.Errorf("agentio: move %s: %w (rollback also failed: %v; restore from the backup)", it.From, err, rbErr)
			}
			return backup, fmt.Errorf("agentio: move %s: %w (nothing was changed)", it.From, err)
		}
		done = append(done, it)
	}
	for _, dir := range plan.Cleanup {
		// os.Remove only deletes an empty dir; a non-empty or missing one is
		// exactly the case to leave alone, so the error is ignored.
		_ = os.Remove(dir)
	}
	return backup, nil
}

// backupMoveItems copies every item into one backup dir beside the source home.
func backupMoveItems(plan MovePlan, now time.Time) (Backup, error) {
	if len(plan.Items) == 0 {
		return Backup{}, nil
	}
	root, err := BackupPath(plan.From.Dir(), now)
	if err != nil {
		return Backup{}, err
	}
	var b Backup
	for _, it := range plan.Items {
		rel, err := filepath.Rel(plan.From.Dir(), it.From)
		if err != nil {
			return Backup{}, fmt.Errorf("agentio: backup path for %s: %w", it.From, err)
		}
		dst := filepath.Join(root, rel)
		if err := CopyTree(it.From, dst); err != nil {
			return Backup{}, fmt.Errorf("agentio: backup %s: %w", it.From, err)
		}
		b.Entries = append(b.Entries, BackupEntry{Original: it.From, Copy: dst})
	}
	return b, nil
}

// rollbackMove renames moved items back, newest first.
func rollbackMove(done []MoveItem) error {
	var errs []error
	for i := len(done) - 1; i >= 0; i-- {
		if err := os.Rename(done[i].To, done[i].From); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
