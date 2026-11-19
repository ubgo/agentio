# Writes: file or vendor command

agentio is read-only by default. When your program has to change something a vendor CLI owns, there are two ways: agentio writes the file itself through a **Writer** (with a backup first), or agentio builds the **vendor's own command** (`claude mcp add-json`, `codex features enable`) as an `agentio.Command` and you run it. Which one applies is decided per file, by who must stay in charge of it.

## Decision table

| Change | How | Claude Code | Codex | Guide | Example |
|---|---|---|---|---|---|
| A top-level key of `settings.json` (user, project or local) | Writer: [`SetUserSetting`](ds:block?id=claude-writer-setusersetting-hfm5v3t7), [`SetProjectSetting`](ds:block?id=claude-writer-setprojectsetting-9xh95gnh) | ✅ | ❌ | [Claude writer](../claude/writer.md#step-1--patch-a-settings-file) | [claudehooks](../../examples/claudehooks/), [doors](../../examples/doors/) |
| Add or remove a hook | Writer: `AddHook`, `EnsureHook`, [`RemoveHooks`](ds:block?id=claude-writer-removehooks-x8p95azs); for every agent the neutral `HooksOf` (Gemini CLI too) | ✅ `settings.json` | ✅ `hooks.json` only (`EnsureHook`, `RemoveHooks`) | [Hooks: install and decode](../capabilities/hooks.md), [Claude writer](../claude/writer.md#step-2--add-a-hook) | [hooks](../../examples/hooks/), [claudehooks](../../examples/claudehooks/) |
| Re-key a folder's history after the folder moved | Writer: [`MoveHistory`](ds:block?id=claude-writer-movehistory-3n8zhjz3), or `HistoryMoverOf` | ✅ | ❌ conversations stay in their home | [Claude writer](../claude/writer.md#step-4--move-history) | [claudehooks](../../examples/claudehooks/) |
| Move conversations to another home | `PlanMove`, then Writer [`ApplyMove`](ds:block?id=claude-writer-applymove-tpwp98h7), or `ConversationMoverOf` | ✅ | ❌ | [Backups, moves and home links](../core/backup-move-link.md#step-2--plan-a-move) | [backupmove](../../examples/backupmove/) |
| Link shared files into a new home | `HomeLinkerOf`, or Writer `ApplyHomeLinks` | ✅ | nothing to link | [Doors](../capabilities/doors.md#step-4--link-shared-home-files) | [doors](../../examples/doors/) |
| Add or remove an MCP server | vendor command: `MCPAddCommand`, `MCPRemoveCommand` | ✅ [`claude mcp add-json`](ds:block?id=claude-agent-mcpaddcommand-p8k42k84) | ✅ [`codex mcp add`](ds:block?id=codex-agent-mcpaddcommand-yajah85d) | [Claude MCP](../claude/mcp.md#step-3--build-add-and-remove-commands), [Codex MCP](../codex/mcp.md#step-2--build-add-and-remove-commands) | [claudeextensions](../../examples/claudeextensions/), [codexconfig](../../examples/codexconfig/) |
| Install, enable or disable a plugin; add a marketplace | vendor command: `PluginInstallCommand`, `PluginEnableCommand`, `MarketplaceAddCommand`, … | ✅ | ❌ (list only: [`PluginListCommand`](ds:block?id=codex-agent-pluginlistcommand-59jvvkz7)) | [Claude plugins](../claude/plugins.md#step-4--build-plugin-commands) | [claudeextensions](../../examples/claudeextensions/) |
| Turn a feature flag on or off | vendor command: `FeatureCommand` | ❌ | ✅ [`codex features enable`](ds:block?id=codex-agent-featurecommand-w47qatyu) | [Codex launch](../codex/launch.md#step-3--build-archive-delete-and-queue-commands) | [codexlaunch](../../examples/codexlaunch/) |
| Archive, unarchive or delete a conversation | vendor command: [`Archive`](ds:block?id=codex-agent-archive-yztnawg5), `Unarchive`, `Delete` | ❌ | ✅ | [Codex launch](../codex/launch.md#step-3--build-archive-delete-and-queue-commands) | [codexlaunch](../../examples/codexlaunch/) |
| Log in or out, create a setup-token | vendor command: `LoginCommand`, `LogoutCommand`, [`SetupTokenCommand`](ds:block?id=claude-agent-setuptokencommand-xces3g4z) | ✅ | ✅ login and logout | [Logins and tokens](../capabilities/logins.md#step-1--build-a-login-command) | [usage](../../examples/usage/), [claudeaccount](../../examples/claudeaccount/) |
| Update or reinstall the CLI | vendor command: `UpdateCommand`, `InstallCommand` | ✅ | ✅ [`UpdateCommand()`](ds:block?id=codex-agent-updatecommand-mmv9gxga) (same method shape) | [Claude install](../claude/install.md#step-3--build-install-and-update-commands) | [claudesettings](../../examples/claudesettings/) |

## If this, then that

- If the file is a Claude Code `settings.json` (any scope), use the Writer. It backs the file up, [writes atomically](ds:block?id=claude-writer-writerawsettings-6sjvkkgu), [refuses a file that does not parse instead of overwriting it](ds:block?id=claude-readrawsettings-peagek4s), and returns an `agentio.Backup`.
- If the file is `~/.claude.json`, a project `.mcp.json`, or anything in a Codex home except `hooks.json`, build the vendor command. Claude Code rewrites `.claude.json` constantly while it runs, and Codex keeps its own SQLite index consistent with its files, so [only the vendor may change them](ds:block?id=claude-writer-t5gq26t8).
- If you build a vendor command, decide how to run it: `exec.Cmd` with the `Command`'s `Env` and `Dir`, a terminal, or print it for the user. agentio never runs one for you.
- If the change carries a secret (an MCP server's env value), it cannot go on argv. [`claude.Agent.MCPAddCommand`](ds:block?id=claude-agent-mcpaddcommand-p8k42k84) refuses a server with env values; ask the user to add that server in the CLI.
- If several homes need the same settings, link them (`HomeLinkerOf`) instead of writing each copy; a later edit then reaches every home.
- If your installer may run twice, use `EnsureHook` or the neutral `PlanHooks` / `ApplyHooks`, which skip what is already there: [`AddHook` appends a new group on every call](ds:block?id=claude-writer-addhook-y3s8tkg6).

## A file write and a vendor command

The Writer path, for a key Claude Code reads from `settings.json`:

```go
// settingModel is the top-level settings.json key that pins the model.
const settingModel = "model"

// pinModel sets the user-scope model of one Claude home through the Writer.
// The file is backed up first and the copy's path comes back in the Backup;
// every other key keeps its value (the file is rewritten indented).
func pinModel(w io.Writer, c *claude.Agent, h claude.Home, model string) error {
	val, err := json.Marshal(model) // values are raw JSON: a string keeps its quotes
	if err != nil {
		return err
	}
	b, err := c.Writer().SetUserSetting(h, claude.SettingsPatch{
		Set: map[string]agentio.Raw{settingModel: val},
	})
	if err != nil {
		return err
	}
	for _, e := range b.Entries {
		fmt.Fprintf(w, "backed up %s to %s\n", e.Original, e.Copy)
	}
	return nil
}
```

On a home whose `settings.json` was `{"theme":"dark"}` this prints [`backed up ~/.claude/settings.json to ~/.claude/settings.json.agentio-bak-20261114T063137Z`](ds:block?id=backuppath-tfjr5hjr) and leaves both keys in the file.

The vendor-command path, for a Codex feature flag:

```go
// enableFeature turns a Codex feature flag on with Codex's own command, so
// Codex edits its config.toml itself. Building the command is Pure; running
// it is a Process cost, done here on purpose with os/exec.
func enableFeature(ctx context.Context, a *codex.Agent, h codex.Home, name string) error {
	cmd := a.FeatureCommand(h, name, true)
	bin, err := a.LookPath() // PATH from Host.Env, never a shell
	if err != nil {
		return err
	}
	run := exec.CommandContext(ctx, bin, cmd.Args...)
	run.Env = cmd.Env // carries the home variable rule for h
	run.Dir = cmd.Dir
	if out, err := run.CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w: %s", cmd.Redacted(), err, out)
	}
	return nil
}
```

The command it runs is [`codex features enable web_search`](ds:block?id=codex-agent-featurecommand-w47qatyu); on a machine without `codex` on the Host's `PATH` it returns [`agentio: binary not found on PATH: codex`](ds:block?id=lookpath-f56nfyzk) before running anything.

## Pitfalls

- **The settings patch rewrites the file's layout.** `SetUserSetting` and `SetProjectSetting` keep the values of the keys they do not touch, but not [byte for byte](ds:block?id=claude-writer-writerawsettings-6sjvkkgu): the file is written back indented with its keys sorted, nested values are re-indented, and `<`, `>`, `&` inside strings come back as `\u003c`-style escapes. Do not rely on hand formatting surviving a settings patch. The hook methods (`AddHook`, `EnsureHook`, `RemoveHooks`, `HooksOf`) splice instead and keep every other byte.
- **Backups pile up.** [Each Writer call](ds:block?id=backupfile-bxsxzm67) that changes an existing file leaves `<file>.agentio-bak-<UTC stamp>` (`ApplyMove`: one `<from home>.agentio-bak-<UTC stamp>/` folder; [a `-N` suffix if that name is taken](ds:block?id=backuppath-tfjr5hjr)); nothing deletes them.
- **[`SettingsPatch.Set`](ds:block?id=claude-writer-applypatch-utnp7z3r) takes raw JSON.** Marshal Go values with `encoding/json`; the string `opus` must be `"opus"` in the patch.
- **`MoveHistory` refuses while the agent runs in the folder.** A live session in `from` (or of one of its conversations) is [`*claude.LiveSessionError`](ds:block?id=claude-writer-movehistory-3n8zhjz3), matching [`agentio.ErrMoveBlocked`](ds:block?id=claude-livesessionerror-unwrap-parprkp9), with the blocker text `PlanMove` uses.
- **`MoveHistory` moves only the history folder.** The folder's state entry in `.claude.json` (trust, last session) [stays under the old path](ds:block?id=claude-writer-movehistory-3n8zhjz3).
- **[`codex.Agent.Writer()`](ds:block?id=codex-writer-tax23vch) writes only `hooks.json`.** Codex reads that file but never writes it; every other Codex change is a vendor command.
- **Running a vendor command is a Process cost and may touch the login**, like any Runner call. Run it on a user action, never in a loop.

## Related

- [Claude writer](../claude/writer.md), [Doors](../capabilities/doors.md#step-3--write-through-a-writer), [Backups, moves and home links](../core/backup-move-link.md), [Targets and commands](../core/launch.md)
- Other choices: [Which door](doors.md), [Which module to import](modules.md)
- Examples: [claudehooks](../../examples/claudehooks/), [backupmove](../../examples/backupmove/), [claudeextensions](../../examples/claudeextensions/), [codexconfig](../../examples/codexconfig/), [codexlaunch](../../examples/codexlaunch/)
- [Feature index](../reference/features.md#l3-claude-writer), [All choosing guides](README.md)
