# Claude stream decoding

A headless Claude Code run (`claude -p`) reports progress on stdout: one JSON object per line with `--output-format stream-json`, or one document at the end with `--output-format json`. The `claude` module decodes both into typed values (session setup, partial text, messages, rate-limit state, the final result with cost and durations) and maps the stream onto neutral `agentio.RunEvent`s. Use it to drive a UI from a background run, collect cost per task, or react to a run that hit a limit. Launching the run is up to you: build the command with [Claude launch and resume](launch.md).

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- The stdout of a run started with `Print: true, OutputFormat: claude.OutputStreamJSON, Verbose: true` (add `IncludePartialMessages: true` for text deltas), or with `OutputFormat: claude.OutputJSON` for Step 2. The decoders read any `io.Reader`: a pipe, a file, a buffer.

The snippets come from [`examples/claudestream`](../../examples/claudestream/), which reads the run recorded from Claude Code 2.1.325 in `claude/testdata/stream/2.1.325`.

## Step 1 — Decode a stream-json run

`claude.NewStreamDecoder(r)` wraps a reader (lines up to 16 MiB). `Next()` returns one `claude.StreamMessage` per line and `io.EOF` at the end; blank lines are skipped, a line that is not JSON is an error. The envelope (`Type`, `Subtype`, `SessionID`, `UUID`, `ParentToolUseID`, `Raw`) is always decoded; the body comes from the accessor for its type:

| Line | Accessor | Result |
|---|---|---|
| `system` / `init` | `AsInit` | `claude.StreamInit`: version, model, permission mode, cwd, tools, MCP servers, slash commands, agents, skills, plugins, capabilities |
| `system` / `status`, `commands_changed` | `AsStatus`, `AsCommandsChanged` | `StreamStatus`, `StreamCommandsChanged` |
| `system` / `api_retry` | `AsAPIRetry` | `claude.APIRetry`: attempt, max retries, delay, HTTP status, error |
| `system` / `task_notification` | `AsTaskNotification` | `claude.TaskNotification` |
| `stream_event` | `AsEvent` | `StreamEventMessage`: the raw Messages API event (`message_start`, `content_block_delta` with a `text_delta`, …) |
| `assistant` | `AsAssistant` | `StreamAssistantMessage`: model, content, stop reason, usage |
| `user` | `AsUser` | `StreamUserMessage`: tool results fed back to the model |
| `rate_limit_event` | `AsRateLimit` | `claude.RateLimitEvent`: plan-limit status, binding window, overage, per-window utilization |
| `result` | `AsResult` | `claude.StreamResult` (see Step 2) |

An accessor called on another type returns `claude.ErrWrongStreamMessage`. A type or subtype this version does not know is not an error: `Type.IsKnown()` is false and `Raw` keeps the line. Each typed body keeps unmodelled keys in `Unknown`.

<!-- snippet: examples/claudestream/main.go#stream -->
```go
// decodeStream reads stream-json lines one at a time. The envelope
// (Type, Subtype, SessionID) is always decoded; the typed body comes from
// the matching AsX accessor. Unknown types are not errors: Raw keeps them.
func decodeStream(w io.Writer, r io.Reader) error {
	d := claude.NewStreamDecoder(r)
	var partial strings.Builder
	for {
		m, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch m.Type {
		case claude.StreamTypeSystem:
			if m.Subtype != claude.SubtypeInit {
				fmt.Fprintf(w, "system/%s\n", m.Subtype)
				continue
			}
			in, err := m.AsInit()
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "init: Claude Code %s, model %s, mode %s, %d tools, %d MCP servers, %d slash commands\n",
				in.ClaudeCodeVersion, in.Model, in.PermissionMode, len(in.Tools), len(in.MCPServers), len(in.SlashCommands))
			if _, err := m.AsResult(); errors.Is(err, claude.ErrWrongStreamMessage) {
				fmt.Fprintln(w, "  AsResult on init:", err)
			}
		case claude.StreamTypeEvent:
			ev, err := m.AsEvent()
			if err != nil {
				return err
			}
			if ev.Event.Delta != nil && ev.Event.Delta.Type == claude.DeltaText {
				partial.WriteString(ev.Event.Delta.Text)
			}
			fmt.Fprintf(w, "stream_event %s\n", ev.Event.Type)
		case claude.StreamTypeAssistant:
			a, err := m.AsAssistant()
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "assistant: model %s, text %q\n", a.Message.Model, preview(a.Message.Content.Text()))
		case claude.StreamTypeRateLimit:
			rl, err := m.AsRateLimit()
			if err != nil {
				return err
			}
			names := make([]string, 0, len(rl.Info.UnifiedWindows))
			for k := range rl.Info.UnifiedWindows {
				names = append(names, k)
			}
			slices.Sort(names)
			for _, k := range names {
				win := rl.Info.UnifiedWindows[k]
				fmt.Fprintf(w, "rate limit: %s, binding %s, window %s at %.0f%%\n", rl.Info.Status, rl.Info.RateLimitType, k, win.Utilization*100)
			}
		case claude.StreamTypeResult:
			res, err := m.AsResult()
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "result: %s, error %t, %d turns, $%.4f, %s wall, stop %s\n", res.Subtype, res.IsError, res.NumTurns, res.TotalCostUSD, res.Duration, res.StopReason)
		default:
			fmt.Fprintf(w, "unknown line type %q (%d raw bytes)\n", m.Type, len(m.Raw))
		}
	}
	fmt.Fprintf(w, "partial text from deltas: %q\n", preview(partial.String()))
	return nil
}
```

