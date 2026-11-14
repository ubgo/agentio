# Codex MCP servers

List the MCP servers Codex would start in a folder (`[mcp_servers.<name>]` from the user config plus trusted project layers), read one env value on purpose as a redacting `EnvValue`, and build the vendor's own `codex mcp add` / `codex mcp remove` commands instead of editing `config.toml`.

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` and `codex.Home` from [The Codex agent](agent.md).
- Project-level servers come from `<project>/.codex/config.toml` and count only in a trusted folder (see [config](config.md#step-2--read-the-effective-config)).

The snippets come from [`examples/codexconfig`](../../examples/codexconfig/); the fixture `config.toml` defines `node_repl` (with a fake `SECRET_TOKEN` env value) and a disabled `computer-use`.

## Step 1 — List MCP servers

`MCPServers(h, dir)` returns `MCPServerEntry` values sorted by name: the server's `Name`, the `Layer` and `Source` file it came from, and the embedded `MCPServer` (command, args, cwd, env names, URL, auth, timeouts, tool filters; every scalar an `Opt`). `Transport()` is `http` when a URL is set, else `stdio`. Listing never spawns a server.

Env values are never loaded into the struct: `EnvKeys` holds names only. `MCPServerEnvValue(h, server, key)` reads one value as an `agentio.EnvValue` that prints `[redacted]`; `ErrNoEnvValue` when the server or key is absent.

<!-- snippet: examples/codexconfig/extensions.go#list-mcp -->
```go
// listMCP lists the effective [mcp_servers] for a folder (user config
// plus trusted project layers). Env values are never in the struct: only
// their names, and one value on request as a redacting EnvValue.
func listMCP(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()
	servers, err := a.MCPServers(h, m.project)
	if err != nil {
		return err
	}
	for _, s := range servers {
		fmt.Fprintf(w, "mcp %s transport=%s layer=%s enabled-set=%v env-keys=%v\n",
			s.Name, s.Transport(), s.Layer, s.Enabled.Set, s.EnvKeys)
	}
	v, err := a.MCPServerEnvValue(h, "node_repl", "SECRET_TOKEN")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "SECRET_TOKEN prints as %v\n", v)
	if _, err := a.MCPServerEnvValue(h, "node_repl", "MISSING"); errors.Is(err, codex.ErrNoEnvValue) {
		fmt.Fprintln(w, "MISSING: not in config.toml")
	}
	return nil
}
```

## Step 2 — Build add and remove commands

`MCPAddCommand(h, name, MCPAddSpec)` builds `codex mcp add NAME --url U [--bearer-token-env-var V]` for an HTTP server or `codex mcp add NAME -- CMD ARGS…` for a stdio one; `MCPRemoveCommand(h, name)` builds `codex mcp remove NAME`. Codex edits its own `config.toml` when you run them.

<!-- snippet: examples/codexconfig/extensions.go#mcp-commands -->
```go
// mcpCommands builds `codex mcp add|remove`: the vendor edits its own
// config.toml when the caller runs them. Env values are refused because
// they would ride on argv.
func mcpCommands(w io.Writer, host agentio.Host) error {
	a := codex.New(host)
	h := a.DefaultHome()
	add, err := a.MCPAddCommand(h, "docs", codex.MCPAddSpec{URL: "https://mcp.example.test", BearerTokenEnvVar: "DOCS_TOKEN"})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, argv(add))
	stdio, err := a.MCPAddCommand(h, "files", codex.MCPAddSpec{Command: "/opt/mcp/files", Args: []string{"--root", "/work"}})
	if err != nil {
		return err
	}
	fmt.Fprintln(w, argv(stdio))
	fmt.Fprintln(w, argv(a.MCPRemoveCommand(h, "docs")))

	_, err = a.MCPAddCommand(h, "x", codex.MCPAddSpec{Command: "srv", Env: map[string]agentio.EnvValue{"K": agentio.NewEnvValue("v")}})
	fmt.Fprintf(w, "env values refused: %v\n", errors.Is(err, agentio.ErrUnsupported))
	return nil
}
```

## Try it

```sh
cd examples && go run ./codexconfig
```

The `== mcp` section:

```text
== mcp
mcp computer-use transport=stdio layer=user enabled-set=true env-keys=[]
mcp node_repl transport=stdio layer=user enabled-set=false env-keys=[CODEX_HOME SECRET_TOKEN]
SECRET_TOKEN prints as [redacted]
MISSING: not in config.toml
codex mcp add docs --url https://mcp.example.test --bearer-token-env-var DOCS_TOKEN
codex mcp add files -- /opt/mcp/files --root /work
codex mcp remove docs
env values refused: true
```

## Pitfalls

- **`MCPAddSpec.Env` is refused** with `agentio.ErrUnsupported`: `codex mcp add` takes env values on argv (`--env K=V`), which would expose them in the process table. Add the server without env and set env in `config.toml` yourself or with the vendor TUI.
- **A URL and a command together, or neither, is `ErrInvalidOption`.**
- **`BearerTokenEnvVar` is a variable name**, never a token. An inline `bearer_token` in `config.toml` is reported only as `HasBearerToken`; its value is never loaded.
- **`Enabled` is an `Opt`.** An absent `enabled` key (`Set=false`) means Codex's default (enabled); only `enabled = false` disables a server.
- **Never print `EnvValue.Reveal()`.** It exists to hand the value to a child process's environment, not to logs.

## Related

- [MCP servers (neutral)](../capabilities/config.md), [Secrets: Token, EnvValue, Personal](../core/secrets.md)
- [Codex config.toml](config.md), [Codex launch](launch.md#step-3--build-archive-delete-and-queue-commands) (`MCPServer` runs Codex itself as an MCP server)
- [Claude MCP servers](../claude/mcp.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
