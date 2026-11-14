# Run events

A headless run is an agent started with a prompt and no terminal, printing a machine-readable event stream on stdout (`claude -p --output-format stream-json`, `codex exec --json`). The `RunEvents` capability turns that stream into neutral `agentio.RunEvent` values, so one loop can follow a Claude Code run and a Codex run the same way: when the conversation started, what the agent said, which tools it ran, and the token counts at the end.

## Before you start

- Modules: `github.com/ubgo/agentio` plus the agents.
- A stream: the stdout of a command built with `LaunchRequest{Headless: true}`, a file, or any `io.Reader`. The decoder is Pure: it reads only the reader you give it.
- agentio builds the command but does not run it; start it yourself (`os/exec` with `cmd.Path`, `cmd.Args`, `cmd.Env`, `cmd.Dir`) and pass its stdout pipe to the decoder.

[`examples/runevents`](../../examples/runevents/) decodes a recorded Claude Code 2.1.325 headless run (`claude/testdata/stream/2.1.325/stream-partial.jsonl`) and a `codex exec --json` sample, and prints the headless launch command of each agent. Nothing is run.

## Step 1 — Decode a headless run

`RunEventsOf(a)` returns the factory; `NewRunDecoder(r)` returns a `RunDecoder` whose `Next()` yields one event at a time and `io.EOF` at the end of the stream.

The launch whose stdout you decode:

<!-- snippet: examples/runevents/main.go#launch -->
```go
// headlessCommand builds the non-interactive launch whose stdout is the
// event stream: Headless picks the vendor's machine-readable output.
func headlessCommand(a agentio.Agent) (agentio.Command, error) {
	return a.Launch(a.DefaultHome().Target(), agentio.LaunchRequest{Prompt: examplePrompt, Headless: true})
}
```

The decode loop:

<!-- snippet: examples/runevents/main.go#decode -->
```go
// decode reads every event until io.EOF. Unknown vendor events arrive as
// RunEventOther with the full line in Raw, so a new vendor event never
// stops the loop; Done carries the run's token counts. Partial-message
// events carry no text, so empty messages are counted but not printed.
func decode(w io.Writer, a agentio.Agent, out io.Reader) error {
	re, ok := agentio.RunEventsOf(a)
	if !ok {
		return fmt.Errorf("%s has no run events decoder", a.ID())
	}
	d := re.NewRunDecoder(out)
	counts := map[agentio.RunEventType]int{}
	for {
		ev, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		counts[ev.Type]++
		switch ev.Type {
		case agentio.RunEventStarted:
			fmt.Fprintf(w, "started conversation %s\n", ev.ConversationID)
		case agentio.RunEventMessage:
			if ev.Text != "" {
				fmt.Fprintf(w, "message %q\n", truncate(ev.Text))
			}
		case agentio.RunEventDone:
			if ev.Tokens != nil {
				fmt.Fprintf(w, "done tokens in=%d cached=%d out=%d reasoning=%d\n", ev.Tokens.Input, ev.Tokens.CachedInput, ev.Tokens.Output, ev.Tokens.Reasoning)
			}
		case agentio.RunEventError:
			fmt.Fprintf(w, "error %q\n", ev.Text)
		}
	}
	for _, t := range agentio.RunEventTypeValues() {
		if counts[t] > 0 {
			fmt.Fprintf(w, "  %-7s x%d\n", t, counts[t])
		}
	}
	return nil
}
```

Each `RunEvent` has `Type`, `ConversationID`, `Text`, `Tokens` (a `*TokenCounts` with `Input`, `CachedInput`, `CacheWrite`, `Output`, `Reasoning`) and `Raw`, which is always the full vendor line. `RunEventType` is a closed enum:

| Type | Meaning |
|---|---|
| `started` | the run began; `ConversationID` is the session or thread to resume later |
| `message` | assistant text (Claude partial-message events arrive with empty `Text`) |
| `tool` | the agent ran a tool, a command, a file change or a web search |
| `usage` | a token tally mid-run |
| `done` | the run finished; `Tokens` holds the run's totals when the vendor sent them |
| `error` | the run or one item failed; `Text` is the vendor message |
| `other` | any event the neutral view does not classify, with `Raw` kept |

When you need the vendor detail (Claude's tool-use blocks, Codex item types), decode `Raw` with the vendor decoder: [Claude stream](../claude/stream.md), [Codex exec](../codex/exec.md).

## Try it

```sh
cd examples && go run ./runevents
```

Output:

```text
== claude ==
launch: claude -p --output-format stream-json --verbose -- "list the files"
started conversation 00000000-0000-4000-8000-000000000001
message "ok"
message "ok"
done tokens in=2 cached=11370 out=4 reasoning=0
  other   x3
  started x1
  message x7
  done    x1
== codex ==
launch: codex exec --json "list the files"
started conversation 0199e1a0-0000-7000-8000-000000000001
message "I will list the files."
done tokens in=1200 cached=1000 out=80 reasoning=16
  other   x2
  started x1
  message x1
  tool    x1
  done    x1
```

The Codex sample holds a non-JSON line (`warning: not json`) and an event type this library does not know (`future.event`); the first is skipped, the second arrives as `other`, and the loop continues.

## Pitfalls

- **End of stream is `io.EOF`.** Compare with `errors.Is(err, io.EOF)` and stop; any other error is a read failure of the underlying reader.
- **Unknown events are not errors.** A new vendor event type arrives as `RunEventOther` with `Raw` set. Never treat `other` as a failure, and never drop `Raw` if you log runs.
- **Empty message events are normal.** Claude's stream-json with partial messages emits message events for deltas and blocks without text; count them, print only non-empty `Text`.
- **`Tokens` can be nil.** Check before dereferencing; only some events carry a tally.
- **Headless needs the vendor's support.** A config-defined agent's `Launch` returns `ErrUnsupported` for `Headless: true`, and it has no `RunEvents` capability: a Spec cannot describe an event format.
- **The decoder does not run anything.** Starting the process, its context and its exit code are yours; a `done` event means the vendor reported the end, not that the process exited zero.

## Related

- [Launch commands](../core/launch.md): `LaunchRequest.Headless`.
- [Shared capabilities and accessors](accessors.md).
- Vendor pages: [Claude stream](../claude/stream.md), [Claude launch](../claude/launch.md), [Codex exec](../codex/exec.md), [Codex launch](../codex/launch.md).
- [Choosing guides](../choosing/README.md). Example: [`examples/runevents`](../../examples/runevents/). Feature index: [features](../reference/features.md).
