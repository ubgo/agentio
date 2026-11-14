# Which module to import

agentio is five Go modules so that a program downloads and compiles only what it uses. The root module is stdlib only; each vendor module carries its own dependencies; the heavy SQLite driver lives in a module of its own. Pick the smallest set that covers the agents and data you need.

## Decision table

| Module | Import it for | Pulls (direct dependencies) | Guide | Example |
|---|---|---|---|---|
| `github.com/ubgo/agentio` | `Host`, `Agent`, `Spec`, `Command`, `Token`, accessors, errors, enums; `agentio/agenttest` for tests; `agentio/frontmatter` | nothing: stdlib only | [Install agentio](../getting-started/install.md#step-1--add-the-module-you-need) | [values](../../examples/values/), [fromspec](../../examples/fromspec/) |
| `github.com/ubgo/agentio/claude` | everything Claude Code | the root module only | [The Claude agent](../claude/agent.md) | [claudesettings](../../examples/claudesettings/) |
| `github.com/ubgo/agentio/codex` | everything Codex: `config.toml`, auth, usage, rollouts, sessions, launch | root, `github.com/pelletier/go-toml/v2` | [The Codex agent](../codex/agent.md) | [codexconfig](../../examples/codexconfig/) |
| `github.com/ubgo/agentio/codex/state` | Codex's SQLite stores: threads, turns, items, goals, queue, memories, logs | root, `codex`, `modernc.org/sqlite` (pure Go; brings `modernc.org/libc` and friends) | [Codex SQLite state](../codex/state.md#step-1--open-the-state-database) | [codexstate](../../examples/codexstate/) |
| `github.com/ubgo/agentio/builtin` | every built-in agent as one typed set, plus config-defined agents | root, `claude`, `codex` (so go-toml too); not `codex/state` | [Built-in agents](../capabilities/builtin.md) | [accessors](../../examples/accessors/) |

## If this, then that

- If your program only knows Claude Code, import `claude` and nothing else. It never downloads go-toml or SQLite.
- If it only knows Codex, import `codex`. Conversations, turns, prompt history, usage and live sessions all work from rollouts and files without SQLite.
- If it loops over every agent the library knows, import `builtin` and use `builtin.New(h)`. Adding a vendor later then needs no change in your code.
- If you need Codex's thread index exactly as Codex sees it (archived flags, thread names, goals, the queue, memories, logs), add `codex/state`. It is the only module that opens SQLite, read-only.
- If you only run config-defined agents (a launcher for CLIs agentio has no module for), the root module alone is enough: `agentio.FromSpec`.
- If you only write tests against fakes, `agentio/agenttest` is part of the root module; there is nothing else to add.

## Pitfalls

- **`builtin` costs Codex's dependency.** A Claude-only tool that imports `builtin` for convenience downloads go-toml; call `claude.New(h)` instead and use `c.Neutral()` where you need the neutral view.
- **`codex/state` is not in `builtin`.** `builtin.New` gives you the `*codex.Agent`; open the SQLite stores yourself with `state.Open(codexAgent.Paths(home))`.
- **Pass `state.Open` the full `codex.Paths`.** It expects every store path that `a.Paths(h)` (or `PathsWith`) computes; a hand-built `Paths` with only the threads file set is refused, even though files open lazily.
- **SQLite is pure Go, but large.** `modernc.org/sqlite` needs no CGO and cross-compiles, yet it adds noticeable build time and binary size; that is the reason it is its own module.
- **Keep module versions together.** The vendor modules require the root module at a matching version; upgrade them together so a vendor module never sees a root API it predates.

## Related

- [Install agentio](../getting-started/install.md), [Built-in agents](../capabilities/builtin.md), [Codex SQLite state](../codex/state.md), [Codex conversations](../codex/conversations.md)
- Other choices: [Concrete type or accessor](concrete-or-accessor.md), [Built-in or config-defined agent](builtin-or-config-agent.md)
- Examples: [quickstart](../../examples/quickstart/), [accessors](../../examples/accessors/), [codexstate](../../examples/codexstate/)
- [Feature index](../reference/features.md), [All choosing guides](README.md)
