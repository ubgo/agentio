# Claude output styles

An output style changes how Claude Code writes its answers (a system-prompt addition picked with `/output-style` or the `outputStyle` setting). Custom styles are Markdown files in `output-styles/` of the home, the project or a plugin. `a.OutputStyles(h, &p)` lists them from their frontmatter. Use it to offer a style picker or to check that the style a setting names exists.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent, a home and optionally a project (see [The Claude agent](agent.md)).
- Disk read, frontmatter only.

The snippet comes from [`examples/claudeextensions`](../../examples/claudeextensions/), which adds one user style and one project style to the fixture home.

## Step 1 — List output styles

Each `OutputStyle` has `Name` (the frontmatter `name`, else the file name), `Description`, `File`, `Scope` (user, project, plugin), `KeepCodingInstructions` and `ForceForPlugin` (pointers: nil when the key is absent), `Builtin` and `Err` (a frontmatter parse error; the style is still listed). The active style name is `Settings.OutputStyle` on the merged settings (see [Claude settings](settings.md)).

<!-- snippet: examples/claudeextensions/main.go#outputstyles -->
```go
// showOutputStyles lists output-styles/*.md of the user, project and plugins.
func showOutputStyles(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("output styles")
	styles, err := a.OutputStyles(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, s := range styles {
		keep := "unset"
		if s.KeepCodingInstructions != nil {
			keep = fmt.Sprint(*s.KeepCodingInstructions)
		}
		p.line(fmt.Sprintf("%-8s scope=%-7s keep coding instructions=%s desc=%q", s.Name, s.Scope, keep, s.Description))
	}
	return nil
}
```

## Step 2 — Check the style a setting names

The active style is a name in the merged settings (`outputStyle` in any settings layer, highest layer wins). Matching that name against the listed styles tells you which file defines it and, through `EffectiveSettings.Origin`, which layer picked it. A name with no matching file is not an error: Claude Code also has styles compiled into the binary.

```go
// activeStyle reports the output style the merged settings name and which
// file defines it. A name with no file is not an error: it can be one of
// the styles built into the binary.
func activeStyle(w io.Writer, a *claude.Agent, proj claude.Project) error {
	h := a.DefaultHome()
	eff, err := a.Settings(h, &proj, nil)
	if err != nil {
		return err
	}
	name := eff.Merged.OutputStyle
	if name == "" {
		fmt.Fprintln(w, "no outputStyle set: Claude Code's default applies")
		return nil
	}
	styles, err := a.OutputStyles(h, &proj)
	if err != nil {
		return err
	}
	for _, s := range styles {
		if s.Name == name {
			fmt.Fprintf(w, "%q is defined in %s (scope %s, set by %s)\n", name, s.File, s.Scope, eff.Origin["outputStyle"])
			return nil
		}
	}
	fmt.Fprintf(w, "%q has no file on disk: built in, or missing\n", name)
	return nil
}
```

On the fixture home no layer sets `outputStyle`, so it prints `no outputStyle set: Claude Code's default applies`. In the example's home, which adds a `terse` user style, setting `"outputStyle": "terse"` in the user `settings.json` (by hand or with `Writer().SetUserSetting`, see [Claude writer](writer.md)) makes it print that file with scope `user`.

## Step 3 — Report broken style files

A style file with bad frontmatter is still listed, with `Err` set and `Name` taken from the file name, so one broken file never hides the others. Show those separately instead of dropping them:

```go
// brokenStyles lists style files whose frontmatter failed to parse. They
// are still returned, with Err set, so a picker can show them greyed out.
func brokenStyles(w io.Writer, a *claude.Agent, proj claude.Project) error {
	styles, err := a.OutputStyles(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, s := range styles {
		if s.Err != nil {
			fmt.Fprintf(w, "%s: %v\n", s.File, s.Err)
		}
	}
	return nil
}
```

The fixture styles all parse, so this prints nothing there.

## Try it

```sh
cd examples && go run ./claudeextensions
```

The section this page covers:

```text
== output styles
terse    scope=user    keep coding instructions=true desc="Short answers"
Teacher  scope=project keep coding instructions=unset desc="Explains every step"
```

## Pitfalls

- **Built-in styles are not listed.** Claude Code ships some styles in the binary; their names were not recorded, so nothing with `Builtin` true comes from disk. A setting naming a built-in style finds no file, which is not an error.
- The name a setting uses is the frontmatter `name` when present (`Teacher` above, from `teach.md`).
- Name matching is exact. `Teacher` and `teacher` are different names to this code; compare the way your UI expects, but report the file's own spelling.
- The project style dir is `<project>/.claude/output-styles/`. Pass `nil` for the project to list user and plugin styles only.
- `KeepCodingInstructions` nil means "not set" (Claude Code's default applies), not false.

## Related

- [Claude settings](settings.md), [Claude skills, slash commands and workflows](skills.md), [Claude plugins](plugins.md), [Claude status line](statusline.md)
- Example: [`examples/claudeextensions`](../../examples/claudeextensions/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
