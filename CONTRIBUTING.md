# Contributing to agentio

Thanks for your interest in improving **agentio**. This guide covers the development setup, the rules every change must keep, and what a good pull request looks like.

By participating, you agree to abide by our [Code of Conduct](CODE_OF_CONDUCT.md). Security problems go through [SECURITY.md](SECURITY.md), never a public issue.

## Development setup

You need Go (the version in the root `go.mod` or newer) and [Task](https://taskfile.dev). [golangci-lint](https://golangci-lint.run) v2 is required for `task check` (config in `.golangci.yml`); [staticcheck](https://staticcheck.dev) is optional and runs when it is installed.

The repository holds several Go modules. A `go.work` ties them together for local development; it is gitignored and never committed, so create it once after cloning:

```sh
git clone https://github.com/ubgo/agentio.git
cd agentio
go work init . claude codex codex/state builtin examples
```

| Module | Directory | Holds |
|---|---|---|
| `github.com/ubgo/agentio` | `.` | neutral core, shared capability interfaces and accessors, `agenttest`, `frontmatter`; stdlib only |
| `github.com/ubgo/agentio/claude` | `claude/` | Claude Code |
| `github.com/ubgo/agentio/codex` | `codex/` | Codex (adds go-toml for `config.toml`) |
| `github.com/ubgo/agentio/codex/state` | `codex/state/` | Codex SQLite stores (adds pure-Go `modernc.org/sqlite`) |
| `github.com/ubgo/agentio/builtin` | `builtin/` | the typed set of built-in and config-defined agents |
| `github.com/ubgo/agentio/examples` | `examples/` | runnable examples; never imported by a library module |

The dependency direction is fixed: the root imports no vendor module and no third-party package; `claude` never imports `codex` or the reverse; `builtin` imports `claude` and `codex` but never `codex/state`. `internal/archtest` fails the build when a change breaks this.

## Checks

Run these before you open a pull request:

| Command | What it does |
|---|---|
| `task lint` | golangci-lint on every module (`.golangci.yml`) |
| `task check` | the full gate: gofmt, `go vet`, staticcheck (if installed), golangci-lint, race tests, cross-compile for 6 platforms with CGO off, standalone (`GOWORK=off`) builds of every module |
| `task test:quick` | unit tests of every module, cached, no race detector (fast loop while you work) |
| `task docs:check` | fails when a Go snippet in the docs differs from the example source it is synced from |
| `task docs:snippets` | rewrites the docs snippets from their example sources |

Code snippets in the docs are copied from compiled examples, not written by hand. Mark a region in Go source with `// snippet:start NAME` and `// snippet:end NAME`, put `<!-- snippet: examples/<dir>/<file>.go#NAME -->` above the fenced block in the markdown, write the code first, then run `task docs:snippets`.

## Rules every change keeps

These are easy to break and the reviews check them. Most are also enforced by `internal/archtest`.

- **Real shapes only.** Every struct is modelled on a real file or response from an installed vendor CLI, or on the vendor's own source. Never guess a shape. Add the sample as a fixture with every value replaced before writing the type.
- **Fixtures, never real accounts.** Tests run against fixture homes (`claude/testdata/homes/<version>/userhome`, `codex/testdata/homes/<version>`), `agenttest` fakes (`NewHost`, `FakeSecrets`, `FakeProcesses`, `FakeExec`, `FailingHTTP`), `httptest` servers and temp dirs. A test never reads your real home, runs a real `claude` or `codex` binary, calls a vendor endpoint or spends quota. Fixtures contain no real tokens, emails, ids, names or local paths.
- **Type-safe first.** Concrete vendor types (`*claude.Agent`) for code that knows the agent; typed accessors (`agentio.UsageOf(a)`) for code that loops over agents. No `any`, no `map[string]any`, no string-keyed features. Enums for fixed sets, `time.Duration` for lengths. Unknown vendor keys go into a typed `Unknown` field; they are never dropped silently, and an unknown enum value is kept, not rejected.
- **No hidden state.** Nothing reads `$HOME`, `os.Environ`, the keychain, the clock or the network except through `agentio.Host`. `HostFromOS()` is the only place that touches the real machine. No package-level variables, no global registry.
- **Every function states its cost** in its doc comment: Pure, Disk, Process (runs a local program), Network (may 429) or Quota (spends model tokens), plus Read or Write. Process, Quota and Write methods live behind the `Runner`, `Prober` and `Writer` doors. Never loop a quota-spending call.
- **Read-only by default.** Writing anything a vendor CLI owns (settings, MCP config, history) is a separate `Writer` method that backs the file up first and returns an `agentio.Backup`.
- **Never refresh, write or print an OAuth token.** Refreshing rotates the refresh token and logs the vendor CLI out. Tokens travel as the redacting `agentio.Token`, never on argv; `Reveal()` is confined to an HTTP auth header and decoding a keychain blob.
- **Vendor facts that bite.** The default home means the home variable (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`) is absent from the child env, not set to the default path. Claude keychain lookups name the account. Never read a transcript whole (head for cwd, bounded tail for the title). Codex conversations cannot move between homes. Find the binary with a PATH walk over `Host.Env`, never a shell (a user's `claude` may be a shell function).
- **Feature bounds are sourced.** A version bound in a feature table names the vendor changelog or release note it came from. A version outside the tested range warns; it never refuses.
- **Clean code.** gofmt, vet, staticcheck and golangci-lint clean with no suppressions (no `//nolint`; the one exclusion in `.golangci.yml` covers example output written with `fmt.Fprint*`). Named constants instead of magic values, and comments that say why.

## Supporting a new vendor version

1. Install the new version and copy a home into a new fixture directory (`claude/testdata/homes/<version>/` or `codex/testdata/homes/<version>/`), replacing every value.
2. Run the drift report (`task drift`) on a machine that has the version installed; it lists unknown keys, enum values, record types and flags without printing any value.
3. Model what changed, add feature-table entries with their changelog source, and raise `TestedMax`.
4. The tests run over every fixture version, so older versions stay covered.

## Branches and commits

- Branch off `main` with a short descriptive name (`fix/...`, `feat/...`, `docs/...`).
- Write commits in Conventional Commit style, `type(scope): description`, where scope is the module or area: `feat(claude): ...`, `fix(codex/state): ...`, `test(archtest): ...`, `docs: ...`.
- One logical change per commit. A change that spans modules lands the root change first.

## Pull request checklist

- [ ] The change is scoped and described (link the issue it closes).
- [ ] `task check` passes; `task docs:check` passes when docs or examples changed.
- [ ] New behaviour has tests against fixtures, and each new guard has a test that fails when the guard is removed.
- [ ] New vendor shapes come from a real sample, recorded as a fixture with values replaced.
- [ ] Docs and CHANGELOG updated for user-facing changes.
- [ ] No unrelated files or formatting churn.

## Releases and versioning

Each module is tagged on its own (`v0.1.1`, `claude/v0.1.1`, `codex/v0.1.1`, `codex/state/v0.1.1`, `builtin/v0.1.1`). Bumps are PATCH by default; a published tag is never moved (use `retract` instead). A cross-module change tags the root first, then the modules that require it. User-facing changes go under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md), following [Keep a Changelog](https://keepachangelog.com/).

## Questions

Open an [issue](https://github.com/ubgo/agentio/issues). We are happy to help you land your first contribution.
