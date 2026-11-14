# Codex conversations and history

List the conversations of a Codex home (one rollout file each), find one by id, stream its user and agent messages with a resumable offset, read the prompt history, and check a model and reasoning effort against the cached model catalog. Every read is bounded: transcripts are never read whole.

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` and `codex.Home` from [The Codex agent](agent.md).
- Rollouts live under `<home>/sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl` (archived ones under `archived_sessions/`), titles in `session_index.jsonl`, prompts in `history.jsonl`, the model catalog in `models_cache.json`.
- For the SQLite thread index (richer metadata, sub-agents, projects) see [Codex SQLite state](state.md).

The snippets come from [`examples/codexconversations`](../../examples/codexconversations/), which copies the fixture home and sets rollout mtimes so the order is stable.

## Step 1 — List conversations

`Conversations(ctx, h, ConversationQuery)` lists rollouts newest first (by file mtime): a directory walk, one bounded head read per file and one read of `session_index.jsonl`. Query fields filter by `Dir`, `Since`, `Sources`, `IncludeArchived` and cap with `Limit`; zero values do not filter. `Conversation(h, id)` finds one by the id in its file name without opening other files (`ErrConversationNotFound` when none). `Titles(h)` reads the index (last line per id wins). `HasHistory(h, dir)` says whether any rollout's cwd is `dir`. `MoveUnsupportedReason()` explains why conversations cannot move between homes.

<!-- snippet: examples/codexconversations/main.go#list -->
```go
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
	fmt.Fprintf(w, "by id: created=%s branch=%s\n", c.Created.Format(time.RFC3339), c.Git.Branch)
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
```

## Step 2 — Read turns

`Turns(h, id, from, fn)` reads from byte offset `from` to the last complete line and calls `fn` once per user or agent message (an `agentio.Turn` with role, text, model and tokens on agent turns), then returns the new offset. Store it and pass it next time to get only new turns.

<!-- snippet: examples/codexconversations/main.go#turns -->
```go
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
```

## Step 3 — Read prompt history

`PromptHistory(h, PromptQuery)` returns prompts newest first, scanning `history.jsonl` from the tail in chunks, so a `Limit` reads only the end of the file. `ConversationID` filters by session; `Since` stops the scan.

<!-- snippet: examples/codexconversations/main.go#history -->
```go
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
```

## Step 4 — List models

`Models(h)` reads `models_cache.json` into a `ModelCatalog` (`ModelInfo` per model: slug, visibility, default and supported reasoning efforts, context window, unknown keys kept). `ModelCatalog.Validate(model, effort)` returns `ErrUnknownModel` or `ErrUnsupportedEffort` before you launch. `ParseModelCatalog` decodes bytes you hold and skips a model that does not decode, with a warning.

<!-- snippet: examples/codexconversations/main.go#models -->
```go
// listModels reads models_cache.json and validates a model and effort
// before a launch.
func listModels(w io.Writer, a *codex.Agent, h codex.Home) error {
	cat, err := a.Models(h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "catalog client=%s fetched=%s\n", cat.ClientVersion, cat.FetchedAt.Format(time.DateOnly))
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
```

## Try it

```sh
cd examples && go run ./codexconversations
```

The first part of the output (the rest belongs to [rollout files](rollout.md)):

```text
0199e1a0-0000-7000-8000-000000000001 "List the files" dir=/work/proj source=cli cli=0.162.1 originator=codex-tui
0199e1a0-0000-7000-8000-000000000002 "hello" dir=/work/other source=vscode cli=0.117.0 originator=Codex Desktop
in /work/other: 1
by id: created=2026-10-14T10:00:00Z branch=main
unknown id: ErrConversationNotFound
session_index title: "List the files"
has history in /work/proj: true
move: Codex indexes conversations in its own SQLite database with …
turn user      "list the files"
turn assistant "There is one file: a.txt" model=gpt-5.5 output=80 reasoning=20
resumed at offset 4236: 0 new turns, offset unchanged=true
prompt 2026-10-15 "and the sizes" session=0199e1a0-0000-7000-8000-000000000001
prompt 2026-10-15 "list the files" session=0199e1a0-0000-7000-8000-000000000001
prompts of 0199e1a0-0000-7000-8000-000000000002: 1
catalog client=0.162.1 fetched=2026-10-14
model gpt-6-luna visible=true default-effort=medium context=272000
model gpt-reserve visible=false default-effort=high context=128000
validate gpt-6-luna/low: <nil>
validate gpt-6-luna/xhigh unsupported: true
validate nope unknown: true
parsed models=1 warnings=1
```

## Pitfalls

- **Conversations cannot move between homes.** Codex indexes rollouts in `state_5.sqlite` with absolute paths, so the neutral history and conversation movers are unsupported for Codex (`agentio.Unsupported` gives the reason). Archive or delete through the vendor commands instead ([launch](launch.md#step-3--build-archive-delete-and-queue-commands)).
- **Order is by file mtime.** Copying a home without preserving mtimes (for example a git checkout) changes the order; the example sets them explicitly.
- **The title falls back to the first user message.** Without a `session_index.jsonl` entry, `Title` is the first user message from the head read, truncated.
- **Codex records no folder in `history.jsonl`.** A `PromptQuery.Dir` cannot match and returns nothing.
- **`HasHistory` builds its cwd index per call.** There is no global cache; call it once per decision, not per row.
- **Both rollout generations are read.** Codex 0.15x and later write `item_completed` items; older versions wrote `user_message` / `agent_message` events. When a file has both, `item_completed` wins so a message is not reported twice.
- **`models_cache.json` is a cache** written by Codex; it can lag behind the server. `Runner.BundledModels` reads the catalog compiled into the binary ([install](install.md#step-3--run-the-binary-through-the-runner)).

## Related

- [Conversations (neutral)](../capabilities/conversations.md)
- [Codex rollout files](rollout.md), [Codex SQLite state](state.md), [Codex live sessions](sessions.md)
- [Claude conversations](../claude/conversations.md), [Claude prompt history](../claude/history.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
