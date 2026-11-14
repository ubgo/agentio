package agentio

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

// TestInsertedBlock finds what one field added to the argv.
func TestInsertedBlock(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ a, b, want []string }{
		{[]string{"-p"}, []string{"-p", "--effort", "max"}, []string{"--effort", "max"}},
		{[]string{"-p", "--", "x"}, []string{"-p", "--model", "m", "--", "x"}, []string{"--model", "m"}},
		{nil, []string{"--", "x"}, []string{"--", "x"}},
		{[]string{"a"}, []string{"a"}, []string{}},
	} {
		if got := insertedBlock(tc.a, tc.b); !slices.Equal(got, tc.want) {
			t.Errorf("insertedBlock(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestOptionValueJSON converts command-line values per kind.
func TestOptionValueJSON(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		k    OptionKey
		in   string
		want string
	}{
		{OptionKey{Key: "m", Kind: OptionString}, "opus", `"opus"`},
		{OptionKey{Key: "e", Kind: OptionEnum, Values: []string{"low", "max"}}, "max", `"max"`},
		{OptionKey{Key: "b", Kind: OptionBool}, "true", `true`},
		{OptionKey{Key: "n", Kind: OptionNumber}, "2.5", `2.5`},
		{OptionKey{Key: "l", Kind: OptionList}, "a, b", `["a","b"]`},
		{OptionKey{Key: "l", Kind: OptionList}, `["a,b"]`, `["a,b"]`},
		{OptionKey{Key: "j", Kind: OptionJSON}, `{"type":"object"}`, `{"type":"object"}`},
	} {
		got, err := OptionValueJSON(tc.k, tc.in)
		if err != nil || string(got) != tc.want {
			t.Errorf("%s=%q: %s %v, want %s", tc.k.Key, tc.in, got, err, tc.want)
		}
	}
	for _, tc := range []struct {
		k  OptionKey
		in string
	}{
		{OptionKey{Key: "e", Kind: OptionEnum, Values: []string{"low"}}, "max"},
		{OptionKey{Key: "b", Kind: OptionBool}, "maybe"},
		{OptionKey{Key: "n", Kind: OptionNumber}, "many"},
		{OptionKey{Key: "l", Kind: OptionEnumList, Values: []string{"user"}}, "user,admin"},
		{OptionKey{Key: "j", Kind: OptionObject}, "{"},
	} {
		if _, err := OptionValueJSON(tc.k, tc.in); !errors.Is(err, ErrInvalidOption) {
			t.Errorf("%s=%q must be refused: %v", tc.k.Key, tc.in, err)
		}
	}
}

// schemaSample is a struct for TestJSONSchemaFor.
type schemaSample struct {
	Name    string            `json:"name"`
	On      *bool             `json:"on,omitempty"`
	Count   int               `json:"count"`
	Tags    []string          `json:"tags"`
	Env     map[string]string `json:"env"`
	Kind    TokenKind         `json:"kind"`
	Skip    string            `json:"-"`
	Unknown Unknown
}

// TestJSONSchemaFor maps Go types to schema types and enums.
func TestJSONSchemaFor(t *testing.T) {
	t.Parallel()
	enums := func(t reflect.Type) []string {
		if t == reflect.TypeFor[TokenKind]() {
			return tokenKindNames()
		}
		return nil
	}
	s := JSONSchemaFor(reflect.TypeFor[schemaSample](), enums, true)
	if s.AdditionalProperties == nil || *s.AdditionalProperties {
		t.Error("closed schema must forbid other keys")
	}
	want := map[string]string{"name": "string", "on": "boolean", "count": "integer", "tags": "array", "env": "object", "kind": "string"}
	for k, typ := range want {
		if s.Properties[k].Type != typ {
			t.Errorf("%s = %q, want %q", k, s.Properties[k].Type, typ)
		}
	}
	if len(s.Properties) != len(want) || len(s.Properties["kind"].Enum) != len(tokenKindNames()) {
		t.Errorf("properties = %v", SchemaKeys(s))
	}
	if b, err := MarshalSchema(s); err != nil || len(b) == 0 {
		t.Errorf("marshal = %s %v", b, err)
	}
}
