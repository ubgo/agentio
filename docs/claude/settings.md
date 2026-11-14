# Claude settings

Claude Code reads settings from up to five layers (managed, CLI `--settings`, local, project, user) and merges them. The `claude` module reads each layer as a typed `claude.Settings`, merges them with Claude Code's rules, and tells you which layer set each key. Use it to show "why is my model sonnet?", to audit hooks or permissions, or to check what environment a session gets.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent, a home and (for project layers) a `claude.Project` (see [The Claude agent](agent.md)).
- Files read: `<home>/settings.json` (user), `<project>/.claude/settings.json` (project), `<repo root>/.claude/settings.local.json` (local), the managed files under the OS's managed dir, and `<home>/remote-settings.json` (server-managed cache). All reads are Disk reads.

The snippets come from [`examples/claudesettings`](../../examples/claudesettings/).

## Step 1 — Read one settings layer

`UserSettings(h)`, `ProjectSettings(p)` and `LocalSettings(p)` each return a `SettingsLayer`: `Scope`, `Source` (kind and path), `Present` (the file exists), `Settings` and `Err` (a parse error kept per layer). `Settings` types the keys the library reads (`Model`, `Permissions`, `Hooks`, `Env`, `StatusLine`, `EnabledPlugins`, `Sandbox`, …); every other key lands in `Settings.Unknown`, and a key whose type changed lands there too with a warning in `Settings.Warnings`. `claude.ParseSettings(name, body)` is the same decode over bytes you already hold.

<!-- snippet: examples/claudesettings/main.go#layers -->
```go
// showSettingsLayers reads the user, project and local layers one by one and
// parses a settings body held in memory.
func showSettingsLayers(p printer, host agentio.Host) error {
	p.section("settings layers")
	a := claude.New(host)
	proj, err := a.Project(filepath.Join(host.UserHome, fixtureProject))
	if err != nil {
		return err
	}
	user, err := a.UserSettings(a.DefaultHome())
	if err != nil {
		return err
	}
	project, err := a.ProjectSettings(proj)
	if err != nil {
		return err
	}
	local, err := a.LocalSettings(proj)
	if err != nil {
		return err
	}
	for _, l := range []claude.SettingsLayer{user, project, local} {
		p.line(fmt.Sprintf("%s present=%t file=%s model=%q unknown keys=%v", l.Scope, l.Present, l.Source.Path, l.Settings.Model, l.Settings.Unknown.Keys()))
	}

	s, err := claude.ParseSettings("inline.json", []byte(`{"model": 7, "theme": "dark"}`))
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("ParseSettings: theme=%q model kept in Unknown=%v warnings=%d", s.Theme, s.Unknown.Keys(), len(s.Warnings)))
	return nil
}
```

## Step 2 — Read the effective settings

`a.Settings(h, &p, cli)` reads every layer (plus `cli`, a `--settings` value you parsed, or `nil`) and returns `EffectiveSettings`: `Layers` highest first, `Merged`, and `Origin` (top-level key → the layer that set it). Merge rules follow the Claude Code docs: lists concatenate across layers (except `fallbackModel` and `availableModels`, which the highest layer wins), the objects `env`, `enabledPlugins`, `modelSettings`, `extraKnownMarketplaces`, `hooks` and `permissions` merge per key, and every other key is won by the highest layer.

<!-- snippet: examples/claudesettings/main.go#effective -->
```go
// showEffectiveSettings merges every layer the way Claude Code does and
// says which layer won each key.
func showEffectiveSettings(p printer, host agentio.Host) error {
	p.section("effective settings")
	a := claude.New(host)
	proj, err := a.Project(filepath.Join(host.UserHome, fixtureProject))
	if err != nil {
		return err
	}
	eff, err := a.Settings(a.DefaultHome(), &proj, nil) // nil: no --settings value
	if err != nil {
		return err
	}
	for _, l := range eff.Layers {
		p.line(fmt.Sprintf("layer %s present=%t", l.Scope, l.Present))
	}
	m := eff.Merged
	p.line(fmt.Sprintf("model=%q from %s", m.Model, eff.Origin["model"]))
	p.line(fmt.Sprintf("default mode=%s allow=%v", m.Permissions.DefaultMode, m.Permissions.Allow))
	p.line(fmt.Sprintf("Stop hook groups=%d (user + project)", len(m.Hooks[claude.HookStop])))
	p.line(fmt.Sprintf("env=%v unknown keys=%v", m.Env, m.Unknown.Keys()))
	return nil
}
```

## Step 3 — Read managed and remote settings

`a.ManagedSettings(h)` returns the admin layers that are present: the server-managed cache (`remote-settings.json`, when non-empty), the managed drop-in dir (later files first) and the managed settings file. `a.RemoteSettings(h)` returns the remote cache as a layer plus its sync time from `remote-settings.json.synced-at`. On macOS, admins can also push settings as a managed preferences plist; `a.Runner().ManagedPlist(ctx)` converts it with `plutil` and returns it as a managed layer (a Process cost, behind the Runner door; it never runs `claude`).

