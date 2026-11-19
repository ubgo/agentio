# hooks

The neutral `agentio.HookInstaller` on three agents: a Claude Code, a Codex and a Gemini CLI home, each seeded with another tool's hook on the same events, get a capture command at prompt submit and turn end. The example plans the install (file, entries, manual steps), applies it with a backup, shows that a second install writes nothing, decodes one payload per agent into `agentio.HookCall`, and removes the command again, which gives the original file back byte for byte. pi has no hook installer and prints why.

Every home is a temp folder (paths print as `~`), through a `Host` built by hand: no real home, no vendor binary, no network. The Claude payload follows the recorded shape; the Codex and Gemini payloads are unverified (typed from the vendor's schema and docs, never recorded).

## Run it

```sh
cd examples && go run ./hooks
```

Test: `cd examples && go test ./hooks/...` runs the same code and checks the output lines.

## Output

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

Guide: [Hooks: install and decode](../../docs/capabilities/hooks.md).
