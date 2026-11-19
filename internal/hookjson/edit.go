package hookjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Keys of the shared hooks shape.
const (
	keyHooks   = "hooks"
	keyType    = "type"
	keyCommand = "command"
	typeCmd    = "command"
)

// Layout of inserted JSON.
const (
	// defaultUnit is the indent step when the file has none to copy (all
	// three vendors write two spaces).
	defaultUnit = "  "
	// memberColon separates a key from its value in multi-line output, as
	// encoding/json's MarshalIndent does.
	memberColon = ": "
)

// ErrShape means the document is valid JSON but not the shape this package
// edits (the top level is not an object, "hooks" is not an object, or an
// event is not an array). The caller refuses to rewrite such a file.
var ErrShape = errors.New("hookjson: unexpected settings shape")

// Handler is one hook handler found in a document.
type Handler struct {
	// Event is the event key the handler sits under.
	Event string
	// IsCommand reports a command hook: type "command", or no type with a
	// command string.
	IsCommand bool
	// Command is the handler's command string ("" when it has none).
	Command string
	// Raw is the handler object's exact bytes.
	Raw []byte
}

// loc addresses one handler: hooks member, event member, group, handler.
type loc struct {
	hooksIdx, eventIdx, groupIdx, handlerIdx int
}

// doc is a parsed document plus the parts this package navigates.
type doc struct {
	b     []byte
	root  node
	found bool // the document holds a value (false: empty file)
	opt   Options
}

// load parses b; the top-level value must be an object. Pure.
func load(b []byte, opt Options) (doc, error) {
	root, found, err := parseDoc(b, opt)
	if err != nil {
		return doc{}, err
	}
	if found && root.kind != kindObject {
		return doc{}, fmt.Errorf("%w: the top-level value is not an object", ErrShape)
	}
	return doc{b: b, root: root, found: found, opt: opt}, nil
}

// hooks returns the "hooks" object, its member index, and ok=false when
// it is absent or null. Pure.
func (d doc) hooks() (node, int, bool, error) {
	if !d.found {
		return node{}, -1, false, nil
	}
	i := d.root.find(keyHooks)
	if i < 0 {
		return node{}, -1, false, nil
	}
	v := d.root.members[i].val
	if d.isNull(v) {
		return node{}, i, false, nil
	}
	if v.kind != kindObject {
		return node{}, i, false, fmt.Errorf("%w: %q is not an object", ErrShape, keyHooks)
	}
	return v, i, true, nil
}

// isNull reports the null literal. Pure.
func (d doc) isNull(n node) bool {
	return n.kind == kindScalar && string(d.b[n.start:n.end]) == literalNull
}

// walk calls fn for every handler object under hooks.<Event>[].hooks[],
// in file order; values of another shape are not hooks this package
// manages and are skipped. fn returns false to stop. Pure.
func (d doc) walk(fn func(Handler, loc) bool) error {
	hk, hi, ok, err := d.hooks()
	if err != nil || !ok {
		return err
	}
	for ei, ev := range hk.members {
		if ev.val.kind != kindArray {
			continue
		}
		for gi, g := range ev.val.elems {
			if g.kind != kindObject {
				continue
			}
			hix := g.find(keyHooks)
			if hix < 0 || g.members[hix].val.kind != kindArray {
				continue
			}
			for ki, h := range g.members[hix].val.elems {
				if h.kind != kindObject {
					continue
				}
				if !fn(d.handler(ev.key, h), loc{hooksIdx: hi, eventIdx: ei, groupIdx: gi, handlerIdx: ki}) {
					return nil
				}
			}
		}
	}
	return nil
}

// handler reads the type and command of one handler object. Pure.
func (d doc) handler(event string, h node) Handler {
	out := Handler{Event: event, Raw: d.b[h.start:h.end]}
	typ, hasType := d.stringMember(h, keyType)
	cmd, hasCmd := d.stringMember(h, keyCommand)
	out.Command = cmd
	out.IsCommand = hasCmd && (typ == typeCmd || !hasType)
	return out
}

