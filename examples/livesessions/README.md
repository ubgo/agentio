# Live sessions example

Lists every home's live sessions with the pid-reuse guard (`agentio.LiveSessionsOf`, `IsLive`), classifies rows of the process table as agent processes (`agentio.ClassifyProcess`, `agentio.ProcessClassifierOf`) and joins a pid to its agent, home and conversation (`agentio.FindLiveSession`), including the Codex case where no pid can match.

The Host reports a fixed process table (two live Claude sessions, a Claude MCP server, the Claude desktop app, a Codex exec run and its npm launcher, an editor); the homes are temp copies of the fixtures with one Codex writer lock added.

## Run

```sh
cd examples && go run ./livesessions
```

Test:

```sh
cd examples && go test ./livesessions/...
```

## Output

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

## Guides

- [Live sessions and processes](../../docs/capabilities/live-sessions.md)
- [Feature index](../../docs/reference/features.md)
