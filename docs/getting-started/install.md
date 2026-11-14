# Install agentio

agentio is split into several Go modules so you only download what you use. Add the root module for the neutral types, then one module per agent you need: `claude` for Claude Code, `codex` for OpenAI Codex, `builtin` when you want every built-in agent at once. A program that only knows Claude Code never pulls the TOML or SQLite code Codex needs.

## Before you start

- Go 1.26.4 or newer (the `go` line of every module).
- A Go module of your own (`go mod init example.com/yourtool`).
- Nothing else: the library never needs the `claude` or `codex` binary to build, and reading a home needs only the files on disk.

## Step 1 — Add the module you need

Pick the modules from this table. Each row lists what it adds on top of the standard library.

| Module | Use it for | Pulls in |
|---|---|---|
| `github.com/ubgo/agentio` | neutral core: `Agent`, `Spec`, `Host`, `Home`, `Target`, `Command`, `Token`, errors, versions, enums, the typed capability accessors (`agentio.UsageOf` …) | stdlib only |
| `github.com/ubgo/agentio/claude` | everything about Claude Code: install, homes, settings, hooks, MCP, account, usage, conversations, sessions, launch | root |
| `github.com/ubgo/agentio/codex` | everything about OpenAI Codex: install, `config.toml`, auth, usage, conversations, sessions, launch | root, `github.com/pelletier/go-toml/v2` |
| `github.com/ubgo/agentio/codex/state` | Codex's SQLite state index (`state_5.sqlite`) | root, `codex`, `modernc.org/sqlite` |
| `github.com/ubgo/agentio/builtin` | every built-in agent in one call (`builtin.New`), plus agents described by config | root, `claude`, `codex` |

Claude Code only:

```sh
go get github.com/ubgo/agentio github.com/ubgo/agentio/claude
```

Codex only:

```sh
go get github.com/ubgo/agentio github.com/ubgo/agentio/codex
```

Every built-in agent, for a tool that loops over whatever is installed:

```sh
go get github.com/ubgo/agentio github.com/ubgo/agentio/builtin
```

Add `github.com/ubgo/agentio/codex/state` only when you read Codex's SQLite index; it is the one module with a large dependency (a pure-Go SQLite, no cgo).

The root module never imports a vendor module, and `claude` never imports `codex`, so the dependency graph stays exactly as the table shows.

## Step 2 — Check that it builds

Run the quick start from this repository to see the library work on a fixture home, with no real account, binary or network involved:

```sh
git clone https://github.com/ubgo/agentio
cd agentio/examples
go run ./quickstart
```

In your own module, build and vet after `go get`:

```sh
go build ./...
go vet ./...
```

## Try it

```sh
cd examples && go run ./quickstart
```

The first lines of the real output:

```text
host: user=user PATH set=true
host: MODE=b EMPTY="" set=true MISSING set=false
install: path=$TMP/bin/claude
install: real=$TMP/lib/node_modules/@anthropic-ai/claude-code/cli.js
install: method=npm-global version=2.1.325
install: shadowed=$TMP/old/bin/claude
home: name=personal default=true dir=~/.claude
home: name=work default=false dir=~/.claude-work
neutral: id=claude name="Claude Code" binary=claude
```

`$TMP` is a temp dir holding a fake npm install and `~` is the fixture user home in `claude/testdata/homes/2.1.325/userhome`. [Your first agent](first-agent.md) walks through the code.

## Pitfalls

- **Importing `builtin` pulls every vendor module.** That is its job. A Claude-only tool should import `claude` directly; it then never downloads go-toml or SQLite.
- **`codex/state` is separate from `codex` on purpose.** `codex` reads rollout files and `config.toml` without SQLite; only the state index needs `modernc.org/sqlite`.
- **Vendor `ParseVersion` has the root shape.** `agentio.ParseVersion`, [`claude.ParseVersion`](ds:block?id=claude-parseversion-n4kvb682) and [`codex.ParseVersion`](ds:block?id=codex-parseversion-zx9z9rav) all return `(agentio.Version, bool)`; `ok` is false when the text holds no version. See [Versions](../core/versions.md).
- **Building never runs the vendor CLI.** No step here needs `claude` or `codex` on PATH; when they are missing, `Install` returns an error matching [`agentio.ErrNotInstalled`](ds:block?id=claude-agent-install-472s4jn6) rather than failing to build.

## Related

- [Your first agent](first-agent.md) — the quick start, step by step
- [Host: the only door to the machine](../core/host.md)
- [Built-in agents](../capabilities/builtin.md)
- [Choosing guides](../choosing/README.md)
- [Feature index](../reference/features.md)
