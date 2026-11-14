# Codex exec event decoding

`codex exec --json` prints one JSON event per line while it works: thread and turn boundaries, items (agent messages, reasoning, commands, file changes, MCP tool calls, web searches, todo lists) and token usage. `codex.ExecDecoder` reads that stdout as typed `codex.ExecEvent` values; `NewRunDecoder` reads the same stream as neutral `agentio.RunEvent` values for code that drives several agents.

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` from [The Codex agent](agent.md) for `NewRunDecoder` and `ExecEventVersion`.
- The stdout of a `codex exec --json` process you run yourself (build the command with [`Exec`](launch.md#step-2--run-exec-headless) and `ExecOptions.JSON`). The decoders only read an `io.Reader`; they never start a process.

The snippets come from [`examples/codexlaunch`](../../examples/codexlaunch/), which feeds a recorded stream (including a diagnostic line and an event type from the future) from a string.

## Step 1 — Decode exec events

`NewExecDecoder(r)` wraps the stdout; `Next()` returns the next event and `io.EOF` at the end, skipping blank and non-JSON lines (exec may print diagnostics). An `ExecEvent` has a `Type` (`thread.started`, `turn.started`, `turn.completed`, `turn.failed`, `item.started`, `item.updated`, `item.completed`, `error`), `ThreadID`, `Usage`, `Error`, `Item` and always the full line in `Raw`. For a known item type exactly one typed pointer of `ExecItem` is set (`AgentMessage`, `Reasoning`, `Command`, `FileChange`, `MCPToolCall`, `CollabToolCall`, `WebSearch`, `TodoList`, `Error`). `ParseExecEvent` decodes one line; `a.ExecEventVersion()` is the Codex version the decoder was written against.

<!-- snippet: examples/codexlaunch/main.go#decode -->
```go
// decodeExec reads `codex exec --json` stdout one event at a time. Blank
// and non-JSON lines are skipped; unknown types keep Type and Raw.
func decodeExec(w io.Writer, a *codex.Agent, _ codex.Home) error {
	fmt.Fprintf(w, "decoder written against codex %s\n", a.ExecEventVersion())
	d := codex.NewExecDecoder(strings.NewReader(execStream))
	for {
		ev, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch {
		case ev.Type == codex.ExecThreadStarted:
			fmt.Fprintf(w, "event %s thread=%s\n", ev.Type, ev.ThreadID)
		case ev.Item != nil && ev.Item.AgentMessage != nil:
			fmt.Fprintf(w, "event %s agent_message %q\n", ev.Type, ev.Item.AgentMessage.Text)
		case ev.Item != nil && ev.Item.Command != nil:
			fmt.Fprintf(w, "event %s command %q exit=%d\n", ev.Type, ev.Item.Command.Command, *ev.Item.Command.ExitCode)
		case ev.Usage != nil:
			fmt.Fprintf(w, "event %s input=%d output=%d\n", ev.Type, ev.Usage.Input, ev.Usage.Output)
		default:
			fmt.Fprintf(w, "event %s known=%v\n", ev.Type, ev.Type.IsKnown())
		}
	}
	one, err := codex.ParseExecEvent([]byte(`{"type":"turn.failed","error":{"message":"quota"}}`))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "parsed %s error=%q\n", one.Type, one.Error.Message)
	return nil
}
```

## Step 2 — Convert to neutral run events

`a.NewRunDecoder(r)` returns an `agentio.RunDecoder` over the same stream: `started` for the thread, `message` for agent messages, `tool` for commands and tool calls, `done` with token counts at turn end, and `other` for everything else (raw line kept).

<!-- snippet: examples/codexlaunch/main.go#neutral-events -->
```go
// neutralEvents reads the same stream as agentio.RunEvent values, the
// shape code that drives several agents consumes.
func neutralEvents(w io.Writer, a *codex.Agent, _ codex.Home) error {
	d := a.NewRunDecoder(strings.NewReader(execStream))
	for {
		ev, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "run event %s", ev.Type)
		if ev.Tokens != nil {
			fmt.Fprintf(w, " reasoning=%d", ev.Tokens.Reasoning)
		}
		fmt.Fprintln(w)
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./codexlaunch
```

The decoding part of the output:

```text
decoder written against codex 0.162.1
event thread.started thread=th-1
event turn.started known=true
event item.completed agent_message "done"
event item.completed command "ls" exit=0
event turn.completed input=10 output=3
event future.event known=false
parsed turn.failed error="quota"
run event started
run event other
run event message
run event tool
run event done reasoning=1
run event other
```

## Pitfalls

- **Unknown is not an error.** An event type the decoder does not know keeps `Type` and `Raw` (`IsKnown()` false); a known type whose payload does not fit keeps `Raw` only. Check the typed pointer before using it.
- **Non-JSON lines vanish.** Warnings exec prints on stdout are skipped silently; read stderr yourself if you need them.
- **`ExitCode` is a pointer** on command items: it is nil while the command is still running (`item.started`).
- **Tool results stay raw.** MCP tool `Arguments`/`Result` and web-search `Action`/`Results` are vendor-defined and kept as `agentio.Raw`.
- **Exec JSON is not the rollout format.** Rollout files on disk use different record types; read them with [`OpenRollout`](rollout.md).
- **Reasoning tokens depend on the version.** `exec --json` usage carries reasoning token counts only since Codex 0.125.0 (`codex.FeatureInfo(codex.FeatureExecJSONReasoningTokens)`); with an older binary expect `reasoning_output_tokens` to be absent, so the count reads zero.

## Related

- [Run events (neutral)](../capabilities/run-events.md)
- [Codex launch, resume and exec](launch.md), [Codex rollout files](rollout.md)
- [Claude stream-json](../claude/stream.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
