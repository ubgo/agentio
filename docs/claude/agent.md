# The Claude agent

`claude.New` gives you Claude Code on one `agentio.Host`: the typed entry point for everything the `claude` module reads (install, homes, settings, hooks, MCP servers, conversations, usage). Use it when your code knows it is talking to Claude Code and wants the rich vendor types; use `Neutral()` when the same code also loops over other agents.

## Before you start

- Add the module: `go get github.com/ubgo/agentio/claude` (it pulls only the stdlib-only root module).
- Build a `Host`. Real programs call `agentio.HostFromOS()`; tests and these examples build one by hand that points at a fixture home (see [Host](../core/host.md)).
- A Claude home is a config dir holding at least one marker (`settings.json`, `projects`, `.credentials.json`, `.claude.json`). The default home is `~/.claude`; others are `~/.claude-<name>` or any dir named by `CLAUDE_CONFIG_DIR`.

The snippets come from [`examples/claudesettings`](../../examples/claudesettings/), which runs on a temp copy of the 2.1.325 fixture home.

## Step 1 — Create the agent

`claude.New` never fails and does no I/O: the agent's `Spec` is a constant. `claude.ID`, `claude.Binary` (`claude`) and `claude.ConfigDirEnv` (`CLAUDE_CONFIG_DIR`) are the identity constants you need when you build commands or environments yourself.

<!-- snippet: examples/claudesettings/main.go#agent -->
```go
// showAgent creates the agent and prints its identity constants.
func showAgent(p printer, host agentio.Host) error {
	p.section("agent")
	a := claude.New(host) // Pure: no I/O, never fails
	p.line(fmt.Sprintf("id=%s name=%q binary=%s home env=%s", a.ID(), a.Name(), claude.Binary, claude.ConfigDirEnv))
	p.line(fmt.Sprintf("spec binary=%s home markers=%v", a.Spec().Binary, a.Spec().HomeMarkers))
	return nil
}
```

## Step 2 — Pick a home

Every reader takes a `claude.Home`, never a bare path, so a Codex home cannot be passed by mistake. The ways to get one:

| Call | Returns | Cost |
|---|---|---|
| `a.DefaultHome()` | `~/.claude` (used with `CLAUDE_CONFIG_DIR` absent) | Pure |
| `a.HomeAt(dir)` | the home at any dir; relative dirs resolve against the user home | Pure |
| `a.HomeFromEnv()` | the home `CLAUDE_CONFIG_DIR` in `Host.Env` selects; `false` when unset or empty | Pure |
| `a.Homes()` | `~/.claude` first, then every `~/.claude-<x>` holding a marker (sorted), then the env home if not listed | Disk read |
| `a.HomeOf(n)` | converts a neutral `agentio.Home`; `ErrForeignHome` for another agent's | Pure |
| `a.IsHome(dir)`, `a.HomeMarkersIn(dir)` | whether `dir` holds a marker, and which | Disk read |
| `claude.HomeMarkers()` | the marker names | Pure |

`Home.Name()` suggests a label (`personal` for the default, `work` for `~/.claude-work`), `Home.IsDefault()` tells the default apart, and `Home.String()` prints `claude:<dir>`.

<!-- snippet: examples/claudesettings/main.go#homes -->
```go
// showHomes lists the homes on the machine and picks one by dir.
func showHomes(p printer, host agentio.Host) error {
	a := claude.New(host)
	homes, err := a.Homes() // ~/.claude first, then every ~/.claude-<x> with a marker
	if err != nil {
		return err
	}
	for _, h := range homes {
		p.line(fmt.Sprintf("home %s name=%s default=%t", h.Dir(), h.Name(), h.IsDefault()))
	}
	work := a.HomeAt(filepath.Join(host.UserHome, ".claude-work"))
	isHome, err := a.IsHome(work.Dir())
	if err != nil {
		return err
	}
	markers, err := a.HomeMarkersIn(work.Dir())
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("HomeAt %s is a home=%t markers=%v (of %v)", work, isHome, markers, claude.HomeMarkers()))
	if _, ok := a.HomeFromEnv(); !ok {
		p.line(fmt.Sprintf("HomeFromEnv: %s is absent, so the default home is in use", claude.ConfigDirEnv))
	}
	if _, err := a.HomeOf(agentio.NewHome("codex", host.UserHome, true, "x")); errors.Is(err, agentio.ErrForeignHome) {
		p.line(fmt.Sprintf("HomeOf refuses a Codex home: %v", err))
	}
	return nil
}
```

