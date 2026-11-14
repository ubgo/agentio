// Package frontmatter parses the YAML frontmatter subset used by SKILL.md,
// agents/*.md, commands and output styles in Claude Code and Codex homes:
// `key: value` maps (nested by indentation), plain / single-quoted /
// double-quoted scalars, plain scalars continued on indented lines, block
// scalars (`|`, `>`, with `-` / `+` chomping), block lists (`- item`) and
// flow lists (`[a, "b"]`).
//
// Why a hand parser: the root module stays stdlib-only and
// these files use a small, stable subset; a full YAML dependency would be
// pulled by every Claude-only caller for a handful of keys. Anchors, tags,
// flow maps and multi-document streams are not supported; such a value is
// kept as its literal text, never an error, so one odd key does not hide
// the rest of the file.
package frontmatter

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// Kind is the shape of a Value.
type Kind uint8

// Kind values.
const (
	// KindScalar is a string (YAML scalars are kept as text; see Doc.Bool).
	KindScalar Kind = iota
	// KindList is a list of scalars.
	KindList
	// KindMap is a nested map.
	KindMap
)

// kindNames is the wire spelling of each Kind.
func kindNames() []string { return []string{"scalar", "list", "map"} }

// String returns the wire spelling. Pure.
func (k Kind) String() string {
	if int(k) < len(kindNames()) {
		return kindNames()[k]
	}
	return fmt.Sprintf("kind(%d)", uint8(k))
}

// MarshalText encodes the wire spelling. Pure.
func (k Kind) MarshalText() ([]byte, error) { return []byte(k.String()), nil }

// Value is one parsed value.
type Value struct {
	Kind Kind
	// Text is the scalar text (KindScalar).
	Text string
	// List holds list items (KindList).
	List []string
	// Map holds a nested map (KindMap).
	Map *Doc
}

// Field is one key and its value, in file order.
type Field struct {
	Key   string
	Value Value
}

// Doc is a parsed map in file order. A repeated key keeps its last value
// on Get (YAML would reject it; vendor CLIs take the last).
type Doc struct {
	Fields []Field
}

// Get returns the value of key. Pure.
func (d Doc) Get(key string) (Value, bool) {
	for i := len(d.Fields) - 1; i >= 0; i-- {
		if d.Fields[i].Key == key {
			return d.Fields[i].Value, true
		}
	}
	return Value{}, false
}

// String returns a scalar value, or "" when absent or not a scalar. Pure.
func (d Doc) String(key string) string {
	v, ok := d.Get(key)
	if !ok || v.Kind != KindScalar {
		return ""
	}
	return v.Text
}

