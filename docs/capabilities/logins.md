# Logins and tokens

Three capabilities cover signing an agent in. `LoginCommander` builds the vendor's interactive login and logout commands as data for you to run in a terminal. `TokenLogin` maps a token the user gives you (a long-lived plan token or an API key) onto the vendor's environment variables, so one launch uses that token instead of the home's stored login. `TokenProber` reads a token's plan usage from one minimal API request. Reach them with `agentio.LoginCommanderOf`, `agentio.TokenLoginOf` and `agentio.TokenProberOf`.

## Before you start

- Modules: `github.com/ubgo/agentio` plus the agents.
- Login commands and token mapping are Pure: nothing is read or run.
- The probe needs `Host.HTTP`; in tests, point the Claude agent at an `httptest` server with `WithBaseURL`.
- Tokens travel as `agentio.Token` inside `agentio.TokenCredential{Kind, Token}`. `Token` prints, formats, logs and marshals as `[redacted]`; only `Reveal()` gives the value.

[`examples/usage`](../../examples/usage/) runs every step below against an in-process fake API with sample token shapes (never real values).

## Step 1 — Build a login command

`LoginCommand(home, flow)` returns the `Command` that signs that home in with a `LoginFlow`: `browser`, `device`, `api-key-stdin` or `access-token-stdin`. A flow the vendor does not have returns `ErrUnsupported`. `LogoutCommand(home)` returns the logout command. The command carries the home rule (the home variable set, or removed for the default home), so it logs in the home you asked for.

<!-- snippet: examples/usage/main.go#login -->
```go
// printLoginCommands builds the login command of every flow, and the logout
// command, for each default home. They are data: the caller runs them in a
// terminal. A flow the vendor does not have is ErrUnsupported.
func printLoginCommands(w io.Writer, agents []agentio.Agent) {
	for _, a := range agents {
		lc, ok := agentio.LoginCommanderOf(a)
		if !ok {
			continue
		}
		home := a.DefaultHome()
		for _, f := range agentio.LoginFlowValues() {
			cmd, err := lc.LoginCommand(home, f)
			if errors.Is(err, agentio.ErrUnsupported) {
				fmt.Fprintf(w, "%s %-19s unsupported\n", a.ID(), f)
				continue
			}
			fmt.Fprintf(w, "%s %-19s %s\n", a.ID(), f, cmd)
		}
		fmt.Fprintf(w, "%s %-19s %s\n", a.ID(), "logout", lc.LogoutCommand(home))
	}
}
```

These commands are interactive: run them attached to the user's terminal (`os/exec` with the terminal's stdin, stdout and stderr), not through `Host.Exec`, which collects output.

## Step 2 — Launch with a token

