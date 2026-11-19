# Process sessions and stop plans

Start from a row of the process table and ask what it is: which agent, which home and conversation, how sure that answer is, what the agent was last doing, and how to stop it without losing the conversation. Use it for a "what is running" monitor, a reaper of idle sessions, or a terminal tab that shows the conversation behind a pid. agentio reads and plans; it never sends a signal.

## Before you start

- Modules: `github.com/ubgo/agentio` plus the agents (`claude`, `codex`, `gemini`, `pi`).
- `Host.Processes` must be set: `HostFromOS` reads the real table; tests use `agenttest.FakeProcesses` (with `Cwds` and `Envs`) or a fixed list like [`examples/processsessions`](../../examples/processsessions/).
- A lister that also implements `agentio.ProcessEnvReader` lets agentio read the home variable a process started with (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `GEMINI_CLI_HOME`, `PI_CODING_AGENT_DIR`); `HostFromOS` does on Linux and macOS.

The example reads a fixed table: a Claude session started from a shell in a terminal, a background Claude session, `claude mcp serve`, the Codex npm launcher with its native child resuming a recorded thread, and a Codex app-server.

## Step 1 — Read the process table once

One `Host.Processes.Processes(ctx)` call is one read of the whole table, and every row already carries what a monitor shows:

| Field | macOS (`ps`) | Linux (`/proc`) | Windows (`Win32_Process`) |
|---|---|---|---|
| `PID`, `PPID` | yes | `stat` | yes |
| `Start` | `lstart`, to the second | `stat` starttime, to the clock tick | `CreationDate` |
| `Exe` | argv[0] | the `exe` link, else argv[0] | `ExecutablePath`, else argv[0] |
| `Argv` | the command split on whitespace | `cmdline` split on NUL | the command line split with the Windows rules |
| `CommandLine` | the command column, unsplit | `cmdline` with NUL turned into spaces | `CommandLine` as is |
| `RSS` (bytes) | `rss` KiB × 1024 | `stat` rss pages × page size | `WorkingSetSize` |
| `TTY` | `tty` (`ttys004`; `??` is none) | the `fd/0` link when it is `/dev/pts/N` or `/dev/ttyN` (`pts/3`); unreadable for another user's process | always "" |
| `CPUTime` | `time` (user plus system) | `stat` utime plus stime | `UserModeTime` plus `KernelModeTime` |

A zero value means unknown. A CPU percentage needs two samples taken some time apart, which is the caller's job. `Process.CommandString()` is the raw line, or `Argv` joined when the table gave none.

<!-- snippet: examples/processsessions/main.go#table -->
```go
// showTable prints the agent rows of one table read with the resource
// columns the same read fills: RSS, controlling terminal, CPU time so far,
// and the unsplit command line.
func showTable(w io.Writer, agents []agentio.Agent, table []agentio.Process) {
	for _, p := range table {
		info, ok := agentio.ClassifyProcess(agents, p)
		if !ok {
			continue
		}
		tty := p.TTY
		if tty == "" {
			tty = "-"
		}
		fmt.Fprintf(w, "pid %d %s %s session=%t rss=%dMiB tty=%s cpu=%s cmd=%q\n",
			p.PID, info.Agent, info.Kind, info.Session, p.RSS/mib, tty, p.CPUTime, p.CommandString())
	}
}
```

