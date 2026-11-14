# Claude local data

Besides settings and conversations, a Claude home holds working data: pre-edit file checkpoints, shell snapshots, IDE lock files, debug logs, cached files (changelog, model catalog) and a cleanup marker. The `claude` module reads each of these and sizes the home's top-level entries. Use it for a disk-usage view, a cleanup tool, or to find the IDE a session is attached to.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent and a home (see [The Claude agent](agent.md)).
- Every call is a Disk read. `DataUsage` walks the whole home and can be slow on a real one.

The snippets come from [`examples/claudeextensions`](../../examples/claudeextensions/), which writes one file of each kind into the temp fixture home.

## Step 1 — Read file history

Before Claude Code edits a file it saves a copy under `file-history/<session>/<hash>@v<n>`. `a.FileHistory(h, session)` lists them as `Checkpoint` values (`Session`, `FileHash`, `Version`, `Path`, `Size`).

<!-- snippet: examples/claudeextensions/main.go#filehistory -->
```go
// showFileHistory lists the pre-edit copies one session saved.
func showFileHistory(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("file history")
	cps, err := a.FileHistory(a.DefaultHome(), checkpointSess)
	if err != nil {
		return err
	}
	for _, c := range cps {
		p.line(fmt.Sprintf("session=%s file hash=%s version=%d size=%d", c.Session, c.FileHash, c.Version, c.Size))
	}
	return nil
}
```

## Step 2 — Read shell snapshots and cleanup

`a.ShellSnapshots(h)` lists `shell-snapshots/snapshot-<shell>-<ms>-<rand>.sh` oldest first, as `ShellSnapshot` (`Shell`, `Created` from the name, `Path`). `a.LastCleanup(h)` reads `.last-cleanup`, the time Claude Code last pruned old data; nil when absent.

<!-- snippet: examples/claudeextensions/main.go#snapshots -->
```go
// showSnapshots lists shell snapshots and when cleanup last ran.
func showSnapshots(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("shell snapshots and cleanup")
	h := a.DefaultHome()
	snaps, err := a.ShellSnapshots(h) // oldest first
	if err != nil {
		return err
	}
	for _, s := range snaps {
		p.line(fmt.Sprintf("shell=%s created=%s", s.Shell, s.Created.UTC().Format(time.RFC3339)))
	}
	last, err := a.LastCleanup(h) // nil when .last-cleanup is absent
	if err != nil {
		return err
	}
	if last != nil {
		p.line(fmt.Sprintf("last cleanup %s", last.UTC().Format(time.RFC3339)))
	}
	return nil
}
```

## Step 3 — Read IDE locks and debug logs

An IDE extension writes `ide/<port>.lock`; `a.IDELocks(h)` returns `IDELock` (`Port`, `PID`, `Workspace`, `IDEName`, `Transport`, the body in `Raw`, other keys in `Unknown`). `a.DebugLogs(h)` lists `debug/<session>.txt` as `DebugLog` (`Session`, `Path`, `Size`, `Modified`, and `Latest` for the file `debug/latest` points to).

<!-- snippet: examples/claudeextensions/main.go#ide -->
```go
// showIDEAndDebug reads IDE lock files and lists debug logs.
func showIDEAndDebug(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("IDE locks and debug logs")
	h := a.DefaultHome()
	locks, err := a.IDELocks(h)
	if err != nil {
		return err
	}
	for _, l := range locks {
		p.line(fmt.Sprintf("ide=%s port=%d pid=%d workspace=%v transport=%s unknown=%v", l.IDEName, l.Port, l.PID, l.Workspace, l.Transport, l.Unknown.Keys()))
	}
	logs, err := a.DebugLogs(h)
	if err != nil {
		return err
	}
	for _, l := range logs {
		p.line(fmt.Sprintf("debug session=%s size=%d latest=%t", l.Session, l.Size, l.Latest))
	}
	return nil
}
```

## Step 4 — Read data usage and the changelog

`a.DataUsage(h)` returns one `DataEntry` per top-level entry of the home (`Name`, `Bytes`, and `Foreign` for entries Claude Code neither documents nor writes, which are never parsed). `a.ModelCatalogFiles(h)` lists `cache/model-catalog/` file paths (names only; the shape is not recorded). `a.Changelog(h)` reads the cached `cache/changelog.md`.

<!-- snippet: examples/claudeextensions/main.go#datausage -->
```go
// showDataUsage sizes every top-level entry of the home (this walks the
// whole tree: slow on a real home), plus the cached catalog and changelog.
func showDataUsage(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("data usage")
	h := a.DefaultHome()
	entries, err := a.DataUsage(h)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
		if e.Foreign { // Claude Code neither documents nor creates it; never parsed
			p.line(fmt.Sprintf("foreign entry %s (%d bytes)", e.Name, e.Bytes))
		}
	}
	p.line(fmt.Sprintf("entries=%v", names))
	catalog, err := a.ModelCatalogFiles(h)
	if err != nil {
		return err
	}
	for _, f := range catalog {
		p.line(fmt.Sprintf("model catalog file %s", f))
	}
	changelog, err := a.Changelog(h)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("changelog first line %q", strings.SplitN(changelog, "\n", 2)[0]))
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudeextensions
```

The sections this page covers:

```text
== file history
session=s1 file hash=abc123 version=2 size=18

== shell snapshots and cleanup
shell=zsh created=2026-11-06T21:20:00Z
last cleanup 2026-11-13T19:30:55Z

== IDE locks and debug logs
ide=VS Code port=41234 pid=5 workspace=[/work/project] transport=ws unknown=[authToken]
debug session=s1 size=10 latest=true

== data usage
foreign entry notes-by-another-tool.txt (21 bytes)
entries=[.credentials.json .last-cleanup .last-update-result.json CLAUDE.md agents cache commands debug file-history history.jsonl ide mcp-needs-auth-cache.json notes-by-another-tool.txt output-styles plugins policy-limits.json projects remote-settings.json remote-settings.json.synced-at rules sessions settings.json shell-snapshots skills stats-cache.json workflows]
model catalog file ~/.claude/cache/model-catalog/models.json
changelog first line "# Changelog"
```

## Pitfalls

- **`DataUsage` walks everything.** `projects/` alone can be gigabytes on a busy home; run it off the UI thread and do not call it in a loop. It does not follow symlinks.
- **IDE locks carry an auth token.** It stays in `IDELock.Unknown` and the raw body; never print `Raw` or the unknown values.
- `IDELock` fields are from the docs; no lock file was recorded, so expect fields in `Unknown` first.
- `ShellSnapshot.Created` comes from the millisecond stamp in the file name, not the file's modification time.
- The listers return empty results, not errors, when a dir is absent (a fresh home has none of these dirs).
- Reading is all the library does here. Deleting checkpoints or logs is up to you, and Claude Code also prunes them on its own schedule (`cleanupPeriodDays` in settings).

## Related

- [Claude paths](paths.md), [Claude conversations](conversations.md), [Claude stats](stats.md), [Claude install and updates](install.md)
- Example: [`examples/claudeextensions`](../../examples/claudeextensions/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
