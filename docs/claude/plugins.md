# Claude plugins and marketplaces

Claude Code plugins bundle skills, commands, subagents, hooks, MCP servers and output styles; they are installed from marketplaces (usually GitHub repos). The `claude` module reads what is installed and enabled, which marketplaces are known, the blocklist and a plugin's manifest, all from disk, and builds the `claude plugin` commands that change them. Use it to show a plugin inventory or to script plugin setup.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent, a home and optionally a project (see [The Claude agent](agent.md)).
- Files: `<home>/plugins/installed_plugins.json`, `known_marketplaces.json`, `blocklist.json`, and each plugin's `.claude-plugin/plugin.json`. Enablement comes from the `enabledPlugins` key of the merged settings.

The snippets come from [`examples/claudeextensions`](../../examples/claudeextensions/), which points the fixture's installed plugin at a plugin dir it creates in the temp home.

## Step 1 — List installed plugins

`a.Plugins(h, &p)` returns `[]InstalledPlugin`: `ID` (a `PluginID`, `<plugin>@<marketplace>`; `ID.Split()` returns both parts), `Scope`, `InstallPath`, `Version`, `InstalledAt`, `LastUpdated`, `Enabled` (the merged `enabledPlugins` says true), `ProjectEnabled` (a project or local layer enables it), `DataDir` and `Unknown`.

<!-- snippet: examples/claudeextensions/main.go#plugins -->
```go
// showPlugins lists installed plugins with their enablement.
func showPlugins(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("plugins")
	plugins, err := a.Plugins(a.DefaultHome(), &proj) // another file version is ErrUnknownVersion
	if err != nil {
		return err
	}
	for _, pl := range plugins {
		name, market := pl.ID.Split()
		p.line(fmt.Sprintf("%s (plugin=%s marketplace=%s) v%s scope=%s enabled=%t", pl.ID, name, market, pl.Version, pl.Scope, pl.Enabled))
		p.line(fmt.Sprintf("  install=%s data=%s", pl.InstallPath, pl.DataDir))
	}
	return nil
}
```

## Step 2 — List marketplaces and the blocklist

`a.Marketplaces(h)` merges `known_marketplaces.json` with the `extraKnownMarketplaces` declared in user settings, sorted by name. A `Marketplace` has `Name`, `Source` (`MarketplaceSource`: `Kind` — `github`, `git`, `url`, `directory`, `command` — plus `Repo`, `URL` or `Path`), `InstallLocation`, `LastUpdated` and `Declared` (known only from settings, not cloned yet). In settings, each declaration is a `MarketplaceDecl`. `a.PluginBlocklist(h)` reads `blocklist.json` into a `Blocklist` (`FetchedAt`, `Plugins` raw); the zero value when absent.

<!-- snippet: examples/claudeextensions/main.go#marketplaces -->
```go
// showMarketplaces lists cloned and declared marketplaces and the blocklist.
func showMarketplaces(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("marketplaces and blocklist")
	h := a.DefaultHome()
	markets, err := a.Marketplaces(h)
	if err != nil {
		return err
	}
	for _, m := range markets {
		p.line(fmt.Sprintf("%s source=%s repo=%s declared=%t updated=%s", m.Name, m.Source.Kind, m.Source.Repo, m.Declared, m.LastUpdated.UTC().Format(time.RFC3339)))
	}
	bl, err := a.PluginBlocklist(h) // zero value when the file is absent
	if err != nil {
		return err
	}
	if bl.FetchedAt != nil {
		p.line(fmt.Sprintf("blocklist fetched %s entries=%s", bl.FetchedAt.UTC().Format(time.RFC3339), bl.Plugins))
	}
	return nil
}
```

## Step 3 — Read a plugin manifest

`a.PluginManifest(installPath)` reads `<installPath>/.claude-plugin/plugin.json`: `Name`, `Version`, `Description` and the component lists (`Skills`, `Agents`, `Commands`, `Hooks`, `MCPServers`, `OutputStyles`), each accepting a string or a list in the file. Other keys stay in `Unknown`.

