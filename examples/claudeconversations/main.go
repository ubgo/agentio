// Command claudeconversations walks everything agentio reads about Claude
// Code's past work in one home: the stats cache, the conversation list,
// turns, a move plan, transcripts (stream, tail, last reply, cost, PR
// links) and the prompt history with pasted content.
//
// It copies the checked-in 2.1.325 fixture home into a temp dir first,
// because two steps append real-shaped records (a cost-state and a pr-link
// line, a history entry with a paste) that the fixture does not carry.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// defaultFixture is the fixture user home, relative to the examples module.
const defaultFixture = "../claude/testdata/homes/2.1.325/userhome"

// Fixture facts: the conversation the steps read, the folder it ran in,
// and the second home a move plan targets.
const (
	conversationID = "3f474440-903e-5410-8bfb-d537b89f9547"
	projectDir     = "/work/proj13"
	historyProject = "/work/proj4"
	// otherConversationID ran in otherProjectDir, an hour before the first.
	otherConversationID = "f85efbdb-fc2f-1e8e-7748-9f915edc8e2d"
	otherProjectDir     = "/work/proj12"
	workHomeName        = ".claude-work"
)

// File modes for the temp copy.
const (
	dirMode  = 0o755
	fileMode = 0o644
)

// tailBytes is how much of a transcript the tail step decodes.
const tailBytes = 64 << 10

// textPreview caps printed message text.
const textPreview = 40

// fixtureNow is the fixed clock.
func fixtureNow() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

func main() {
	fixture := flag.String("fixture", defaultFixture, "Claude fixture user home to copy and read")
	flag.Parse()
	tmp, err := os.MkdirTemp("", "claudeconversations-")
	if err == nil {
		err = run(context.Background(), os.Stdout, *fixture, tmp)
		// Best-effort cleanup of our own temp dir: run's error is the one
		// worth reporting, and a leftover dir under $TMPDIR is harmless.
		_ = os.RemoveAll(tmp)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "claudeconversations:", err)
		os.Exit(1)
	}
}

// copyTree copies the fixture into dst so appends never touch checked-in
// files.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, dirMode)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, fileMode)
	})
}

// appendLines appends JSON lines to a file.
func appendLines(path string, lines ...string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, fileMode)
	if err != nil {
		return err
	}
	for _, l := range lines {
		if _, err := io.WriteString(f, l+"\n"); err != nil {
			_ = f.Close() // the write error above is the one to report
			return err
		}
	}
	return f.Close()
}

// Real-shaped records the fixture transcript lacks (values are placeholders).
const (
	costStateLine = `{"type":"cost-state","sessionId":"` + conversationID + `","startTime":1794590000000,"totalCostUSD":4.21,"totalDuration":3600000,"totalAPIDuration":1500000,"totalAPIDurationWithoutRetries":1480000,"totalToolDuration":600000,"totalLinesAdded":120,"totalLinesRemoved":30,"hasUnknownModelCost":false,"modelUsage":{"claude-opus-5-5":{"inputTokens":1200,"outputTokens":3400,"cacheReadInputTokens":500000,"cacheCreationInputTokens":20000,"webSearchRequests":0,"costUSD":4.21,"contextWindow":1000000,"maxOutputTokens":64000}}}`
	prLinkLine    = `{"type":"pr-link","prNumber":42,"prRepository":"example/project","prUrl":"https://github.com/example/project/pull/42","sessionId":"` + conversationID + `","timestamp":"2026-11-13T19:45:00Z"}`
	pasteHash     = "0123456789abcdef"
	pasteText     = "a long pasted log line"
	historyLine   = `{"display":"fix this [Pasted text #1]","pastedContents":{"1":{"id":1,"type":"text","contentHash":"` + pasteHash + `"}},"timestamp":1794599000000,"project":"` + historyProject + `","sessionId":"6d50c0ec-647e-18aa-84c7-04170288be89"}`
)

