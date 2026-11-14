package agentcli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testNow is the fixed clock of the move tests.
func testNow() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

func writeT(t *testing.T, p, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readT(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestBackupFile_NeverOverwrites proves invariant 11's naming: the stamp
// is UTC and a second backup in the same second gets "-2".
func TestBackupFile_NeverOverwrites(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "settings.json")
	writeT(t, p, "v1")
	b1, err := BackupFile(p, testNow().In(time.FixedZone("x", 3600)))
	if err != nil {
		t.Fatal(err)
	}
	writeT(t, p, "v2")
	b2, err := BackupFile(p, testNow())
	if err != nil {
		t.Fatal(err)
	}
	want1 := p + ".agentcli-bak-20261113T120000Z"
	if b1.Entries[0].Copy != want1 || b2.Entries[0].Copy != want1+"-2" {
		t.Errorf("copies = %s, %s", b1.Entries[0].Copy, b2.Entries[0].Copy)
	}
	if readT(t, want1) != "v1" || readT(t, want1+"-2") != "v2" {
		t.Error("backup contents")
	}
	empty, err := BackupFile(filepath.Join(t.TempDir(), "missing"), testNow())
	if err != nil || !empty.IsEmpty() {
		t.Errorf("missing file: %+v %v", empty, err)
	}
}

func movePlan(root string) (MovePlan, string, string) {
	from := NewHome("toy", filepath.Join(root, "from"), true, "default")
	to := NewHome("toy", filepath.Join(root, "to"), false, "to")
	return MovePlan{Agent: "toy", From: from, To: to}, from.Dir(), to.Dir()
}

func TestApplyMovePlan_MovesAndBacksUp(t *testing.T) {
	t.Parallel()
	plan, from, to := movePlan(t.TempDir())
	src := filepath.Join(from, "p", "a.jsonl")
	writeT(t, src, "A")
	plan.Items = []MoveItem{{From: src, To: filepath.Join(to, "p", "a.jsonl")}}
	plan.Cleanup = []string{filepath.Join(from, "p")}
	b, err := ApplyMovePlan(plan, testNow())
	if err != nil {
		t.Fatal(err)
	}
	if readT(t, filepath.Join(to, "p", "a.jsonl")) != "A" {
		t.Error("not moved")
	}
	if len(b.Entries) != 1 || readT(t, b.Entries[0].Copy) != "A" || !strings.HasPrefix(b.Entries[0].Copy, from+backupInfix) {
		t.Errorf("backup = %+v", b)
	}
	if _, err := os.Stat(filepath.Join(from, "p")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("empty source dir not cleaned up")
	}
}

func TestApplyMovePlan_BlockersTouchNothing(t *testing.T) {
	t.Parallel()
	plan, from, to := movePlan(t.TempDir())
	src := filepath.Join(from, "a.jsonl")
	writeT(t, src, "A")
	plan.Items = []MoveItem{{From: src, To: filepath.Join(to, "a.jsonl")}}
	plan.Blockers = []string{"agent running", "id exists"}
	_, err := ApplyMovePlan(plan, testNow())
	if !errors.Is(err, ErrMoveBlocked) || !strings.Contains(err.Error(), "id exists") {
		t.Fatalf("err = %v", err)
	}
	if readT(t, src) != "A" {
		t.Error("source changed")
	}
	if m, _ := filepath.Glob(from + backupInfix + "*"); len(m) != 0 {
		t.Error("a blocked move must not write a backup")
	}
}

func TestApplyMovePlan_RollbackOnLaterFailure(t *testing.T) {
	t.Parallel()
	plan, from, to := movePlan(t.TempDir())
	first := filepath.Join(from, "p", "a.jsonl")
	writeT(t, first, "A")
	second := filepath.Join(from, "p", "b.jsonl")
	writeT(t, second, "B")
	// The second target's parent is a file, so its rename fails after the
	// first item already moved.
	writeT(t, filepath.Join(to, "blocked"), "")
	plan.Items = []MoveItem{
		{From: first, To: filepath.Join(to, "p", "a.jsonl")},
		{From: second, To: filepath.Join(to, "blocked", "b.jsonl")},
	}
	plan.Cleanup = []string{filepath.Join(from, "p")}
	_, err := ApplyMovePlan(plan, testNow())
	if err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("err = %v, want a rolled-back failure", err)
	}
	if readT(t, first) != "A" || readT(t, second) != "B" {
		t.Error("sources not restored")
	}
	if _, err := os.Stat(filepath.Join(to, "p", "a.jsonl")); !errors.Is(err, fs.ErrNotExist) {
		t.Error("first item left in the target")
	}
}

func TestApplyMovePlan_ForeignHomes(t *testing.T) {
	t.Parallel()
	plan, _, _ := movePlan(t.TempDir())
	plan.To = NewHome("other", "/x", false, "")
	if _, err := ApplyMovePlan(plan, testNow()); !errors.Is(err, ErrForeignHome) {
		t.Errorf("err = %v", err)
	}
}
