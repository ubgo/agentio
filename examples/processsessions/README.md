# Process sessions example

Reads a fixed process table once and, for each agent row, prints its resource columns (RSS, terminal, CPU time, the raw command line), the conversation agentio joins it to with the join's confidence (`agentio.FindProcessSession`), the agent's stop plan (`agentio.StopPlannerOf`: targets, graceful signal, grace, refusals, a stop command, the resume command), the parent chain of one session (`agentio.Ancestors`), and the start-time re-check a caller runs right before it signals (`Host.ProcessesAlive`, one table read for every target). The resume command is printed with `StopPlan.ResumeLine`. agentio never signals; the plan is data.

The Host reports a fixed process table (a Claude session started from a shell in a terminal, a background Claude session, `claude mcp serve`, the Codex npm launcher with its native child, a Codex app-server); the homes are temp copies of the fixtures with one background session file added.

## Run

```sh
cd examples && go run ./processsessions
```

Test:

```sh
cd examples && go test ./processsessions/...
```

Guide: [Process sessions and stop plans](../../docs/capabilities/process-sessions.md).
