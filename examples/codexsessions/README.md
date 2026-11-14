# Codex live sessions and processes

Adds two thread-writer lock files to a temp copy of the fixture home (one taken after the running codex started, one older) and a fake process table, then lists live sessions, checks `IsLive` against `LiveConversation`, lists the lock files, classifies processes (cli, exec, npm launcher, app-server, not codex), reads `chat_processes.json` and looks for the daemon socket.

## Run it

```sh
cd examples && go run ./codexsessions
```

`-fixture DIR` copies another Codex home instead of the default `../codex/testdata/homes/0.162.1`.

The test (`go test ./codexsessions/`) runs the same code and checks the key lines below.

## Output

```text
session 0199e1a0-0000-7000-8000-000000000001 alive=true pid=0 dir="/work/proj" status=unknown started=2026-11-13T11:00:00Z
session 01a1d83a-e1cb-70d1-bee7-7aebab05bc2b alive=false pid=0 dir="" status=unknown started=2026-11-12T09:00:00Z
0199e1a0-0000-7000-8000-000000000001 IsLive=true lock file exists=true
01a1d83a-e1cb-70d1-bee7-7aebab05bc2b IsLive=false lock file exists=true
lock 0199e1a0-0000-7000-8000-000000000001 taken=2026-11-13T11:00:00Z
lock 01a1d83a-e1cb-70d1-bee7-7aebab05bc2b taken=2026-11-12T09:00:00Z
ParseThreadLockName("01a1d83a-e1cb-70d1-bee7-7aebab05bc2b.lock") = "01a1d83a-e1cb-70d1-bee7-7aebab05bc2b", true
ParseThreadLockName(".coordination.lock") = "", false
ParseThreadLockName("notes.lock") = "", false
process pid=101 kind=cli cwd="/work/proj"
process pid=102 kind=exec cwd=""
process pid=103 kind=launcher cwd=""
process pid=104 kind=app-server cwd=""
classify pid=101 codex=true kind=cli session=true
classify pid=102 codex=true kind=exec session=true
classify pid=103 codex=true kind=launcher session=false
classify pid=104 codex=true kind=app-server session=false
classify pid=105 codex=false kind= session=false
chat process "npm run dev" pid=4242 conversation=0199e1a0-0000-7000-8000-000000000001 started=2026-11-13T11:00:00Z
daemon socket present: false
```

## Guides

- [Codex live sessions and processes](../../docs/codex/sessions.md)
- [Feature index](../../docs/reference/features.md)