## Step 3 — Open a project

`a.Project(dir)` makes `dir` absolute and walks up for `.git`. `Project.Dir` is where Claude runs; `Project.RepoRoot` is the git root (`""` outside git), which is where `.claude/settings.local.json` lives. Project-aware readers (`Settings`, `Hooks`, `MCPServers`, `Skills`, …) take `*Project`; pass `nil` for home-only answers.

<!-- snippet: examples/claudesettings/main.go#project -->
```go
// showProject opens the fixture project: its dir and git root.
func showProject(p printer, host agentio.Host) error {
	a := claude.New(host)
	proj, err := a.Project(filepath.Join(host.UserHome, fixtureProject))
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("project dir=%s repo root=%q", proj.Dir, proj.RepoRoot))
	return nil
}
```

## Step 4 — Get the neutral view

`*claude.Agent` is not an `agentio.Agent` itself, so vendor methods can keep rich return types (`a.Usage` returns `claude.Usage`). `a.Neutral()` returns the `agentio.Agent` view for code that handles several agents through the typed accessors (`agentio.UsageOf`, `agentio.ConfigOf`, …). The rich types have their own `Neutral()`: `Home.Neutral()`, `Install.Neutral()`, `Identity.Neutral()`, `Usage.Neutral()`, `Conversation.Neutral()`, `LiveSession.Neutral(h)`.

<!-- snippet: examples/claudesettings/main.go#neutral -->
```go
// showNeutral gets the agentio.Agent view and a neutral home handle.
func showNeutral(p printer, host agentio.Host) error {
	a := claude.New(host)
	n := a.Neutral() // the agentio.Agent view for code that loops over agents
	home := a.DefaultHome().Neutral()
	p.line(fmt.Sprintf("neutral id=%s default home=%s agent of home=%s", n.ID(), n.DefaultHome(), home.Agent()))
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudesettings
```

The section this page covers (paths under the temp user home print as `~`):

```text
== agent
id=claude name="Claude Code" binary=claude home env=CLAUDE_CONFIG_DIR
spec binary=claude home markers=[settings.json projects .credentials.json .claude.json]
home ~/.claude name=personal default=true
home ~/.claude-work name=work default=false
HomeAt claude:~/.claude-work is a home=true markers=[settings.json projects .claude.json] (of [settings.json projects .credentials.json .claude.json])
HomeFromEnv: CLAUDE_CONFIG_DIR is absent, so the default home is in use
HomeOf refuses a Codex home: agentio: home belongs to another agent: codex home passed to claude
project dir=~/project repo root=""
neutral id=claude default home=claude:~/.claude agent of home=claude
```

## Pitfalls

- **The default home means `CLAUDE_CONFIG_DIR` is absent**, not set to `~/.claude`. Claude Code keys its keychain entry on the exact variable value, so setting it to the default path behaves like a different home. Launch builders remove the variable for the default home; do the same if you build an environment yourself.
- `Homes()` skips look-alikes such as `~/.claude-worktrees` (git worktrees, no marker). A `~/.claude-<x>` dir you created but never ran Claude Code in is skipped too until it holds a marker.
- `HomeAt` does not check the dir exists or is a home; call `IsHome` when that matters.
- `Project` walks up to the filesystem root. A project folder inside another repo picks up that repo as `RepoRoot`, which moves where `settings.local.json` is read from.
- Find the binary with `a.LookPath()` (a PATH walk through `Host.Env`), never a shell: `claude` is often also a shell function or alias.

## Related

- [Claude paths](paths.md), [Claude settings](settings.md), [Claude install and updates](install.md)
- Neutral view: [Typed accessors](../capabilities/accessors.md), [Homes](../core/homes.md), [Agent, Spec and Core](../core/agent-and-spec.md)
- Example: [`examples/claudesettings`](../../examples/claudesettings/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
