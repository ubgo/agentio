package agentio

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"time"
)

// SchemaEnums returns the allowed string values of a named string type
// (claude.Effort -> low, medium, …) or nil when the type is open. A vendor
// passes one to JSONSchemaFor so enum fields become JSON Schema enums.
type SchemaEnums func(t reflect.Type) []string

// SchemaNode is one JSON Schema (draft 2020-12) node, the subset
// JSONSchemaFor emits: enough for an editor's completion and a config
// loader's type check, never a validator of every vendor rule.
type SchemaNode struct {
	Type                 string                `json:"type,omitempty"`
	Enum                 []string              `json:"enum,omitempty"`
	Items                *SchemaNode           `json:"items,omitempty"`
	Properties           map[string]SchemaNode `json:"properties,omitempty"`
	AdditionalProperties *bool                 `json:"additionalProperties,omitempty"`
	Description          string                `json:"description,omitempty"`
}

// JSON Schema type names.
const (
	schemaObject  = "object"
	schemaArray   = "array"
	schemaString  = "string"
	schemaBoolean = "boolean"
	schemaInteger = "integer"
	schemaNumber  = "number"
)

// JSONSchemaFor builds the schema of struct type t from its json tags:
// strings (and named strings, with enums from SchemaEnums), bools, ints,
// floats, slices, maps with string keys and nested structs; Unknown and
// `json:"-"` fields are skipped. closed sets additionalProperties false on
// t itself (a launch-options block, where an unknown key is a typo) and
// leaves nested objects open. A field type it cannot describe (Raw, a
// time.Duration) is emitted without a type, which accepts any value.
// Pure.
func JSONSchemaFor(t reflect.Type, enums SchemaEnums, closed bool) SchemaNode {
	n := schemaOf(t, enums, 0)
	if closed {
		f := false
		n.AdditionalProperties = &f
	}
	return n
}

// maxSchemaDepth stops a self-referencing type from recursing forever.
const maxSchemaDepth = 8

// schemaOf describes one type. Pure.
func schemaOf(t reflect.Type, enums SchemaEnums, depth int) SchemaNode {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if depth > maxSchemaDepth || t == reflect.TypeFor[Raw]() || t == reflect.TypeFor[time.Duration]() {
		return SchemaNode{}
	}
	switch t.Kind() {
	case reflect.String:
		n := SchemaNode{Type: schemaString}
		if enums != nil {
			n.Enum = enums(t)
		}
		return n
	case reflect.Bool:
		return SchemaNode{Type: schemaBoolean}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return SchemaNode{Type: schemaInteger}
	case reflect.Uint8:
		// A uint8 with MarshalText is a library enum spelled by name.
		if enums != nil {
			if e := enums(t); e != nil {
				return SchemaNode{Type: schemaString, Enum: e}
			}
		}
		return SchemaNode{Type: schemaInteger}
	case reflect.Float32, reflect.Float64:
		return SchemaNode{Type: schemaNumber}
	case reflect.Slice, reflect.Array:
		item := schemaOf(t.Elem(), enums, depth+1)
		return SchemaNode{Type: schemaArray, Items: &item}
	case reflect.Map:
		return SchemaNode{Type: schemaObject}
	case reflect.Struct:
		n := SchemaNode{Type: schemaObject, Properties: map[string]SchemaNode{}}
		unknownType := reflect.TypeFor[Unknown]()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name, ok := jsonFieldName(f)
			if !ok || f.Type == unknownType {
				continue
			}
			n.Properties[name] = schemaOf(f.Type, enums, depth+1)
		}
		return n
	}
	return SchemaNode{}
}

// jsonFieldName is the JSON key of an exported field; false for `json:"-"`
// and unexported fields. Pure.
func jsonFieldName(f reflect.StructField) (string, bool) {
	if !f.IsExported() {
		return "", false
	}
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", false
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		name = f.Name
	}
	return name, true
}

// MarshalSchema renders a schema with sorted keys (encoding/json sorts map
// keys), for a stable fragment a caller can embed. Pure.
func MarshalSchema(n SchemaNode) (Raw, error) {
	b, err := json.Marshal(n)
	return b, err
}

// SchemaKeys lists a schema's top-level property names, sorted. Pure.
func SchemaKeys(n SchemaNode) []string {
	out := make([]string, 0, len(n.Properties))
	for k := range n.Properties {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
