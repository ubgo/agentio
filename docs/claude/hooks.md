# Claude hooks

Hooks are programs Claude Code runs on events (before a tool call, when a session stops, when a prompt is submitted, …). The `claude` module covers both sides: listing the hooks configured in every settings layer, and writing a hook program in Go that decodes Claude Code's JSON input and replies. Use it to audit hooks, or to build a hook (a policy gate, a logger, a notifier) with typed input instead of hand-parsed JSON.

## Before you start

- `go get github.com/ubgo/agentio/claude`; for listing, an agent, a home and optionally a project (see [The Claude agent](agent.md)). A hook program needs no agent or Host: it only reads stdin and writes stdout.
- To add or remove hooks in a settings file, see [Claude writer](writer.md) (`AddHook`, the idempotent `EnsureHook`, `RemoveHooks`). To install one command into Claude Code, Codex and Gemini CLI alike, see [Hooks: install and decode](../capabilities/hooks.md).

The snippets come from [`examples/claudehooks`](../../examples/claudehooks/). Its `hook` mode is a complete hook program.

## Step 1 — List configured hooks

`a.Hooks(h, &p)` flattens every handler in effect into `ScopedHook` values: `Event`, `Matcher`, `Handler`, `Scope` (managed, local, project, user, in that order) and `Source`. One settings layer's `hooks` object is `claude.Hooks` (event → `[]HookMatcher`); a `HookMatcher` groups `HookHandler`s under one matcher string. A `HookHandler` carries the handler `Type` (`command`, `http`, `mcp_tool`, `prompt`, `agent`), the type's fields (`Command`, `URL`, `Prompt`, …) and `Timeout` as a `time.Duration`; unmodelled keys stay in `Unknown` and are written back on a round trip.

<!-- snippet: examples/claudehooks/main.go#hooks -->
```go
// showHooks lists every hook handler in effect, with the layer it came from,
// counted per event so the output stays short.
func showHooks(p printer, a *claude.Agent) error {
	p.section("configured hooks")
	proj, err := project(a)
	if err != nil {
		return err
	}
	hooks, err := a.Hooks(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	type key struct {
		scope   claude.SettingsScope
		event   claude.HookEvent
		matcher string
	}
	count := map[key]int{}
	var keys []key
	for _, h := range hooks {
		k := key{h.Scope, h.Event, h.Matcher}
		if count[k] == 0 {
			keys = append(keys, k)
		}
		count[k]++
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].scope != keys[j].scope {
			return keys[i].scope < keys[j].scope
		}
		if keys[i].event != keys[j].event {
			return keys[i].event < keys[j].event
		}
		return keys[i].matcher < keys[j].matcher
	})
	for _, k := range keys {
		p.line(fmt.Sprintf("%-7s %-17s matcher=%-20q handlers=%d", k.scope, k.event, k.matcher, count[k]))
	}
	first := hooks[0].Handler
	p.line(fmt.Sprintf("first handler: type=%s command=%s timeout=%s", first.Type, first.Command, first.Timeout))
	return nil
}
```

`claude.HookEvents()` lists the documented events, `HookEvent.SupportsMatcher()` says whether an event filters on a matcher, and `HookMatcher.Matches(subject)` applies Claude Code's rules: `""` or `"*"` match everything, a plain name list like `Write|Edit` matches names exactly, anything else is a regular expression.

<!-- snippet: examples/claudehooks/main.go#matchers -->
```go
// showMatchers checks which events filter on a matcher and how a matcher
// string is applied to a tool name.
func showMatchers(p printer, _ *claude.Agent) error {
	p.section("events and matchers")
	var withMatcher []claude.HookEvent
	for _, ev := range claude.HookEvents() {
		if ev.SupportsMatcher() {
			withMatcher = append(withMatcher, ev)
		}
	}
	p.line(fmt.Sprintf("documented events=%d, %d take a matcher, e.g. %v", len(claude.HookEvents()), len(withMatcher), withMatcher[:3]))
	p.line(fmt.Sprintf("Stop takes a matcher: %t", claude.HookStop.SupportsMatcher()))
	for _, c := range []struct{ matcher, tool string }{
		{"Write|Edit|MultiEdit", "Edit"},
		{"Write|Edit|MultiEdit", "NotebookEdit"},
		{"mcp__.*__write.*", "mcp__fs__write_file"},
		{"", "Bash"},
	} {
		m := claude.HookMatcher{Matcher: c.matcher}
		p.line(fmt.Sprintf("matcher %-22q tool %-20s -> %t", c.matcher, c.tool, m.Matches(c.tool)))
	}
	return nil
}
```

## Step 2 — Decode hook input

`claude.DecodeHookInput(os.Stdin)` returns a `HookInput`: the common fields (`SessionID`, `TranscriptPath`, `Cwd`, `PermissionMode`, `HookEventName`, `Effort`) plus the per-event ones (`ToolName`, `ToolInput`, `ToolResponse`, `Prompt`, `LastAssistantMessage`, `Source`, `Reason`, model switch fields, …). Keys the library does not model are kept in `Unknown`, and the whole payload in `Raw`. `ToolInput` is raw JSON; decode it into the shape of the tool you gate. `HookInput.Call()` maps the input onto the neutral `agentio.HookCall` (what `HookInstaller.DecodeHook` returns), and `claude.HookKindOf(event)` names the neutral kind of an event.

