# Hooks: install and decode

A hook is a shell command an agent runs at a moment of its loop: when the user submits a prompt, when the agent finishes its answer, when a session starts or ends. Claude Code, Codex and Gemini CLI all read hooks from a JSON file of the same shape (event, then matcher groups, then command handlers), under different event names. `agentio.HookInstaller` (reached with `agentio.HooksOf`) installs one command at neutral moments into any of them, removes it again, and decodes the payload the agent sends the command on stdin. Use it for a capture tool, a notifier or a policy logger that should work with every agent and every home without knowing each vendor's file.

Installing merges. A settings file is shared by every tool that adds a hook or a status line, so the installer appends only its own entries, keeps every other byte of the file as it was, backs the file up first, and does nothing on a second run.

## Before you start

- Modules: `github.com/ubgo/agentio` plus the agents you install into (`claude`, `codex`, `gemini`).
- The home must be one of the agent's homes (`a.DefaultHome()`, `a.Homes()`); a home of another agent is `agentio.ErrForeignHome`.
- Writing is a Disk write to a file the vendor owns. Run the example only on its temp homes; test your own code against fixture homes (see [Testing your own code](../testing/your-code.md)).

The snippets come from [`examples/hooks`](../../examples/hooks/). It seeds a Claude Code, a Codex and a Gemini CLI home with another tool's hooks, installs a capture command at prompt submit and turn end, decodes one payload per agent, and removes the command again.

## Kinds and events per agent

`agentio.HookKind` names the moment; each agent maps it to its own event. `HookKinds()` lists what an agent supports, and `PlanHooks` fills `HookEntry.Event` from the kind, so a plan always shows the vendor's spelling.

| Kind | Claude Code | Codex | Gemini CLI | pi |
|---|---|---|---|---|
| `prompt-submit` | `UserPromptSubmit` | `UserPromptSubmit` | `BeforeAgent` | none |
| `turn-end` | `Stop` | `Stop` | `AfterAgent` | none |
| `session-start` | `SessionStart` | `SessionStart` | `SessionStart` | none |
| `session-end` | `SessionEnd` | `SessionEnd` | `SessionEnd` | none |
| File | `<home>/settings.json` | `<home>/hooks.json` | `<home>/settings.json` (`hooks` key) | none |

pi has no `HookInstaller`: its extensions are TypeScript modules loaded into the process, so there is no settings key to add a shell command to. `agentio.Unsupported(a, agentio.CapHooks)` returns that reason, and an agent defined only by a `Spec` returns its own (a spec names no hook file). Each vendor package also exports the mapping: `claude.HookEventFor` / `claude.HookKindOf`, and the same pair in `codex` and `gemini`.

## Step 1 — Plan, apply, and apply again

`PlanHooks(home, want)` reads the hook file and returns a `HookPlan`: `File` (the file it would change), `Add` (the entries it would append), `Present` (requested entries the file already has: a command hook with the same event and the same command, in any group) and `Manual` (steps the library does not take for the hooks to run, one line each). `ApplyHooks(plan)` appends `Add` after backing the file up and returns the `agentio.Backup`. A plan whose `IsNoop()` is true writes nothing and returns an empty backup.

<!-- snippet: examples/hooks/main.go#install -->
```go
// install plans the capture hooks, applies the plan (the file is backed up
// first and only the new entries are appended), then plans again: the
// second plan finds every entry Present and applying it writes nothing.
func install(p printer, inst agentio.HookInstaller, h agentio.Home) error {
	plan, err := inst.PlanHooks(h, want())
	if err != nil {
		return err
	}
	p("file " + plan.File)
	for _, e := range plan.Add {
		p(fmt.Sprintf("  add %s -> %s", e.Kind, e.Event))
	}
	for _, m := range plan.Manual {
		p("  manual: " + m)
	}
	b, err := inst.ApplyHooks(plan)
	if err != nil {
		return err
	}
	for _, e := range b.Entries {
		p("  backup " + e.Copy)
	}
	again, err := inst.PlanHooks(h, want())
	if err != nil {
		return err
	}
	b, err = inst.ApplyHooks(again)
	if err != nil {
		return err
	}
	p(fmt.Sprintf("  second install: present=%d noop=%t backup empty=%t", len(again.Present), again.IsNoop(), b.IsEmpty()))
	return nil
}
```

