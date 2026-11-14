package claude

import "github.com/khanakia/agentcli/frontmatter"

// ParseFrontmatter splits a SKILL.md / command / subagent / output style /
// rule file into its YAML frontmatter and body, using the stdlib-only root
// frontmatter parser (OD-1 resolved without a YAML dependency). cost: Pure.
func ParseFrontmatter(b []byte) (frontmatter.Doc, []byte, error) { return frontmatter.Parse(b) }
