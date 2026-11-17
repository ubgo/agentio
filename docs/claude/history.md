# Claude prompt history

Claude Code appends every prompt you type to `<home>/history.jsonl` (the list its up-arrow recall shows), with the folder and conversation it was typed in. Large pastes are stored separately in `<home>/paste-cache/<hash>.txt`. The `claude` module reads that history newest first, filtered by folder, conversation or time, and follows paste hashes to their content. Use it for a "recent prompts" picker, a per-project prompt log, or a search over what you asked.

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- A home with a `history.jsonl` (a missing file reads as no history, not an error).

The snippets come from [`examples/claudeconversations`](../../examples/claudeconversations/). It copies the checked-in 2.1.325 fixture home into a temp dir and appends one history line whose paste points into `paste-cache` (the recorded shape with placeholder values), because the fixture has no paste.

## Step 1 — Read prompt history

`a.PromptHistory(h, q)` reads `history.jsonl` backwards from the end in 256 KiB chunks and returns matching `claude.HistoryEntry` values newest first: `Display` (the prompt as shown, with `[Pasted text #N]` placeholders), `Pasted`, `Time`, `Project` (the folder) and `SessionID` (the conversation). `claude.HistoryQuery` filters: `Project` (compared as clean paths), `SessionID`, `Since` and `Limit`. The scan stops at `Limit` matches or at the first entry older than `Since`, so asking for the newest few prompts of a large file reads only its last chunk.

`claude.ParseHistoryLine(line)` decodes one line you read yourself. Lines that do not parse are skipped by `PromptHistory`.

<!-- snippet: examples/claudeconversations/main.go#history -->
```go
// promptHistory reads history.jsonl backwards from the end: newest first,
// stopping at Limit, so a large file costs only its last chunks.
func promptHistory(w io.Writer, a *claude.Agent, h claude.Home) error {
	recent, err := a.PromptHistory(h, claude.HistoryQuery{Limit: 3})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "prompt history, newest 3:")
	for _, e := range recent {
		fmt.Fprintf(w, "  %s %s %q (%d pastes)\n", e.Time.UTC().Format(time.RFC3339), e.Project, e.Display, len(e.Pasted))
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
	fmt.Fprintf(w, "  parsed line: %q at %s, %d pastes\n", e.Display, e.Time.UTC().Format(time.RFC3339), len(e.Pasted))
	return nil
}
```

## Step 2 — Read pasted content

Each `claude.Paste` in `HistoryEntry.Pasted` has `ID`, `Type` and either `Content` (small pastes, inline) or `ContentHash` (large ones). `a.PastedContent(h, hash)` reads `paste-cache/<hash>.txt` (bounded). A hash with a path separator or a dot is refused, and a missing file is `agentio.ErrNotFound`. Pastes come back sorted by `ID`.

<!-- snippet: examples/claudeconversations/main.go#paste -->
```go
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
```

## Try it

```sh
cd examples && go run ./claudeconversations
```

The lines for this page:

```text
prompt history, newest 3:
  2026-11-13T19:43:20Z /work/proj4 "fix this [Pasted text #1]" (1 pastes)
  2026-11-13T19:35:10Z /work/proj13 "fixture text 139" (0 pastes)
  2026-11-13T19:10:49Z /work/proj4 "fixture text 138" (0 pastes)
  prompts typed in /work/proj4: 6
  parsed line: "hello" at 2026-11-13T18:37:08Z, 0 pastes
paste #1 (text, hash true): "a long pasted log line"
  path-like hash refused: true
```

## Pitfalls

- **`Since` assumes the file is in time order.** The reverse scan stops at the first entry older than `Since`; an out-of-order line earlier in the file is not reached. Claude Code appends in order, so this only matters for files edited by hand.
- **History is per home.** Each Claude home has its own `history.jsonl`; prompts typed under `~/.claude-work` are not in `~/.claude`. Loop over `a.Homes()` for a combined view.
- **`Display` is not the full prompt.** Pasted text appears as a `[Pasted text #N]` placeholder; join it with `Pasted` (inline `Content` or `PastedContent`) to rebuild what was sent.
- **Empty pastes come in two shapes.** Claude Code writes `[]` or `{}` for no pastes and an object keyed by paste id otherwise; both decode, and `Pasted` is nil when empty.
- **Prompts can hold secrets.** People paste keys and logs into prompts; treat `Display` and paste content as sensitive when you log or upload them.

## Related

- [Claude conversations](conversations.md), [Claude transcripts](transcript.md), [Claude paths](paths.md)
- Neutral view: [Conversations and history](../capabilities/conversations.md#step-3--read-prompt-history)
- Example: [`examples/claudeconversations`](../../examples/claudeconversations/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
