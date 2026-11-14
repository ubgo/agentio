# Codex instructions, skills and plugins

Which `AGENTS.md` files Codex loads for a folder and in what order, which skills it can see (user, bundled system, project, curated vendor imports, plugin-shipped), and which plugins are installed and enabled. All disk reads; nothing is executed.

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` and `codex.Home` from [The Codex agent](agent.md).
- A project folder with a root marker (`.git` by default, or the user config's `project_root_markers`) so the walk from root to folder works.

The snippets come from [`examples/codexconfig`](../../examples/codexconfig/), whose temp project has `.git/` and an `AGENTS.md`.

## Step 1 — List instruction files

`InstructionFiles(h, dir)` lists, in load order, `<home>/AGENTS.md` (scope user), then for each directory from the project root down to `dir` the first existing of `AGENTS.override.md`, `AGENTS.md`, then each `project_doc_fallback_filenames` entry (scope project). `Truncated` marks the file where the `project_doc_max_bytes` budget runs out. `ProjectRoot(h, dir)` is the root Codex walks from; `found=false` means no marker, and Codex then reads only `dir` itself.

<!-- snippet: examples/codexconfig/extensions.go#instructions -->
```go
// listInstructions lists the AGENTS.md files Codex would load for a
// folder, in load order, and the project root it walks from.
func listInstructions(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()
	files, err := a.InstructionFiles(h, m.project)
	if err != nil {
		return err
	}
	for _, f := range files {
		fmt.Fprintf(w, "instructions %s scope=%s kind=%s bytes=%d truncated=%v\n",
			m.rel(f.Path), f.Scope, f.Kind, f.Bytes, f.Truncated)
	}
	root, found, err := a.ProjectRoot(h, m.project)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "project root=%s found=%v\n", m.rel(root), found)
	return nil
}
```

## Step 2 — List skills

`Skills(h, dir)` reads `SKILL.md` front matter (first 8 KiB only) from `<home>/skills`, the bundled `skills/.system`, project `.codex/skills` from `dir` up to the project root, curated vendor imports (scope `codex.SkillScopeVendor`) and skills inside installed plugins. A `[[skills.config]]` entry with `enabled = false` marks a skill disabled by path or name. Other front-matter scalars land in `Skill.Extra`.

<!-- snippet: examples/codexconfig/extensions.go#skills -->
```go
// listSkills reads SKILL.md front matter from user, system, project,
// vendor and plugin skill folders.
func listSkills(w io.Writer, m machine) error {
	a := codex.New(m.host)
	skills, err := a.Skills(a.DefaultHome(), m.project)
	if err != nil {
		return err
	}
	for _, s := range skills {
		fmt.Fprintf(w, "skill %s scope=%s enabled=%v plugin=%q\n", s.Name, s.Scope, s.Enabled, s.Plugin)
	}
	return nil
}
```

## Step 3 — List plugins and marketplaces

`Plugins(h)` walks `plugins/cache/<marketplace>/<plugin>/`, picks the version directory with the newest mtime, reads its manifest (`Interface`, unknown keys kept), and joins `[plugins."<plugin>@<marketplace>"] enabled`. `Marketplaces(cfg)` lists `[marketplaces.*]` from a config you already read. `PluginListCommand(h, available)` builds `codex plugin list --json [--available]`, which reads remote catalogs when run.

<!-- snippet: examples/codexconfig/extensions.go#plugins -->
```go
// listPlugins joins installed plugin folders with config enablement, and
// lists configured marketplaces.
func listPlugins(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()
	plugins, err := a.Plugins(h)
	if err != nil {
		return err
	}
	for _, p := range plugins {
		fmt.Fprintf(w, "plugin %s@%s version=%s enabled=%v display=%q\n",
			p.Name, p.Marketplace, p.Version, p.Enabled, p.Interface.DisplayName)
	}
	cfg, err := a.ConfigFile(h)
	if err != nil {
		return err
	}
	for _, mp := range a.Marketplaces(cfg) {
		fmt.Fprintf(w, "marketplace %s type=%s source=%s\n", mp.Name, mp.SourceType, mp.Source)
	}
	// Running this command reads remote catalogs; it is only built here.
	fmt.Fprintln(w, argv(a.PluginListCommand(h, true)))
	return nil
}
```

## Try it

```sh
cd examples && go run ./codexconfig
```

The `== instructions and skills` section:

```text
== instructions and skills
instructions ~/.codex/AGENTS.md scope=user kind=AGENTS.md bytes=0 truncated=false
instructions ~/work/proj/AGENTS.md scope=project kind=AGENTS.md bytes=16 truncated=false
project root=~/work/proj found=true
skill squad scope=user enabled=true plugin=""
skill imagegen scope=system enabled=true plugin=""
skill pr-review scope=plugin enabled=true plugin="github"
plugin github@openai-curated version=26.928.31416 enabled=true display="GitHub"
marketplace openai-bundled type=local source=/opt/test/marketplace
codex plugin list --json --available
```

## Pitfalls

- **Untrusted folders get no project docs.** `InstructionFiles` skips project files for a folder that is not trusted in `[projects]`, as Codex does; only the user `AGENTS.md` is listed.
- **`AGENTS.override.md` shadows `AGENTS.md` in the same directory**, it does not add to it.
- **An empty user `AGENTS.md` is still listed** (`bytes=0`): it exists, so Codex loads it.
- **An empty `project_root_markers = []` disables the parent walk.** `Config.ProjectRootMarkersSet` tells that apart from the absent key (default `.git`).
- **Plugin version choice is a heuristic.** The newest-mtime version directory is picked; how Codex itself picks one is not verified.
- **Front matter only.** Skill bodies are never read; a front matter past 8 KiB is cut.

## Related

- [Instruction files, skills, plugins (neutral)](../capabilities/config.md)
- [Codex config.toml](config.md), [Codex hooks and rules](hooks-rules.md)
- [Claude memory files](../claude/memory.md), [Claude skills](../claude/skills.md), [Claude plugins](../claude/plugins.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
