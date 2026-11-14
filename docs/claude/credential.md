# Claude credentials and tokens

The `claude` module reads the OAuth login Claude Code stored for a home, tells a setup-token from an API key, maps a token onto the variables Claude Code reads, and builds the `claude auth` commands. Use it to show "logged in until …" for each home, to check a pasted token before you launch with it, or to send someone to the right login command. [Nothing here refreshes a token, writes a login or prints a token value](ds:block?id=claude-agent-usablecredential-e8k4sjep).

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- A `Host` with `UserHome`, `User` (the OS login: [keychain lookups name it](ds:block?id=claude-agent-keychainaccount-hv9hcdts)) and `Secrets` (the keychain, or a fake). `HostFromOS()` gives the real ones; the example builds one by hand.
- A Claude home with a login: `<home>/.credentials.json` (the only store on Linux and Windows, `%USERPROFILE%\.claude\.credentials.json` there) or a keychain entry (macOS). On a Host whose `GOOS` is linux or windows the keychain is not consulted at all.

The snippets come from [`examples/claudeaccount`](../../examples/claudeaccount/). It reads the checked-in 2.1.325 fixture: the default home `~/.claude` has a `.credentials.json`; the work home `~/.claude-work` gets its login from an in-memory keychain holding a placeholder token.

## Step 1 — Read the credential

[`a.Credential(h)`](ds:block?id=claude-agent-credentialcontext-tcjbfq3k) (or `CredentialContext` with a context for the keychain call) looks in both stores and returns the login with the latest `ExpiresAt`: a refreshed token can sit in one store while the other keeps an old copy. The result is a `claude.Credential`: `Source` (`claude.SourceFile` or `claude.SourceKeychain`), `Location` (file path or keychain service name), `ExpiresAt`, `Scopes`, `SubscriptionType`, `RateLimitTier` and `AccessToken`, an `agentio.Token` that prints as [`[redacted]`](ds:block?id=token-3nk7gtub) in every format. [The refresh token is never decoded](ds:block?id=claude-wirecredentials-fx9rcyeb).

[`claude.KeychainServices(dir, isDefault)`](ds:block?id=claude-keychainservices-vae8mc8e) lists the keychain service names a home may use, most specific first: `Claude Code-credentials-<hash>` (the hash is over the exact `CLAUDE_CONFIG_DIR` string), then for the default home the bare legacy `Claude Code-credentials`. Each lookup names `Host.User` as the account first and [falls back to a service-only lookup](ds:block?id=claude-agent-keychainget-uj32ktdh), because stale same-service entries with another account exist. [`Credential.Expired(now)`](ds:block?id=claude-credential-expired-k3dc67ku) is true only for a known expiry at or before `now`. [`a.Identity(h)`](ds:block?id=claude-agent-identity-5vqz5ezp) returns the same facts without the token as a `claude.CredentialSummary` (see [Claude account and identity](account.md)).

<!-- snippet: examples/claudeaccount/main.go#credential -->
```go
// readCredential prints where a home's login lives and when it expires. The
// access token itself is an agentio.Token: printing it shows [redacted].
func readCredential(ctx context.Context, w io.Writer, a *claude.Agent, h claude.Home) error {
	c, err := a.CredentialContext(ctx, h)
	if errors.Is(err, agentio.ErrNotLoggedIn) {
		fmt.Fprintf(w, "%s: not logged in\n", h.Name())
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s: login in %s (%s)\n", h.Name(), c.Source, location(a, c))
	fmt.Fprintf(w, "  expires %s, expired now: %t\n", c.ExpiresAt.Format(time.RFC3339), c.Expired(a.Host().Now()))
	fmt.Fprintf(w, "  plan %s / %s, %d scopes, token %v\n", c.SubscriptionType, c.RateLimitTier, len(c.Scopes), c.AccessToken)
	fmt.Fprintf(w, "  keychain names tried: %d\n", len(claude.KeychainServices(h.Dir(), h.IsDefault())))
	return nil
}

// location shows a file path under ~ and a keychain service name with its
// per-directory hash elided, so the output is the same on every machine.
func location(a *claude.Agent, c claude.Credential) string {
	if c.Source == claude.SourceFile {
		return tilde(a, c.Location)
	}
	if i := strings.LastIndex(c.Location, "-"); i > 0 {
		return c.Location[:i] + "-<hash>"
	}
	return c.Location
}
```

## Step 2 — Detect a token's kind

