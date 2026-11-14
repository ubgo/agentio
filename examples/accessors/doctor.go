package main

import (
	"context"
	"fmt"
	"io"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/builtin"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
)

// untestedClaude is a version far below Claude's tested range, to show the
// warning VersionCheck gives for it.
const untestedClaude = "2.0.0"

// snippet:start doctor
// printDoctor runs the disk-only doctor checks of every home. A Fix, when
// set, is a Command the caller may offer to run; nothing here runs it.
func printDoctor(ctx context.Context, w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	for _, a := range agents {
		d, ok := agentio.DoctorOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return err
		}
		for _, home := range homes {
			for _, c := range d.Checks(ctx, home) {
				line := fmt.Sprintf("%s %s %-5s %-15s %s", a.ID(), home.Name(), c.Status, c.ID, tidy(h, c.Message))
				if c.Fix != nil {
					line += " (fix: " + c.Fix.String() + ")"
				}
				fmt.Fprintln(w, line)
			}
		}
	}
	return nil
}

// snippet:end doctor

// snippet:start versioncheck
// printVersionCheck compares the installed version (read from disk, never
// by running the binary) with each vendor module's tested range.
func printVersionCheck(w io.Writer, agents builtin.Agents) {
	if inst, err := agents.Claude.Install(); err == nil {
		c := agentio.VersionCheck(claude.Compat(), inst.Version)
		fmt.Fprintf(w, "claude %s %s\n", c.Status, c.Message)
	}
	if inst, err := agents.Codex.Install(); err == nil {
		c := agentio.VersionCheck(codex.Compat(), inst.Version)
		fmt.Fprintf(w, "codex  %s %s\n", c.Status, c.Message)
	}
	old := agentio.VersionCheck(claude.Compat(), agentio.MustVersion(untestedClaude))
	fmt.Fprintf(w, "claude %s %s\n", old.Status, old.Message)
	unknown := agentio.VersionCheck(claude.Compat(), agentio.Version{})
	fmt.Fprintf(w, "claude %s %s\n", unknown.Status, unknown.Message)
}

// snippet:end versioncheck
