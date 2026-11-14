# agentinfo example

A machine report: everything agentio can read about every built-in coding agent on this computer. For each agent it prints the install and tested-version status, the supported capabilities, and for each home the vendor paths, a value-free settings summary, Codex profiles, providers and hooks, config layers, project trust, launch defaults, MCP servers, instruction files, skills, plugins, the signed-in identity (personal values redacted), cached usage, recent conversations, live sessions and doctor checks; then the running agent processes.

It reads disk and the process table only. It runs a vendor binary only with `--runner` (which may touch the login) and calls a vendor endpoint only with `--network` (one request per home; may 429).

Unlike the other examples, the command reads the real machine by default (through `agentio.HostFromOS`, in `runOS`), because it is the report `task example` prints. The report itself is built by `run(ctx, host, options, w)`, which takes the Host as a parameter; `main_test.go` runs it on copies of the Claude Code 2.1.325 and Codex 0.162.1 fixture homes with `agenttest` fakes whose HTTP client and process runner fail the test on use.

## Run

On this machine (disk reads only):

```sh
cd examples && go run ./agentinfo
```

Flags: `--json` (the full typed report as JSON; personal values marshal redacted), `--dir PATH` (project folder for per-project reads; default the current directory), `--runner`, `--network`. `task example` runs the same command from the repository root.

Test (fixture homes, never the real machine):

```sh
cd examples && go test ./agentinfo/...
```

## Output

An excerpt of the report the test produces on the fixture homes (user home shown as `~`, no binary on `PATH`):

```text
=== Claude Code (claude) ===
install:   not installed (agentio: binary not found on PATH: claude)
compat:    unknown (tested 2.1.323..2.1.325, installed -)
supported: installer binary-version identity usage cached-usage subscription token-login token-prober login-commander conversations history-checker history-mover conversation-mover prompt-history live-sessions process-classifier turns home-linker instruction-files mcp-servers skills plugins config run-events doctor

--- home ~/.claude (default)  name=personal
layer:     local    ~/project/.claude/settings.local.json ok
layer:     project  ~/project/.claude/settings.json ok
layer:     user     ~/.claude/settings.json ok
trust:     unknown
defaults:  model=sonnet autonomy=full
mcp (3):
  proj-db              stdio  project  fixture-db-mcp --ro env=[DB_PASSWORD]  (~/project/.mcp.json)
  fixture-http         http   user     https://mcp.example.com/mcp env=[]  (~/.claude.json)
  fixture-stdio        stdio  user     fixture-mcp serve env=[FIXTURE_TOKEN]  (~/.claude.json)
identity:  logged-in=true email=[redacted] org=[redacted] plan=Max (20x) billing=subscription credential=file expires=2026-11-13 20:56 expired=false
cached usage (cache, as of 2026-10-28 08:38, 16d3h old):
  session        short    6.0%  len=5h0m0s resets=2026-10-28 12:59
  weekly_all     week     2.0%  len=168h0m0s resets=2026-11-04 05:59
  weekly_scoped  week     0.0%  len=168h0m0s resets=2026-11-04 06:00
live sessions (2):
  pid 4242 6d50c0ec-647e-18aa-84c7-04170288be89 interactive alive=true status=busy since=2026-11-13 19:10 /work/proj4
  pid 4243 3e03d92d-0f60-fb93-80b9-9524ba57cf17 interactive alive=false status=idle since=2026-11-13 19:36 /work/proj6

=== Codex (codex) ===
--- home ~/.codex (default)  name=personal
profile:   ollama-launch        legacy ~/.codex/config.toml
profile:   work                 v2     ~/.codex/work.config.toml
skills (3): squad[user] imagegen[system] github:pr-review[plugin]
identity:  logged-in=true email=[redacted] org= plan=Plus billing=subscription credential=file expires=2026-09-21 14:13 expired=true
cached usage (transcript, as of 2026-10-14 10:00, 30d2h old):
  primary        week    12.0%  len=168h0m0s resets=2026-09-21 14:13
```

## Guides

- [Shared capabilities and accessors](../../docs/capabilities/accessors.md)
- [Install, version and identity](../../docs/capabilities/install-identity.md)
- [Usage and subscription](../../docs/capabilities/usage.md)
- [Config, MCP, instructions, skills, plugins and models](../../docs/capabilities/config.md)
- [Live sessions and processes](../../docs/capabilities/live-sessions.md)
- [Doctor checks](../../docs/capabilities/doctor.md)
- [Built-in agents](../../docs/capabilities/builtin.md)
- [Feature index](../../docs/reference/features.md)
