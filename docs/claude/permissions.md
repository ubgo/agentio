# Claude permissions

Permission rules decide which tools Claude Code may use without asking. They live in the `permissions` object of every settings layer (`allow`, `ask`, `deny` lists and a `defaultMode`). `a.Permissions(h, &p)` lists every rule with the list and layer it came from, and the mode that wins. Use it to audit what a project allows, or to explain why a tool did or did not prompt.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent, a home and a project (see [The Claude agent](agent.md)).
- Reads the same files as [Claude settings](settings.md); Disk read.

The snippets come from [`examples/claudesettings`](../../examples/claudesettings/).

## Step 1 — Read the effective permissions

`EffectivePermissions` has `Rules` (each a `ScopedRule`: the parsed `Rule`, its `Kind` — `RuleAllow`, `RuleAsk`, `RuleDeny` — its `Scope` and a parse `Err`), `DefaultMode` and `ModeSource` (the layer that set the mode). Rules come highest layer first. The full `Permissions` object of one layer (with `AdditionalDirectories`, `DisableBypassPermissionsMode` and the `blockReads…`/`blockWrites…` switches) is `Settings.Permissions` on a layer or on the merged settings.

<!-- snippet: examples/claudesettings/main.go#permissions -->
```go
// showPermissions lists every rule of every layer and the winning mode.
func showPermissions(p printer, host agentio.Host) error {
	p.section("permissions")
	a := claude.New(host)
	proj, err := a.Project(filepath.Join(host.UserHome, fixtureProject))
	if err != nil {
		return err
	}
	perms, err := a.Permissions(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("default mode=%s set by %s", perms.DefaultMode, perms.ModeSource))
	for _, r := range perms.Rules {
		p.line(fmt.Sprintf("%s %s tool=%s specifier=%q", r.Scope, r.Kind, r.Rule.Tool, r.Rule.Specifier))
	}
	return nil
}
```

## Step 2 — Parse a rule

`claude.ParseRule` splits `Bash(git *)` into `Tool: Bash` and `Specifier: git *`; a bare tool name has an empty specifier. An unbalanced parenthesis is an error.

<!-- snippet: examples/claudesettings/main.go#rules -->
```go
// showRules parses rule strings the way they appear in settings files.
func showRules(p printer, _ agentio.Host) error {
	p.section("parse rules")
	for _, s := range []string{"Bash(git *)", "Read", "mcp__github__get_issue", "Bash(git *"} {
		r, err := claude.ParseRule(s)
		if err != nil {
			p.line(fmt.Sprintf("%q: %v", s, err))
			continue
		}
		p.line(fmt.Sprintf("%q -> tool=%s specifier=%q", s, r.Tool, r.Specifier))
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudesettings
```

The sections this page covers:

```text
== permissions
default mode=bypassPermissions set by user
project allow tool=Bash specifier="go test *"

== parse rules
"Bash(git *)" -> tool=Bash specifier="git *"
"Read" -> tool=Read specifier=""
"mcp__github__get_issue" -> tool=mcp__github__get_issue specifier=""
"Bash(git *": claude: invalid permission rule "Bash(git *"
```

## Pitfalls

- **`default` and `manual` are one mode.** `PermissionMode` decodes both spellings to `claude.ModeManual`, so comparing against the string `"default"` never matches; compare against the constant.
- A rule that does not parse is still listed, with `Err` set and the text in `Rule.Raw`. Filter on `Err` before acting on rules.
- `PermissionMode` is an open set: a mode a newer Claude Code adds decodes with `IsKnown()` false instead of failing.
- The library lists rules; it does not evaluate them against a tool call. Deny beats ask beats allow in Claude Code, and a managed layer can lock settings; reproduce that only if you need to.
- MCP tool rules use the `mcp__<server>__<tool>` names with no parentheses.

## Related

- [Claude settings](settings.md), [Claude hooks](hooks.md) (a `PreToolUse` hook can deny a call too), [Claude policy limits](policy.md)
- Neutral view: [Config layers](../capabilities/config.md)
- Example: [`examples/claudesettings`](../../examples/claudesettings/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
