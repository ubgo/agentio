// Command conversations lists resumable conversations, reads their turns
// incrementally from a byte offset, reads typed prompt history, checks
// whether a folder has history and moves history after a folder rename and
// between two homes, for every agent that supports each step. It works on
// temporary copies of the recorded fixture homes.
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

// Folders recorded in the fixtures (conversation cwd values).
const (
	recordedDir = "/work/proj13"
	codexDir    = "/work/proj"
	renamedDir  = "/work/proj13-renamed"
	emptyDir    = "/work/never-used"
	listLimit   = 5
	promptLimit = 3
)

func main() {
	if err := runMain(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "conversations:", err)
		os.Exit(1)
	}
}

// runMain copies the fixtures to a temp dir and runs every step.
func runMain(w io.Writer) error {
	tmp, err := os.MkdirTemp("", "agentio-conversations-")
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
		{"conversations", func() error { return listConversations(ctx, w, h, agents) }},
		{"turns", func() error { return readTurns(ctx, w, agents) }},
		{"prompt history", func() error { return readPrompts(w, agents) }},
		{"check and move", func() error { return checkAndMove(w, h, agents) }},
	}
	for _, s := range steps {
		fmt.Fprintf(w, "== %s ==\n", s.title)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.title, err)
		}
	}
	return nil
}

// snippet:start list
// listConversations lists the newest conversations of every home, newest
// first, then looks one up by ID. Disk · Read; transcripts are never read
// whole (head for the folder, tail for the title).
func listConversations(ctx context.Context, w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	for _, a := range agents {
		cl, ok := agentio.ConversationsOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return err
		}
		for _, home := range homes {
			convs, err := cl.Conversations(ctx, home, agentio.ConversationQuery{Limit: listLimit})
			if err != nil {
				return err
			}
			for _, c := range convs {
				fmt.Fprintf(w, "%s %s %s %s %q\n", a.ID(), home.Name(), c.ID, c.Dir, c.Title)
			}
			if len(convs) == 0 {
				continue
			}
			one, err := cl.Conversation(home, convs[0].ID)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "  by id: %s\n", tidy(h, one.Path))
		}
	}
	return nil
}

// snippet:end list

// newestConversation is the newest conversation of the default home; false
// when there is none. Disk · Read.
func newestConversation(ctx context.Context, a agentio.Agent) (agentio.Conversation, bool, error) {
	cl, ok := agentio.ConversationsOf(a)
	if !ok {
		return agentio.Conversation{}, false, nil
	}
	convs, err := cl.Conversations(ctx, a.DefaultHome(), agentio.ConversationQuery{Limit: 1})
	if err != nil || len(convs) == 0 {
		return agentio.Conversation{}, false, err
	}
	return convs[0], true, nil
}

// snippet:start turns
// readTurns reads a conversation's turns from offset 0, keeps the returned
// offset, and reads again from it: the second read sees only lines written
// since, so a watcher re-reads nothing. The offset never moves past an
// incomplete trailing line.
func readTurns(ctx context.Context, w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		tr, ok := agentio.TurnsOf(a)
		if !ok {
			continue
		}
		conv, found, err := newestConversation(ctx, a)
		if err != nil || !found {
			return err
		}
		var tokens agentio.TokenCounts
		count := 0
		off, err := tr.Turns(a.DefaultHome(), conv.ID, 0, func(t agentio.Turn) error {
			count++
			if t.Tokens != nil {
				tokens = tokens.Add(*t.Tokens)
			}
			if t.Text != "" {
				fmt.Fprintf(w, "%s %-9s model=%q text=%q\n", a.ID(), t.Role, t.Model, t.Text)
			}
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s: %d turns, offset %d, tokens in=%d cached=%d out=%d\n", a.ID(), count, off, tokens.Input, tokens.CachedInput, tokens.Output)
		again := 0
		if _, err := tr.Turns(a.DefaultHome(), conv.ID, off, func(agentio.Turn) error { again++; return nil }); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s: %d new turns after offset %d\n", a.ID(), again, off)
	}
	return nil
}

// snippet:end turns

// snippet:start prompts
// readPrompts reads the newest typed prompts, scanning history from its
// tail; Dir narrows to one folder for agents that record it.
func readPrompts(w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		pr, ok := agentio.PromptHistoryOf(a)
		if !ok {
			continue
		}
		entries, err := pr.PromptHistory(a.DefaultHome(), agentio.PromptQuery{Limit: promptLimit})
		if err != nil {
			return err
		}
		for _, e := range entries {
			fmt.Fprintf(w, "%s %s dir=%q %q\n", a.ID(), e.Time.UTC().Format("2006-01-02 15:04"), e.Dir, e.Text)
		}
	}
	return nil
}

// snippet:end prompts

// snippet:start move
// checkAndMove asks whether a folder has history, re-keys Claude's history
// after the folder was renamed, and plans a move of one folder's
// conversations to another home. Moves are writes: each returns its backup.
// Codex declares why it cannot move.
func checkAndMove(w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	for _, a := range agents {
		if hc, ok := agentio.HistoryCheckerOf(a); ok {
			for _, dir := range []string{recordedDir, codexDir, emptyDir} {
				has, err := hc.HasHistory(a.DefaultHome(), dir)
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "%s has history in %s: %t\n", a.ID(), dir, has)
			}
		}
		hm, ok := agentio.HistoryMoverOf(a)
		if !ok {
			reason, _ := agentio.Unsupported(a, agentio.CapHistoryMover)
			fmt.Fprintf(w, "%s cannot move history: %.60s...\n", a.ID(), reason)
			continue
		}
		b, err := hm.MoveHistory(a.DefaultHome(), recordedDir, renamedDir)
		if err != nil {
			return err
		}
		for _, e := range b.Entries {
			fmt.Fprintf(w, "%s moved %s -> %s, backup %s\n", a.ID(), recordedDir, renamedDir, tidy(h, e.Copy))
		}
		if err := planMove(w, h, a); err != nil {
			return err
		}
	}
	return nil
}

// planMove plans moving the renamed folder's conversations from the
// default home to the agent's second home and prints the plan; applying it
// is one more call (ApplyMove) on the same door.
func planMove(w io.Writer, h agentio.Host, a agentio.Agent) error {
	cm, ok := agentio.ConversationMoverOf(a)
	if !ok {
		return nil
	}
	homes, err := a.Homes()
	if err != nil || len(homes) < 2 {
		return err
	}
	plan, err := cm.PlanMove(agentio.MoveRequest{From: homes[0], To: homes[1], Dir: renamedDir})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s plan %s -> %s: %d conversations, %d items, blockers %v\n", a.ID(), plan.From.Name(), plan.To.Name(), len(plan.Conversations), len(plan.Items), plan.Blockers)
	b, err := cm.ApplyMove(plan)
	if err != nil {
		return err
	}
	for _, e := range b.Entries {
		fmt.Fprintf(w, "%s applied, backup %s\n", a.ID(), tidy(h, e.Copy))
	}
	return nil
}

// snippet:end move
