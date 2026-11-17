package agentio

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// Launch option registry: each typed launch option of a vendor (claude.
// LaunchOptions, codex.LaunchOptions) as data, so a launcher can validate
// `--option key=value` and a profile's typed block, list the valid keys,
// and emit a JSON Schema fragment.
//
// Why it is derived, not written down: the vendor's argv builder is the
// one place that knows which flag a field emits and which feature gate it
// checks. DeriveOptionKeys sets each field to a sample value, runs that
// builder, and records what changed, so the registry cannot drift from
// the flags that are really built. The keys and value types come from the
// struct's json tags and Go types.

// OptionKind is the value type of a launch option.
type OptionKind string

// OptionKind values.
const (
	// OptionString is free text (a model, a path, a name).
	OptionString OptionKind = "string"
	// OptionEnum is one of OptionKey.Values.
	OptionEnum OptionKind = "enum"
	// OptionBool is true or false; true emits the flag.
	OptionBool OptionKind = "bool"
	// OptionNumber is a decimal number.
	OptionNumber OptionKind = "number"
	// OptionList is a list of strings (repeated flag or comma-joined).
	OptionList OptionKind = "list"
	// OptionEnumList is a list of OptionKey.Values.
	OptionEnumList OptionKind = "enum-list"
	// OptionJSON is a JSON value passed through (a JSON Schema).
	OptionJSON OptionKind = "json"
	// OptionObject is a JSON object with a vendor shape (subagents).
	OptionObject OptionKind = "object"
)

// OptionKindValues is the canonical list of kinds. Pure.
func OptionKindValues() []OptionKind {
	return []OptionKind{OptionString, OptionEnum, OptionBool, OptionNumber, OptionList, OptionEnumList, OptionJSON, OptionObject}
}

// OptionKey is one launch option.
type OptionKey struct {
	// Key is the stable snake_case config key (the field's json tag).
	Key string `json:"key"`
	// Field is the Go field name in the vendor's LaunchOptions.
	Field string `json:"field"`
	// Kind is the value type; Values the allowed values of an enum kind.
	Kind   OptionKind `json:"kind"`
	Values []string   `json:"values,omitempty"`
	// Flag is the vendor flag the option emits ("--effort"); "" for a
	// positional value (the prompt) or verbatim args.
	Flag string `json:"flag,omitempty"`
	// Feature is the feature gate the flag checks (a features.tsv name)
	// and Gate its span: the versions the flag exists in. An unknown span
	// (no recorded Since or Until) means "assumed available".
	Feature string      `json:"feature,omitempty"`
	Gate    FeatureSpan `json:"gate,omitzero"`
	// Concern is the neutral LaunchRequest concern the option overlaps
	// (model, prompt, permission level…); ConcernNone for the rest. A
	// launcher uses it for one winner per concern.
	Concern Concern `json:"concern,omitempty"`
}

// OptionFeature is a feature gate a probe saw, by name with its span.
type OptionFeature struct {
	Name string
	Span FeatureSpan
}

// OptionProbe builds argv for v (a value of the vendor's options struct)
// and returns the args and the feature gates its flags checked; an error
// means the combination is invalid (the next base is tried).
type OptionProbe func(v reflect.Value) (args []string, features []OptionFeature, err error)

// OptionDerivation is what DeriveOptionKeys needs from a vendor.
type OptionDerivation struct {
	// Type is the options struct type.
	Type reflect.Type
	// Probe runs the vendor's builder.
	Probe OptionProbe
	// Enums gives the allowed values of the vendor's named string types.
	Enums SchemaEnums
	// Bases are option values tried in order as the starting point for a
	// field (zero first): a field that needs another one set (an output
	// format needs print mode) is probed on the first base that builds.
	Bases []reflect.Value
	// Samples are sample values for field types the generic rules cannot
	// make (a map of vendor structs), keyed by type.
	Samples map[reflect.Type]reflect.Value
	// Skip are Go field names that are not launch options (a config
	// handle used for a check, a field set by the library itself).
	Skip []string
	// Concerns names the neutral concern of the keys that overlap one
	// (by key); a key not in the struct is a derivation error, so a
	// renamed field cannot silently lose its concern.
	Concerns map[string]Concern
}

