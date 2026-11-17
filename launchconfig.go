package agentio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// Launch configuration: the typed vendor options and the per-launch
// settings a launcher (agx) passes on top of a neutral LaunchRequest.
//
// Why it is a capability of the neutral Agent and not a vendor call: a
// launcher must stay agent-neutral (no "if claude" code), yet it has to
// pass a profile's typed block ({"effort": "high"}) and a merged settings
// map. The rich vendor builders take those, but calling them would make
// the launcher redo the request mapping (autonomy, headless, credential),
// which is vendor knowledge. LaunchConfigured keeps that mapping in the
// vendor module and applies the config over it.

// LaunchConfig is what a launch adds to a LaunchRequest. The zero value
// adds nothing: LaunchConfigured(t, r, LaunchConfig{}) builds exactly what
// Launch(t, r) builds.
type LaunchConfig struct {
	// Options is a JSON object of the agent's option keys
	// (LaunchConfigurer.OptionKeys), such as {"effort": "high"}; nil or
	// empty = none.
	Options Raw
	// Settings is a nested JSON object of the agent's settings keys
	// (Claude: settings.json keys; Codex: config.toml paths as nested
	// objects); nil or empty = none.
	Settings Raw
	// Delivery is how Settings reach an agent that takes a settings
	// object (SettingsFile, the default, needs RunDir and RunID).
	Delivery SettingsDelivery
	// RunDir is the caller's per-run folder the settings file is written
	// into; RunID names the file (settings-<RunID>.json).
	RunDir, RunID string
}

// SettingsIssue is one finding of CheckSettings.
type SettingsIssue struct {
	// Key is the dotted key path ("permissions.defaultMode").
	Key string `json:"key"`
	// Detail names what was expected (a type, a set of values), never the
	// value given.
	Detail string `json:"detail,omitempty"`
	// Suggestion is the closest known key ("did you mean"); "" when none
	// is close.
	Suggestion string `json:"suggestion,omitempty"`
}

// SettingsCheck is what CheckSettings found in a settings object.
type SettingsCheck struct {
	// TypeErrors are modelled keys with a value of the wrong type; a
	// launch with any is refused.
	TypeErrors []SettingsIssue `json:"type_errors,omitempty"`
	// Unknown are keys (or values) the library does not model; they are
	// passed on and become warnings, because the agent may know them.
	Unknown []SettingsIssue `json:"unknown,omitempty"`
	// Concerns maps each given key path that overlaps a neutral concern
	// to it (Claude "model" → ConcernModel, "apiKeyHelper" →
	// ConcernCredential), so a launcher can apply one winner per concern.
	Concerns map[string]Concern `json:"concerns,omitempty"`
}

// OK reports a settings object without type errors. Pure.
func (c SettingsCheck) OK() bool { return len(c.TypeErrors) == 0 }

// LaunchConfigurer builds a launch with typed vendor options and settings
// through the neutral Agent (an agent without either has a typed
// unsupported reason instead).
type LaunchConfigurer interface {
	// OptionKeys lists the agent's typed launch options, each with the
	// neutral Concern it overlaps. Pure.
	OptionKeys() ([]OptionKey, error)
	// OptionsSchema is the closed JSON Schema of an options object. Pure.
	OptionsSchema() (SchemaNode, error)
	// CheckOptions validates an options object without building: an
	// unknown key is an error listing the valid keys, a wrong value type
	// an error naming the key and the expected kind. Pure.
	CheckOptions(options Raw) error
	// CheckSettings type-checks a settings object: type errors, unknown
	// keys (with a suggestion) and the keys that overlap a neutral
	// concern. A body that is not a JSON object is an error. Pure.
	CheckSettings(settings Raw) (SettingsCheck, error)
	// RawSettingsArg reports whether raw args (LaunchRequest.Args)
	// already deliver settings (Claude: --settings), so the launcher skips
	// its own; the agent would use only one of them. Pure.
	RawSettingsArg(args []string) bool
	// LaunchConfigured builds what Launch(t, r) builds (autonomy rules,
	// credential env and scrub, home rule, dir, version warnings) with c's
	// options applied over what r maps to and c's settings delivered. An
	// option that sets a field r already sets is ErrInvalidOption naming
	// both; every emitted flag is version-gated; settings with a type
	// error are refused, unknown ones become Command.Warnings. r.Args stay
	// last and verbatim. Pure, except Disk · Write when c.Delivery is
	// SettingsFile and settings are delivered (the file in c.RunDir).
	LaunchConfigured(t Target, r LaunchRequest, c LaunchConfig) (Command, error)
}

