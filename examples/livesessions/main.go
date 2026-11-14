// Command livesessions lists the live sessions of every home, classifies
// rows of the process table as agent processes, and joins a pid to its
// agent, home and conversation. The process table is a fixed list and the
// homes are temporary copies of the recorded fixtures.
package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
)

func main() {
	if err := runMain(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "livesessions:", err)
		os.Exit(1)
	}
}

// runMain copies the fixtures to a temp dir and runs every step.
func runMain(w io.Writer) error {
	tmp, err := os.MkdirTemp("", "agentio-livesessions-")
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
	steps := []struct {
		title string
		fn    func() error
	}{
		{"live sessions", func() error { return listLive(w, agents) }},
		{"classify", func() error { return classify(ctx, w, h, agents) }},
		{"find by pid", func() error { return findByPID(w, agents) }},
	}
	for _, s := range steps {
		fmt.Fprintf(w, "== %s ==\n", s.title)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.title, err)
		}
	}
	return nil
}

// snippet:start live
// listLive lists every home's live sessions. Alive is the checked answer:
// the session's process is in the table with a matching start time; a
// registry entry whose process is gone stays listed with Alive false.
func listLive(w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		ls, ok := agentio.LiveSessionsOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return err
		}
		for _, home := range homes {
			sessions, err := ls.LiveSessions(home)
			if err != nil {
				return err
			}
			for _, s := range sessions {
				fmt.Fprintf(w, "%s %s pid=%d conv=%.8s dir=%s kind=%q status=%s alive=%t\n",
					a.ID(), home.Name(), s.PID, s.ConversationID, s.Dir, s.Kind, s.Status, s.Alive)
			}
			if len(sessions) > 0 {
				live, err := ls.IsLive(home, sessions[0].ConversationID)
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "  IsLive(%.8s) = %t\n", sessions[0].ConversationID, live)
			}
		}
	}
	return nil
}

// snippet:end live

// snippet:start classify
// classify reads the process table once and asks the agents, in order,
// which one each row belongs to. Classification is Pure: it runs on rows
// the caller already has.
func classify(ctx context.Context, w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	rows, err := h.Processes.Processes(ctx)
	if err != nil {
		return err
	}
	for _, p := range rows {
		info, ok := agentio.ClassifyProcess(agents, p)
		if !ok {
			fmt.Fprintf(w, "pid %d %s: not an agent\n", p.PID, p.Argv[0])
			continue
		}
		fmt.Fprintf(w, "pid %d: %s %s session=%t\n", p.PID, info.Agent, info.Kind, info.Session)
	}
	// One agent alone: its own classifier.
	if pc, ok := agentio.ProcessClassifierOf(agents[0]); ok {
		_, mine := pc.ClassifyProcess(rows[len(rows)-1])
		fmt.Fprintf(w, "%s claims the last row: %t\n", agents[0].ID(), mine)
	}
	return nil
}

// snippet:end classify

// snippet:start find
// findByPID joins a pid to its agent, home and conversation by looping
// every home's live sessions. A Codex pid never matches: Codex writer
// locks do not name the process that holds them.
func findByPID(w io.Writer, agents []agentio.Agent) error {
	for _, pid := range []int{claudeBusyPID, claudeWorkPID, codexExecPID} {
		a, s, found, err := agentio.FindLiveSession(agents, pid)
		if err != nil {
			return err
		}
		if !found {
			fmt.Fprintf(w, "pid %d: no live session found\n", pid)
			continue
		}
		fmt.Fprintf(w, "pid %d: %s home %s conversation %s in %s\n", pid, a.ID(), s.Home.Name(), s.ConversationID, s.Dir)
	}
	return nil
}

// snippet:end find
