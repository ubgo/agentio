package agentio_test

import (
	"errors"
	"testing"

	"github.com/ubgo/agentio"
)

// TestTrimLineEnd drops "\n", "\r\n" or a lone "\r", and nothing else.
func TestTrimLineEnd(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"{}\n": "{}", "{}\r\n": "{}", "{}\r": "{}", "{}": "{}", " {} \n": " {} ", "\n": "", "{}\n\n": "{}\n",
	} {
		if got := string(agentio.TrimLineEnd([]byte(in))); got != want {
			t.Errorf("TrimLineEnd(%q) = %q, want %q", in, got, want)
		}
	}
}

// plainTurnFiles is a TurnFileReader without BadLineReporter.
type plainTurnFiles struct{ calls *int }

func (p plainTurnFiles) TurnsAt(string, agentio.Offset, func(agentio.Turn) error) (agentio.Offset, error) {
	*p.calls++
	return 7, nil
}

func (plainTurnFiles) TranscriptOwner(string) (agentio.TranscriptRef, error) {
	return agentio.TranscriptRef{}, agentio.ErrNotFound
}

// reportingTurnFiles reports one bad line.
type reportingTurnFiles struct{ plainTurnFiles }

func (reportingTurnFiles) TurnsAtReporting(path string, _ agentio.Offset, _ func(agentio.Turn) error, bad func(agentio.BadLine)) (agentio.Offset, error) {
	bad(agentio.BadLine{Path: path, Offset: 3, Err: errors.New("torn")})
	return 9, nil
}

// TestTurnsAtReporting uses the reporting form when the reader has one
// and falls back to TurnsAt (bad never called) otherwise.
func TestTurnsAtReporting(t *testing.T) {
	t.Parallel()
	calls := 0
	var bads []agentio.BadLine
	bad := func(b agentio.BadLine) { bads = append(bads, b) }
	off, err := agentio.TurnsAtReporting(plainTurnFiles{&calls}, "p", 0, nil, bad)
	if err != nil || off != 7 || calls != 1 || len(bads) != 0 {
		t.Errorf("fallback: %d %v, %d calls, %v", off, err, calls, bads)
	}
	off, err = agentio.TurnsAtReporting(reportingTurnFiles{plainTurnFiles{&calls}}, "p", 0, nil, bad)
	if err != nil || off != 9 || len(bads) != 1 || bads[0].Offset != 3 || bads[0].Path != "p" {
		t.Errorf("reporting: %d %v %v", off, err, bads)
	}
}

// TestHookPlanMatching returns the existing entries a RemoveHooks call
// with the same match would drop.
func TestHookPlanMatching(t *testing.T) {
	t.Parallel()
	p := agentio.HookPlan{Existing: []agentio.HookEntry{
		{Kind: agentio.HookTurnEnd, Event: "Stop", Command: "/old/bin/ailog hook stop"},
		{Kind: agentio.HookTurnEnd, Event: "Stop", Command: "/usr/local/bin/other"},
		{Kind: agentio.HookPromptSubmit, Event: "UserPromptSubmit", Command: "/old/bin/ailog hook prompt"},
	}}
	got := p.Matching(func(c string) bool { return len(c) > 8 && c[:8] == "/old/bin" })
	if len(got) != 2 || got[0].Event != "Stop" || got[1].Kind != agentio.HookPromptSubmit {
		t.Errorf("Matching = %+v", got)
	}
	if (agentio.HookPlan{}).Matching(func(string) bool { return true }) != nil {
		t.Error("no existing entries, no matches")
	}
}
