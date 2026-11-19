# opencode 1.18.30 fixture home

`userhome` is a user home whose `.local/share/opencode` is an opencode data dir holding the legacy `storage/` tree. Every value is invented: no real path, id, prompt or link. Tests copy it into a temp dir.

The tree is the shape a real machine running opencode 1.18.30 still holds from older releases (opencode 1.2.0 migrated it into `opencode.db` and left the files in place). Each file keeps the keys of a real file of that release; ids keep opencode's prefixes and length, and their order follows creation like opencode's ascending ids.

| Path | Basis |
|---|---|
| `storage/migration` | the real file: the number of the last storage migration (2) |
| `storage/project/global.json` | the real global project file (id, worktree, sandboxes, time) |
| `storage/session/global/ses_…AAAA.json` | a 0.14.7 session: shared (`share.url`), title, directory, created and updated times |
| `storage/message/ses_…AAAA/*.json` | 0.14.7 messages: user messages with role and time only; assistant messages with `system`, provider and model ids, mode, path, cost, tokens and `time.completed` |
| `storage/part/msg_…AAAA…/*.json` | the part types of that session: `step-start`, `reasoning` (with provider metadata), `tool` (completed state), `text` (with and without time and metadata, one `synthetic: true`), `step-finish` |
| `storage/session/global/ses_…BBBB.json` and its messages | a 1.1.14 session: user messages carry `agent` and `model`, assistant messages carry `parentID` and `agent` but never got `time.completed` (a local provider that never answered) |

The SQLite database is not part of this fixture: `opencode/store` tests build one in a temp dir from `opencode/store/testdata/schema-1.18.30.sql`, the schema of the real file with no rows.