`TokenLogin` has four Pure methods: `TokenKinds()` (supported kinds, default first), `DetectTokenKind(t)` (recognises a kind from the token's shape), `TokenEnv(cred)` (the env changes that make the CLI use the token) and `TokenSetupCommand(home)` (the vendor command that mints a long-lived token, such as `claude setup-token`). Usually you do not call `TokenEnv` yourself: set `LaunchRequest.Credential` and `Launch` applies it, after scrubbing the vendor's other credential variables so they cannot win.

<!-- snippet: examples/usage/main.go#token -->
```go
// printTokenLogin recognises token shapes, maps a token onto the vendor's
// variables and builds a launch that uses it instead of the home's login.
// The token lives only in Command.Env; argv and printed values are redacted.
func printTokenLogin(w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		tl, ok := agentio.TokenLoginOf(a)
		if !ok {
			reason, _ := agentio.Unsupported(a, agentio.CapTokenLogin)
			fmt.Fprintf(w, "%s: no token login (%s)\n", a.ID(), reason)
			continue
		}
		fmt.Fprintf(w, "%s kinds: %v\n", a.ID(), tl.TokenKinds())
		for _, s := range []string{sampleOAuthToken, sampleAPIKey, sampleRefresh} {
			kind, ok := tl.DetectTokenKind(agentio.NewToken(s))
			fmt.Fprintf(w, "  %.13s... kind=%s usable=%t\n", s, kind, ok)
		}
		cred := agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken(sampleOAuthToken)}
		env, err := tl.TokenEnv(cred)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "  token env: %v\n", env)
		cmd, err := a.Launch(a.DefaultHome().Target(), agentio.LaunchRequest{Prompt: "hello", Credential: &cred})
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "  launch: %s\n", cmd)
		fmt.Fprintf(w, "  secret env names: %v\n", cmd.SecretEnvNames())
		fmt.Fprintf(w, "  token on argv: %t\n", strings.Contains(cmd.String(), sampleOAuthToken))
		if setup, ok := tl.TokenSetupCommand(a.DefaultHome()); ok {
			fmt.Fprintf(w, "  setup: %s\n", setup)
		}
	}
	return nil
}
```

`TokenKind` is `oauth-token` (a long-lived plan token: spends the plan quota, never refreshed), `api-key` (billed per token) or `unknown`. `DetectTokenKind` reports false for shapes that must not be launched with, such as a refresh token. `agentio.CredentialSource` (`file`, `keychain`, `keyring`, `env`, `none`) tells you where a home's own login lives; [Install, version and identity](install-identity.md#step-3--read-the-signed-in-identity) reads it.

## Step 3 — Probe a token

`TokenUsage(ctx, cred)` sends the smallest valid request with the token and reads the plan windows from the response's rate-limit headers. It costs `quota read`: it spends a few tokens of the account's plan. A 429 that carries the headers is parsed as usage at its limit. An API key has no plan windows and is never sent; the call returns `ErrBilledPerToken`.

<!-- snippet: examples/usage/main.go#probe -->
```go
// printTokenProbe reads a token's usage from one minimal request and its
// rate-limit headers. It spends a little quota: run it once on demand,
// never in a loop. An API key has no plan windows and is never probed.
func printTokenProbe(ctx context.Context, w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		tp, ok := agentio.TokenProberOf(a)
		if !ok {
			continue
		}
		oauth := agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken(sampleOAuthToken)}
		u, err := tp.TokenUsage(ctx, oauth)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s oauth token: max %.0f%% source=%s\n", a.ID(), u.MaxPercent(), u.Source)
		printWindows(w, u)
		key := agentio.TokenCredential{Kind: agentio.TokenAPIKey, Token: agentio.NewToken(sampleAPIKey)}
		if _, err := tp.TokenUsage(ctx, key); errors.Is(err, agentio.ErrBilledPerToken) {
			fmt.Fprintf(w, "%s api key: billed per token, not probed\n", a.ID())
		}
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./usage
```

The last three sections of the output:

```text
== login commands ==
claude browser             claude auth login --claudeai
claude device              unsupported
claude api-key-stdin       unsupported
claude access-token-stdin  unsupported
claude logout              claude auth logout
codex browser             codex login
codex device              codex login --device-auth
codex api-key-stdin       codex login --with-api-key
codex access-token-stdin  codex login --with-access-token
codex logout              codex logout
== token login ==
claude kinds: [oauth-token api-key]
  sk-ant-oat01-... kind=oauth-token usable=true
  sk-ant-api03-... kind=api-key usable=true
  sk-ant-ort01-... kind=unknown usable=false
  token env: [CLAUDE_CODE_OAUTH_TOKEN=[redacted]]
  launch: claude -- hello
  secret env names: [CLAUDE_CODE_OAUTH_TOKEN]
  token on argv: false
  setup: claude setup-token
codex: no token login (Codex has no token login: it logs in with `codex login` (ChatGPT, API key or access token on stdin))
== token probe ==
claude oauth token: max 37% source=probe
  Current session  short  37.0% resets 2026-11-13T17:00:00Z
  All models       week   21.0% resets 2026-11-13T17:00:00Z
claude api key: billed per token, not probed
```

## Pitfalls

- **Never put a token on argv.** `Command.String()` and `Args` never hold one; the token lives only in `Command.Env`. Print commands with `String()` or `Redacted()`, never by dumping `Env`. `Command.SecretEnvNames()` lists the variables that carry a secret.
- **Never loop the probe.** `TokenUsage` spends quota on every call. Run it once when the user asks, and prefer cached or live usage for status displays.
- **Codex has neither `TokenLogin` nor `TokenProber`.** Its token logins are `codex login --with-api-key` / `--with-access-token`, which read the secret from stdin and store it in the home. `agentio.Unsupported` says so.
- **Claude's login flows other than `browser` are `ErrUnsupported`.** Claude Code logs in through the browser; tokens go through `TokenEnv` instead.
- **A refresh token is not usable.** `DetectTokenKind` reports false for it; the vendor-level `claude.Agent.DetectTokenKind` returns `ErrUnusableToken` with the reason.
- **The home rule always wins.** `Launch` scrubs inherited credential variables first, then applies `Target.ExtraEnv`, then (token login) the scrub again and the token, then the home variable last, so neither a profile nor a token can redirect a launch to another home.

## Related

- [Usage and subscription](usage.md): cached and live usage for a home's own login.
- [Doors: Runner, Prober and Writer](doors.md#step-2--spend-quota-through-a-prober): the vendor Prober door behind `TokenProberOf`.
- [Launch commands](../core/launch.md), [Secrets and personal values](../core/secrets.md).
- Vendor pages: [Claude credential](../claude/credential.md), [Claude launch](../claude/launch.md), [Claude usage](../claude/usage.md#step-4--probe-a-tokens-usage), [Codex launch](../codex/launch.md).
- [Choosing guides](../choosing/README.md). Example: [`examples/usage`](../../examples/usage/). Feature index: [features](../reference/features.md).