// IsEmptyJSON reports a Raw that carries nothing: nil, blank, null or {}.
// Pure.
func IsEmptyJSON(r Raw) bool {
	t := bytes.TrimSpace(r)
	if len(t) == 0 || string(t) == jsonNull {
		return true
	}
	var m map[string]json.RawMessage
	return json.Unmarshal(t, &m) == nil && len(m) == 0
}

// RequestMapper maps a LaunchRequest onto a vendor's options struct (a
// reflect.Value of the struct, not a pointer); OverlayOptions calls it
// with one request field set at a time to name a conflict's source.
type RequestMapper func(r LaunchRequest) (reflect.Value, error)

// requestPart is one set field of a LaunchRequest, alone.
type requestPart struct {
	name string
	req  LaunchRequest
}

// requestParts splits r into its set mappable fields, one request each.
// Dir and Credential are left out: they map to the Command, never to an
// option field. Pure.
func requestParts(r LaunchRequest) []requestPart {
	var out []requestPart
	add := func(name string, set bool, one LaunchRequest) {
		if set {
			out = append(out, requestPart{name: name, req: one})
		}
	}
	add("Prompt", r.Prompt != "", LaunchRequest{Prompt: r.Prompt})
	add("ResumeID", r.ResumeID != "", LaunchRequest{ResumeID: r.ResumeID})
	add("Model", r.Model != "", LaunchRequest{Model: r.Model})
	add("AddDirs", len(r.AddDirs) > 0, LaunchRequest{AddDirs: r.AddDirs})
	add("Autonomy", r.Autonomy != AutonomyDefault, LaunchRequest{Autonomy: r.Autonomy})
	add("Headless", r.Headless, LaunchRequest{Headless: true})
	add("Args", len(r.Args) > 0, LaunchRequest{Args: r.Args})
	return out
}

// requestFieldPrefix names a LaunchRequest field in conflict errors.
const requestFieldPrefix = "LaunchRequest."

// OverlayOptions applies an options object over dst, a pointer to the
// vendor's options struct already mapped from r: only the keys given are
// set. A key whose field r already set is ErrInvalidOption naming the key
// and the request field (found by mapping each set field of r alone).
// Unknown keys and wrong value kinds are refused as in DecodeOptions.
// Empty options do nothing. Pure.
func OverlayOptions(options Raw, keys []OptionKey, dst reflect.Value, r LaunchRequest, mapOne RequestMapper) error {
	if IsEmptyJSON(options) {
		return nil
	}
	given, err := optionObject(options, keys)
	if err != nil {
		return err
	}
	for _, name := range given {
		k, _ := FindOptionKey(keys, name)
		f := dst.Elem().FieldByName(k.Field)
		if !f.IsValid() || f.IsZero() {
			continue
		}
		return fmt.Errorf("%w: option %q conflicts with %s%s", ErrInvalidOption, k.Key, requestFieldPrefix, requestSource(r, k.Field, mapOne))
	}
	return DecodeOptions(options, keys, dst)
}

// requestSource names the request field whose mapping sets field. Pure.
func requestSource(r LaunchRequest, field string, mapOne RequestMapper) string {
	for _, p := range requestParts(r) {
		v, err := mapOne(p.req)
		if err != nil {
			continue
		}
		if f := v.FieldByName(field); f.IsValid() && !f.IsZero() {
			return p.name
		}
	}
	return "?"
}

// optionObject checks an options object (every key known, every value of
// its kind) and returns its keys, sorted. Pure.
func optionObject(data Raw, keys []OptionKey) ([]string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
		return nil, fmt.Errorf("%w: options must be a JSON object", ErrInvalidOption)
	}
	names := make([]string, 0, len(raw))
	for name, v := range raw {
		k, err := FindOptionKey(keys, name)
		if err != nil {
			return nil, err
		}
		if err := checkKindJSON(k, v); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names, nil
}

