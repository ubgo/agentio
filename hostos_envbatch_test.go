package agentio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// TestLinuxEnvsAt reads every readable environ and leaves out a pid
// without one.
func TestLinuxEnvsAt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "42", "environ"), "PATH=/bin\x00AGX_RUN_ID=r1\x00")
	writeTestFile(t, filepath.Join(root, "43", "environ"), "")
	got := linuxEnvsAt(root, []int{42, 43, 44})
	if len(got) != 1 || !slices.Equal(got[42], []string{"PATH=/bin", "AGX_RUN_ID=r1"}) {
		t.Fatalf("envs = %v", got)
	}
}

// TestDarwinEnvsFrom splits one `ps -E -o pid=,command=` run into an env
// per pid: rows in any order, a padded pid, a hidden env left out, an
// unrequested leading number and a newline inside a value kept on its
// row.
func TestDarwinEnvsFrom(t *testing.T) {
	t.Parallel()
	out := "  823 claude --resume x RUNBAY_DIR=/work PATH=/bin\n" +
		"   77 /bin/sleep 60\n" +
		"  501 claude CLAUDE_CONFIG_DIR=/Users/alice/.claude-work NOTE=line1\n" +
		"999 line2 AFTER=1\n"
	got := darwinEnvsFrom([]int{77, 501, 823, 12}, out)
	want := map[int][]string{
		823: {"RUNBAY_DIR=/work", "PATH=/bin"},
		501: {"CLAUDE_CONFIG_DIR=/Users/alice/.claude-work", "NOTE=line1", "AFTER=1"},
	}
	if len(got) != len(want) {
		t.Fatalf("envs = %v", got)
	}
	for pid, env := range want {
		if !slices.Equal(got[pid], env) {
			t.Errorf("pid %d env = %v, want %v", pid, got[pid], env)
		}
	}
	if got := darwinEnvsFrom([]int{1}, ""); len(got) != 0 {
		t.Errorf("empty output = %v", got)
	}
}

// TestOSEnvBatch reads the test process's own environment through the
// real host in one batch on linux and darwin (both show a process its
// own user's environment), and is ErrUnsupported on windows.
func TestOSEnvBatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	self := os.Getpid()
	got, err := osProcesses{}.EnvBatch(ctx, []int{self})
	switch runtime.GOOS {
	case goosLinux, goosDarwin:
		if err != nil || len(got[self]) == 0 {
			t.Fatalf("own env = %v %v", got, err)
		}
		if empty, err := (osProcesses{}).EnvBatch(ctx, nil); err != nil || len(empty) != 0 {
			t.Errorf("no pids = %v %v", empty, err)
		}
	case goosWindows:
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("windows: %v", err)
		}
	}
}
