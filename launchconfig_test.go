package agentio

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// overlaySample is a vendor-like options struct for TestOverlayOptions.
type overlaySample struct {
	Model  string   `json:"model,omitempty"`
	Effort string   `json:"effort,omitempty"`
	Dirs   []string `json:"dirs,omitempty"`
	Bare   bool     `json:"bare,omitempty"`
	Extra  []string `json:"extra_args,omitempty"`
}

// overlayKeys is the registry of overlaySample, written out.
func overlayKeys() []OptionKey {
	return []OptionKey{
		{Key: "model", Field: "Model", Kind: OptionString, Concern: ConcernModel},
		{Key: "effort", Field: "Effort", Kind: OptionEnum, Values: []string{"low", "high"}},
		{Key: "dirs", Field: "Dirs", Kind: OptionList, Concern: ConcernAddDir},
		{Key: "bare", Field: "Bare", Kind: OptionBool},
		{Key: "extra_args", Field: "Extra", Kind: OptionList},
	}
}

// mapOverlaySample maps the request fields overlaySample has.
func mapOverlaySample(r LaunchRequest) (reflect.Value, error) {
	return reflect.ValueOf(overlaySample{Model: r.Model, Dirs: r.AddDirs, Extra: r.Args}), nil
}

// TestOverlayOptions: only given keys are set; a key the request already
// set names both; unknown keys and wrong kinds are refused.
func TestOverlayOptions(t *testing.T) {
	t.Parallel()
	r := LaunchRequest{Model: "opus", AddDirs: []string{"/a"}}
	v, _ := mapOverlaySample(r)
	o, _ := reflect.TypeAssert[overlaySample](v)
	if err := OverlayOptions(Raw(`{"effort": "high", "bare": true}`), overlayKeys(), reflect.ValueOf(&o), r, mapOverlaySample); err != nil {
		t.Fatal(err)
	}
	if o.Model != "opus" || o.Effort != "high" || !o.Bare || !slices.Equal(o.Dirs, []string{"/a"}) {
		t.Errorf("overlay = %+v", o)
	}
	for _, tc := range []struct{ options, want string }{
		{`{"model": "sonnet"}`, `option "model" conflicts with the request's model`},
		{`{"dirs": ["/b"]}`, `option "dirs" conflicts with the request's add-dir folders`},
		{`{"modle": "x"}`, `unknown key "modle" (valid: bare, dirs, effort, extra_args, model)`},
		{`{"bare": "yes"}`, "bare: expected true or false (bool)"},
		{`{"dirs": "/b"}`, "dirs: expected an array of strings (list)"},
		{`{"effort": "max"}`, `effort="max" is not one of low, high`},
		{`"x"`, "options must be a JSON object"},
	} {
		fresh := overlaySample{Model: "opus", Dirs: []string{"/a"}}
		err := OverlayOptions(Raw(tc.options), overlayKeys(), reflect.ValueOf(&fresh), r, mapOverlaySample)
		if !errors.Is(err, ErrInvalidOption) || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "LaunchRequest") {
			t.Errorf("%s: %v", tc.options, err)
		}
	}
	for _, empty := range []Raw{nil, Raw(""), Raw(" null "), Raw("{}")} {
		if err := OverlayOptions(empty, overlayKeys(), reflect.ValueOf(&o), r, mapOverlaySample); err != nil {
			t.Errorf("empty %q: %v", empty, err)
		}
		if err := CheckOptionsObject(empty, overlayKeys()); err != nil {
			t.Errorf("check empty %q: %v", empty, err)
		}
	}
	if err := CheckOptionsObject(Raw(`{"effort": "max"}`), overlayKeys()); !errors.Is(err, ErrInvalidOption) {
		t.Errorf("check enum: %v", err)
	}
}

// TestOverlayOptions_RawArgsTogether: an options list that lands where the
// request's raw args go is never a conflict: raw args always win, so both
// are kept, the option's first, then the request's raw args (later flags
// win in a vendor CLI's parser).
func TestOverlayOptions_RawArgsTogether(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		r       LaunchRequest
		options string
		want    []string
	}{
		"option and raw args": {LaunchRequest{Args: []string{"--raw", "1"}}, `{"extra_args": ["--opt", "2"]}`, []string{"--opt", "2", "--raw", "1"}},
		"option only":         {LaunchRequest{}, `{"extra_args": ["--opt"]}`, []string{"--opt"}},
		"raw args only":       {LaunchRequest{Args: []string{"--raw"}}, `{"bare": true}`, []string{"--raw"}},
		"empty option list":   {LaunchRequest{Args: []string{"--raw"}}, `{"extra_args": []}`, []string{"--raw"}},
	} {
		v, _ := mapOverlaySample(tc.r)
		o, _ := reflect.TypeAssert[overlaySample](v)
		if err := OverlayOptions(Raw(tc.options), overlayKeys(), reflect.ValueOf(&o), tc.r, mapOverlaySample); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !slices.Equal(o.Extra, tc.want) {
			t.Errorf("%s: extra = %q, want %q", name, o.Extra, tc.want)
		}
	}
}

