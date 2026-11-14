package claude

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/ubgo/agentio"
)

// TestLaunchOptionKeys: every field is in the registry with a flag (except
// the positional prompt and verbatim args), and spot checks match the
// builder.
func TestLaunchOptionKeys(t *testing.T) {
	t.Parallel()
	keys, err := LaunchOptionKeys()
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]agentio.OptionKey{}
	for _, k := range keys {
		by[k.Key] = k
		if k.Flag == "" && k.Key != "prompt" && k.Key != "extra_args" {
			t.Errorf("%s has no flag", k.Key)
		}
	}
	for key, want := range map[string]struct {
		flag string
		kind agentio.OptionKind
	}{
		"effort":                    {flagEffort, agentio.OptionEnum},
		"append_system_prompt_file": {flagAppendSystemPromptFile, agentio.OptionString},
		"mcp_configs":               {flagMCPConfig, agentio.OptionList},
		"output_format":             {flagOutputFormat, agentio.OptionEnum},
		"include_partial_messages":  {flagIncludePartialMessages, agentio.OptionBool},
		"max_budget_usd":            {flagMaxBudgetUSD, agentio.OptionNumber},
		"setting_sources":           {flagSettingSources, agentio.OptionEnumList},
		"agents":                    {flagAgents, agentio.OptionObject},
		"json_schema":               {flagJSONSchema, agentio.OptionJSON},
	} {
		k, ok := by[key]
		if !ok || k.Flag != want.flag || k.Kind != want.kind {
			t.Errorf("%s = %+v, want %s %s", key, k, want.flag, want.kind)
		}
	}
	if e := by["effort"]; e.Feature != FeatureFlagEffort.String() || !slices.Equal(e.Values, []string{"low", "medium", "high", "xhigh", "max"}) {
		t.Errorf("effort gate = %+v", e)
	}
	if e := by["mcp_configs"]; e.Gate.Since.String() != "0.2.75" {
		t.Errorf("mcp_configs since = %s", e.Gate.Since)
	}
}

// TestParseLaunchOption: key=value lands on the typed field; bad keys and
// values are refused with the choices.
func TestParseLaunchOption(t *testing.T) {
	t.Parallel()
	var o LaunchOptions
	for _, kv := range [][2]string{{"effort", "max"}, {"mcp_configs", "a.json,b.json"}, {"verbose", "true"}, {"max_budget_usd", "2.5"}, {"allowed_tools", `["Bash(ls:*)","Read"]`}} {
		if err := ParseLaunchOption(&o, kv[0], kv[1]); err != nil {
			t.Fatalf("%s: %v", kv[0], err)
		}
	}
	if o.Effort != EffortMax || !slices.Equal(o.MCPConfigs, []string{"a.json", "b.json"}) || !o.Verbose || *o.MaxBudgetUSD != 2.5 || len(o.AllowedTools) != 2 {
		t.Errorf("options = %+v", o)
	}
	args, err := o.Args()
	if err != nil || !slices.Contains(args, flagEffort) {
		t.Errorf("args = %v %v", args, err)
	}
	if err := ParseLaunchOption(&o, "effort", "huge"); !errors.Is(err, agentio.ErrInvalidOption) || !strings.Contains(err.Error(), "xhigh") {
		t.Errorf("bad enum = %v", err)
	}
	if err := ParseLaunchOption(&o, "efort", "max"); err == nil || !strings.Contains(err.Error(), "effort") {
		t.Errorf("unknown key = %v", err)
	}
	if err := ParseLaunchOption(&o, "verbose", "yes please"); !errors.Is(err, agentio.ErrInvalidOption) {
		t.Errorf("bad bool = %v", err)
	}
}

// TestDecodeLaunchOptions: the agx spec 7.3 profile block decodes; unknown
// keys and bad enums fail.
func TestDecodeLaunchOptions(t *testing.T) {
	t.Parallel()
	o, err := DecodeLaunchOptions([]byte(`{"append_system_prompt_file": "~/prompts/work.md", "mcp_configs": ["~/mcp/github.json"], "effort": "high"}`))
	if err != nil || o.Effort != EffortHigh || o.AppendSystemPromptFile != "~/prompts/work.md" || len(o.MCPConfigs) != 1 {
		t.Fatalf("%+v %v", o, err)
	}
	if _, err := DecodeLaunchOptions([]byte(`{"effort": "high", "codex_profile": "deep"}`)); err == nil || !strings.Contains(err.Error(), "valid:") {
		t.Errorf("unknown key = %v", err)
	}
	if _, err := DecodeLaunchOptions([]byte(`{"permission_mode": "yolo"}`)); !errors.Is(err, agentio.ErrInvalidOption) {
		t.Errorf("bad enum = %v", err)
	}
	s, err := LaunchOptionsSchema()
	if err != nil || s.AdditionalProperties == nil || *s.AdditionalProperties || s.Properties["effort"].Description != flagEffort {
		t.Errorf("schema = %+v %v", s.Properties["effort"], err)
	}
}
