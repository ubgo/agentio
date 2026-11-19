package claude

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// claudeUserLine and claudeAnswerLine are a minimal user prompt and
// assistant answer (alice's fixture session).
const (
	claudeUserLine   = `{"type":"user","uuid":"u1","sessionId":"s1","cwd":"/work/alice","message":{"role":"user","content":"hi"}}`
	claudeAnswerLine = `{"type":"assistant","uuid":"a1","sessionId":"s1","cwd":"/work/alice","message":{"role":"assistant","model":"m","content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":1,"output_tokens":1}}}`
)

// TestTurnsFileRawAndBadLines: Turn.Raw is the line without "\n" or
// "\r\n"; a bad line in the middle is skipped, reported with its offset,
// and the turns after it are read.
func TestTurnsFileRawAndBadLines(t *testing.T) {
	dir := t.TempDir()
	bad := "{not json\n"
	body := claudeUserLine + "\r\n" + bad + claudeAnswerLine + "\n"
	agenttest.WriteFile(t, dir, "s1.jsonl", body)
	path := filepath.Join(dir, "s1.jsonl")
	var turns []agentio.Turn
	var bads []agentio.BadLine
	off, err := TurnsFileReporting(path, 0, func(tr agentio.Turn) error { turns = append(turns, tr); return nil }, func(b agentio.BadLine) { bads = append(bads, b) })
	if err != nil || int(off) != len(body) {
		t.Fatalf("read: offset %d, %v", off, err)
	}
	if len(turns) != 2 || !bytes.Equal(turns[0].Raw, []byte(claudeUserLine)) || !bytes.Equal(turns[1].Raw, []byte(claudeAnswerLine)) {
		t.Fatalf("turns = %d: %+v", len(turns), turns)
	}
	wantAt := agentio.Offset(len(claudeUserLine) + len("\r\n"))
	if len(bads) != 1 || bads[0].Offset != wantAt || bads[0].Path != path || bads[0].Err == nil {
		t.Fatalf("bad lines = %+v, want one at %d", bads, wantAt)
	}
	// The neutral forms agree; plain TurnsAt skips silently.
	n := New(agenttest.NewHost(t)).Neutral()
	r, _ := agentio.TurnFilesOf(n)
	bads = nil
	if _, err := agentio.TurnsAtReporting(r, path, 0, func(agentio.Turn) error { return nil }, func(b agentio.BadLine) { bads = append(bads, b) }); err != nil || len(bads) != 1 {
		t.Errorf("neutral reporting: %v %v", bads, err)
	}
	count := 0
	if _, err := r.TurnsAt(path, 0, func(agentio.Turn) error { count++; return nil }); err != nil || count != 2 {
		t.Errorf("plain TurnsAt: %d turns, %v", count, err)
	}
}

// TestSubagentRunsWithoutMeta lists an older subagent transcript that has
// no meta file beside a run with one, and ignores workflow journals.
func TestSubagentRunsWithoutMeta(t *testing.T) {
	a, h, _ := newReadersAgent(t, filepath.Join("testdata", "homes", "2.1.327", "userhome"))
	sub := filepath.Join(a.HistoryDir(h, "/work/alice"), "sess-1", subagentsDir)
	agenttest.WriteFile(t, sub, "agent-a1.meta.json", `{"agentType":"Explore"}`)
	agenttest.WriteFile(t, sub, "agent-a1.jsonl", "{}\n")
	agenttest.WriteFile(t, sub, "agent-b2.jsonl", "{}\n")
	agenttest.WriteFile(t, sub, "workflows/wf_1/journal.jsonl", "{}\n")
	agenttest.WriteFile(t, sub, "notes.jsonl", "{}\n")
	rs, err := a.SubagentRuns(h, "/work/alice", "sess-1")
	if err != nil || len(rs) != 2 {
		t.Fatalf("runs = %+v, %v", rs, err)
	}
	if rs[0].ID != "agent-a1" || !rs[0].HasMeta || rs[0].AgentType != "Explore" {
		t.Errorf("meta run = %+v", rs[0])
	}
	if rs[1].ID != "agent-b2" || rs[1].HasMeta || rs[1].Transcript != filepath.Join(sub, "agent-b2.jsonl") || rs[1].AgentType != "" {
		t.Errorf("meta-less run = %+v", rs[1])
	}
}

// TestHookPlanExistingKinds maps each existing hook's event to its
// neutral kind (HookUnknown for PreToolUse).
func TestHookPlanExistingKinds(t *testing.T) {
	a, _ := fixtureAgent(t, fixtureVersions(t)[0])
	inst, _ := agentio.HooksOf(a.Neutral())
	h := a.DefaultHome()
	agenttest.WriteFile(t, h.Dir(), settingsFile, hookSeed)
	p, err := inst.PlanHooks(h.Neutral(), nil)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]agentio.HookKind{}
	for _, e := range p.Existing {
		kinds[e.Event] = e.Kind
	}
	if len(p.Existing) != 5 || kinds["Stop"] != agentio.HookTurnEnd || kinds["UserPromptSubmit"] != agentio.HookPromptSubmit || kinds["PreToolUse"] != agentio.HookUnknown {
		t.Fatalf("existing = %+v", p.Existing)
	}
	if !p.IsNoop() {
		t.Error("a listing plan adds nothing")
	}
}
