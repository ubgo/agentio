# claudeconversations

Walks Claude Code's past work in one home: the stats cache (`Agent.Stats`, `ParseStats`), the conversation list (`Agent.Conversations`, `Agent.Conversation`, `HasHistory`), turns from an offset (`Agent.Turns`), a move plan (`Agent.PlanMove`, `ErrAmbiguousID`), transcripts (`OpenTranscript`, `TranscriptAt`, `ParseRecord`, `TranscriptTail`, `LastAssistant`, `SessionCost`, `PRLinks`, `InternalRecords`) and the prompt history with pasted content (`PromptHistory`, `ParseHistoryLine`, `PastedContent`).

Copies the fixture home `claude/testdata/homes/2.1.325/userhome` into a temp dir (`~` in the output) and appends real-shaped records the fixture lacks: a `cost-state` and a `pr-link` line in one transcript, and a history entry whose paste points into `paste-cache`. Transcript modification times are pinned so the order is stable. The checked-in fixture is never changed.

## Run

```sh
cd examples && go run ./claudeconversations
```

`go test ./claudeconversations/` runs the same code and checks key output lines.

## Output

```text
stats v5 computed 2026-10-26: 141 sessions, 93795 messages since 2026-10-11
  2026-10-24: 8944 messages, 8 sessions, 2595 tool calls
  2026-10-25: 10933 messages, 0 sessions, 3239 tool calls
  2026-10-26: 13460 messages, 14 sessions, 3323 tool calls
  model claude-opus-5-5: 39830885 output tokens, $0.00
  model claude-sonnet-5-5: 604908 output tokens, $0.00
  longest session: 98h47m0s, 670 messages
  busiest hour: 17:00 (23 sessions started)
conversations in personal: 2
  3f474440  "Fixture title 166" (ai-title) dir /work/proj13, v2.1.325, updated 2026-11-13T12:00:00Z
  f85efbdb  "Fixture title 185" (ai-title) dir /work/proj12, v2.1.325, updated 2026-11-13T11:00:00Z
  only /work/proj13, limit 1: 1
conversation 3f474440: 47179 bytes at ~/.claude/projects/-work-proj13/3f474440-903e-5410-8bfb-d537b89f9547.jsonl
  history for /work/proj13: true
  history for /work/never-used: false
  unknown id: true
turns: 1 user, 11 assistant, offset 47179
  last turn: assistant, model claude-opus-5-5, 57 output tokens
  turns after offset 47179: 0
move plan personal -> work for /work/proj13: 1 conversations, 1 items, 0 blockers, 0 left
  ~/.claude/projects/-work-proj13/3f474440-903e-5410-8bfb-d537b89f9547.jsonl -> ~/.claude-work/projects/-work-proj13/3f474440-903e-5410-8bfb-d537b89f9547.jsonl
  prefix 3f47: claude: ambiguous conversation prefix "3f47" matches 2 conversations
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
prompt history, newest 3:
  2026-11-13T19:43:20Z /work/proj4 "fix this [Pasted text #1]" (1 pastes)
  2026-11-13T19:35:10Z /work/proj13 "fixture text 139" (0 pastes)
  2026-11-13T19:10:49Z /work/proj4 "fixture text 138" (0 pastes)
  prompts typed in /work/proj4: 6
  parsed line: "hello" at 2026-11-13T18:37:08Z, 0 pastes
paste #1 (text, hash true): "a long pasted log line"
  path-like hash refused: true
```

## Guides

- [Claude stats](../../docs/claude/stats.md)
- [Claude conversations](../../docs/claude/conversations.md)
- [Claude transcripts](../../docs/claude/transcript.md)
- [Claude prompt history](../../docs/claude/history.md)
- [Feature index](../../docs/reference/features.md)
