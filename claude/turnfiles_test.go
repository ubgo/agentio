package claude

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// update rewrites the golden turn files (go test -run Turns -update).
var update = flag.Bool("update", false, "rewrite testdata/turns/*.golden.json")

// turnFixture is the fake transcript: string and block user content, a
// thinking-only and a tool-use-only assistant line, a tool_result user
// line and two answers.
const turnFixture = "aaaaaaaa-0000-4000-8000-000000000001"

// TestTurnsAtGolden reads the fixture transcript through the neutral
// TurnFileReader and compares the normalized turns with the golden file,
// then replays a live writer and checks it sees the same turns.
func TestTurnsAtGolden(t *testing.T) {
	a, _ := fixtureAgent(t, "2.1.325")
	r, ok := agentio.TurnFilesOf(a.Neutral())
	if !ok {
		t.Fatal("claude lacks CapTurnFiles")
	}
	src := filepath.Join("testdata", "turns", turnFixture+transcriptExt)
	turns, off := agenttest.ReadTurns(t, r, src)
	agenttest.CheckTurns(t, filepath.Join("testdata", "turns", turnFixture+".golden.json"), turns, *update)
	if fi, err := os.Stat(src); err != nil || off != agentio.Offset(fi.Size()) {
		t.Errorf("cursor %d, want the file size (%v)", off, err)
	}
	if turns[0].ID != "u0000000-0000-4000-8000-000000000001" {
		t.Errorf("first turn id %q, want the record uuid", turns[0].ID)
	}
	grown := agenttest.GrowingTurns(t, r, src, turnFixture+transcriptExt)
	agenttest.CheckTurns(t, filepath.Join("testdata", "turns", turnFixture+".golden.json"), grown, false)
}

// TestTranscriptOwner maps a transcript path to its home and id, and a
// path outside a projects dir to ErrNotFound.
func TestTranscriptOwner(t *testing.T) {
	a, dir := fixtureAgent(t, "2.1.325")
	r, _ := agentio.TurnFilesOf(a.Neutral())
	home := filepath.Join(dir, ".claude-work")
	ref, err := r.TranscriptOwner(filepath.Join(home, projectsDir, "-work-x", turnFixture+transcriptExt))
	if err != nil {
		t.Fatal(err)
	}
	if ref.Agent != ID || ref.Home.Dir() != home || ref.ConversationID != turnFixture {
		t.Errorf("ref = %+v", ref)
	}
	for _, p := range []string{filepath.Join(dir, "notes", "x.jsonl"), filepath.Join(home, projectsDir, "-work-x", "x.txt")} {
		if _, err := r.TranscriptOwner(p); !errors.Is(err, agentio.ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", p, err)
		}
	}
}
