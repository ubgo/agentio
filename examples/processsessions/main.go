// Command processsessions reads a process table once and, for each agent
// row, shows its resource columns, the conversation agentio ties it to and
// how sure that join is, the plan for stopping it without losing the
// conversation, its launcher chain, and the start-time re-check a caller
// runs right before it signals. agentio itself never signals: the plan is
// data. The process table is a fixed list and the homes are temporary
// copies of the recorded fixtures.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
)

func main() {
	if err := runMain(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "processsessions:", err)
		os.Exit(1)
	}
}

// runMain copies the fixtures to a temp dir and runs every step.
func runMain(w io.Writer) error {
	tmp, err := os.MkdirTemp("", "agentio-processsessions-")
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

// run prints each step for Claude and Codex.
func run(ctx context.Context, w io.Writer, h agentio.Host) error {
	agents := []agentio.Agent{claude.New(h).Neutral(), codex.New(h).Neutral()}
	table, err := h.Processes.Processes(ctx)
	if err != nil {
		return err
	}
	steps := []struct {
		title string
		fn    func() error
	}{
		{"table", func() error { showTable(w, agents, table); return nil }},
		{"sessions and stop plans", func() error { return sessionsAndPlans(ctx, w, agents, table) }},
		{"ancestors", func() error { showAncestors(w, table); return nil }},
		{"kill safety", func() error { return recheck(ctx, w, h, table) }},
	}
	for _, s := range steps {
		fmt.Fprintf(w, "== %s ==\n", s.title)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.title, err)
		}
	}
	return nil
}

// snippet:start table
// showTable prints the agent rows of one table read with the resource
// columns the same read fills: RSS, controlling terminal, CPU time so far,
// and the unsplit command line.
func showTable(w io.Writer, agents []agentio.Agent, table []agentio.Process) {
	for _, p := range table {
		info, ok := agentio.ClassifyProcess(agents, p)
		if !ok {
			continue
		}
		tty := p.TTY
		if tty == "" {
			tty = "-"
		}
		fmt.Fprintf(w, "pid %d %s %s session=%t rss=%dMiB tty=%s cpu=%s cmd=%q\n",
			p.PID, info.Agent, info.Kind, info.Session, p.RSS/mib, tty, p.CPUTime, p.CommandString())
	}
}

// snippet:end table

// snippet:start sessions
// sessionsAndPlans joins each agent row to its conversation and asks the
// agent how to stop it. Only a JoinExact session is evidence about the
// process; argv and inferred joins are labels to show.
func sessionsAndPlans(ctx context.Context, w io.Writer, agents []agentio.Agent, table []agentio.Process) error {
	for _, p := range table {
		info, ok := agentio.ClassifyProcess(agents, p)
		if !ok {
			continue
		}
		s, found, err := agentio.FindProcessSession(ctx, agents, p)
		if err != nil {
			return err
		}
		if found {
			fmt.Fprintf(w, "pid %d: %s home %s conversation %.8s confidence %s status %s kind %q\n",
				p.PID, s.Agent, s.Home.Name(), s.ConversationID, s.Confidence, s.Status, s.Kind)
		} else {
			fmt.Fprintf(w, "pid %d: %s %s, no session of its own\n", p.PID, info.Agent, info.Kind)
		}
		planner, ok := agentio.StopPlannerOf(agentByID(agents, info.Agent))
		if !ok {
			continue
		}
		plan, err := planner.StopPlan(p, table, s)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "  stop: %s\n", describePlan(plan))
	}
	return nil
}

// describePlan renders a StopPlan on one line.
func describePlan(plan agentio.StopPlan) string {
	if plan.Refuse != "" {
		return "refused: " + plan.Refuse
	}
	out := fmt.Sprintf("targets %v, %s then wait %s, force %t", plan.Targets, plan.Graceful, plan.Grace, plan.Force)
	if plan.Command != nil {
		out += ", run instead: " + plan.Command.Path + " " + strings.Join(plan.Command.Args, " ")
	}
	if line := plan.ResumeLine(); line != "" {
		out += ", resume: " + line
	}
	return out
}

// snippet:end sessions

// agentByID returns the agent with id; nil when none.
func agentByID(agents []agentio.Agent, id agentio.ID) agentio.Agent {
	for _, a := range agents {
		if a.ID() == id {
			return a
		}
	}
	return nil
}

// snippet:start ancestors
// showAncestors walks the busy Claude session's parents, nearest first.
// Which of them is a terminal, tmux or an IDE is the caller's knowledge.
func showAncestors(w io.Writer, table []agentio.Process) {
	const maxHops = 4
	for _, a := range agentio.Ancestors(table, claudeBusyPID, maxHops) {
		fmt.Fprintf(w, "pid %d <- %d %s\n", claudeBusyPID, a.PID, a.CommandString())
	}
}

// snippet:end ancestors

// snippet:start safety
// recheck is the step a caller runs right before it signals its targets:
// the start time it saw in the table read must still match, so a pid that
// ended and was reused by another process is never signalled. One
// ProcessesAlive call re-checks every target with one table read. Act
// only on LivenessAlive; LivenessUnknown is a reason to skip, never to
// force.
func recheck(ctx context.Context, w io.Writer, h agentio.Host, table []agentio.Process) error {
	seen := map[int]time.Time{}
	for _, p := range table {
		seen[p.PID] = p.Start
	}
	// The second ref is a start time recorded an hour before the pid's
	// current process started: the pid was reused.
	refs := []agentio.ProcessRef{
		{PID: codexChildPID, Start: seen[codexChildPID]},
		{PID: codexChildPID, Start: seen[codexChildPID].Add(-time.Hour)},
	}
	lives, err := h.ProcessesAlive(ctx, refs)
	if err != nil {
		return err
	}
	for i, r := range refs {
		action := "skip"
		if lives[i] == agentio.LivenessAlive {
			action = "signal"
		}
		fmt.Fprintf(w, "pid %d started %s: %s, %s\n", r.PID, r.Start.UTC().Format(time.TimeOnly), lives[i], action)
	}
	return nil
}

// snippet:end safety
