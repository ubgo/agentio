# Codex live sessions and processes

Is Codex running, and on which conversation? Codex holds a lock file per conversation it is writing (`<home>/thread-writer-locks/<thread-id>.lock`); this page turns those files plus one process-table read into live sessions, lists and classifies running Codex processes, and reads the background commands the desktop app tracks.

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` and `codex.Home` from [The Codex agent](agent.md).
- `Host.Processes` lists the process table. Examples use `agenttest.FakeProcesses`; `agentio.HostFromOS()` reads the real one.

The snippets come from [`examples/codexsessions`](../../examples/codexsessions/). It adds two lock files to a copy of the fixture home (one newer and one older than the running codex) and a fake process table.

## Step 1 — List live sessions

`LiveSessions(h)` returns one `agentio.LiveSession` per lock file: `ConversationID` is the file's thread id, `StartedAt` its mtime, `Dir` the rollout's cwd when the rollout exists, `Status` unknown. `Alive` is true only when the process table holds a Codex process (not the npm launcher) that started no later than the lock; with none, the file is a crash leftover. `IsLive(h, id)` is that checked answer for one conversation; `LiveConversation(h, id)` only tests that the lock file exists.

<!-- snippet: examples/codexsessions/main.go#live -->
```go
// listLiveSessions turns thread-writer locks into live sessions. A lock
// counts as Alive only when a Codex process that started no later than
// the lock is running; otherwise it is a crash leftover. PID stays 0:
// the lock file does not say which process holds it.
func listLiveSessions(w io.Writer, a *codex.Agent, h codex.Home) error {
	ls, err := a.LiveSessions(h)
	if err != nil {
		return err
	}
	for _, s := range ls {
		fmt.Fprintf(w, "session %s alive=%v pid=%d dir=%q status=%s started=%s\n",
			s.ConversationID, s.Alive, s.PID, s.Dir, s.Status, s.StartedAt.UTC().Format(time.RFC3339))
	}
	for _, id := range []string{liveThread, staleThread} {
		live, err := a.IsLive(h, id)
		if err != nil {
			return err
		}
		exists, err := a.LiveConversation(h, id)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s IsLive=%v lock file exists=%v\n", id, live, exists)
	}
	return nil
}
```

## Step 2 — Read thread-writer locks

`ThreadLocks(h)` lists the lock files themselves (`ThreadID`, `Path`, `ModTime` = when the writer took the thread). It skips `.coordination.lock`, directories and any name that is not `<uuid>.lock`; contents are never read (they are empty). `ParseThreadLockName` applies the same name rule.

<!-- snippet: examples/codexsessions/main.go#locks -->
```go
// readThreadLocks lists the lock files themselves; .coordination.lock and
// any name that is not <uuid>.lock are skipped.
func readThreadLocks(w io.Writer, a *codex.Agent, h codex.Home) error {
	tl, err := a.ThreadLocks(h)
	if err != nil {
		return err
	}
	for _, l := range tl {
		fmt.Fprintf(w, "lock %s taken=%s\n", l.ThreadID, l.ModTime.UTC().Format(time.RFC3339))
	}
	for _, name := range []string{staleThread + lockSuffix, ".coordination.lock", "notes.lock"} {
		id, ok := codex.ParseThreadLockName(name)
		fmt.Fprintf(w, "ParseThreadLockName(%q) = %q, %v\n", name, id, ok)
	}
	return nil
}
```

## Step 3 — List processes and chat processes

`Processes(ctx)` lists running Codex processes with one process-table read (plus one cwd lookup per match) as `LiveProcess` values with a `Kind`: `cli`, `exec`, `app-server` (daemon or proxy), `mcp-server`, or `launcher` (the npm `node …/codex.js` wrapper). `ClassifyProcess(p)` classifies one row; `Session` is true for `cli` and `exec`, the processes that own a conversation. `ChatProcesses(h)` reads `process_manager/chat_processes.json`, the background commands a chat started. `DaemonSocket(h)` reports the app-server socket path when it exists.

<!-- snippet: examples/codexsessions/main.go#processes -->
```go
// listProcesses classifies running Codex processes (one process-table
// read), then reads background chat processes and the daemon socket.
func listProcesses(w io.Writer, a *codex.Agent, h codex.Home, table []agentio.Process) error {
	ps, err := a.Processes(context.Background())
	if err != nil {
		return err
	}
	for _, p := range ps {
		fmt.Fprintf(w, "process pid=%d kind=%s cwd=%q\n", p.PID, p.Kind, p.Cwd)
	}
	for _, p := range table {
		info, ok := a.ClassifyProcess(p)
		fmt.Fprintf(w, "classify pid=%d codex=%v kind=%s session=%v\n", p.PID, ok, info.Kind, info.Session)
	}

	chats, err := a.ChatProcesses(h)
	if err != nil {
		return err
	}
	for _, c := range chats {
		fmt.Fprintf(w, "chat process %q pid=%d conversation=%s started=%s\n",
			c.Command, c.OSPid, c.ConversationID, c.StartedAt.UTC().Format(time.RFC3339))
	}
	_, ok := a.DaemonSocket(h)
	fmt.Fprintf(w, "daemon socket present: %v\n", ok)
	return nil
}
```

## Try it

```sh
cd examples && go run ./codexsessions
```

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

## Pitfalls

- **`PID` is always 0.** A lock file does not say which process holds it (several Codex processes, or one app-server daemon writing many threads, are common), so `agentio.FindLiveSession` never matches a Codex pid. Match Codex sessions by conversation id.
- **`Alive` is a heuristic.** Codex itself decides liveness by trying the lock; this library never takes it, because even a brief probe would make a writer starting at that instant fail with "already has an active writer". Any Codex process started before the lock counts, including one of another home (a process's `CODEX_HOME` is not readable), so `Alive=true` can be a false positive; `false` with a lock file is a crash leftover.
- **A failed process-table read leaves every `Alive` false** rather than failing the call.
- **`LiveConversation` is unchecked.** It is true for a crash leftover too; use `IsLive` for a decision.
- **`Dir` can be empty.** Codex may create the rollout later than the lock, or never for some threads.
- **`LiveProcess.Home` is empty** unless argv names the home: the environment of another process is not readable on macOS.
- **`ChatProcesses` reads either container shape** (a top-level array, or an object holding one array); its sample recorded the entry keys only.

## Related

- [Live sessions (neutral)](../capabilities/live-sessions.md)
- [Codex conversations](conversations.md), [Codex SQLite state](state.md)
- [Claude live sessions](../claude/sessions.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