[`a.TokenKinds()`](ds:block?id=claude-agent-tokenkinds-rgwudtpt) lists the token logins Claude supports, OAuth token first (so a plain login spends plan quota rather than an API key): `agentio.TokenOAuthToken` (what `claude setup-token` prints) and `agentio.TokenAPIKey`. [`a.DetectTokenKind(t)`](ds:block?id=claude-agent-detecttokenkind-8826wbv6) classifies a token by its prefix and returns `(kind, known, err)`: `known` is false for an unrecognised prefix, and an OAuth refresh token or an Admin API key is refused with an error wrapping `claude.ErrUnusableToken` (no login can run Claude with them). [`a.TokenLifetime(kind)`](ds:block?id=claude-agent-tokenlifetime-x3rp4mqq) is [`claude.SetupTokenLifetime`](ds:block?id=claude-setuptokenlifetime-rwu3ayz3) (one year, the believed validity of a setup-token) for an OAuth token and 0 ("until revoked") for an API key.

<!-- snippet: examples/claudeaccount/main.go#kinds -->
```go
// detectKinds classifies tokens by prefix. The samples are placeholders; a
// refresh token or Admin key is refused with ErrUnusableToken.
func detectKinds(w io.Writer, a *claude.Agent) error {
	fmt.Fprintf(w, "token logins: %v\n", a.TokenKinds())
	for _, sample := range []string{"sk-ant-oat01-EXAMPLE", "sk-ant-api03-EXAMPLE", "sk-ant-ort01-EXAMPLE", "sk-ant-admin01-EXAMPLE", "not-a-claude-token"} {
		kind, known, err := a.DetectTokenKind(agentio.NewToken(sample))
		switch {
		case errors.Is(err, claude.ErrUnusableToken):
			fmt.Fprintf(w, "  %-14s refused: %v\n", sample[:min(len(sample), 14)], err)
		case err != nil:
			return err
		case !known:
			fmt.Fprintf(w, "  %-14s unknown prefix\n", sample[:min(len(sample), 14)])
		default:
			fmt.Fprintf(w, "  %-14s %s, valid for %s\n", sample[:min(len(sample), 14)], kind, lifetime(a.TokenLifetime(kind)))
		}
	}
	return nil
}

// lifetime renders a token lifetime; 0 means until revoked.
func lifetime(d time.Duration) string {
	if d == 0 {
		return "until revoked"
	}
	return d.String()
}
```

## Step 3 — Build a token environment

[`a.TokenEnv(cred)`](ds:block?id=claude-agent-tokenenv-tatsnfw8) maps a `agentio.TokenCredential` onto Claude's variables: an OAuth token goes to `CLAUDE_CODE_OAUTH_TOKEN` (`claude.OAuthTokenEnv`), an API key to `ANTHROPIC_API_KEY` (`claude.APIKeyEnv`). Each change is a secret `agentio.EnvChange`: its `String()` and JSON show `[redacted]`, and [its `Value()` is empty](ds:block?id=envchange-value-43pazd3a). You rarely call it yourself: a neutral launch with `LaunchRequest.Credential` set calls it and [scrubs the other credential variables](ds:block?id=core-envchanges-f8hrn3we) so the token is the login used (see [Logins and tokens](../capabilities/logins.md#step-2--launch-with-a-token)). Another kind returns an `agentio.ErrUnsupported` error.

<!-- snippet: examples/claudeaccount/main.go#tokenenv -->
```go
// tokenEnv shows which variable carries each token kind. The value is a
// secret EnvChange: String() prints [redacted], never the token.
func tokenEnv(w io.Writer, a *claude.Agent) error {
	for _, kind := range a.TokenKinds() {
		changes, err := a.TokenEnv(agentio.TokenCredential{Kind: kind, Token: agentio.NewToken("sk-ant-EXAMPLE")})
		if err != nil {
			return err
		}
		for _, c := range changes {
			fmt.Fprintf(w, "token env for %s: %s\n", kind, c)
		}
	}
	return nil
}
```

## Step 4 — Build login commands

[`a.LoginCommand(h, mode, email)`](ds:block?id=claude-agent-logincommand-dnxkn4ar) builds `claude auth login` with `--claudeai` (`claude.LoginClaudeAI`, the default), `--console` (`claude.LoginConsole`) or `--sso` (`claude.LoginSSO`), plus `--email` when given. Like every launch, it scrubs `CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY` and [`ANTHROPIC_AUTH_TOKEN`](ds:block?id=claude-credentialenv-kqm45qxy) from the child env, so the new login is the one Claude uses afterwards. `a.LogoutCommand(h)` and [`a.SetupTokenCommand(h)`](ds:block?id=claude-agent-setuptokencommand-xces3g4z) build `claude auth logout` and `claude setup-token`. All three are Pure: they return an `agentio.Command` you run yourself (they open a browser or ask for input). [For the default home `CLAUDE_CONFIG_DIR` is removed from the env](ds:block?id=homeenvchange-59nx5j7k); for any other home it is set to the home dir.

