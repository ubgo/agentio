package codex

import (
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestClassifyMCPNeverSession: no `codex mcp …` row is a session, so a
// reaper never stops one. `mcp serve` and a bare `mcp` (older Codex ran
// its MCP server so) are MCP servers; the config subcommands are short
// commands; `mcp-server` stays an MCP server; the CLI and exec stay
// sessions.
func TestClassifyMCPNeverSession(t *testing.T) {
	t.Parallel()
	a := New(agenttest.NewHost(t))
	for _, tc := range []struct {
		argv    []string
		kind    agentio.ProcessKind
		session bool
	}{
		{[]string{"codex", "mcp"}, agentio.ProcessMCPServer, false},
		{[]string{"codex", "mcp", "serve"}, agentio.ProcessMCPServer, false},
		{[]string{"/opt/homebrew/bin/codex", "-c", "model=o3", "mcp", "serve"}, agentio.ProcessMCPServer, false},
		{[]string{"codex", "mcp", "list"}, agentio.ProcessCommand, false},
		{[]string{"codex", "mcp", "--json", "list"}, agentio.ProcessCommand, false},
		{[]string{"codex", "mcp", "add", "alice-docs", "--", "npx", "docs-mcp"}, agentio.ProcessCommand, false},
		{[]string{"codex", "mcp", "remove", "alice-docs"}, agentio.ProcessCommand, false},
		{[]string{"codex", "mcp-server"}, agentio.ProcessMCPServer, false},
		{[]string{"codex"}, agentio.ProcessCLI, true},
		{[]string{"codex", "exec", "fix it"}, agentio.ProcessExec, true},
	} {
		info, ok := a.ClassifyProcess(agentio.Process{PID: 1, Argv: tc.argv})
		if !ok || info.Kind != tc.kind || info.Session != tc.session {
			t.Errorf("%q = %+v %v, want %s session=%v", tc.argv, info, ok, tc.kind, tc.session)
		}
		if tc.session {
			continue
		}
		plan, err := a.StopPlan(agentio.Process{PID: 1, Argv: tc.argv}, nil, agentio.ProcessSession{})
		if err != nil || plan.Refuse == "" || len(plan.Targets) != 0 {
			t.Errorf("%q stop plan = %+v %v, want a refusal", tc.argv, plan, err)
		}
	}
}
