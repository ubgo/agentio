# Changelog

All notable changes to agentio are recorded in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Each Go module is tagged on its own (`v0.1.1`, `claude/v0.1.1`, `codex/v0.1.1`, `codex/state/v0.1.1`, `builtin/v0.1.1`); an entry names the module it affects when it is not the whole library.

## [Unreleased]

### Added

- Modules `github.com/ubgo/agentio` (neutral core, typed capability accessors, `agenttest`), `claude` (Claude Code), `codex` (Codex), `codex/state` (Codex SQLite stores, read-only) and `builtin` (every built-in agent plus config-defined agents).

### Changed

- Every launch scrubs the vendor's inherited credential variables (`Spec.CredentialEnv`), for a home login too, before `Target.ExtraEnv` is applied: an inherited `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN` or `CLAUDE_CODE_OAUTH_TOKEN` no longer overrides a Claude home's login. Codex's `CredentialEnv` is now only `CODEX_API_KEY` (it beats the home login in `codex exec`); `OPENAI_API_KEY` is kept because Codex auth does not read it and a model provider may name it as its key.

- `Host` has `GOOS` and `SystemRoot`, filled only by `HostFromOS` (`Host.SystemPath` joins a system path under the root; `agenttest.WithSystem` sets both). Claude `SystemPaths`, `ManagedSettings`, the managed `CLAUDE.md` in `MemoryFiles`, managed MCP servers, `Runner.ManagedPlist` and Codex's `/etc/codex` layers resolve through them instead of `runtime.GOOS` and absolute paths; a fixture or hand-built Host has no system locations, so the machine's `/etc` never changes its output.
- `HistoryMover.MoveHistory` returns `agentio.HistoryMove{Moved, Backup}` instead of a bare `Backup`, so a folder without history (`Moved` false) is no longer indistinguishable from a move; `claude.Writer.MoveHistory` returns the same type instead of `(moved, backup, err)`.
- `HomeLink` carries the `Agent` that planned it (`agentio.CheckLinksOwner` checks a plan).
- Config layers are listed highest precedence first for every vendor and the neutral `ConfigReader.ConfigLayers` (documented on the interface): Codex `ConfigLayers` and `EffectiveConfig.Layers` now match Claude, which already listed the winning layer first (module `codex`).

### Fixed

- `ParseVersion` only starts a version at a word boundary: `x86_64 build 7` parses as `7.0.0`, no longer `64.0.0`.
- Claude usage, profile and token-probe calls return `ErrUnsupported` when `Host.HTTP` is nil instead of building a default client that reached the real network (module `claude`).
- User-facing strings no longer name private design ids: the Claude model-lister Unsupported reason dropped "(OD-15)" and the Codex accept-edits reason "(OD-14)"; an architecture test fails on any string literal naming one.
- The Claude doctor `retention` check warns about clock skew when the `.last-cleanup` stamp is after the host clock, instead of printing a negative age ("last cleanup -7 h ago").
- `claude.Writer.MoveHistory` refuses while a Claude session runs in the source folder (or one of its conversations is live), with a typed `*claude.LiveSessionError` matching `agentio.ErrMoveBlocked` and the blocker text `PlanMove` lists.
- `claude.Writer.ApplyHomeLinks` and `Core.ApplyHomeLinks` refuse links planned for another agent's homes with `ErrForeignHome`, as `ApplyMove` does.

### Documentation

- A documentation site under `docs/`: a learning path from the first `Host` to a production setup, a "start from your goal" table, and guides for the core, the shared capabilities, every Claude Code and Codex area, testing, and compatibility. Every guide has complete snippets, the real output of a runnable example, and pitfalls taken from the code and tests.
- Choosing guides (`docs/choosing/`): concrete vendor type or neutral accessor, which door (Runner, Prober, Writer), live or cached usage, built-in or config-defined agent, which module to import, and file writes or vendor commands.
- A feature index (`docs/reference/features.md`) listing every feature with its package, the guide step that teaches it and an example that calls it.
- An architecture diagram (`docs/assets/architecture.svg`).
- Runnable examples under `examples/`, one per area, each with a README showing its real output and a test that runs it on the repository's fixture homes, fakes and local test servers (no real account, binary or network). `examples/agentinfo` and `examples/drift` gained READMEs and fixture-home tests, and `drift` a `-fixtures` flag.
- A docs checker (`internal/docscheck`, run by `task docs:check`) that fails on broken links and anchors, hard-wrapped Markdown, local paths, attribution lines, and snippets that differ from the example source they are copied from; `task docs:snippets` rewrites snippet blocks from source.
- Repository files: README, CONTRIBUTING, SECURITY, CODE_OF_CONDUCT, LICENSE, issue and pull request templates, and `llms.txt`.

[Unreleased]: https://github.com/ubgo/agentio/commits/main
