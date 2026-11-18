# Live sessions and processes

A live session is a conversation an agent process is running right now. agentio answers three questions about them: which sessions are live in a home (`LiveSessionLister`), whether a row of the process table is an agent process and of which kind (`ProcessClassifier`, plus the `ClassifyProcess` helper over many agents), and which agent, home and conversation a pid belongs to (`FindLiveSession`). Use them before resuming or moving a conversation, in a "what is running" panel, or to clean up stale processes.

## Before you start

- Modules: `github.com/ubgo/agentio` plus the agents.
- `Host.Processes` must be set: `HostFromOS` reads `/proc` on Linux and `ps` on macOS; tests use `agenttest.FakeProcesses` or a fixed list like [`examples/livesessions`](../../examples/livesessions/).
- The homes on disk: Claude records sessions in `<home>/sessions/<pid>.json`; Codex holds a writer lock per thread in `<home>/thread-writer-locks/<thread id>.lock`.

The example copies both fixture homes into a temp dir, takes a Codex writer lock for one recorded thread, and reports a fixed process table: two live Claude sessions, a Claude MCP server, the Claude desktop app, a Codex exec run with its npm launcher, and an editor.

## Step 1 — List live sessions

`LiveSessions(home)` lists every session the home records, each checked against one read of the process table. `IsLive(home, conversationID)` is the checked yes/no for one conversation.

<!-- snippet: examples/livesessions/main.go#live -->
```go
// listLive lists every home's live sessions. Alive is the checked answer:
// the session's process is in the table with a matching start time; a
// registry entry whose process is gone stays listed with Alive false.
func listLive(w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		ls, ok := agentio.LiveSessionsOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return err
		}
		for _, home := range homes {
			sessions, err := ls.LiveSessions(home)
			if err != nil {
				return err
			}
			for _, s := range sessions {
				fmt.Fprintf(w, "%s %s pid=%d conv=%.8s dir=%s kind=%q status=%s alive=%t\n",
					a.ID(), home.Name(), s.PID, s.ConversationID, s.Dir, s.Kind, s.Status, s.Alive)
			}
			if len(sessions) > 0 {
				live, err := ls.IsLive(home, sessions[0].ConversationID)
				if err != nil {
					return err
				}
				fmt.Fprintf(w, "  IsLive(%.8s) = %t\n", sessions[0].ConversationID, live)
			}
		}
	}
	return nil
}
```

An `agentio.LiveSession` has `PID`, `ConversationID`, `Dir`, `Home`, `Kind` (a `SessionKind`: `interactive`, `bg`; open set), `Status` (a `SessionStatus`: `idle`, `busy`, `unknown`), `StatusSince`, `StartedAt` and `Alive`. `Alive` is the answer to trust: the pid is in the table and its start time matches the recorded one, so a reused pid does not count.

## Step 2 — Classify a process

`ClassifyProcess(p)` on one agent, or `agentio.ClassifyProcess(agents, p)` over many, recognises a process row. It is Pure: it runs on rows you already have, so read the table once (`Host.Processes.Processes(ctx)`) and classify every row. The result is a `ProcessInfo` with `Agent`, `Kind` (a `ProcessKind`: `cli`, `exec`, `app-server`, `mcp-server`, `desktop`, `bridge`, `launcher`) and `Session` (true for a conversation process, the kind you would reap).

<!-- snippet: examples/livesessions/main.go#classify -->
```go
// classify reads the process table once and asks the agents, in order,
// which one each row belongs to. Classification is Pure: it runs on rows
// the caller already has.
func classify(ctx context.Context, w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	rows, err := h.Processes.Processes(ctx)
	if err != nil {
		return err
	}
	for _, p := range rows {
		info, ok := agentio.ClassifyProcess(agents, p)
		if !ok {
			fmt.Fprintf(w, "pid %d %s: not an agent\n", p.PID, p.Argv[0])
			continue
		}
		fmt.Fprintf(w, "pid %d: %s %s session=%t\n", p.PID, info.Agent, info.Kind, info.Session)
	}
	// One agent alone: its own classifier.
	if pc, ok := agentio.ProcessClassifierOf(agents[0]); ok {
		_, mine := pc.ClassifyProcess(rows[len(rows)-1])
		fmt.Fprintf(w, "%s claims the last row: %t\n", agents[0].ID(), mine)
	}
	return nil
}
```

`agentio.Process` is one row: `PID`, `PPID`, `Start`, `Exe` (the executable path when known, else argv[0]) and `Argv`. The multi-agent helper asks agents in the order given and returns the first match; `builtin.Agents.All()` orders built-ins before config-defined agents.