// stringMember decodes member key of obj when it is a string. Pure.
func (d doc) stringMember(obj node, key string) (string, bool) {
	i := obj.find(key)
	if i < 0 || obj.members[i].val.kind != kindString {
		return "", false
	}
	v := obj.members[i].val
	s, err := decodeString(d.b[v.start:v.end])
	return s, err == nil
}

// Handlers lists every handler of the document in file order. Pure.
func Handlers(b []byte, opt Options) ([]Handler, error) {
	d, err := load(b, opt)
	if err != nil {
		return nil, err
	}
	var out []Handler
	err = d.walk(func(h Handler, _ loc) bool {
		out = append(out, h)
		return true
	})
	return out, err
}

// Has reports a command hook with exactly this command under event. Pure.
func Has(b []byte, event, command string, opt Options) (bool, error) {
	hs, err := Handlers(b, opt)
	if err != nil {
		return false, err
	}
	for _, h := range hs {
		if h.Event == event && h.IsCommand && h.Command == command {
			return true, nil
		}
	}
	return false, nil
}

// commandGroup is the group a neutral install appends: no matcher (every
// vendor reads a missing matcher as "match all"), one command handler.
type commandGroup struct {
	Hooks []commandHandler `json:"hooks"`
}

// commandHandler is one command handler in key order type, command.
type commandHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// CommandGroup returns the compact JSON of a matcher group holding one
// command handler; '&', '<' and '>' stay literal so a shell command
// reads as typed. Pure.
func CommandGroup(command string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(commandGroup{Hooks: []commandHandler{{Type: typeCmd, Command: command}}}); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// AppendGroup appends group (a JSON object) as the last matcher group of
// event, creating the event key, the "hooks" object or the whole document
// when missing. Only the inserted bytes are new; everything else in b is
// kept byte for byte. Pure.
func AppendGroup(b []byte, event string, group []byte, opt Options) ([]byte, error) {
	var g map[string]json.RawMessage
	if err := json.Unmarshal(group, &g); err != nil {
		return nil, fmt.Errorf("hookjson: group is not a JSON object: %w", err)
	}
	group = unescapeHTML(compact(group))
	d, err := load(b, opt)
	if err != nil {
		return nil, err
	}
	e := editor{d: d, unit: d.unit()}
	if !d.found {
		return e.newDocument(event, group), nil
	}
	hk, hi, ok, err := d.hooks()
	if err != nil {
		return nil, err
	}
	switch {
	case hi < 0:
		return e.insertItem(d.root, keyHooks, e.object(event, e.array(group))), nil
	case !ok: // "hooks": null
		return e.replace(d.root.members[hi].val, e.object(event, e.array(group))), nil
	}
	ei := hk.find(event)
	if ei < 0 {
		return e.insertItem(hk, event, e.array(group)), nil
	}
	ev := hk.members[ei].val
	switch {
	case ev.kind == kindArray:
		return e.insertItem(ev, "", e.raw(group)), nil
	case d.isNull(ev):
		return e.replace(ev, e.array(group)), nil
	}
	return nil, fmt.Errorf("%w: hooks.%s is not an array", ErrShape, event)
}

// RemoveHandlers removes every handler for which drop reports true. A
// group left without handlers is removed, then an event left without
// groups, then a "hooks" object left empty; every other byte is kept.
// Pure.
func RemoveHandlers(b []byte, drop func(Handler) bool, opt Options) ([]byte, int, error) {
	removed := 0
	for {
		d, err := load(b, opt)
		if err != nil {
			return nil, 0, err
		}
		var at loc
		hit := false
		if err := d.walk(func(h Handler, l loc) bool {
			if drop(h) {
				at, hit = l, true
				return false
			}
			return true
		}); err != nil {
			return nil, 0, err
		}
		if !hit {
			return b, removed, nil
		}
		b = d.removeAt(at)
		removed++
	}
}

// removeAt removes one handler, cascading to its group, event and the
// "hooks" member when each would be left empty. Pure.
func (d doc) removeAt(l loc) []byte {
	hk := d.root.members[l.hooksIdx].val
	ev := hk.members[l.eventIdx].val
	g := ev.elems[l.groupIdx]
	handlers := g.members[g.find(keyHooks)].val
	switch {
	case handlers.itemCount() > 1:
		return removeItem(d.b, handlers, l.handlerIdx)
	case ev.itemCount() > 1:
		return removeItem(d.b, ev, l.groupIdx)
	case hk.itemCount() > 1:
		return removeItem(d.b, hk, l.eventIdx)
	}
	return removeItem(d.b, d.root, l.hooksIdx)
}

// removeItem deletes item i of container c with the separator that joined
// it, so the neighbours keep their bytes: the last item takes the comma
// before it, any other the comma after it. A sole item leaves the
// container empty ({} or []). Pure.
func removeItem(b []byte, c node, i int) []byte {
	n := c.itemCount()
	var from, to int
	switch {
	case n == 1:
		from, to = c.start+1, c.end-1
	case i > 0:
		_, from = c.itemSpan(i - 1)
		_, to = c.itemSpan(i)
	default:
		from, _ = c.itemSpan(0)
		to, _ = c.itemSpan(1)
	}
	return splice(b, from, to, nil)
}

// splice replaces b[from:to] with ins in a new slice. Pure.
func splice(b []byte, from, to int, ins []byte) []byte {
	out := make([]byte, 0, len(b)-(to-from)+len(ins))
	out = append(out, b[:from]...)
	out = append(out, ins...)
	return append(out, b[to:]...)
}

// unit is the file's indent step: the first top-level member's indent
// beyond the line of the opening brace; defaultUnit when there is none.
// Pure.
func (d doc) unit() string {
	if !d.found || len(d.root.members) == 0 {
		return defaultUnit
	}
	first := d.root.members[0].start
	if !bytes.ContainsRune(d.b[d.root.start:first], '\n') {
		return defaultUnit
	}
	outer, inner := lineIndent(d.b, d.root.start), lineIndent(d.b, first)
	if len(inner) > len(outer) && inner[:len(outer)] == outer {
		return inner[len(outer):]
	}
	return defaultUnit
}

// lineIndent is the leading blanks of the line holding pos. Pure.
func lineIndent(b []byte, pos int) string {
	start := bytes.LastIndexByte(b[:pos], '\n') + 1
	end := start
	for end < len(b) && (b[end] == ' ' || b[end] == '\t') {
		end++
	}
	return string(b[start:end])
}

// compact strips insignificant whitespace from valid JSON. Pure.
func compact(b []byte) []byte {
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		return b
	}
	return buf.Bytes()
}

