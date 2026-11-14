# Claude paths

`a.Paths(h)`, `a.ProjectPaths(h, p)` and `a.SystemPaths()` compute where Claude Code keeps every file, so your code never hard-codes a name like `.claude.json` or `projects/`. Use them when you need a file the library has no reader for, or to show a user where something lives.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent and a home (see [The Claude agent](agent.md)).
- All three calls are Pure: they compute paths and touch nothing, so a path may not exist.

The snippet comes from [`examples/claudesettings`](../../examples/claudesettings/).

## Step 1 — Get home paths

`Paths` has one field per file or dir of a home: `GlobalConfig`, `Settings`, `Credentials`, `Memory`, `Rules`, `Skills`, `Commands`, `Agents`, `OutputStyles`, `Workflows`, `Projects`, `History`, `Sessions`, `Jobs`, `Teams`, `Tasks`, `FileHistory`, `ShellSnapshots`, `Debug`, `Backups`, `Cache`, `IDE`, `StatsCache`, `PolicyLimits`, `RemoteSettings`, the daemon files, and `Plugins` (a `PluginPaths` with `Installed`, `KnownMarketplaces`, `Blocklist`, `Cache`, `Marketplaces`, `Data`, …). Paths are computed from the home dir alone, so they work for a home that does not exist yet (a new `CLAUDE_CONFIG_DIR` you are about to create):

```go
// homePaths prints where a few files of one home live. Pure: nothing is
// read, so a path may not exist yet.
func homePaths(w io.Writer, a *claude.Agent, dir string) {
	p := a.Paths(a.HomeAt(dir))
	fmt.Fprintln(w, "global config:", p.GlobalConfig)
	fmt.Fprintln(w, "settings:", p.Settings)
	fmt.Fprintln(w, "credentials:", p.Credentials)
	fmt.Fprintln(w, "prompt history:", p.History)
	fmt.Fprintln(w, "installed plugins:", p.Plugins.Installed)
}
```

For a second home at `~/.claude-work` this prints `~/.claude-work/.claude.json` as the global config: for any home other than the default, `.claude.json` lives inside the home dir.

## Step 2 — Get project paths

`ProjectPaths` covers one project folder for one home: the four memory files Claude Code looks for (`CLAUDE.md`, `.claude/CLAUDE.md`, `CLAUDE.local.md`, `AGENTS.md`), `Settings`, `LocalSettings` (at the repo root, or `Dir` outside git), `MCP` (`.mcp.json`), the project `.claude/` subdirs, `HistoryDir` and `AutoMemory`. `SystemPaths` returns the managed (admin) locations for `Host.GOOS` under `Host.SystemRoot`: `ManagedSettings`, `ManagedDropIn`, `ManagedMCP`, `ManagedMemory` (`/etc/claude-code` on linux, `/Library/Application Support/ClaudeCode` on macOS, `C:\Program Files\ClaudeCode` on Windows). `HostFromOS` fills both fields; a fixture host sets neither, so every field is `""` and the managed readers skip them. `a.Project(dir)` is the one call here that reads disk: it resolves the folder and walks up to the git root, which decides where `LocalSettings` lives.

```go
// projectPaths prints the Claude files of one project folder for the
// default home, then the managed (admin) paths of this OS.
func projectPaths(w io.Writer, a *claude.Agent, dir string) error {
	proj, err := a.Project(dir) // Disk read: resolves dir and walks up for .git
	if err != nil {
		return err
	}
	pp := a.ProjectPaths(a.DefaultHome(), proj) // Pure
	for _, m := range pp.Memory {
		fmt.Fprintln(w, "memory file:", m)
	}
	fmt.Fprintln(w, "settings:", pp.Settings)
	fmt.Fprintln(w, "local settings:", pp.LocalSettings)
	fmt.Fprintln(w, "mcp:", pp.MCP)
	fmt.Fprintln(w, "history dir:", pp.HistoryDir)
	fmt.Fprintln(w, "managed settings:", a.SystemPaths().ManagedSettings) // Pure
	return nil
}
```


## Step 3 — Encode a history folder

