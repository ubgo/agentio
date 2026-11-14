package builtin

import (
	"errors"
	"slices"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
	"github.com/ubgo/agentio/gemini"
	"github.com/ubgo/agentio/pi"
)

// sampleSpec is a minimal config-only agent used to prove custom specs load.
func sampleSpec(id agentio.ID) agentio.Spec {
	return agentio.Spec{
		ID: id, Name: "Sample", Binary: "sample",
		HomeEnv: "SAMPLE_HOME", DefaultHome: ".sample", HomeMarkers: []string{"config.json"},
		NewArgs: []string{"{args}"}, ResumeArgs: []string{"--resume", "{id}", "{args}"},
	}
}

func TestNewOrderAndLookup(t *testing.T) {
	h := agenttest.NewHost(t)
	a, err := New(h, sampleSpec("sample"))
	if err != nil {
		t.Fatal(err)
	}
	all := a.All()
	if want := []agentio.ID{claude.ID, codex.ID, gemini.ID, pi.ID, "sample"}; !slices.Equal(ids(all), want) {
		t.Fatalf("order = %v, want %v", ids(all), want)
	}
	if _, ok := a.ByID("sample"); !ok {
		t.Fatal("ByID(sample) not found")
	}
	if _, ok := a.ByID("missing"); ok {
		t.Fatal("ByID(missing) found")
	}
}

func TestNewRejectsBuiltinAndDuplicateIDs(t *testing.T) {
	h := agenttest.NewHost(t)
	for _, id := range []agentio.ID{claude.ID, codex.ID, gemini.ID, pi.ID} {
		if _, err := New(h, sampleSpec(id)); !errors.Is(err, agentio.ErrInvalidSpec) {
			t.Fatalf("built-in id %s: err = %v", id, err)
		}
	}
	if _, err := New(h, sampleSpec("x"), sampleSpec("x")); !errors.Is(err, agentio.ErrInvalidSpec) {
		t.Fatalf("duplicate id: err = %v", err)
	}
}

func ids(as []agentio.Agent) []agentio.ID {
	out := make([]agentio.ID, 0, len(as))
	for _, a := range as {
		out = append(out, a.ID())
	}
	return out
}
