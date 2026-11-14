# Run events example

Decodes the event stream of a headless run with the neutral decoder (`agentio.RunEventsOf`, `NewRunDecoder`, `Next` until `io.EOF`): the recorded Claude Code 2.1.325 `stream-json` run in `claude/testdata/stream/2.1.325` and a `codex exec --json` sample with a non-JSON line and an unknown event. It also prints the headless launch command (`LaunchRequest{Headless: true}`) whose stdout such a decoder reads. Nothing is run.

## Run

```sh
cd examples && go run ./runevents
```

Test:

```sh
cd examples && go test ./runevents/...
```

## Output

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

## Guides

- [Run events](../../docs/capabilities/run-events.md)
- [Feature index](../../docs/reference/features.md)