`ApplyHooks` re-reads the file before it writes, so an entry that appeared between `PlanHooks` and `ApplyHooks` (another process installed it) is not added twice. It only writes the file its own `PlanHooks` names: a plan of another agent is `agentio.ErrForeignHome`, a plan whose `File` was changed is `agentio.ErrInvalidHook`. An unknown kind is `agentio.ErrUnsupported`, an empty command `agentio.ErrInvalidHook`.

### Manual steps

| Agent | When | Line |
|---|---|---|
| Claude Code | the user settings set `disableAllHooks: true` | no hook runs until it is removed |
| Codex | the plan adds anything | Codex asks you to review new or changed hooks before it runs them (Review hooks in the TUI); the trust lands in `config.toml`, which agentio keeps read-only |
| Codex | `config.toml` sets `[features] hooks = false` | hooks are on by default in 0.162.1; set it back to true |
| Gemini CLI | `hooksConfig.enabled` is false | no hook runs |
| Gemini CLI | `hooksConfig.disabled` lists the command | Gemini CLI skips that hook |
| Gemini CLI | folder trust is on (the default) | settings hooks run only in folders Gemini CLI trusts |

Running sessions read hooks when they start: Claude Code takes a snapshot of its hooks at startup, so a session already open sees the change after a restart (or a review in its `/hooks` menu).

## Step 2 — What the merge keeps

The edit is a splice into the file's bytes, never a rewrite through a map:

- Each requested entry becomes one new matcher group with one command handler (`{"hooks": [{"type": "command", "command": "…"}]}`, no matcher, which every vendor reads as "match all"), appended after the last group of its event. The event key, the `hooks` object or the whole file are created when missing.
- Every other hook (other events, other groups on the same event, other handlers, unknown handler keys), every other top-level key, their order, the indentation and Gemini's comments keep their exact bytes. The new group follows the file's layout (indent step, one line or many).
- `&`, `<` and `>` in a command are written as typed, not as `&`.
- A file that is valid JSON but not this shape (the top level is not an object, `hooks` or an event is not what the vendor reads) is refused with an error; it is never rewritten.

The `claude` Writer uses the same splice for `AddHook`, `RemoveHooks` and `EnsureHook` (see [Claude writer](../claude/writer.md)). `agenttest.HookMerge` runs every guarantee of this page against a `HookInstaller` on a seeded file: use it when you implement one (see [Fakes and fixture hosts](../testing/agenttest.md)).

## Step 3 — Decode the payload

The installed command receives one JSON object on stdin. `DecodeHook(stdin)` turns it into an `agentio.HookCall`: `Agent`, `Kind` (the neutral kind of the event, `HookUnknown` for an event with none), `Event` (the vendor name), `ConversationID`, `Dir`, `TranscriptPath`, `Prompt` (prompt-submit), `LastAssistantMessage` (turn-end, when the agent sends it inline), `PermissionMode`, `Model`, and `Raw`, the whole payload. A field the agent does not send stays empty.

<!-- snippet: examples/hooks/main.go#decode -->
```go
// decode is what the installed command does with its stdin: one payload,
// decoded into the neutral call whatever agent sent it.
func decode(p printer, inst agentio.HookInstaller, payload string) error {
	c, err := inst.DecodeHook(strings.NewReader(payload))
	if err != nil {
		return err
	}
	p(fmt.Sprintf("  decoded %s (%s) conv=%.8s dir=%s prompt=%q answer=%q", c.Kind, c.Event, c.ConversationID, c.Dir, c.Prompt, c.LastAssistantMessage))
	return nil
}
```

One command can serve every agent: install it with an argument naming the agent (`capture hook --agent codex`), then call `DecodeHook` of that agent. For vendor detail, decode with the vendor function instead: `claude.DecodeHookInput`, `codex.DecodeHookInput`, `gemini.DecodeHookInput`, each with typed per-event fields, `Unknown` and `Raw`.

### Unverified payloads

Only the Claude Code payload is modelled on recorded runs. No Codex or Gemini CLI hook run has been recorded yet, so their payload types are unverified:

- **Codex** (`codex.HookInput`): the fields of the command-hook input schemas embedded in the Codex 0.162.1 binary: `session_id`, `transcript_path` (may be null), `cwd`, `hook_event_name`, `model`, `permission_mode`, `turn_id`, `prompt`, `last_assistant_message` (may be null), `stop_hook_active`, `source`, `reason`, `agent_id`, `agent_type`.
- **Gemini CLI** (`gemini.HookInput`): the base fields of the 0.68.0 hooks reference (`session_id`, `transcript_path`, `cwd`, `hook_event_name`, `timestamp`) plus `prompt` (BeforeAgent, AfterAgent), `prompt_response` (AfterAgent, mapped to `LastAssistantMessage`), `stop_hook_active`, `source` and `reason`. It sends no model or permission mode.