// TestDeriveOptionKeysConcerns: a concern for a key the struct lacks is
// a derivation error, so a renamed field cannot lose its concern.
func TestDeriveOptionKeysConcerns(t *testing.T) {
	t.Parallel()
	probe := func(v reflect.Value) ([]string, []OptionFeature, error) {
		o, _ := reflect.TypeAssert[overlaySample](v)
		var args []string
		if o.Model != "" {
			args = append(args, "--model", o.Model)
		}
		if o.Effort != "" {
			args = append(args, "--effort", o.Effort)
		}
		for _, d := range o.Dirs {
			args = append(args, "--dir", d)
		}
		if o.Bare {
			args = append(args, "--bare")
		}
		args = append(args, o.Extra...)
		return args, nil, nil
	}
	d := OptionDerivation{Type: reflect.TypeFor[overlaySample](), Probe: probe, Concerns: map[string]Concern{"model": ConcernModel}}
	keys, err := DeriveOptionKeys(d)
	if err != nil {
		t.Fatal(err)
	}
	if k, _ := FindOptionKey(keys, "model"); k.Concern != ConcernModel {
		t.Errorf("model concern = %s", k.Concern)
	}
	if k, _ := FindOptionKey(keys, "bare"); k.Concern != ConcernNone {
		t.Errorf("bare concern = %s", k.Concern)
	}
	d.Concerns["mdoel"] = ConcernModel
	if _, err := DeriveOptionKeys(d); !errors.Is(err, ErrOptionDerivation) || !strings.Contains(err.Error(), "mdoel") {
		t.Errorf("unknown concern key: %v", err)
	}
	js, err := json.Marshal(keys[0])
	if err != nil || !strings.Contains(string(js), `"concern":"model"`) {
		t.Errorf("json = %s %v", js, err)
	}
}

// TestMergeSettings: top-level keys merge; a key in both names why.
func TestMergeSettings(t *testing.T) {
	t.Parallel()
	got, err := MergeSettings(Raw(`{"model":"opus"}`), Raw(`{"apiKeyHelper":"h"}`), "the login")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(got, &m); err != nil || m["model"] != "opus" || m["apiKeyHelper"] != "h" {
		t.Errorf("merged = %s", got)
	}
	if _, err := MergeSettings(Raw(`{"apiKeyHelper":"a"}`), Raw(`{"apiKeyHelper":"h"}`), "the login"); !errors.Is(err, ErrInvalidOption) || !strings.Contains(err.Error(), `"apiKeyHelper" is also set by the login`) {
		t.Errorf("duplicate: %v", err)
	}
	if got, _ := MergeSettings(nil, Raw(`{"a":1}`), ""); string(got) != `{"a":1}` {
		t.Errorf("into empty = %s", got)
	}
	if got, _ := MergeSettings(Raw(`{"a":1}`), nil, ""); string(got) != `{"a":1}` {
		t.Errorf("nothing added = %s", got)
	}
	if _, err := MergeSettings(Raw(`[1]`), Raw(`{"a":1}`), ""); !errors.Is(err, ErrInvalidOption) {
		t.Errorf("non-object base: %v", err)
	}
}