Classification ([Live sessions](live-sessions.md#step-2--classify-a-process)) tests a row against each agent's `ProcessMatch`: the basename of `Exe` or argv[0] (a Windows `.exe`, `.cmd` or `.bat` ending is ignored), the script an interpreter runs (`ScriptBasenames`, `ScriptContains`: `node …/bin/claude`, `node …/@openai/codex/bin/codex.js`, but not `npm install @openai/codex`), and exclusions tested against the unsplit line, so a macOS path with a space such as `Application Support/Claude` still vetoes a match. `ProcessMatch.ScriptIndex(p)` tells where an interpreter row's own arguments start.

## Step 2 — Join a process to its conversation

`agentio.FindProcessSession(ctx, agents, p)` finds the agent that claims the row and asks its `ProcessSessionReader` (`agentio.ProcessSessionsOf(a)` for one agent). The answer is an `agentio.ProcessSession`: `Agent`, `PID`, `Home`, `ConversationID`, `Dir`, `Path` (the transcript), `Version`, `Kind`, `Entrypoint` (how the agent says it was started, such as Claude `cli` or Codex `codex-tui`), `Status`, `StatusSince`, `LastActivity` and `Confidence`. `found` is false for a row that is not a conversation process (an MCP server, an npm launcher, Gemini's relaunch parent) or that nothing ties to a home.

`Confidence` says how the row was tied to the conversation:

| Confidence | Meaning | Who |
|---|---|---|
| `exact` | a file the agent wrote for this pid names the conversation, with its start time checked against the row | Claude `sessions/<pid>.json` |
| `argv` | the process's own command line names the conversation | Claude `--resume <uuid>`, Codex `resume <uuid>`, Gemini `--resume <uuid>`, pi `--session` |
| `inferred` | matched by working directory and start time | Codex, Gemini, pi without an id in argv |
| `unknown` | the home is known, the conversation is not | a new session that has not written yet |

Only `exact` is evidence about the process. Show `argv` and `inferred` joins with a marker; never kill on them.

<!-- snippet: examples/processsessions/main.go#sessions -->
```go
// sessionsAndPlans joins each agent row to its conversation and asks the
// agent how to stop it. Only a JoinExact session is evidence about the
// process; argv and inferred joins are labels to show.
func sessionsAndPlans(ctx context.Context, w io.Writer, agents []agentio.Agent, table []agentio.Process) error {
	for _, p := range table {
		info, ok := agentio.ClassifyProcess(agents, p)
		if !ok {
			continue
		}
		s, found, err := agentio.FindProcessSession(ctx, agents, p)
		if err != nil {
			return err
		}
		if found {
			fmt.Fprintf(w, "pid %d: %s home %s conversation %.8s confidence %s status %s kind %q\n",
				p.PID, s.Agent, s.Home.Name(), s.ConversationID, s.Confidence, s.Status, s.Kind)
		} else {
			fmt.Fprintf(w, "pid %d: %s %s, no session of its own\n", p.PID, info.Agent, info.Kind)
		}
		planner, ok := agentio.StopPlannerOf(agentByID(agents, info.Agent))
		if !ok {
			continue
		}
		plan, err := planner.StopPlan(p, table, s)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "  stop: %s\n", describePlan(plan))
	}
	return nil
}

// describePlan renders a StopPlan on one line.
func describePlan(plan agentio.StopPlan) string {
	if plan.Refuse != "" {
		return "refused: " + plan.Refuse
	}
	out := fmt.Sprintf("targets %v, %s then wait %s, force %t", plan.Targets, plan.Graceful, plan.Grace, plan.Force)
	if plan.Command != nil {
		out += ", run instead: " + plan.Command.Path + " " + strings.Join(plan.Command.Args, " ")
	}
	if plan.Resume != nil {
		out += ", resume: " + plan.Resume.Path + " " + strings.Join(plan.Resume.Args, " ")
	}
	return out
}
```

Per agent:

- **Claude Code** reads `<home>/sessions/<pid>.json` directly in the home the process's `CLAUDE_CONFIG_DIR` names (every discovered home when the environment is unreadable) and accepts it only when its `startedAt` is within two minutes of the row's start, so a file left by an earlier process with the same pid is skipped. `Status` is `idle`, `busy`, or `waiting` (a dialog or permission prompt waits on the user); `LastActivity` is the newer of `statusUpdatedAt` and `updatedAt`. Without a file, a UUID after `--resume`, `-r` or `--session-id` gives an `argv` join, in the home whose `projects/` holds that transcript when the environment is unreadable.
- **Codex** reads `codex resume <id>` or `codex exec resume <id>`, finding the rollout by name. Without an id it infers the rollout whose `session_meta` cwd equals the process cwd (or its `-C` / `--cd` dir) and whose creation is the closest after the process start, reading only `session_meta` heads in the date folders from a day before the start to two days after it.
- **Gemini CLI** answers only for the relaunch child. `--resume <uuid>` is an `argv` join; `--resume latest` (or an index) is the chat of the cwd's project written last; otherwise the newest chat of the project that started after the process.
- **pi** sets its process title to `pi`, which hides argv on Linux and macOS, so its join is usually `inferred`: the newest session file of the cwd started after the process (`--continue`: the newest at all). `--session <path or id>` counts where argv survives (Windows).

`LastActivity` for Codex, Gemini and pi is the transcript's modification time. Unlike terminal I/O time, it moves while a turn streams with a quiet screen and stays put while a spinner redraws.

## Step 3 — Plan the stop

`agentio.StopPlannerOf(a).StopPlan(p, table, s)` returns an `agentio.StopPlan`: the `Targets` to signal in order, the `Graceful` request (`terminate`, `interrupt`, or `none`), the `Grace` to wait before forcing, whether forcing is safe for the conversation (`Force`), whether the transcript survives the stop (`KeepsConversation`), the agent's own stop `Command` to run instead of a signal, the `Resume` command when the conversation is known, or a `Refuse` reason with no targets.

| Agent | Plan |
|---|---|
| Claude Code | `claude mcp serve` refused; a session gets SIGTERM and a 4 s grace; a background session (`Kind` `bg`) is ended with `claude stop <id>` in its home, because its supervisor respawns a killed worker, and `Force` is false |
| Codex | `app-server` refused (it serves many threads and the TUI reattaches), `mcp-server` refused; the npm launcher targets its native child from the table, then itself, and is refused when the child is not in the table; the CLI and `exec` get SIGTERM |
| Gemini CLI | the relaunch child, then its parent, whichever of the two `p` is; a management subcommand (`gemini mcp list`) refused |
| pi | SIGTERM |
| config agents | a non-session kind refused, else SIGTERM with `DefaultStopGrace` |

On a Windows Host every plan has `Graceful` `none` and `Force` true: Windows has no SIGTERM, and a console control event reaches only processes on the sender's own console, so another console's agent can only be forced. The turn in flight is lost; the transcript up to it is kept.

## Step 4 — Walk the launcher chain

`agentio.Ancestors(table, pid, maxHops)` returns a row's parents, nearest first, from the same table read. It stops at a pid missing from the table, at init and at a cycle. Which of them is a terminal, tmux, an IDE or a job runner is the caller's knowledge; agentio ships no list of tools.

<!-- snippet: examples/processsessions/main.go#ancestors -->
```go
// showAncestors walks the busy Claude session's parents, nearest first.
// Which of them is a terminal, tmux or an IDE is the caller's knowledge.
func showAncestors(w io.Writer, table []agentio.Process) {
	const maxHops = 4
	for _, a := range agentio.Ancestors(table, claudeBusyPID, maxHops) {
		fmt.Fprintf(w, "pid %d <- %d %s\n", claudeBusyPID, a.PID, a.CommandString())
	}
}
```

## Step 5 — Re-check right before signalling

A table read is a snapshot: by the time a user confirms, a pid can have ended and been reused by an unrelated program. Right before signalling each target, call `Host.ProcessAlive(ctx, pid, start, "", "")` with the start time from the read the plan came from, and act only on `LivenessAlive`. `LivenessEnded` means the process is gone or the pid was reused; `LivenessUnknown` is a reason to skip, never to force. Re-check again before forcing the targets still running after `Grace`.

<!-- snippet: examples/processsessions/main.go#safety -->
```go
// recheck is the step a caller runs right before it signals a target:
// the start time it saw in the table read must still match, so a pid that
// ended and was reused by another process is never signalled. Act only on
// LivenessAlive; LivenessUnknown is a reason to skip, never to force.
func recheck(ctx context.Context, w io.Writer, h agentio.Host, table []agentio.Process) error {
	seen := map[int]time.Time{}
	for _, p := range table {
		seen[p.PID] = p.Start
	}
	// The second entry is a start time recorded an hour before the pid's
	// current process started: the pid was reused.
	for _, c := range []struct {
		pid   int
		start time.Time
	}{{codexChildPID, seen[codexChildPID]}, {codexChildPID, seen[codexChildPID].Add(-time.Hour)}} {
		l, err := h.ProcessAlive(ctx, c.pid, c.start, "", "")
		if err != nil {
			return err
		}
		action := "skip"
		if l == agentio.LivenessAlive {
			action = "signal"
		}
		fmt.Fprintf(w, "pid %d started %s: %s, %s\n", c.pid, c.start.UTC().Format(time.TimeOnly), l, action)
	}
	return nil
}
```

The full recipe: classify, keep `Session` rows, join (`FindProcessSession`), plan (`StopPlan`); when `Refuse` is set, show it; when `Command` is set, run it instead of signalling; else re-check each target, send `Graceful` (none on Windows), wait up to `Grace`, re-check, force the survivors when `Force` is true; then offer `Resume`.

## Try it

```sh
cd examples && go run ./processsessions
```

Output:

```text
== table ==
pid 4242 claude cli session=true rss=512MiB tty=ttys004 cpu=1h12m6s cmd="claude"
pid 4500 claude cli session=true rss=300MiB tty=- cpu=0s cmd="claude --bg"
pid 5001 claude mcp-server session=false rss=90MiB tty=- cpu=0s cmd="claude mcp serve"
pid 6000 codex launcher session=false rss=60MiB tty=ttys005 cpu=0s cmd="node /usr/lib/node_modules/@openai/codex/bin/codex.js resume 0199e1a0-0000-7000-8000-000000000001"
pid 6001 codex cli session=true rss=140MiB tty=ttys005 cpu=3s cmd="/usr/lib/node_modules/@openai/codex/vendor/codex/codex resume 0199e1a0-0000-7000-8000-000000000001"
pid 6100 codex app-server session=false rss=80MiB tty=- cpu=0s cmd="codex app-server"
== sessions and stop plans ==
pid 4242: claude home personal conversation 6d50c0ec confidence exact status busy kind "interactive"
  stop: targets [4242], terminate then wait 4s, force true, resume: claude --resume 6d50c0ec-647e-18aa-84c7-04170288be89
pid 4500: claude home personal conversation aaaaaaaa confidence exact status busy kind "bg"
  stop: targets [4500], terminate then wait 4s, force false, run instead: claude stop aaaaaaaa-0000-4000-8000-000000000045, resume: claude --resume aaaaaaaa-0000-4000-8000-000000000045
pid 5001: claude mcp-server, no session of its own
  stop: refused: claude mcp serve is an MCP server for another client, not a conversation
pid 6000: codex launcher, no session of its own
  stop: targets [6001 6000], terminate then wait 4s, force true
pid 6001: codex home personal conversation 0199e1a0 confidence argv status unknown kind ""
  stop: targets [6001], terminate then wait 4s, force true, resume: codex resume 0199e1a0-0000-7000-8000-000000000001
pid 6100: codex app-server, no session of its own
  stop: refused: codex app-server serves many threads and the TUI reattaches to it; stop the sessions, not the daemon
== ancestors ==
pid 4242 <- 200 -zsh
pid 4242 <- 100 /Applications/Terminal.app/Contents/MacOS/Terminal
== kill safety ==
pid 6001 started 19:10:25: alive, signal
pid 6001 started 18:10:25: ended, skip
```

## Pitfalls

- **An inferred join is a guess.** Two sessions in one folder, or a session that has not written yet, can be joined to the wrong conversation. Only `exact` is evidence; the stop plan itself never depends on the join, only its `Resume` and `Command` do.
- **The home of a process may be unreadable.** On Windows (and for protected macOS processes) the start environment cannot be read, so agentio tries every discovered home: an exact Claude join still works through the session file, the others fall back to argv or report nothing. A home outside the discovered ones is then missed.
- **A Codex rollout appears at the first turn.** A Codex session idle for more than two days after it started is not inferred, and a process resumed with `resume --last` has no id to read.
- **pi argv is hidden.** On Linux and macOS a pi row is just `pi`; `--session` and `--continue` are only visible on Windows.
- **The launcher's own session is its child's.** The Codex npm launcher and Gemini's relaunch parent have no session of their own; call `FindProcessSession` on the child for the conversation.
- **agentio does not signal.** Mapping `terminate` to SIGTERM and `interrupt` to SIGINT, waiting and forcing are the caller's code, as is any confirmation step.

## Related

- [Live sessions and processes](live-sessions.md): sessions listed per home, process classification.
- [Host](../core/host.md): `Host.Processes`, `ProcessEnvReader`, `ProcessAlive`.
- [Platforms](../platforms.md): what each OS can read.
- Vendor pages: [Claude sessions](../claude/sessions.md), [Codex sessions](../codex/sessions.md), [Gemini CLI](../gemini/README.md), [pi](../pi/README.md).
- Example: [`examples/processsessions`](../../examples/processsessions/).
