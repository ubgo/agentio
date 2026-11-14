// Command capmatrix prints which shared capability each built-in agent has
// (Claude Code, Codex, Gemini CLI, pi), straight from agentio.Supported,
// and the reason each agent gives for a capability it lacks
// (agentio.Unsupported). Nothing is read from disk: capability support is
// a property of the agent's type and data, not of a machine.
//
//	cd examples && go run ./capmatrix
package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/builtin"
)

// Cell marks.
const (
	markYes = "yes"
	markNo  = "-"
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "capmatrix:", err)
		os.Exit(1)
	}
}

// run prints the matrix and the reasons. Pure.
func run(w io.Writer) error {
	all, err := builtin.New(agentio.Host{UserHome: "/home/user", User: "user"})
	if err != nil {
		return err
	}
	matrix(w, all.All())
	return nil
}

// snippet:start matrix
// matrix prints one row per capability and one column per agent, then
// every missing capability with the agent's own reason.
func matrix(w io.Writer, agents []agentio.Agent) {
	header := []string{fmt.Sprintf("%-19s", "capability")}
	for _, a := range agents {
		header = append(header, fmt.Sprintf("%-7s", a.ID()))
	}
	fmt.Fprintln(w, strings.TrimRight(strings.Join(header, " "), " "))
	var reasons []string
	for _, c := range agentio.CapabilityValues() {
		row := []string{fmt.Sprintf("%-19s", c)}
		for _, a := range agents {
			mark := markNo
			if slices.Contains(agentio.Supported(a), c) {
				mark = markYes
			} else if r, ok := agentio.Unsupported(a, c); ok {
				reasons = append(reasons, fmt.Sprintf("%s %s: %s", a.ID(), c, r))
			}
			row = append(row, fmt.Sprintf("%-7s", mark))
		}
		fmt.Fprintln(w, strings.TrimRight(strings.Join(row, " "), " "))
	}
	fmt.Fprintln(w, "why not:")
	for _, r := range reasons {
		fmt.Fprintln(w, "  "+r)
	}
}

// snippet:end matrix
