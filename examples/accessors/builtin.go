package main

import (
	"fmt"
	"io"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/builtin"
	"github.com/ubgo/agentio/codex"
)

// snippet:start build
// buildAgents builds Claude Code and Codex on h. Nothing is read from disk
// yet; the typed fields keep vendor calls compile-checked and All gives the
// neutral views for loops.
func buildAgents(h agentio.Host) (builtin.Agents, error) {
	return builtin.New(h)
}

// snippet:end build

// snippet:start byid
// findByID looks an agent up by the ID a user typed or a config file named;
// an unknown ID is false, never a panic or a default agent.
func findByID(w io.Writer, agents builtin.Agents) {
	for _, id := range []agentio.ID{codex.ID, "aider"} {
		a, ok := agents.ByID(id)
		if !ok {
			fmt.Fprintf(w, "%s: no such agent\n", id)
			continue
		}
		fmt.Fprintf(w, "%s: %s\n", id, a.Name())
	}
}

// snippet:end byid

// snippet:start custom
// addCustom passes a config-defined agent to builtin.New. It joins All after
// the built-ins; reusing a built-in ID is an error, never an override.
func addCustom(w io.Writer, h agentio.Host) error {
	aider := agentio.Spec{ID: "aider", Name: "Aider", Binary: "aider", NewArgs: []string{"{args}"}}
	agents, err := builtin.New(h, aider)
	if err != nil {
		return err
	}
	for _, a := range agents.All() {
		fmt.Fprintf(w, "%s (%s)\n", a.ID(), a.Name())
	}
	clash := agentio.Spec{ID: codex.ID, Name: "My Codex", Binary: "codex"}
	if _, err := builtin.New(h, clash); err != nil {
		fmt.Fprintln(w, "refused:", err)
	}
	return nil
}

// snippet:end custom
