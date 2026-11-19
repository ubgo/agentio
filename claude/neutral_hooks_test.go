package claude

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// hookSeed is a realistic user settings.json: top-level keys in a hand
// order, three foreign hooks on the events a capture tool installs into
// (one with a matcher, one group with two handlers and an unknown handler
// key), another event, and a status line.
const hookSeed = `{
  "permissions": {
    "allow": ["Bash(git status)"],
    "defaultMode": "acceptEdits"
  },
  "model": "opus",
  "hooks": {
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/usr/local/bin/other-tool notify",
            "timeout": 5
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/usr/local/bin/other-tool stop"
          },
          {
            "type": "command",
            "command": "/opt/notifier/bin/ding --quiet",
            "futureKey": {"x": 1}
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [{"type": "command", "command": "/usr/local/bin/other-tool start"}]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{"type": "command", "command": "/usr/local/bin/guard"}]
      }
    ]
  },
  "statusLine": {"type": "command", "command": "/usr/local/bin/line"},
  "theme": "dark"
}
`

// TestHookInstallerMerge runs the shared merge guarantees on a home whose
// settings.json holds other tools' hooks.
func TestHookInstallerMerge(t *testing.T) {
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithNow(agenttest.FixedNow))
	agenttest.HookMerge(t, a.Neutral(), a.DefaultHome().Neutral(), hookSeed, false)
}

