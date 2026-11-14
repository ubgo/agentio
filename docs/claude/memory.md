# Claude memory files

Claude Code loads instruction ("memory") files into every session: the managed `CLAUDE.md`, the user's `CLAUDE.md` and rules, the project's `CLAUDE.md`, `.claude/CLAUDE.md`, rules and `AGENTS.md` from the filesystem root down to the project, and `CLAUDE.local.md`. It also keeps auto memory, notes it writes itself per project. `a.MemoryFiles` lists them in load order with their `@imports`. Use it to show exactly which instructions a session starts with.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent, a home and a `claude.Project` (see [The Claude agent](agent.md)).
- All calls here are Disk reads except `ParseImports`, which is Pure.

The snippets come from [`examples/claudeextensions`](../../examples/claudeextensions/), which adds two auto-memory notes to the fixture home.

## Step 1 — List memory files

`MemoryFiles(h, p, follow)` returns `[]MemoryFile` in load order. Each has `Kind` (`MemoryManaged`, `MemoryUser`, `MemoryUserRule`, `MemoryProject`, `MemoryProjectRule`, `MemoryAgentsMD`, `MemoryLocal`, `MemoryImport`, …), `Path`, `RealPath` (symlinks resolved), `PathsGlob` (a rule's frontmatter `paths`), `Imports` and `Size`.

<!-- snippet: examples/claudeextensions/main.go#memory -->
```go
// showMemory lists the instruction files Claude Code loads for the project,
// in load order, with the @imports each one names.
func showMemory(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("memory files")
	files, err := a.MemoryFiles(a.DefaultHome(), proj, false) // false: list imports, do not expand them
	if err != nil {
		return err
	}
	for _, f := range files {
		p.line(fmt.Sprintf("%-12s %s size=%d paths=%v imports=%d", f.Kind, f.Path, f.Size, f.PathsGlob, len(f.Imports)))
	}
	return nil
}
```

## Step 2 — Read auto memory

`AutoMemory(h, p)` lists `<home>/projects/<EncodePath(dir)>/memory/*.md` with `Kind` `MemoryAuto`; `MEMORY.md` is the index Claude Code reads first.

<!-- snippet: examples/claudeextensions/main.go#automemory -->
```go
// showAutoMemory lists the notes Claude Code saved for this project itself.
func showAutoMemory(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("auto memory")
	notes, err := a.AutoMemory(a.DefaultHome(), proj)
	if err != nil {
		return err
	}
	// The folder is <home>/projects/<EncodePath(project dir)>/memory; the
	// encoded name holds the temp dir, so it is shown symbolically.
	dir := a.ProjectPaths(a.DefaultHome(), proj).AutoMemory
	p.line(fmt.Sprintf("dir=%s", strings.Replace(dir, claude.EncodePath(proj.Dir), "<EncodePath(project dir)>", 1)))
	for _, n := range notes {
		p.line(fmt.Sprintf("%s %s size=%d", n.Kind, filepath.Base(n.Path), n.Size))
	}
	return nil
}
```

## Step 3 — Follow imports

A memory file can pull in another with a line starting `@path`. `claude.ParseImports(body)` returns those lines as `Import` values (`Raw`, `Line`), skipping fenced code; it leaves `Resolved` empty. `MemoryFiles` fills `Resolved` and `Exists` relative to the importing file (or the user home for `~/`) and marks `External` imports outside the project and the home. With `follow` true it also lists the imported files (as `MemoryImport`, depth-capped at 5), skipping files already listed.

<!-- snippet: examples/claudeextensions/main.go#imports -->
```go
// showImports parses @path lines from a memory body and shows how the
// listed user CLAUDE.md resolves its own imports.
func showImports(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("imports")
	body := "See @docs/build.md\n```\n@not-an-import.md\n```\n  @~/notes.md\nmail me at user@example.com\n"
	for _, imp := range claude.ParseImports([]byte(body)) {
		p.line(fmt.Sprintf("line %d raw=%s", imp.Line, imp.Raw))
	}
	files, err := a.MemoryFiles(a.DefaultHome(), proj, true) // true: follow imports, depth-capped
	if err != nil {
		return err
	}
	for _, f := range files {
		for _, imp := range f.Imports {
			p.line(fmt.Sprintf("%s imports %s -> %s exists=%t", filepath.Base(f.Path), imp.Raw, imp.Resolved, imp.Exists))
		}
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudeextensions
```

The sections this page covers:

```text
== memory files
user         ~/.claude/CLAUDE.md size=97 paths=[] imports=2
user-rule    ~/.claude/rules/style.md size=44 paths=[src/**/*.go] imports=0
project      ~/project/CLAUDE.md size=25 paths=[] imports=0

== auto memory
dir=~/.claude/projects/<EncodePath(project dir)>/memory
auto MEMORY.md size=40
auto build.md size=16

== imports
line 5 raw=~/notes.md
CLAUDE.md imports rules/style.md -> ~/.claude/rules/style.md exists=true
CLAUDE.md imports ~/missing-file.md -> ~/missing-file.md exists=false
```

In the sample body only `@~/notes.md` counts: `See @docs/build.md` does not start the line, the fenced `@not-an-import.md` is code, and an email address is not an import.

## Pitfalls

- **An import must start the line** (leading spaces allowed). An `@path` in the middle of a sentence is not an import, for Claude Code or for `ParseImports`.
- A missing import is listed with `Exists` false, not an error; report it, since Claude Code silently loads nothing for it.
- With `follow` true, a file already listed (for example a rule that `CLAUDE.md` also imports) is not repeated, so the followed list can be the same length as the plain one.
- Rules with a `paths` glob load only when Claude works on matching files; `MemoryFiles` lists them regardless and gives you `PathsGlob` to decide.
- The managed `CLAUDE.md` path comes from the OS (see [Claude paths](paths.md)), not from the Host.
- Auto memory lives under the history folder, so it follows the folder on a history move (see [Claude writer](writer.md)).

## Related

- [Claude skills, slash commands and workflows](skills.md), [Claude subagents](subagents.md), [Claude paths](paths.md)
- Neutral view: [Config: instruction files](../capabilities/config.md)
- Example: [`examples/claudeextensions`](../../examples/claudeextensions/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
