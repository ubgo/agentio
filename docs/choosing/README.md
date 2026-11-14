# Choosing guides

agentio gives you more than one way to do most things on purpose: a concrete vendor type or a neutral accessor, a free disk read or a network call, a file write or a vendor command. Each guide below answers one of those choices with a decision table, "if this, then that" rules, the traps, and a guide and runnable example for every option. Every claim is checked against the code.

| Question | Short answer | Guide |
|---|---|---|
| Do I use `*claude.Agent` or `agentio.UsageOf(a)`? | Concrete type for code that knows the vendor; accessors for loops over agents. Most programs use both. | [Concrete vendor type or neutral accessor](concrete-or-accessor.md) |
| Which door do I need: Runner, Prober or Writer? | None for reads. Runner to run the binary, Prober to spend quota on a token (Claude only), Writer to change Claude settings and hooks. | [Which door](doors.md) |
| Live or cached usage? | Cached in loops and status lines (free); live once per user action (may 429); probe only for bare tokens. | [Live or cached usage](live-or-cached-usage.md) |
| Built-in or config-defined agent? | Built-in for Claude Code and Codex; a `Spec` for any other CLI you only need to find, list and launch. | [Built-in or config-defined agent](builtin-or-config-agent.md) |
| Which modules do I import? | Only the vendors you use. `codex/state` only if you need Codex's SQLite stores: it pulls a full SQLite driver. | [Which module to import](modules.md) |
| Write the file or run the vendor's command? | Writer for Claude `settings.json` and hooks; vendor commands for `.claude.json`, `.mcp.json` and everything Codex. | [Writes: file or vendor command](writes.md) |

## Related

- [Learning path](../README.md#learning-path) and [start from your goal](../README.md#start-from-your-goal)
- [Feature index](../reference/features.md): every capability and method with its cost, guide and example
