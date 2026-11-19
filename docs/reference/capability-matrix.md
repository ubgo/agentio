# Capability matrix

Which of the 32 shared capabilities each built-in agent has. A missing capability is never empty data: its accessor returns false and `agentio.Unsupported` gives the agent's reason, listed under the table. The table is the output of [`examples/capmatrix`](../../examples/capmatrix/), which reads it from `agentio.Supported`; its test pins the rows.

| Capability | Claude Code | Codex | Gemini CLI | pi | opencode |
|---|---|---|---|---|---|
| `installer` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `binary-version` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `identity` | ✅ | ✅ | ✅ | ✅ | — |
| `usage` | ✅ | ✅ | — | — | — |
| `cached-usage` | ✅ | ✅ | — | — | — |
| `subscription` | ✅ | ✅ | — | — | — |
| `token-login` | ✅ | — | ✅ | ✅ | — |
| `token-prober` | ✅ | — | — | — | — |
| `login-commander` | ✅ | ✅ | — | — | — |
| `conversations` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `history-checker` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `history-mover` | ✅ | — | — | — | — |
| `conversation-mover` | ✅ | — | — | — | — |
| `prompt-history` | ✅ | ✅ | — | — | — |
| `live-sessions` | ✅ | ✅ | — | — | — |
| `process-classifier` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `turns` | ✅ | ✅ | — | ✅ | ✅ |
| `home-linker` | ✅ | ✅ | ✅ | — | — |
| `instruction-files` | ✅ | ✅ | ✅ | ✅ | — |
| `mcp-servers` | ✅ | ✅ | ✅ | ✅ | — |
| `skills` | ✅ | ✅ | ✅ | ✅ | — |
| `plugins` | ✅ | ✅ | ✅ | — | — |
| `config` | ✅ | ✅ | ✅ | ✅ | — |
| `models` | — | ✅ | — | — | — |
| `run-events` | ✅ | ✅ | — | — | — |
| `doctor` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `live-credential` | ✅ | — | — | — | — |
| `launch-config` | ✅ | ✅ | — | — | — |
| `process-sessions` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `stop-planner` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `turn-files` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `hooks` | ✅ | ✅ | ✅ | — | — |

## Why a capability is missing

