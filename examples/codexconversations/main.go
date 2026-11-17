// Command codexconversations lists Codex conversations, reads their turns
// and prompt history, checks a model against the cached catalog, and
// streams a rollout file record by record, all on a copy of the checked-in
// fixture home.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/codex"
)

// defaultFixture is the checked-in Codex home, relative to the examples module.
const defaultFixture = "../codex/testdata/homes/0.162.1"

// Fixture facts.
const (
	codexDir    = ".codex"
	fixtureUser = "user"
	convNew     = "0199e1a0-0000-7000-8000-000000000001" // cli, /work/proj, 2026-10-14
	convOld     = "0199e1a0-0000-7000-8000-000000000002" // vscode, /work/other, 2026-03-01
	tailBytes   = 400                                    // RolloutTail window for the demo
)

// rolloutTimes are the mtimes the copy gets: Codex sorts by file mtime,
// and a git checkout leaves them arbitrary. A function, not a var: the repo
// allows no package-level vars.
func rolloutTimes() map[string]time.Time {
	return map[string]time.Time{
		"sessions/2026/03/01/rollout-2026-03-01T08-00-00-" + convOld + ".jsonl": time.Date(2026, 3, 1, 8, 0, 5, 0, time.UTC),
		"sessions/2026/10/14/rollout-2026-10-14T10-00-00-" + convNew + ".jsonl": time.Date(2026, 10, 14, 10, 0, 12, 0, time.UTC),
	}
}

func main() {
	fixture := flag.String("fixture", defaultFixture, "Codex fixture home to copy")
	flag.Parse()
	if err := run(os.Stdout, *fixture); err != nil {
		fmt.Fprintln(os.Stderr, "codexconversations:", err)
		os.Exit(1)
	}
}

// run copies the fixture and prints every section.
func run(w io.Writer, fixture string) error {
	userHome, err := os.MkdirTemp("", "codexconversations-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(userHome) }()
	home := filepath.Join(userHome, codexDir)
	if err := agentio.CopyTree(fixture, home); err != nil {
		return err
	}
	for rel, ts := range rolloutTimes() {
		if err := os.Chtimes(filepath.Join(home, rel), ts, ts); err != nil {
			return err
		}
	}
	host := agentio.Host{
		UserHome:  userHome,
		User:      fixtureUser,
		Env:       []string{"HOME=" + userHome},
		Now:       agenttest.FixedNow,
		Secrets:   agenttest.NewFakeSecrets(),
		Processes: &agenttest.FakeProcesses{},
	}
	a := codex.New(host)
	h := a.DefaultHome()
	steps := []func(io.Writer, *codex.Agent, codex.Home) error{
		listConversations, readTurns, readPromptHistory, listModels,
		openRollout, readTail, sumTokens,
	}
	for _, step := range steps {
		if err := step(w, a, h); err != nil {
			return err
		}
	}
	return nil
}

// snippet:start list
// listConversations lists rollouts newest first (one bounded head read
// per file, never a whole transcript), filters by folder, and finds one
// conversation by id.
func listConversations(w io.Writer, a *codex.Agent, h codex.Home) error {
	cs, err := a.Conversations(context.Background(), h, codex.ConversationQuery{Limit: 10})
	if err != nil {
		return err
	}
	for _, c := range cs {
		fmt.Fprintf(w, "%s %q dir=%s source=%s cli=%s originator=%s\n",
			c.ID, c.Title, c.Dir, c.Source, c.CLIVersion, c.Originator)
	}
	only, err := a.Conversations(context.Background(), h, codex.ConversationQuery{Dir: "/work/other"})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "in /work/other: %d\n", len(only))

	c, err := a.Conversation(h, convNew)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "by id: created=%s branch=%s\n", c.Created.UTC().Format(time.RFC3339), c.Git.Branch)
	if _, err := a.Conversation(h, "0199e1a0-0000-7000-8000-00000000ffff"); errors.Is(err, codex.ErrConversationNotFound) {
		fmt.Fprintln(w, "unknown id: ErrConversationNotFound")
	}

	titles, err := a.Titles(h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "session_index title: %q\n", titles[convNew].Name)

	has, err := a.HasHistory(h, "/work/proj")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "has history in /work/proj: %v\n", has)
	fmt.Fprintf(w, "move: %.60s…\n", a.MoveUnsupportedReason())
	return nil
}

// snippet:end list

// snippet:start turns
// readTurns streams the user and agent messages of one conversation and
// returns an offset; reading again from it yields only new turns.
func readTurns(w io.Writer, a *codex.Agent, h codex.Home) error {
	off, err := a.Turns(h, convNew, 0, func(t agentio.Turn) error {
		fmt.Fprintf(w, "turn %-9s %q", t.Role, t.Text)
		if t.Tokens != nil {
			fmt.Fprintf(w, " model=%s output=%d reasoning=%d", t.Model, t.Tokens.Output, t.Tokens.Reasoning)
		}
		fmt.Fprintln(w)
		return nil
	})
	if err != nil {
		return err
	}
	n := 0
	again, err := a.Turns(h, convNew, off, func(agentio.Turn) error { n++; return nil })
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "resumed at offset %d: %d new turns, offset unchanged=%v\n", off, n, again == off)
	return nil
}