// ErrOptionDerivation means a field the registry could not describe.
var ErrOptionDerivation = errors.New("agentio: launch option registry")

// DeriveOptionKeys builds the registry for d.Type in field order. Each
// exported field needs a json tag (its key). A field whose sample changes
// nothing in the built argv is an error, so a new field without builder
// support fails the vendor's test rather than vanishing. Pure (runs only
// the vendor's pure builder).
func DeriveOptionKeys(d OptionDerivation) ([]OptionKey, error) {
	bases := d.Bases
	if len(bases) == 0 || !bases[0].IsZero() {
		bases = append([]reflect.Value{reflect.New(d.Type).Elem()}, bases...)
	}
	var out []OptionKey
	var errs []error
	for i := 0; i < d.Type.NumField(); i++ {
		f := d.Type.Field(i)
		if !f.IsExported() || slices.Contains(d.Skip, f.Name) {
			continue
		}
		key, ok := jsonFieldName(f)
		if !ok || key == f.Name {
			errs = append(errs, fmt.Errorf("%s: no json key", f.Name))
			continue
		}
		k, err := deriveOne(d, bases, f, key)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, k)
	}
	for key := range d.Concerns {
		if !slices.ContainsFunc(out, func(k OptionKey) bool { return k.Key == key }) {
			errs = append(errs, fmt.Errorf("concern for unknown key %s", key))
		}
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("%w: %w", ErrOptionDerivation, errors.Join(errs...))
	}
	return out, nil
}

// deriveOne probes one field on the first base that builds with it.
func deriveOne(d OptionDerivation, bases []reflect.Value, f reflect.StructField, key string) (OptionKey, error) {
	kind, values := optionKind(f.Type, d.Enums)
	k := OptionKey{Key: key, Field: f.Name, Kind: kind, Values: values, Concern: d.Concerns[key]}
	sample, ok := d.Samples[f.Type]
	if !ok {
		if sample, ok = sampleValue(f.Type, values); !ok {
			return k, fmt.Errorf("%s: no sample for %s", f.Name, f.Type)
		}
	}
	for _, base := range bases {
		if !base.Field(f.Index[0]).IsZero() {
			continue // the base sets this field itself
		}
		baseArgs, baseFeats, err := d.Probe(base)
		if err != nil {
			continue
		}
		v := reflect.New(d.Type).Elem()
		v.Set(base)
		v.Field(f.Index[0]).Set(sample)
		args, feats, err := d.Probe(v)
		if err != nil {
			continue
		}
		added := insertedBlock(baseArgs, args)
		if len(added) == 0 {
			return k, fmt.Errorf("%s: its sample emits nothing", f.Name)
		}
		if strings.HasPrefix(added[0], "-") && added[0] != argsSeparator {
			k.Flag = added[0]
		}
		for _, ft := range feats {
			if !slices.ContainsFunc(baseFeats, func(b OptionFeature) bool { return b.Name == ft.Name }) {
				k.Feature, k.Gate = ft.Name, ft.Span
				break
			}
		}
		return k, nil
	}
	return k, fmt.Errorf("%s: no base builds with it set", f.Name)
}

// argsSeparator is the "--" that ends flags before a positional prompt.
const argsSeparator = "--"

// insertedBlock is what b adds to a: the args between their common
// prefix and common suffix. Pure.
func insertedBlock(a, b []string) []string {
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	return b[p : len(b)-s]
}

