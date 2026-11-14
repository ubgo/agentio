# Claude skills, slash commands and workflows

Claude Code picks up skills (`skills/<name>/SKILL.md`), slash commands (`commands/*.md`) and workflows (`workflows/*.js`) from the home, the project and installed plugins. The `claude` module lists each from its frontmatter, with the scope it came from, and exposes the frontmatter parser it uses. Use it to build a picker, check for name clashes, or audit what tools a skill may use.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent, a home and optionally a project (see [The Claude agent](agent.md)). The parser is also usable alone from `github.com/ubgo/agentio/frontmatter` in the root module (stdlib only).
- Listing reads frontmatter only, never a skill body (Disk read).

The snippets come from [`examples/claudeextensions`](../../examples/claudeextensions/), which installs a small plugin with one skill and one command into the fixture home.

## Step 1 — List skills

`a.Skills(h, &p)` lists `<home>/skills/*/SKILL.md`, the project's `.claude/skills` and each installed plugin's skills dir. A `Skill` has `Name`, `Description`, `WhenToUse`, `ArgumentHint`, `Scope` (`SkillScopeUser`, `SkillScopeProject`, `SkillScopePlugin`, …), `Plugin` (the `PluginID` for plugin skills), `Dir`, `File`, the invocation switches, `AllowedTools`/`DisallowedTools`, `Model`, `Effort`, `Paths`, `Hooks`, `Metadata`, every other key in `Unknown`, and `Err` for a frontmatter that did not parse.

<!-- snippet: examples/claudeextensions/main.go#skills -->
```go
// showSkills lists user, project and plugin skills from their frontmatter.
func showSkills(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("skills")
	skills, err := a.Skills(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, s := range skills {
		p.line(fmt.Sprintf("%-10s scope=%-7s plugin=%q model=%q tools=%v desc=%q", s.Name, s.Scope, s.Plugin, s.Model, s.AllowedTools, s.Description))
	}
	return nil
}
```

## Step 2 — List slash commands

`a.SlashCommands(h, &p)` lists `commands/*.md` in the user, project and plugin dirs. `SlashCommand` embeds `Skill` (same fields; it is named so it never collides with `agentio.Command`).

<!-- snippet: examples/claudeextensions/main.go#commands -->
```go
// showSlashCommands lists commands/*.md of the user, project and plugins.
func showSlashCommands(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("slash commands")
	cmds, err := a.SlashCommands(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, c := range cmds {
		p.line(fmt.Sprintf("/%s scope=%s hint=%q desc=%q", c.Name, c.Scope, c.ArgumentHint, c.Description))
	}
	return nil
}
```

## Step 3 — List workflows

`a.Workflows(h, &p)` lists `workflows/*.js` in the user and project dirs; each becomes `/<name>`. A `Workflow` is `Name`, `File` and `Scope`.

<!-- snippet: examples/claudeextensions/main.go#workflows -->
```go
// showWorkflows lists workflows/*.js, each of which becomes /<name>.
func showWorkflows(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("workflows")
	flows, err := a.Workflows(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, f := range flows {
		p.line(fmt.Sprintf("/%s scope=%s file=%s", f.Name, f.Scope, f.File))
	}
	return nil
}
```

## Step 4 — Parse frontmatter

`claude.ParseFrontmatter(b)` splits a SKILL.md, command, subagent, output style or rule file into a `frontmatter.Doc` and the body. The `frontmatter` package does the work: `Parse` (split and parse), `Split` (find the `---` block), `ParseYAML` (parse a block without delimiters). A `Doc` keeps `Fields` in file order; `String`, `List`, `Map`, `Bool`, `Get` and `Keys` read it. Each `Value` has a `Kind` (`KindScalar`, `KindList`, `KindMap`).