// TestClosestKey suggests near keys only.
func TestClosestKey(t *testing.T) {
	t.Parallel()
	known := []string{"cleanupPeriodDays", "permissions.allow", "model"}
	for in, want := range map[string]string{
		"cleanupPeriodDay": "cleanupPeriodDays",
		"permissions.alow": "permissions.allow",
		"Model":            "model",
		"modle":            "model",
		"theme":            "",
		"model":            "",
	} {
		if got := ClosestKey(in, known); got != want {
			t.Errorf("ClosestKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSchemaPaths lists nested properties dotted.
func TestSchemaPaths(t *testing.T) {
	t.Parallel()
	s := JSONSchemaFor(reflect.TypeFor[schemaSample](), nil, false)
	s.Properties["nested"] = SchemaNode{Type: schemaObject, Properties: map[string]SchemaNode{"inner": {Type: schemaString}}}
	got := SchemaPaths(s)
	if !slices.Contains(got, "nested.inner") || !slices.Contains(got, "nested") || !slices.IsSorted(got) {
		t.Errorf("paths = %v", got)
	}
}

// TestLaunchConfigEnums round-trips the two new closed sets.
func TestLaunchConfigEnums(t *testing.T) {
	t.Parallel()
	for _, d := range SettingsDeliveryValues() {
		var back SettingsDelivery
		b, _ := d.MarshalText()
		if err := back.UnmarshalText(b); err != nil || back != d {
			t.Errorf("delivery %s: %v", d, err)
		}
	}
	if d, ok := ParseSettingsDelivery("inline"); !ok || d != SettingsInline || SettingsDelivery(0) != SettingsFile {
		t.Error("delivery parse / default")
	}
	for _, c := range ConcernValues() {
		got, ok := ParseConcern(c.String())
		if !ok || got != c {
			t.Errorf("concern %s", c)
		}
	}
	var c Concern
	if err := c.UnmarshalText([]byte("budget")); err == nil {
		t.Error("an unknown concern was accepted")
	}
	if CapLaunchConfig.String() != "launch-config" || CapLaunchConfig.Cost() != (Cost{Class: CostDisk, Access: Write}) {
		t.Errorf("capability %s %+v", CapLaunchConfig, CapLaunchConfig.Cost())
	}
	if WarnUnknownKey.String() != "unknown-key" {
		t.Errorf("warning kind %s", WarnUnknownKey)
	}
}

// TestSpecAgentHasNoLaunchConfig: a config-only agent declares why.
func TestSpecAgentHasNoLaunchConfig(t *testing.T) {
	t.Parallel()
	a, err := FromSpec(Spec{ID: "custom", Name: "Custom", Binary: "custom", DefaultHome: ".custom"}, Host{UserHome: "/h"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := LaunchConfigOf(a); ok {
		t.Fatal("a spec agent must not have a launch configurer")
	}
	if reason, ok := Unsupported(a, CapLaunchConfig); !ok || reason != specLaunchConfigReason {
		t.Errorf("reason = %q %v", reason, ok)
	}
}

// pathSample is an options struct with path-tagged fields for
// TestDeriveOptionKeysPaths.
type pathSample struct {
	Model  string   `json:"model,omitempty"`
	File   string   `json:"file,omitempty" agentio:"path"`
	Dirs   []string `json:"dirs,omitempty" agentio:"path"`
	Labels []string `json:"labels,omitempty"`
}

// pathProbe renders pathSample as flags.
func pathProbe(v reflect.Value) ([]string, []OptionFeature, error) {
	o, _ := reflect.TypeAssert[pathSample](v)
	var args []string
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	if o.File != "" {
		args = append(args, "--file", o.File)
	}
	for _, d := range o.Dirs {
		args = append(args, "--dir", d)
	}
	for _, l := range o.Labels {
		args = append(args, "--label", l)
	}
	return args, nil, nil
}

// TestDeriveOptionKeysPaths: a field tagged agentio:"path" is a path
// option (every element of a list), so a launcher knows which values to
// expand ~ in; no other key is; the tag on a value that is not text is a
// derivation error, so it cannot mark a bool or a number by mistake.
func TestDeriveOptionKeysPaths(t *testing.T) {
	t.Parallel()
	keys, err := DeriveOptionKeys(OptionDerivation{Type: reflect.TypeFor[pathSample](), Probe: pathProbe})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, k := range keys {
		if k.Path {
			paths = append(paths, k.Key)
		}
	}
	if !slices.Equal(paths, []string{"file", "dirs"}) {
		t.Errorf("path keys = %v, want [file dirs]", paths)
	}
	type badPath struct {
		On bool `json:"on,omitempty" agentio:"path"`
	}
	probe := func(v reflect.Value) ([]string, []OptionFeature, error) {
		if o, _ := reflect.TypeAssert[badPath](v); o.On {
			return []string{"--on"}, nil, nil
		}
		return nil, nil, nil
	}
	if _, err := DeriveOptionKeys(OptionDerivation{Type: reflect.TypeFor[badPath](), Probe: probe}); !errors.Is(err, ErrOptionDerivation) || !strings.Contains(err.Error(), "On") {
		t.Errorf("path tag on a bool: %v", err)
	}
	type unknownTag struct {
		Name string `json:"name,omitempty" agentio:"paht"`
	}
	nameProbe := func(v reflect.Value) ([]string, []OptionFeature, error) {
		if o, _ := reflect.TypeAssert[unknownTag](v); o.Name != "" {
			return []string{"--name", o.Name}, nil, nil
		}
		return nil, nil, nil
	}
	if _, err := DeriveOptionKeys(OptionDerivation{Type: reflect.TypeFor[unknownTag](), Probe: nameProbe}); !errors.Is(err, ErrOptionDerivation) || !strings.Contains(err.Error(), "paht") {
		t.Errorf("unknown agentio tag: %v", err)
	}
}

// TestNewLaunchSentinel: a vendor's launch-refusal sentinel keeps its own
// identity and message, and every error wrapping it also matches
// ErrInvalidOption, so a caller classifies a refused launch the same way
// for every vendor.
func TestNewLaunchSentinel(t *testing.T) {
	t.Parallel()
	a, b := NewLaunchSentinel("vendor: invalid launch options"), NewLaunchSentinel("vendor: invalid launch options")
	err := fmt.Errorf("%w: --resume and --continue are exclusive", a)
	if !errors.Is(err, a) || !errors.Is(err, ErrInvalidOption) || errors.Is(err, b) {
		t.Errorf("Is: own %v, invalid option %v, other %v", errors.Is(err, a), errors.Is(err, ErrInvalidOption), errors.Is(err, b))
	}
	if err.Error() != "vendor: invalid launch options: --resume and --continue are exclusive" {
		t.Errorf("message = %q", err.Error())
	}
	if errors.Is(ErrInvalidOption, a) {
		t.Error("ErrInvalidOption must not match a vendor sentinel")
	}
}
