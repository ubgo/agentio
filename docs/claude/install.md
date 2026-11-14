# Claude install and updates

`a.Install()` tells you where Claude Code is installed, how (native installer, npm, Homebrew cask), which version is on disk and what the last auto-update did, all without running `claude`. Use it for a status screen, a doctor check, or to pick the `Target.Version` a launch is checked against.

## Before you start

- `go get github.com/ubgo/agentio/claude`; a `Host` whose `Env` holds the `PATH` to search (see [The Claude agent](agent.md)).
- The example builds a fake native install in a temp user home: `~/.local/share/claude/versions/2.1.325` (a file) and the `~/.local/bin/claude` symlink the native installer makes, with `PATH=~/.local/bin`.

The snippets come from [`examples/claudesettings`](../../examples/claudesettings/).

## Step 1 — Read the install

`Install` walks `PATH` from `Host.Env` (never a shell), resolves symlinks and derives the method from the real path: a `.../.local/share/claude/versions/<v>` path is `agentio.InstallNative` (version from the file name), a `node_modules/@anthropic-ai/claude-code` path is `claude.InstallNPMGlobal` (version from its `package.json`), a Caskroom path is `claude.InstallHomebrewCask` (no version on disk). It also lists other `claude` entries on `PATH` (`Shadowed`) and copies the default home's update facts: `RawMethod` and `AutoUpdates` from `.claude.json`, `Channel` from `settings.json`, `LastUpdate` from `.last-update-result.json`. `claude.ParseVersion` parses the `claude --version` text (`2.1.325 (Claude Code)`) for callers that ran the binary themselves.

<!-- snippet: examples/claudesettings/main.go#install -->
```go
// showInstall reads the install from disk, the last update and builds the
// install and update commands without running anything.
func showInstall(p printer, host agentio.Host) error {
	p.section("install")
	a := claude.New(host)
	inst, err := a.Install() // ErrNotInstalled when claude is not on PATH
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("path=%s real=%s method=%s version=%s", inst.Path, inst.RealPath, inst.Method, inst.Version))
	p.line(fmt.Sprintf("raw method=%q auto updates=%v protected for native=%t", inst.RawMethod, deref(inst.AutoUpdates), inst.ProtectedForNative))

	last, err := a.LastUpdate(a.DefaultHome()) // nil when the file is absent
	if err != nil {
		return err
	}
	if last != nil {
		p.line(fmt.Sprintf("last update %s: %s -> %s outcome=%s", last.At.Format(time.RFC3339), last.From, last.To, last.Outcome))
	}
	changelog, err := a.Changelog(a.DefaultHome()) // "" when cache/changelog.md is absent
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("cached changelog: %d bytes", len(changelog)))

	v, err := claude.ParseVersion("2.1.325 (Claude Code)")
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("ParseVersion -> %s", v))
	p.line(fmt.Sprintf("update command: %s", a.UpdateCommand()))
	p.line(fmt.Sprintf("install command: %s", a.InstallCommand("stable")))
	return nil
}
```

## Step 2 — Read the last update

`a.LastUpdate(h)` decodes `<home>/.last-update-result.json` into `UpdateResult` (`At`, `From`, `To`, `Outcome`, `Status`, `ErrorCode`, unknown keys kept); it returns `nil` when the file is absent. `a.Changelog(h)` reads the cached `cache/changelog.md` (bounded to 4 MiB) and returns `""` when absent. Both are Disk reads of the default home. A failed update keeps its `ErrorCode`, which is what a status screen wants to show:

```go
// lastUpdate prints what the last auto-update did and how large the cached
// changelog is. Both are Disk reads of the default home; nil and "" mean the
// file is absent, which is not an error.
func lastUpdate(w io.Writer, a *claude.Agent) error {
	h := a.DefaultHome()
	last, err := a.LastUpdate(h)
	if err != nil {
		return err
	}
	if last == nil {
		fmt.Fprintln(w, "no update recorded")
	} else {
		fmt.Fprintf(w, "%s: %s -> %s outcome=%s\n", last.At.Format(time.RFC3339), last.From, last.To, last.Outcome)
		if last.ErrorCode != nil {
			fmt.Fprintf(w, "error code %s\n", *last.ErrorCode)
		}
	}
	changelog, err := a.Changelog(h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "cached changelog: %d bytes\n", len(changelog))
	return nil
}
```

On the fixture home this prints `2026-11-12T22:39:35Z: 2.1.323 -> 2.1.325 outcome=success` and `cached changelog: 0 bytes`.

## Step 3 — Build install and update commands

`a.UpdateCommand()` is `claude update`; `a.InstallCommand(target)` is `claude install <target>` where target is `stable`, `latest` or a version. Both are Pure: they return an `agentio.Command` (path, args, env) that you print or run yourself. Nothing in the library runs them.

```go
// installCommands prints the update and install commands. Both are Pure:
// they build an agentio.Command and run nothing; your program decides
// whether and when to run them.
func installCommands(w io.Writer, a *claude.Agent) {
	fmt.Fprintln(w, a.UpdateCommand().Redacted())
	for _, target := range []string{"stable", "latest", "2.1.325"} {
		fmt.Fprintln(w, a.InstallCommand(target).Redacted())
	}
}
```

This prints `claude update`, `claude install stable`, `claude install latest` and `claude install 2.1.325`. To run one, hand the `Command` to your own `os/exec` code or to a `Runner` you hold on purpose; running `claude install` replaces the binary every session on the machine uses.

## Try it

```sh
cd examples && go run ./claudesettings
```

The section this page covers:

```text
== install
path=~/.local/bin/claude real=~/.local/share/claude/versions/2.1.325 method=native version=2.1.325
raw method="native" auto updates=false protected for native=true
last update 2026-11-12T22:39:35Z: 2.1.323 -> 2.1.325 outcome=success
cached changelog: 0 bytes
ParseVersion -> 2.1.325
update command: claude update
install command: claude install stable
```

## Pitfalls

- `Install` returns `agentio.ErrNotInstalled` when `claude` is not on the Host's `PATH`. With `agentio.HostFromOS()` that is the process `PATH`, which under a launchd job, cron or an IDE is often shorter than your shell's.
- The version is what is on disk, not what is running. A session started before an auto-update keeps running the old binary; live sessions report their own version.
- A Homebrew cask install has no version on disk, so `Version` is the zero value. Use `Runner().BinaryVersion` (runs `claude --version`, a Process cost) if you need it; see [Claude runner](runner.md).
- `claude.ParseVersion` returns `(agentio.Version, error)`, while the root `agentio.ParseVersion` returns `(Version, bool)`. Same name, different result shape.
- `UpdateCommand` is a method on `*claude.Agent`; the Codex module has a package function `codex.UpdateCommand(method)` with a different signature. Do not expect one shape across vendors.
- The update facts (`RawMethod`, `AutoUpdates`, `Channel`, `LastUpdate`) always come from the default home, whichever home you work with.

## Related

- [The Claude agent](agent.md), [Claude paths](paths.md), [Claude doctor](doctor.md), [Claude runner](runner.md)
- Neutral view: [Install and identity](../capabilities/install-identity.md), [Versions](../core/versions.md), [Compatibility](../compat/compat.md)
- Example: [`examples/claudesettings`](../../examples/claudesettings/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
