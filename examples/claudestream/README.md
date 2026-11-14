# claudestream

Decodes the output of a headless Claude Code run: the `--output-format stream-json` lines (`NewStreamDecoder`, `StreamDecoder.Next`, `AsInit`, `AsEvent`, `AsAssistant`, `AsRateLimit`, `AsResult`, `AsAPIRetry`, `ErrWrongStreamMessage`), the single `--output-format json` document (`DecodeJSONResult`) and neutral run events (`StreamMessage.Neutral`).

Reads the run recorded from Claude Code 2.1.325 in `claude/testdata/stream/2.1.325`; the `api_retry` line is built in the documented shape because the recorded run had no retry. Nothing is launched.

## Run

```sh
cd examples && go run ./claudestream
```

`go test ./claudestream/` runs the same code and checks key output lines.

## Output

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

## Guides

- [Claude stream decoding](../../docs/claude/stream.md)
- [Feature index](../../docs/reference/features.md)
