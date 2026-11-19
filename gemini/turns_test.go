package gemini

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

// Fake chats: a JSONL chat with a re-appended message, an info record, a
// $patch, a $rewindTo and a removeIds patch; a legacy single-object chat.
const (
	chatJSONL  = "session-2026-11-12T08-00-cccccccc.jsonl"
	chatLegacy = "session-2026-10-01T07-00-cccccccc.json"
)

// chatGolden is the golden file of a fixture chat. Pure.
func chatGolden(name string) string {
	return filepath.Join("testdata", "turns", name+".golden.json")
}

// TestTurnsAtGolden compares both fake chats with their golden files; the
// JSONL chat is also replayed as a growing file and must yield the same
// turns. The legacy chat's cursor counts messages.
func TestTurnsAtGolden(t *testing.T) {
	f := setup(t)
	r, ok := agentio.TurnFilesOf(f.a.Neutral())
	if !ok {
		t.Fatal("gemini lacks CapTurnFiles")
	}
	if _, ok := agentio.TurnsOf(f.a.Neutral()); ok {
		t.Error("gemini must keep CapTurns unsupported: a rewind cannot be reported through fn")
	}
	src := filepath.Join("testdata", "turns", chatJSONL)
	turns, _ := agenttest.ReadTurns(t, r, src)
	agenttest.CheckTurns(t, chatGolden(chatJSONL), turns, *update)
	agenttest.CheckTurns(t, chatGolden(chatJSONL), agenttest.GrowingTurns(t, r, src, chatJSONL), false)

	legacy := filepath.Join("testdata", "turns", chatLegacy)
	lt, off := agenttest.ReadTurns(t, r, legacy)
	agenttest.CheckTurns(t, chatGolden(chatLegacy), lt, *update)
	if off != 3 {
		t.Errorf("legacy cursor %d, want 3 messages", off)
	}
	n := 0
	if again, err := r.TurnsAt(legacy, off, func(agentio.Turn) error { n++; return nil }); err != nil || again != off || n != 0 {
		t.Errorf("legacy re-read: %d %v %d turns", again, err, n)
	}
}

// TestTurnsAtReappendKeepsID guards A3 for Gemini: the re-appended model
// message carries the same ID both times, the second with its tokens.
func TestTurnsAtReappendKeepsID(t *testing.T) {
	f := setup(t)
	var ids []string
	var tokens []bool
	if _, err := f.a.TurnsAt(filepath.Join("testdata", "turns", chatJSONL), 0, func(tn agentio.Turn) error {
		if tn.ID == "g-model-1" {
			ids = append(ids, tn.ID)
			tokens = append(tokens, tn.Tokens != nil)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The CLI appended the message first without tokens, then re-appended
	// it with its tokens and tool result: two turns, one ID.
	if len(ids) != 2 || tokens[0] || !tokens[1] {
		t.Errorf("g-model-1 emitted %d times, tokens %v", len(ids), tokens)
	}
}

// TestChatOwner maps a main chat, a subagent chat and a legacy chat to the
// home and the full session id, and refuses foreign paths.
func TestChatOwner(t *testing.T) {
	f := setup(t)
	r, _ := agentio.TurnFilesOf(f.a.Neutral())
	chats := filepath.Join(f.home.Dir(), tmpDir, "demo", chatsDir)
	for path, id := range map[string]string{
		filepath.Join(chats, "session-2026-11-01T10-00-11111111.jsonl"):                                            "11111111-1111-4111-8111-111111111111",
		filepath.Join(chats, "session-2026-10-20T12-00-44444444.json"):                                             "44444444-4444-4444-8444-444444444444",
		filepath.Join(chats, "11111111-1111-4111-8111-111111111111", "55555555-5555-4555-8555-555555555555.jsonl"): "",
	} {
		ref, err := r.TranscriptOwner(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if ref.Agent != ID || ref.Home.Dir() != f.home.Dir() || (id != "" && ref.ConversationID != id) || ref.ConversationID == "" {
			t.Errorf("%s: ref = %+v", path, ref)
		}
	}
	notMeta := filepath.Join(chats, "session-2026-01-01T00-00-00000000.jsonl")
	if err := os.WriteFile(notMeta, []byte("{\"id\":\"x\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(f.home.Dir(), "notes", "a.jsonl"), filepath.Join(chats, "a.txt"), notMeta} {
		if _, err := r.TranscriptOwner(p); !errors.Is(err, agentio.ErrNotFound) {
			t.Errorf("%s: err = %v, want ErrNotFound", p, err)
		}
	}
}