// optionKind maps a field type to its kind and enum values. Pure.
func optionKind(t reflect.Type, enums SchemaEnums) (OptionKind, []string) {
	if t == reflect.TypeFor[Raw]() {
		return OptionJSON, nil
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	enumOf := func(t reflect.Type) []string {
		if enums == nil || t.Kind() != reflect.String {
			return nil
		}
		return enums(t)
	}
	switch t.Kind() {
	case reflect.String:
		if v := enumOf(t); v != nil {
			return OptionEnum, v
		}
		return OptionString, nil
	case reflect.Bool:
		return OptionBool, nil
	case reflect.Int, reflect.Int64, reflect.Float32, reflect.Float64:
		return OptionNumber, nil
	case reflect.Slice:
		if t.Elem().Kind() == reflect.String {
			if v := enumOf(t.Elem()); v != nil {
				return OptionEnumList, v
			}
			return OptionList, nil
		}
		return OptionObject, nil
	}
	return OptionObject, nil
}

// sampleValue is a non-zero value of t for probing: the first enum value
// for an enum, "x" for text, true, 1, a one-element list, a pointer to a
// sample, or {} for JSON. false when the type needs a vendor sample.
func sampleValue(t reflect.Type, values []string) (reflect.Value, bool) {
	if t == reflect.TypeFor[Raw]() {
		return reflect.ValueOf(Raw("{}")), true
	}
	v := reflect.New(t).Elem()
	switch t.Kind() {
	case reflect.String:
		s := "x"
		if len(values) > 0 {
			s = values[0]
		}
		v.SetString(s)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int64:
		v.SetInt(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	case reflect.Slice:
		e, ok := sampleValue(t.Elem(), values)
		if !ok {
			return v, false
		}
		v = reflect.Append(v, e)
	case reflect.Pointer:
		e, ok := sampleValue(t.Elem(), values)
		if !ok {
			return v, false
		}
		p := reflect.New(t.Elem())
		p.Elem().Set(e)
		v = p
	default:
		return v, false
	}
	return v, true
}

// OptionValueJSON turns a command-line value for k into the JSON value its
// field decodes from: text is quoted, a bool must be true or false, a
// number must parse, a list is a JSON array or comma-separated text, JSON
// and object kinds must already be JSON. Enum values are checked against
// k.Values. Pure.
func OptionValueJSON(k OptionKey, value string) (Raw, error) {
	bad := func(why string) (Raw, error) {
		return nil, fmt.Errorf("%w: %s=%q: %s", ErrInvalidOption, k.Key, value, why)
	}
	switch k.Kind {
	case OptionString:
		return json.Marshal(value)
	case OptionEnum:
		if !slices.Contains(k.Values, value) {
			return bad("one of " + strings.Join(k.Values, ", "))
		}
		return json.Marshal(value)
	case OptionBool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return bad("true or false")
		}
		return json.Marshal(b)
	case OptionNumber:
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return bad("a number")
		}
		return json.Marshal(n)
	case OptionList, OptionEnumList:
		var items []string
		if strings.HasPrefix(strings.TrimSpace(value), "[") {
			if err := json.Unmarshal([]byte(value), &items); err != nil {
				return bad("a JSON array of strings")
			}
		} else {
			for _, it := range strings.Split(value, ",") {
				if it = strings.TrimSpace(it); it != "" {
					items = append(items, it)
				}
			}
		}
		if k.Kind == OptionEnumList {
			for _, it := range items {
				if !slices.Contains(k.Values, it) {
					return bad(it + " is not one of " + strings.Join(k.Values, ", "))
				}
			}
		}
		return json.Marshal(items)
	}
	if !json.Valid([]byte(value)) {
		return bad("JSON")
	}
	return Raw(value), nil
}

// ErrInvalidOption means a launch option key or value the registry
// refuses.
var ErrInvalidOption = errors.New("agentio: invalid launch option")

