package pi

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestTurnsAtRawTrimmed: Turn.Raw is the session line without "\n" or
// "\r\n", whichever the file uses.
func TestTurnsAtRawTrimmed(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", "turns", "2026-11-13T10-00-00-000Z_dddddddd-0000-7000-8000-000000000001.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(agenttest.NewHost(t))
	for name, body := range map[string][]byte{"lf.jsonl": src, "crlf.jsonl": bytes.ReplaceAll(src, []byte("\n"), []byte("\r\n"))} {
		dir := t.TempDir()
		agenttest.WriteFile(t, dir, name, string(body))
		n := 0
		if _, err := a.TurnsAt(filepath.Join(dir, name), 0, func(tr agentio.Turn) error {
			n++
			if len(tr.Raw) == 0 || tr.Raw[len(tr.Raw)-1] != '}' {
				t.Errorf("%s: raw of %s ends %q", name, tr.ID, tr.Raw[max(0, len(tr.Raw)-2):])
			}
			return nil
		}); err != nil || n == 0 {
			t.Errorf("%s: %d turns, %v", name, n, err)
		}
	}
}
