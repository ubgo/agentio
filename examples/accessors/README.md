# Accessors example

Walks every shared capability of the built-in agents through the typed accessors (`agentio.CachedUsageOf`, `agentio.ConfigOf`, ...) with no type assertion: which capabilities each agent has (`agentio.Supported`), why one is missing (`agentio.Unsupported`), what each costs (`Capability.Cost`), the install, binary version and identity, config layers, MCP servers, instruction files, skills, plugins, models, doctor checks, `agentio.VersionCheck`, and `builtin.New` / `Agents.ByID` / config-defined agents.

It copies the recorded Claude Code 2.1.325 user home and the Codex 0.162.1 home (as `~/.codex`) into a temp dir, lays out a fake native Claude install and a fake npm Codex install there, and answers `--version` from a canned runner. It never runs a real binary, opens the keychain or uses the network.

## Run

```sh
cd examples && go run ./accessors
```

Test:

```sh
cd examples && go test ./accessors/...
```

## Output

The first four sections; the full output continues with install, binary version, identity, config, MCP, instructions, skills and plugins, models, doctor, version check, by id and custom.

```text
== homes ==
claude personal ~/.claude (cached usage 6%)
claude work ~/.claude-work
codex personal ~/.codex (cached usage 12%)
== supported ==
claude: 25 capabilities
  [installer binary-version identity usage cached-usage subscription token-login token-prober login-commander conversations history-checker history-mover conversation-mover prompt-history live-sessions process-classifier turns home-linker instruction-files mcp-servers skills plugins config run-events doctor]
codex: 22 capabilities
  [installer binary-version identity usage cached-usage subscription login-commander conversations history-checker prompt-history live-sessions process-classifier turns home-linker instruction-files mcp-servers skills plugins config models run-events doctor]
== unsupported ==
claude lacks models: Claude Code keeps no model catalog on disk that has a recorded shape; a static list would rot (OD-15)
codex lacks token-login: Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin)
codex lacks token-prober: Codex has no quota probe: no token login to probe
codex lacks history-mover: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
codex lacks conversation-mover: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
== cost ==
installer            disk read
binary-version       process read (may touch login)
identity             process read
usage                network read
cached-usage         disk read
subscription         network read
token-login          pure read
token-prober         quota read
login-commander      pure read
conversations        disk read
history-checker      disk read
history-mover        disk write
conversation-mover   disk write
prompt-history       disk read
live-sessions        process read
process-classifier   pure read
turns                disk read
home-linker          disk write
instruction-files    disk read
mcp-servers          disk read
skills               disk read
plugins              disk read
config               disk read
models               disk read
run-events           pure read
doctor               disk read
```

## Guides

- [Shared capabilities and accessors](../../docs/capabilities/accessors.md)
- [Install, version and identity](../../docs/capabilities/install-identity.md)
- [Config, MCP, instructions, skills, plugins and models](../../docs/capabilities/config.md)
- [Doctor checks](../../docs/capabilities/doctor.md)
- [Built-in agents](../../docs/capabilities/builtin.md)
- [Feature index](../../docs/reference/features.md)
