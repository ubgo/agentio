package hookjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// settingsFixture is a realistic Claude Code settings.json: unrelated
// top-level keys in a hand-picked order, three foreign hooks on the events
// a capture tool installs into (one with a matcher and two handlers, one
// with an unknown handler key), and another event.
const settingsFixture = `{
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
        "matcher": "",
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

// ours is the command the tests install.
const ours = "/usr/local/bin/capture hook --agent claude"

func mustGroup(t *testing.T, cmd string) []byte {
	t.Helper()
	g, err := CommandGroup(cmd)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func mustAppend(t *testing.T, b []byte, event, cmd string, opt Options) []byte {
	t.Helper()
	out, err := AppendGroup(b, event, mustGroup(t, cmd), opt)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// topLevelRaw returns each top-level member's raw value, in file order.
func topLevelRaw(t *testing.T, b []byte) ([]string, map[string]string) {
	t.Helper()
	d, err := load(b, Options{Comments: true})
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	vals := map[string]string{}
	for _, m := range d.root.members {
		keys = append(keys, m.key)
		vals[m.key] = string(b[m.val.start:m.val.end])
	}
	return keys, vals
}

// TestAppendKeepsEverythingElse: an install adds only its own groups; every
// unrelated top-level key keeps its bytes and order, and every foreign
// handler keeps its bytes.
func TestAppendKeepsEverythingElse(t *testing.T) {
	src := []byte(settingsFixture)
	out := mustAppend(t, src, "UserPromptSubmit", ours, Options{})
	out = mustAppend(t, out, "Stop", ours, Options{})
	out = mustAppend(t, out, "SessionStart", ours, Options{})
	if !json.Valid(out) {
		t.Fatalf("invalid JSON:\n%s", out)
	}
	keysBefore, before := topLevelRaw(t, src)
	keysAfter, after := topLevelRaw(t, out)
	if strings.Join(keysBefore, ",") != strings.Join(keysAfter, ",") {
		t.Fatalf("top-level order changed: %v -> %v", keysBefore, keysAfter)
	}
	for k, v := range before {
		if k != keyHooks && after[k] != v {
			t.Errorf("top-level %q changed:\n%s\n->\n%s", k, v, after[k])
		}
	}
	hsBefore, err := Handlers(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	hsAfter, err := Handlers(out, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hsAfter) != len(hsBefore)+3 {
		t.Fatalf("handlers %d -> %d", len(hsBefore), len(hsAfter))
	}
	for _, h := range hsBefore {
		if !bytes.Contains(out, h.Raw) {
			t.Errorf("foreign handler bytes changed: %s", h.Raw)
		}
	}
	for _, ev := range []string{"UserPromptSubmit", "Stop", "SessionStart"} {
		if ok, _ := Has(out, ev, ours, Options{}); !ok {
			t.Errorf("%s: ours missing", ev)
		}
	}
	// The new group follows the file's two-space layout.
	want := "      {\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"" + ours + "\"\n          }\n        ]\n      }\n    ],\n    \"Stop\""
	if !strings.Contains(string(out), want) {
		t.Fatalf("layout:\n%s", out)
	}
}

// TestRemoveRestoresBytes: uninstalling what an install added gives back
// the original file byte for byte; foreign hooks on the same events stay.
func TestRemoveRestoresBytes(t *testing.T) {
	src := []byte(settingsFixture)
	out := mustAppend(t, src, "UserPromptSubmit", ours, Options{})
	out = mustAppend(t, out, "Stop", ours, Options{})
	out = mustAppend(t, out, "SessionStart", ours, Options{})
	back, n, err := RemoveHandlers(out, func(h Handler) bool { return h.IsCommand && h.Command == ours }, Options{})
	if err != nil || n != 3 {
		t.Fatalf("removed %d, %v", n, err)
	}
	if string(back) != settingsFixture {
		t.Fatalf("not restored:\n%s", back)
	}
}

// TestRemoveDropsOnlyEmptyGroups: removing one handler of a two-handler
// group keeps the group and its other handler byte for byte; removing the
// sole handler of a group drops the group.
func TestRemoveDropsOnlyEmptyGroups(t *testing.T) {
	src := []byte(settingsFixture)
	out, n, err := RemoveHandlers(src, func(h Handler) bool { return h.Command == "/usr/local/bin/other-tool stop" }, Options{})
	if err != nil || n != 1 {
		t.Fatalf("removed %d, %v", n, err)
	}
	if !strings.Contains(string(out), `"matcher": "",`) || !strings.Contains(string(out), `"futureKey": {"x": 1}`) {
		t.Fatalf("group or sibling lost:\n%s", out)
	}
	out, n, err = RemoveHandlers(out, func(h Handler) bool { return h.Event == "PreToolUse" }, Options{})
	if err != nil || n != 1 || strings.Contains(string(out), "PreToolUse") {
		t.Fatalf("emptied event kept: %d %v\n%s", n, err, out)
	}
	if !json.Valid(out) {
		t.Fatalf("invalid:\n%s", out)
	}
	_, n, err = RemoveHandlers(out, func(Handler) bool { return false }, Options{})
	if err != nil || n != 0 {
		t.Fatal("no-op remove")
	}
}

// TestEmptyAndMissing: an empty file becomes a fresh document; removing
// everything from it leaves an empty object.
func TestEmptyAndMissing(t *testing.T) {
	out := mustAppend(t, nil, "Stop", ours, Options{})
	want := "{\n  \"hooks\": {\n    \"Stop\": [\n      {\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n            \"command\": \"" + ours + "\"\n          }\n        ]\n      }\n    ]\n  }\n}\n"
	if string(out) != want {
		t.Fatalf("fresh document:\n%s", out)
	}
	back, _, err := RemoveHandlers(out, func(Handler) bool { return true }, Options{})
	if err != nil || string(back) != "{}\n" {
		t.Fatalf("emptied: %q %v", back, err)
	}
	out = mustAppend(t, []byte("{}"), "Stop", ours, Options{})
	if string(out) != "{\"hooks\":{\"Stop\":[{\"hooks\":[{\"type\":\"command\",\"command\":\""+ours+"\"}]}]}}" {
		t.Fatalf("single-line {}: %s", out)
	}
	out = mustAppend(t, []byte("{\n}\n"), "Stop", ours, Options{})
	if !json.Valid(out) || !strings.HasPrefix(string(out), "{\n  \"hooks\": {\n") {
		t.Fatalf("multi-line {}: %s", out)
	}
}

// TestNullAndEmptyContainers covers "hooks": null, an empty hooks object
// and an empty event array.
func TestNullAndEmptyContainers(t *testing.T) {
	for _, src := range []string{
		"{\n  \"a\": 1,\n  \"hooks\": null\n}\n",
		"{\n  \"a\": 1,\n  \"hooks\": {}\n}\n",
		"{\n  \"a\": 1,\n  \"hooks\": {\n    \"Stop\": []\n  }\n}\n",
		"{\n  \"a\": 1,\n  \"hooks\": {\n    \"Stop\": null\n  }\n}\n",
	} {
		out := mustAppend(t, []byte(src), "Stop", ours, Options{})
		if !json.Valid(out) {
			t.Fatalf("%q -> invalid:\n%s", src, out)
		}
		if ok, _ := Has(out, "Stop", ours, Options{}); !ok {
			t.Fatalf("%q: not added:\n%s", src, out)
		}
		if !strings.HasPrefix(string(out), "{\n  \"a\": 1,\n  \"hooks\": ") {
			t.Fatalf("%q: prefix changed:\n%s", src, out)
		}
	}
}

// TestMinifiedStaysOnOneLine: a single-line file gets a single-line edit.
func TestMinifiedStaysOnOneLine(t *testing.T) {
	src := `{"model":"opus","hooks":{"Stop":[{"hooks":[{"type":"command","command":"x"}]}]},"theme":"dark"}`
	out := mustAppend(t, []byte(src), "Stop", ours, Options{})
	if bytes.ContainsRune(out, '\n') || !json.Valid(out) {
		t.Fatalf("minified: %s", out)
	}
	back, _, _ := RemoveHandlers(out, func(h Handler) bool { return h.Command == ours }, Options{})
	if string(back) != src {
		t.Fatalf("not restored: %s", back)
	}
}

// TestCommentsAndBOM: Gemini's commented settings keep their comments,
// and a BOM stays in front.
func TestCommentsAndBOM(t *testing.T) {
	src := bom + "// user settings\n{\n  /* model */ \"model\": {\"name\": \"x\"}, // trailing\n  \"ui\": {\"theme\": \"GitHub\"}\n}\n"
	if _, err := AppendGroup([]byte(src), "AfterAgent", mustGroup(t, ours), Options{}); !errors.Is(err, errSyntax) {
		t.Fatalf("comments accepted without the option: %v", err)
	}
	out := mustAppend(t, []byte(src), "AfterAgent", ours, Options{Comments: true})
	for _, keep := range []string{bom + "// user settings\n", "/* model */", "// trailing\n"} {
		if !strings.Contains(string(out), keep) {
			t.Fatalf("lost %q:\n%s", keep, out)
		}
	}
	back, _, err := RemoveHandlers(out, func(h Handler) bool { return h.Command == ours }, Options{Comments: true})
	if err != nil || string(back) != src {
		t.Fatalf("not restored (%v):\n%s", err, back)
	}
	only := "// nothing yet\n"
	out = mustAppend(t, []byte(only), "AfterAgent", ours, Options{Comments: true})
	if !strings.HasPrefix(string(out), only+"{\n") {
		t.Fatalf("comment-only file:\n%s", out)
	}
}

// TestShellCharactersStayLiteral: '&&' and '>' are written as typed and
// the command round-trips.
func TestShellCharactersStayLiteral(t *testing.T) {
	cmd := `cd "/tmp/x" && run --out > log.txt 2>&1 \ <in`
	out := mustAppend(t, nil, "Stop", cmd, Options{})
	if !strings.Contains(string(out), `&& run --out > log.txt 2>&1 \\ <in`) {
		t.Fatalf("escaped: %s", out)
	}
	if ok, _ := Has(out, "Stop", cmd, Options{}); !ok {
		t.Fatal("command does not round-trip")
	}
	if got := string(unescapeHTML([]byte(`"\\u0026 \u0026"`))); got != `"\\u0026 &"` {
		t.Fatalf("escaped backslash mistaken for an escape: %s", got)
	}
}

// TestShapeErrors: a file this package cannot edit safely is refused.
func TestShapeErrors(t *testing.T) {
	for _, src := range []string{`[]`, `{"hooks": []}`, `{"hooks": {"Stop": {}}}`} {
		if _, err := AppendGroup([]byte(src), "Stop", mustGroup(t, ours), Options{}); !errors.Is(err, ErrShape) {
			t.Errorf("%s: %v", src, err)
		}
	}
	for _, src := range []string{`{"a":}`, `{"a":1,}`, `{"a":1} x`, `{"a":"`, `{"a":tru}`, strings.Repeat("[", maxDepth+1)} {
		if _, err := AppendGroup([]byte(src), "Stop", mustGroup(t, ours), Options{}); !errors.Is(err, errSyntax) {
			t.Errorf("%s: %v", src, err)
		}
	}
	if _, err := AppendGroup([]byte(`{}`), "Stop", []byte(`[1]`), Options{}); err == nil {
		t.Error("non-object group accepted")
	}
}

// TestHandlerKinds: type-less command handlers count as commands; http
// handlers do not; malformed groups are skipped, not fatal.
func TestHandlerKinds(t *testing.T) {
	src := `{"hooks":{"Stop":[{"hooks":[{"command":"a"},{"type":"http","url":"u","command":"b"}]},"junk",{"hooks":"x"}],"Odd":{"k":1}}}`
	hs, err := Handlers([]byte(src), Options{})
	if err != nil || len(hs) != 2 || !hs[0].IsCommand || hs[1].IsCommand {
		t.Fatalf("%+v %v", hs, err)
	}
	if ok, _ := Has([]byte(src), "Stop", "b", Options{}); ok {
		t.Fatal("http handler counted as a command hook")
	}
}

// TestDuplicateKeyLastWins: like encoding/json, the last "hooks" counts.
func TestDuplicateKeyLastWins(t *testing.T) {
	src := `{"hooks":{"Stop":[]},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"x"}]}]}}`
	if ok, _ := Has([]byte(src), "Stop", "x", Options{}); !ok {
		t.Fatal("first duplicate used")
	}
}
