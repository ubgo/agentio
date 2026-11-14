# Conversations example

Lists conversations newest first and looks one up by ID (`agentio.ConversationsOf`), reads a conversation's turns from a byte offset and again from the returned offset (`agentio.TurnsOf`), reads typed prompt history (`agentio.PromptHistoryOf`), asks whether a folder has history (`agentio.HistoryCheckerOf`), re-keys Claude history after a folder rename (`agentio.HistoryMoverOf`) and plans and applies a move between two Claude homes (`agentio.ConversationMoverOf`). Codex declares why it cannot move history.

It works on temp copies of the Claude and Codex fixture homes, so the moves never touch the checked-in files.

## Run

```sh
cd examples && go run ./conversations
```

Test:

```sh
cd examples && go test ./conversations/...
```

## Output

```text
== conversations ==
claude personal f85efbdb-fc2f-1e8e-7748-9f915edc8e2d /work/proj12 "Fixture title 185"
claude personal 3f474440-903e-5410-8bfb-d537b89f9547 /work/proj13 "Fixture title 166"
  by id: ~/.claude/projects/-work-proj12/f85efbdb-fc2f-1e8e-7748-9f915edc8e2d.jsonl
claude work 0b5e3a8e-0000-4000-8000-000000000099 /work/proj99 "Work home fixture"
  by id: ~/.claude-work/projects/-work-proj99/0b5e3a8e-0000-4000-8000-000000000099.jsonl
codex personal 0199e1a0-0000-7000-8000-000000000001 /work/proj "List the files"
codex personal 0199e1a0-0000-7000-8000-000000000002 /work/other "hello"
  by id: ~/.codex/sessions/2026/10/14/rollout-2026-10-14T10-00-00-0199e1a0-0000-7000-8000-000000000001.jsonl
== turns ==
claude user      model="" text="fixture text 167"
claude assistant model="claude-opus-5-5" text="fixture text 176"
claude assistant model="claude-opus-5-5" text="fixture text 180"
claude user      model="" text="fixture text 183"
claude: 8 turns, offset 23060, tokens in=12 cached=2805258 out=5281
claude: 0 new turns after offset 23060
codex user      model="gpt-5.5" text="list the files"
codex assistant model="gpt-5.5" text="There is one file: a.txt"
codex: 2 turns, offset 4236, tokens in=1200 cached=1000 out=80
codex: 0 new turns after offset 4236
== prompt history ==
claude 2026-11-13 19:35 dir="/work/proj13" "fixture text 139"
claude 2026-11-13 19:10 dir="/work/proj4" "fixture text 138"
claude 2026-11-13 19:08 dir="/work/proj4" "fixture text 137"
codex 2026-10-15 10:01 dir="" "and the sizes"
codex 2026-10-15 10:00 dir="" "list the files"
codex 2026-03-01 08:00 dir="" "hello"
== check and move ==
claude has history in /work/proj13: true
claude has history in /work/proj: false
claude has history in /work/never-used: false
claude history moved=true
claude moved /work/proj13 -> /work/proj13-renamed, backup ~/.claude/projects/-work-proj13.agentio-bak-20261113T120000Z
claude plan personal -> work: 1 conversations, 1 items, blockers []
claude applied, backup ~/.claude.agentio-bak-20261113T120000Z/projects/-work-proj13-renamed/3f474440-903e-5410-8bfb-d537b89f9547.jsonl
codex has history in /work/proj13: false
codex has history in /work/proj: true
codex has history in /work/never-used: false
codex cannot move history: Codex indexes conversations in its own SQLite database with ...
```

## Guides

- [Conversations, turns and prompt history](../../docs/capabilities/conversations.md)
- [Feature index](../../docs/reference/features.md)
