package codex

import (
	"errors"
	"testing"

	"github.com/ubgo/agentio"
)

// TestLaunchOptionKeys: every option has a flag but the positional prompt;
// spot checks match the builder; the spec 7.3 block decodes.
func TestLaunchOptionKeys(t *testing.T) {
	t.Parallel()
	keys, err := LaunchOptionKeys()
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]agentio.OptionKey{}
	for _, k := range keys {
		by[k.Key] = k
		if k.Flag == "" && k.Key != "prompt" {
			t.Errorf("%s has no flag", k.Key)
		}
	}
	if k := by["codex_profile"]; k.Flag != flagProfile || k.Kind != agentio.OptionString {
		t.Errorf("codex_profile = %+v", k)
	}
	if k := by["sandbox"]; k.Kind != agentio.OptionEnum || len(k.Values) != 3 || k.Flag != flagSandbox {
		t.Errorf("sandbox = %+v", k)
	}
	if k := by["bypass_all"]; k.Flag != flagBypass {
		t.Errorf("bypass_all = %+v", k)
	}
	o, err := DecodeLaunchOptions([]byte(`{"codex_profile": "deep", "search": true}`))
	if err != nil || o.Profile != "deep" || !o.Search {
		t.Fatalf("%+v %v", o, err)
	}
	if _, err := DecodeLaunchOptions([]byte(`{"effort": "high"}`)); !errors.Is(err, agentio.ErrInvalidOption) {
		t.Errorf("a claude key on codex = %v", err)
	}
	if err := ParseLaunchOption(&o, "sandbox", "workspace-write"); err != nil || o.Sandbox != ModeWorkspaceWrite {
		t.Errorf("sandbox = %v %v", o.Sandbox, err)
	}
	if err := ParseLaunchOption(&o, "sandbox", "external-sandbox"); !errors.Is(err, agentio.ErrInvalidOption) {
		t.Errorf("external-sandbox is not a CLI value: %v", err)
	}
}