// htmlEscaped maps the four hex digits of a \u escape that encoding/json
// writes for '&', '<' or '>' back to the character. Pure.
func htmlEscaped(hex string) (byte, bool) {
	switch hex {
	case "0026":
		return '&', true
	case "003c", "003C":
		return '<', true
	case "003e", "003E":
		return '>', true
	}
	return 0, false
}

// unicodeEscapeLen is the length of a \uXXXX escape.
const unicodeEscapeLen = 6

// unescapeHTML turns encoding/json's HTML escapes back into the literal
// characters, which JSON allows: a command "a && b" then reads as typed.
// Escapes are consumed pairwise so an escaped backslash is never mistaken
// for the start of an escape. Pure.
func unescapeHTML(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		if b[i] != '\\' || i+1 >= len(b) {
			out = append(out, b[i])
			continue
		}
		if b[i+1] == 'u' && i+unicodeEscapeLen <= len(b) {
			if c, ok := htmlEscaped(string(b[i+2 : i+unicodeEscapeLen])); ok {
				out = append(out, c)
				i += unicodeEscapeLen - 1
				continue
			}
		}
		out = append(out, b[i], b[i+1])
		i++
	}
	return out
}

// fragment renders a value at a given line indent: multi-line output
// (indent prefix, unit step) or compact single-line output.
type fragment func(indent string, multi bool) []byte

