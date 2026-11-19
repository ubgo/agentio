# Codex hooks and rules

Read Codex's hook configuration (the `[hooks]` table of `config.toml`, its `hooks.state` trust records, and `<home>/hooks.json`), decode the JSON Codex hands a hook program or a `notify` program, parse execpolicy `*.rules` files into typed prefix rules without executing them, and install or remove command hooks in `hooks.json`.

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` and `codex.Home` from [The Codex agent](agent.md).
- Codex hook events use the same names as Claude Code hooks (`SessionStart`, `PreToolUse`, `Stop`, …); handler types are `command`, `mcp_tool`, `prompt` and `agent`.

The snippets come from [`examples/codexconfig`](../../examples/codexconfig/).

## Step 1 — Read hook config

`HookConfig(h)` returns `Hooks`: `Events` maps each `HookEvent` to its `[[hooks.<Event>]]` matcher groups (`HookMatcherGroup` with an optional `Matcher` and `HookHandler` list), and `State` is the `hooks.state` table. `HookState(h)` returns that table alone, keyed `"<hooks file>:<event_snake>:<group>:<handler>"`: Codex records the hash of a hook the user trusted there, and can disable one. `HooksFile(h)` reads `<home>/hooks.json` (`found=false` when absent); `ParseHooksFile` decodes bytes you hold. Handler timeouts are `time.Duration`; unmodelled handler keys are kept (`Unknown` for TOML, `UnknownJSON` for `hooks.json`).

<!-- snippet: examples/codexconfig/hooks.go#hooks -->
```go
// readHooks reads the [hooks] table of config.toml, its hooks.state
// trust records, and <home>/hooks.json.
func readHooks(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()
	hk, err := a.HookConfig(h)
	if err != nil {
		return err
	}
	events := make([]string, 0, len(hk.Events))
	for ev := range hk.Events {
		events = append(events, string(ev))
	}
	sort.Strings(events)
	for _, name := range events {
		for _, g := range hk.Events[codex.HookEvent(name)] {
			for _, hd := range g.Hooks {
				fmt.Fprintf(w, "config hook %s matcher=%q type=%s command=%q timeout=%v\n",
					name, g.Matcher.V, hd.Type, hd.Command.V, hd.Timeout.V)
			}
		}
	}

	state, err := a.HookState(h)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s := state[k]
		fmt.Fprintf(w, "state %s enabled-set=%v enabled=%v\n", k, s.Enabled.Set, s.Enabled.V)
	}

	hf, found, err := a.HooksFile(h)
	if err != nil || !found {
		return err
	}
	fmt.Fprintf(w, "hooks.json %q events=%d\n", hf.Description.V, len(hf.Events))

	// ParseHooksFile is the same decoder for bytes you already hold.
	parsed, warns, err := codex.ParseHooksFile([]byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"/bin/done"}]}]}}`), "inline.json")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "parsed Stop groups=%d warnings=%d known=%v\n",
		len(parsed.Events[codex.HookStop]), len(warns), codex.HookStop.IsKnown())
	return nil
}
```

## Step 2 — Decode hook input and notify payloads

A hook program reads one JSON object on stdin; `DecodeHookInput` returns its `Event`, the typed fields (`SessionID`, `TranscriptPath`, `Cwd`, `Model`, `PermissionMode`, `TurnID`, `Prompt`, `LastAssistantMessage`, `StopHookActive`, `Source`, `Reason`, `AgentID`, `AgentType`), every other key in `Unknown`, and the full `Raw` payload. These fields are unverified: they come from the command-hook input schemas embedded in the Codex 0.162.1 binary, not from a recorded run, so read them defensively and keep `Raw`. `HookInput.Call()` maps the input onto the neutral `agentio.HookCall`. The `notify` program configured in `config.toml` gets a JSON object as its last argument; `DecodeNotify` returns its `Type` and `Raw`.