// snippet:end turns

// snippet:start history
// readPromptHistory reads history.jsonl from the tail: a Limit reads only
// the end of the file.
func readPromptHistory(w io.Writer, a *codex.Agent, h codex.Home) error {
	es, err := a.PromptHistory(h, agentio.PromptQuery{Limit: 2})
	if err != nil {
		return err
	}
	for _, e := range es {
		fmt.Fprintf(w, "prompt %s %q session=%s\n", e.Time.UTC().Format(time.DateOnly), e.Text, e.SessionID)
	}
	one, err := a.PromptHistory(h, agentio.PromptQuery{ConversationID: convOld})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "prompts of %s: %d\n", convOld, len(one))
	return nil
}

// snippet:end history

// snippet:start models
// listModels reads models_cache.json and validates a model and effort
// before a launch.
func listModels(w io.Writer, a *codex.Agent, h codex.Home) error {
	cat, err := a.Models(h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "catalog client=%s fetched=%s\n", cat.ClientVersion, cat.FetchedAt.UTC().Format(time.DateOnly))
	for _, m := range cat.Models {
		fmt.Fprintf(w, "model %s visible=%v default-effort=%s context=%d\n", m.Slug, m.Visible, m.DefaultReasoning, m.ContextWindow)
	}
	fmt.Fprintf(w, "validate gpt-6-luna/low: %v\n", cat.Validate("gpt-6-luna", codex.EffortLow))
	fmt.Fprintf(w, "validate gpt-6-luna/xhigh unsupported: %v\n", errors.Is(cat.Validate("gpt-6-luna", codex.EffortXHigh), codex.ErrUnsupportedEffort))
	fmt.Fprintf(w, "validate nope unknown: %v\n", errors.Is(cat.Validate("nope", ""), codex.ErrUnknownModel))

	// ParseModelCatalog decodes bytes you hold; a bad model entry is
	// skipped with a warning.
	parsed, warns, err := codex.ParseModelCatalog([]byte(`{"models":[{"slug":"m1"},{"slug":7}]}`))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "parsed models=%d warnings=%d\n", len(parsed.Models), len(warns))
	return nil
}

// snippet:end models

// snippet:start open
// openRollout streams a rollout from the start, counting record kinds,
// then resumes from the saved offset. Unknown record kinds keep their raw
// line instead of failing.
func openRollout(w io.Writer, a *codex.Agent, h codex.Home) error {
	c, err := a.Conversation(h, convNew)
	if err != nil {
		return err
	}
	r, err := a.OpenRollout(c.Path)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	var kinds []codex.RecordKind
	for {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if rec.SessionMeta != nil {
			fmt.Fprintf(w, "session_meta cwd=%s version=%s unknown keys=%d\n",
				rec.SessionMeta.Cwd, rec.SessionMeta.Version(), len(rec.SessionMeta.Unknown))
		}
		if rec.TurnContext != nil {
			fmt.Fprintf(w, "turn_context model=%s approval=%s sandbox=%s\n",
				rec.TurnContext.Model, rec.TurnContext.ApprovalPolicy, rec.TurnContext.SandboxPolicy.Type)
		}
		kinds = append(kinds, rec.Kind)
	}
	fmt.Fprintf(w, "records=%d last kind=%s known=%v\n", len(kinds), kinds[len(kinds)-1], kinds[len(kinds)-1].IsKnown())

	saved := r.Offset()
	more, err := a.OpenRolloutAt(c.Path, saved)
	if err != nil {
		return err
	}
	defer func() { _ = more.Close() }()
	if _, err := more.Next(); errors.Is(err, io.EOF) {
		fmt.Fprintf(w, "nothing new after offset %d\n", saved)
	}

	rec, err := codex.ParseRecord([]byte(`{"type":"compacted","payload":{"message":"summary"}}`))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "parsed kind=%s message=%q\n", rec.Kind, rec.Compacted.Message)
	return nil
}

// snippet:end open

// snippet:start tail
// readTail decodes only the complete lines in the last bytes of a
// rollout; the first, possibly cut, line is dropped.
func readTail(w io.Writer, a *codex.Agent, h codex.Home) error {
	c, err := a.Conversation(h, convNew)
	if err != nil {
		return err
	}
	recs, err := a.RolloutTail(c.Path, tailBytes)
	if err != nil {
		return err
	}
	for _, rec := range recs {
		fmt.Fprintf(w, "tail %s\n", rec.Kind)
	}
	return nil
}

// snippet:end tail

// snippet:start tokens
// sumTokens returns the thread totals of the last token_usage_record (or,
// in older rollouts, the last token_count), from a tail read.
func sumTokens(w io.Writer, a *codex.Agent, h codex.Home) error {
	c, err := a.Conversation(h, convNew)
	if err != nil {
		return err
	}
	t, err := a.ConversationTokens(c.Path)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "tokens input=%d cached=%d output=%d reasoning=%d total=%d\n",
		t.Input, t.CachedInput, t.Output, t.ReasoningOutput, t.Total)
	n := t.Neutral()
	fmt.Fprintf(w, "neutral reasoning=%d cache-write=%d\n", n.Reasoning, n.CacheWrite)
	return nil
}

// snippet:end tokens