// run copies the fixture home into tmp, appends the few records the
// fixture lacks (a cost line, a PR link, a pasted prompt) and prints each
// conversation read. It writes only under tmp.
func run(ctx context.Context, w io.Writer, fixture, tmp string) error {
	if err := copyTree(fixture, tmp); err != nil {
		return err
	}
	host := agentio.Host{UserHome: tmp, Env: []string{"HOME=" + tmp, "PATH="}, Now: fixtureNow}
	a := claude.New(host)
	h := a.DefaultHome()
	transcript := filepath.Join(a.HistoryDir(h, projectDir), conversationID+".jsonl")
	if err := appendLines(transcript, costStateLine, prLinkLine); err != nil {
		return err
	}
	if err := appendLines(filepath.Join(h.Dir(), "history.jsonl"), historyLine); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(h.Dir(), "paste-cache"), dirMode); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(h.Dir(), "paste-cache", pasteHash+".txt"), []byte(pasteText), fileMode); err != nil {
		return err
	}
	// Updated is the transcript's mtime; pin both so the order is stable.
	for path, at := range map[string]time.Time{
		transcript: fixtureNow(),
		filepath.Join(a.HistoryDir(h, otherProjectDir), otherConversationID+".jsonl"): fixtureNow().Add(-time.Hour),
	} {
		if err := os.Chtimes(path, at, at); err != nil {
			return err
		}
	}

	for _, step := range []func() error{
		func() error { return readStats(w, a, h) },
		func() error { return listConversations(ctx, w, a, h) },
		func() error { return openConversation(w, a, h) },
		func() error { return readTurns(w, a, h) },
		func() error { return planMove(w, a, h, a.HomeAt(workHomeName)) },
		func() error { return streamTranscript(w, a, h, transcript) },
		func() error { return readTail(w, a, h) },
		func() error { return lastReply(w, a, h) },
		func() error { return costAndLinks(w, a, h) },
		func() error { return promptHistory(w, a, h) },
		func() error { return pastedContent(w, a, h) },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// tilde shows a path under the user home as ~/….
func tilde(a *claude.Agent, p string) string {
	return strings.Replace(p, a.Host().UserHome, "~", 1)
}

// preview shortens text to one line of at most textPreview runes.
func preview(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > textPreview {
		return string(r[:textPreview]) + "…"
	}
	return s
}

// snippet:start stats
// readStats reads stats-cache.json (the data behind Claude Code's /usage
// screen). A missing file is the zero Stats; another format version is
// agentio.ErrUnknownVersion with Raw kept.
func readStats(w io.Writer, a *claude.Agent, h claude.Home) error {
	s, err := a.Stats(h)
	if errors.Is(err, agentio.ErrUnknownVersion) {
		fmt.Fprintf(w, "stats: format version %d not supported (%d raw bytes kept)\n", s.Version, len(s.Raw))
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "stats v%d computed %s: %d sessions, %d messages since %s\n", s.Version, s.LastComputed, s.TotalSessions, s.TotalMessages, s.FirstSession.Format(time.DateOnly))
	for _, d := range s.Daily {
		fmt.Fprintf(w, "  %s: %d messages, %d sessions, %d tool calls\n", d.Date, d.MessageCount, d.SessionCount, d.ToolCallCount)
	}
	models := make([]string, 0, len(s.ModelUsage))
	for m := range s.ModelUsage {
		models = append(models, m)
	}
	slices.Sort(models)
	for _, m := range models {
		u := s.ModelUsage[m]
		fmt.Fprintf(w, "  model %s: %d output tokens, $%.2f\n", m, u.Output, u.CostUSD)
	}
	if l := s.Longest; l != nil {
		fmt.Fprintf(w, "  longest session: %s, %d messages\n", l.Duration.Round(time.Minute), l.MessageCount)
	}
	peak := 0
	for hour, n := range s.HourCounts {
		if n > s.HourCounts[peak] {
			peak = hour
		}
	}
	fmt.Fprintf(w, "  busiest hour: %02d:00 (%d sessions started)\n", peak, s.HourCounts[peak])
	return nil
}

// snippet:end stats

// snippet:start list
// listConversations lists resumable conversations newest first. Each
// transcript is read only at its head (cwd) and its last 256 KiB (title).
func listConversations(ctx context.Context, w io.Writer, a *claude.Agent, h claude.Home) error {
	all, err := a.Conversations(ctx, h, agentio.ConversationQuery{})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "conversations in %s: %d\n", h.Name(), len(all))
	for _, c := range all {
		fmt.Fprintf(w, "  %s  %-18q (%s) dir %s, v%s, updated %s\n", c.ID[:8], c.Title, c.TitleSource, c.Dir, c.Version, c.Updated.Format(time.RFC3339))
	}
	in, err := a.Conversations(ctx, h, agentio.ConversationQuery{Dir: projectDir, Limit: 1})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  only %s, limit 1: %d\n", projectDir, len(in))
	return nil
}

// snippet:end list

// snippet:start open
// openConversation finds one conversation by id and answers "has this
// folder any history?" with a single stat.
func openConversation(w io.Writer, a *claude.Agent, h claude.Home) error {
	c, err := a.Conversation(h, conversationID)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "conversation %s: %d bytes at %s\n", c.ID[:8], c.Size, tilde(a, c.Path))
	for _, dir := range []string{projectDir, "/work/never-used"} {
		has, err := a.HasHistory(h, dir)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "  history for %s: %t\n", dir, has)
	}
	if _, err := a.Conversation(h, "00000000-missing"); errors.Is(err, agentio.ErrNotFound) {
		fmt.Fprintln(w, "  unknown id:", errors.Is(err, agentio.ErrNotFound))
	}
	return nil
}

