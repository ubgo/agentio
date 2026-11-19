package agenttest

import (
	"bytes"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/internal/hookjson"
)

// Commands HookMerge installs. Fake paths: nothing ever runs them.
const (
	hookMergeCommand = "/usr/local/bin/agenttest-capture hook"
	hookMergeLate    = "/usr/local/bin/agenttest-capture late"
)

// HookMerge checks the merge guarantees of a's HookInstaller on home h,
// whose hook file it first fills with seed (a realistic settings file
// holding other tools' hooks, ideally on the same events). It checks that:
//
//   - PlanHooks of every kind in HookKinds reports all of them as Add;
//   - ApplyHooks backs the file up first (the Backup copy holds seed) and
//     adds only its own entries: removing them from the result gives seed
//     back byte for byte, so every foreign hook, unknown key, top-level key
//     and their order are untouched;
//   - a second plan has every entry Present, and applying it writes
//     nothing and returns an empty Backup;
//   - an entry that appeared between PlanHooks and ApplyHooks is not added
//     twice;
//   - RemoveHooks removes only the matching commands and leaves seed
//     byte for byte; a second RemoveHooks removes nothing and writes
//     nothing;
//   - an unknown kind is ErrUnsupported, an empty command ErrInvalidHook,
//     a plan whose File was changed ErrInvalidHook.
//
// comments says the file format allows // and /* */ comments (Gemini).
// Disk · Write (inside h only).
func HookMerge(t *testing.T, a agentio.Agent, h agentio.Home, seed string, comments bool) {
	t.Helper()
	inst, ok := agentio.HooksOf(a)
	if !ok {
		t.Fatalf("%s has no HookInstaller", a.ID())
	}
	opt := hookjson.Options{Comments: comments}
	var want []agentio.HookEntry
	for _, k := range inst.HookKinds() {
		want = append(want, agentio.HookEntry{Kind: k, Command: hookMergeCommand})
	}
	p, err := inst.PlanHooks(h, want)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	WriteFile(t, "", p.File, seed)
	if p, err = inst.PlanHooks(h, want); err != nil || len(p.Add) != len(want) || len(p.Present) != 0 {
		t.Fatalf("plan on seed: add=%d present=%d err=%v", len(p.Add), len(p.Present), err)
	}
	for _, e := range p.Add {
		if e.Event == "" {
			t.Fatalf("plan entry without a vendor event: %+v", e)
		}
	}
	b, err := inst.ApplyHooks(p)
	if err != nil || len(b.Entries) != 1 {
		t.Fatalf("apply: %+v %v", b, err)
	}
	if saved := readFile(t, b.Entries[0].Copy); saved != seed {
		t.Fatalf("backup does not hold the original:\n%s", saved)
	}
	after := readFile(t, p.File)
	checkOnlyOurs(t, after, seed, opt, hookMergeCommand)
	for _, e := range p.Add {
		if ok, err := hookjson.Has([]byte(after), e.Event, e.Command, opt); err != nil || !ok {
			t.Fatalf("%s not installed (%v):\n%s", e.Event, err, after)
		}
	}

	again, err := inst.PlanHooks(h, want)
	if err != nil || !again.IsNoop() || len(again.Present) != len(want) {
		t.Fatalf("second plan: add=%d present=%d err=%v", len(again.Add), len(again.Present), err)
	}
	if b, err := inst.ApplyHooks(again); err != nil || !b.IsEmpty() || readFile(t, p.File) != after {
		t.Fatalf("second apply wrote: %+v %v", b, err)
	}

	late := []agentio.HookEntry{{Kind: want[0].Kind, Command: hookMergeLate}}
	stale, err := inst.PlanHooks(h, late)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := inst.PlanHooks(h, late)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inst.ApplyHooks(fresh); err != nil {
		t.Fatal(err)
	}
	if b, err := inst.ApplyHooks(stale); err != nil || !b.IsEmpty() {
		t.Fatalf("stale plan wrote: %+v %v", b, err)
	}
	if n := strings.Count(readFile(t, p.File), hookMergeLate); n != 1 {
		t.Fatalf("late entry written %d times", n)
	}

	ours := func(c string) bool { return c == hookMergeCommand || c == hookMergeLate }
	n, b, err := inst.RemoveHooks(h, ours)
	if err != nil || n != len(want)+1 || b.IsEmpty() {
		t.Fatalf("remove: %d %+v %v", n, b, err)
	}
	if got := readFile(t, p.File); got != seed {
		t.Fatalf("remove did not restore the original:\n%s", got)
	}
	if n, b, err := inst.RemoveHooks(h, ours); err != nil || n != 0 || !b.IsEmpty() {
		t.Fatalf("second remove: %d %+v %v", n, b, err)
	}

	if _, err := inst.PlanHooks(h, []agentio.HookEntry{{Kind: agentio.HookUnknown, Command: hookMergeCommand}}); !errors.Is(err, agentio.ErrUnsupported) {
		t.Errorf("unknown kind: %v", err)
	}
	if _, err := inst.PlanHooks(h, []agentio.HookEntry{{Kind: want[0].Kind}}); !errors.Is(err, agentio.ErrInvalidHook) {
		t.Errorf("empty command: %v", err)
	}
	moved := p
	moved.File += ".other"
	if _, err := inst.ApplyHooks(moved); !errors.Is(err, agentio.ErrInvalidHook) {
		t.Errorf("plan for another file: %v", err)
	}
}

// checkOnlyOurs fails unless removing every handler running one of cmds
// from after gives seed back exactly, and every foreign handler of seed
// still appears verbatim in after.
func checkOnlyOurs(t *testing.T, after, seed string, opt hookjson.Options, cmds ...string) {
	t.Helper()
	back, _, err := hookjson.RemoveHandlers([]byte(after), func(h hookjson.Handler) bool {
		return h.IsCommand && slices.Contains(cmds, h.Command)
	}, opt)
	if err != nil {
		t.Fatal(err)
	}
	if string(back) != seed {
		t.Fatalf("install changed bytes outside its own entries:\n%s", after)
	}
	foreign, err := hookjson.Handlers([]byte(seed), opt)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range foreign {
		if !bytes.Contains([]byte(after), h.Raw) {
			t.Errorf("foreign hook changed: %s", h.Raw)
		}
	}
}

// readFile returns a file's content or fails the test.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
