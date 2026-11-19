package gemini

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// hookSeed is a realistic user settings.json with comments (Gemini CLI
// strips them): other sections, three foreign hooks on the events a
// capture tool installs into (one group with two handlers, a named one
// with a timeout and an unknown key, one with a lifecycle matcher), and a
// tool hook.
const hookSeed = `// User settings; comments are allowed here.
{
  "security": {
    "auth": { "selectedType": "oauth-personal" }
  },
  "model": { "name": "gemini-2.5-pro" }, // pinned
  "hooks": {
    "BeforeAgent": [
      { "hooks": [ { "type": "command", "command": "/usr/local/bin/other-tool notify" } ] }
    ],
    "AfterAgent": [
      {
        "hooks": [
          { "type": "command", "command": "/usr/local/bin/other-tool stop", "name": "other-stop", "timeout": 5000 },
          { "type": "command", "command": "/opt/notifier/bin/ding", "futureKey": true }
        ]
      }
    ],
    "SessionStart": [
      { "matcher": "startup", "hooks": [ { "type": "command", "command": "/usr/local/bin/other-tool start" } ] }
    ],
    /* tool guard */
    "BeforeTool": [
      { "matcher": "write_file|replace", "hooks": [ { "type": "command", "command": "/usr/local/bin/guard" } ] }
    ]
  },
  "ui": { "theme": "GitHub" }
}
`

// TestHookInstallerMerge runs the shared merge guarantees on a commented
// settings.json.
func TestHookInstallerMerge(t *testing.T) {
	f := setup(t, agenttest.WithNow(agenttest.FixedNow))
	agenttest.HookMerge(t, f.a.Neutral(), f.home.Neutral(), hookSeed, true)
}

// TestHookInstallerPlanGemini checks the event names and the Manual notes.
func TestHookInstallerPlanGemini(t *testing.T) {
	f := setup(t, agenttest.WithNow(agenttest.FixedNow))
	inst, ok := agentio.HooksOf(f.a.Neutral())
	if !ok {
		t.Fatal("no HookInstaller")
	}
	want := []agentio.HookEntry{
		{Kind: agentio.HookPromptSubmit, Command: "c"}, {Kind: agentio.HookTurnEnd, Command: "c"},
		{Kind: agentio.HookSessionStart, Command: "c"}, {Kind: agentio.HookSessionEnd, Command: "c"},
	}
	p, err := inst.PlanHooks(f.home.Neutral(), want)
	if err != nil {
		t.Fatal(err)
	}
	var evs []string
	for _, e := range p.Add {
		evs = append(evs, e.Event)
	}
	if strings.Join(evs, ",") != "BeforeAgent,AfterAgent,SessionStart,SessionEnd" || p.File != f.home.UserSettingsPath() {
		t.Fatalf("plan %+v", p)
	}
	if len(p.Manual) != 1 || p.Manual[0] != manualFolderTrust {
		t.Fatalf("fixture manual %v", p.Manual)
	}
	agenttest.WriteFile(t, "", p.File, `{"security": {"folderTrust": {"enabled": false}}, "hooksConfig": {"enabled": false, "disabled": ["c"]}}`)
	p, err = inst.PlanHooks(f.home.Neutral(), want)
	if err != nil || strings.Join(p.Manual, "|") != manualHooksOff+"|"+manualHookOff+"c" {
		t.Fatalf("manual %q %v", p.Manual, err)
	}
	if err := os.Remove(p.File); err != nil {
		t.Fatal(err)
	}
	if p, err = inst.PlanHooks(f.home.Neutral(), want); err != nil || len(p.Add) != 4 || len(p.Manual) != 1 {
		t.Fatalf("missing file: %+v %v", p, err)
	}
	if b, err := inst.ApplyHooks(p); err != nil || !b.IsEmpty() {
		t.Fatalf("apply on a missing file: %+v %v", b, err)
	}
	sf, err := ReadSettingsFile(agentio.ConfigUser, p.File)
	if err != nil || !sf.Exists || sf.Settings.Unknown["hooks"] == nil {
		t.Fatalf("created file does not read: %+v %v", sf, err)
	}
}

