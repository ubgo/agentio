# Config, MCP, instructions, skills, plugins and models

Both vendors have settings files in layers, MCP servers, instruction files (`CLAUDE.md`, `AGENTS.md`), skills, plugins and, for Codex, a model catalog. The vendor modules model each of these in full detail; the shared capabilities give you the facts both share, in neutral types, so one screen can show "what applies in this folder" for every agent. Each read takes a home and, where it matters, the project folder `dir`, and costs `disk read`.

## Before you start

- Modules: `github.com/ubgo/agentio` plus the agents.
- The homes on disk and, for per-project reads, the project folder. Pass `dir = ""` to ask about no project.
- Nothing is spawned: MCP servers are listed from config, never started.

[`examples/accessors`](../../examples/accessors/) runs every step on copies of the Claude Code 2.1.325 and Codex 0.162.1 fixture homes, with `~/project` as the project folder. The copy matters: instruction-file discovery walks up from the project folder, and inside the repository it would also find the repository's own files.

## Step 1 — Read config layers

`ConfigOf(a)` returns a `ConfigReader` with three reads: `ConfigLayers(home, dir)` (the config files that apply, each with its `ConfigScope` and whether it `Exists`), `ProjectTrust(home, dir)` (a `Trust`: `trusted`, `untrusted`, `unknown`) and `Defaults(home, dir)` (the effective launch `Model` and `Autonomy` after merging the layers).

<!-- snippet: examples/accessors/config.go#config -->
```go
// printConfig prints the config files that apply in dir (precedence order,
// with whether each exists), the project trust and the effective launch
// defaults, for each agent's first home.
func printConfig(w io.Writer, h agentio.Host, agents []agentio.Agent, dir string) error {
	for _, a := range agents {
		cr, ok := agentio.ConfigOf(a)
		if !ok {
			continue
		}
		home, found, err := firstHome(a)
		if err != nil || !found {
			return err
		}
		layers, err := cr.ConfigLayers(home, dir)
		if err != nil {
			return err
		}
		for _, l := range layers {
			fmt.Fprintf(w, "%s layer %-8s exists=%-5t %s\n", a.ID(), l.Scope, l.Exists, tidy(h, l.Path))
		}
		trust, err := cr.ProjectTrust(home, dir)
		if err != nil {
			return err
		}
		def, err := cr.Defaults(home, dir)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s trust=%s model=%q autonomy=%s\n", a.ID(), trust, def.Model, def.Autonomy)
	}
	return nil
}
```

`ConfigScope` is `managed`, `user`, `project`, `local`, `profile`, `cli`, `plugin` or `unknown`. `Defaults.Model` is "" when the vendor default applies; `Defaults.Autonomy` is `AutonomyDefault` when nothing is configured or the vendor mode has no neutral equivalent.

## Step 2 — List MCP servers

`MCPServersOf(a)` lists configured MCP servers for a home and folder. Each `MCPServer` has `Name`, `Transport` (`stdio`, `http`, `sse`, `ws`; open set), `Scope`, `Command` and `Args` or `URL`, `EnvNames`, `Enabled` (nil when the vendor does not say) and `Source`, the file it came from.

<!-- snippet: examples/accessors/config.go#mcp -->
```go
// printMCP lists configured MCP servers without spawning one. Env values
// never appear: MCPServer carries variable names only.
func printMCP(w io.Writer, agents []agentio.Agent, dir string) error {
	for _, a := range agents {
		ml, ok := agentio.MCPServersOf(a)
		if !ok {
			continue
		}
		home, found, err := firstHome(a)
		if err != nil || !found {
			return err
		}
		servers, err := ml.MCPServers(home, dir)
		if err != nil {
			return err
		}
		for _, s := range servers {
			target := s.Command
			if s.URL != "" {
				target = s.URL
			}
			fmt.Fprintf(w, "%s %-14s %-5s scope=%-7s %s env=%v\n", a.ID(), s.Name, s.Transport, s.Scope, target, s.EnvNames)
		}
	}
	return nil
}
```

