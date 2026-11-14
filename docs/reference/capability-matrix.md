# Capability matrix

Which of the 27 shared capabilities each built-in agent has. A missing capability is never empty data: its accessor returns false and `agentio.Unsupported` gives the agent's reason, listed under the table. The table is the output of [`examples/capmatrix`](../../examples/capmatrix/), which reads it from `agentio.Supported`; its test pins the rows.

| Capability | Claude Code | Codex | Gemini CLI | pi |
|---|---|---|---|---|
| `installer` | ✅ | ✅ | ✅ | ✅ |
| `binary-version` | ✅ | ✅ | ✅ | ✅ |
| `identity` | ✅ | ✅ | ✅ | ✅ |
| `usage` | ✅ | ✅ | — | — |
| `cached-usage` | ✅ | ✅ | — | — |
| `subscription` | ✅ | ✅ | — | — |
| `token-login` | ✅ | — | ✅ | ✅ |
| `token-prober` | ✅ | — | — | — |
| `login-commander` | ✅ | ✅ | — | — |
| `conversations` | ✅ | ✅ | ✅ | ✅ |
| `history-checker` | ✅ | ✅ | ✅ | ✅ |
| `history-mover` | ✅ | — | — | — |
| `conversation-mover` | ✅ | — | — | — |
| `prompt-history` | ✅ | ✅ | — | — |
| `live-sessions` | ✅ | ✅ | — | — |
| `process-classifier` | ✅ | ✅ | ✅ | ✅ |
| `turns` | ✅ | ✅ | — | ✅ |
| `home-linker` | ✅ | ✅ | ✅ | — |
| `instruction-files` | ✅ | ✅ | ✅ | ✅ |
| `mcp-servers` | ✅ | ✅ | ✅ | ✅ |
| `skills` | ✅ | ✅ | ✅ | ✅ |
| `plugins` | ✅ | ✅ | ✅ | — |
| `config` | ✅ | ✅ | ✅ | ✅ |
| `models` | — | ✅ | — | — |
| `run-events` | ✅ | ✅ | — | — |
| `doctor` | ✅ | ✅ | ✅ | ✅ |
| `live-credential` | ✅ | — | — | — |

## Why a capability is missing

- **gemini, `usage`:** Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
- **pi, `usage`:** pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
- **gemini, `cached-usage`:** Gemini CLI leaves no usage or quota cache on disk; per-message token counts in chats are not plan usage
- **pi, `cached-usage`:** pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
- **gemini, `subscription`:** Gemini CLI quota and tier come from a logged-in Code Assist API call (retrieveUserQuota, loadCodeAssist) whose responses are not recorded yet
- **pi, `subscription`:** pi has no plan or quota; it records per-message token cost in session files (pi.Agent.SessionUsage)
- **codex, `token-login`:** Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin)
- **codex, `token-prober`:** Codex has no quota probe: no token login to probe
- **gemini, `token-prober`:** probing a Gemini key spends quota through a model call that is not modelled
- **pi, `token-prober`:** pi has no plan or quota to probe
- **gemini, `login-commander`:** Gemini CLI has no login subcommand; it asks for the auth method in its first interactive run
- **pi, `login-commander`:** pi logs in with /login inside its interactive UI; there is no login subcommand to build
- **codex, `history-mover`:** Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
- **gemini, `history-mover`:** Gemini CLI keys chats by a project slug registry (projects.json, .project_root markers); moving them is not modelled
- **pi, `history-mover`:** pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
- **codex, `conversation-mover`:** Codex indexes conversations in its own SQLite database with absolute paths (state_5.sqlite threads.rollout_path), so moving its files would break its index; continue Codex work in the original home
- **gemini, `conversation-mover`:** Gemini CLI keys chats by a project slug registry (projects.json, .project_root markers); moving them is not modelled
- **pi, `conversation-mover`:** pi keys sessions by the folder they ran in and records that cwd in every header; moving them is not modelled
- **gemini, `prompt-history`:** the per-project prompt log (tmp/<slug>/logs.json) is recorded only from source, never from a real run
- **pi, `prompt-history`:** pi keeps no prompt history file; prompts live only in session files
- **gemini, `live-sessions`:** Gemini CLI writes no session lock or pid file to join a process to a chat
- **pi, `live-sessions`:** pi writes no live-session registry; only the process table shows a running pi
- **gemini, `turns`:** Gemini chats rewrite earlier records ($rewindTo, $patch), so an offset-based turn reader would report dropped turns
- **pi, `home-linker`:** pi has no documented set of files to share between agent dirs
- **pi, `plugins`:** pi packages and extensions are code loaded into the process, not plugins with a manifest this library models
- **claude, `models`:** Claude Code keeps no model catalog on disk that has a recorded shape; a static list would rot
- **gemini, `models`:** Gemini CLI keeps its model catalog in its own defaults, not in a file on disk
- **pi, `models`:** pi's model catalog (models-store.json) has no recorded shape yet
- **gemini, `run-events`:** the --output-format stream-json event shape is not recorded from a real run yet
- **pi, `run-events`:** pi's --mode json event stream is recorded once but not yet modelled as a decoder
- **codex, `live-credential`:** Codex reads its login from auth.json or the keyring and refreshes it itself; it has no credential-helper setting
- **gemini, `live-credential`:** Gemini CLI reads its login (oauth_creds.json, the keychain or GEMINI_API_KEY) at start and refreshes OAuth itself; it has no setting that re-reads a key from a caller's command
- **pi, `live-credential`:** pi can read a key from a !command in auth.json, but when it re-runs that command is not documented or recorded

## Related

- [Shared capabilities and accessors](../capabilities/accessors.md), [Built-in agents](../capabilities/builtin.md)
- [Gemini CLI](../gemini/README.md), [pi](../pi/README.md), [Claude Code](../claude/agent.md), [Codex](../codex/agent.md)
- [Feature index](features.md)
