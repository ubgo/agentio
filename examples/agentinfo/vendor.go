package main

import (
	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/builtin"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
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
func vendorHomeReport(hr *HomeReport, all builtin.Agents, id agentio.ID, h agentio.Home, dir string) {
	switch id {
	case claude.ID:
		claudeHome(hr, all.Claude, h, dir)
	case codex.ID:
		codexHome(hr, all.Codex, h)
	}
}
