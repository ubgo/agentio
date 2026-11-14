# Codex auth, identity and usage

Who is logged in to a Codex home and on which plan, read locally from [`auth.json`](ds:block?id=codex-agent-identitycontext-gdq8smc7) or the OS keyring without verifying or refreshing anything; how much of the rate limit is used, from one call to the vendor's usage endpoint or for free from the last snapshot in the newest rollout; and the [`codex login`](ds:block?id=codex-agent-logincommand-q3tdrhcq) / `codex logout` commands. Tokens never leave the library as text: personal values print [`[redacted]`](ds:block?id=personal-yfzy8bms).

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` and `codex.Home` from [The Codex agent](agent.md).
- [`cli_auth_credentials_store`](ds:block?id=codex-agent-loadauth-hsxkaah9) in `config.toml` decides where the login lives: [`file`](ds:block?id=codex-agent-credentialstore-7q7gydxk) (`auth.json`, the default), `keyring` (the OS keyring through `Host.Secrets`), `auto` (keyring, falling back to the file) or `ephemeral` (in memory only).
- [`Usage`](ds:block?id=codex-agent-usage-bnqunb32) needs `Host.HTTP` and spends one network request; [`CachedUsage`](ds:block?id=codex-agent-cachedusage-8nmze7z6) and [`Identity`](ds:block?id=codex-agent-identitycontext-gdq8smc7) never touch the network.

The snippets come from [`examples/codexaccount`](../../examples/codexaccount/). It runs on a copy of the fixture home with a fake keyring, a clock before the fixture token expires, and an `httptest` server answering the usage request.

## Step 1 — Read the identity

[`Identity(h)`](ds:block?id=codex-applyidclaims-m54n9gf8) decodes the login's JWT claims locally: `Mode` (the subscription login or `codex.AuthAPIKey`), `LoggedIn`, `Plan`, subscription dates, token expiry times, `CredentialStore` (where the login was found) and `Email`, `Name`, `AccountID`, `UserID` as redacting `agentio.Personal`. [`IdentityContext`](ds:block?id=codex-agent-identitycontext-gdq8smc7) is the same with a context for the keyring lookup. [`ParseAuth`](ds:block?id=codex-parseauth-4se37c7r) decodes `auth.json` bytes you hold. `Plan` keeps unknown plans verbatim; [`ParsePlan`](ds:block?id=codex-parseplan-btvpuu2q) normalises raw text the way Codex does, [`DisplayName`](ds:block?id=codex-plan-displayname-bfdt6s7g) and [`IsWorkspace`](ds:block?id=codex-plan-isworkspace-rf9e4tu9) describe it.

<!-- snippet: examples/codexaccount/main.go#identity -->
```go
// readIdentity decodes the login of the default home from auth.json:
// claims are read locally, never verified, refreshed or sent anywhere.
// Personal values (email, ids) print redacted.
func readIdentity(w io.Writer, host agentio.Host) error {
	a := codex.New(host)
	id, err := a.Identity(a.DefaultHome())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "logged-in=%v api-key=%v plan=%s (%s) workspace=%v\n",
		id.LoggedIn, id.Mode == codex.AuthAPIKey, id.Plan, id.Plan.DisplayName(), id.Plan.IsWorkspace())
	fmt.Fprintf(w, "email=%v account=%v store=%s\n", id.Email, id.AccountID, id.CredentialStore)
	fmt.Fprintf(w, "access token expires %s\n", id.AccessExpiresAt.Format(time.RFC3339))

	n := id.Neutral()
	fmt.Fprintf(w, "neutral plan=%q billing=%s\n", n.PlanLabel, n.Billing)

	// ParsePlan normalises raw plan strings the way Codex does.
	fmt.Fprintf(w, "ParsePlan(hc)=%s ParsePlan(Education)=%s\n", codex.ParsePlan("hc"), codex.ParsePlan("Education"))
	return nil
}
```

For a keyring home, [`KeyringEntry(h)`](ds:block?id=codex-agent-keyringentry-zd3j66a7) names the item Codex reads (service `codex.KeyringService`, account [`codex.KeyringAccount(dir)`](ds:block?id=codex-keyringaccount-pfgfsmdf): `cli|` plus 16 hex digits of a hash of the resolved home path), never its value. A failed read is a [`*codex.KeyringError`](ds:block?id=codex-keyringerror-5yhnmn95) naming the entry; a value that is not auth JSON matches [`codex.ErrKeyringValue`](ds:block?id=codex-agent-loadkeyring-2zu65fku).

<!-- snippet: examples/codexaccount/main.go#keyring -->
```go
// readKeyringIdentity reads a home whose config.toml says
// cli_auth_credentials_store = "keyring": the login comes from the OS
// keyring (here a fake) under an account derived from the home path.
func readKeyringIdentity(w io.Writer, host agentio.Host, secrets *agenttest.FakeSecrets) error {
	a := codex.New(host)
	auth, err := os.ReadFile(a.Paths(a.DefaultHome()).Auth)
	if err != nil {
		return err
	}
	work, err := newKeyringHome(a, filepath.Join(host.UserHome, keyringDir))
	if err != nil {
		return err
	}
	entry := a.KeyringEntry(work)
	fmt.Fprintf(w, "keyring service=%q account matches KeyringAccount: %v\n",
		entry.Service, entry.Account == codex.KeyringAccount(work.Dir()))
	secrets.Put(entry.Service, entry.Account, string(auth))

	id, err := a.IdentityContext(context.Background(), work)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "keyring login: logged-in=%v store=%s source=%q\n", id.LoggedIn, id.CredentialStore, id.Source)

	// A keyring item that is not auth JSON is a *KeyringError naming the
	// entry, never the value.
	broken, err := newKeyringHome(a, filepath.Join(host.UserHome, brokenDir))
	if err != nil {
		return err
	}
	be := a.KeyringEntry(broken)
	secrets.Put(be.Service, be.Account, "not json")
	_, err = a.Identity(broken)
	var ke *codex.KeyringError
	if errors.As(err, &ke) {
		fmt.Fprintf(w, "keyring error: value is not auth JSON: %v\n", errors.Is(err, codex.ErrKeyringValue))
	}
	return nil
}