// TestEnsureHookGemini: idempotent add through the vendor Writer.
func TestEnsureHookGemini(t *testing.T) {
	f := setup(t, agenttest.WithNow(agenttest.FixedNow))
	w := f.a.Writer()
	added, b, err := w.EnsureHook(f.home, HookAfterAgent, "/usr/local/bin/capture")
	if err != nil || !added || b.IsEmpty() {
		t.Fatalf("first: %t %+v %v", added, b, err)
	}
	if added, b, err := w.EnsureHook(f.home, HookAfterAgent, "/usr/local/bin/capture"); err != nil || added || !b.IsEmpty() {
		t.Fatalf("second: %t %+v %v", added, b, err)
	}
	if _, _, err := w.EnsureHook(f.home, HookAfterAgent, ""); err == nil {
		t.Fatal("empty command accepted")
	}
	if n, _, err := w.RemoveHooks(f.home, func(c string) bool { return c == "/usr/local/bin/capture" }); err != nil || n != 1 {
		t.Fatalf("remove %d %v", n, err)
	}
	if !HookBeforeAgent.IsKnown() || HookEvent("Later").IsKnown() || len(HookEvents()) != 11 {
		t.Fatal("event list")
	}
}

// Unverified payloads: no Gemini CLI hook run is recorded yet. These follow
// docs/hooks/reference.md and createBaseInput of the 0.68.0 package,
// values replaced.
const (
	unverifiedBeforeAgent = `{"session_id":"7c0e0000-0000-4000-8000-000000000001","transcript_path":"/home/test/.gemini/tmp/demo/chats/session-x.json","cwd":"/work/app","hook_event_name":"BeforeAgent","timestamp":"2026-11-19T10:00:00.000Z","prompt":"add a test"}`
	unverifiedAfterAgent  = `{"session_id":"7c0e0000-0000-4000-8000-000000000001","transcript_path":"","cwd":"/work/app","hook_event_name":"AfterAgent","timestamp":"not a time","prompt":"add a test","prompt_response":"Added the test.","stop_hook_active":false,"later_key":1}`
)

// TestDecodeHookUnverified decodes the documented payload shapes.
func TestDecodeHookUnverified(t *testing.T) {
	inst, _ := agentio.HooksOf(New(agentio.Host{}).Neutral())
	c, err := inst.DecodeHook(strings.NewReader(unverifiedBeforeAgent))
	if err != nil {
		t.Fatal(err)
	}
	if c.Agent != ID || c.Kind != agentio.HookPromptSubmit || c.Event != "BeforeAgent" || c.Prompt != "add a test" || c.Dir != "/work/app" ||
		c.ConversationID != "7c0e0000-0000-4000-8000-000000000001" || !strings.HasSuffix(c.TranscriptPath, "session-x.json") || string(c.Raw) != unverifiedBeforeAgent {
		t.Fatalf("%+v", c)
	}
	in, err := DecodeHookInput(strings.NewReader(unverifiedBeforeAgent))
	if err != nil || !in.Timestamp.Equal(time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("timestamp %v %v", in.Timestamp, err)
	}
	in, err = DecodeHookInput(strings.NewReader(unverifiedAfterAgent))
	if err != nil || !in.Timestamp.IsZero() || in.Unknown["later_key"] == nil || in.Unknown["timestamp"] != nil {
		t.Fatalf("%+v %v", in, err)
	}
	if c := in.Call(); c.Kind != agentio.HookTurnEnd || c.LastAssistantMessage != "Added the test." {
		t.Fatalf("%+v", c)
	}
	if c, _ := inst.DecodeHook(strings.NewReader(`{"hook_event_name":"BeforeTool"}`)); c.Kind != agentio.HookUnknown {
		t.Fatalf("%+v", c)
	}
	if _, err := inst.DecodeHook(strings.NewReader(`{`)); err == nil {
		t.Fatal("bad payload accepted")
	}
}
