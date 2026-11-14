package codex

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// TOMLRaw is the exact source text of one TOML value (or, for a table
// sub-tree, its `key = value` lines), cut from the file by parser position.
// It keeps vendor data the library does not model without a
// map[string]any decode (SPEC §5.5).
type TOMLRaw string

// UnknownTOML maps a dotted key (TOML spelling, quoted parts where needed)
// to the source text of its value: every key a struct does not model.
type UnknownTOML map[string]TOMLRaw

// tomlKind classifies one flattened TOML value.
type tomlKind uint8

const (
	tomlString tomlKind = iota
	tomlBool
	tomlInt
	tomlFloat
	tomlDateTime
	tomlArray
	// tomlEmptyTable is an inline `{}`: presence without leaves.
	tomlEmptyTable
)

// String names the kind for warnings. Pure.
func (k tomlKind) String() string {
	switch k {
	case tomlString:
		return "string"
	case tomlBool:
		return "bool"
	case tomlInt:
		return "integer"
	case tomlFloat:
		return "float"
	case tomlDateTime:
		return "datetime"
	case tomlArray:
		return "array"
	case tomlEmptyTable:
		return "table"
	}
	return "unknown"
}

// tomlValue is one scalar or array value with its source text.
type tomlValue struct {
	kind tomlKind
	str  string // string value, or datetime source text
	b    bool
	i    int64
	f    float64
	arr  []tomlValue
	raw  string
}

// tomlEntry is one leaf of the document: a fully resolved key path (array
// of tables elements carry their index as a path part) and its value.
type tomlEntry struct {
	path []string
	val  tomlValue
	line int
}

// key renders the entry's path in TOML dotted-key spelling.
func (e tomlEntry) key() string { return dottedKey(e.path) }

// dottedKey quotes every part that is not a bare key.
func dottedKey(parts []string) string {
	var sb strings.Builder
	for i, p := range parts {
		if i > 0 {
			sb.WriteByte('.')
		}
		if isBareKey(p) {
			sb.WriteString(p)
		} else {
			sb.WriteString(strconv.Quote(p))
		}
	}
	return sb.String()
}

func isBareKey(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		bare := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
		if !bare {
			return false
		}
	}
	return true
}

// TOML keyword spellings for floats.
const (
	tomlInf  = "inf"
	tomlNaN  = "nan"
	tomlSign = "+-"
)

// tomlFlattener walks go-toml/v2's unstable AST one expression at a time.
type tomlFlattener struct {
	p   unstable.Parser
	src []byte
	out []tomlEntry
	// arrayIndex: original dotted header of an array of tables → index of
	// its current element, so nested [[a.b]] under [[a]] resolve to a.<i>.b.
	arrayIndex map[string]int
	// arrayCount: resolved dotted header → elements seen so far.
	arrayCount map[string]int
}

// parseTOML flattens src into leaf entries in document order. Inline tables
// and arrays of inline tables are flattened like their header forms, so
// `env = {A = "x"}` and `[x.env] A = "x"` produce the same entry. Pure.
func parseTOML(src []byte) ([]tomlEntry, error) {
	f := &tomlFlattener{src: src, arrayIndex: map[string]int{}, arrayCount: map[string]int{}}
	f.p.Reset(src)
	var prefix []string
	for f.p.NextExpression() {
		e := f.p.Expression()
		switch e.Kind {
		case unstable.Table:
			prefix = f.resolve(keyParts(e))
		case unstable.ArrayTable:
			orig := keyParts(e)
			parent := f.resolve(orig[:len(orig)-1])
			resolved := append(append([]string{}, parent...), orig[len(orig)-1])
			rk := dottedKey(resolved)
			idx := f.arrayCount[rk]
			f.arrayCount[rk] = idx + 1
			f.arrayIndex[dottedKey(orig)] = idx
			prefix = append(resolved, strconv.Itoa(idx))
		case unstable.KeyValue:
			path := append(append([]string{}, prefix...), keyParts(e)...)
			if err := f.emit(path, e.Value(), f.valueText(e)); err != nil {
				return nil, err
			}
		}
	}
	if err := f.p.Error(); err != nil {
		return nil, fmt.Errorf("codex: toml: %w", err)
	}
	return f.out, nil
}

// resolve inserts the current element index after every prefix of orig
// that names an array of tables.
func (f *tomlFlattener) resolve(orig []string) []string {
	out := make([]string, 0, len(orig)+1)
	for i, part := range orig {
		out = append(out, part)
		if idx, ok := f.arrayIndex[dottedKey(orig[:i+1])]; ok {
			out = append(out, strconv.Itoa(idx))
		}
	}
	return out
}

func keyParts(n *unstable.Node) []string {
	var parts []string
	it := n.Key()
	for it.Next() {
		parts = append(parts, string(it.Node().Data))
	}
	return parts
}