<!-- snippet: examples/claudeextensions/main.go#frontmatter -->
```go
// showFrontmatter parses a SKILL.md the way the listers do, then reads the
// same bytes with the frontmatter package directly.
func showFrontmatter(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("frontmatter")
	data, err := os.ReadFile(filepath.Join(a.Paths(a.DefaultHome()).Skills, "demo-skill", "SKILL.md"))
	if err != nil {
		return err
	}
	doc, body, err := claude.ParseFrontmatter(data)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("keys=%v", doc.Keys()))
	p.line(fmt.Sprintf("description=%q", doc.String("description")))
	p.line(fmt.Sprintf("allowed-tools=%v owner=%s", doc.List("allowed-tools"), doc.Map("metadata").String("owner")))
	if v, ok := doc.Bool("user-invocable"); ok {
		p.line(fmt.Sprintf("user-invocable=%t", v))
	}
	p.line(fmt.Sprintf("body starts %q", strings.SplitN(strings.TrimSpace(string(body)), "\n", 2)[0]))

	front, _, ok := frontmatter.Split([]byte("---\ntools: Read, Grep\n---\nbody\n"))
	if !ok {
		return errors.New("no frontmatter block")
	}
	d, err := frontmatter.ParseYAML(front)
	if err != nil {
		return err
	}
	tools, _ := d.Get("tools")
	p.line(fmt.Sprintf("Split+ParseYAML: tools kind=%s text=%q List=%v", tools.Kind, tools.Text, d.List("tools")))
	whole, _, err := frontmatter.Parse([]byte("no frontmatter here\n"))
	p.line(fmt.Sprintf("Parse without a block: fields=%d err=%v", len(whole.Fields), err))
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudeextensions
```

The sections this page covers:

```text
== skills
demo-skill scope=user    plugin="" model="opus" tools=[Read Grep] desc="A fixture skill folded over two lines."
proj-skill scope=project plugin="" model="" tools=[] desc="Project scoped fixture skill"
plug-skill scope=plugin  plugin="demo-plugin@claude-plugins-official" model="" tools=[] desc="Skill shipped by a plugin"

== slash commands
/hello scope=user hint="<name>" desc="Say hello"
/deploy scope=plugin hint="" desc="Deploy the app"

== workflows
/release scope=user file=~/.claude/workflows/release.js
/triage scope=project file=~/project/.claude/workflows/triage.js

== frontmatter
keys=[name description argument-hint allowed-tools user-invocable model metadata license]
description="A fixture skill folded over two lines."
allowed-tools=[Read Grep] owner=fixture
user-invocable=true
body starts "# Body is never read by the lister"
Split+ParseYAML: tools kind=scalar text="Read, Grep" List=[Read Grep]
Parse without a block: fields=0 err=<nil>
```

## Pitfalls

- **A broken frontmatter does not hide the file.** The entry is still listed with `Err` set; check it before trusting the fields.
- **The parser is a YAML subset**, chosen so the root module stays stdlib-only: maps, scalars (plain, quoted, folded and literal blocks), block and flow lists. Anchors, tags, flow maps and multi-document files are kept as literal text, never an error.
- `Doc.List` splits a comma-separated scalar (`tools: Read, Grep`) into items, because Claude Code accepts both spellings; `Get` returns the raw scalar.
- All scalars are text. `Doc.Bool` parses `true`/`false`; numbers you must parse yourself.
- A repeated key keeps its last value on `Get`, as the vendor CLIs do; strict YAML would reject the file.
- No frontmatter is not an error: `Parse` returns an empty `Doc` and the whole input as body.
- **Plugin skills and commands are listed for every installed plugin, enabled or not** (the example's plugin is disabled, yet `plug-skill` and `/deploy` appear). Filter on `Plugin` against `InstalledPlugin.Enabled` if you want what a session will offer (see [Claude plugins](plugins.md)). A plugin whose install path is missing on disk contributes nothing.
- Built-in (bundled) skills and commands are not on disk and are not listed. A running session reports its full list in the stream `init` and `commands_changed` lines (see [Claude stream](stream.md)).

## Related

- [Claude plugins](plugins.md), [Claude subagents](subagents.md), [Claude output styles](output-styles.md), [Claude memory files](memory.md)
- Neutral view: [Config: skills and plugins](../capabilities/config.md)
- Example: [`examples/claudeextensions`](../../examples/claudeextensions/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