// editor inserts fragments in the style of the document.
type editor struct {
	d    doc
	unit string
}

// raw renders compact JSON v. Pure.
func (e editor) raw(v []byte) fragment {
	return func(indent string, multi bool) []byte {
		if !multi {
			return v
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, v, indent, e.unit); err != nil {
			return v
		}
		return buf.Bytes()
	}
}

// array renders [elem]. Pure.
func (e editor) array(elem []byte) fragment {
	return e.raw(append(append([]byte{'['}, elem...), ']'))
}

// keyed renders `"key": value` (multi-line) or `"key":value`. Pure.
func keyed(key string, val []byte, multi bool) []byte {
	k, _ := json.Marshal(key) // a Go string always marshals
	if multi {
		k = append(k, memberColon...)
	} else {
		k = append(k, ':')
	}
	return append(k, val...)
}

// object renders {"key": value} from a value fragment. Pure.
func (e editor) object(key string, val fragment) fragment {
	return func(indent string, multi bool) []byte {
		if !multi {
			return append(append([]byte{'{'}, keyed(key, val("", false), false)...), '}')
		}
		inner := indent + e.unit
		out := append([]byte("{\n"+inner), keyed(key, val(inner, true), true)...)
		return append(out, "\n"+indent+"}"...)
	}
}

// multi reports a document laid out over several lines. Pure.
func (e editor) multi() bool { return bytes.ContainsRune(e.d.b, '\n') }

// insertItem appends an item after the last item of container c: the
// member key: val for an object (key must be set), the element val for an
// array. Pure.
func (e editor) insertItem(c node, key string, val fragment) []byte {
	b := e.d.b
	n := c.itemCount()
	render := func(indent string, multi bool) []byte {
		if c.kind == kindObject {
			return keyed(key, val(indent, multi), multi)
		}
		return val(indent, multi)
	}
	if n > 0 {
		ls, le := c.itemSpan(n - 1)
		if bytes.ContainsRune(b[c.start:ls], '\n') {
			indent := lineIndent(b, ls)
			return splice(b, le, le, append([]byte(",\n"+indent), render(indent, true)...))
		}
		sep := ","
		if b[ls-1] == ' ' {
			sep = ", "
		}
		return splice(b, le, le, append([]byte(sep), render("", false)...))
	}
	inner := b[c.start+1 : c.end-1]
	blank := len(bytes.TrimSpace(inner)) == 0
	to := c.start + 1
	if blank {
		to = c.end - 1
	}
	if !e.multi() {
		return splice(b, c.start+1, to, render("", false))
	}
	outer := lineIndent(b, c.start)
	ins := append([]byte("\n"+outer+e.unit), render(outer+e.unit, true)...)
	if blank {
		ins = append(ins, "\n"+outer...)
	}
	return splice(b, c.start+1, to, ins)
}

// replace swaps value v (a null) for a rendered fragment. Pure.
func (e editor) replace(v node, frag fragment) []byte {
	return splice(e.d.b, v.start, v.end, frag(lineIndent(e.d.b, v.start), e.multi()))
}

// newDocument writes {"hooks": {event: [group]}} for an empty file. A
// file holding only comments keeps them, above the new object. Pure.
func (e editor) newDocument(event string, group []byte) []byte {
	body := e.object(keyHooks, e.object(event, e.array(group)))("", true)
	out := append([]byte{}, e.d.b...)
	if len(bytes.TrimSpace(bytes.TrimPrefix(out, []byte(bom)))) > 0 && !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	out = append(out, body...)
	return append(out, '\n')
}
