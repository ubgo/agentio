# The pi agent

`pi.New` gives you the pi coding agent (npm `@earendil-works/pi-coding-agent`, formerly `@mariozechner/pi-coding-agent`) on one `agentio.Host`: its install, agent dirs, settings, sessions, instruction files, MCP servers, skills and prompt templates, plus the commands that launch or resume it. Use the `pi` module when your code knows it talks to pi; use `Neutral()` when the same code loops over Claude Code, Codex and pi.

pi differs from the other built-ins in three ways that shape this API:
- **No permission prompts.** pi "does not ask for approval before every tool call", so its only honest autonomy levels are `full` (no flag) and `read-only` (the tool allowlist pi documents as read-only). `ask` and `accept-edits` are refused with "pi has no permission modes; it always runs without prompts".
- **No plan or quota.** Usage windows are unsupported with that reason; pi records each message's tokens and cost in the session file, which [`SessionUsage`](ds:block?id=pi-agent-sessionusage-r6p3u6g7) sums.
- **It authenticates by provider variables.** `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `GEMINI_API_KEY` and the rest are pi's login mechanism, and a stored `auth.json` credential already wins over them, so a launch never scrubs any of them.

## Before you start

- Add the module: `go get github.com/ubgo/agentio/pi` (stdlib only, plus the root module).
- Build a `Host` (`agentio.HostFromOS()` in real programs; see [Host](../core/host.md)).
- An agent dir (a home) is `~/.pi/agent` by default. [`PI_CODING_AGENT_DIR`](ds:block?id=pi-spec-r6p1s2d3) names another agent dir **itself** (pi adds no `agent` suffix and creates no `~/.pi`). A dir counts as a home when it holds `auth.json`, `models-store.json`, `settings.json` or `sessions/`; pi writes the first two on every start, even for `--help`.

The snippets come from [`examples/pi`](../../examples/pi/), which runs on a temp copy of the 1.5.2 fixture agent dir.

## Step 1 — Install and version

[`Install`](ds:block?id=pi-agent-install-r6p2i4f5) finds `pi` on `PATH` and reads the version from the `package.json` of either npm package above the resolved binary. It never runs pi; `Runner().BinaryVersion` runs `pi --version` when you need the binary's own answer. The tested range is `pi.TestedMin`..`pi.TestedMax` (1.5.2), and `features.tsv` records when each flag appeared (from pi's CHANGELOG), so `LaunchOptions.Args` refuses a flag the installed version provably lacks.

<!-- snippet: examples/pi/main.go#install -->
```go
// install reads pi's install from disk: the PATH hit, the file it links
// to, and the version from the npm package.json above it. pi never runs.
func install(p printer, a *pi.Agent) error {
	p.line("== install ==")
	in, err := a.Install()
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("install: %s -> %s method=%s version=%s tested=%s..%s", in.Path, in.RealPath, in.Method, in.Version, pi.TestedMin, pi.TestedMax))
	return nil
}
```

## Step 2 — Homes

| Call | Returns | Cost |
|---|---|---|
| `a.DefaultHome()` | `~/.pi/agent` (used with `PI_CODING_AGENT_DIR` absent) | Pure |
| `a.HomeAt(dir)` | the agent dir at any path | Pure |
| `a.HomeFromEnv()` | the agent dir `PI_CODING_AGENT_DIR` names; `false` when unset or empty | Pure |
| `a.Homes()` | the default home when it is one, then the env home | Disk read |
| `a.Paths(h)` | every file of the agent dir (settings, auth, mcp, trust, sessions, skills, prompts) | Pure |

pi has no naming convention for extra agent dirs, so `Homes` globs nothing.

<!-- snippet: examples/pi/main.go#homes -->
```go
// homes lists the agent dirs: ~/.pi/agent, plus the one PI_CODING_AGENT_DIR
// names (it names the agent dir itself).
func homes(p printer, a *pi.Agent) error {
	p.line("== homes ==")
	hs, err := a.Homes()
	if err != nil {
		return err
	}
	for _, h := range hs {
		p.line(fmt.Sprintf("home: %s default=%t dir=%s", h.Name(), h.IsDefault(), h.Dir()))
	}
	return nil
}
```

## Step 3 — Sessions

pi keeps one JSONL file per session at `<agent dir>/sessions/--<cwd>--/<timestamp>_<id>.jsonl`. The folder name is the cwd without its leading slash, with every `/`, `\` and `:` turned into `-` (`EncodeSessionDir`: `/work/proj` is `--work-proj--`, `C:\work\x` is `--C--work-x--`). `PI_CODING_AGENT_SESSION_DIR` or the `sessionDir` setting moves sessions to one flat folder, filtered by the header's cwd (`SessionStore`).

[`Sessions`](ds:block?id=pi-agent-sessions-r6p4c8h9) reads only the header line, a head window for the first prompt and a tail window for the name, last prompt and model, newest first by modification time (the order pi's `--continue` uses). `Turns` reads messages incrementally from an offset; `SessionUsage` sums every assistant message's tokens and cost, reading the whole file once.

<!-- snippet: examples/pi/main.go#sessions -->
```go
// sessions lists the home's sessions (header, head and tail windows only),
// then reads one session's turns and token cost.
func sessions(p printer, a *pi.Agent) error {
	p.line("== sessions ==")
	h := a.DefaultHome()
	ss, err := a.Sessions(h, agentio.ConversationQuery{})
	if err != nil {
		return err
	}
	for _, s := range ss {
		p.line(fmt.Sprintf("session %s %q cwd=%s model=%s v%d", s.ID[:8], s.Title(), s.Cwd, s.Model, s.Version))
		p.line(fmt.Sprintf("  first prompt %q, last prompt %q", s.FirstPrompt, s.LastPrompt))
		var user, assistant int
		if _, err := a.Turns(h, s.ID, 0, func(t agentio.Turn) error {
			if t.Role == agentio.RoleUser {
				user++
			} else {
				assistant++
			}
			return nil
		}); err != nil {
			return err
		}
		p.line(fmt.Sprintf("  turns: %d user, %d assistant", user, assistant))
		u, err := a.SessionUsage(h, s.ID)
		if err != nil {
			return err
		}
		p.line(fmt.Sprintf("  usage: %d messages, %d input and %d output tokens, $%.4f", u.Messages, u.Tokens.Input, u.Tokens.Output, u.CostUSD))
	}
	return nil
}
```

## Step 4 — Settings, trust and defaults

`settings.json` (global in the agent dir, project in `.pi/settings.json`) has about 80 keys and pi accepts unknown ones. `pi.Settings` types the keys a launcher needs (`defaultProvider`, `defaultModel`, `defaultThinkingLevel`, `defaultTools`, `sessionDir`, `cacheWarming`, the resource lists) and keeps every other key raw in `Unknown`; `MarshalJSON` writes them back, so a round trip loses nothing. A modelled key whose value changed type moves to `Unknown` with a `type-changed` warning instead of failing the file. `SettingsKeys` lists the typed keys.

`ProjectTrust` follows pi's order: the nearest saved decision in `trust.json` walking up from the canonical dir, then the global `defaultProjectTrust`. Project settings, `.pi/mcp.json`, `.pi/skills` and `.pi/SYSTEM.md` apply only to a trusted project; `Defaults` and the listings honour that.

<!-- snippet: examples/pi/main.go#settings -->
```go
// settings parses a settings.json: typed keys come out typed, the rest is
// kept raw and written back, so a round trip loses nothing.
func settings(p printer, a *pi.Agent) error {
	p.line("== settings ==")
	h := a.DefaultHome()
	if err := os.WriteFile(a.Paths(h).Settings, []byte(exampleSettings), filePerm); err != nil {
		return err
	}
	s, found, warns, err := a.ReadSettings(h)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("settings: found=%t provider=%s model=%s unknown=%v warnings=%d", found, s.DefaultProvider.Or(""), s.DefaultModel.Or(""), s.Unknown.Keys(), len(warns)))
	out, err := json.Marshal(s)
	if err != nil {
		return err
	}
	p.line("round trip: " + string(out))
	d, err := a.Defaults(h, "")
	if err != nil {
		return err
	}
	p.line("default model: " + d.Model)
	return nil
}
```

## Step 5 — Launch and resume

The neutral `Launch` maps a request onto `pi.LaunchOptions`:

| Request | pi argv |
|---|---|
| `Prompt` | `-- <prompt>` last, so a prompt starting with `-` is never a flag; pi stays interactive after answering |
| `Model` | `--model <pattern>` (`provider/id`, optional `:<thinking>`) |
| `Headless` | `--mode json` (JSONL events on stdout, then exit) |
| `Autonomy` read-only | `--tools read,grep,find,ls --no-mcp --no-extensions` |
| `Autonomy` full | nothing (needs `Target.AllowDangerous`) |
| `Autonomy` ask, accept-edits | refused |
| `ResumeID` | `--session <id or path>` |
| `AddDirs` | refused: pi has no such flag; its tools reach any path the process can |
| `Credential` | the provider's key variable (`TokenCredential.Provider` is required) |

Resume by the session's `Path`, or run in its recorded cwd: an id that pi finds only under another folder makes it ask whether to fork the session. Use `pi.LaunchOptions` directly for the flags the neutral request does not carry (`--thinking`, `--session-id`, `--fork`, `--name`, `--exclude-tools`, `--append-system-prompt`, `--approve`, `--offline`).

<!-- snippet: examples/pi/main.go#launch -->
```go
// launch builds commands through the neutral view: a new interactive run
// with a prompt, a read-only headless run, a resume by session file, a
// provider token login, and the levels pi refuses.
func launch(p printer, a *pi.Agent) error {
	p.line("== launch ==")
	n := a.Neutral()
	def := n.DefaultHome().Target()
	cmd, err := n.Launch(def, agentio.LaunchRequest{Prompt: "fix the failing tests"})
	if err != nil {
		return err
	}
	p.line("new: " + cmd.String())
	cmd, err = n.Launch(def, agentio.LaunchRequest{Prompt: "review src/", Autonomy: agentio.AutonomyReadOnly, Headless: true})
	if err != nil {
		return err
	}
	p.line("read-only headless: " + cmd.String())
	ss, err := a.Sessions(a.DefaultHome(), agentio.ConversationQuery{Limit: 1})
	if err != nil || len(ss) == 0 {
		return fmt.Errorf("no session to resume: %w", err)
	}
	cmd, err = n.Launch(def, agentio.LaunchRequest{ResumeID: ss[0].Path, Dir: ss[0].Cwd})
	if err != nil {
		return err
	}
	p.line("resume: " + cmd.String())
	cred := &agentio.TokenCredential{Kind: agentio.TokenAPIKey, Provider: "anthropic", Token: agentio.NewToken(exampleKey)}
	cmd, err = n.Launch(def, agentio.LaunchRequest{Credential: cred})
	if err != nil {
		return err
	}
	red := cmd.Redacted()
	anth, _ := agentio.EnvLookup(red.Env, "ANTHROPIC_API_KEY")
	openai, _ := agentio.EnvLookup(red.Env, "OPENAI_API_KEY")
	p.line(fmt.Sprintf("token login: %s ANTHROPIC_API_KEY=%s OPENAI_API_KEY kept=%t", cmd, anth, openai != ""))
	for _, level := range []agentio.Autonomy{agentio.AutonomyAsk, agentio.AutonomyAcceptEdits} {
		_, err := n.Launch(def, agentio.LaunchRequest{Autonomy: level})
		p.line(fmt.Sprintf("%s: %v", level, err))
	}
	return nil
}
```

## Step 6 — Logins, doctor and capabilities

[`Logins`](ds:block?id=pi-agent-logins-r6p6o4k5) describes `auth.json` by provider and type (`api_key` or `oauth`), whether a key is a `!command`, and the OAuth expiry; it never returns a key, access or refresh value. The OAuth field names come from pi's source (no logged-in recording yet). `Identity` reports a login when `auth.json` has an entry or a provider key variable is set. The doctor checks the binary, home, tested version, `settings.json`, logins, and notes that `cacheWarming: idle` (pi's default) spends tokens between runs.

<!-- snippet: examples/pi/main.go#capabilities -->
```go
// capabilities prints what pi supports and why the rest is missing.
func capabilities(p printer, a *pi.Agent) {
	p.line("== capabilities ==")
	n := a.Neutral()
	p.line(fmt.Sprintf("supported: %v", agentio.Supported(n)))
	for _, c := range agentio.CapabilityValues() {
		if reason, ok := agentio.Unsupported(n, c); ok {
			p.line(fmt.Sprintf("lacks %s: %s", c, reason))
		}
	}
}
```

## Try it

```sh
cd examples && go run ./pi
```

The full output is in [`examples/pi/README.md`](../../examples/pi/README.md).

## Pitfalls

- **Never call `pi auth check` without `--no-refresh`.** It refreshes OAuth tokens and rewrites `auth.json`; `pi auth print-bearer-token` refreshes too. This module never builds either.
- **`pi` in the process table hides its argv.** pi sets its process title to `pi`, so on Linux the command line is just `pi` and the executable is `node`; only the basename matches.
- **A relative session dir is refused.** pi resolves `PI_CODING_AGENT_SESSION_DIR` or `sessionDir` against its own working directory, which a reader cannot know.
- **Read-only is a tool allowlist, not a sandbox.** pi runs with the process's permissions; `--tools read,grep,find,ls` only removes the tools that write. Isolate pi in a container when that matters.
- **Every provider has its own key variable.** A token login without `TokenCredential.Provider` is refused rather than guessed.

## Related

- [Targets and commands](../core/launch.md), [Homes](../core/homes.md), [Host](../core/host.md)
- Example: [`examples/pi`](../../examples/pi/)
