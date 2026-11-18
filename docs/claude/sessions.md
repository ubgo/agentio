# Claude live sessions and background work

What Claude Code is doing right now: each running session registers itself in `<home>/sessions/<pid>.json`, background jobs keep state under `<home>/jobs/`, a supervisor daemon writes lock and status files, and agent teams and task lists have their own folders. The `claude` module reads all of these, checks the registry against the process table, and classifies Claude processes it finds there. Use it for a "which sessions are running and where" panel, to refuse moving a folder while Claude works in it, or to list background jobs that wait for you.

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- `Host.Processes` (a process lister; `HostFromOS` reads the real table) for `Alive`, `Processes` and `Daemon.Running`. Without one, entries are listed with `Alive` false.

The snippets come from [`examples/claudesessions`](../../examples/claudesessions/). The registry comes from the checked-in 2.1.325 fixture home; the process table is an in-memory fake; job, daemon, team and task files are written in their recorded shapes (placeholder values) into a temp dir used as a second home.

## Step 1 — List live sessions

`a.LiveSessions(h)` reads every `sessions/<pid>.json` (never the `*.key` files next to them) and returns `[]claude.LiveSession` sorted by pid: `PID`, `SessionID` (the conversation), `Cwd`, `Name`, `Version`, `Kind`, `Entrypoint`, `Status` (a `claude.SessionStatus`: `StatusIdle`, `StatusBusy`, `StatusShell`, `StatusWaiting`), `WaitingFor` (the vendor's reason text while waiting, such as `dialog open`), `StartedAt`, `UpdatedAt`, `StatusUpdatedAt`, `JobID`, `BridgeSessionID`, peer fields, `Path` and `Unknown`. `Alive` is true only when the pid is in the process table and its start time is within two minutes of `StartedAt`: a pid reused by another program reads as not alive.

`a.LiveSessionsIn(h, dir)` keeps the entries whose `Cwd` is `dir`; `a.IsLive(h, conversationID)` reports an alive session running that conversation. `LiveSession.Neutral(h)` and `SessionStatus.Neutral()` project onto `agentio.LiveSession` and `agentio.SessionStatus` (`shell` counts as busy: it is running a command, not waiting for input).

<!-- snippet: examples/claudesessions/main.go#live -->
```go
// liveSessions reads the sessions/<pid>.json registry and marks each entry
// Alive only when its pid is running and started at the recorded time: a
// pid reused by another program reads as not alive.
func liveSessions(w io.Writer, a *claude.Agent, h claude.Home) error {
	all, err := a.LiveSessions(h)
	if err != nil {
		return err
	}
	for _, s := range all {
		fmt.Fprintf(w, "session pid %d: %s, status %s (neutral %s), alive %t, cwd %s, v%s\n",
			s.PID, s.Name, s.Status, s.Status.Neutral(), s.Alive, s.Cwd, s.Version)
	}
	in, err := a.LiveSessionsIn(h, busySessionCwd)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  sessions in %s: %d\n", busySessionCwd, len(in))
	live, err := a.IsLive(h, busySessionID)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  conversation %s live: %t\n", busySessionID[:8], live)
	if len(in) > 0 {
		n := in[0].Neutral(h)
		fmt.Fprintf(w, "  neutral: home %s, kind %s, status %s since %s\n", n.Home.Name(), n.Kind, n.Status, n.StatusSince.UTC().Format(time.RFC3339))
	}
	return nil
}
```

## Step 2 — List agent processes

`claude.ClassifyProcess(p)` says whether one process-table row is Claude Code: it matches the `claude` basename of the executable or `argv[0]`, excludes the desktop app and the browser extension host, and reports `claude mcp serve` as `agentio.ProcessMCPServer` (Claude Code serving MCP to another client, not a conversation) and every other match as a CLI session. `a.Processes(ctx)` applies it to `Host.Processes` and returns `[]claude.LiveProcess` with a best-effort `Cwd` each; it never runs `claude`.

<!-- snippet: examples/claudesessions/main.go#processes -->
```go
// processes classifies the process table: `claude mcp serve` is an MCP
// server, any other claude process is a CLI session; other programs and
// the desktop app are not Claude Code.
func processes(ctx context.Context, w io.Writer, a *claude.Agent) error {
	for _, p := range processTable().list {
		info, ok := claude.ClassifyProcess(p)
		if !ok {
			fmt.Fprintf(w, "pid %d %s: not Claude Code\n", p.PID, strings.Join(p.Argv, " "))
			continue
		}
		fmt.Fprintf(w, "pid %d %s: %s, session %t\n", p.PID, strings.Join(p.Argv, " "), info.Kind, info.Session)
	}
	list, err := a.Processes(ctx)
	if err != nil {
		return err
	}
	for _, lp := range list {
		cwd := lp.Cwd
		if cwd == "" {
			cwd = "(unknown)"
		}
		fmt.Fprintf(w, "  claude process %d: %s, cwd %s\n", lp.PID, lp.Kind, cwd)
	}
	return nil
}
```

## Step 3 — Read jobs and the daemon

Background sessions (`claude --bg`) are jobs. `a.Jobs(h)` reads every `jobs/<id>/state.json` into `[]claude.Job` (`State`, `Detail`, `Needs`, `Intent`, `Name`, `Tokens`, `SessionID`, `Cwd`, `CLIVersion`, `RespawnFlags`, `Children` links such as a PR, times, `Unknown`). `a.JobTimeline(h, id)` reads the last 256 KiB of `jobs/<id>/timeline.jsonl` as `[]claude.JobEvent`; an unknown id is `agentio.ErrNotFound`. `a.Daemon(h)` reads the supervisor's `daemon.lock`, `daemon.status.json`, `daemon/roster.json` and `daemon-auth-status.json` into a `claude.Daemon`; `Running` applies the same pid-and-start-time check as `Alive`. `a.AttachCommand(id)`, `a.LogsCommand(id)` and `a.StopCommand(id)` build `claude attach|logs|stop <id>` (Pure; you run them).

<!-- snippet: examples/claudesessions/main.go#jobs -->
```go
// background reads background jobs, a job's timeline and the supervisor
// daemon's files, and builds (never runs) the attach, logs and stop
// commands for a job.
func background(w io.Writer, a *claude.Agent, h claude.Home) error {
	jobs, err := a.Jobs(h)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		fmt.Fprintf(w, "job %s %q: state %s, needs %q, %d tokens, %d children, unknown keys %d\n",
			j.ID, j.Name, j.State, j.Needs, j.Tokens, len(j.Children), len(j.Unknown))
		events, err := a.JobTimeline(h, j.ID)
		if err != nil {
			return err
		}
		for _, ev := range events {
			fmt.Fprintf(w, "  %s %s: %s\n", ev.At.UTC().Format(time.RFC3339), ev.State, ev.Text)
		}
		for _, c := range []agentio.Command{a.AttachCommand(j.ID), a.LogsCommand(j.ID), a.StopCommand(j.ID)} {
			fmt.Fprintf(w, "  command: %s\n", c)
		}
	}
	d, err := a.Daemon(h)
	if err != nil {
		return err
	}
	if d.Lock != nil {
		fmt.Fprintf(w, "daemon: pid %d v%s, running %t", d.Lock.PID, d.Lock.Version, d.Running)
		if d.Auth != nil {
			fmt.Fprintf(w, ", auth %s", d.Auth.Status)
		}
		fmt.Fprintln(w)
	}
	return nil
}
```

## Step 4 — Read teams and task lists

`a.Teams(h)` reads every `teams/<team>/config.json` into `[]claude.Team` (`Name`, `CreatedAt`, `LeadAgentID`, `LeadSessionID`, `LeadProcess`, `Members` with agent type, backend, cwd and pane, `Unknown`). `a.TaskLists(h)` reads each `tasks/<list>/` folder into a `claude.TaskList` whose items keep their name and raw body: their shape has no recorded sample yet, so the library does not invent one.

<!-- snippet: examples/claudesessions/main.go#teams -->
```go
// teams reads agent teams and task lists. Task items have no recorded
// shape yet, so their bodies stay raw.
func teams(w io.Writer, a *claude.Agent, h claude.Home) error {
	ts, err := a.Teams(h)
	if err != nil {
		return err
	}
	for _, t := range ts {
		fmt.Fprintf(w, "team %s: lead %s, %d members", t.Name, t.LeadAgentID, len(t.Members))
		if t.LeadProcess != nil {
			fmt.Fprintf(w, ", lead pid %d", t.LeadProcess.PID)
		}
		fmt.Fprintln(w)
		for _, m := range t.Members {
			fmt.Fprintf(w, "  member %s (%s, %s) in %s\n", m.Name, m.AgentType, m.BackendType, m.Cwd)
		}
	}
	lists, err := a.TaskLists(h)
	if err != nil {
		return err
	}
	for _, l := range lists {
		for _, it := range l.Items {
			fmt.Fprintf(w, "task list %s: item %s, %d raw bytes\n", l.Name, it.Name, len(it.Raw))
		}
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudesessions
```

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

## Pitfalls

- **The registry is advisory.** Claude Code leaves `sessions/<pid>.json` behind when a process dies; always check `Alive`. Unreadable or malformed entries are skipped silently (the [doctor](doctor.md) counts stale ones as `stale-live-sessions`).
- **Pid reuse is real.** A pid in the table is not enough: the start time must match `StartedAt` within two minutes. A process-table row with no start time is trusted on the pid alone.
- **No process lister, no liveness.** If `Host.Processes` is nil or its read fails, every entry has `Alive` false and `IsLive` returns false; `Processes` returns an `agentio.ErrUnsupported` error when the lister is nil.
- **On macOS argv is split on spaces.** The fake table above is exact; a real `ps`-based row splits an argument containing a space, which can affect the `mcp serve` check for unusual paths.
- **`Cwd` of a process is best effort.** It is empty when the host cannot read it (another user's process, a restricted platform).
- **Each home has its own registry.** Loop over `a.Homes()` to see every running session; a move plan does this for the two homes involved.
- **Jobs and teams keep unknown keys.** New fields land in `Unknown`; daemon workers and task items stay raw until a sample is recorded.

## Related

- [Claude conversations](conversations.md#step-4--plan-a-move) (live sessions block a move), [Claude launch and resume](launch.md), [Claude Runner](runner.md#step-3--read-background-agents-and-auto-mode) (`claude agents --json`), [Claude doctor](doctor.md)
- Neutral view: [Live sessions and processes](../capabilities/live-sessions.md)
- Example: [`examples/claudesessions`](../../examples/claudesessions/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
