# Codex conversations, history and rollouts

Lists the conversations of a temp copy of the fixture home (rollout mtimes are set so the order is stable), finds one by id, streams its turns with a resumable offset, reads prompt history, validates models against the cached catalog, and streams a rollout record by record, reads its tail and sums its tokens.

## Run it

```sh
cd examples && go run ./codexconversations
```

`-fixture DIR` copies another Codex home instead of the default `../codex/testdata/homes/0.162.1`.

The test (`go test ./codexconversations/`) runs the same code and checks the key lines below.

## Output

```text
0199e1a0-0000-7000-8000-000000000001 "List the files" dir=/work/proj source=cli cli=0.162.1 originator=codex-tui
0199e1a0-0000-7000-8000-000000000002 "hello" dir=/work/other source=vscode cli=0.117.0 originator=Codex Desktop
in /work/other: 1
by id: created=2026-10-14T10:00:00Z branch=main
unknown id: ErrConversationNotFound
session_index title: "List the files"
has history in /work/proj: true
move: Codex indexes conversations in its own SQLite database with …
turn user      "list the files"
turn assistant "There is one file: a.txt" model=gpt-5.5 output=80 reasoning=20
resumed at offset 4236: 0 new turns, offset unchanged=true
prompt 2026-10-15 "and the sizes" session=0199e1a0-0000-7000-8000-000000000001
prompt 2026-10-15 "list the files" session=0199e1a0-0000-7000-8000-000000000001
prompts of 0199e1a0-0000-7000-8000-000000000002: 1
catalog client=0.162.1 fetched=2026-10-14
model gpt-6-luna visible=true default-effort=medium context=272000
model gpt-reserve visible=false default-effort=high context=128000
validate gpt-6-luna/low: <nil>
validate gpt-6-luna/xhigh unsupported: true
validate nope unknown: true
parsed models=1 warnings=1
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

## Guides

- [Codex conversations and history](../../docs/codex/conversations.md)
- [Codex rollout files](../../docs/codex/rollout.md)
- [Feature index](../../docs/reference/features.md)