<!-- snippet: examples/codexconfig/hooks.go#payloads -->
```go
// decodePayloads reads what Codex hands a hook program on stdin and a
// notify program as its last argument.
func decodePayloads(w io.Writer) error {
	in, err := codex.DecodeHookInput(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"shell"}`))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "hook input event=%s raw bytes=%d\n", in.Event, len(in.Raw))

	n, err := codex.DecodeNotify(`{"type":"agent-turn-complete","turn-id":"t1"}`)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "notify type=%s\n", n.Type)
	return nil
}
```

## Step 3 — Read execpolicy rules

`Rules(h, dir)` parses every `*.rules` file in `<home>/rules` and `<dir>/.codex/rules`. Execpolicy files are Starlark: only `prefix_rule(pattern=[…], decision="…")` calls are understood, each becoming a `PrefixRule` with its file and line; every other statement is returned as a `RuleParseIssue`, never evaluated. `ParseRules` does the same for bytes. `RuleDecision` is `allow`, `prompt` or `forbidden`. `ParseSessionSource` converts the `threads.source` text of the SQLite index (a plain string or the JSON of an object variant) to a `SessionSource`.

<!-- snippet: examples/codexconfig/hooks.go#rules -->
```go
// readRules parses execpolicy *.rules files: prefix_rule calls become
// typed rules; anything else is reported as an issue, never evaluated.
func readRules(w io.Writer, m machine) error {
	a := codex.New(m.host)
	rules, issues, err := a.Rules(a.DefaultHome(), m.project)
	if err != nil {
		return err
	}
	for _, r := range rules {
		fmt.Fprintf(w, "rule %v -> %s (%s:%d)\n", r.Pattern, r.Decision, m.rel(r.File), r.Line)
	}
	for _, is := range issues {
		fmt.Fprintf(w, "issue %s:%d %s\n", m.rel(is.File), is.Line, is.Detail)
	}

	// ParseRules never fails outright: a statement it cannot read comes back
	// as an issue, so an empty result must be checked before indexing.
	parsed, parseIssues := codex.ParseRules([]byte(`prefix_rule(pattern=["rm"], decision="forbidden")`), "inline.rules")
	if len(parsed) == 0 {
		return fmt.Errorf("inline rule not parsed: %v", parseIssues)
	}
	fmt.Fprintf(w, "parsed %v -> %s\n", parsed[0].Pattern, parsed[0].Decision)

	// threads.source text in the SQLite index is a string or a JSON object.
	fmt.Fprintf(w, "source %s %s\n", codex.ParseSessionSource("cli"),
		codex.ParseSessionSource(`{"subagent":{"thread_spawn":{"depth":1}}}`))
	return nil
}
```

## Step 4 — Install and remove hooks

`<home>/hooks.json` is the one Codex file the `Writer` edits: Codex reads it but never writes it. `a.Writer().EnsureHook(h, event, command)` appends a command hook unless one with the same command is already on that event (then nothing is written), and `a.Writer().RemoveHooks(h, match)` removes the command hooks whose command `match` reports true, dropping a group, event or `hooks` key only when it is left empty. Both splice into the file's bytes and back it up first; the `description`, other tools' hooks and unknown keys keep their exact bytes.

The neutral `agentio.HooksOf(a.Neutral())` does the same through kinds (prompt-submit `UserPromptSubmit`, turn-end `Stop`, session-start `SessionStart`, session-end `SessionEnd`) with a plan you can show first; see [Hooks: install and decode](../capabilities/hooks.md). `config.toml` stays read-only, so its plan reports two steps as `Manual` instead of taking them: Codex asks you to review a new or changed hook before it runs it (the trust is recorded in `config.toml` `hooks.state`), and a `[features] hooks = false` must be set back to true (hooks are stable and on by default in 0.162.1).

## Try it

```sh
cd examples && go run ./codexconfig
```

The `== hooks and rules` section:

```text
== hooks and rules
config hook PermissionRequest matcher="Bash" type=command command="/opt/test/hooks/permission.sh" timeout=10s
config hook PreToolUse matcher="shell" type=command command="/opt/test/hooks/pre-tool.sh" timeout=0s
config hook PreToolUse matcher="shell" type=mcp_tool command="" timeout=0s
config hook SessionStart matcher="" type=command command="/opt/test/hook.sh" timeout=30s
config hook Stop matcher="" type=command command="/opt/test/hooks/stop.sh" timeout=0s
config hook UserPromptSubmit matcher="" type=command command="/opt/test/hooks/prompt.sh" timeout=0s
state /home/test/.codex/hooks.json:session_start:0:0 enabled-set=false enabled=false
state /home/test/.codex/hooks.json:stop:0:0 enabled-set=true enabled=false
hooks.json "test hooks" events=2
parsed Stop groups=1 warnings=0 known=true
hook input event=PreToolUse raw bytes=52
notify type=agent-turn-complete
rule [python3 -m http.server] -> allow (~/.codex/rules/default.rules:1)
rule [ollama show] -> allow (~/.codex/rules/default.rules:2)
rule [git status] -> prompt (~/.codex/rules/default.rules:5)
rule [npm test] -> allow (~/work/proj/.codex/rules/a.rules:1)
issue ~/.codex/rules/default.rules:4 statement is not a prefix_rule call
parsed [rm] -> forbidden
source cli subagent
```

## Pitfalls

- **Hooks live in two places.** `[hooks]` in `config.toml` and `<home>/hooks.json` both define event groups; read both. `hooks.state` keys name the hooks file, so a state entry can refer to `hooks.json` handlers.
- **Unset is not zero.** `timeout=0s` above means the key is absent (`Timeout.Set=false`); `additionalContextLimit = 0` is set and disables context spilling. A `hooks.state` entry without `enabled` (`Enabled.Set=false`) is trusted, not disabled.
- **`mcp_tool` handlers have no `Command`.** Read `Server`, `Tool` and `Input` (TOML source text) or `InputJSON` (from `hooks.json`).
- **A bad handler degrades, the file still reads.** `ParseHooksFile` keeps an undecodable handler's raw keys in `UnknownJSON` with a warning.
- **Rules are never executed.** `load(...)` and any other Starlark statement is an issue, so a rule set that builds rules programmatically reads as issues only. Do not treat `Rules` as Codex's final policy decision.
- **Only `hooks.json` is written.** Hooks in the `[hooks]` table of `config.toml` are read, never changed; `RemoveHooks` does not see them.
- **A new hook does not run until it is trusted.** After an install, start Codex on that home and review the hooks once.

## Related

- [Codex config.toml](config.md), [Codex instructions, skills and plugins](instructions-skills.md), [Hooks: install and decode](../capabilities/hooks.md)
- [Claude hooks](../claude/hooks.md), [Claude permissions](../claude/permissions.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
