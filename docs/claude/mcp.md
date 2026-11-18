# Claude MCP servers

Claude Code connects to MCP servers configured in several places: per project in `.claude.json` (local scope), the project's `.mcp.json` (project scope, needs approval), the home's `.claude.json` (user scope), the managed `managed-mcp.json`, and a legacy `<home>/mcp.json`. `a.MCPServers(h, &p)` lists all of them from disk without starting any. Use it to show which servers a session will get, which still wait for approval, and which need a login.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent, a home and optionally a project (see [The Claude agent](agent.md)).
- Listing is a Disk read and never spawns a server. Changes are vendor commands the module builds for you; it never edits `.claude.json` or `.mcp.json` itself.

The snippets come from [`examples/claudeextensions`](../../examples/claudeextensions/).

## Step 1 — List MCP servers

Each `MCPServer` has `Name`, `Scope` (`MCPScopeLocal`, `MCPScopeProject`, `MCPScopeUser`, `MCPScopeManaged`, `MCPScopeLegacy`, …), `Source`, `Transport` (`stdio`, `sse`, `http`, `ws`; an empty type in the file means stdio), `Command`/`Args` or `URL`, `Env` and `Headers` (values are `agentio.EnvValue` and print `[redacted]`), `OAuth` (`MCPOAuth`: client id and callback port, never a secret) and, for project servers, `Approval` (`approved`, `rejected`, `pending`). `claude.ParseMCPConfig(body)` decodes an `mcpServers` file such as one you pass with `--mcp-config`.

<!-- snippet: examples/claudeextensions/main.go#mcp -->
```go
// showMCP lists every file-configured MCP server with its scope and
// approval. Listing never starts a server; env and header values print
// redacted.
func showMCP(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("MCP servers")
	servers, err := a.MCPServers(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, s := range servers {
		p.line(fmt.Sprintf("%-13s scope=%-7s transport=%-5s approval=%q env=%v headers=%v", s.Name, s.Scope, s.Transport, s.Approval, s.Env, s.Headers))
	}
	parsed, err := claude.ParseMCPConfig([]byte(`{"mcpServers": {"docs": {"type": "http", "url": "https://mcp.example.com/docs", "oauth": {"clientId": "docs-cli", "callbackPort": 8765}}}}`))
	if err != nil {
		return err
	}
	docs := parsed["docs"]
	p.line(fmt.Sprintf("ParseMCPConfig: docs transport=%s oauth client=%s port=%d", docs.Transport, docs.OAuth.ClientID, docs.OAuth.CallbackPort))
	return nil
}
```

## Step 2 — Find servers that need auth

`a.MCPNeedsAuth(h)` reads `mcp-needs-auth-cache.json`: server name → when Claude Code last saw it asking for a login. It returns nil when the file is absent.

<!-- snippet: examples/claudeextensions/main.go#needsauth -->
```go
// showMCPNeedsAuth reads which servers Claude Code saw asking for a login.
func showMCPNeedsAuth(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("MCP servers needing auth")
	seen, err := a.MCPNeedsAuth(a.DefaultHome()) // nil when the cache file is absent
	if err != nil {
		return err
	}
	for name, at := range seen {
		p.line(fmt.Sprintf("%s needs auth since %s", name, at.UTC().Format(time.RFC3339)))
	}
	return nil
}
```

## Step 3 — Build add and remove commands

`MCPAddCommand(h, dir, server)` builds `claude mcp add-json -s <scope> <name> <json>`, `MCPRemoveCommand` builds `claude mcp remove -s <scope> <name>`, `MCPListCommand` builds `claude mcp list`, and `MCPResetProjectChoicesCommand` builds `claude mcp reset-project-choices`. All are Pure: they return an `agentio.Command` with the home's environment applied and `dir` as the working directory, for you to run or print.

<!-- snippet: examples/claudeextensions/main.go#mcpcommands -->
```go
// showMCPCommands builds the `claude mcp` commands that change servers.
// They are data: print them, or run them yourself.
func showMCPCommands(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("MCP commands")
	h := a.DefaultHome()
	add, err := a.MCPAddCommand(h, proj.Dir, claude.MCPServer{
		Name: "docs", Scope: claude.MCPScopeUser, Transport: claude.TransportHTTP, URL: "https://mcp.example.com/docs",
	})
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("add:    %s", add))
	p.line(fmt.Sprintf("remove: %s", a.MCPRemoveCommand(h, proj.Dir, claude.MCPScopeUser, "docs")))
	p.line(fmt.Sprintf("list:   %s", a.MCPListCommand(h, proj.Dir)))
	p.line(fmt.Sprintf("reset:  %s", a.MCPResetProjectChoicesCommand(h, proj.Dir)))
	_, err = a.MCPAddCommand(h, proj.Dir, claude.MCPServer{
		Name: "db", Scope: claude.MCPScopeUser, Command: "db-mcp",
		Env: map[string]agentio.EnvValue{"DB_PASSWORD": agentio.NewEnvValue("hunter2")},
	})
	p.line(fmt.Sprintf("server with env values refused: %v", err))
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudeextensions
```

The sections this page covers:

```text
== MCP servers
proj-db       scope=project transport=stdio approval="approved" env=map[DB_PASSWORD:[redacted]] headers=map[]
fixture-http  scope=user    transport=http  approval="" env=map[] headers=map[Authorization:[redacted]]
fixture-stdio scope=user    transport=stdio approval="" env=map[FIXTURE_TOKEN:[redacted]] headers=map[]
ParseMCPConfig: docs transport=http oauth client=docs-cli port=8765

== MCP servers needing auth
fixture-http needs auth since 2026-11-06T21:20:00Z

== MCP commands
add:    claude mcp add-json -s user docs "{\"type\":\"http\",\"url\":\"https://mcp.example.com/docs\"}"
remove: claude mcp remove -s user docs
list:   claude mcp list
reset:  claude mcp reset-project-choices
server with env values refused: agentio: unsupported: claude: mcp add-json with env or headers: agentio passes the server JSON on argv (agentio.Command has no stdin); env values and headers are secrets and never go on argv
```

## Pitfalls

- **A server with env values or headers cannot be added through `MCPAddCommand`.** The library passes the server JSON on argv and never puts secrets on argv. Claude Code 2.1.327 added `claude mcp add-json <name> -`, which reads the JSON from stdin (feature `FeatureMCPAddJSONStdin`), but `agentio.Command` has no stdin channel, so the refusal stays. Add the server without them, then set the values through Claude Code itself.
- **Never run `MCPListCommand` casually.** `claude mcp list` starts every server for a health check; the library builds it but never runs it. Use `MCPServers` to list.
- Project servers from `.mcp.json` are listed even when not approved; check `Approval` before treating one as active. `pending` servers are what the doctor's `mcp-project-pending` check reports.
- Servers that come from plugins, claude.ai connectors or a session's `--mcp-config` are not in these files and are not listed; a running session reports its real set in the stream `init` line (see [Claude stream](stream.md)).
- `Env` and `Headers` print redacted even in JSON; `Reveal()` is there for code that must pass a value on, and nothing else.

## Related

- [Claude global state](global-config.md), [Claude plugins](plugins.md), [Claude settings](settings.md), [Claude doctor](doctor.md)
- Neutral view: [Config: MCP servers, instructions, skills, plugins](../capabilities/config.md)
- Example: [`examples/claudeextensions`](../../examples/claudeextensions/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
