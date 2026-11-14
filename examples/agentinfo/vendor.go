package main

import (
	"github.com/khanakia/agentcli"
	"github.com/khanakia/agentcli/builtin"
	"github.com/khanakia/agentcli/claude"
	"github.com/khanakia/agentcli/codex"
)

// vendorReport adds the L3 compat line for agents the example knows by type.
// Disk · Read.
func vendorReport(r *AgentReport, all builtin.Agents, _ options) {
	switch r.ID {
	case claude.ID:
		r.Compat = claudeCompat(r.Install)
	case codex.ID:
		r.Compat = codexCompat(all.Codex, r.Install)
	}
}

// vendorHomeReport adds the L3 paths and settings summary of one home.
// Disk · Read.
func vendorHomeReport(hr *HomeReport, all builtin.Agents, id agentcli.ID, h agentcli.Home, dir string) {
	switch id {
	case claude.ID:
		claudeHome(hr, all.Claude, h, dir)
	case codex.ID:
		codexHome(hr, all.Codex, h)
	}
}