// FindOptionKey returns the key named key; the error lists the valid keys.
// Pure.
func FindOptionKey(keys []OptionKey, key string) (OptionKey, error) {
	names := make([]string, 0, len(keys))
	for _, k := range keys {
		if k.Key == key {
			return k, nil
		}
		names = append(names, k.Key)
	}
	slices.Sort(names)
	return OptionKey{}, fmt.Errorf("%w: unknown key %q (valid: %s)", ErrInvalidOption, key, strings.Join(names, ", "))
}

// DecodeOptions strictly decodes a typed options block (a profile's
// "claude": {...}) into dst, a pointer to the vendor's options struct: an
// unknown key fails with the valid keys listed, a value of the wrong
// kind names the key and the expected kind, and enum values are checked
// against the registry. Pure.
func DecodeOptions(data []byte, keys []OptionKey, dst reflect.Value) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidOption, err)
	}
	for name, v := range raw {
		k, err := FindOptionKey(keys, name)
		if err != nil {
			return err
		}
		if err := checkKindJSON(k, v); err != nil {
			return err
		}
		if err := checkEnumJSON(k, v); err != nil {
			return err
		}
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst.Interface()); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidOption, err)
	}
	return nil
}

// SetOption applies key=value (a command-line --option) to dst, a pointer
// to the vendor's options struct, through the registry. Pure.
func SetOption(keys []OptionKey, dst reflect.Value, key, value string) error {
	k, err := FindOptionKey(keys, key)
	if err != nil {
		return err
	}
	js, err := OptionValueJSON(k, value)
	if err != nil {
		return err
	}
	f := dst.Elem().FieldByName(k.Field)
	if !f.IsValid() {
		return fmt.Errorf("%w: %s has no field %s", ErrInvalidOption, key, k.Field)
	}
	fresh := reflect.New(f.Type())
	if err := json.Unmarshal(js, fresh.Interface()); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrInvalidOption, key, err)
	}
	f.Set(fresh.Elem())
	return nil
}

// checkEnumJSON checks an enum or enum-list value against k.Values. Pure.
func checkEnumJSON(k OptionKey, v json.RawMessage) error {
	var items []string
	switch k.Kind {
	case OptionEnum:
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			return fmt.Errorf("%w: %s: %w", ErrInvalidOption, k.Key, err)
		}
		items = []string{s}
	case OptionEnumList:
		if err := json.Unmarshal(v, &items); err != nil {
			return fmt.Errorf("%w: %s: %w", ErrInvalidOption, k.Key, err)
		}
	default:
		return nil
	}
	for _, it := range items {
		if !slices.Contains(k.Values, it) {
			return fmt.Errorf("%w: %s=%q is not one of %s", ErrInvalidOption, k.Key, it, strings.Join(k.Values, ", "))
		}
	}
	return nil
}

// OptionsSchema is the closed JSON Schema of a typed options block,
// built from the registry (types, enums, and the flag as description).
// Pure.
func OptionsSchema(keys []OptionKey) SchemaNode {
	f := false
	n := SchemaNode{Type: schemaObject, Properties: map[string]SchemaNode{}, AdditionalProperties: &f}
	for _, k := range keys {
		var p SchemaNode
		switch k.Kind {
		case OptionString:
			p = SchemaNode{Type: schemaString}
		case OptionEnum:
			p = SchemaNode{Type: schemaString, Enum: k.Values}
		case OptionBool:
			p = SchemaNode{Type: schemaBoolean}
		case OptionNumber:
			p = SchemaNode{Type: schemaNumber}
		case OptionList:
			p = SchemaNode{Type: schemaArray, Items: &SchemaNode{Type: schemaString}}
		case OptionEnumList:
			p = SchemaNode{Type: schemaArray, Items: &SchemaNode{Type: schemaString, Enum: k.Values}}
		case OptionObject:
			p = SchemaNode{Type: schemaObject}
		}
		p.Description = k.Flag
		n.Properties[k.Key] = p
	}
	return n
}
