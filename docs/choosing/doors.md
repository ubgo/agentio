# Which door: Runner, Prober or Writer

Plain reads (files on disk, the process table) need nothing special. Three kinds of call can cost more than a read, so each sits behind a door you have to ask for: the **Runner** runs the vendor binary, the **Prober** spends a little of the account's quota, and the **Writer** changes files the vendor CLI owns. This page helps you decide whether you need a door at all, and which one.

## Decision table

| You want to | Door | Cost | Claude Code | Codex | Config-defined agent | Guide | Example |
|---|---|---|---|---|---|---|---|
| Read anything the vendor keeps on disk | none | Disk · Read | ✅ | ✅ | homes only | [Shared capabilities](../capabilities/accessors.md#step-1--loop-over-agents-with-an-accessor) | [accessors](../../examples/accessors/) |
| Know the installed version | none: `Install()` reads it from disk | Disk · Read | ✅ (not for a Homebrew cask) | ✅ | ❌ | [Install and identity](../capabilities/install-identity.md#step-1--read-the-install) | [accessors](../../examples/accessors/) |
| Ask the binary its version | Runner: `BinaryVersion` / `BinaryVersionOf` | Process · Read, may touch the login | ✅ | ✅ | ✅ when the Spec has `VersionArgs` | [Install and identity](../capabilities/install-identity.md#step-2--run-the-binary-for-its-version) | [doors](../../examples/doors/) |
| Get an answer only the CLI gives (auth status, plugin list, feature flags) | Runner: vendor methods | Process · Read, may touch the login | ✅ `AuthStatus`, `Plugins`, `Marketplaces`, `BackgroundAgents`, `AutoModeConfig` | ✅ `LoginStatus`, `Features`, `BundledModels` | `Core.Runner().Run` | [Doors](../capabilities/doors.md#step-1--run-the-vendor-binary-through-a-runner) | [doors](../../examples/doors/), [claudeaccount](../../examples/claudeaccount/) |
| Read plan usage of a long-lived token (no stored login) | Prober: `TokenUsage` / `TokenProberOf` | Quota | ✅ | ❌ no token login | ❌ | [Doors](../capabilities/doors.md#step-2--spend-quota-through-a-prober) | [doors](../../examples/doors/), [claudeusage](../../examples/claudeusage/) |
| Change a settings key, add or remove a hook | Writer | Disk · Write, backs up first | ✅ | ❌ no methods | ❌ | [Claude writer](../claude/writer.md#step-1--patch-a-settings-file) | [claudehooks](../../examples/claudehooks/) |
| Move a folder's history or conversations between homes | Writer, or `HistoryMoverOf` / `ConversationMoverOf` | Disk · Write | ✅ | ❌ by design | ❌ | [Claude writer](../claude/writer.md#step-4--move-history) | [backupmove](../../examples/backupmove/) |
| Link shared files (settings, skills) into a new home | `HomeLinkerOf` | Disk · Write | ✅ | reported, but nothing to link | ✅ when the Spec has `SharedHomeFiles` | [Doors](../capabilities/doors.md#step-4--link-shared-home-files) | [doors](../../examples/doors/) |
| Add an MCP server, install a plugin, log in | none: a vendor command built as data | Pure to build; you run it | ✅ | ✅ | `LoginCommand` | [Writes: file or vendor command](writes.md) | [claudeextensions](../../examples/claudeextensions/) |

## If this, then that

- If a disk read answers the question, use it and hold no door. `Install()` gives the version for native and npm installs without running anything; `IdentityOf` gives the login without a network call or a binary run.
- If you must run the binary, do it once, on a user action, through the Runner. Vendor CLIs can refresh and rewrite the login when they start, which is why every Runner call is marked "may touch the login".
- If you have only a token (CI, a setup-token in a secret store) and need its plan usage, the Prober is the only way. If the home has a stored login, live usage (`UsageOf`) is a plain network read and spends no quota; see [which usage to read](live-or-cached-usage.md).
- If you change a Claude Code settings file, use the Writer: it backs the file up, writes atomically and keeps keys it does not touch byte for byte. If the change is to `~/.claude.json`, a project `.mcp.json` or anything of Codex, build the vendor command instead.
- If your program is a read-only dashboard, never call `.Runner()`, `.Prober()` or `.Writer()`. A reviewer can check that with one search, which is the point of the doors.
- If you write tests, give the Host a fake `Exec` (`agenttest.FakeExec`) and an `httptest` server: a Host with no `Exec` makes every Runner call fail instead of running a real binary.

## Pitfalls

- **Doors are not permissions.** Holding a `*claude.Writer` is a code-review signal, not a sandbox: it can write wherever the process can.
- **The cost of a neutral capability is its worst case.** `CapIdentity` reports Process cost because Claude may read the macOS keychain; Codex reads a file. Check `Capability.Cost()` once, not per call.
- **A Runner call fails without `Host.Exec`.** Vendor Runner methods return an `ErrUnsupported` error; that is the safe default for tests and read-only tools.
- **The Prober spends quota every call.** One minimal request per token, on demand; never in a loop or on a timer. An API key is never probed (`ErrBilledPerToken`).
- **`MoveHistory` refuses while the agent runs in the folder.** Like `PlanMove`, it checks live sessions and returns a `*claude.LiveSessionError` (matching `agentio.ErrMoveBlocked`) with the same blocker text; exit the session and retry.
- **Codex conversations cannot move between homes.** Codex indexes rollouts by absolute path in its own SQLite database, so `HistoryMoverOf` and `ConversationMoverOf` are false for Codex and `agentio.Unsupported` says why.
- **Backups are never deleted.** Every Writer call leaves `<file>.agentio-bak-<UTC stamp>` beside the file and returns it in the `agentio.Backup`; keep or show that value.

## Related

- [Doors: Runner, Prober and Writer](../capabilities/doors.md), [Claude writer](../claude/writer.md), [Claude Runner](../claude/runner.md), [Codex install](../codex/install.md#step-3--run-the-binary-through-the-runner)
- [Backups, moves and home links](../core/backup-move-link.md), [Logins and tokens](../capabilities/logins.md)
- Other choices: [Live or cached usage](live-or-cached-usage.md), [Writes: file or vendor command](writes.md)
- Examples: [doors](../../examples/doors/), [claudehooks](../../examples/claudehooks/), [backupmove](../../examples/backupmove/), [claudeusage](../../examples/claudeusage/)
- [Feature index](../reference/features.md#doors-runner-prober-writer), [All choosing guides](README.md)
