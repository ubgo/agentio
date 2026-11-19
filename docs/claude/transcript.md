# Claude transcripts

A Claude Code transcript (`<home>/projects/<folder>/<id>.jsonl`) is one JSON record per line: user and assistant messages, titles, the last prompt, cost totals, PR links, file checkpoints and more. The `claude` module streams records with typed accessors, decodes just the tail of a long file, finds the last assistant reply, and reads the session's cost and PR links. Use it when the normalized turns of [Claude conversations](conversations.md) are not enough: a transcript viewer, a "what did it last say" notification, or a cost report per session.

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- A conversation id in a home, or a transcript path.

The snippets come from [`examples/claudeconversations`](../../examples/claudeconversations/). It copies the checked-in 2.1.325 fixture home into a temp dir and appends one `cost-state` and one `pr-link` record (recorded shapes, placeholder values) to a transcript, because the fixture has neither.

## Step 1 — Open a transcript

`a.OpenTranscript(h, id)` opens a conversation's transcript from the start and returns a `*claude.TranscriptReader`; `a.OpenTranscriptFile(path, offset)` opens a path at a byte offset (for a live capture that got the path from a hook). `Next()` returns one `claude.Record` per complete line and `io.EOF` at the end, never advancing past a line still being written; `Offset()` is where to resume; `Close()` releases the file. `a.TranscriptAt(path)` maps a transcript path back to its home and conversation id, or returns `agentio.ErrForeignHome` for a path outside a Claude projects dir. The neutral `TurnFileReader.TranscriptOwner` (`agentio.TurnFilesOf`) wraps it and reports such a path as `agentio.ErrNotFound`, as every agent does; a subagent transcript (`projects/<key>/<session>/subagents/…`) is not mapped. Normalized turns carry the record `UUID` as `Turn.ID`.

A `claude.Record` is the envelope every line shares: `Type` (a `claude.RecordType`), `Subtype`, `UUID`, `ParentUUID`, `SessionID`, `Timestamp`, `Cwd`, `GitBranch`, `Version`, `Entrypoint`, `IsSidechain`, and `Raw` (the whole line). `claude.ParseRecord(line)` decodes any JSON object, whatever its type; `Type.IsKnown()` and `claude.RecordTypes()` tell recorded types from new ones. Typed bodies come from accessors: `AsUser`, `AsAssistant`, `AsTitle`, `AsLastPrompt`, `AsCostState`, `AsPRLink`, `AsSessionOwner`, `AsContinuedIn`, `AsAgentName`, `AsPermissionMode`. Calling one on another type is `claude.ErrWrongRecordType`; calling any on an internal record (`api-request`, `api-request-blob`, `atis-latch`) is `claude.ErrInternalRecord`.

<!-- snippet: examples/claudeconversations/main.go#transcript -->
```go
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
```

## Step 2 — Read the tail

`a.TranscriptTail(h, id, n)` decodes the records in the last `n` bytes of a conversation (`a.TranscriptTailFile(path, n)` for a path); a partial first line is dropped. Titles and the last prompt are appended as the session goes, so the newest ones sit at the end: read a few dozen KiB instead of a file that can reach hundreds of MiB. The typed accessors give `claude.Title` (`Type` tells `ai-title` from `custom-title`), `claude.LastPrompt`, `claude.UserRecord` (`Content`, `PermissionMode`, `IsToolResult`) and `claude.AssistantRecord` (`Model`, `StopReason`, `Effort`, `Content`, `Usage`). `Content.Text()` joins the text blocks with newlines whether the message content was a plain string or a block array.

<!-- snippet: examples/claudeconversations/main.go#tail -->
```go
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
```

## Step 3 — Read the last assistant reply

`a.LastAssistant(h, id)` returns the newest assistant record that has text in the last 256 KiB; tool-only turns are skipped. `found` is false when the tail has none. `Usage.Neutral()` maps the message's usage onto `agentio.TokenCounts` (`Input`, `CachedInput`, `CacheWrite`, `Output`).

<!-- snippet: examples/claudeconversations/main.go#last -->
```go
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
```

## Step 4 — Read cost and PR links

`a.SessionCost(h, id)` returns the last `cost-state` record of the tail as a `*claude.CostState` (nil when there is none): `StartTime`, `TotalCostUSD`, `TotalDuration`, `TotalAPIDuration`, `TotalToolDuration` (all `time.Duration`), `TotalLinesAdded`, `TotalLinesRemoved`, `HasUnknownModelCost` and per-model `ModelUsage`. `a.PRLinks(h, id)` returns the `pr-link` records (`Number`, `Repository`, `URL`, `Timestamp`). `a.InternalRecords(h, id)` returns the internal records as raw bytes only.

<!-- snippet: examples/claudeconversations/main.go#cost -->
```go
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
```

## Try it

```sh
cd examples && go run ./claudeconversations
```

The lines for this page:

```text
transcript 3f474440 belongs to home personal
  54 records: 7 user, 11 assistant, 3 ai-title, 4 api-request; offset 47179
  internal record: claude: internal transcript record is never decoded
  wrong type: claude: wrong transcript record type: mode is not a title
  parsed unknown line: type "future-record", known false
tail: 54 records, title "Fixture title 166" (ai-title), last prompt "fixture last prompt"
last reply: model claude-opus-5-5, stop "end_turn", text "fixture text 165"
  tokens: 2 in, 111734 cached in, 57 out
cost: $4.21 over 1h0m0s (API 25m0s), +120/-30 lines, 1 models
pr: example/project #42 https://github.com/example/project/pull/42
internal records in tail: 11
```

## Pitfalls

- **Internal records are never decoded.** `api-request`, `api-request-blob` and `atis-latch` may carry system prompts; every typed accessor refuses them with `ErrInternalRecord`, and `InternalRecords` hands them out raw only. Do not log or upload their bytes.
- **Never read a transcript whole when a tail will do.** Transcripts grow to hundreds of MiB; `LastAssistant`, `SessionCost`, `PRLinks` and `InternalRecords` read only the last 256 KiB, so a cost record older than that is not found.
- **Unknown record types are data, not errors.** A new type decodes into a `Record` with `IsKnown()` false and the full line in `Raw`; the reader never fails on it. A line that is not JSON at all is an error from `Next`.
- **The tail starts mid-file.** `TranscriptTail` drops the first, partial line of the window and skips lines that do not parse, so the record count depends on `n`.
- **Several titles accumulate.** Claude Code appends a new `ai-title` as the conversation evolves; the last one wins (the example keeps overwriting). A `custom-title` set by the user takes precedence in [conversation listings](conversations.md#step-1--list-conversations).
- **Not every user record is a prompt.** `UserRecord.IsToolResult` is true when every block is a tool result; filter on it (as `Turns` does) before showing "what the user typed".

## Related

- [Claude conversations](conversations.md), [Claude prompt history](history.md), [Claude stats](stats.md), [Claude stream decoding](stream.md) (the headless output, a different format)
- Neutral view: [Conversations and history](../capabilities/conversations.md#step-2--read-turns-from-an-offset)
- Example: [`examples/claudeconversations`](../../examples/claudeconversations/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
