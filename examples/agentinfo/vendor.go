package main

import (
	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/builtin"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
	"github.com/ubgo/agentio/gemini"
	"github.com/ubgo/agentio/pi"
)

// vendorReport adds the L3 compat line for agents the example knows by type.
// Disk · Read.
func vendorReport(r *AgentReport, all builtin.Agents, _ options) {
	switch r.ID {
	case claude.ID:
		r.Compat = claudeCompat(r.Install)
	case codex.ID:
		r.Compat = codexCompat(all.Codex, r.Install)
	case gemini.ID:
		r.Compat = compatReport(gemini.Compat(), gemini.TestedMin, gemini.TestedMax, r.Install)
	case pi.ID:
		r.Compat = compatReport(pi.Compat(), pi.TestedMin, pi.TestedMax, r.Install)
	}
}

// compatReport compares the on-disk version with a vendor's tested range.
// Pure.
func compatReport(c agentio.Compat, testedMin, testedMax string, inst *agentio.Install) *CompatReport {
	r := &CompatReport{TestedMin: testedMin, TestedMax: testedMax}
	if inst != nil {
		r.Installed = inst.Version.String()
		r.Status = c.Check(inst.Version)
	}
	return r
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