// snippet:end open

// snippet:start turns
// readTurns reads normalized turns from offset 0 and keeps the returned
// offset: a second call from it reads only what was appended since.
func readTurns(w io.Writer, a *claude.Agent, h claude.Home) error {
	counts := map[agentio.Role]int{}
	var last agentio.Turn
	off, err := a.Turns(h, conversationID, 0, func(t agentio.Turn) error {
		counts[t.Role]++
		last = t
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "turns: %d user, %d assistant, offset %d\n", counts[agentio.RoleUser], counts[agentio.RoleAssistant], off)
	if last.Tokens != nil {
		fmt.Fprintf(w, "  last turn: %s, model %s, %d output tokens\n", last.Role, last.Model, last.Tokens.Output)
	}
	more := 0
	if _, err := a.Turns(h, conversationID, off, func(agentio.Turn) error { more++; return nil }); err != nil {
		return err
	}
	fmt.Fprintf(w, "  turns after offset %d: %d\n", off, more)
	return nil
}

// snippet:end turns

// snippet:start move
// planMove computes, without changing anything, what moving one folder's
// conversations to another home would rename. A prefix that matches more
// than one conversation is ErrAmbiguousID.
func planMove(w io.Writer, a *claude.Agent, from, to claude.Home) error {
	plan, err := a.PlanMove(agentio.MoveRequest{From: from.Neutral(), To: to.Neutral(), Dir: projectDir})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "move plan %s -> %s for %s: %d conversations, %d items, %d blockers, %d left\n",
		from.Name(), to.Name(), plan.Dir, len(plan.Conversations), len(plan.Items), len(plan.Blockers), len(plan.Left))
	for _, it := range plan.Items {
		fmt.Fprintf(w, "  %s -> %s\n", tilde(a, it.From), tilde(a, it.To))
	}
	// A second transcript sharing the prefix makes "3f47" ambiguous.
	dir := a.HistoryDir(from, projectDir)
	twin := filepath.Join(dir, "3f474440-0000-4000-8000-000000000002.jsonl")
	if err := os.WriteFile(twin, nil, fileMode); err != nil {
		return err
	}
	defer func() { _ = os.Remove(twin) }()
	_, err = a.PlanMove(agentio.MoveRequest{From: from.Neutral(), To: to.Neutral(), Dir: projectDir, OnlyIDs: []string{"3f47"}})
	if errors.Is(err, claude.ErrAmbiguousID) {
		fmt.Fprintln(w, "  prefix 3f47:", err)
	}
	return nil
}

// snippet:end move

// snippet:start transcript
// streamTranscript reads every record of a transcript in order. Any JSON
// object decodes into a Record; typed accessors refuse other types
// (ErrWrongRecordType) and internal records (ErrInternalRecord).
func streamTranscript(w io.Writer, a *claude.Agent, h claude.Home, path string) error {
	home, id, err := a.TranscriptAt(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "transcript %s belongs to home %s\n", id[:8], home.Name())
	r, err := a.OpenTranscript(h, conversationID)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	counts := map[claude.RecordType]int{}
	var internal, wrong error
	for {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		counts[rec.Type]++
		if !rec.Type.IsKnown() {
			fmt.Fprintf(w, "  new record type %q (kept in Raw)\n", rec.Type)
		}
		switch rec.Type {
		case claude.RecordAPIRequest:
			_, internal = rec.AsUser()
		case claude.RecordMode:
			_, wrong = rec.AsTitle()
		}
	}
	fmt.Fprintf(w, "  %d records: %d user, %d assistant, %d ai-title, %d api-request; offset %d\n",
		sum(counts), counts[claude.RecordUser], counts[claude.RecordAssistant], counts[claude.RecordAITitle], counts[claude.RecordAPIRequest], r.Offset())
	fmt.Fprintf(w, "  internal record: %v\n  wrong type: %v\n", internal, wrong)
	rec, err := claude.ParseRecord([]byte(`{"type":"future-record","sessionId":"x"}`))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  parsed unknown line: type %q, known %t\n", rec.Type, rec.Type.IsKnown())
	return nil
}

