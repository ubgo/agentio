package gemini

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestTurnsAtDir stamps every turn of a main chat and of a subagent chat
// with the project dir from tmp/<slug>/.project_root, and leaves Dir ""
// without the marker. Raw is the line without "\r\n".
func TestTurnsAtDir(t *testing.T) {
	f := setup(t)
	src, err := os.ReadFile(filepath.Join("testdata", "turns", chatJSONL))
	if err != nil {
		t.Fatal(err)
	}
	crlf := bytes.ReplaceAll(src, []byte("\n"), []byte("\r\n"))
	slug := filepath.Join(t.TempDir(), "tmp", "alice-proj")
	agenttest.WriteFile(t, slug, projectRootFile, "/work/alice-proj\n")
	agenttest.WriteFile(t, filepath.Join(slug, chatsDir), chatJSONL, string(crlf))
	agenttest.WriteFile(t, filepath.Join(slug, chatsDir, "cccccccc-parent"), "agent-1.jsonl", string(src))
	bare := filepath.Join(t.TempDir(), chatJSONL)
	agenttest.WriteFile(t, filepath.Dir(bare), chatJSONL, string(src))
	for _, tc := range []struct {
		path, dir string
	}{
		{filepath.Join(slug, chatsDir, chatJSONL), "/work/alice-proj"},
		{filepath.Join(slug, chatsDir, "cccccccc-parent", "agent-1.jsonl"), "/work/alice-proj"},
		{bare, ""},
	} {
		n := 0
		if _, err := f.a.TurnsAt(tc.path, 0, func(tr agentio.Turn) error {
			n++
			if tr.Dir != tc.dir || bytes.HasSuffix(tr.Raw, []byte("\r")) || bytes.HasSuffix(tr.Raw, []byte("\n")) || tr.AgentVersion != "" {
				t.Errorf("%s: turn %s dir %q raw end %q", tc.path, tr.ID, tr.Dir, tr.Raw[max(0, len(tr.Raw)-2):])
			}
			return nil
		}); err != nil || n == 0 {
			t.Errorf("%s: %d turns, %v", tc.path, n, err)
		}
	}
}
