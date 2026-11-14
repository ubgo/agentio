# Claude conversations

Every Claude Code conversation is a transcript file under `<home>/projects/<encoded folder>/<id>.jsonl`. The `claude` module lists them with titles and folders, finds one by id, reads its turns incrementally from a byte offset, and plans moving a folder's conversations to another home. Use it for a "resume a recent conversation" picker, a per-folder history view, a live tail of a running session, or moving work between a personal and a work account.

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- A home with transcripts. Paths: the history dir of folder `/work/proj13` is `<home>/projects/-work-proj13` (every byte of the path that is not `[A-Za-z0-9]` becomes `-`; `a.HistoryDir(h, dir)` computes it).

The snippets come from [`examples/claudeconversations`](../../examples/claudeconversations/), which copies the checked-in 2.1.325 fixture home into a temp dir and pins the transcripts' modification times so the order is stable.

## Step 1 — List conversations

`a.Conversations(ctx, h, q)` returns `[]claude.Conversation`, newest first by file modification time. Each embeds the neutral `agentio.Conversation` (`ID`, `Title`, `Dir`, `Path`, `Created`, `Updated`, `Home`) and adds `TitleSource` (`claude.TitleCustom`, `TitleAI` or `TitleLastPrompt`), `Size`, `Version` (the Claude Code version that wrote the first record), `GitBranch`, `Owner`, `ContinuedIn` and `Name`.

`agentio.ConversationQuery` narrows the list: `Dir` reads only that folder's history dir (one directory, the way Claude Code itself looks it up), `Since` drops files modified before it, `Limit` caps the count. Each transcript is read at its head (for the folder) and its last 256 KiB (for the title), never whole.

<!-- snippet: examples/claudeconversations/main.go#list -->
```go
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
```

## Step 2 — Open one conversation

`a.Conversation(h, id)` finds `<id>.jsonl` in any project dir of the home and returns the same `claude.Conversation`; an unknown id, or a transcript that has no turn yet, is `agentio.ErrNotFound`. `a.HasHistory(h, dir)` answers "did Claude Code ever run in this folder in this home?" with one `stat` of the history dir and no transcript read: cheap enough for a file browser.

<!-- snippet: examples/claudeconversations/main.go#open -->
```go
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
```

## Step 3 — Read turns

`a.Turns(h, id, from, fn)` reads the transcript from byte offset `from` to the last complete line, calls `fn` with each normalized `agentio.Turn` (`Role`, `Time`, `Text`, `Model`, `StopReason`, `Tokens`, `AgentVersion`, `Raw`), and returns the new offset. Save the offset and call again later to read only what was appended: this is how a live view follows a running session. A trailing line Claude Code is still writing is never consumed, so the next call picks it up whole. Tool-result user lines and assistant lines with neither text nor usage are skipped: they are not turns a person typed or read. Returning an error from `fn` stops the read and returns the offset reached.

`claude.TurnsFile(path, from, fn)` does the same for a transcript path you already hold (for example from a hook's `transcript_path`).

<!-- snippet: examples/claudeconversations/main.go#turns -->
```go
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
```

## Step 4 — Plan a move

`a.PlanMove(req)` computes, without changing anything, an `agentio.MovePlan` for moving the conversations of `req.Dir` from home `req.From` to home `req.To`: the transcripts, each conversation's side folder, its `file-history` checkpoints and `session-env` snapshot, and on a full move the folder's auto memory. `Blockers` lists reasons the move must not run (a live Claude session in the folder in either home, a target file that already exists); `Left` lists entries found for the folder that are not moved, so nothing is dropped silently; `Cleanup` lists source dirs removed afterwards if empty. `req.OnlyIDs` limits the move to some conversations by full id or unique prefix: no match is `agentio.ErrNotFound`, a prefix matching several is `claude.ErrAmbiguousID`.

Applying a plan writes files, so it lives behind the Writer door: `a.Writer().ApplyMove(plan)` (see [Claude Writer](writer.md)) or the neutral `agentio.ApplyMovePlan`, which backs every item up first.

<!-- snippet: examples/claudeconversations/main.go#move -->
```go
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
```

## Try it

```sh
cd examples && go run ./claudeconversations
```

The lines for this page:

```text
conversations in personal: 2
  3f474440  "Fixture title 166" (ai-title) dir /work/proj13, v2.1.325, updated 2026-11-13T12:00:00Z
  f85efbdb  "Fixture title 185" (ai-title) dir /work/proj12, v2.1.325, updated 2026-11-13T11:00:00Z
  only /work/proj13, limit 1: 1
conversation 3f474440: 47179 bytes at ~/.claude/projects/-work-proj13/3f474440-903e-5410-8bfb-d537b89f9547.jsonl
  history for /work/proj13: true
  history for /work/never-used: false
  unknown id: true
turns: 1 user, 11 assistant, offset 47179
  last turn: assistant, model claude-opus-5-5, 57 output tokens
  turns after offset 47179: 0
move plan personal -> work for /work/proj13: 1 conversations, 1 items, 0 blockers, 0 left
  ~/.claude/projects/-work-proj13/3f474440-903e-5410-8bfb-d537b89f9547.jsonl -> ~/.claude-work/projects/-work-proj13/3f474440-903e-5410-8bfb-d537b89f9547.jsonl
  prefix 3f47: claude: ambiguous conversation prefix "3f47" matches 2 conversations
```

## Pitfalls

- **`Dir` from a query beats the recorded cwd.** With `q.Dir` set, every result's `Dir` is `q.Dir`: the cwd written inside old transcripts is stale after a history move. Without `q.Dir`, `Dir` is the cwd of the transcript's first record.
- **`Updated` and `Since` use the file's modification time.** Copying or touching transcripts changes the order; that is why the example pins mtimes.
- **Transcripts without a folder are not listed.** A file whose head has no cwd (a session that never ran a turn) is skipped by `Conversations`, and `Conversation` reports it as `ErrNotFound`, because it cannot be resumed in a known folder. Set-aside files (`<id>.orphaned-….jsonl`, `.superseded-…`) are skipped too.
- **An unreadable transcript is skipped, not an error.** The list is best effort; a permission problem on one file does not hide the rest.
- **`Turns` only sees complete lines.** If Claude Code is mid-write, the offset stops before the partial line; call again later rather than treating a short read as the end.
- **A move with a blocker must not run.** `PlanMove` returns the plan with `Blockers` set and a nil error; check `len(plan.Blockers)` before applying. Claude Code running in the folder would keep writing to the old path.
- **Conversations can move between Claude homes.** Unlike Codex (whose SQLite index pins absolute paths), a Claude transcript is self-contained; the plan moves its side data too.

## Related

- [Claude transcripts](transcript.md) (records, tail, last reply, cost), [Claude prompt history](history.md), [Claude live sessions](sessions.md), [Claude launch and resume](launch.md#step-2--resume-a-conversation), [Claude Writer](writer.md)
- Neutral view: [Conversations and history](../capabilities/conversations.md)
- Example: [`examples/claudeconversations`](../../examples/claudeconversations/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
