package claude

import (
	"testing"

	"github.com/ubgo/agentio"
)

// TestClassifyMCPNeverSession: no `claude mcp …` row is a session: `mcp
// serve` is an MCP server, every other mcp subcommand a short command,
// and both are refused by the stop plan.
func TestClassifyMCPNeverSession(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		argv []string
		kind agentio.ProcessKind
	}{
		{[]string{"claude", "mcp", "serve"}, agentio.ProcessMCPServer},
		{[]string{"claude", "mcp", "list"}, agentio.ProcessCommand},
		{[]string{"claude", "mcp", "get", "alice-docs"}, agentio.ProcessCommand},
		{[]string{"claude", "mcp", "add", "alice-docs", "--", "npx", "docs-mcp"}, agentio.ProcessCommand},
		{[]string{"claude", "mcp", "remove", "alice-docs"}, agentio.ProcessCommand},
		{[]string{"claude", "mcp"}, agentio.ProcessCommand},
	} {
		info, ok := ClassifyProcess(agentio.Process{PID: 1, Argv: tc.argv})
		if !ok || info.Kind != tc.kind || info.Session {
			t.Errorf("%q = %+v %v, want %s, not a session", tc.argv, info, ok, tc.kind)
		}
	}
	if info, _ := ClassifyProcess(agentio.Process{PID: 1, Argv: []string{"claude", "--resume", "x"}}); !info.Session || info.Kind != agentio.ProcessCLI {
		t.Errorf("a conversation stays a session: %+v", info)
	}
}