// newKeyringHome creates a home whose config.toml selects the keyring store.
func newKeyringHome(a *codex.Agent, dir string) (codex.Home, error) {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return codex.Home{}, err
	}
	cfg := []byte("cli_auth_credentials_store = \"keyring\"\n")
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), cfg, filePerm); err != nil {
		return codex.Home{}, err
	}
	return a.HomeAt(dir)
}
```

## Step 2 — Read live usage

[`Usage(ctx, h)`](ds:block?id=codex-agent-usage-bnqunb32) makes one `GET` to `codex.UsageEndpoint` (or `<base>/wham/usage` on a copy from [`a.WithBaseURL(base)`](ds:block?id=codex-agent-withbaseurl-exqfdthx)) with the home's access token and returns the plan, `LimitReached` and the rate-limit `Windows` (each with `Length` as a `time.Duration`, `Percent`, `ResetsAt`). [`ParseUsage`](ds:block?id=codex-parseusage-67d5nukv) decodes a body you fetched yourself. [`Subscription(ctx, h)`](ds:block?id=codex-agent-subscription-zk2v5smc) answers from the id_token claims on disk and only falls back to one usage fetch when the plan claim is absent; its `Source` says which.

<!-- snippet: examples/codexaccount/main.go#usage -->
```go
// readUsage makes one GET to the usage endpoint with the home's access
// token. WithBaseURL sends it to baseURL (here the local server) instead of
// chatgpt.com. Call it once; never poll.
func readUsage(w io.Writer, host agentio.Host, baseURL string) error {
	a := codex.New(host).WithBaseURL(baseURL)
	ctx := context.Background()
	u, err := a.Usage(ctx, a.DefaultHome())
	switch {
	case errors.Is(err, agentio.ErrExpired):
		return fmt.Errorf("token expired: run codex once to refresh it: %w", err)
	case errors.Is(err, agentio.ErrRateLimited):
		return fmt.Errorf("rate limited: try later: %w", err)
	case err != nil:
		return err
	}
	fmt.Fprintf(w, "usage plan=%s limit-reached=%v source=%s\n", u.Plan, u.LimitReached, u.Source)
	for _, win := range u.Windows {
		fmt.Fprintf(w, "window %s length=%s used=%.1f%%\n", win.Vendor, win.Length, win.Percent)
	}

	// ParseUsage decodes a body you fetched yourself.
	if _, err := codex.ParseUsage([]byte(`{"rate_limit":{}}`)); errors.Is(err, codex.ErrNoWindows) {
		fmt.Fprintln(w, "a body without windows is ErrNoWindows")
	}

	// Subscription answers from the id_token claims; only a login without
	// a plan claim costs a usage fetch.
	sub, err := a.Subscription(ctx, a.DefaultHome())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "subscription plan=%s source=%s until=%s\n", sub.Plan, sub.Source, sub.ActiveUntil.Format("2006-01-02"))
	return nil
}
```

## Step 3 — Read cached usage

[`CachedUsage(h)`](ds:block?id=codex-agent-cachedusage-8nmze7z6) returns the last `token_count` rate-limit snapshot of the newest rollout that has one (of the five newest) from a bounded tail read: usage with zero network calls, timestamped by its record ([`ObservedAt`](ds:block?id=codex-usagefromsnapshot-8sff5ghq)), `found=false` when none of those rollouts has one.

<!-- snippet: examples/codexaccount/main.go#cached-usage -->
```go
// readCachedUsage reads the last rate-limit snapshot from the newest
// rollout: zero network calls, timestamped by its record.
func readCachedUsage(w io.Writer, host agentio.Host) error {
	a := codex.New(host)
	u, found, err := a.CachedUsage(a.DefaultHome())
	if err != nil || !found {
		return err
	}
	fmt.Fprintf(w, "cached source=%s observed=%s\n", u.Source, u.ObservedAt.Format(time.RFC3339))
	for _, win := range u.Windows {
		fmt.Fprintf(w, "cached window %s limit=%s length=%s used=%.0f%%\n", win.Vendor, win.LimitID, win.Length, win.Percent)
	}
	return nil
}
```

## Step 4 — Build login commands

[`LoginCommand(h, flow)`](ds:block?id=codex-agent-logincommand-q3tdrhcq) builds `codex login` for each `agentio.LoginFlow`: browser, `--device-auth`, `--with-api-key`, `--with-access-token`. For the stdin flows the caller writes the secret to the process's stdin; it is never on argv. [`LogoutCommand(h)`](ds:block?id=codex-agent-logoutcommand-gcygwwqe) builds `codex logout`. Both set [`CODEX_HOME`](ds:block?id=codex-agent-plaincommand-v7yqu9ez) for a non-default home.

<!-- snippet: examples/codexaccount/main.go#login -->
```go
// loginCommands builds `codex login` and `codex logout`. Secrets for the
// stdin flows are written to the process's stdin, never put on argv.
func loginCommands(w io.Writer, host agentio.Host) error {
	a := codex.New(host)
	h := a.DefaultHome()
	for _, f := range agentio.LoginFlowValues() {
		c, err := a.LoginCommand(h, f)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%-18s codex %s\n", f, strings.Join(c.Args, " "))
	}
	fmt.Fprintf(w, "logout             codex %s\n", strings.Join(a.LogoutCommand(h).Args, " "))
	return nil
}
```

## Try it

```sh
cd examples && go run ./codexaccount
```

```text
logged-in=true api-key=false plan=plus (Plus) workspace=false
email=[redacted] account=[redacted] store=file
access token expires 2026-09-21T14:13:20Z
neutral plan="Plus" billing=subscription
ParsePlan(hc)=enterprise ParsePlan(Education)=edu
keyring service="Codex Auth" account matches KeyringAccount: true
keyring login: logged-in=true store=keyring source=""
keyring error: value is not auth JSON: true
usage plan=plus limit-reached=false source=server
window primary_window length=5h0m0s used=42.5%
window secondary_window length=168h0m0s used=10.0%
a body without windows is ErrNoWindows
subscription plan=plus source=cache until=2026-11-01
server saw a bearer token: true
cached source=transcript observed=2026-10-14T10:00:08Z
cached window primary limit=codex length=168h0m0s used=12%
browser            codex login
device             codex login --device-auth
api-key-stdin      codex login --with-api-key
access-token-stdin codex login --with-access-token
logout             codex logout
```

## Pitfalls

- **Nothing here refreshes a token.** An expired access token makes `Usage` return [`agentio.ErrExpired`](ds:block?id=codex-agent-usage-bnqunb32); run `codex` once to refresh it. Refreshing from your code would rotate the refresh token and log the CLI out.
- **`Usage` is a network read that may 429.** A 429 maps to [`agentio.ErrRateLimited`](ds:block?id=codex-agent-usage-bnqunb32) ([with the server's retry-after](ds:block?id=ratelimiterror-pbyjx7ce)), 401/403 to `agentio.ErrUnauthorized`; there is no retry. Call it once per user action, never in a loop; prefer `CachedUsage` for dashboards.
- **API-key logins have no plan usage.** `Usage` returns [`agentio.ErrBilledPerToken`](ds:block?id=codex-agent-usage-bnqunb32); [`Identity.Neutral()`](ds:block?id=codex-identity-neutral-8g73697s) reports per-token billing.
- **No stored login is not an error.** `Identity` returns [`LoggedIn=false`](ds:block?id=codex-agent-identitycontext-gdq8smc7) with a nil error. An `ephemeral` store returns [`agentio.ErrUnsupported`](ds:block?id=codex-agent-loadauth-hsxkaah9).
- **Use `WithBaseURL` for a test server or proxy.** [`a.WithBaseURL(srv.URL)`](ds:block?id=codex-agent-withbaseurl-exqfdthx) returns a copy that calls `<base>/wham/usage` instead of [`codex.UsageEndpoint`](ds:block?id=codex-usageendpoint-2rx9qqg7) (`codex.DefaultBaseURL` + `/wham/usage`), as the example does; set `Host.HTTP` to the server's client.
- **The usage endpoint is undocumented** and parsed defensively: `Credits` and [`IndividualLimit`](ds:block?id=codex-usage-adqs3xpc) stay raw, and a body without windows is [`codex.ErrNoWindows`](ds:block?id=codex-parseusage-67d5nukv).
- **Cached windows name themselves differently.** Server windows have `Vendor` [`primary_window`](ds:block?id=codex-parseusage-67d5nukv) / `secondary_window`; snapshot windows have `primary` / `secondary` and a [`LimitID`](ds:block?id=codex-usagefromsnapshot-8sff5ghq).
- **Personal values redact in every output** (`%v`, `%+v`, JSON, `slog`). Call [`Reveal()`](ds:block?id=personal-yfzy8bms) only to pass a value on, never to print it.

## Related

- [Install and identity (neutral)](../capabilities/install-identity.md), [Usage (neutral)](../capabilities/usage.md), [Logins](../capabilities/logins.md)
- [Secrets: Token, EnvValue, Personal](../core/secrets.md), [Errors](../core/errors.md)
- [Codex rollout files](rollout.md), [Codex doctor](doctor.md)
- [Claude account](../claude/account.md), [Claude usage](../claude/usage.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
