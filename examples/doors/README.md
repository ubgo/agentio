# Doors example

Shows the three doors in front of anything that is not a plain disk read: the Runner (`claude.Agent.Runner()`, `codex.Agent.Runner()`, the root `Core.Runner()` with `Run` and `Version`), the Prober (`claude.Agent.Prober().TokenUsage`) and the Writer (`claude.Agent.Writer().SetUserSetting`, which returns the `agentio.Backup` it made), plus the `HomeLinker` write door (`agentio.HomeLinkerOf`).

Binary runs are answered by a recording runner with canned `--version` output, the probe hits an in-process fake API, and every write lands in a temp copy of the Claude fixture home. The last section lists every command that reached `Host.Exec`.

## Run

```sh
cd examples && go run ./doors
```

Test:

```sh
cd examples && go test ./doors/...
```

## Output

```text
== runner ==
claude 2.1.325, codex 0.162.1
aider 0.86.1; `aider --help` exited 1
== prober ==
Current session 64% resets 16:00 UTC
== writer ==
backed up ~/.claude/settings.json
       to ~/.claude/settings.json.agentio-bak-20261113T120000Z
theme now "dark"
== home linker ==
~/.claude-new/CLAUDE.md -> ~/.claude/CLAUDE.md (existed false)
~/.claude-new/skills -> ~/.claude/skills (existed false)
~/.claude-new/settings.json -> ~/.claude/settings.json (existed false)
replan CLAUDE.md exists=true
replan skills exists=true
replan settings.json exists=true
== host.exec calls ==
claude --version
codex --version
aider --version
aider --help
```

## Guides

- [Doors: Runner, Prober and Writer](../../docs/capabilities/doors.md)
- [Feature index](../../docs/reference/features.md)
