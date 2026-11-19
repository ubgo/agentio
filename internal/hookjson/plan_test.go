package hookjson

import (
	"errors"
	"testing"

	"github.com/ubgo/agentio"
)

// eventOf maps only two kinds, like a vendor with a partial event set.
func eventOf(k agentio.HookKind) (string, bool) {
	switch k {
	case agentio.HookPromptSubmit:
		return "UserPromptSubmit", true
	case agentio.HookTurnEnd:
		return "Stop", true
	}
	return "", false
}

// TestResolve fills events, drops repeats and refuses unknown kinds and
// empty commands.
func TestResolve(t *testing.T) {
	got, err := Resolve("claude", []agentio.HookEntry{
		{Kind: agentio.HookTurnEnd, Command: "c", Event: "Wrong"},
		{Kind: agentio.HookTurnEnd, Command: "c"},
		{Kind: agentio.HookPromptSubmit, Command: "c"},
	}, eventOf, "why")
	if err != nil || len(got) != 2 || got[0].Event != "Stop" || got[1].Event != "UserPromptSubmit" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := Resolve("claude", []agentio.HookEntry{{Kind: agentio.HookSessionEnd, Command: "c"}}, eventOf, "why"); !errors.Is(err, agentio.ErrUnsupported) {
		t.Fatalf("unsupported kind: %v", err)
	}
	if _, err := Resolve("claude", []agentio.HookEntry{{Kind: agentio.HookTurnEnd}}, eventOf, "why"); !errors.Is(err, agentio.ErrInvalidHook) {
		t.Fatalf("empty command: %v", err)
	}
}

// TestSplitAndCheckPlan partitions against the fixture and guards plans.
func TestSplitAndCheckPlan(t *testing.T) {
	entries := []agentio.HookEntry{
		{Kind: agentio.HookPromptSubmit, Event: "UserPromptSubmit", Command: "/usr/local/bin/other-tool notify"},
		{Kind: agentio.HookTurnEnd, Event: "Stop", Command: ours},
	}
	present, missing, err := Split([]byte(settingsFixture), entries, Options{})
	if err != nil || len(present) != 1 || len(missing) != 1 || missing[0].Command != ours {
		t.Fatalf("%+v %+v %v", present, missing, err)
	}
	if _, _, err := Split([]byte(`[`), entries, Options{}); err == nil {
		t.Fatal("bad file split")
	}
	out, err := AppendEntries([]byte(settingsFixture), missing, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, missing, _ := Split(out, entries, Options{}); len(missing) != 0 {
		t.Fatal("append did not add")
	}
	p := agentio.HookPlan{Agent: "claude", File: "/h/settings.json", Add: missing}
	if err := CheckPlan(p, "claude", "/h/settings.json"); err != nil {
		t.Fatal(err)
	}
	if err := CheckPlan(p, "codex", "/h/settings.json"); !errors.Is(err, agentio.ErrForeignHome) {
		t.Fatal(err)
	}
	if err := CheckPlan(p, "claude", "/h/other.json"); !errors.Is(err, agentio.ErrInvalidHook) {
		t.Fatal(err)
	}
	p.Add = []agentio.HookEntry{{Command: "x"}}
	if err := CheckPlan(p, "claude", "/h/settings.json"); !errors.Is(err, agentio.ErrInvalidHook) {
		t.Fatal(err)
	}
	if !DropCommands(func(c string) bool { return c == "x" })(Handler{IsCommand: true, Command: "x"}) {
		t.Fatal("DropCommands")
	}
}
