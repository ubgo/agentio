package claude

import "github.com/ubgo/agentio/frontmatter"

// ParseFrontmatter splits a SKILL.md / command / subagent / output style /
// rule file into its YAML frontmatter and body, using the stdlib-only root
// frontmatter parser (no YAML dependency for Claude-only callers).
// cost: Pure.
func ParseFrontmatter(b []byte) (frontmatter.Doc, []byte, error) { return frontmatter.Parse(b) }