// TestHookInstallerPlan checks the Claude event names, the user settings
// file and the disableAllHooks note.
func TestHookInstallerPlan(t *testing.T) {
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithNow(agenttest.FixedNow))
	inst, ok := agentio.HooksOf(a.Neutral())
	if !ok {
		t.Fatal("no HookInstaller")
	}
	h := a.DefaultHome()
	path := agenttest.WriteFile(t, h.Dir(), settingsFile, `{"disableAllHooks": true}`)
	p, err := inst.PlanHooks(h.Neutral(), []agentio.HookEntry{
		{Kind: agentio.HookPromptSubmit, Command: "c"}, {Kind: agentio.HookTurnEnd, Command: "c", Event: "Ignored"},
		{Kind: agentio.HookSessionStart, Command: "c"}, {Kind: agentio.HookSessionEnd, Command: "c"},
		{Kind: agentio.HookSessionEnd, Command: "c"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var evs []string
	for _, e := range p.Add {
		evs = append(evs, e.Event)
	}
	if strings.Join(evs, ",") != "UserPromptSubmit,Stop,SessionStart,SessionEnd" || p.File != path || p.Agent != ID {
		t.Fatalf("plan %+v", p)
	}
	if len(p.Manual) != 1 || !strings.Contains(p.Manual[0], "disableAllHooks") {
		t.Fatalf("manual %v", p.Manual)
	}
	foreign := p
	foreign.Agent = "codex"
	if _, err := inst.ApplyHooks(foreign); !errors.Is(err, agentio.ErrForeignHome) {
		t.Fatalf("foreign plan: %v", err)
	}
	if b, err := inst.ApplyHooks(agentio.HookPlan{Agent: ID, Home: h.Neutral(), File: path}); err != nil || !b.IsEmpty() {
		t.Fatalf("empty plan wrote: %+v %v", b, err)
	}
}

// TestHookInstallerMissingFile installs into a home without settings.json:
// the file is created, there is nothing to back up.
func TestHookInstallerMissingFile(t *testing.T) {
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithNow(agenttest.FixedNow))
	h := a.DefaultHome()
	if err := os.Remove(filepath.Join(h.Dir(), settingsFile)); err != nil {
		t.Fatal(err)
	}
	inst, _ := agentio.HooksOf(a.Neutral())
	p, err := inst.PlanHooks(h.Neutral(), []agentio.HookEntry{{Kind: agentio.HookTurnEnd, Command: "/usr/local/bin/x"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := inst.ApplyHooks(p)
	if err != nil || !b.IsEmpty() {
		t.Fatalf("apply: %+v %v", b, err)
	}
	s, err := a.UserSettings(h)
	if err != nil || len(s.Settings.Hooks[HookStop]) != 1 {
		t.Fatalf("settings %+v %v", s.Settings.Hooks, err)
	}
}

// TestDecodeHookCall maps the recorded Claude Code payload shape.
func TestDecodeHookCall(t *testing.T) {
	inst, _ := agentio.HooksOf(New(agentio.Host{}).Neutral())
	payload := `{"session_id":"s1","transcript_path":"/t.jsonl","cwd":"/w","hook_event_name":"UserPromptSubmit","permission_mode":"default","prompt":"hi","model":"opus","new_key":1}`
	c, err := inst.DecodeHook(strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if c.Agent != ID || c.Kind != agentio.HookPromptSubmit || c.Event != "UserPromptSubmit" || c.ConversationID != "s1" || c.Dir != "/w" ||
		c.TranscriptPath != "/t.jsonl" || c.Prompt != "hi" || c.PermissionMode != string(ModeManual) || c.Model != "opus" || string(c.Raw) != payload {
		t.Fatalf("%+v", c)
	}
	c, err = inst.DecodeHook(strings.NewReader(`{"hook_event_name":"Stop","last_assistant_message":"done"}`))
	if err != nil || c.Kind != agentio.HookTurnEnd || c.LastAssistantMessage != "done" {
		t.Fatalf("%+v %v", c, err)
	}
	if c, _ := inst.DecodeHook(strings.NewReader(`{"hook_event_name":"PreToolUse"}`)); c.Kind != agentio.HookUnknown || c.Event != "PreToolUse" {
		t.Fatalf("%+v", c)
	}
	if _, err := inst.DecodeHook(strings.NewReader(`[`)); err == nil {
		t.Fatal("bad payload accepted")
	}
}

// TestEnsureHook: a second EnsureHook with the same command writes
// nothing; another tool's handler with the same command and another
// timeout counts as present; a new handler in the same group is added
// alone.
func TestEnsureHook(t *testing.T) {
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithNow(agenttest.FixedNow))
	h := a.DefaultHome()
	path := agenttest.WriteFile(t, h.Dir(), settingsFile, hookSeed)
	w := a.Writer()
	m := HookMatcher{Hooks: []HookHandler{{Type: HookTypeCommand, Command: "/usr/local/bin/capture", Timeout: 5 * time.Second}}}
	added, b, err := w.EnsureHook(h, nil, ScopeUser, HookStop, m)
	if err != nil || !added || b.IsEmpty() {
		t.Fatalf("first: %t %+v %v", added, b, err)
	}
	before, _ := os.ReadFile(path)
	added, b, err = w.EnsureHook(h, nil, ScopeUser, HookStop, m)
	if err != nil || added || !b.IsEmpty() {
		t.Fatalf("second: %t %+v %v", added, b, err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("second EnsureHook changed the file")
	}
	other := HookMatcher{Hooks: []HookHandler{{Type: HookTypeCommand, Command: "/usr/local/bin/other-tool stop", Timeout: time.Minute}}}
	if added, _, _ := w.EnsureHook(h, nil, ScopeUser, HookStop, other); added {
		t.Fatal("same command with another timeout added")
	}
	mixed := HookMatcher{Hooks: []HookHandler{
		{Type: HookTypeCommand, Command: "/usr/local/bin/capture"},
		{Type: HookTypeHTTP, URL: "http://127.0.0.1:9/hook"},
	}}
	if added, _, err := w.EnsureHook(h, nil, ScopeUser, HookStop, mixed); err != nil || !added {
		t.Fatalf("mixed: %t %v", added, err)
	}
	s, _ := a.UserSettings(h)
	groups := s.Settings.Hooks[HookStop]
	if last := groups[len(groups)-1]; len(last.Hooks) != 1 || last.Hooks[0].Type != HookTypeHTTP {
		t.Fatalf("last group %+v", last)
	}
	if added, _, _ := w.EnsureHook(h, nil, ScopeUser, HookStop, mixed); added {
		t.Fatal("http handler with the same JSON added twice")
	}
}

// TestAddHookKeepsBytes: the long-standing AddHook / RemoveHooks pair now
// splices too, so a round trip gives the original file back.
func TestAddHookKeepsBytes(t *testing.T) {
	a, _ := fixtureAgent(t, fixtureVersions(t)[0], agenttest.WithNow(agenttest.FixedNow))
	h := a.DefaultHome()
	path := agenttest.WriteFile(t, h.Dir(), settingsFile, hookSeed)
	w := a.Writer()
	if _, err := w.AddHook(h, nil, ScopeUser, HookStop, HookMatcher{Matcher: "", Hooks: []HookHandler{{Type: HookTypeCommand, Command: "a && b > c"}}}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), `"command": "a && b > c"`) {
		t.Fatalf("command escaped:\n%s", got)
	}
	n, _, err := w.RemoveHooks(h, nil, ScopeUser, func(_ HookEvent, hh HookHandler) bool { return hh.Command != "a && b > c" })
	if err != nil || n != 1 {
		t.Fatalf("remove %d %v", n, err)
	}
	if got, _ := os.ReadFile(path); string(got) != hookSeed {
		t.Fatalf("not restored:\n%s", got)
	}
	agenttest.WriteFile(t, h.Dir(), settingsFile, `{"hooks": []}`)
	if _, err := w.AddHook(h, nil, ScopeUser, HookStop, HookMatcher{}); err == nil {
		t.Fatal("hooks array rewritten")
	}
}
