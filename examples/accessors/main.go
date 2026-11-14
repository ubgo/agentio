// Command accessors walks every shared capability of the built-in agents
// through the typed accessors (agentio.InstallerOf, agentio.ConfigOf, ...)
// on recorded fixture homes: which capabilities each agent has, why one is
// missing, what each costs, the install, version and identity, config, MCP
// servers, instruction files, skills, plugins, models and doctor checks.
//
// It reads only the fixtures in this repository and a temporary fake
// install; it never runs a real binary, opens the keychain or uses the
// network.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ubgo/agentio"
)

// projectDir is the fixture project folder, relative to the user home; the
// per-project reads (config layers, MCP, instructions, skills) look at it.
const projectDir = "project"

func main() {
	if err := runMain(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "accessors:", err)
		os.Exit(1)
	}
}

// runMain builds the example machine in a temporary folder and runs the
// walk. Disk · Write (temp dir), then Disk · Read.
func runMain(w io.Writer) error {
	tmp, err := os.MkdirTemp("", "agentio-accessors-")
	if err != nil {
		return err
	}
	// The temp dir holds only copies; removing it is cleanup.
	defer func() { _ = os.RemoveAll(tmp) }()
	h, err := setup(tmp)
	if err != nil {
		return err
	}
	return run(context.Background(), w, h)
}

// run prints every section for the agents on h. Each section is one guide
// step; the functions below are the snippets the guides embed.
func run(ctx context.Context, w io.Writer, h agentio.Host) error {
	agents, err := buildAgents(h)
	if err != nil {
		return err
	}
	all := agents.All()
	dir := filepath.Join(h.UserHome, projectDir)
	sections := []struct {
		title string
		fn    func() error
	}{
		{"homes", func() error { return listHomes(w, h, all) }},
		{"supported", func() error { listSupported(w, all); return nil }},
		{"unsupported", func() error { listUnsupported(w, all); return nil }},
		{"cost", func() error { listCosts(w); return nil }},
		{"install", func() error { printInstalls(w, h, all); return nil }},
		{"binary version", func() error { printBinaryVersions(ctx, w, all); return nil }},
		{"identity", func() error { return printIdentities(w, h, all) }},
		{"config", func() error { return printConfig(w, h, all, dir) }},
		{"mcp", func() error { return printMCP(w, all, dir) }},
		{"instructions", func() error { return printInstructions(w, h, all, dir) }},
		{"skills and plugins", func() error { return printSkillsAndPlugins(w, all, dir) }},
		{"models", func() error { return printModels(w, all) }},
		{"doctor", func() error { return printDoctor(ctx, w, h, all) }},
		{"version check", func() error { printVersionCheck(w, agents); return nil }},
		{"by id", func() error { findByID(w, agents); return nil }},
		{"custom", func() error { return addCustom(w, h) }},
	}
	for _, s := range sections {
		fmt.Fprintf(w, "== %s ==\n", s.title)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.title, err)
		}
	}
	return nil
}

// snippet:start loop
// listHomes loops over every agent and every home with no type assertion:
// Homes is on the Agent interface, cached usage is reached through its
// typed accessor and skipped for an agent that does not have it.
func listHomes(w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	for _, a := range agents {
		homes, err := a.Homes()
		if err != nil {
			return err
		}
		for _, home := range homes {
			line := fmt.Sprintf("%s %s %s", a.ID(), home.Name(), tidy(h, home.Dir()))
			if cu, ok := agentio.CachedUsageOf(a); ok {
				u, found, err := cu.CachedUsage(home)
				if err != nil {
					return err
				}
				if found {
					line += fmt.Sprintf(" (cached usage %.0f%%)", u.MaxPercent())
				}
			}
			fmt.Fprintln(w, line)
		}
	}
	return nil
}

// snippet:end loop

// snippet:start supported
// listSupported prints every capability an agent has, from the one table
// Supported reads; the count makes the difference between agents visible.
func listSupported(w io.Writer, agents []agentio.Agent) {
	for _, a := range agents {
		caps := agentio.Supported(a)
		fmt.Fprintf(w, "%s: %d capabilities\n", a.ID(), len(caps))
		fmt.Fprintf(w, "  %v\n", caps)
	}
}

// snippet:end supported

// snippet:start unsupported
// listUnsupported prints, for every capability an agent lacks, the reason
// it declares; "(no reason declared)" when the agent says nothing.
func listUnsupported(w io.Writer, agents []agentio.Agent) {
	for _, a := range agents {
		have := map[agentio.Capability]bool{}
		for _, c := range agentio.Supported(a) {
			have[c] = true
		}
		for _, c := range agentio.CapabilityValues() {
			if have[c] {
				continue
			}
			reason, declared := agentio.Unsupported(a, c)
			if !declared {
				reason = "(no reason declared)"
			}
			fmt.Fprintf(w, "%s lacks %s: %s\n", a.ID(), c, reason)
		}
	}
}

// snippet:end unsupported

// snippet:start cost
// listCosts prints the worst-case cost of every capability: a caller that
// loops over homes skips anything that is not Pure or Disk unless the user
// asked for it.
func listCosts(w io.Writer) {
	for _, c := range agentio.CapabilityValues() {
		cost := c.Cost()
		line := fmt.Sprintf("%-20s %s %s", c, cost.Class, cost.Access)
		if cost.MayTouchLogin {
			line += " (may touch login)"
		}
		fmt.Fprintln(w, line)
	}
}

// snippet:end cost
