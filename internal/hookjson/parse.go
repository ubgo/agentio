// Package hookjson edits the command-hook shape that Claude Code
// (settings.json), Codex (hooks.json) and Gemini CLI (settings.json) share:
//
//	{"hooks": {"<Event>": [{"matcher": "…", "hooks": [{"type": "command", "command": "…"}]}]}}
//
// Every edit is a splice into the original bytes: hooks, groups, handlers
// and top-level keys the edit does not touch keep their exact bytes, order
// and formatting. Why: a settings file is shared by many tools (ailog,
// status lines, notifiers), and re-marshalling it through a map would
// reorder keys and reformat hand-written JSON on every install. Everything
// here is Pure; the vendor Writer doors read, back up and write the file.
//
// The package is internal to the agentio module tree, so the vendor
// modules (claude, codex, gemini) share it without making it public API.
package hookjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// Value kinds of a parsed node.
const (
	kindObject byte = '{'
	kindArray  byte = '['
	kindString byte = '"'
	kindScalar byte = 'x' // number, true, false or null
)

// maxDepth bounds nesting so a hostile file cannot exhaust the stack.
const maxDepth = 512

// bom is the UTF-8 byte order mark; an editor on Windows may save one.
const bom = "\xef\xbb\xbf"

// literalNull is the JSON null literal.
const literalNull = "null"

// errSyntax is wrapped by every parse error.
var errSyntax = errors.New("hookjson: invalid JSON")

// Options say which input the vendor accepts.
type Options struct {
	// Comments allows // line and /* block */ comments outside strings
	// (Gemini CLI reads settings.json through strip-json-comments). They
	// are kept as they are, like whitespace.
	Comments bool
}

// node is one parsed JSON value with its byte span [start, end).
type node struct {
	kind       byte
	start, end int
	members    []member // objects
	elems      []node   // arrays
}

// member is one object member; start is the key's opening quote.
type member struct {
	key   string
	start int
	val   node
}

// find returns the index of the last member named key (encoding/json and
// JSON.parse both let the last duplicate win), or -1. Pure.
func (n node) find(key string) int {
	for i := len(n.members) - 1; i >= 0; i-- {
		if n.members[i].key == key {
			return i
		}
	}
	return -1
}

// itemCount is the number of members or elements. Pure.
func (n node) itemCount() int {
	if n.kind == kindObject {
		return len(n.members)
	}
	return len(n.elems)
}

// itemSpan is the span of item i: a member from its key to the end of its
// value, an element from start to end. Pure.
func (n node) itemSpan(i int) (start, end int) {
	if n.kind == kindObject {
		return n.members[i].start, n.members[i].val.end
	}
	return n.elems[i].start, n.elems[i].end
}

// parser is a small recursive-descent JSON parser that records spans.
type parser struct {
	b     []byte
	i     int
	opt   Options
	depth int
}

// parseDoc parses a whole document. A document that is empty (or only
// whitespace, comments and a BOM) returns ok=false and no error. Pure.
func parseDoc(b []byte, opt Options) (root node, ok bool, err error) {
	p := &parser{b: b, opt: opt}
	if bytes.HasPrefix(b, []byte(bom)) {
		p.i = len(bom)
	}
	if err := p.ws(); err != nil {
		return node{}, false, err
	}
	if p.i == len(b) {
		return node{}, false, nil
	}
	root, err = p.value()
	if err != nil {
		return node{}, false, err
	}
	if err := p.ws(); err != nil {
		return node{}, false, err
	}
	if p.i != len(b) {
		return node{}, false, p.errorf("data after the top-level value")
	}
	return root, true, nil
}

// errorf builds a syntax error naming the byte offset. Pure.
func (p *parser) errorf(msg string) error {
	return fmt.Errorf("%w at byte %d: %s", errSyntax, p.i, msg)
}

// ws skips whitespace and, when allowed, comments. Pure.
func (p *parser) ws() error {
	for p.i < len(p.b) {
		switch c := p.b[p.i]; {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			p.i++
		case c == '/' && p.opt.Comments && p.i+1 < len(p.b) && p.b[p.i+1] == '/':
			for p.i < len(p.b) && p.b[p.i] != '\n' {
				p.i++
			}
		case c == '/' && p.opt.Comments && p.i+1 < len(p.b) && p.b[p.i+1] == '*':
			end := bytes.Index(p.b[p.i+2:], []byte("*/"))
			if end < 0 {
				return p.errorf("unterminated comment")
			}
			p.i += 2 + end + 2
		default:
			return nil
		}
	}
	return nil
}

