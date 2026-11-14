package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RuleDecision is a prefix_rule decision (execpolicy/src/decision.rs).
type RuleDecision string

// RuleDecision values.
const (
	DecisionAllow     RuleDecision = "allow"
	DecisionPrompt    RuleDecision = "prompt"
	DecisionForbidden RuleDecision = "forbidden"
)

// IsKnown reports a known decision. Pure.
func (d RuleDecision) IsKnown() bool {
	return d == DecisionAllow || d == DecisionPrompt || d == DecisionForbidden
}

// PrefixRule is one `prefix_rule(pattern=[…], decision="…")` call.
type PrefixRule struct {
	Pattern  []string
	Decision RuleDecision
	File     string
	Line     int
}

// RuleParseIssue is a statement the parser did not understand: reported,
// never executed (execpolicy files are Starlark).
type RuleParseIssue struct {
	File   string
	Line   int
	Detail string
}

// execpolicy spelling.
const (
	rulesExt        = ".rules"
	ruleCallPrefix  = "prefix_rule("
	argPattern      = "pattern"
	argDecision     = "decision"
	commentByte     = '#'
	issueNotPrefix  = "statement is not a prefix_rule call"
	issueBadArgs    = "prefix_rule arguments did not parse"
	issueAltPattern = "pattern alternatives (nested lists) are not modelled"
)

// Rules parses every *.rules file in <home>/rules and <dir>/.codex/rules
// (project). Only prefix_rule calls are understood; anything else is
// returned as an issue, never evaluated. Disk · Read.
func (a *Agent) Rules(h Home, dir string) ([]PrefixRule, []RuleParseIssue, error) {
	dirs := []string{a.Paths(h).Rules}
	if dir != "" {
		dirs = append(dirs, filepath.Join(dir, dirProjectCodex, dirRules))
	}
	var rules []PrefixRule
	var issues []RuleParseIssue
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("codex: read %s: %w", d, err)
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), rulesExt) {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, n := range names {
			path := filepath.Join(d, n)
			b, err := readFile(path)
			if err != nil {
				return nil, nil, fmt.Errorf("codex: read %s: %w", path, err)
			}
			r, is := ParseRules(b, path)
			rules = append(rules, r...)
			issues = append(issues, is...)
		}
	}
	return rules, issues, nil
}

// ParseRules parses execpolicy source. Pure.
func ParseRules(src []byte, file string) ([]PrefixRule, []RuleParseIssue) {
	var rules []PrefixRule
	var issues []RuleParseIssue
	for _, st := range splitStatements(trimBOM(src)) {
		text := strings.TrimSpace(st.text)
		if text == "" {
			continue
		}
		if !strings.HasPrefix(text, ruleCallPrefix) || !strings.HasSuffix(text, ")") {
			issues = append(issues, RuleParseIssue{File: file, Line: st.line, Detail: issueNotPrefix})
			continue
		}
		args := text[len(ruleCallPrefix) : len(text)-1]
		r, detail := parseRuleArgs(args)
		if detail != "" {
			issues = append(issues, RuleParseIssue{File: file, Line: st.line, Detail: detail})
			continue
		}
		r.File, r.Line = file, st.line
		rules = append(rules, r)
	}
	return rules, issues
}

type statement struct {
	text string
	line int
}

// splitStatements splits source into top-level statements: a newline ends
// a statement only outside brackets and strings; comments are dropped.
func splitStatements(src []byte) []statement {
	var out []statement
	var cur bytes.Buffer
	depth, line, start := 0, 1, 1
	var quote byte
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case quote != 0:
			cur.WriteByte(c)
			if c == '\\' && i+1 < len(src) {
				i++
				cur.WriteByte(src[i])
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
			cur.WriteByte(c)
		case c == commentByte:
			for i < len(src) && src[i] != '\n' {
				i++
			}
			i--
		case c == '(' || c == '[' || c == '{':
			depth++
			cur.WriteByte(c)
		case c == ')' || c == ']' || c == '}':
			depth--
			cur.WriteByte(c)
		case c == '\n' && depth <= 0:
			if strings.TrimSpace(cur.String()) != "" {
				out = append(out, statement{text: cur.String(), line: start})
			}
			cur.Reset()
			start = line + 1
		case c == '\n':
			cur.WriteByte(' ')
		default:
			if strings.TrimSpace(cur.String()) == "" && c != ' ' && c != '\t' && c != '\r' {
				start = line
			}
			cur.WriteByte(c)
		}
		if c == '\n' {
			line++
		}
	}
	if strings.TrimSpace(cur.String()) != "" {
		out = append(out, statement{text: cur.String(), line: start})
	}
	return out
}

// parseRuleArgs reads `pattern=[…], decision="…"` (either order). The
// values are JSON-compatible literals (double-quoted strings, lists).
func parseRuleArgs(args string) (PrefixRule, string) {
	var r PrefixRule
	for _, kv := range splitTopLevel(args) {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return PrefixRule{}, issueBadArgs
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		v = singleToDoubleQuotes(v)
		switch k {
		case argPattern:
			var parts []json.RawMessage
			if json.Unmarshal([]byte(v), &parts) != nil {
				return PrefixRule{}, issueBadArgs
			}
			for _, p := range parts {
				var s string
				if json.Unmarshal(p, &s) != nil {
					return PrefixRule{}, issueAltPattern
				}
				r.Pattern = append(r.Pattern, s)
			}
		case argDecision:
			var s string
			if json.Unmarshal([]byte(v), &s) != nil {
				return PrefixRule{}, issueBadArgs
			}
			r.Decision = RuleDecision(s)
		}
	}
	if len(r.Pattern) == 0 {
		return PrefixRule{}, issueBadArgs
	}
	return r, ""
}

// splitTopLevel splits on commas outside brackets and strings.
func splitTopLevel(s string) []string {
	var out []string
	depth, last := 0, 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '[' || c == '(' || c == '{':
			depth++
		case c == ']' || c == ')' || c == '}':
			depth--
		case c == ',' && depth == 0:
			out = append(out, s[last:i])
			last = i + 1
		}
	}
	if strings.TrimSpace(s[last:]) != "" {
		out = append(out, s[last:])
	}
	return out
}

// singleToDoubleQuotes rewrites 'x' Starlark strings to "x" so the value
// decodes as JSON (no escapes inside single-quoted parts are rewritten
// beyond the quotes themselves).
func singleToDoubleQuotes(v string) string {
	if !strings.Contains(v, "'") {
		return v
	}
	var sb strings.Builder
	inDouble, inSingle := false, false
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '\\' && i+1 < len(v):
			sb.WriteByte(c)
			i++
			sb.WriteByte(v[i])
			continue
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == '\'' && !inDouble:
			inSingle = !inSingle
			sb.WriteByte('"')
			continue
		case c == '"' && inSingle:
			sb.WriteString(`\"`)
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}
