package agentio

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
)

// Scalar is the set of types Opt may hold. Why a union instead of an
// unconstrained parameter: the no-`any` rule (SPEC §5.5) also bans `any` as
// a constraint, and presence tracking is only needed for scalars (structs
// are pointers, lists and maps are nil when absent).
type Scalar interface {
	~bool | ~string | ~int | ~int32 | ~int64 | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~float64 | time.Time
}

// Opt is a presence-tracked scalar: a vendor key that a version could drop.
// Absent means "not set", never the zero value. JSON: an absent key leaves
// Set false; null also reads as not set; marshals as null when unset (use
// `json:",omitzero"` to drop it).
type Opt[T Scalar] struct {
	V   T
	Set bool
}

// Some returns a set Opt. Pure.
func Some[T Scalar](v T) Opt[T] { return Opt[T]{V: v, Set: true} }

// Get returns the value and whether it was set. Pure.
func (o Opt[T]) Get() (T, bool) { return o.V, o.Set }

// Or returns the value, or def when unset. Pure.
func (o Opt[T]) Or(def T) T {
	if o.Set {
		return o.V
	}
	return def
}

// IsZero reports unset, so `omitzero` drops an unset Opt. Pure.
func (o Opt[T]) IsZero() bool { return !o.Set }

// MarshalJSON writes the value, or null when unset. Pure.
func (o Opt[T]) MarshalJSON() ([]byte, error) {
	if !o.Set {
		return []byte(jsonNull), nil
	}
	return json.Marshal(o.V)
}

// UnmarshalJSON reads the value; null reads as unset. A value of the wrong
// type is an error the vendor reader turns into a WarnTypeChanged warning
// for that field only (invariant 17). Pure.
func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	if string(b) == jsonNull {
		*o = Opt[T]{}
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*o = Opt[T]{V: v, Set: true}
	return nil
}

// Raw is a vendor payload not yet typed (or kept beside a typed one), so L3
// decoders and drift tools can still reach it.
type Raw = json.RawMessage

// Unknown holds the keys of a vendor JSON object that the struct does not
// model (invariant 13: kept, never dropped). Values stay raw.
type Unknown map[string]json.RawMessage

// Keys returns the unknown key names, sorted. Pure.
func (u Unknown) Keys() []string {
	out := make([]string, 0, len(u))
	for k := range u {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// JSON tag parsing.
const (
	jsonTag       = "json"
	jsonTagSkip   = "-"
	jsonTagSep    = ","
	jsonTagInline = "inline"
)

// ExtractUnknown returns the keys of the JSON object data that are not in
// known. Pure.
func ExtractUnknown(data []byte, known []string) (Unknown, error) {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("agentio: unknown keys: %w", err)
	}
	for _, k := range known {
		delete(all, k)
	}
	if len(all) == 0 {
		return nil, nil
	}
	return all, nil
}

// ExtractUnknownFor is ExtractUnknown with the known keys taken from the
// json tags (or field names) of struct type model, recursing into embedded
// structs. Fields of type Unknown and `json:"-"` fields are skipped. Why
// reflection on the type and not a value: the struct's tags are the single
// source of truth for "modelled", so a new field never needs a second list.
// Pure.
func ExtractUnknownFor(data []byte, model reflect.Type) (Unknown, error) {
	return ExtractUnknown(data, JSONKeys(model))
}

// JSONKeys lists the JSON object keys struct type t models. Pure.
func JSONKeys(t reflect.Type) []string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	unknownType := reflect.TypeFor[Unknown]()
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type == unknownType {
			continue
		}
		tag := f.Tag.Get(jsonTag)
		if tag == jsonTagSkip {
			continue
		}
		name, opts, _ := strings.Cut(tag, jsonTagSep)
		if f.Anonymous && name == "" {
			out = append(out, JSONKeys(f.Type)...)
			continue
		}
		if opts == jsonTagInline {
			out = append(out, JSONKeys(f.Type)...)
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out = append(out, name)
	}
	return out
}