// value parses one value at p.i (whitespace already skipped). Pure.
func (p *parser) value() (node, error) {
	if p.i >= len(p.b) {
		return node{}, p.errorf("unexpected end")
	}
	switch p.b[p.i] {
	case '{':
		return p.object()
	case '[':
		return p.array()
	case '"':
		start := p.i
		if err := p.str(); err != nil {
			return node{}, err
		}
		return node{kind: kindString, start: start, end: p.i}, nil
	default:
		return p.scalar()
	}
}

// enter and leave bound the nesting depth.
func (p *parser) enter() error {
	p.depth++
	if p.depth > maxDepth {
		return p.errorf("nesting too deep")
	}
	return nil
}

func (p *parser) leave() { p.depth-- }

// object parses {…}. Pure.
func (p *parser) object() (node, error) {
	if err := p.enter(); err != nil {
		return node{}, err
	}
	defer p.leave()
	n := node{kind: kindObject, start: p.i}
	p.i++ // '{'
	if err := p.ws(); err != nil {
		return node{}, err
	}
	if p.i < len(p.b) && p.b[p.i] == '}' {
		p.i++
		n.end = p.i
		return n, nil
	}
	for {
		if p.i >= len(p.b) || p.b[p.i] != '"' {
			return node{}, p.errorf("expected a member name")
		}
		keyStart := p.i
		if err := p.str(); err != nil {
			return node{}, err
		}
		key, err := decodeString(p.b[keyStart:p.i])
		if err != nil {
			return node{}, err
		}
		if err := p.ws(); err != nil {
			return node{}, err
		}
		if p.i >= len(p.b) || p.b[p.i] != ':' {
			return node{}, p.errorf("expected ':'")
		}
		p.i++
		if err := p.ws(); err != nil {
			return node{}, err
		}
		v, err := p.value()
		if err != nil {
			return node{}, err
		}
		n.members = append(n.members, member{key: key, start: keyStart, val: v})
		done, err := p.next('}')
		if err != nil {
			return node{}, err
		}
		if done {
			n.end = p.i
			return n, nil
		}
	}
}

// array parses […]. Pure.
func (p *parser) array() (node, error) {
	if err := p.enter(); err != nil {
		return node{}, err
	}
	defer p.leave()
	n := node{kind: kindArray, start: p.i}
	p.i++ // '['
	if err := p.ws(); err != nil {
		return node{}, err
	}
	if p.i < len(p.b) && p.b[p.i] == ']' {
		p.i++
		n.end = p.i
		return n, nil
	}
	for {
		v, err := p.value()
		if err != nil {
			return node{}, err
		}
		n.elems = append(n.elems, v)
		done, err := p.next(']')
		if err != nil {
			return node{}, err
		}
		if done {
			n.end = p.i
			return n, nil
		}
	}
}

// next consumes the separator after an item: ',' (more follow, whitespace
// skipped) or the closing byte (done). Pure.
func (p *parser) next(closing byte) (done bool, err error) {
	if err := p.ws(); err != nil {
		return false, err
	}
	if p.i >= len(p.b) {
		return false, p.errorf("unexpected end")
	}
	switch p.b[p.i] {
	case ',':
		p.i++
		return false, p.ws()
	case closing:
		p.i++
		return true, nil
	}
	return false, p.errorf("expected ',' or '" + string(closing) + "'")
}

// str skips a string literal; escapes are checked by decodeString. Pure.
func (p *parser) str() error {
	p.i++ // opening quote
	for p.i < len(p.b) {
		switch c := p.b[p.i]; {
		case c == '\\':
			p.i += 2
		case c == '"':
			p.i++
			return nil
		case c < 0x20:
			return p.errorf("control character in string")
		default:
			p.i++
		}
	}
	return p.errorf("unterminated string")
}

// scalar parses a number or literal and validates it with encoding/json.
// Pure.
func (p *parser) scalar() (node, error) {
	start := p.i
	for p.i < len(p.b) && isScalarByte(p.b[p.i]) {
		p.i++
	}
	if p.i == start || !json.Valid(p.b[start:p.i]) {
		return node{}, p.errorf("invalid value")
	}
	return node{kind: kindScalar, start: start, end: p.i}, nil
}

// isScalarByte reports a byte that may appear in a number or literal. Pure.
func isScalarByte(c byte) bool {
	return c == '-' || c == '+' || c == '.' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// decodeString decodes a string literal (escapes included). Pure.
func decodeString(lit []byte) (string, error) {
	var s string
	if err := json.Unmarshal(lit, &s); err != nil {
		return "", fmt.Errorf("%w: bad string %s: %v", errSyntax, lit, err)
	}
	return s, nil
}
