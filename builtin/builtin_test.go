package builtin

import (
	"errors"
	"testing"

	"github.com/khanakia/agentcli"
	"github.com/khanakia/agentcli/agenttest"
	"github.com/khanakia/agentcli/claude"
	"github.com/khanakia/agentcli/codex"
)

// sampleSpec is a minimal config-only agent used to prove custom specs load.
func sampleSpec(id agentcli.ID) agentcli.Spec {
	return agentcli.Spec{
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
	if len(all) != 3 || all[0].ID() != claude.ID || all[1].ID() != codex.ID || all[2].ID() != "sample" {
		t.Fatalf("order = %v", ids(all))
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
	if _, err := New(h, sampleSpec(claude.ID)); !errors.Is(err, agentcli.ErrInvalidSpec) {
		t.Fatalf("built-in id: err = %v", err)
	}
	if _, err := New(h, sampleSpec("x"), sampleSpec("x")); !errors.Is(err, agentcli.ErrInvalidSpec) {
		t.Fatalf("duplicate id: err = %v", err)
	}
}

func ids(as []agentcli.Agent) []agentcli.ID {
	out := make([]agentcli.ID, 0, len(as))
	for _, a := range as {
		out = append(out, a.ID())
	}
	return out
}
