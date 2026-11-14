# Claude stats

Claude Code keeps a precomputed activity summary per home in `stats-cache.json`: the data behind its `/usage` screen. `a.Stats(h)` decodes it into totals, per-day activity and tokens, per-model usage, the longest session and an hour-of-day histogram. Use it for an activity dashboard or a "how much did I use this week" report without scanning a single transcript.

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- A home where Claude Code has computed its stats (`<home>/stats-cache.json`, format version 5).

The snippet comes from [`examples/claudeconversations`](../../examples/claudeconversations/), which reads a temp copy of the checked-in 2.1.325 fixture home.

## Step 1 — Read the stats cache

`a.Stats(h)` reads `<home>/stats-cache.json` (disk only). A missing file is the zero `claude.Stats` with a nil error. The fields:

- `Version`, `LastComputed` (a date string such as `2026-10-26`: the cache is recomputed by Claude Code, not live), `FirstSession`, `TotalSessions`, `TotalMessages`.
- `Daily` — `[]claude.DailyActivity` rows (`Date`, `MessageCount`, `SessionCount`, `ToolCallCount`).
- `DailyTokens` — `[]claude.DailyTokens` rows (`Date`, `TokensByModel` keyed by model id).
- `ModelUsage` — `map[string]claude.ModelUsage` keyed by model id: token counts (`Input`, `Output`, `CacheRead`, `CacheCreation`, `WebSearch`), `CostUSD`, `ContextWindow`, `MaxOutputTokens`.
- `Longest` — `*claude.LongestSession` (`SessionID`, `Duration` as a `time.Duration`, `MessageCount`, `Timestamp`).
- `HourCounts` — a `[24]int64` of sessions started per hour of day.
- `Unknown` — top-level keys this version does not model.

`claude.ParseStats(data)` decodes a body you read yourself. Any version other than 5 returns an error wrapping `agentio.ErrUnknownVersion` together with a `Stats` whose `Version` and `Raw` are set, so you can still show "format N not supported" or hand the raw bytes on.

<!-- snippet: examples/claudeconversations/main.go#stats -->
```go
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
```

## Try it

```sh
cd examples && go run ./claudeconversations
```

The lines for this page:

```text
stats v5 computed 2026-10-26: 141 sessions, 93795 messages since 2026-10-11
  2026-10-24: 8944 messages, 8 sessions, 2595 tool calls
  2026-10-25: 10933 messages, 0 sessions, 3239 tool calls
  2026-10-26: 13460 messages, 14 sessions, 3323 tool calls
  model claude-opus-5-5: 39830885 output tokens, $0.00
  model claude-sonnet-5-5: 604908 output tokens, $0.00
  longest session: 98h47m0s, 670 messages
  busiest hour: 17:00 (23 sessions started)
```

## Pitfalls

- **The cache is as old as `LastComputed`.** Claude Code recomputes it on its own schedule; for today's numbers read [plan usage](usage.md) or sum turns from [conversations](conversations.md#step-3--read-turns).
- **`CostUSD` can be zero.** It is zero for every model in the fixture; do not present `$0.00` as "free". The per-session [`SessionCost`](transcript.md#step-4--read-cost-and-pr-links) carries a running cost when the transcript has one.
- **Map order is random.** `ModelUsage` and `TokensByModel` are maps; sort the keys before printing, as the example does.
- **Version 5 is the only recorded format.** Another version is an error by design (with `Raw` kept), not a best-effort decode that could show wrong totals.
- **Hours are the cache's own hour keys.** Keys outside 0–23 are ignored; whether they are local time or UTC is Claude Code's choice, not converted by the library.

## Related

- [Claude conversations](conversations.md), [Claude transcripts](transcript.md), [Claude plan usage](usage.md)
- Example: [`examples/claudeconversations`](../../examples/claudeconversations/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
