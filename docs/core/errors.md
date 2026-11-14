# Errors

agentio reports every expected failure with a sentinel error from the root package. Vendor packages wrap them with their own wording (`fmt.Errorf("claude: …: %w", agentio.ErrExpired)`), so the message text changes between packages but the sentinel does not: branch with `errors.Is`, never on the message. You want this page to turn a library error into the right advice for your user, and to handle a rate limit without telling them to log in again.

## Before you start

- Module: `github.com/ubgo/agentio`; the example also uses `codex` for an agent ID.
- Nothing on disk.

## Step 1 — Match a sentinel

Every sentinel answers one question. The list:

| Sentinel | Means | Tell the user |
|---|---|---|
| [`ErrNotInstalled`](ds:block?id=errnotinstalled-e8y46yuj) | the binary is not on PATH | install the CLI |
| [`ErrNotLoggedIn`](ds:block?id=errnotloggedin-hd6d747t) | the home holds no usable credential | log in with the vendor CLI |
| [`ErrExpired`](ds:block?id=errexpired-yj9h93tv) | the local credential is past its expiry (never refreshed by the library) | run the vendor CLI once |
| [`ErrUnauthorized`](ds:block?id=errunauthorized-bx364rgx) | the server refused a credential that looked valid | log in again |
| [`ErrScopeLimited`](ds:block?id=errscopelimited-6v2zr8df) | the credential can run the agent but cannot read account, plan or usage | not a login problem |
| [`ErrBilledPerToken`](ds:block?id=errbilledpertoken-6maccp3u) | no usage windows exist: an answer, not a failure | show "billed per token" |
| [`ErrRateLimited`](ds:block?id=errratelimited-6rsdy999) | a transient 429 ([Step 2](#step-2--handle-rate-limits)) | try again later |
| [`ErrUnsupported`](ds:block?id=errunsupported-e8zgsckh) | the agent lacks the capability or launch field; always wrapped with what and why | show the reason |
| [`ErrForeignHome`](ds:block?id=errforeignhome-z79t6z5e) | a home of another agent was passed | a bug in the caller |
| [`ErrUnknownVersion`](ds:block?id=errunknownversion-k45eb95g) | a vendor file declares a format version the library has no sample for; only that reader fails | upgrade agentio |
| [`ErrFeatureUnavailable`](ds:block?id=errfeatureunavailable-fzzq5s2u) | the installed vendor version provably lacks a flag or file | upgrade the CLI |
| [`ErrMoveBlocked`](ds:block?id=errmoveblocked-8a8jj7sq) | a move plan has blockers | show the blockers |
| [`ErrUnknownAgent`](ds:block?id=errunknownagent-fbp52357) | an ID lookup failed | check the agent ID |
| [`ErrNotFound`](ds:block?id=errnotfound-cusd5chu) | a named item (conversation, session, file) does not exist | — |
| [`ErrDangerousNotAllowed`](ds:block?id=errdangerousnotallowed-cfd5h3du) | full autonomy without `Target.AllowDangerous` | a policy answer |
| [`ErrInvalidSpec`](ds:block?id=errinvalidspec-835a58xh), [`ErrInvalidID`](ds:block?id=errinvalidid-typkqvpz) | a config-defined agent or ID was rejected | fix the config |

One function can map them all:

<!-- snippet: examples/values/errors.go#match-sentinel -->
```go
// explain maps an agentio error to what a caller tells the user. Branch
// with errors.Is: vendor packages wrap the sentinels with their own
// wording, so the message text is not stable but the sentinel is.
func explain(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, agentio.ErrNotInstalled):
		return "install the CLI first"
	case errors.Is(err, agentio.ErrNotLoggedIn):
		return "log in with the vendor CLI"
	case errors.Is(err, agentio.ErrExpired):
		return "run the vendor CLI once to refresh its login"
	case errors.Is(err, agentio.ErrScopeLimited):
		return "this credential can run the agent but not read usage (not a login problem)"
	case errors.Is(err, agentio.ErrBilledPerToken):
		return "billed per token: there are no usage windows"
	case errors.Is(err, agentio.ErrRateLimited):
		return "rate-limited: try again later"
	case errors.Is(err, agentio.ErrUnsupported):
		return "not supported by this agent"
	case errors.Is(err, agentio.ErrInvalidSpec), errors.Is(err, agentio.ErrInvalidID):
		return "fix the agent config"
	case errors.Is(err, agentio.ErrNotFound):
		return "no such item"
	}
	return "unexpected: " + err.Error()
}

// matchSentinels produces real errors from the library and explains each.
func matchSentinels(p printer) error {
	_, notInstalled := agentio.LookPath("", "claude")
	_, badID := agentio.ParseID("My Agent")
	_, badSpec := agentio.NewCore(agentio.Spec{ID: "toy"}, agentio.Host{})
	// A config agent whose Spec has no PromptArgs refuses a prompt with
	// ErrUnsupported instead of dropping it.
	toy, err := agentio.FromSpec(agentio.Spec{ID: "toy", Binary: "toy", NewArgs: []string{"{args}"}}, agentio.Host{})
	if err != nil {
		return err
	}
	_, unsupported := toy.Launch(toy.DefaultHome().Target(), agentio.LaunchRequest{Prompt: "hi"})
	byHand := agentio.UnsupportedError(codex.ID, "history move", "conversations are indexed in SQLite")
	expired := fmt.Errorf("claude: credentials: %w", agentio.ErrExpired)
	for _, err := range []error{notInstalled, badID, badSpec, unsupported, byHand, expired} {
		p.line(fmt.Sprintf("error: %v\n  -> %s", err, explain(err)))
	}
	return nil
}
```

[`agentio.UnsupportedError(agent, what, reason)`](ds:block?id=unsupportederror-tub9t75m) builds the `ErrUnsupported` error every agent returns, so you can show the agent's own reason ("conversations are indexed in SQLite") instead of a generic message.

## Step 2 — Handle rate limits

A 429 from a vendor usage or profile endpoint is `ErrRateLimited`, never a login error. [`agentio.RateLimitError(header, now)`](ds:block?id=ratelimiterror-pbyjx7ce) is how every vendor client words it: it reads `Retry-After` (seconds, or an HTTP date relative to `now`, which is `Host.Now()`) and adds the wait to the message. [`agentio.RetryAfter(value, now)`](ds:block?id=retryafter-v8wbfwra) parses the header on its own.

<!-- snippet: examples/values/errors.go#rate-limits -->
```go
// rateLimits words a 429 the way every vendor client does: RateLimitError
// reads Retry-After (seconds or an HTTP date relative to now) and wraps
// ErrRateLimited, never a login error. Pure.
func rateLimits(p printer) error {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for _, retry := range []string{"120", now.Add(90 * time.Second).Format(http.TimeFormat), ""} {
		h := http.Header{}
		if retry != "" {
			h.Set("Retry-After", retry)
		}
		err := agentio.RateLimitError(h, now)
		p.line(fmt.Sprintf("429 retry-after=%q: %v (rate-limited=%t)", retry, err, errors.Is(err, agentio.ErrRateLimited)))
	}
	if d, ok := agentio.RetryAfter("30", now); ok {
		p.line(fmt.Sprintf("RetryAfter(30) = %s", d))
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./values
```

The lines for this page:

```text
error: agentio: binary not found on PATH: claude
  -> install the CLI first
error: agentio: invalid agent id: "My Agent": only [a-z0-9-] allowed
  -> fix the agent config
error: agentio: invalid spec: toy: binary is empty
  -> fix the agent config
error: agentio: unsupported: toy: prompt: not described by the agent spec
  -> not supported by this agent
error: agentio: unsupported: codex: history move: conversations are indexed in SQLite
  -> not supported by this agent
error: claude: credentials: agentio: login expired
  -> run the vendor CLI once to refresh its login
429 retry-after="120": agentio: rate-limited; try again shortly (retry after 2m0s) (rate-limited=true)
429 retry-after="Sat, 14 Nov 2026 12:01:30 GMT": agentio: rate-limited; try again shortly (retry after 1m30s) (rate-limited=true)
429 retry-after="": agentio: rate-limited; try again shortly (rate-limited=true)
RetryAfter(30) = 30s
```

## Pitfalls

- **Match with `errors.Is`, never with `==` or the message.** Every vendor error wraps the sentinel; [`err == agentio.ErrExpired`](ds:block?id=errexpired-yj9h93tv) is false for `claude: credentials: agentio: login expired`.
- **[`ErrScopeLimited`](ds:block?id=errscopelimited-6v2zr8df) is not a login problem.** A setup-token can run Claude Code but cannot read the account or usage; telling the user to log in again does not help.
- **[`ErrBilledPerToken`](ds:block?id=errbilledpertoken-6maccp3u) is an answer.** An API-key home has no usage windows; show that instead of an error.
- **[`ErrExpired`](ds:block?id=errexpired-yj9h93tv) is never fixed by the library.** It never refreshes an OAuth token, because refreshing rotates the refresh token and logs the vendor CLI out.
- **Do not poll after a 429.** Usage endpoints rate-limit hard; cache the last answer and wait at least the `Retry-After` hint. A `Retry-After` date in the past, a negative number or garbage gives no hint at all ([`RetryAfter`](ds:block?id=retryafter-v8wbfwra) returns false).
- **[`ErrDangerousNotAllowed`](ds:block?id=errdangerousnotallowed-cfd5h3du) is not `ErrUnsupported`.** Refusing full autonomy is a policy answer; the agent does support it once `Target.AllowDangerous` is set.
- **Vendor packages add their own sentinels** for vendor-only cases (for example [`claude.ErrHistoryExists`](ds:block?id=claude-errhistoryexists-23j5925s), [`claude.ErrAmbiguousID`](ds:block?id=claude-errambiguousid-t68armxm)); they are documented on the vendor pages.

## Related

- [Optional values, unknown keys and warnings](values.md)
- [Usage and subscription](../capabilities/usage.md)
- [Shared capabilities and accessors](../capabilities/accessors.md#step-3--ask-why-a-capability-is-missing)
- [Per-feature version table](../compat/features.md#step-3--handle-featureunavailableerror)
- Example: [`examples/values`](../../examples/values/)
- [Feature index](../reference/features.md)