- **opencode, `identity`:** opencode keeps one credential per model provider in auth.json; there is no single account identity to report, and this package never reads that file
- **gemini, `usage`:** Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
- **pi, `usage`:** pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
- **opencode, `usage`:** opencode has no plan or quota of its own; it bills through each provider's key, and per-message tokens and cost are not plan usage
- **gemini, `cached-usage`:** Gemini CLI leaves no usage or quota cache on disk; per-message token counts in chats are not plan usage
- **pi, `cached-usage`:** pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
- **opencode, `cached-usage`:** opencode has no plan or quota of its own; it bills through each provider's key, and per-message tokens and cost are not plan usage
- **gemini, `subscription`:** Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
- **pi, `subscription`:** pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
- **opencode, `subscription`:** opencode has no plan or quota of its own; it bills through each provider's key, and per-message tokens and cost are not plan usage
- **codex, `token-login`:** Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin)
- **opencode, `token-login`:** opencode logins are per provider in auth.json (opencode providers login); a per-provider token variable is not modelled
- **codex, `token-prober`:** Codex has no quota probe: no token login to probe
- **gemini, `token-prober`:** probing a Gemini key spends quota through a model call that is not modelled
- **pi, `token-prober`:** pi has no plan or quota to probe
- **opencode, `token-prober`:** opencode logins are per provider in auth.json (opencode providers login); a per-provider token variable is not modelled
- **gemini, `login-commander`:** Gemini CLI has no login subcommand; it asks for the auth method in its first interactive run
- **pi, `login-commander`:** pi logs in with /login inside its interactive UI; there is no login subcommand to build
- **opencode, `login-commander`:** opencode's login (opencode providers login) asks for a provider interactively; its flow is not modelled
- **codex, `history-mover`:** Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
- **gemini, `history-mover`:** Gemini CLI keys chats by a project slug registry (projects.json, .project_root markers); moving them is not modelled
- **pi, `history-mover`:** pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
- **opencode, `history-mover`:** opencode stores the session directory inside opencode.db rows (and legacy session files); rewriting them is not modelled
- **codex, `conversation-mover`:** Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
- **gemini, `conversation-mover`:** Gemini CLI keys chats by a project slug registry (projects.json, .project_root markers); moving them is not modelled
- **pi, `conversation-mover`:** pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
- **opencode, `conversation-mover`:** opencode stores the session directory inside opencode.db rows (and legacy session files); rewriting them is not modelled
- **gemini, `prompt-history`:** the per-project prompt log (tmp/<slug>/logs.json) is recorded only from source, never from a real run
- **pi, `prompt-history`:** pi keeps no prompt history file; prompts live only in session files
- **opencode, `prompt-history`:** opencode keeps no prompt log apart from the sessions themselves
- **gemini, `live-sessions`:** Gemini CLI writes no session lock or pid file to join a process to a chat
- **pi, `live-sessions`:** pi writes no live-session registry; only the process table shows a running pi
- **opencode, `live-sessions`:** opencode writes no lock or pid file per session; ProcessSessions joins a process by its command line instead
- **gemini, `turns`:** Gemini chats rewrite earlier records ($rewindTo, $patch), so an offset-based turn reader would report dropped turns
- **pi, `home-linker`:** pi has no documented set of files to share between agent dirs
- **opencode, `home-linker`:** opencode's shareable files (opencode.json, agents, commands, plugins) live in the XDG config dir, apart from the data dir, so there is nothing to link between data dirs
- **opencode, `instruction-files`:** opencode's config (opencode.json with instructions, mcp, agents and plugins in the XDG config dir and the project) is not modelled yet
- **opencode, `mcp-servers`:** opencode's config (opencode.json with instructions, mcp, agents and plugins in the XDG config dir and the project) is not modelled yet
- **opencode, `skills`:** opencode's config (opencode.json with instructions, mcp, agents and plugins in the XDG config dir and the project) is not modelled yet
- **pi, `plugins`:** pi packages and extensions are code loaded into the process, not plugins with a manifest this library models
- **opencode, `plugins`:** opencode's config (opencode.json with instructions, mcp, agents and plugins in the XDG config dir and the project) is not modelled yet
- **opencode, `config`:** opencode's config (opencode.json with instructions, mcp, agents and plugins in the XDG config dir and the project) is not modelled yet
- **claude, `models`:** Claude Code keeps no model catalog on disk that has a recorded shape; a static list would rot
- **gemini, `models`:** Gemini CLI keeps its model catalog in its own defaults, not in a file on disk
- **pi, `models`:** pi's model catalog (models-store.json) has no recorded shape yet
- **opencode, `models`:** opencode fetches its model catalog from models.dev at run time; no catalog file is modelled
- **gemini, `run-events`:** the --output-format stream-json event shape is not recorded from a real run yet
- **pi, `run-events`:** pi's --mode json event stream is recorded once but not yet modelled as a decoder
- **opencode, `run-events`:** the opencode run --format json event shape is not recorded from a real run yet
- **codex, `live-credential`:** Codex reads its login from auth.json or the keyring and refreshes it itself; it has no credential-helper setting
- **gemini, `live-credential`:** Gemini CLI reads its login (oauth_creds.json, the keychain or GEMINI_API_KEY) at start and refreshes OAuth itself; it has no setting that re-reads a key from a caller's command
- **pi, `live-credential`:** pi can read a key from a !command in auth.json, but when it re-runs that command is not documented or recorded
- **opencode, `live-credential`:** opencode reads provider keys from auth.json and the environment at start; it has no setting that re-reads a key from a caller's command
- **gemini, `launch-config`:** Gemini CLI has no settings flag and no typed launch options in agentio; pass flags as raw args
- **pi, `launch-config`:** pi has no settings flag and no typed launch options in agentio; pass flags as raw args
- **opencode, `launch-config`:** opencode's launch options beyond the neutral request (agent, variant, port, attach) are not typed in agentio; pass them as raw args
- **pi, `hooks`:** pi has no shell-command hooks: its extensions are TypeScript modules loaded into the process, so there is no settings key to add a command to
- **opencode, `hooks`:** opencode has no shell-command hooks; its plugins are JavaScript or TypeScript modules

## Related

- [Shared capabilities and accessors](../capabilities/accessors.md), [Built-in agents](../capabilities/builtin.md)
- [Gemini CLI](../gemini/README.md), [pi](../pi/README.md), [opencode](../opencode/README.md), [Claude Code](../claude/agent.md), [Codex](../codex/agent.md)
- [Feature index](features.md)
