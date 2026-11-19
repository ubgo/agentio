package pi

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

// piBranches is a fake session with two branches from one answer (a
// branch_summary starts the second), a tool-call-only assistant message
// and a toolResult message.
const piBranches = "2026-11-13T10-00-00-000Z_dddddddd-0000-7000-8000-000000000001.jsonl"

// TestTurnsAtGolden compares the fake session's turns (every branch, in
// file order, ids from the entries) with the golden file, also as a
// growing file.
func TestTurnsAtGolden(t *testing.T) {
	a, _ := newAgent(t)
	r, ok := agentio.TurnFilesOf(a.Neutral())
	if !ok {
		t.Fatal("pi lacks CapTurnFiles")
	}
	src := filepath.Join("testdata", "turns", piBranches)
	turns, _ := agenttest.ReadTurns(t, r, src)
	golden := filepath.Join("testdata", "turns", piBranches+".golden.json")
	agenttest.CheckTurns(t, golden, turns, *update)
	agenttest.CheckTurns(t, golden, agenttest.GrowingTurns(t, r, src, piBranches), false)
	if _, err := r.TurnsAt(filepath.Join("testdata", "turns", "missing.jsonl"), 0, func(agentio.Turn) error { return nil }); err == nil {
		t.Error("a missing file must fail")
	}
}

// TestSessionOwner maps a default-tree path and a flat custom-dir path to
// their home and header id, and refuses a foreign file.
func TestSessionOwner(t *testing.T) {
	flat := t.TempDir()
	a, host := newAgent(t, SessionDirEnv+"="+flat)
	r, _ := agentio.TurnFilesOf(a.Neutral())
	h := a.DefaultHome()
	tree := filepath.Join(h.Dir(), sessionsDir, "--home-fixture-proj--", "2026-11-14T11-41-31-118Z_"+fixtureSessionID+sessionExt)
	ref, err := r.TranscriptOwner(tree)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Agent != ID || ref.Home.Dir() != h.Dir() || ref.ConversationID != fixtureSessionID {
		t.Errorf("tree ref = %+v", ref)
	}
	b, err := os.ReadFile(filepath.Join("testdata", "turns", piBranches))
	if err != nil {
		t.Fatal(err)
	}
	inFlat := filepath.Join(flat, piBranches)
	if err := os.WriteFile(inFlat, b, 0o600); err != nil {
		t.Fatal(err)
	}
	ref, err = r.TranscriptOwner(inFlat)
	if err != nil || ref.Home.Dir() != h.Dir() || ref.ConversationID != "dddddddd-0000-7000-8000-000000000001" {
		t.Errorf("flat ref = %+v %v", ref, err)
	}
	elsewhere := filepath.Join(host.UserHome, "notes", piBranches)
	if err := os.MkdirAll(filepath.Dir(elsewhere), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(elsewhere, b, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{elsewhere, filepath.Join(flat, "x.txt"), filepath.Join(flat, "missing.jsonl")} {
		if _, err := r.TranscriptOwner(p); !errors.Is(err, agentio.ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", p, err)
		}
	}
}
