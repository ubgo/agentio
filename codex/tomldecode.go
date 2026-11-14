package codex

import (
	"strings"
	"time"

	"github.com/khanakia/agentcli"
)

// Opt is the root presence-tracked scalar, re-exported so Codex config
// types read Opt[T] (CODEX_API.md §0).
type Opt[T agentcli.Scalar] = agentcli.Opt[T]

// tomlDecoder collects what one file's typed decode could not place:
// unknown keys (kept as source text) and type-changed keys (kept as source
// text plus a Warning), so one bad key never fails the file (invariant 17).
type tomlDecoder struct {
	file  string
	warns agentcli.Warnings
	// target is where the entry being decoded goes when it is unknown or
	// type-changed: the Unknown map of the struct that owns it, keyed
	// relative to that struct (prefix parts dropped).
	target *UnknownTOML
	prefix int
	// counts tallies every leaf for Config.KeyCount.
	counts KeyCounts
}

// KeyCounts says how a file's leaf keys were placed: Typed into struct
// fields, Raw into a TOMLRaw sub-tree the library keeps but does not type
// yet, Unknown into an Unknown map (unknown or type-changed keys).
// Total = Typed + Raw + Unknown; nested tables and array-of-table
// elements count per leaf.
type KeyCounts struct {
	Total, Typed, Raw, Unknown int
}

// raw appends e to a raw sub-tree and counts it.
func (d *tomlDecoder) raw(dst *TOMLRaw, e tomlEntry, n int) {
	d.counts.Raw++
	appendRaw(dst, e, n)
}

// scope points unknown keys at u, keyed after the first n path parts.
func (d *tomlDecoder) scope(u *UnknownTOML, n int) { d.target, d.prefix = u, n }

func newTOMLDecoder(file string) *tomlDecoder {
	return &tomlDecoder{file: file}
}

// keep records e as unknown under its full dotted key.
func (d *tomlDecoder) keep(e tomlEntry) {
	d.counts.Unknown++
	if d.target == nil {
		return
	}
	if *d.target == nil {
		*d.target = UnknownTOML{}
	}
	key := dottedKey(rest(e, d.prefix))
	if key == "" {
		key = e.key()
	}
	(*d.target)[key] = TOMLRaw(e.val.raw)
}

// mismatch keeps e and warns that its type differs from the model.
func (d *tomlDecoder) mismatch(e tomlEntry, want tomlKind) {
	d.keep(e)
	d.warns = append(d.warns, agentcli.Warning{
		Kind:   agentcli.WarnTypeChanged,
		File:   d.file,
		Key:    e.key(),
		Detail: "expected " + want.String() + ", found " + e.val.kind.String(),
	})
}

// rest is the entry path after the first n parts (the part a nested
// decoder switches on).
func rest(e tomlEntry, n int) []string {
	if len(e.path) <= n {
		return nil
	}
	return e.path[n:]
}

func (d *tomlDecoder) str(dst *Opt[string], e tomlEntry) {
	if e.val.kind != tomlString {
		d.mismatch(e, tomlString)
		return
	}
	*dst = agentcli.Some(e.val.str)
}

// enum decodes a string into an open string enum, keeping unknown values.
func enumOpt[T ~string](d *tomlDecoder, dst *Opt[T], e tomlEntry) {
	if e.val.kind != tomlString {
		d.mismatch(e, tomlString)
		return
	}
	*dst = agentcli.Some(T(e.val.str))
}

func (d *tomlDecoder) boolean(dst *Opt[bool], e tomlEntry) {
	if e.val.kind != tomlBool {
		d.mismatch(e, tomlBool)
		return
	}
	*dst = agentcli.Some(e.val.b)
}

func (d *tomlDecoder) integer(dst *Opt[int64], e tomlEntry) {
	if e.val.kind != tomlInt {
		d.mismatch(e, tomlInt)
		return
	}
	*dst = agentcli.Some(e.val.i)
}

// duration decodes an integer (or float) count of unit.
func (d *tomlDecoder) duration(dst *Opt[time.Duration], e tomlEntry, unit time.Duration) {
	switch e.val.kind {
	case tomlInt:
		*dst = agentcli.Some(time.Duration(e.val.i) * unit)
	case tomlFloat:
		*dst = agentcli.Some(time.Duration(e.val.f * float64(unit)))
	default:
		d.mismatch(e, tomlInt)
	}
}

// strings decodes an array of strings; a single string is accepted where
// upstream allows "string or array" (normalised to one element).
func (d *tomlDecoder) strList(dst *[]string, e tomlEntry, allowScalar bool) {
	if allowScalar && e.val.kind == tomlString {
		*dst = []string{e.val.str}
		return
	}
	if e.val.kind != tomlArray {
		d.mismatch(e, tomlArray)
		return
	}
	out := make([]string, 0, len(e.val.arr))
	for _, v := range e.val.arr {
		if v.kind != tomlString {
			d.mismatch(e, tomlArray)
			return
		}
		out = append(out, v.str)
	}
	*dst = out
}

// strMap sets one key of a string→string table.
func (d *tomlDecoder) strMap(dst *map[string]string, key string, e tomlEntry) {
	if e.val.kind != tomlString {
		d.mismatch(e, tomlString)
		return
	}
	if *dst == nil {
		*dst = map[string]string{}
	}
	(*dst)[key] = e.val.str
}

// appendRaw adds one entry to a raw sub-tree as a `relative.key = value`
// line, so the sub-tree stays valid TOML text.
func appendRaw(dst *TOMLRaw, e tomlEntry, n int) {
	var sb strings.Builder
	sb.WriteString(string(*dst))
	r := rest(e, n)
	if len(r) == 0 {
		if *dst == "" {
			*dst = TOMLRaw(e.val.raw)
			return
		}
		sb.WriteString(e.val.raw)
	} else {
		sb.WriteString(dottedKey(r))
		sb.WriteString(" = ")
		sb.WriteString(e.val.raw)
	}
	sb.WriteByte('\n')
	*dst = TOMLRaw(sb.String())
}

// addName appends name once.
func addName(dst *[]string, name string) {
	for _, n := range *dst {
		if n == name {
			return
		}
	}
	*dst = append(*dst, name)
}