Every other key is kept in `Unknown` and the payload in `Raw`, so nothing is lost if a recorded run shows more.

## Step 4 — Remove

`RemoveHooks(home, match)` removes every command hook of the hook file whose command `match` reports true, and nothing else. A group left without handlers is removed with it, then an event left without groups, then a `hooks` object left empty; a group that still has another tool's handler stays. Removing nothing writes nothing and returns an empty backup. Install followed by remove gives the original file back byte for byte.

<!-- snippet: examples/hooks/main.go#uninstall -->
```go
// uninstall removes the capture command and nothing else: the other
// tool's hook, every other key and the file's layout come back as they
// were.
func uninstall(p printer, inst agentio.HookInstaller, h agentio.Home, original string) error {
	n, b, err := inst.RemoveHooks(h, func(command string) bool { return command == captureCommand })
	if err != nil {
		return err
	}
	plan, err := inst.PlanHooks(h, want())
	if err != nil {
		return err
	}
	now, err := os.ReadFile(plan.File)
	if err != nil {
		return err
	}
	p(fmt.Sprintf("  removed=%d backup=%t file restored=%t", n, !b.IsEmpty(), string(now) == original))
	return nil
}
```

## Try it

```sh
cd examples && go run ./hooks
```

```text
== Claude Code
file ~/.claude/settings.json
  add prompt-submit -> UserPromptSubmit
  add turn-end -> Stop
  backup ~/.claude/settings.json.agentio-bak-20261119T120000Z
  second install: present=2 noop=true backup empty=true
  decoded prompt-submit (UserPromptSubmit) conv=0b5e3a8e dir=/work/app prompt="add a test" answer=""
  removed=2 backup=true file restored=true
== Codex
file ~/.codex/hooks.json
  add prompt-submit -> UserPromptSubmit
  add turn-end -> Stop
  manual: Codex asks you to review new or changed hooks before it runs them: start codex on this home and trust them (Review hooks)
  backup ~/.codex/hooks.json.agentio-bak-20261119T120000Z
  second install: present=2 noop=true backup empty=true
  decoded turn-end (Stop) conv=019a0000 dir=/work/app prompt="" answer="Added the test."
  removed=2 backup=true file restored=true
== Gemini CLI
file ~/.gemini/settings.json
  add prompt-submit -> BeforeAgent
  add turn-end -> AfterAgent
  manual: Gemini CLI runs settings hooks only in folders it trusts while folder trust is on (security.folderTrust.enabled): trust the folders you work in
  backup ~/.gemini/settings.json.agentio-bak-20261119T120000Z
  second install: present=2 noop=true backup empty=true
  decoded turn-end (AfterAgent) conv=7c0e0000 dir=/work/app prompt="add a test" answer="Added the test."
  removed=2 backup=true file restored=true
== pi
no hook installer: pi has no shell-command hooks: its extensions are TypeScript modules loaded into the process, so there is no settings key to add a command to
```

## Pitfalls

- **Present means same event and same command.** Another tool's handler with exactly your command (even with another timeout) counts as installed. Make your command distinctive, for example with an `--agent` argument.
- **Only the user file of a home.** The installer edits the home's own file (Claude and Gemini user `settings.json`, Codex `hooks.json`). Project settings and Codex's `[hooks]` table in `config.toml` are read by the vendor readers but never written here; for Claude project files use `Writer.EnsureHook` with `ScopeProject` or `ScopeLocal`.
- **Every home separately.** A user with several homes (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `GEMINI_CLI_HOME`) needs one install per home; loop over `a.Homes()`.
- **Manual lines are not errors.** A plan with `Manual` still applies; show the lines to the user, because the hook will not run until they act.
- **Codex and Gemini payloads are unverified.** Read optional fields defensively and keep `Raw` if you store events.

## Related

- [Claude hooks](../claude/hooks.md), [Claude writer](../claude/writer.md), [Codex hooks and rules](../codex/hooks-rules.md), [Gemini CLI](../gemini/README.md), [pi](../pi/README.md)
- [Doors](doors.md), [Writes: file or vendor command](../choosing/writes.md), [Backup, move and link](../core/backup-move-link.md)
- [Shared capabilities and accessors](accessors.md)