// valueText cuts the value's source text out of a KeyValue's raw range:
// everything after the `=` that follows the last key part.
func (f *tomlFlattener) valueText(kv *unstable.Node) string {
	var last *unstable.Node
	it := kv.Key()
	for it.Next() {
		last = it.Node()
	}
	end := int(kv.Raw.Offset + kv.Raw.Length)
	if last == nil || end > len(f.src) {
		return ""
	}
	from := int(last.Raw.Offset + last.Raw.Length)
	eq := bytes.IndexByte(f.src[from:end], '=')
	if eq < 0 {
		return ""
	}
	return strings.TrimSpace(string(f.src[from+eq+1 : end]))
}

func (f *tomlFlattener) line(n *unstable.Node) int {
	if n.Raw.Length == 0 && n.Raw.Offset == 0 {
		return 0
	}
	return f.p.Shape(n.Raw).Start.Line
}

// emit appends the leaves of value v found at path.
func (f *tomlFlattener) emit(path []string, v *unstable.Node, raw string) error {
	switch v.Kind {
	case unstable.InlineTable:
		it := v.Children()
		hasLeaf := false
		for it.Next() {
			kv := it.Node()
			if kv.Kind != unstable.KeyValue {
				continue
			}
			hasLeaf = true
			sub := append(append([]string{}, path...), keyParts(kv)...)
			if err := f.emit(sub, kv.Value(), f.valueText(kv)); err != nil {
				return err
			}
		}
		if !hasLeaf {
			f.out = append(f.out, tomlEntry{path: path, val: tomlValue{kind: tomlEmptyTable, raw: raw}})
		}
		return nil
	case unstable.Array:
		if arrayOfTables(v) {
			it := v.Children()
			i := 0
			for it.Next() {
				el := it.Node()
				if el.Kind != unstable.InlineTable {
					continue
				}
				sub := append(append([]string{}, path...), strconv.Itoa(i))
				if err := f.emit(sub, el, ""); err != nil {
					return err
				}
				i++
			}
			return nil
		}
	}
	val, err := f.value(v)
	if err != nil {
		return err
	}
	if raw != "" {
		val.raw = raw
	}
	f.out = append(f.out, tomlEntry{path: path, val: val, line: f.line(v)})
	return nil
}

// arrayOfTables reports a non-empty array whose elements are all inline tables.
func arrayOfTables(v *unstable.Node) bool {
	it := v.Children()
	n := 0
	for it.Next() {
		switch it.Node().Kind {
		case unstable.Comment:
			continue
		case unstable.InlineTable:
			n++
		default:
			return false
		}
	}
	return n > 0
}

// value converts a scalar or array node.
func (f *tomlFlattener) value(v *unstable.Node) (tomlValue, error) {
	raw := ""
	if v.Raw.Length > 0 {
		raw = string(f.p.Raw(v.Raw))
	}
	data := string(v.Data)
	switch v.Kind {
	case unstable.String:
		return tomlValue{kind: tomlString, str: data, raw: raw}, nil
	case unstable.Bool:
		return tomlValue{kind: tomlBool, b: data == "true", raw: raw}, nil
	case unstable.Integer:
		n, err := strconv.ParseInt(strings.ReplaceAll(data, "_", ""), 0, 64)
		if err != nil {
			return tomlValue{}, fmt.Errorf("codex: toml integer %q: %w", data, err)
		}
		return tomlValue{kind: tomlInt, i: n, raw: raw}, nil
	case unstable.Float:
		return tomlValue{kind: tomlFloat, f: parseTOMLFloat(data), raw: raw}, nil
	case unstable.LocalDate, unstable.LocalTime, unstable.LocalDateTime, unstable.DateTime:
		return tomlValue{kind: tomlDateTime, str: data, raw: raw}, nil
	case unstable.Array:
		out := tomlValue{kind: tomlArray, arr: []tomlValue{}}
		it := v.Children()
		var parts []string
		for it.Next() {
			el := it.Node()
			if el.Kind == unstable.Comment {
				continue
			}
			if el.Kind == unstable.InlineTable {
				// mixed array with tables: keep only as raw text
				continue
			}
			ev, err := f.value(el)
			if err != nil {
				return tomlValue{}, err
			}
			out.arr = append(out.arr, ev)
			parts = append(parts, ev.raw)
		}
		out.raw = "[" + strings.Join(parts, ", ") + "]"
		return out, nil
	}
	return tomlValue{}, fmt.Errorf("codex: toml: unexpected %s value", v.Kind)
}

func parseTOMLFloat(s string) float64 {
	clean := strings.ReplaceAll(s, "_", "")
	body := strings.TrimLeft(clean, tomlSign)
	neg := strings.HasPrefix(clean, "-")
	switch body {
	case tomlInf:
		if neg {
			return math.Inf(-1)
		}
		return math.Inf(1)
	case tomlNaN:
		return math.NaN()
	}
	n, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0
	}
	return n
}
