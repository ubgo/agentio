package codex

import (
	"os"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// hooksSeed is a realistic hooks.json (hook_config.rs HooksFile shape):
// a description, three foreign hooks on the events a capture tool
// installs into (one group with two handlers and an unknown key, one with
// a Windows command and a matcher), and another event.
const hooksSeed = `{
  "description": "team hooks",
  "hooks": {
    "UserPromptSubmit": [
      {"hooks": [{"type": "command", "command": "/usr/local/bin/other-tool notify", "timeout": 5}]}
    ],
    "Stop": [
      {
        "hooks": [
          {"type": "command", "command": "/usr/local/bin/other-tool stop", "statusMessage": "saving"},
          {"type": "command", "command": "/opt/notifier/bin/ding", "futureKey": true}
        ]
      }
    ],
    "SessionStart": [
      {"matcher": "startup", "hooks": [{"type": "command", "command": "/usr/local/bin/other-tool start", "commandWindows": "C:\\tools\\start.cmd"}]}
    ],
    "PreToolUse": [
      {"matcher": "shell", "hooks": [{"type": "command", "command": "/usr/local/bin/guard"}]}
    ]
  }
}
`

// TestHookInstallerMerge runs the shared merge guarantees on hooks.json.
func TestHookInstallerMerge(t *testing.T) {
	a, h := setup(t, agenttest.WithNow(agenttest.FixedNow))
	agenttest.HookMerge(t, a.Neutral(), h.Neutral(), hooksSeed, false)
	hf, ok, err := a.HooksFile(h)
	if err != nil || !ok || hf.Description.V != "team hooks" || len(hf.Events[HookStop][0].Hooks) != 2 {
		t.Fatalf("hooks.json after the round trip: %+v %v", hf, err)
	}
}

// TestHookInstallerPlanCodex checks the event names, the review note and
// the feature-flag note; config.toml is never written.
func TestHookInstallerPlanCodex(t *testing.T) {
	a, h := setup(t, agenttest.WithNow(agenttest.FixedNow))
	inst, ok := agentio.HooksOf(a.Neutral())
	if !ok {
		t.Fatal("no HookInstaller")
	}
	want := []agentio.HookEntry{
		{Kind: agentio.HookPromptSubmit, Command: "c"}, {Kind: agentio.HookTurnEnd, Command: "c"},
		{Kind: agentio.HookSessionStart, Command: "c"}, {Kind: agentio.HookSessionEnd, Command: "c"},
	}
	p, err := inst.PlanHooks(h.Neutral(), want)
	if err != nil {
		t.Fatal(err)
	}
	var evs []string
	for _, e := range p.Add {
		evs = append(evs, e.Event)
	}
	if strings.Join(evs, ",") != "UserPromptSubmit,Stop,SessionStart,SessionEnd" || p.File != a.Paths(h).Hooks {
		t.Fatalf("plan %+v", p)
	}
	if len(p.Manual) != 1 || p.Manual[0] != manualReviewHooks {
		t.Fatalf("manual %v", p.Manual)
	}
	cfgPath := a.Paths(h).Config
	cfg, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	off := strings.Replace(string(cfg), "hooks = true", "hooks = false", 1)
	agenttest.WriteFile(t, "", cfgPath, off)
	if p, err = inst.PlanHooks(h.Neutral(), want); err != nil || len(p.Manual) != 2 || p.Manual[0] != manualFeatureHooks {
		t.Fatalf("feature off: %v %v", p.Manual, err)
	}
	if _, err := inst.ApplyHooks(p); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(cfgPath); string(got) != off {
		t.Fatal("config.toml was written")
	}
	if p, _ := inst.PlanHooks(h.Neutral(), want); len(p.Manual) != 1 {
		t.Fatalf("review note on a no-op plan: %v", p.Manual)
	}
}

// TestEnsureHookCodex: idempotent add on hooks.json, created when absent.
func TestEnsureHookCodex(t *testing.T) {
	a, h := setup(t, agenttest.WithNow(agenttest.FixedNow))
	if err := os.Remove(a.Paths(h).Hooks); err != nil {
		t.Fatal(err)
	}
	w := a.Writer()
	added, b, err := w.EnsureHook(h, HookStop, "/usr/local/bin/capture")
	if err != nil || !added || !b.IsEmpty() {
		t.Fatalf("first: %t %+v %v", added, b, err)
	}
	if added, _, err := w.EnsureHook(h, HookStop, "/usr/local/bin/capture"); err != nil || added {
		t.Fatalf("second: %t %v", added, err)
	}
	hf, ok, err := a.HooksFile(h)
	if err != nil || !ok || hf.Events[HookStop][0].Hooks[0].Command.V != "/usr/local/bin/capture" {
		t.Fatalf("%+v %v", hf, err)
	}
	if _, _, err := w.EnsureHook(h, HookStop, ""); err == nil {
		t.Fatal("empty command accepted")
	}
	if n, _, err := w.RemoveHooks(h, func(string) bool { return true }); err != nil || n != 1 {
		t.Fatalf("remove %d %v", n, err)
	}
}

// Unverified payloads: no Codex hook run is recorded yet. These follow the
// command-hook input schemas embedded in the Codex 0.162.1 binary, values
// replaced.
const (
	unverifiedPromptPayload = `{"session_id":"019a0000-0000-7000-8000-000000000001","transcript_path":"/home/test/.codex/sessions/2026/11/19/rollout-x.jsonl","cwd":"/work/app","hook_event_name":"UserPromptSubmit","model":"gpt-5.5","permission_mode":"default","prompt":"add a test","turn_id":"turn-1"}`
	unverifiedStopPayload   = `{"session_id":"019a0000-0000-7000-8000-000000000001","transcript_path":null,"cwd":"/work/app","hook_event_name":"Stop","model":"gpt-5.5","permission_mode":"default","last_assistant_message":"Added the test.","stop_hook_active":false,"turn_id":"turn-1","later_key":1}`
	unverifiedEndPayload    = `{"session_id":"s","transcript_path":"/t","cwd":"/w","hook_event_name":"SessionEnd","reason":"other"}`
)

// TestDecodeHookUnverified decodes the schema-shaped payloads.
func TestDecodeHookUnverified(t *testing.T) {
	inst, _ := agentio.HooksOf(New(agentio.Host{}).Neutral())
	c, err := inst.DecodeHook(strings.NewReader(unverifiedPromptPayload))
	if err != nil {
		t.Fatal(err)
	}
	if c.Agent != ID || c.Kind != agentio.HookPromptSubmit || c.ConversationID != "019a0000-0000-7000-8000-000000000001" || c.Dir != "/work/app" ||
		c.Prompt != "add a test" || c.Model != "gpt-5.5" || c.PermissionMode != "default" || !strings.HasSuffix(c.TranscriptPath, "rollout-x.jsonl") {
		t.Fatalf("%+v", c)
	}
	in, err := DecodeHookInput(strings.NewReader(unverifiedStopPayload))
	if err != nil {
		t.Fatal(err)
	}
	if in.Event != HookStop || in.TurnID != "turn-1" || in.LastAssistantMessage != "Added the test." || in.TranscriptPath != "" || in.Unknown["later_key"] == nil || string(in.Raw) != unverifiedStopPayload {
		t.Fatalf("%+v", in)
	}
	if c := in.Call(); c.Kind != agentio.HookTurnEnd || c.LastAssistantMessage != "Added the test." {
		t.Fatalf("%+v", c)
	}
	if c, err := inst.DecodeHook(strings.NewReader(unverifiedEndPayload)); err != nil || c.Kind != agentio.HookSessionEnd {
		t.Fatalf("%+v %v", c, err)
	}
	if in, err := DecodeHookInput(strings.NewReader(`{"event":"Stop"}`)); err != nil || in.Event != HookStop {
		t.Fatalf("fallback key: %+v %v", in, err)
	}
	if _, err := DecodeHookInput(strings.NewReader(`{"prompt":1}`)); err == nil {
		t.Fatal("wrong field type accepted")
	}
	if _, err := inst.DecodeHook(strings.NewReader(`nope`)); err == nil {
		t.Fatal("bad payload accepted")
	}
}