Claude Code stores a folder's conversations under `<home>/projects/<EncodePath(dir)>`: every byte that is not an ASCII letter or digit becomes `-`. `a.HistoryDir(h, dir)` joins that for you. The example shows all three steps in one function:

<!-- snippet: examples/claudesettings/main.go#paths -->
```go
// showPaths computes home, project and system paths and the history folder
// a directory maps to. Every call here is Pure.
func showPaths(p printer, host agentio.Host) error {
	p.section("paths")
	a := claude.New(host)
	h := a.DefaultHome()
	paths := a.Paths(h)
	p.line(fmt.Sprintf("global config=%s settings=%s", paths.GlobalConfig, paths.Settings))
	p.line(fmt.Sprintf("projects=%s sessions=%s", paths.Projects, paths.Sessions))
	p.line(fmt.Sprintf("plugins installed=%s", paths.Plugins.Installed))

	proj, err := a.Project(filepath.Join(host.UserHome, fixtureProject))
	if err != nil {
		return err
	}
	pp := a.ProjectPaths(h, proj)
	p.line(fmt.Sprintf("project settings=%s local=%s mcp=%s", pp.Settings, pp.LocalSettings, pp.MCP))
	p.line(fmt.Sprintf("project memory files=%v", pp.Memory))

	// Managed (admin) paths come from Host.GOOS and Host.SystemRoot. This
	// fixture host sets neither, so it has no managed locations and the
	// machine's /etc never changes the output; HostFromOS fills both.
	p.line(fmt.Sprintf("managed settings (fixture host)=%q", a.SystemPaths().ManagedSettings))
	real := host
	real.GOOS, real.SystemRoot = linuxGOOS, unixSystemRoot
	p.line(fmt.Sprintf("managed settings (linux host)=%s", claude.New(real).SystemPaths().ManagedSettings))

	p.line(fmt.Sprintf("EncodePath(%q) = %s", historyFolder, claude.EncodePath(historyFolder)))
	p.line(fmt.Sprintf("HistoryDir = %s", a.HistoryDir(h, historyFolder)))
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudesettings
```

The section this page covers:

```text
== paths
global config=~/.claude.json settings=~/.claude/settings.json
projects=~/.claude/projects sessions=~/.claude/sessions
plugins installed=~/.claude/plugins/installed_plugins.json
project settings=~/project/.claude/settings.json local=~/project/.claude/settings.local.json mcp=~/project/.mcp.json
project memory files=[~/project/CLAUDE.md ~/project/.claude/CLAUDE.md ~/project/CLAUDE.local.md ~/project/AGENTS.md]
managed settings (fixture host)=""
managed settings (linux host)=/etc/claude-code/managed-settings.json
EncodePath("/work/proj12") = -work-proj12
HistoryDir = ~/.claude/projects/-work-proj12
```

## Pitfalls

- **`GlobalConfig` moves with the home.** For the default home it is `~/.claude.json` (beside the dir, not inside it); for any other home it is `<home>/.claude.json`.
- `EncodePath` is lossy: `/work/a_b` and `/work/a-b` map to the same folder name. Never decode a folder name back to a path; the cwd recorded inside old transcripts can also be stale after a move. Look up by the path you have.
- `SystemPaths` follows the Host, not the running program: it uses `Host.GOOS` and `Host.SystemRoot`, which only `HostFromOS` fills. A Host you build yourself has no managed locations until you set both (for example `GOOS: "linux", SystemRoot: "/"`, or a temp dir in a test with `agenttest.WithSystem`).
- `Credentials` is often absent on macOS, where the login lives in the keychain; read logins with the credential API, not this path (see [Claude credentials](credential.md)).
- `LocalSettings` sits at the git root, not at `Dir`, when the project is inside a repo.

## Related

- [The Claude agent](agent.md), [Claude settings](settings.md), [Claude global state](global-config.md), [Claude writer](writer.md) (moving history)
- Neutral view: [Homes](../core/homes.md), [Moves, backups and links](../core/backup-move-link.md)
- Example: [`examples/claudesettings`](../../examples/claudesettings/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