## Step 3 — List instruction files

`InstructionFilesOf(a)` lists the instruction files that apply in `dir`, in the order the agent loads them. Each `InstructionFile` has `Path`, `Scope` (an `InstructionScope`: `managed`, `user`, `project`, `local`, `auto`, `imported`), `Kind` (the vendor's name for it: `user`, `user-rule`, `project`, `AGENTS.md`, ...) and `Bytes`.

<!-- snippet: examples/accessors/config.go#instructions -->
```go
// printInstructions lists the instruction files that apply in dir, in the
// order the agent loads them.
func printInstructions(w io.Writer, h agentio.Host, agents []agentio.Agent, dir string) error {
	for _, a := range agents {
		in, ok := agentio.InstructionFilesOf(a)
		if !ok {
			continue
		}
		home, found, err := firstHome(a)
		if err != nil || !found {
			return err
		}
		files, err := in.InstructionFiles(home, dir)
		if err != nil {
			return err
		}
		for _, f := range files {
			fmt.Fprintf(w, "%s %-8s %-10s %4d bytes %s\n", a.ID(), f.Scope, f.Kind, f.Bytes, tidy(h, f.Path))
		}
	}
	return nil
}
```

## Step 4 — List skills and plugins

`SkillsOf(a)` lists skills from their `SKILL.md` frontmatter only (`Name`, `Description`, `Dir`, `Plugin`, `Scope`, `Enabled`); the body is never read. `SkillScope` is `user`, `project`, `plugin`, `managed` or `system`. `PluginsOf(a)` lists installed plugins (`Name`, `Marketplace`, `Version`, `Dir`, `Enabled`).

<!-- snippet: examples/accessors/config.go#skills -->
```go
// printSkillsAndPlugins lists skills (frontmatter only) and installed
// plugins for each agent's first home.
func printSkillsAndPlugins(w io.Writer, agents []agentio.Agent, dir string) error {
	for _, a := range agents {
		home, found, err := firstHome(a)
		if err != nil || !found {
			return err
		}
		if sl, ok := agentio.SkillsOf(a); ok {
			skills, err := sl.Skills(home, dir)
			if err != nil {
				return err
			}
			for _, s := range skills {
				fmt.Fprintf(w, "%s skill  %-14s scope=%-7s plugin=%q\n", a.ID(), s.Name, s.Scope, s.Plugin)
			}
		}
		if pl, ok := agentio.PluginsOf(a); ok {
			plugins, err := pl.Plugins(home, dir)
			if err != nil {
				return err
			}
			for _, p := range plugins {
				fmt.Fprintf(w, "%s plugin %s@%s version=%s enabled=%t\n", a.ID(), p.Name, p.Marketplace, p.Version, p.Enabled)
			}
		}
	}
	return nil
}
```

## Step 5 — List models

`ModelsOf(a)` lists an on-disk model catalog: `Slug`, `DisplayName`, `Visible` and `ContextWindow`. Only Codex keeps one (`models_cache.json`); Claude Code has no recorded catalog on disk, so the accessor reports false and `agentio.Unsupported` says why.

<!-- snippet: examples/accessors/config.go#models -->
```go
// printModels lists the on-disk model catalog. Only Codex keeps one; for
// Claude the accessor reports false and Unsupported says why.
func printModels(w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		ml, ok := agentio.ModelsOf(a)
		if !ok {
			reason, _ := agentio.Unsupported(a, agentio.CapModels)
			fmt.Fprintf(w, "%s: no model catalog (%s)\n", a.ID(), reason)
			continue
		}
		home, found, err := firstHome(a)
		if err != nil || !found {
			return err
		}
		models, err := ml.Models(home)
		if err != nil {
			return err
		}
		for _, m := range models {
			fmt.Fprintf(w, "%s model %-16s visible=%-5t context=%d\n", a.ID(), m.Slug, m.Visible, m.ContextWindow)
		}
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./accessors
```

The config through models sections of the output:

```text
== config ==
claude layer local    exists=true  ~/project/.claude/settings.local.json
claude layer project  exists=true  ~/project/.claude/settings.json
claude layer user     exists=true  ~/.claude/settings.json
claude trust=unknown model="sonnet" autonomy=full
codex layer managed  exists=false /etc/codex/config.toml
codex layer user     exists=true  ~/.codex/config.toml
codex layer managed  exists=false /etc/codex/managed_config.toml
codex trust=unknown model="gpt-5.6-terra" autonomy=default
== mcp ==
claude proj-db        stdio scope=project fixture-db-mcp env=[DB_PASSWORD]
claude fixture-http   http  scope=user    https://mcp.example.com/mcp env=[]
claude fixture-stdio  stdio scope=user    fixture-mcp env=[FIXTURE_TOKEN]
codex computer-use   stdio scope=user    /opt/test/computer-use env=[]
codex node_repl      stdio scope=user    /opt/test/node_repl env=[CODEX_HOME SECRET_TOKEN]
== instructions ==
claude user     user         97 bytes ~/.claude/CLAUDE.md
claude user     user-rule    44 bytes ~/.claude/rules/style.md
claude project  project      25 bytes ~/project/CLAUDE.md
codex user     AGENTS.md     0 bytes ~/.codex/AGENTS.md
== skills and plugins ==
claude skill  demo-skill     scope=user    plugin=""
claude skill  proj-skill     scope=project plugin=""
claude plugin demo-plugin@claude-plugins-official version=1.0.0 enabled=false
codex skill  squad          scope=user    plugin=""
codex skill  imagegen       scope=system  plugin=""
codex skill  pr-review      scope=plugin  plugin="github"
codex plugin github@openai-curated version=26.928.31416 enabled=true
== models ==
claude: no model catalog (Claude Code keeps no model catalog on disk that has a recorded shape; a static list would rot (OD-15))
codex model gpt-6-luna       visible=true  context=272000
codex model gpt-reserve      visible=false context=128000
```

## Pitfalls

- **Layer order differs by vendor.** Claude lists layers highest precedence first (local, project, user); Codex lists them in its own resolution order, where the system `config.toml` comes before the user file and `managed_config.toml` after it. Use `Scope` to label a layer; do not infer precedence from the slice index across vendors.
- **Managed layers are absolute system paths.** Codex's managed files live under `/etc/codex` and are stat'ed on the real filesystem even with a fixture Host. They are listed with `Exists` false when absent.
- **MCP env values never appear.** `EnvNames` are names only; the vendor types carry the values as `agentio.EnvValue`, which redacts like a token.
- **Instruction discovery walks up from `dir`.** Claude collects `CLAUDE.md` files from the project folder's ancestors too, so a folder inside another project picks up the outer project's files. Tests should copy fixtures to a temp dir, as the example does.
- **`ProjectTrust` is `unknown` for a folder the agent never recorded.** The fixture project has no trust entry, so both vendors answer `unknown`, not `untrusted`.
- **`Defaults.Autonomy` maps vendor modes.** Claude's `bypassPermissions` reads as `AutonomyFull`; a mode with no neutral equivalent reads as `AutonomyDefault`. The vendor settings types keep the exact mode.
- **Skills are frontmatter only.** `Description` is what the frontmatter says; read the `SKILL.md` body yourself if you need it.

## Related

- [Shared capabilities and accessors](accessors.md), [Doctor checks](doctor.md).
- Vendor pages: [Claude settings](../claude/settings.md), [Claude MCP](../claude/mcp.md), [Claude memory](../claude/memory.md), [Claude skills](../claude/skills.md), [Claude plugins](../claude/plugins.md), [Codex config](../codex/config.md), [Codex MCP](../codex/mcp.md), [Codex instructions and skills](../codex/instructions-skills.md).
- [Values and enums](../core/values.md).
- [Choosing guides](../choosing/README.md). Example: [`examples/accessors`](../../examples/accessors/). Feature index: [features](../reference/features.md).