<!-- snippet: examples/claudesettings/main.go#managed -->
```go
// showManagedSettings reads the admin layers and the server-managed cache.
func showManagedSettings(p printer, host agentio.Host) error {
	p.section("managed and remote settings")
	a := claude.New(host)
	h := a.DefaultHome()
	managed, err := a.ManagedSettings(h) // only present layers come back
	if err != nil {
		return err
	}
	for _, l := range managed {
		p.line(fmt.Sprintf("managed layer source=%s file=%s", l.Source.Kind, l.Source.Path))
	}
	remote, synced, err := a.RemoteSettings(h)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("remote present=%t synced=%s", remote.Present, synced.UTC().Format(time.RFC3339)))
	return nil
}
```

## Step 4 — List environment variables in effect

`a.EnvInEffect(h, &p)` lists every known variable set in `Host.Env` (`From` is `claude.EnvFromProcess`) followed by every `env` entry of every settings layer, highest first, so you see both a process value and the layer value that would override it. Values are `agentio.EnvValue` and print `[redacted]`; call `Reveal()` only when you must. `claude.KnownEnv()` is the table of variables the library knows, each with its `EnvGroup`, `Secret` and `Deprecated` flags.

<!-- snippet: examples/claudesettings/main.go#env -->
```go
// showEnv lists the variables Claude Code would see, with the layer that
// set each; values print redacted.
func showEnv(p printer, host agentio.Host) error {
	p.section("environment in effect")
	a := claude.New(host)
	proj, err := a.Project(filepath.Join(host.UserHome, fixtureProject))
	if err != nil {
		return err
	}
	env, err := a.EnvInEffect(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, e := range env {
		p.line(fmt.Sprintf("%s=%s from=%s secret=%t", e.Name, e.Value, e.From, e.Secret))
	}
	var secrets []string
	for _, v := range claude.KnownEnv() {
		if v.Secret {
			secrets = append(secrets, v.Name)
		}
	}
	p.line(fmt.Sprintf("known variables=%d, secret ones=%v", len(claude.KnownEnv()), secrets))
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudesettings
```

The sections this page covers:

```text
== settings layers
user present=true file=~/.claude/settings.json model="opus" unknown keys=[]
project present=true file=~/project/.claude/settings.json model="" unknown keys=[futureKey]
local present=true file=~/project/.claude/settings.local.json model="sonnet" unknown keys=[]
ParseSettings: theme="dark" model kept in Unknown=[model] warnings=1

== effective settings
layer local present=true
layer project present=true
layer user present=true
model="sonnet" from local
default mode=bypassPermissions allow=[Bash(go test *)]
Stop hook groups=2 (user + project)
env=map[FIXTURE_FLAG:1] unknown keys=[futureKey]

== managed and remote settings
remote present=true synced=2026-11-13T19:38:51Z

== environment in effect
FIXTURE_FLAG=[redacted] from=local secret=false
known variables=56, secret ones=[CLAUDE_CODE_OAUTH_TOKEN ANTHROPIC_API_KEY ANTHROPIC_AUTH_TOKEN ANTHROPIC_FOUNDRY_API_KEY ANTHROPIC_FOUNDRY_AUTH_TOKEN AWS_BEARER_TOKEN_BEDROCK ANTHROPIC_AWS_API_KEY CLAUDE_CODE_MESSAGING_TOKEN]
```

The fixture's remote cache is `{}`, so `ManagedSettings` returns no layer while `RemoteSettings` still reports the file and its sync time.

## Pitfalls

- **A broken layer does not fail the call.** `Settings` keeps a layer that fails to parse with `Err` set and skips it in the merge; check `Layers[i].Err` (or the doctor's `settings-parse` check) before trusting `Merged`.
- **A changed key type degrades, it does not fail.** `ParseSettings` puts `"model": 7` into `Unknown` with an `agentio.WarnTypeChanged` warning and still decodes the rest. Read `Settings.Warnings` when you report drift.
- Settings are presence-tracked: an absent scalar is a nil pointer or `""`, an absent list or map is nil. `Merged.Permissions` is nil when no layer has a `permissions` key, so guard before dereferencing.
- `Origin` names the highest layer for keys that merge (`hooks`, `env`, …), even when lower layers contributed entries.
- `ManagedSettings` reads absolute OS paths through the real filesystem (see [Claude paths](paths.md)): a fixture Host does not hide an admin's `/etc/claude-code/managed-settings.json` on the machine running the code.
- `EnvInEffect` lists only known variables from the process environment; an unknown process variable is not listed, while every `env` key of a settings layer is.
- Settings files are only read here. To change one, use the [Claude writer](writer.md), which backs the file up first.

## Related

- [Claude permissions](permissions.md), [Claude hooks](hooks.md), [Claude status line](statusline.md), [Claude policy limits](policy.md), [Claude writer](writer.md)
- Neutral view: [Config layers](../capabilities/config.md)
- Example: [`examples/claudesettings`](../../examples/claudesettings/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