`system/api_retry` did not occur in the recorded run; its fields follow the vendor docs. This snippet decodes one in that shape:

<!-- snippet: examples/claudestream/main.go#retry -->
```go
// decodeAPIRetry decodes a system/api_retry line: Claude Code retrying a
// failed API call (documented shape; a healthy run has none).
func decodeAPIRetry(w io.Writer) error {
	m, err := claude.NewStreamDecoder(strings.NewReader(apiRetryLine)).Next()
	if err != nil {
		return err
	}
	retry, err := m.AsAPIRetry()
	if err != nil {
		return err
	}
	status := 0
	if retry.ErrorStatus != nil {
		status = *retry.ErrorStatus
	}
	fmt.Fprintf(w, "api_retry: attempt %d of %d after %s, HTTP %d (%s)\n", retry.Attempt, retry.MaxRetries,
		time.Duration(retry.RetryDelayMS)*time.Millisecond, status, retry.Error)
	return nil
}
```

## Step 2 — Decode a JSON result

`claude -p --output-format json` prints a single document, the same object as the stream's `result` line. `claude.DecodeJSONResult(r)` reads it into a `claude.StreamResult`: `Subtype` (`SubtypeSuccess` or an error subtype such as `SubtypeErrorMaxTurns`; `Subtype.IsResultError()` tells them apart), `IsError`, `Result` (the final text), `SessionID` (resume with it), `NumTurns`, `StopReason`, `TerminalReason`, `TotalCostUSD`, `Usage`, per-model `ModelUsage`, `PermissionDenials`, durations as `time.Duration` (`Duration`, `APIDuration`, `TTFT`, …), `CredentialExpiresAt` and `Raw`. A document whose `type` is not `result` is `claude.ErrWrongStreamMessage`.

<!-- snippet: examples/claudestream/main.go#json -->
```go
// decodeJSON reads the one document `claude -p --output-format json`
// prints: the same object as the stream's result line.
func decodeJSON(w io.Writer, r io.Reader) error {
	res, err := claude.DecodeJSONResult(r)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "json result: %s, text %q, session %s\n", res.Subtype, preview(res.Result), res.SessionID)
	fmt.Fprintf(w, "  usage: %d in, %d cache read, %d cache write, %d out; API %s, TTFT %s\n",
		res.Usage.Input, res.Usage.CacheRead, res.Usage.CacheCreation, res.Usage.Output, res.APIDuration, res.TTFT)
	models := make([]string, 0, len(res.ModelUsage))
	for m := range res.ModelUsage {
		models = append(models, m)
	}
	slices.Sort(models)
	for _, m := range models {
		fmt.Fprintf(w, "  model %s: $%.4f\n", m, res.ModelUsage[m].CostUSD)
	}
	return nil
}
```

## Step 3 — Convert to neutral run events

