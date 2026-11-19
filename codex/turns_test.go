package codex

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// update rewrites the golden turn files (go test -run Turns -update).
var update = flag.Bool("update", false, "rewrite testdata/turns/*.golden.json")

// Fake rollouts: the older user_message / agent_message events with phase
// and a token_count after the answer, and the 0.16x item_completed shape
// with a commentary message whose token_count comes after a tool item.
const (
	rolloutOld = "rollout-2026-11-10T09-00-00-bbbbbbbb-0000-7000-8000-000000000001.jsonl"
	rolloutNew = "rollout-2026-11-11T10-00-00-bbbbbbbb-0000-7000-8000-000000000002.jsonl"
)

// goldenOf is the golden file of a fixture rollout. Pure.
func goldenOf(name string) string {
	return filepath.Join("testdata", "turns", strings.TrimSuffix(name, rolloutExt)+".golden.json")
}

// TestTurnsAtGolden compares the normalized turns of each fake rollout
// with its golden file, and checks that a live reader (the file growing
// half a line at a time) sees exactly the same turns, tokens included.
func TestTurnsAtGolden(t *testing.T) {
	a, _ := setup(t)
	r, ok := agentio.TurnFilesOf(a.Neutral())
	if !ok {
		t.Fatal("codex lacks CapTurnFiles")
	}
	for _, name := range []string{rolloutOld, rolloutNew} {
		t.Run(name, func(t *testing.T) {
			src := filepath.Join("testdata", "turns", name)
			turns, _ := agenttest.ReadTurns(t, r, src)
			agenttest.CheckTurns(t, goldenOf(name), turns, *update)
			agenttest.CheckTurns(t, goldenOf(name), agenttest.GrowingTurns(t, r, src, name), false)
		})
	}
}

// TestPhasePairsTokensWithAnswer guards A2: a commentary message is
// Interim with no tokens, and the token_count after a tool item is not
// paired with it; the final answer gets its own token_count.
func TestPhasePairsTokensWithAnswer(t *testing.T) {
	a, _ := setup(t)
	turns, _ := agenttest.ReadTurns(t, a.Neutral().(agentio.TurnFileReader), filepath.Join("testdata", "turns", rolloutNew))
	if len(turns) != 3 {
		t.Fatalf("got %d turns, want user, commentary, answer", len(turns))
	}
	com, ans := turns[1], turns[2]
	if !com.Interim || com.Tokens != nil || com.ID != "item-agent-1" {
		t.Errorf("commentary = %+v", com)
	}
	if ans.Interim || ans.Tokens == nil || ans.Tokens.Input != 1000 || ans.ID != "item-agent-2" {
		t.Errorf("answer = %+v", ans)
	}
}

