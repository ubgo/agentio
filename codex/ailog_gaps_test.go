package codex

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// Rollout lines for alice's fixture session: a header, a turn context
// with a personality, a prompt, and an answer released by its tokens.
const (
	gapMeta     = `{"timestamp":"2026-11-19T09:00:00.000Z","type":"session_meta","payload":{"id":"bbbbbbbb-0000-7000-8000-0000000000a1","cwd":"/work/alice","cli_version":"0.162.1"}}`
	gapContext  = `{"timestamp":"2026-11-19T09:00:01.000Z","type":"turn_context","payload":{"turn_id":"t1","root_turn_id":"t1","cwd":"/work/alice","model":"gpt-fake","personality":"pragmatic","approvals_reviewer":"user"}}`
	gapPrompt   = `{"timestamp":"2026-11-19T09:00:02.000Z","type":"event_msg","payload":{"type":"user_message","message":"fake prompt"}}`
	gapAnswer   = `{"timestamp":"2026-11-19T09:00:03.000Z","type":"event_msg","payload":{"type":"agent_message","message":"fake answer"}}`
	gapTokens   = `{"timestamp":"2026-11-19T09:00:04.000Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}}}`
	gapComplete = `{"timestamp":"2026-11-19T09:00:05.000Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"t1"}}`
)

// TestTurnsAtSkipsBadLine: a torn line in the middle of a rollout no
// longer ends the read; it is reported with its offset and the answer
// after it is read. Raw is the line without "\r\n".
func TestTurnsAtSkipsBadLine(t *testing.T) {
	dir := t.TempDir()
	bad := `{"timestamp":"2026-11-19T09:00:02.5`
	lines := []string{gapMeta, gapContext, gapPrompt, bad, gapAnswer, gapTokens, gapComplete}
	body := strings.Join(lines, "\r\n") + "\r\n"
	agenttest.WriteFile(t, dir, "rollout.jsonl", body)
	path := filepath.Join(dir, "rollout.jsonl")
	a := New(agenttest.NewHost(t))
	var turns []agentio.Turn
	var bads []agentio.BadLine
	off, err := a.TurnsAtReporting(path, 0, func(tr agentio.Turn) error { turns = append(turns, tr); return nil }, func(b agentio.BadLine) { bads = append(bads, b) })
	if err != nil || int(off) != len(body) {
		t.Fatalf("read: offset %d of %d, %v", off, len(body), err)
	}
	if len(turns) != 2 || turns[0].Text != "fake prompt" || turns[1].Text != "fake answer" || turns[1].Tokens == nil {
		t.Fatalf("turns = %+v", turns)
	}
	if !bytes.Equal(turns[0].Raw, []byte(gapPrompt)) {
		t.Errorf("raw keeps a terminator: %q", turns[0].Raw)
	}
	wantAt := agentio.Offset(len(strings.Join(lines[:3], "\r\n")) + len("\r\n"))
	if len(bads) != 1 || bads[0].Offset != wantAt || bads[0].Path != path {
		t.Fatalf("bad lines = %+v, want one at %d", bads, wantAt)
	}
	// Plain TurnsAt reads the same turns without an error.
	count := 0
	if _, err := a.TurnsAt(path, 0, func(agentio.Turn) error { count++; return nil }); err != nil || count != 2 {
		t.Errorf("plain TurnsAt: %d turns, %v", count, err)
	}
}

// TestTurnContextPersonality keeps personality and the keys TurnContext
// does not model.
func TestTurnContextPersonality(t *testing.T) {
	rec, err := ParseRecord([]byte(gapContext))
	if err != nil || rec.TurnContext == nil {
		t.Fatalf("parse: %+v %v", rec, err)
	}
	tc := rec.TurnContext
	if tc.Personality != PersonalityPragmatic || !tc.Personality.IsKnown() {
		t.Errorf("personality = %q", tc.Personality)
	}
	if tc.Unknown["root_turn_id"] == nil || tc.Unknown["approvals_reviewer"] == nil || tc.Unknown["personality"] != nil {
		t.Errorf("unknown = %v", tc.Unknown)
	}
}
