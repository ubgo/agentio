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
claude: 27 capabilities
  [installer binary-version identity usage cached-usage subscription token-login token-prober login-commander conversations history-checker history-mover conversation-mover prompt-history live-sessions process-classifier turns home-linker instruction-files mcp-servers skills plugins config run-events doctor live-credential launch-config]
codex: 23 capabilities
  [installer binary-version identity usage cached-usage subscription login-commander conversations history-checker prompt-history live-sessions process-classifier turns home-linker instruction-files mcp-servers skills plugins config models run-events doctor launch-config]
gemini: 14 capabilities
  [installer binary-version identity token-login conversations history-checker process-classifier home-linker instruction-files mcp-servers skills plugins config doctor]
pi: 13 capabilities
  [installer binary-version identity token-login conversations history-checker process-classifier turns instruction-files mcp-servers skills config doctor]
== unsupported ==
claude lacks models: Claude Code keeps no model catalog on disk that has a recorded shape; a static list would rot
codex lacks token-login: Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin)
codex lacks token-prober: Codex has no quota probe: no token login to probe
codex lacks history-mover: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
codex lacks conversation-mover: Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
codex lacks live-credential: Codex reads its login from auth.json or the keyring and refreshes it itself; it has no credential-helper setting
gemini lacks usage: Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
gemini lacks cached-usage: Gemini CLI leaves no usage or quota cache on disk; per-message token counts in chats are not plan usage
gemini lacks subscription: Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
gemini lacks token-prober: probing a Gemini key spends quota through a model call that is not modelled
gemini lacks login-commander: Gemini CLI has no login subcommand; it asks for the auth method in its first interactive run
gemini lacks history-mover: Gemini CLI keys chats by a project slug registry (projects.json, .project_root markers); moving them is not modelled
gemini lacks conversation-mover: Gemini CLI keys chats by a project slug registry (projects.json, .project_root markers); moving them is not modelled
gemini lacks prompt-history: the per-project prompt log (tmp/<slug>/logs.json) is recorded only from source, never from a real run
gemini lacks live-sessions: Gemini CLI writes no session lock or pid file to join a process to a chat
gemini lacks turns: Gemini chats rewrite earlier records ($rewindTo, $patch), so an offset-based turn reader would report dropped turns
gemini lacks models: Gemini CLI keeps its model catalog in its own defaults, not in a file on disk
gemini lacks run-events: the --output-format stream-json event shape is not recorded from a real run yet
gemini lacks live-credential: Gemini CLI reads its login (oauth_creds.json, the keychain or GEMINI_API_KEY) at start and refreshes OAuth itself; it has no setting that re-reads a key from a caller's command
gemini lacks launch-config: Gemini CLI has no settings flag and no typed launch options in agentio; pass flags as raw args
pi lacks usage: pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
pi lacks cached-usage: pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
pi lacks subscription: pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
pi lacks token-prober: pi has no plan or quota to probe
pi lacks login-commander: pi logs in with /login inside its interactive UI; there is no login subcommand to build
pi lacks history-mover: pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
pi lacks conversation-mover: pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
pi lacks prompt-history: pi keeps no prompt history file; prompts live only in session files
pi lacks live-sessions: pi writes no live-session registry; only the process table shows a running pi
pi lacks home-linker: pi has no documented set of files to share between agent dirs
pi lacks plugins: pi packages and extensions are code loaded into the process, not plugins with a manifest this library models
pi lacks models: pi's model catalog (models-store.json) has no recorded shape yet
pi lacks run-events: pi's --mode json event stream is recorded once but not yet modelled as a decoder
pi lacks live-credential: pi can read a key from a !command in auth.json, but when it re-runs that command is not documented or recorded
pi lacks launch-config: pi has no settings flag and no typed launch options in agentio; pass flags as raw args
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
live-credential      pure read
launch-config        disk write
```

## Guides

- [Shared capabilities and accessors](../../docs/capabilities/accessors.md)
- [Install, version and identity](../../docs/capabilities/install-identity.md)
- [Config, MCP, instructions, skills, plugins and models](../../docs/capabilities/config.md)
- [Doctor checks](../../docs/capabilities/doctor.md)
- [Built-in agents](../../docs/capabilities/builtin.md)
- [Feature index](../../docs/reference/features.md)
