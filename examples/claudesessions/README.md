# claudesessions

Shows Claude Code work that is running now: the `sessions/<pid>.json` registry checked against the process table (`LiveSessions`, `LiveSessionsIn`, `IsLive`, `LiveSession.Neutral`, `SessionStatus`), Claude processes (`ClassifyProcess`, `Agent.Processes`), background jobs and the daemon (`Jobs`, `JobTimeline`, `Daemon`, `AttachCommand`, `LogsCommand`, `StopCommand`), teams and task lists (`Teams`, `TaskLists`).

The registry comes from the fixture home `claude/testdata/homes/2.1.325/userhome`; the process table is an in-memory fake in which pid 4243 was reused by another program (so its session reads as not alive). Job, daemon, team and task files are written in their recorded shapes into a temp dir used as a second home.

## Run

```sh
cd examples && go run ./claudesessions
```

`go test ./claudesessions/` runs the same code and checks key output lines.

## Output

```text
session pid 4242: fixture-session-0, status busy (neutral busy), alive true, cwd /work/proj4, v2.1.325
session pid 4243: fixture-session-1, status idle (neutral idle), alive false, cwd /work/proj6, v2.1.325
  sessions in /work/proj4: 1
  conversation 6d50c0ec live: true
  neutral: home personal, kind interactive, status busy since 2026-11-13T19:10:49Z
pid 4242 claude: cli, session true
pid 4243 python3 worker.py: not Claude Code
pid 5001 claude mcp serve: mcp-server, session false
pid 5002 node server.js: not Claude Code
  claude process 4242: cli, cwd /work/proj4
  claude process 5001: mcp-server, cwd (unknown)
job ab12 "fix-tests": state blocked, needs "approval", 230135 tokens, 1 children, unknown keys 1
  2026-11-13T18:00:00Z running: working
  2026-11-13T19:00:00Z blocked: opened a PR
  command: claude attach ab12
  command: claude logs ab12
  command: claude stop ab12
daemon: pid 4242 v2.1.325, running true, auth ok
team review: lead lead, 1 members, lead pid 4242
  member reviewer (reviewer, tmux) in /work/proj4
task list review: item 1.json, 27 raw bytes
```

## Guides

- [Claude live sessions and background work](../../docs/claude/sessions.md)
- [Feature index](../../docs/reference/features.md)