<!-- snippet: examples/claudeaccount/main.go#login -->
```go
// loginCommands builds (never runs) the login, logout and setup-token
// commands. A non-default home carries CLAUDE_CONFIG_DIR in Command.Env.
func loginCommands(w io.Writer, a *claude.Agent, work claude.Home) {
	for _, c := range []agentio.Command{
		a.LoginCommand(a.DefaultHome(), claude.LoginClaudeAI, ""),
		a.LoginCommand(work, claude.LoginConsole, "user@example.com"),
		a.LogoutCommand(work),
		a.SetupTokenCommand(a.DefaultHome()),
	} {
		_, homeSet := agentio.EnvLookup(c.Env, claude.ConfigDirEnv)
		fmt.Fprintf(w, "command: %-50s %s set: %t\n", c.String(), claude.ConfigDirEnv, homeSet)
	}
}
```

## Try it

```sh
cd examples && go run ./claudeaccount
```

The lines for this page (the rest are on [Claude account and identity](account.md)):

```text
personal: login in file (~/.claude/.credentials.json)
  expires 2026-11-13T20:56:01Z, expired now: false
  plan max / default_claude_max_20x, 6 scopes, token [redacted]
  keychain names tried: 2
work: login in keychain (Claude Code-credentials-<hash>)
  expires 2026-11-13T18:00:00Z, expired now: false
  plan pro / default_claude_pro, 2 scopes, token [redacted]
  keychain names tried: 1
token logins: [oauth-token api-key]
  sk-ant-oat01-E oauth-token, valid for 8760h0m0s
  sk-ant-api03-E api-key, valid for until revoked
  sk-ant-ort01-E refused: claude: this token cannot run Claude: it is an OAuth refresh token; use the token `claude setup-token` prints
  sk-ant-admin01 refused: claude: this token cannot run Claude: it is an Admin API key
  not-a-claude-t unknown prefix
token env for oauth-token: CLAUDE_CODE_OAUTH_TOKEN=[redacted]
token env for api-key: ANTHROPIC_API_KEY=[redacted]
command: claude auth login --claudeai                       CLAUDE_CONFIG_DIR set: false
command: claude auth login --console --email user@example.com CLAUDE_CONFIG_DIR set: true
command: claude auth logout                                 CLAUDE_CONFIG_DIR set: true
command: claude setup-token                                 CLAUDE_CONFIG_DIR set: false
```

## Pitfalls

- **No login is [`agentio.ErrNotLoggedIn`, joined with the reasons](ds:block?id=claude-agent-credentialcontext-tcjbfq3k).** A missing file and no keychain entry give `ErrNotLoggedIn`; an unreadable file or a keychain error is joined into the same error. Test with `errors.Is`, not equality.
- **The keychain hash is over the exact directory string.** [`KeychainServices` cleans the path and hashes it](ds:block?id=claude-keychainservices-vae8mc8e); a home reached through a different spelling (a symlinked path) maps to another service name, the same way Claude Code itself does. Pointing `CLAUDE_CONFIG_DIR` at the default home makes Claude Code use the hashed name and look logged out (the [doctor](doctor.md) reports this as [`home-env-default`](ds:block?id=claude-agent-doctor-v2kafdh7)).
- **Unknown expiry is never expired.** [`Expired` returns false when `ExpiresAt` is nil](ds:block?id=claude-credential-expired-k3dc67ku); the server's 401 decides.
- **An expired login is not refreshed.** Refreshing rotates the refresh token and logs Claude Code out, so the library never does it. [Live usage refuses an expired login with `agentio.ErrExpired`](ds:block?id=claude-agent-usablecredential-e8k4sjep); run `claude` once in that home to refresh it.
- **On Linux and Windows the keychain is not consulted.** On a Host whose `GOOS` is linux or windows Claude never asks `Host.Secrets`, and on any OS a `SecretStore` that answers `agentio.ErrUnsupported` [reads as "no entry"](ds:block?id=claude-agent-keychainget-uj32ktdh); `.credentials.json` is the store there.
- **`TokenLifetime` for a setup-token is a belief, not a fact.** [One year is not verified against the server](ds:block?id=claude-setuptokenlifetime-rwu3ayz3); treat it as a hint for "renew soon" messages.
- **[`DetectTokenKind` trims spaces but does not validate](ds:block?id=claude-agent-detecttokenkind-8826wbv6).** A known prefix says what kind a token claims to be, not that it works; only a request (for example [a probe](usage.md#step-4--probe-a-tokens-usage)) proves that.

## Related

- [Claude account and identity](account.md), [Claude plan usage](usage.md), [Claude launch and resume](launch.md), [Claude doctor](doctor.md)
- Neutral view: [Logins and tokens](../capabilities/logins.md), [Install and identity](../capabilities/install-identity.md#step-3--read-the-signed-in-identity)
- Example: [`examples/claudeaccount`](../../examples/claudeaccount/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
