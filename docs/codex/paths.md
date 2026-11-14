# Codex paths and inventory

[`codex.Paths`](ds:block?id=codex-paths-mgfjercg) names every file and directory Codex uses in one home (config, auth, rollouts, the six SQLite stores, caches, desktop-app state), so your code never hard-codes a file name. [`Inventory`](ds:block?id=codex-agent-inventory-ukxqm5eh) then says which of them exist, and lists top-level entries the library does not know yet.

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` and a `codex.Home` from [The Codex agent](agent.md).
- `Paths` and [`PathsWith`](ds:block?id=codex-agent-pathswith-tqqdjxqc) are pure: they compute names and never check the disk. [`Inventory`](ds:block?id=codex-agent-inventory-ukxqm5eh) stats each path once.

The snippets come from [`examples/codexconfig`](../../examples/codexconfig/).

## Step 1 — Get home paths

[`a.Paths(h)`](ds:block?id=codex-agent-paths-37cc6b4n) assumes the SQLite files and logs live in the home. When the config may move them, read the config and call [`a.PathsWith(h, cfg)`](ds:block?id=codex-agent-pathswith-tqqdjxqc): it honours `sqlite_home` and `log_dir` from `config.toml`, and `CODEX_SQLITE_HOME` from `Host.Env` (which wins over `sqlite_home`). [`Paths.ProfileConfig(name)`](ds:block?id=codex-paths-profileconfig-k6ajnjam) is the v2 profile file `<home>/<name>.config.toml`.

<!-- snippet: examples/codexconfig/config.go#paths -->
```go
// homePaths names the files of a home without touching disk. PathsWith
// also honours sqlite_home, log_dir and CODEX_SQLITE_HOME.
func homePaths(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()
	p := a.Paths(h)
	fmt.Fprintf(w, "config=%s auth=%s\n", m.rel(p.Config), m.rel(p.Auth))
	fmt.Fprintf(w, "sessions=%s state db=%s\n", m.rel(p.Sessions), m.rel(p.StateDB))
	fmt.Fprintf(w, "v2 profile work=%s\n", m.rel(p.ProfileConfig("work")))

	cfg, err := a.ConfigFile(h)
	if err != nil {
		return err
	}
	pw := a.PathsWith(h, cfg)
	fmt.Fprintf(w, "sqlite home with config=%s\n", m.rel(pw.SQLiteHome))
	return nil
}
```

## Step 2 — List the home inventory

`Inventory` returns one [`InventoryEntry`](ds:block?id=codex-agent-inventory-ukxqm5eh) per known path (`Name` is the `Paths` field name, or `Marker` for a home marker, plus `Exists`, `IsDir`, `Size`, `ModTime`) and `Unknown`, the top-level names `Paths` does not cover.

<!-- snippet: examples/codexconfig/config.go#inventory -->
```go
// homeInventory stats every known path once and names unknown entries.
func homeInventory(w io.Writer, m machine) error {
	a := codex.New(m.host)
	inv, err := a.Inventory(a.DefaultHome())
	if err != nil {
		return err
	}
	var present []string
	for _, e := range inv.Entries {
		if e.Exists {
			present = append(present, e.Name)
		}
	}
	fmt.Fprintf(w, "present: %s\n", strings.Join(present, " "))
	fmt.Fprintf(w, "unknown top-level entries: %d\n", len(inv.Unknown))
	return nil
}
```

## Try it

```sh
cd examples && go run ./codexconfig
```

The `== paths` section:

```text
== paths
config=~/.codex/config.toml auth=~/.codex/auth.json
sessions=~/.codex/sessions state db=~/.codex/state_5.sqlite
v2 profile work=~/.codex/work.config.toml
sqlite home with config=~/.codex
present: Config AgentsMD Rules Hooks Auth InstallationID Sessions SessionIndex History ModelsCache VersionCache Skills Plugins ThreadWriterLocks
unknown top-level entries: 0
```

## Pitfalls

- **Use `PathsWith` before opening SQLite.** `Paths` ignores `sqlite_home` and [`CODEX_SQLITE_HOME`](ds:block?id=codex-agent-paths-37cc6b4n); [`state.Open`](state.md#step-1--open-the-state-database) takes whatever `Paths` you give it, so pass `PathsWith`.
- **Paths are not existence checks.** A path in `Paths` may not exist (a fresh home has no `state_5.sqlite`); check [`Inventory`](ds:block?id=codex-paths-mgfjercg) or handle `fs.ErrNotExist`.
- **Desktop-app paths are listed only.** [`DesktopState`](ds:block?id=codex-paths-mgfjercg), `DesktopDB`, `IPCSocket`, `ChatProcesses`, `ComputerUse`, `Browser` and `Visualizations` are named so callers can find them; apart from `ChatProcesses` and [`DaemonSocket`](ds:block?id=codex-agent-daemonsocket-5k6q3q87) ([sessions](sessions.md#step-3--list-processes-and-chat-processes)) nothing decodes them.
- **[`Inventory`](ds:block?id=codex-agent-inventory-ukxqm5eh) does not walk directories.** `Size` is 0 for directories; unknown entries are reported by name only, never read.

## Related

- [Codex config.toml](config.md), [Codex SQLite state](state.md)
- [Homes](../core/homes.md), [Backup, move and link](../core/backup-move-link.md)
- [Claude paths](../claude/paths.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