## Step 3 — Find the session of a PID

`agentio.FindLiveSession(agents, pid)` joins a pid to its agent, home and live session by looping `Homes()` and `LiveSessions` of each agent. Use it when you start from a process (a "kill" menu, a terminal tab title) and want the conversation.

<!-- snippet: examples/livesessions/main.go#find -->
```go
// findByPID joins a pid to its agent, home and conversation by looping
// every home's live sessions. A Codex pid never matches: Codex writer
// locks do not name the process that holds them.
func findByPID(w io.Writer, agents []agentio.Agent) error {
	for _, pid := range []int{claudeBusyPID, claudeWorkPID, codexExecPID} {
		a, s, found, err := agentio.FindLiveSession(agents, pid)
		if err != nil {
			return err
		}
		if !found {
			fmt.Fprintf(w, "pid %d: no live session found\n", pid)
			continue
		}
		fmt.Fprintf(w, "pid %d: %s home %s conversation %s in %s\n", pid, a.ID(), s.Home.Name(), s.ConversationID, s.Dir)
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./livesessions
```

Output:

```text
== live sessions ==
claude personal pid=4242 conv=6d50c0ec dir=/work/proj4 kind="interactive" status=busy alive=true
claude personal pid=4243 conv=3e03d92d dir=/work/proj6 kind="interactive" status=idle alive=false
  IsLive(6d50c0ec) = true
claude work pid=4342 conv=78d3d87c dir=/work/proj15 kind="interactive" status=idle alive=true
claude work pid=4343 conv=9094af2d dir=/work/proj17 kind="interactive" status=busy alive=false
  IsLive(78d3d87c) = true
codex personal pid=0 conv=0199e1a0 dir=/work/proj kind="" status=unknown alive=true
  IsLive(0199e1a0) = true
== classify ==
pid 4242: claude cli session=true
pid 4342: claude cli session=true
pid 5001: claude mcp-server session=false
pid 5002 /Applications/Claude.app/Contents/MacOS/Claude: not an agent
pid 6000: codex launcher session=false
pid 6001: codex exec session=true
pid 7001 vim: not an agent
claude claims the last row: false
== find by pid ==
pid 4242: claude home personal conversation 6d50c0ec-647e-18aa-84c7-04170288be89 in /work/proj4
pid 4342: claude home work conversation 78d3d87c-4440-06f3-ceba-625a49fad9ec in /work/proj15
pid 6001: no live session found
```

## Pitfalls

- **`FindLiveSession` never matches a Codex pid.** Codex writer locks do not say which process holds them (several Codex processes, or one app-server writing many threads, are common), so Codex live sessions have `PID` 0 and the helper returns false for every Codex process. Use `IsLive(home, conversationID)` for Codex instead.
- **Codex `Alive` is a best guess.** A Codex lock counts as live when the table holds a Codex process (not the npm launcher) that started no later than the lock. A process's `CODEX_HOME` cannot be read, so a Codex process of another home also counts. agentio never takes the lock to test it: even a brief probe could make a Codex starting at that instant fail.
- **Listed is not running.** A Claude `sessions/*.json` entry stays after a crash; it is listed with `Alive` false (pid 4243 above). Filter on `Alive`.
- **No process table, nothing alive.** A Host without `Processes`, or a failed table read, leaves every `Alive` false without an error.
- **Homes that `Homes()` does not discover are missed** by `FindLiveSession`. A Codex home outside `~/.codex` and `CODEX_HOME`, for example, must be checked with that agent's lister directly.
- **Desktop apps are not the CLI.** The Claude desktop app and the browser extension host are excluded by the Spec's process match, so they classify as "not an agent". On macOS, `Argv` comes from `ps` split on spaces, so an argument containing a space is split too.
- **Status mapping is coarse.** Vendor statuses beyond idle and busy map onto the nearest neutral value (a Claude `shell` status reads as `busy`, a Claude `waiting` status, which means a dialog waits on the user, reads as `idle`); read the vendor type for the exact one.

## Related

- [Conversations, turns and prompt history](conversations.md): resume or move what you found.
- [Shared capabilities and accessors](accessors.md).
- [Host](../core/host.md): `Host.Processes` and `agentio.Process`.
- Vendor pages: [Claude sessions](../claude/sessions.md), [Codex sessions](../codex/sessions.md).
- [Choosing guides](../choosing/README.md). Example: [`examples/livesessions`](../../examples/livesessions/). Feature index: [features](../reference/features.md).
