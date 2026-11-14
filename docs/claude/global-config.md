# Claude global state (.claude.json)

Claude Code keeps a large JSON file per home (`~/.claude.json` for the default home, `<home>/.claude.json` for others) with install facts, the logged-in account, user-scope MCP servers, a state entry per project folder and a cached copy of plan usage. `a.GlobalConfig(h)` types the parts worth reading and keeps the hundred-odd UI flags and caches in `Unknown`. Use it for account and per-project facts without running `claude` or going online.

## Before you start

- `go get github.com/ubgo/agentio/claude`; an agent and a home (see [The Claude agent](agent.md)).
- The file is often several MB. Each call below decodes it once (Disk read); call `GlobalConfig` once and reuse the value if you need several parts.

The snippets come from [`examples/claudesettings`](../../examples/claudesettings/).

## Step 1 — Read the global config

`GlobalConfig` returns install facts (`InstallMethod`, `AutoUpdates`, `NumStartups`, `FirstStartVersion`, …), `OAuthAccount`, `MCPServers` (user scope), `Projects` (folder → `ProjectState`) and `CachedUsage`. `a.Account(h)` returns just the account, nil when nobody is logged in. `claude.ParseGlobalConfig(body)` decodes bytes you already hold. Every id, email and name in `OAuthAccount` is an `agentio.Personal` and prints `[redacted]`.

<!-- snippet: examples/claudesettings/main.go#global -->
```go
// showGlobalConfig reads .claude.json: install facts, the logged-in account
// (personal values print redacted) and the size of what stays unmodelled.
func showGlobalConfig(p printer, host agentio.Host) error {
	p.section("global config")
	a := claude.New(host)
	h := a.DefaultHome()
	g, err := a.GlobalConfig(h) // a missing file is the zero value with Path set
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("path=%s install method=%q startups=%d", g.Path, g.InstallMethod, g.NumStartups))
	p.line(fmt.Sprintf("projects=%d user MCP servers=%d unmodelled keys=%d", len(g.Projects), len(g.MCPServers), len(g.Unknown)))

	acct, err := a.Account(h) // nil when nobody is logged in
	if err != nil {
		return err
	}
	if acct != nil {
		p.line(fmt.Sprintf("account email=%s org type=%s billing=%s", acct.EmailAddress, acct.OrganizationType, acct.BillingType))
	}

	body, err := os.ReadFile(g.Path)
	if err != nil {
		return err
	}
	parsed, err := claude.ParseGlobalConfig(body) // the same decode, from bytes
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("ParseGlobalConfig: first start version=%s", parsed.FirstStartVersion))
	return nil
}
```

## Step 2 — Read one project's state

`a.ProjectState(h, dir)` returns the entry for an absolute folder: `TrustAccepted`, `AllowedTools`, local-scope `MCPServers`, which `.mcp.json` servers were enabled or disabled, and `Last` (totals of the last session there: cost, durations, lines, tokens, per-model usage). It returns nil when Claude Code never ran in that folder. `a.CachedUsageBody(h)` returns the raw `cachedUsageUtilization` body and its fetch time; parse the body with `claude.ParseUsage`, or call `a.CachedUsage(h)` which does both (see [Claude usage](usage.md)).

<!-- snippet: examples/claudesettings/main.go#projectstate -->
```go
// showProjectState reads one folder's entry in .claude.json and the cached
// usage body Claude Code stored there.
func showProjectState(p printer, host agentio.Host) error {
	p.section("project state")
	a := claude.New(host)
	h := a.DefaultHome()
	st, err := a.ProjectState(h, "/work/proj2") // nil when Claude Code never ran there
	if err != nil {
		return err
	}
	if st != nil {
		p.line(fmt.Sprintf("dir=%s trusted=%t last session cost=$%.2f models=%d", st.Dir, st.TrustAccepted, st.Last.Cost, len(st.Last.ModelUsage)))
	}
	if never, err := a.ProjectState(h, "/never/used"); err == nil && never == nil {
		p.line("/never/used: no entry")
	}
	cached, err := a.CachedUsageBody(h) // raw body; parse it with claude.ParseUsage
	if err != nil {
		return err
	}
	if cached != nil {
		p.line(fmt.Sprintf("cached usage fetched %s, %d bytes", cached.FetchedAt.UTC().Format(time.RFC3339), len(cached.Body)))
	}
	return nil
}
```

## Step 3 — List config backups

Claude Code saves copies of `.claude.json` under `<home>/backups/`. `a.ConfigBackups(h)` lists them, newest name last; use it to offer a restore after a bad write.

<!-- snippet: examples/claudesettings/main.go#backups -->
```go
// showConfigBackups lists the .claude.json backups Claude Code keeps.
func showConfigBackups(p printer, host agentio.Host) error {
	p.section("config backups")
	a := claude.New(host)
	backups, err := a.ConfigBackups(a.DefaultHome()) // newest name last
	if err != nil {
		return err
	}
	for _, b := range backups {
		p.line(fmt.Sprintf("backup %s", b))
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudesettings
```

The sections this page covers:

```text
== global config
path=~/.claude.json install method="native" startups=1072
projects=3 user MCP servers=2 unmodelled keys=58
account email=[redacted] org type=claude_max billing=stripe_subscription
ParseGlobalConfig: first start version=2.1.296

== project state
dir=/work/proj2 trusted=false last session cost=$6.07 models=2
/never/used: no entry
cached usage fetched 2026-10-28T08:38:37Z, 3787 bytes

== config backups
backup ~/.claude/backups/.claude.json.backup.1794000000000
```

## Pitfalls

- **A missing file is not an error.** `GlobalConfig` returns the zero value with `Path` set for a fresh home; check `NumStartups` or `OAuthAccount` rather than the error to tell "never used" apart.
- The project key is the absolute folder exactly as Claude Code saw it. Pass a cleaned absolute path; a trailing slash or a symlinked path finds no entry.
- `Account` present does not mean logged in: the login lives in the keychain or `.credentials.json`. Use `a.Identity(h)` for "is someone logged in" (see [Claude account](account.md)).
- Personal values print redacted in `fmt`, logs and JSON. `Reveal()` exists, but anything you reveal is yours to protect.
- The library never writes `.claude.json`. Changes to MCP servers or project choices go through vendor commands (see [Claude MCP servers](mcp.md)); the [Claude writer](writer.md) edits only settings files.

## Related

- [Claude account](account.md), [Claude usage](usage.md), [Claude MCP servers](mcp.md), [Claude paths](paths.md)
- Neutral view: [Install and identity](../capabilities/install-identity.md), [Secrets and personal values](../core/secrets.md)
- Example: [`examples/claudesettings`](../../examples/claudesettings/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
