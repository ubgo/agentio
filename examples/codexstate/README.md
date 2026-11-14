# Codex SQLite state

Reads Codex's SQLite stores with the `codex/state` module. No database is checked in: the example builds the six stores in a temp home from the upstream migrations in `codex/state/testdata/migrations` (the SQL Codex runs), seeds a few made-up rows, then reads only through the read-only `state.DB`: schema versions, threads and their conversation projection, sub-agent edges, Desktop projects, turns, paged items, goal, queue, memories, memory jobs and filtered logs.

## Run it

```sh
cd examples && go run ./codexstate
```

`-migrations DIR` reads migrations from another directory. The example writes the demo databases through the `sqlite` driver that importing `codex/state` registers (it adds no driver import of its own); `codex/state` itself opens every file read-only.

The test (`go test ./codexstate/`) runs the same code and checks the key lines below.

## Output

```text
state_5.sqlite at state_5.sqlite schema=59 max-known=59
thread_history_1.sqlite at thread_history_1.sqlite schema=8 max-known=8
logs_2.sqlite at logs_2.sqlite schema=2 max-known=2
thread 019a0000-0000-7000-8000-00000000000b "Sub task" source=subagent archived=true pinned=false tokens=10 history=legacy sandbox=danger-full-access
thread 019a0000-0000-7000-8000-00000000000a "Fix the build" source=vscode archived=false pinned=true tokens=1234 history=paginated sandbox=workspace-write
active threads in /work/app: 1
as conversation: "Fix the build" dir=/work/app originator=Codex Desktop branch=main
unknown thread: ErrNotFound
spawned 019a0000-0000-7000-8000-00000000000b status=closed nickname=helper role=worker
project p1 "App" roots=[/work/app /work/app-docs]
turn t1 status=completed duration=1m0s
turn t2 status=failed duration=10s error={"message":"boom"}
page 1 item i1 type=userMessage known=true ordinal=4
page 1 item i2 type=agentMessage known=true ordinal=5
page 2 item i3 type=userMessage known=true ordinal=10
legacy thread turns: 0
goal "ship it" status=budget_limited used=1200 budget=1000 time=1m30s
queued q1 order=1 payload={"text":"first"}
queued q2 order=2 payload={"text":"second"}
memory thread=019a0000-0000-7000-8000-00000000000a summary="fixed the build" phase2=true
memory job stage1 status=done retries-left=3
log 2026-10-26T07:33:30Z WARN codex_core "slow tool call"
```

## Guides

- [Codex SQLite state (codex/state)](../../docs/codex/state.md)
- [Feature index](../../docs/reference/features.md)
