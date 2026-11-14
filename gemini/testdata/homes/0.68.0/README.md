# Gemini CLI 0.68.0 fixture home

`userhome/.gemini` is a Gemini CLI home and `project/` a workspace for it. Every value is invented: no real path, email, token or session id. Tests copy both into a temp dir and replace two placeholders: `@PROJECT@` with the temp project's absolute path (written with `json.Marshal` where it sits inside JSON) and `@PROJECT_HASH@` with its sha256.

What each file is based on:

| File | Basis |
|---|---|
| `projects.json`, `state.json`, `tmp/demo/.project_root`, `tmp/demo/logs.json`, `history/demo/.project_root` | observed live: created by `gemini` 0.68.0 on a throwaway HOME with no login (shapes kept, paths replaced) |
| `settings.json` | the 0.68.0 settings schema (keys real, values invented); the comment lines are valid because the CLI strips comments |
| `tmp/demo/chats/*.jsonl` and the legacy `.json` chat | source-derived from the bundled chat recording service (no logged-in run recorded) |
| `oauth_creds.json` | source-derived: field names from the core credential cache code; the tokens are fake strings |
| `google_accounts.json` | source-derived from the core user account manager |
| `trustedFolders.json`, `mcp-server-enablement.json`, `extensions/extension-enablement.json` | source-derived from the CLI's trust, MCP enablement and extension enablement managers |
| `extensions/demo-ext/gemini-extension.json` | the documented manifest keys (name, version, contextFileName, mcpServers, excludeTools, settings) |
| `GEMINI.md`, `skills/`, `commands/`, `project/` | invented content in the layouts the CLI reads |

Chats in `tmp/demo/chats`:

- `session-2026-11-01T10-00-11111111.jsonl`: a /help line, then the first real prompt (the title), a model reply with a tool call and two `$set` updates.
- `session-2026-11-02T09-30-22222222.jsonl`: a `$set` summary (the title) and a `$rewindTo`.
- `session-2026-11-03T08-00-33333333.jsonl`: metadata only, so not resumable (the CLI skips it).
- `session-2026-10-20T12-00-44444444.json`: a legacy single-object chat.
- `11111111-.../55555555-....jsonl`: a subagent chat, never listed.