`StreamMessage.Neutral()` maps a line onto `agentio.RunEvent`: `system/init` is `RunEventStarted`; `assistant` and `stream_event` are `RunEventMessage` (with the text, or the partial text of a `text_delta`); a `user` line with a tool result is `RunEventTool`; `result` is `RunEventDone`, or `RunEventError` for `is_error` or an error subtype, with the final text and token counts; everything else is `RunEventOther`. `Raw` always holds the line. Code that runs several agents gets the same events from `agentio.RunEventsOf(a.Neutral())` and its `NewRunDecoder(r)` (see [Run events](../capabilities/run-events.md)).

<!-- snippet: examples/claudestream/main.go#neutral -->
```go
// neutralEvents maps each line onto agentio.RunEvent: Started, Message,
// Tool, Done or Error, Other for the rest. Code that loops over agents
// reads every vendor's run this way.
func neutralEvents(w io.Writer, r io.Reader) error {
	d := claude.NewStreamDecoder(r)
	counts := map[agentio.RunEventType]int{}
	for {
		m, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		ev := m.Neutral()
		counts[ev.Type]++
		if ev.Type == agentio.RunEventDone && ev.Tokens != nil {
			fmt.Fprintf(w, "neutral done: %q, %d output tokens\n", preview(ev.Text), ev.Tokens.Output)
		}
	}
	for _, t := range []agentio.RunEventType{agentio.RunEventStarted, agentio.RunEventMessage, agentio.RunEventTool, agentio.RunEventDone, agentio.RunEventOther} {
		fmt.Fprintf(w, "  %s events: %d\n", t, counts[t])
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudestream
```

```text
init: Claude Code 2.1.325, model claude-haiku-5-5, mode bypassPermissions, 5 tools, 2 MCP servers, 3 slash commands
  AsResult on init: claude: wrong stream message type: system/init
system/status
system/commands_changed
stream_event message_start
stream_event content_block_start
stream_event content_block_delta
assistant: model claude-haiku-5-5, text "ok"
stream_event content_block_stop
stream_event message_delta
stream_event message_stop
rate limit: allowed, binding seven_day, window seven_day at 52%
result: success, error false, 1 turns, $0.0045, 1.279s wall, stop end_turn
partial text from deltas: "ok"
api_retry: attempt 1 of 10 after 500ms, HTTP 529 (overloaded)
json result: success, text "ok", session 00000000-0000-4000-8000-000000000001
  usage: 2 in, 11370 cache read, 17940 cache write, 4 out; API 1.199s, TTFT 1.328s
  model claude-haiku-5-5: $0.0037
neutral done: "ok", 4 output tokens
  started events: 1
  message events: 7
  tool events: 0
  done events: 1
  other events: 3
```

## Pitfalls

- **stream-json needs `-p` and `--verbose`.** `LaunchOptions.Args` refuses `--output-format` without `Print` (`ErrInvalidLaunch`); Claude Code itself requires `--verbose` for stream-json in print mode, and `--include-partial-messages` only works with stream-json.
- **Utilization in `rate_limit_event` is a fraction.** `RateLimitWindow.Utilization` 0.52 means 52%; reset times are converted from Unix seconds.
- **Partial text arrives twice.** With partial messages on, the text comes as `text_delta` events and again whole in the `assistant` line; show one or the other, not both (the neutral view emits both as `RunEventMessage`).
- **Error results are assumed to share the success shape.** The four error subtypes are named by the vendor, but no error result was recorded; fields that differ land in `Unknown`.
- **Some lines are docs-only.** `user`, `api_retry` and `task_notification` did not occur in the recorded run; their typed shapes follow the vendor docs and transcript records.
- **A non-JSON line stops the decoder.** `Next` returns the error; if your run mixes other output into stdout (a wrapper script printing text), filter it before decoding.
- **Costs are list prices.** `TotalCostUSD` and `ModelUsage[...].CostUSD` are what Claude Code computes; under a subscription no money changes hands per run.

## Related

- [Claude launch and resume](launch.md) (start the run), [Claude plan usage](usage.md), [Claude transcripts](transcript.md) (the on-disk record of the same session)
- Neutral view: [Run events](../capabilities/run-events.md)
- Example: [`examples/claudestream`](../../examples/claudestream/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