// sum adds the per-type counts.
func sum(m map[claude.RecordType]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

// snippet:end transcript

// snippet:start tail
// readTail decodes only the last bytes of a transcript: the newest title
// and last prompt sit at the end, so a long file is never read whole.
func readTail(w io.Writer, a *claude.Agent, h claude.Home) error {
	recs, err := a.TranscriptTail(h, conversationID, tailBytes)
	if err != nil {
		return err
	}
	var title claude.Title
	var prompt claude.LastPrompt
	for _, rec := range recs {
		switch rec.Type {
		case claude.RecordAITitle, claude.RecordCustomTitle:
			if t, err := rec.AsTitle(); err == nil {
				title = t
			}
		case claude.RecordLastPrompt:
			if p, err := rec.AsLastPrompt(); err == nil {
				prompt = p
			}
		}
	}
	fmt.Fprintf(w, "tail: %d records, title %q (%s), last prompt %q\n", len(recs), title.Title, title.Type, prompt.Text)
	return nil
}

// snippet:end tail

// snippet:start last
// lastReply returns the newest assistant record that has text (tool-only
// turns are skipped), from the last 256 KiB.
func lastReply(w io.Writer, a *claude.Agent, h claude.Home) error {
	ar, ok, err := a.LastAssistant(h, conversationID)
	if err != nil || !ok {
		return err
	}
	fmt.Fprintf(w, "last reply: model %s, stop %q, text %q\n", ar.Model, ar.StopReason, preview(ar.Content.Text()))
	if ar.Usage != nil {
		n := ar.Usage.Neutral()
		fmt.Fprintf(w, "  tokens: %d in, %d cached in, %d out\n", n.Input, n.CachedInput, n.Output)
	}
	return nil
}

// snippet:end last

// snippet:start cost
// costAndLinks reads the session's running cost totals, the pull requests
// it opened, and counts the internal records (raw only: they may carry
// system prompts and are never decoded).
func costAndLinks(w io.Writer, a *claude.Agent, h claude.Home) error {
	cost, err := a.SessionCost(h, conversationID)
	if err != nil {
		return err
	}
	if cost != nil {
		fmt.Fprintf(w, "cost: $%.2f over %s (API %s), +%d/-%d lines, %d models\n", cost.TotalCostUSD, cost.TotalDuration, cost.TotalAPIDuration,
			cost.TotalLinesAdded, cost.TotalLinesRemoved, len(cost.ModelUsage))
	}
	links, err := a.PRLinks(h, conversationID)
	if err != nil {
		return err
	}
	for _, l := range links {
		fmt.Fprintf(w, "pr: %s #%d %s\n", l.Repository, l.Number, l.URL)
	}
	internal, err := a.InternalRecords(h, conversationID)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "internal records in tail: %d\n", len(internal))
	return nil
}

// snippet:end cost

// snippet:start history
// promptHistory reads history.jsonl backwards from the end: newest first,
// stopping at Limit, so a large file costs only its last chunks.
func promptHistory(w io.Writer, a *claude.Agent, h claude.Home) error {
	recent, err := a.PromptHistory(h, claude.HistoryQuery{Limit: 3})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "prompt history, newest 3:")
	for _, e := range recent {
		fmt.Fprintf(w, "  %s %s %q (%d pastes)\n", e.Time.Format(time.RFC3339), e.Project, e.Display, len(e.Pasted))
	}
	in, err := a.PromptHistory(h, claude.HistoryQuery{Project: historyProject})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  prompts typed in %s: %d\n", historyProject, len(in))
	e, err := claude.ParseHistoryLine([]byte(`{"display":"hello","pastedContents":[],"timestamp":1794595028123,"project":"/work/proj4","sessionId":"s"}`))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  parsed line: %q at %s, %d pastes\n", e.Display, e.Time.Format(time.RFC3339), len(e.Pasted))
	return nil
}

// snippet:end history

// snippet:start paste
// pastedContent follows a paste's ContentHash into paste-cache: large
// pastes are stored there instead of inline in history.jsonl.
func pastedContent(w io.Writer, a *claude.Agent, h claude.Home) error {
	recent, err := a.PromptHistory(h, claude.HistoryQuery{Limit: 1})
	if err != nil || len(recent) == 0 {
		return err
	}
	for _, p := range recent[0].Pasted {
		text := p.Content
		if p.ContentHash != "" {
			if text, err = a.PastedContent(h, p.ContentHash); err != nil {
				return err
			}
		}
		fmt.Fprintf(w, "paste #%d (%s, hash %t): %q\n", p.ID, p.Type, p.ContentHash != "", text)
	}
	if _, err := a.PastedContent(h, "../escape"); errors.Is(err, agentio.ErrNotFound) {
		fmt.Fprintln(w, "  path-like hash refused:", errors.Is(err, agentio.ErrNotFound))
	}
	return nil
}

// snippet:end paste
