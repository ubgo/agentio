# Codex rollout files

A rollout is the JSONL transcript Codex writes for one conversation. `RolloutReader` streams it record by record as typed `codex.Record` values and tracks a byte offset so you can follow a file Codex is still writing; `RolloutTail` decodes only the end; `ConversationTokens` sums a conversation's token use from a tail read.

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` from [The Codex agent](agent.md).
- A rollout path: `Conversation.Path` from [Codex conversations](conversations.md#step-1--list-conversations).
- Records are an open set: each line's top-level `type` (`session_meta`, `turn_context`, `response_item`, `event_msg`, `token_usage_record`, `compacted`, `world_state`, and whatever a newer Codex adds).

The snippets come from [`examples/codexconversations`](../../examples/codexconversations/).

## Step 1 — Open a rollout

`OpenRollout(path)` opens a file for streaming from the start; `OpenRolloutAt(path, offset)` resumes after an offset you stored. `RolloutReader.Next` returns the next complete `Record` (`io.EOF` at the end), `Offset` is the byte offset just after the last complete line returned, `Close` releases the file. Exactly one typed payload pointer is set for a known kind (`SessionMeta`, `TurnContext`, `Response`, `Event`, `TokenUsage`, `Compacted`); `Raw` always holds the original line without its terminator. `ParseRecord` decodes a single line you hold; a line that is not valid JSON is an error matching `codex.ErrInvalidLine`. `TurnContext` carries `Personality` (`pragmatic`, a `codex.Personality`) and keeps every key it does not model in `Unknown`. `SessionMeta.Created()` and `Version()` parse its timestamp and CLI version.

<!-- snippet: examples/codexconversations/main.go#open -->
```go
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
```

An agent message has a `Phase` (`EventMsg.Phase` on an older `agent_message` event, `TurnItem.Phase` on an `item_completed` `AgentMessage` item): `PhaseCommentary` for a message Codex shows while it keeps working, `PhaseFinalAnswer` for the answer that ends the turn. The set is open: an unknown value is kept as written and `IsKnown` reports false. `Phase.Interim()` is true for any phase other than `final_answer`; an absent phase (older rollouts wrote none) is an answer.

Within a turn, Codex writes the answer first, then a `response_item` copy, a `token_usage_record`, and only then the `token_count` event that carries the answer's `last_token_usage`, followed by `task_complete`. A `token_count` also follows every intermediate model response, so the one written after a commentary message and a tool call belongs to that response, not to the commentary.

## Step 2 — Read the tail

`RolloutTail(path, maxBytes)` decodes the complete lines within the last `maxBytes` of the file. The first, possibly cut, line is dropped. Use it for "what happened last" without reading the whole transcript.

<!-- snippet: examples/codexconversations/main.go#tail -->
```go
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
```

## Step 3 — Sum conversation tokens

`ConversationTokens(path)` returns the thread totals of the last `token_usage_record` (or, for older rollouts, the last `token_count` total), from a tail read, as `codex.TokenCounts`. `Neutral()` maps them onto `agentio.TokenCounts` (cache-write input becomes `CacheWrite`, reasoning output becomes `Reasoning`; the total is dropped because it is derivable).

<!-- snippet: examples/codexconversations/main.go#tokens -->
```go
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
```

## Try it

```sh
cd examples && go run ./codexconversations
```

The rollout part of the output:

```text
session_meta cwd=/work/proj version=0.162.1 unknown keys=3
turn_context model=gpt-5.5 approval=on-request sandbox=workspace-write
records=14 last kind=brand_new_record_type known=false
nothing new after offset 4236
parsed kind=compacted message="summary"
tail world_state
tail brand_new_record_type
tokens input=1200 cached=1000 output=80 reasoning=20 total=1280
neutral reasoning=20 cache-write=0
```

## Pitfalls

- **An incomplete trailing line is never consumed.** While Codex is writing, `Next` returns `io.EOF` before a partial last line and `Offset` stays before it; resume later with `OpenRolloutAt`.
- **Unknown data degrades, it does not fail.** A new record kind keeps `Kind` and `Raw` with no typed payload (`IsKnown()` is false); a payload that does not fit its kind's model keeps `Raw` only. Unknown `session_meta` keys go to `SessionMeta.Unknown`. Only a line that is not a JSON object is an error.
- **Lines are bounded.** A line longer than the reader's limit returns `codex.ErrLineTooLong` instead of growing memory without bound.
- **`world_state` stays raw.** It is large and rarely needed; decode `Record.WorldState` yourself if you must.
- **Never read a rollout whole to find one thing.** Use `Conversations` (head read) for metadata, `RolloutTail` for the end, and `Turns` or `TurnsAt` with a stored cursor to follow messages.

## Related

- [Codex conversations and history](conversations.md), [Codex auth and usage](auth-usage.md#step-3--read-cached-usage) (cached usage comes from the same tail read)
- [Codex exec event decoding](exec.md) for `codex exec --json` output, which is a different format
- [Claude transcripts](../claude/transcript.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