// TestHeldAnswerAcrossReads guards A4: an answer whose token_count is not
// written yet is not emitted and the cursor stays on its line; once the
// token_count lands, the next read emits it once, with tokens.
func TestHeldAnswerAcrossReads(t *testing.T) {
	a, _ := setup(t)
	data, err := os.ReadFile(filepath.Join("testdata", "turns", rolloutNew))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(string(data), "\n")
	answer := 11 // the final_answer item_completed line (ordinal 11)
	if !strings.Contains(lines[answer], `"final_answer"`) {
		t.Fatalf("line %d is not the answer", answer)
	}
	dst := filepath.Join(t.TempDir(), rolloutNew)
	write := func(n int) {
		if err := os.WriteFile(dst, []byte(strings.Join(lines[:n], "")), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var got []agentio.Turn
	collect := func(tn agentio.Turn) error { got = append(got, tn); return nil }
	write(answer + 3) // answer, its response_item and token_usage_record; no token_count yet
	off, err := a.TurnsAt(dst, 0, collect)
	if err != nil {
		t.Fatal(err)
	}
	if want := agentio.Offset(len(strings.Join(lines[:answer], ""))); off != want {
		t.Fatalf("cursor %d, want %d (the answer's line)", off, want)
	}
	if len(got) != 2 {
		t.Fatalf("emitted %d turns before the token_count, want 2 (user, commentary)", len(got))
	}
	again, err := a.TurnsAt(dst, off, collect)
	if err != nil || again != off || len(got) != 2 {
		t.Fatalf("re-read without new lines: off %d err %v turns %d", again, err, len(got))
	}
	write(answer + 4) // the token_count arrives
	off, err = a.TurnsAt(dst, off, collect)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[2].Tokens == nil || got[2].Tokens.Output != 25 || got[2].Model != "gpt-fake-2" || got[2].ConversationID == "" || got[2].Dir != "/work/fake-new" {
		t.Fatalf("answer after its token_count = %+v", got[len(got)-1])
	}
	if want := agentio.Offset(len(strings.Join(lines[:answer+4], ""))); off != want {
		t.Errorf("cursor %d, want %d", off, want)
	}
}

// TestCrashedSessionKeepsAnswerHeld documents the edge case: a rollout
// that ends right after an answer (no token_count, no turn end) never
// emits it, and the cursor stays before it.
func TestCrashedSessionKeepsAnswerHeld(t *testing.T) {
	a, _ := setup(t)
	data, err := os.ReadFile(filepath.Join("testdata", "turns", rolloutOld))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(string(data), "\n")
	last := 12 // the phase-less answer of the second turn
	dst := filepath.Join(t.TempDir(), rolloutOld)
	if err := os.WriteFile(dst, []byte(strings.Join(lines[:last+1], "")), 0o600); err != nil {
		t.Fatal(err)
	}
	var texts []string
	off, err := a.TurnsAt(dst, 0, func(tn agentio.Turn) error { texts = append(texts, tn.Text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if texts[len(texts)-1] != "fake old prompt two" || off != agentio.Offset(len(strings.Join(lines[:last], ""))) {
		t.Errorf("texts %q, cursor %d", texts, off)
	}
}

// TestRolloutOwner maps sessions/ and archived_sessions/ paths to their
// home and id from the path, and refuses a foreign path.
func TestRolloutOwner(t *testing.T) {
	a, h := setup(t)
	r, _ := agentio.TurnFilesOf(a.Neutral())
	live := filepath.Join(h.Dir(), "sessions/2026/10/14/rollout-2026-10-14T10-00-00-"+convNew+".jsonl")
	archived := filepath.Join(h.Dir(), dirArchivedSessions, "rollout-2026-01-01T00-00-00-"+convOld+".jsonl")
	for path, id := range map[string]string{live: convNew, archived: convOld} {
		ref, err := r.TranscriptOwner(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if ref.Agent != ID || ref.Home.Dir() != h.Dir() || !ref.Home.IsDefault() || ref.ConversationID != id {
			t.Errorf("%s: ref = %+v", path, ref)
		}
	}
	// A name without a trailing uuid takes the id from the session_meta head.
	odd := filepath.Join(h.Dir(), "sessions/2026/10/14/rollout-renamed.jsonl")
	b, err := os.ReadFile(live)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(odd, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if ref, err := r.TranscriptOwner(odd); err != nil || ref.ConversationID != convNew {
		t.Errorf("head id: %+v %v", ref, err)
	}
	for _, p := range []string{filepath.Join(h.Dir(), "notes", rolloutNew), filepath.Join(h.Dir(), "sessions", "x.jsonl"), filepath.Join(t.TempDir(), "elsewhere", "sessions", rolloutNew)} {
		if _, err := r.TranscriptOwner(p); !errors.Is(err, agentio.ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", p, err)
		}
	}
}

// TestPhaseDecoded checks the wire phase on both message shapes, and that
// an unknown phase is kept as written and treated as interim.
func TestPhaseDecoded(t *testing.T) {
	ev, err := ParseRecord([]byte(`{"type":"event_msg","payload":{"type":"agent_message","message":"x","phase":"planning"}}`))
	if err != nil || ev.Event.Phase != "planning" || ev.Event.Phase.IsKnown() || !ev.Event.Phase.Interim() {
		t.Errorf("event phase = %+v %v", ev.Event, err)
	}
	it, err := ParseRecord([]byte(`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"AgentMessage","id":"i","content":[{"type":"Text","text":"y"}],"phase":"final_answer"}}}`))
	if err != nil || it.Event.Item.Phase != PhaseFinalAnswer || !it.Event.Item.Phase.IsKnown() || it.Event.Item.Phase.Interim() {
		t.Errorf("item phase = %+v %v", it.Event.Item, err)
	}
	if Phase("").Interim() || !PhaseCommentary.Interim() {
		t.Error("an absent phase is an answer, commentary is interim")
	}
}
