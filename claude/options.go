package claude

import (
	"reflect"

	"github.com/ubgo/agentio"
)

// Launch option registry (agentio.DeriveOptionKeys): every LaunchOptions
// field as a stable snake_case key with its value type, allowed values,
// the flag it emits and its feature gate from features.tsv. A launcher
// validates `--option effort=max` and a profile's "claude": {...} block
// with it and builds a JSON Schema fragment from it.

// optionDerivation is how the registry probes the real builder: print mode
// as a base for the flags that need -p, stream-json for the ones that need
// that too, and a sample subagent for the agents map.
func optionDerivation() agentio.OptionDerivation {
	t := reflect.TypeFor[LaunchOptions]()
	return agentio.OptionDerivation{
		Type:  t,
		Probe: probeLaunchOptions,
		Enums: schemaEnums,
		Bases: []reflect.Value{
			reflect.ValueOf(LaunchOptions{Print: true}),
			reflect.ValueOf(LaunchOptions{Print: true, OutputFormat: OutputStreamJSON}),
		},
		Samples: map[reflect.Type]reflect.Value{
			reflect.TypeFor[map[string]SubagentDef](): reflect.ValueOf(map[string]SubagentDef{"x": {Description: "d", Prompt: "p"}}),
		},
	}
}

// probeLaunchOptions runs build on v and names the features it checked.
// cost: Pure.
func probeLaunchOptions(v reflect.Value) ([]string, []agentio.OptionFeature, error) {
	o, _ := reflect.TypeAssert[LaunchOptions](v)
	b, err := o.build()
	if err != nil {
		return nil, nil, err
	}
	feats := make([]agentio.OptionFeature, len(b.feats))
	for i, f := range b.feats {
		feats[i] = agentio.OptionFeature{Name: f.String(), Span: FeatureInfo(f)}
	}
	return b.args, feats, nil
}

// LaunchOptionKeys lists every launch option in field order, derived from
// the argv builder so it cannot drift from the flags really built. The
// error is non-nil only if a LaunchOptions field lacks builder support
// (a test keeps it nil). cost: Pure.
func LaunchOptionKeys() ([]agentio.OptionKey, error) {
	return agentio.DeriveOptionKeys(optionDerivation())
}

// ParseLaunchOption applies one `key=value` to o through the registry:
// an unknown key lists the valid ones, an enum value is checked, a list
// takes a JSON array or comma-separated text. cost: Pure.
func ParseLaunchOption(o *LaunchOptions, key, value string) error {
	keys, err := LaunchOptionKeys()
	if err != nil {
		return err
	}
	return agentio.SetOption(keys, reflect.ValueOf(o), key, value)
}

// DecodeLaunchOptions strictly decodes a typed block ({"effort": "high",
// "mcp_configs": ["~/mcp/github.json"]}): unknown keys and bad enum values
// fail with the valid choices listed. cost: Pure.
func DecodeLaunchOptions(data []byte) (LaunchOptions, error) {
	keys, err := LaunchOptionKeys()
	if err != nil {
		return LaunchOptions{}, err
	}
	var o LaunchOptions
	err = agentio.DecodeOptions(data, keys, reflect.ValueOf(&o))
	return o, err
}

// LaunchOptionsSchema is the closed JSON Schema of a typed block.
// cost: Pure.
func LaunchOptionsSchema() (agentio.SchemaNode, error) {
	keys, err := LaunchOptionKeys()
	return agentio.OptionsSchema(keys), err
}
