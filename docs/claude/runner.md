# Claude Runner

Some facts only the `claude` binary knows: its exact version, the plugin and marketplace state as it resolves them, its background agents, its auto-mode configuration. [`a.Runner()`](ds:block?id=claude-runner-dbm6t8tp) is the Process door to those reads: each method runs one `claude` subcommand through `Host.Exec` [with the home rule applied and no terminal](ds:block?id=claude-runner-run-af4j3992), and decodes its output. Use it when the disk reads are not enough and the user has agreed to run the binary; nothing in the library calls a Runner method on its own.

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- `Host.Exec`: a process runner. `HostFromOS` gives the real one; tests and examples use a fake that answers canned output. [A host without one returns an `agentio.ErrUnsupported` error.](ds:block?id=claude-runner-run-af4j3992)
- Every method is labelled "may touch login": vendor subcommands are never assumed side-effect free.

The snippets come from [`examples/claudelaunch`](../../examples/claudelaunch/), whose process runner answers each argv line with recorded-shape JSON (placeholder values) and never starts a process.

## Step 1 — Read the binary version

[`r.BinaryVersion(ctx)`](ds:block?id=claude-runner-binaryversion-p4863j5t) runs `claude --version` for the default home and parses `2.1.325 (Claude Code)` with `claude.ParseVersion`. Prefer [`a.TargetFor(h)`](launch.md#step-3--fill-the-target-version), which reads the version from the install path without running anything; use the Runner when the install layout does not carry a version (a Homebrew cask) or to confirm what actually runs.

<!-- snippet: examples/claudelaunch/main.go#version -->
```go
// runnerVersion runs `claude --version` through the Runner door: a Process
// read, labelled "may touch login", so nothing calls it implicitly.
func runnerVersion(ctx context.Context, w io.Writer, r *claude.Runner) error {
	v, err := r.BinaryVersion(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "runner version: %s\n", v)
	return nil
}
```

## Step 2 — List plugins and marketplaces

[`r.Plugins(ctx, h, available)`](ds:block?id=claude-runner-plugins-5mz2n2aw) runs `claude plugin list --json` (with `--available` when asked) and returns `[]claude.RunnerPlugin`: `ID`, `Version`, `Scope`, `Enabled`, `ProjectEnabled`, `InstallPath`, `InstalledAt`, `LastUpdated`. [`r.Marketplaces(ctx, h)`](ds:block?id=claude-runner-marketplaces-f5xgxq7b) runs `claude plugin marketplace list --json` and returns `[]claude.RunnerMarketplace` (`Name`, `Source`, `Repo`, `InstallLocation`). The disk readers in [Claude plugins](plugins.md) give the same lists from `installed_plugins.json` and `known_marketplaces.json` without a process; the Runner shows the binary's merged view.

<!-- snippet: examples/claudelaunch/main.go#plugins -->
```go
// runnerPlugins lists plugins and marketplaces as the claude binary reports
// them (`plugin list --json`, `plugin marketplace list --json`).
func runnerPlugins(ctx context.Context, w io.Writer, r *claude.Runner, h claude.Home) error {
	plugins, err := r.Plugins(ctx, h, false)
	if err != nil {
		return err
	}
	for _, p := range plugins {
		fmt.Fprintf(w, "runner plugin %s v%s (%s), enabled %t, installed %s\n", p.ID, p.Version, p.Scope, p.Enabled, p.InstalledAt.UTC().Format("2006-01-02"))
	}
	markets, err := r.Marketplaces(ctx, h)
	if err != nil {
		return err
	}
	for _, m := range markets {
		fmt.Fprintf(w, "runner marketplace %s: %s %s\n", m.Name, m.Source, m.Repo)
	}
	return nil
}
```

## Step 3 — Read background agents and auto mode

[`r.BackgroundAgents(ctx, h, all)`](ds:block?id=claude-runner-backgroundagents-2943yj66) runs `claude agents --json` (with `--all` for finished ones too) and returns `[]claude.BackgroundAgent` (`ID`, `PID`, `SessionID`, `Name`, `Cwd`, `Kind`, `State`, `Status`, `StartedAt`, `Unknown`). [Claude live sessions](sessions.md#step-3--read-jobs-and-the-daemon) reads the same jobs from disk. [`r.AutoModeConfig(ctx, h)`](ds:block?id=claude-runner-automodeconfig-ed2d3mz9) and [`r.AutoModeDefaults(ctx, h)`](ds:block?id=claude-runner-automodedefaults-8p2z5nv4) run `claude auto-mode config` and `claude auto-mode defaults` and return the output as `agentio.Raw`: no sample of their shape is recorded yet, so the library does not type it.

<!-- snippet: examples/claudelaunch/main.go#agents -->
```go
// runnerAgents lists background agents (`agents --json --all`) and reads
// the auto-mode config and defaults, which stay raw JSON until a sample of
// their shape is recorded.
func runnerAgents(ctx context.Context, w io.Writer, r *claude.Runner, h claude.Home) error {
	bg, err := r.BackgroundAgents(ctx, h, true)
	if err != nil {
		return err
	}
	for _, b := range bg {
		fmt.Fprintf(w, "background agent %s %q: pid %d, state %s, cwd %s\n", b.ID, b.Name, b.PID, b.State, b.Cwd)
	}
	cfg, err := r.AutoModeConfig(ctx, h)
	if err != nil {
		return err
	}
	defs, err := r.AutoModeDefaults(ctx, h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "auto mode config %s, defaults %s\n", cfg, defs)
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudelaunch
```

The lines for this page:

```text
runner version: 2.1.325
runner plugin demo@example-market v1.2.0 (user), enabled true, installed 2026-11-01
runner marketplace example-market: github example/market
background agent ab12 "fix-tests": pid 4242, state blocked, cwd /work/proj4
auto mode config {"enabled": true}, defaults {"enabled": false}
```

## Pitfalls

- **Every call starts a process.** Each method is Process · Read; do not call them in a render loop. Cache results and refresh on demand.
- **"May touch login" is literal.** `claude auth status` (see [Claude account and identity](account.md#step-3--read-auth-status-through-the-runner)) is documented to create `~/.claude.json` when missing; other subcommands are not verified side-effect free. Ask the user before running them on their machine.
- **[A non-zero exit is an error.](ds:block?id=claude-runner-run-af4j3992)** The error names the command and carries a short, single-line stderr snippet; a missing binary is the runner's own error (wrapped with the command line).
- **[The home rule applies.](ds:block?id=homeenvchange-59nx5j7k)** Runner calls for the default home have `CLAUDE_CONFIG_DIR` absent; for another home it is set. [`BinaryVersion` always uses the default home](ds:block?id=claude-runner-binaryversion-p4863j5t).
- **Output shapes come from recorded samples.** [Unknown keys of `BackgroundAgent` land in `Unknown`](ds:block?id=claude-runner-backgroundagents-2943yj66); the plugin and marketplace rows drop keys they do not model.
- **Tests use a fake runner keyed by argv.** [`agenttest.FakeExec`](ds:block?id=agenttest-fakeexec-run-cg8fa6zc) answers by `Command.String()` (for example `claude plugin list --json`) and records calls, so a test can assert which commands ran and that none carried a token.

## Related

- [Claude account and identity](account.md#step-3--read-auth-status-through-the-runner) (`AuthStatus`), [Claude plugins](plugins.md), [Claude live sessions](sessions.md), [Claude doctor](doctor.md), [Claude launch and resume](launch.md)
- Neutral view: [The doors](../capabilities/doors.md#step-1--run-the-vendor-binary-through-a-runner), [Install and identity](../capabilities/install-identity.md#step-2--run-the-binary-for-its-version)
- Example: [`examples/claudelaunch`](../../examples/claudelaunch/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