// CheckOptionsObject validates an options object against keys without
// decoding it: unknown keys list the valid ones, a value of the wrong
// kind names the key and the expected kind, an enum value outside its
// set lists the choices. Empty options are valid. Pure.
func CheckOptionsObject(options Raw, keys []OptionKey) error {
	if IsEmptyJSON(options) {
		return nil
	}
	given, err := optionObject(options, keys)
	if err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(options, &raw); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidOption, err)
	}
	for _, name := range given {
		k, _ := FindOptionKey(keys, name)
		if err := checkEnumJSON(k, raw[name]); err != nil {
			return err
		}
	}
	return nil
}

// kindExpectation is the JSON shape each option kind takes, for errors.
func kindExpectation(k OptionKind) string {
	switch k {
	case OptionString, OptionEnum:
		return "a string"
	case OptionBool:
		return "true or false"
	case OptionNumber:
		return "a number"
	case OptionList, OptionEnumList:
		return "an array of strings"
	case OptionObject:
		return "an object"
	}
	return "JSON"
}

// checkKindJSON checks a value's JSON type against k.Kind (null is
// allowed: it leaves the field zero). The error names the key and the
// expected kind, never the value. Pure.
func checkKindJSON(k OptionKey, v json.RawMessage) error {
	t := bytes.TrimSpace(v)
	if len(t) == 0 || string(t) == jsonNull || k.Kind == OptionJSON {
		return nil
	}
	ok := false
	switch k.Kind {
	case OptionString, OptionEnum:
		ok = t[0] == '"'
	case OptionBool:
		ok = string(t) == "true" || string(t) == "false"
	case OptionNumber:
		ok = t[0] == '-' || (t[0] >= '0' && t[0] <= '9')
	case OptionList, OptionEnumList:
		var items []string
		ok = json.Unmarshal(t, &items) == nil
	case OptionObject:
		ok = t[0] == '{'
	}
	if !ok {
		return fmt.Errorf("%w: %s: expected %s (%s)", ErrInvalidOption, k.Key, kindExpectation(k.Kind), k.Kind)
	}
	return nil
}

// MergeSettings merges add's top-level keys into base (two JSON objects);
// a key present in both is ErrInvalidOption naming it and why (the two
// sources compete for one concern). Either may be empty. Pure.
func MergeSettings(base, add Raw, why string) (Raw, error) {
	if IsEmptyJSON(add) {
		return base, nil
	}
	if IsEmptyJSON(base) {
		return add, nil
	}
	var b, a map[string]json.RawMessage
	if err := json.Unmarshal(base, &b); err != nil || b == nil {
		return nil, fmt.Errorf("%w: settings must be a JSON object", ErrInvalidOption)
	}
	if err := json.Unmarshal(add, &a); err != nil || a == nil {
		return nil, fmt.Errorf("%w: settings must be a JSON object", ErrInvalidOption)
	}
	for k, v := range a {
		if _, dup := b[k]; dup {
			return nil, fmt.Errorf("%w: settings key %q is also set by %s", ErrInvalidOption, k, why)
		}
		b[k] = v
	}
	out, err := json.Marshal(b)
	return out, err
}

// maxSuggestDistance bounds "did you mean": a key further than this many
// edits (or a third of its length, whichever is larger) is not suggested.
const maxSuggestDistance = 2

// ClosestKey returns the known key nearest to key by edit distance, for a
// "did you mean" on an unknown key; "" when none is close. Ties go to the
// first in known. Comparison ignores case. Pure.
func ClosestKey(key string, known []string) string {
	limit := max(maxSuggestDistance, len(key)/3)
	best, bestD := "", limit+1
	lk := strings.ToLower(key)
	for _, k := range known {
		if k == key {
			continue
		}
		if d := editDistance(lk, strings.ToLower(k)); d < bestD {
			best, bestD = k, d
		}
	}
	return best
}

// editDistance is the Levenshtein distance between a and b (bytes). Pure.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// SchemaPaths lists the dotted key paths of every property in n, nested
// objects included ("permissions", "permissions.allow"), sorted. Pure.
func SchemaPaths(n SchemaNode) []string {
	var out []string
	var walk func(prefix string, n SchemaNode)
	walk = func(prefix string, n SchemaNode) {
		for k, p := range n.Properties {
			path := prefix + k
			out = append(out, path)
			walk(path+".", p)
		}
	}
	walk("", n)
	slices.Sort(out)
	return out
}
