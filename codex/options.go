package codex

import (
	"reflect"

	"github.com/ubgo/agentio"
)

// Launch option registry (agentio.DeriveOptionKeys): every LaunchOptions
// field as a stable snake_case key ("codex_profile", "search") with its
// value type, allowed values, the flag it emits and its feature gate,
// derived from the real builder. Config overrides (-c) are not options:
// they go through Override, checked with CheckOverride and ConfigKeys.

// optionSkip are fields that are not launch options: overrides have their
// own table, ProfileCheck is a config handle for a check, and
// PromptFromStdin only applies to `codex exec`.
func optionSkip() []string { return []string{"Overrides", "ProfileCheck", "PromptFromStdin"} }

// optionEnums are the values the CLI accepts for each enum flag.
func optionEnums(t reflect.Type) []string {
	switch t {
	case reflect.TypeFor[SandboxMode]():
		return valuesOf(ModeReadOnly, ModeWorkspaceWrite, ModeDangerFullAccess)
	case reflect.TypeFor[ApprovalPolicy]():
		return valuesOf(ApprovalOnRequest, ApprovalNever, ApprovalUntrusted, ApprovalOnFailure)
	}
	return nil
}

// probeLaunchOptions renders the TUI flags for v on a target that allows
// dangerous modes (so bypass_all is probed, not refused). cost: Pure.
func probeLaunchOptions(v reflect.Value) ([]string, []agentio.OptionFeature, error) {
	o, _ := reflect.TypeAssert[LaunchOptions](v)
	b := newBuilder(agentio.Target{AllowDangerous: true})
	b.global(o, false)
	if b.err != nil {
		return nil, nil, b.err
	}
	args := b.args
	if o.Prompt != "" {
		args = append(args, o.Prompt)
	}
	feats := make([]agentio.OptionFeature, len(b.feats))
	for i, f := range b.feats {
		feats[i] = agentio.OptionFeature{Name: f.String(), Span: FeatureInfo(f)}
	}
	return args, feats, nil
}

// LaunchOptionKeys lists every launch option in field order, derived from
// the builder. cost: Pure.
func LaunchOptionKeys() ([]agentio.OptionKey, error) {
	return agentio.DeriveOptionKeys(agentio.OptionDerivation{
		Type:  reflect.TypeFor[LaunchOptions](),
		Probe: probeLaunchOptions,
		Enums: optionEnums,
		Skip:  optionSkip(),
	})
}

// ParseLaunchOption applies one `key=value` to o through the registry.
// cost: Pure.
func ParseLaunchOption(o *LaunchOptions, key, value string) error {
	keys, err := LaunchOptionKeys()
	if err != nil {
		return err
	}
	return agentio.SetOption(keys, reflect.ValueOf(o), key, value)
}

// DecodeLaunchOptions strictly decodes a typed block ({"codex_profile":
// "deep", "search": true}); unknown keys and bad enums fail. cost: Pure.
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