<!-- snippet: examples/claudeextensions/main.go#manifest -->
```go
// showManifest reads the installed plugin's .claude-plugin/plugin.json.
func showManifest(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("plugin manifest")
	plugins, err := a.Plugins(a.DefaultHome(), &proj)
	if err != nil || len(plugins) == 0 {
		return err
	}
	m, err := a.PluginManifest(plugins[0].InstallPath)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("name=%s version=%s skills=%v agents=%v unknown=%v", m.Name, m.Version, m.Skills, m.Agents, m.Unknown.Keys()))
	return nil
}
```

## Step 4 — Build plugin commands

`PluginInstallCommand(h, id, scope)`, `PluginEnableCommand`, `PluginDisableCommand`, `MarketplaceAddCommand(h, source)` and `MarketplaceUpdateCommand(h, name)` build the `claude plugin …` commands for the home. They are Pure and run nothing.

<!-- snippet: examples/claudeextensions/main.go#plugincommands -->
```go
// showPluginCommands builds the `claude plugin` commands; nothing runs.
func showPluginCommands(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("plugin commands")
	h := a.DefaultHome()
	p.line(a.PluginInstallCommand(h, pluginID, claude.PluginScopeProject).String())
	p.line(a.PluginEnableCommand(h, pluginID).String())
	p.line(a.PluginDisableCommand(h, pluginID).String())
	p.line(a.MarketplaceAddCommand(h, "anthropics/claude-plugins-official").String())
	p.line(a.MarketplaceUpdateCommand(h, "claude-plugins-official").String())
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudeextensions
```

The sections this page covers:

```text
== plugins
demo-plugin@claude-plugins-official (plugin=demo-plugin marketplace=claude-plugins-official) v1.0.0 scope=user enabled=false
  install=~/.claude/plugins/cache/claude-plugins-official/demo-plugin/1.0.0 data=~/.claude/plugins/data/demo-plugin-claude-plugins-official

== marketplaces and blocklist
claude-plugins-official source=github repo=anthropics/claude-plugins-official declared=false updated=2026-11-13T19:17:32Z
blocklist fetched 2026-11-13T10:00:00Z entries=[]

== plugin manifest
name=demo-plugin version=1.0.0 skills=[./skills] agents=[./agents/helper.md] unknown=[homepage]

== plugin commands
claude plugin install demo-plugin@claude-plugins-official -s project
claude plugin enable demo-plugin@claude-plugins-official
claude plugin disable demo-plugin@claude-plugins-official
claude plugin marketplace add anthropics/claude-plugins-official
claude plugin marketplace update claude-plugins-official
```

The fixture's settings do not list the plugin in `enabledPlugins`, so it is installed but `enabled=false`.

## Pitfalls

- **Installed is not enabled.** A plugin in `installed_plugins.json` does nothing until `enabledPlugins` in some settings layer says true; read `Enabled` and `ProjectEnabled`.
- `Plugins` supports `installed_plugins.json` version 2 only; another version returns `agentio.ErrUnknownVersion` (wrapped, naming the version) rather than a guess.
- `InstallPath` is an absolute path recorded by Claude Code on the machine that installed the plugin. A copied home can point at paths that do not exist; `PluginManifest` then fails and plugin skills are not listed. The doctor's `plugins-missing` check reports this.
- The blocklist entry shape was never sampled, so `Blocklist.Plugins` stays raw JSON.
- The library never runs `claude plugin` itself. For the CLI's own view (which runs `claude` and may touch the login), use `Runner().Plugins` and `Runner().Marketplaces` (see [Claude runner](runner.md)).

## Related

- [Claude skills, slash commands and workflows](skills.md), [Claude subagents](subagents.md), [Claude output styles](output-styles.md), [Claude MCP servers](mcp.md), [Claude doctor](doctor.md)
- Neutral view: [Config: skills and plugins](../capabilities/config.md)
- Example: [`examples/claudeextensions`](../../examples/claudeextensions/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