<!-- snippet: examples/claudehooks/main.go#showhook -->
```go
// showHookProgram feeds the sample PreToolUse input to hookMain and prints
// both what was decoded and what the hook replied.
func showHookProgram(p printer, _ *claude.Agent) error {
	p.section("hook program")
	in, err := claude.DecodeHookInput(strings.NewReader(preToolUseInput))
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("event=%s tool=%s mode=%s effort=%s unknown=%v", in.HookEventName, in.ToolName, in.PermissionMode, in.Effort.Level, in.Unknown.Keys()))
	var reply bytes.Buffer
	if err := hookMain(strings.NewReader(preToolUseInput), &reply); err != nil {
		return err
	}
	p.line(fmt.Sprintf("reply: %s", strings.TrimSpace(reply.String())))
	return nil
}
```

## Step 3 — Reply from a hook

A hook replies by printing one JSON document. `claude.HookOutput` has the common fields (`Continue`, `StopReason`, `SuppressOutput`, `SystemMessage`, `Decision`, `Reason`) and `HookSpecificOutput` for the per-event part (`PermissionDecision`, `PermissionDecisionReason`, `AdditionalContext`, …). `HookOutput.Encode(w)` writes it. This is the whole hook program of the example: it denies Bash commands starting with `rm -rf` and stays silent otherwise.

<!-- snippet: examples/claudehooks/main.go#hookprogram -->
```go
// hookMain is a complete PreToolUse hook: it decodes the input Claude Code
// writes on stdin and denies Bash commands that start with "rm -rf".
// Anything else gets no output, which lets the normal permission flow run.
func hookMain(stdin io.Reader, stdout io.Writer) error {
	in, err := claude.DecodeHookInput(stdin)
	if err != nil {
		return err
	}
	if in.HookEventName != claude.HookPreToolUse || in.ToolName != "Bash" {
		return nil
	}
	var bash struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(in.ToolInput, &bash); err != nil {
		return err
	}
	if !strings.HasPrefix(bash.Command, dangerousPrefix) {
		return nil
	}
	out := claude.HookOutput{
		HookSpecificOutput: &claude.HookSpecificOutput{
			HookEventName:            claude.HookPreToolUse,
			PermissionDecision:       "deny",
			PermissionDecisionReason: "recursive delete is blocked by policy",
		},
	}
	return out.Encode(stdout)
}
```

To use it, build the binary and add it as a `PreToolUse` hook with matcher `Bash` (by hand or with `Writer.AddHook`).

## Try it

```sh
cd examples && go run ./claudehooks
```

The sections this page covers:

```text
== configured hooks
project Stop              matcher=""                   handlers=1
user    Elicitation       matcher=""                   handlers=1
user    Notification      matcher=""                   handlers=1
user    PermissionRequest matcher=""                   handlers=1
user    PostToolUse       matcher=""                   handlers=1
user    PostToolUse       matcher="Write|Edit|MultiEdit" handlers=1
user    PreToolUse        matcher="Bash"               handlers=4
user    SessionEnd        matcher=""                   handlers=1
user    SessionStart      matcher=""                   handlers=1
user    Stop              matcher=""                   handlers=1
user    StopFailure       matcher=""                   handlers=1
user    UserPromptSubmit  matcher=""                   handlers=1
first handler: type=command command=/usr/local/bin/fixture-hook timeout=10s

== events and matchers
documented events=33, 17 take a matcher, e.g. [SessionStart PreToolUse PermissionRequest]
Stop takes a matcher: false
matcher "Write|Edit|MultiEdit" tool Edit                 -> true
matcher "Write|Edit|MultiEdit" tool NotebookEdit         -> false
matcher "mcp__.*__write.*"     tool mcp__fs__write_file  -> true
matcher ""                     tool Bash                 -> true

== hook program
event=PreToolUse tool=Bash mode=manual effort=high unknown=[new_field]
reply: {"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"recursive delete is blocked by policy"}}
```

Run the hook mode directly on the sample input:

```sh
cd examples && go run ./claudehooks hook < claudehooks/testdata/pretooluse.json
```

```text
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"recursive delete is blocked by policy"}}
```

## Pitfalls

- **`Timeout` is seconds on disk and a `time.Duration` in Go.** A handler written with `Timeout: 5 * time.Second` is stored as `5`; a sub-second duration does not survive the round trip.
- **`permission_mode: "default"` decodes to `claude.ModeManual`.** Compare modes against the constants, never the string.
- `Matches` treats a matcher with regex characters as a regex. `Write|Edit` is an exact name list, so it does not match `NotebookEdit`; `.*Edit` would.
- An event that does not take a matcher (`Stop`, `UserPromptSubmit`, …) has no subject to filter on, so a matcher written there does not narrow anything; `SupportsMatcher` tells you which events these are.
- `Hooks` lists what the settings files say. It does not report `disableAllHooks` or plugin hooks as disabled; read `Settings.DisableAllHooks` from the merged settings.
- A hook that prints nothing lets Claude Code continue normally; print a reply only when you decide something.
- Hook input fields beyond the common ones are taken from recorded hook logs and the docs; new fields arrive in `Unknown` first.

## Related

- [Claude writer](writer.md) (add and remove hooks), [Hooks: install and decode](../capabilities/hooks.md), [Claude status line](statusline.md), [Claude settings](settings.md), [Claude permissions](permissions.md)
- Example: [`examples/claudehooks`](../../examples/claudehooks/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