// List returns a list value; a scalar becomes a one-item list, and a
// comma-separated scalar ("Read, Grep") is split, as vendor CLIs accept
// both spellings for tool lists. Pure.
func (d Doc) List(key string) []string {
	v, ok := d.Get(key)
	if !ok {
		return nil
	}
	switch v.Kind {
	case KindList:
		return append([]string(nil), v.List...)
	case KindScalar:
		if v.Text == "" {
			return nil
		}
		var out []string
		for _, p := range strings.Split(v.Text, listComma) {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return nil
}

// Bool parses a YAML 1.2 boolean scalar (true/false, any case). ok is
// false when absent or not a boolean. Pure.
func (d Doc) Bool(key string) (value, ok bool) {
	switch strings.ToLower(d.String(key)) {
	case yamlTrue:
		return true, true
	case yamlFalse:
		return false, true
	}
	return false, false
}

// Map returns a nested map, or an empty Doc. Pure.
func (d Doc) Map(key string) Doc {
	v, ok := d.Get(key)
	if !ok || v.Kind != KindMap || v.Map == nil {
		return Doc{}
	}
	return *v.Map
}

// Keys lists keys in file order, without duplicates. Pure.
func (d Doc) Keys() []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range d.Fields {
		if !seen[f.Key] {
			seen[f.Key] = true
			out = append(out, f.Key)
		}
	}
	return out
}

// Syntax constants.
const (
	delimiter    = "---"
	docEnd       = "..."
	bom          = "\ufeff"
	keySep       = ":"
	listItem     = "-"
	listComma    = ","
	commentStart = "#"
	flowOpen     = "["
	flowClose    = "]"
	literalBlock = '|'
	foldedBlock  = '>'
	chompStrip   = '-'
	chompKeep    = '+'
	yamlTrue     = "true"
	yamlFalse    = "false"
	tabWidth     = 2
)

// Split separates the frontmatter from the body. ok is false when data
// does not start with a "---" line or the block is never closed (then the
// whole file is body). Pure.
func Split(data []byte) (front, body []byte, ok bool) {
	s := strings.TrimPrefix(string(data), bom)
	first, rest, found := strings.Cut(s, "\n")
	if !found || strings.TrimRight(first, " \t\r") != delimiter {
		return nil, data, false
	}
	off := 0
	for off <= len(rest) {
		line, after, more := strings.Cut(rest[off:], "\n")
		t := strings.TrimRight(line, " \t\r")
		if t == delimiter || t == docEnd {
			return []byte(rest[:off]), []byte(after), true
		}
		if !more {
			break
		}
		off += len(line) + 1
	}
	return nil, data, false
}

// Parse splits data and parses its frontmatter. No frontmatter gives an
// empty Doc and the whole data as body. Pure.
func Parse(data []byte) (Doc, []byte, error) {
	front, body, ok := Split(data)
	if !ok {
		return Doc{}, data, nil
	}
	d, err := ParseYAML(front)
	return d, body, err
}

// line is one source line with its indent.
type line struct {
	indent int
	text   string // without indent and trailing spaces
	num    int
}

// ParseYAML parses a frontmatter block (without delimiters). Pure.
func ParseYAML(front []byte) (Doc, error) {
	var lines []line
	for i, raw := range strings.Split(string(bytes.ReplaceAll(front, []byte("\r\n"), []byte("\n"))), "\n") {
		expanded := strings.ReplaceAll(raw, "\t", strings.Repeat(" ", tabWidth))
		text := strings.TrimLeft(expanded, " ")
		lines = append(lines, line{indent: len(expanded) - len(text), text: strings.TrimRight(text, " "), num: i + 1})
	}
	p := &parser{lines: lines}
	d, err := p.parseMap(0)
	if err != nil {
		return Doc{}, err
	}
	return d, nil
}

// parser walks lines once.
type parser struct {
	lines []line
	pos   int
}

// skippable reports a blank or comment line.
func skippable(l line) bool { return l.text == "" || strings.HasPrefix(l.text, commentStart) }

// next returns the next meaningful line index at or after pos, or -1.
func (p *parser) next() int {
	for i := p.pos; i < len(p.lines); i++ {
		if !skippable(p.lines[i]) {
			return i
		}
	}
	return -1
}

// parseMap reads keys at exactly indent until a shallower line.
func (p *parser) parseMap(indent int) (Doc, error) {
	var d Doc
	for {
		i := p.next()
		if i < 0 || p.lines[i].indent < indent {
			return d, nil
		}
		l := p.lines[i]
		if l.indent > indent {
			return d, fmt.Errorf("frontmatter: line %d: unexpected indent", l.num)
		}
		key, rest, ok := cutKey(l.text)
		if !ok {
			return d, fmt.Errorf("frontmatter: line %d: expected key: value", l.num)
		}
		p.pos = i + 1
		v, err := p.parseValue(indent, rest)
		if err != nil {
			return d, err
		}
		d.Fields = append(d.Fields, Field{Key: key, Value: v})
	}
}

// cutKey splits "key: rest" (also "key:" at line end); quoted keys are unquoted.
func cutKey(text string) (key, rest string, ok bool) {
	if strings.HasPrefix(text, listItem+" ") || text == listItem {
		return "", "", false
	}
	idx := -1
	for i := 0; i < len(text); i++ {
		if text[i] == keySep[0] && (i+1 == len(text) || text[i+1] == ' ') {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return "", "", false
	}
	return unquote(strings.TrimSpace(text[:idx])), strings.TrimSpace(text[idx+1:]), true
}

// parseValue reads the value of a key at keyIndent whose inline part is rest.
func (p *parser) parseValue(keyIndent int, rest string) (Value, error) {
	switch {
	case rest == "" || strings.HasPrefix(rest, commentStart):
		i := p.next()
		if i < 0 || p.lines[i].indent < keyIndent || (p.lines[i].indent == keyIndent && !isListLine(p.lines[i].text)) {
			return Value{Kind: KindScalar}, nil
		}
		if isListLine(p.lines[i].text) {
			return Value{Kind: KindList, List: p.parseList(p.lines[i].indent)}, nil
		}
		m, err := p.parseMap(p.lines[i].indent)
		if err != nil {
			return Value{}, err
		}
		return Value{Kind: KindMap, Map: &m}, nil
	case rest[0] == literalBlock || rest[0] == foldedBlock:
		return Value{Kind: KindScalar, Text: p.parseBlock(keyIndent, rest)}, nil
	case strings.HasPrefix(rest, flowOpen) && strings.HasSuffix(stripComment(rest), flowClose):
		return Value{Kind: KindList, List: parseFlowList(stripComment(rest))}, nil
	}
	return Value{Kind: KindScalar, Text: p.parseScalar(keyIndent, rest)}, nil
}

// isListLine reports a "- item" line.
func isListLine(text string) bool { return text == listItem || strings.HasPrefix(text, listItem+" ") }

// parseList reads "- item" lines at indent.
func (p *parser) parseList(indent int) []string {
	var out []string
	for {
		i := p.next()
		if i < 0 || p.lines[i].indent != indent || !isListLine(p.lines[i].text) {
			return out
		}
		p.pos = i + 1
		item := strings.TrimSpace(strings.TrimPrefix(p.lines[i].text, listItem))
		out = append(out, p.parseScalar(indent, item))
	}
}

// parseScalar unquotes an inline scalar; a plain scalar continues on
// following lines indented deeper than its key, folded with spaces.
func (p *parser) parseScalar(keyIndent int, rest string) string {
	if rest != "" && (rest[0] == '"' || rest[0] == '\'') {
		return unquote(stripComment(rest))
	}
	parts := []string{stripComment(rest)}
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		if l.text == "" || l.indent <= keyIndent {
			break
		}
		parts = append(parts, stripComment(l.text))
		p.pos++
	}
	return strings.Join(parts, " ")
}

// parseBlock reads a `|` / `>` block scalar.
func (p *parser) parseBlock(keyIndent int, header string) string {
	style, chomp := header[0], byte(0)
	if len(header) > 1 && (header[1] == chompStrip || header[1] == chompKeep) {
		chomp = header[1]
	}
	var body []string
	blockIndent := -1
	for p.pos < len(p.lines) {
		l := p.lines[p.pos]
		if l.text != "" && l.indent <= keyIndent {
			break
		}
		if l.text != "" && blockIndent < 0 {
			blockIndent = l.indent
		}
		text := ""
		if l.text != "" {
			text = strings.Repeat(" ", max(0, l.indent-blockIndent)) + l.text
		}
		body = append(body, text)
		p.pos++
	}
	trailing := 0
	for len(body) > 0 && body[len(body)-1] == "" {
		body = body[:len(body)-1]
		trailing++
	}
	var s string
	if style == literalBlock {
		s = strings.Join(body, "\n")
	} else {
		s = foldLines(body)
	}
	switch chomp {
	case chompStrip:
		return s
	case chompKeep:
		return s + strings.Repeat("\n", trailing+1)
	}
	if s == "" {
		return ""
	}
	return s + "\n"
}

// foldLines joins lines with spaces; a blank line becomes a newline;
// more-indented lines keep their breaks (YAML folding rules, simplified).
func foldLines(lines []string) string {
	var b strings.Builder
	for i, l := range lines {
		switch {
		case i == 0:
		case l == "" || lines[i-1] == "":
			b.WriteString("\n")
		case strings.HasPrefix(l, " ") || strings.HasPrefix(lines[i-1], " "):
			b.WriteString("\n")
		default:
			b.WriteString(" ")
		}
		b.WriteString(l)
	}
	return b.String()
}

// parseFlowList splits "[a, 'b, c', "d"]" at top-level commas.
func parseFlowList(s string) []string {
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return nil
	}
	var out []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		switch {
		case quote != 0:
			cur.WriteByte(c)
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
			cur.WriteByte(c)
		case c == listComma[0]:
			out = append(out, unquote(strings.TrimSpace(cur.String())))
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	if last := strings.TrimSpace(cur.String()); last != "" {
		out = append(out, unquote(last))
	}
	return out
}

// stripComment removes a trailing " #comment" outside quotes.
func stripComment(s string) string {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			if i == 0 {
				quote = c
			}
		case c == commentStart[0] && i > 0 && s[i-1] == ' ':
			return strings.TrimRight(s[:i], " ")
		}
	}
	return s
}

// unquote removes YAML single or double quotes; anything else is returned
// as is. A double-quoted string that strconv cannot read keeps its inner
// text verbatim (never an error: one odd value must not hide the file).
func unquote(s string) string {
	if len(s) < 2 {
		return s
	}
	switch {
	case s[0] == '\'' && s[len(s)-1] == '\'':
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	case s[0] == '"' && s[len(s)-1] == '"':
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		return s[1 : len(s)-1]
	}
	return s
}
