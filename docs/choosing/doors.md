# Which door: Runner, Prober or Writer

Plain reads (files on disk, the process table) need nothing special. Three kinds of call can cost more than a read, so each sits behind a door you have to ask for: the **Runner** runs the vendor binary, the **Prober** spends a little of the account's quota, and the **Writer** changes files the vendor CLI owns. This page helps you decide whether you need a door at all, and which one.

## Decision table

| You want to | Door | Cost | Claude Code | Codex | Config-defined agent | Guide | Example |
|---|---|---|---|---|---|---|---|
| Read anything the vendor keeps on disk | none | Disk · Read | ✅ | ✅ | homes only | [Shared capabilities](../capabilities/accessors.md#step-1--loop-over-agents-with-an-accessor) | [accessors](../../examples/accessors/) |
| Know the installed version | none: [`Install()`](ds:block?id=codex-agent-install-7wqjtcqk) reads it from disk | Disk · Read | ✅ [(not for a Homebrew cask)](ds:block?id=claude-methodandversion-8h4tytjj) | ✅ (npm, bun and pnpm installs only) | ❌ | [Install and identity](../capabilities/install-identity.md#step-1--read-the-install) | [accessors](../../examples/accessors/) |
| Ask the binary its version | Runner: `BinaryVersion` / `BinaryVersionOf` | Process · Read, may touch the login | ✅ | ✅ | ✅ when the Spec has [`VersionArgs`](ds:block?id=runner-version-87cgudkv) | [Install and identity](../capabilities/install-identity.md#step-2--run-the-binary-for-its-version) | [doors](../../examples/doors/) |
| Get an answer only the CLI gives (auth status, plugin list, feature flags) | Runner: vendor methods | Process · Read, may touch the login | ✅ `AuthStatus`, `Plugins`, `Marketplaces`, `BackgroundAgents`, `AutoModeConfig` | ✅ `LoginStatus`, `Features`, `BundledModels` | [`Core.Runner().Run`](ds:block?id=runner-run-5rn5nerj) | [Doors](../capabilities/doors.md#step-1--run-the-vendor-binary-through-a-runner) | [doors](../../examples/doors/), [claudeaccount](../../examples/claudeaccount/) |
| Read plan usage of a long-lived token (no stored login) | Prober: [`TokenUsage`](ds:block?id=claude-quota-door-tokenusage-j2fw66uu) / `TokenProberOf` | Quota | ✅ | ❌ no token login | ❌ | [Doors](../capabilities/doors.md#step-2--spend-quota-through-a-prober) | [doors](../../examples/doors/), [claudeusage](../../examples/claudeusage/) |
| Change a settings key, add or remove a hook | Writer | Disk · Write, [backs up first](ds:block?id=claude-writer-writerawsettings-6sjvkkgu) | ✅ | ❌ no methods | ❌ | [Claude writer](../claude/writer.md#step-1--patch-a-settings-file) | [claudehooks](../../examples/claudehooks/) |
| Move a folder's history or conversations between homes | Writer, or `HistoryMoverOf` / `ConversationMoverOf` | Disk · Write | ✅ | [❌ by design](ds:block?id=codex-agent-moveunsupportedreason-yn4v8j7h) | ❌ | [Claude writer](../claude/writer.md#step-4--move-history) | [backupmove](../../examples/backupmove/) |
| Link shared files (settings, skills) into a new home | `HomeLinkerOf` | Disk · Write | ✅ | [reported, but nothing to link](ds:block?id=codex-spec-ut8me6cu) | ✅ when the Spec has `SharedHomeFiles` | [Doors](../capabilities/doors.md#step-4--link-shared-home-files) | [doors](../../examples/doors/) |
| Add an MCP server, install a plugin, log in | none: a vendor command built as data | Pure to build; you run it | ✅ | ✅ | [`LoginCommand`](ds:block?id=core-logincommand-3b2a9mth) | [Writes: file or vendor command](writes.md) | [claudeextensions](../../examples/claudeextensions/) |

## If this, then that

- If a disk read answers the question, use it and hold no door. [`Install()`](ds:block?id=claude-agent-install-472s4jn6) gives the version for native and npm installs without running anything; [`IdentityOf`](ds:block?id=claude-agent-identity-5vqz5ezp) gives the login without a network call or running the vendor binary (Claude may read the macOS keychain).
- If you must run the binary, do it once, on a user action, through the Runner. Vendor CLIs can refresh and rewrite the login when they start, which is why [every Runner call is marked "may touch the login"](ds:block?id=claude-runner-dbm6t8tp).
- If you have only a token (CI, a setup-token in a secret store) and need its plan usage, the Prober is the only way. If the home has a stored login, [live usage (`UsageOf`) is a plain network read and spends no quota](ds:block?id=claude-agent-usage-zq8vtbvb); see [which usage to read](live-or-cached-usage.md).
- If you change a Claude Code settings file, use the Writer: it backs the file up, writes atomically and keeps the values of keys it does not touch (re-indented, not [byte for byte](ds:block?id=claude-writer-writerawsettings-6sjvkkgu)). If the change is to `~/.claude.json`, a project `.mcp.json` or anything of Codex, [build the vendor command instead](ds:block?id=claude-writer-t5gq26t8).
- If your program is a read-only dashboard, never call `.Runner()`, `.Prober()` or `.Writer()`. A reviewer can check that with one search, which is the point of the doors.
- If you write tests, give the Host a fake `Exec` ([`agenttest.FakeExec`](ds:block?id=agenttest-fakeexec-anmuphec)) and an `httptest` server: a Host with no `Exec` [makes every Runner call fail](ds:block?id=runner-run-5rn5nerj) instead of running a real binary.

## Pitfalls

- **Doors are not permissions.** Holding a [`*claude.Writer`](ds:block?id=claude-writer-t5gq26t8) is a code-review signal, not a sandbox: it can write wherever the process can.
- **The cost of a neutral capability is its worst case.** [`CapIdentity`](ds:block?id=capabilitytable-brzshzmm) reports Process cost because Claude may read the macOS keychain; [Codex reads a file](ds:block?id=codex-agent-identity-j4hp7h58) (or the OS keyring when its config stores the login there). Check [`Capability.Cost()`](ds:block?id=capability-cost-u64h92wf) once, not per call.
- **A Runner call fails without [`Host.Exec`](ds:block?id=claude-runner-run-af4j3992).** Claude Runner methods return an [`ErrUnsupported`](ds:block?id=codex-runner-run-wwh9d3qf) error; Codex and config-defined Runner calls return a plain "no process runner in host" error. Either way nothing runs, the safe default for tests and read-only tools.
- **The Prober spends quota every call.** One minimal request per token, on demand; never in a loop or on a timer. An API key is never probed ([`ErrBilledPerToken`](ds:block?id=claude-quota-door-tokenusage-j2fw66uu)).
- **`MoveHistory` refuses while the agent runs in the folder.** Like `PlanMove`, it checks live sessions and returns a [`*claude.LiveSessionError`](ds:block?id=claude-writer-movehistory-3n8zhjz3) (matching [`agentio.ErrMoveBlocked`](ds:block?id=claude-livesessionerror-unwrap-parprkp9)) with the same blocker text; exit the session and retry.
- **Codex conversations cannot move between homes.** Codex indexes rollouts by [absolute path in its own SQLite database](ds:block?id=codex-agent-moveunsupportedreason-yn4v8j7h), so `HistoryMoverOf` and `ConversationMoverOf` are false for Codex and [`agentio.Unsupported`](ds:block?id=codex-neutral-unsupportedreason-evgu3az7) says why.
- **Backups are never deleted.** Every Writer call that changes an existing file leaves [`<file>.agentio-bak-<UTC stamp>`](ds:block?id=backupfile-bxsxzm67) beside it (`ApplyMove`: one `<from home>.agentio-bak-<UTC stamp>/` folder) and returns it in the `agentio.Backup`; keep or show that value.

## Related

- [Doors: Runner, Prober and Writer](../capabilities/doors.md), [Claude writer](../claude/writer.md), [Claude Runner](../claude/runner.md), [Codex install](../codex/install.md#step-3--run-the-binary-through-the-runner)
- [Backups, moves and home links](../core/backup-move-link.md), [Logins and tokens](../capabilities/logins.md)
- Other choices: [Live or cached usage](live-or-cached-usage.md), [Writes: file or vendor command](writes.md)
- Examples: [doors](../../examples/doors/), [claudehooks](../../examples/claudehooks/), [backupmove](../../examples/backupmove/), [claudeusage](../../examples/claudeusage/)
- [Feature index](../reference/features.md#doors-runner-prober-writer), [All choosing guides](README.md)
