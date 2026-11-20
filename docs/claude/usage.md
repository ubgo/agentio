# Claude plan usage

How much of a Claude plan is used, per window (the five-hour session, the weekly limits, per-model weekly limits, extra usage). The `claude` module reads it four ways, from cheapest to most expensive: the copy Claude Code cached on disk, the live usage endpoint for a home's login, the decoded extra-usage object, and a probe that spends a few tokens to read a token login's rate-limit headers. Use it for a usage bar, a "switch account when above 80%" rule, or a plan dashboard.

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- A home with a login (see [Claude credentials and tokens](credential.md)) for the live read; the cached read needs only `.claude.json`.
- [`Host.HTTP`](ds:block?id=claude-agent-httpclient-e4beyzpg) for network reads (nil makes every network call return `ErrUnsupported`; it never builds a client of its own) and `Host.Now` for `ObservedAt` and expiry checks.

The snippets come from [`examples/claudeusage`](../../examples/claudeusage/). It points the agent at a local `httptest` server with `WithBaseURL`; the fake usage endpoint serves the body the fixture home cached, and the fake `/v1/messages` answers with rate-limit headers. No request reaches the vendor and no quota is spent.

## Step 1 — Read live usage

[`a.Usage(ctx, h)`](ds:block?id=claude-agent-usage-zq8vtbvb) resolves the home's login, [refuses an expired one before any request (`agentio.ErrExpired`)](ds:block?id=claude-agent-usablecredential-e8k4sjep), and fetches `/api/oauth/usage` once. The result is a `claude.Usage`: the neutral `agentio.Usage` (`Windows`, `Billing`, `Source` = `agentio.UsageServer`, `ObservedAt`, `Raw`) plus `Limits` (the wire rows as `claude.Limit`) and `Extra`. Each window has a `Label` (claude.ai wording: [`Current session`](ds:block?id=claude-limit-label-dr46cv8f), `All models`, or the model name of a scoped limit), `Percent` (0–100), `ResetsAt`, `Length` and its [`Class()`](ds:block?id=window-class-sgsfcuv5) (`short`, `day`, `week`, `month`). [`u.MaxPercent()`](ds:block?id=usage-maxpercent-b8fwrpb4) is the highest window.

[`claude.ParseUsage(body)`](ds:block?id=claude-parseusage-zn4jj9gg) decodes a body you hold (network or cached). It reads the current `limits` list or, when that is absent, the legacy per-window objects (`five_hour`, `seven_day`, `seven_day_opus`, [`seven_day_sonnet`](ds:block?id=claude-legacylimits-htdx64yk)); a body with neither is `claude.ErrNoLimits`, never "0% used". [`Limit.Label()`](ds:block?id=claude-limit-label-dr46cv8f) falls back to the raw kind for kinds this version does not know. [`a.WithBaseURL(url)`](ds:block?id=claude-agent-withbaseurl-gud8dj5z) returns a copy that sends every network call (usage, profile, probe) to another origin; `claude.DefaultBaseURL` is the real one.

<!-- snippet: examples/claudeusage/main.go#live -->
```go
// readLive fetches /api/oauth/usage once for h's login. Each window has a
// label, a percent, a reset time and a length class; Limits keeps the wire
// rows. Never call this in a loop: the endpoint rate-limits (429).
func readLive(ctx context.Context, w io.Writer, a *claude.Agent, h claude.Home) (claude.Usage, error) {
	u, err := a.Usage(ctx, h)
	switch {
	case errors.Is(err, agentio.ErrNotLoggedIn), errors.Is(err, agentio.ErrExpired):
		fmt.Fprintln(w, "no usable login:", err)
		return u, nil
	case errors.Is(err, agentio.ErrRateLimited):
		fmt.Fprintln(w, "rate limited; use the cached copy instead")
		return u, nil
	case err != nil:
		return u, err
	}
	fmt.Fprintf(w, "live usage (%s, billing %s), highest window %.0f%%\n", u.Source, u.Billing, u.MaxPercent())
	for _, win := range u.Windows {
		fmt.Fprintf(w, "  %-16s %3.0f%%  class %-5s resets %s\n", win.Label, win.Percent, win.Class(), win.ResetsAt.UTC().Format(time.RFC3339))
	}
	for _, l := range u.Limits {
		fmt.Fprintf(w, "  wire row: kind %s, group %s, severity %s, active %t, label %q\n", l.Kind, l.Group, l.Severity, l.IsActive, l.Label())
	}
	if _, err := claude.ParseUsage([]byte(`{"limits":[]}`)); errors.Is(err, claude.ErrNoLimits) {
		fmt.Fprintln(w, "  empty body:", err)
	}
	return u, nil
}
```

## Step 2 — Read cached usage

[`a.CachedUsage(h)`](ds:block?id=claude-agent-cachedusage-h2vr9pps) parses `cachedUsageUtilization` from the home's `.claude.json`: the last usage body Claude Code fetched itself, with `Source` = `agentio.UsageCache` and `ObservedAt` = its fetch time. `found` is false when the home has no cached body. It is a disk read, safe to call in a loop; show its age next to the numbers, because it is only as fresh as Claude Code's last fetch. [`a.CachedUsageBody(h)`](ds:block?id=claude-agent-cachedusagebody-xz4k7r3g) returns the raw `*claude.CachedUsage` (body, fetch time, account UUID) when you want to parse it yourself.

<!-- snippet: examples/claudeusage/main.go#cached -->
```go
// readCached parses the usage body Claude Code itself cached in
// .claude.json: no network, safe in a loop, as old as its ObservedAt.
func readCached(w io.Writer, a *claude.Agent, h claude.Home) error {
	u, found, err := a.CachedUsage(h)
	if err != nil {
		return err
	}
	if !found {
		fmt.Fprintln(w, "cached usage: none in this home")
		return nil
	}
	age := a.Host().Now().Sub(u.ObservedAt).Round(time.Hour)
	fmt.Fprintf(w, "cached usage (%s), observed %s (%s old), %d windows\n", u.Source, u.ObservedAt.UTC().Format(time.RFC3339), age, len(u.Windows))
	return nil
}
```

## Step 3 — Read extra usage

Extra usage is pay-as-you-go overage beyond the plan. [`claude.ExtraUsageOf(u)`](ds:block?id=claude-extrausageof-g9ejxj4p) decodes the `extra_usage` object from any `agentio.Usage`'s `Raw` body, so code that holds only the neutral value (for example from [`agentio.UsageOf`](../capabilities/usage.md#step-2--read-live-usage)) can still read it. The result is a `claude.ExtraUsage`: `IsEnabled`, `Utilization` (nil when unset) and `Unknown` for the object's other keys. When extra usage is enabled with a utilization, [`ParseUsage` also adds an `Extra usage` window](ds:block?id=claude-parseusage-zn4jj9gg).

<!-- snippet: examples/claudeusage/main.go#extra -->
```go
// readExtra decodes the pay-as-you-go overage object from a neutral
// usage's Raw body: code that only holds an agentio.Usage can still get it.
func readExtra(w io.Writer, u claude.Usage) error {
	extra, err := claude.ExtraUsageOf(u.Neutral())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "extra usage: enabled %t, utilization set %t, unknown keys %d\n", extra.IsEnabled, extra.Utilization != nil, len(extra.Unknown))
	return nil
}
```

## Step 4 — Probe a token's usage

A token login (a `claude setup-token` token) has no `.claude.json` cache and cannot read the usage endpoint (its scope is too narrow: [`agentio.ErrScopeLimited`](ds:block?id=claude-agent-get-8w45njgb)). [`a.Prober().TokenUsage(ctx, cred)`](ds:block?id=claude-quota-door-tokenusage-j2fw66uu) sends one minimal `/v1/messages` request (one output token on the cheapest model) and reads the `anthropic-ratelimit-unified-*` headers with `Source` = `agentio.UsageProbe`: one window per `<window>` name in the headers, `Current session` (`5h`) and `All models` (`7d`) first, then any other, such as the per-model weekly window `7d_oi` that appears when the probe asks with Fable 5.2. Each window carries its header name in `ID`, its length, the part after `_` as `Scope`, its `-status` in `Status` (`rejected` refuses requests now) and `Binding` when the `representative-claim` header names it (`seven_day_oi` names `7d_oi`). [`claude.ParseProbeHeaders(h)`](ds:block?id=claude-parseprobeheaders-63ksqmwv) does the header part on its own: a window whose utilization and reset are both absent is omitted, half a `5h` or `7d` pair or a malformed value there is `claude.ErrProbeHeaders`, and a broken unknown window is skipped. An API key has no plan windows: it returns `agentio.ErrBilledPerToken` with `Billing` = per token and sends nothing. A 429 that still carries the headers is read as usage at its limit.

A window that applies to one model only shows up only when the probe asks with that model. `a.Prober().TokenUsageModel(ctx, cred, model)` (the neutral door: type-assert the `TokenProberOf` value to `agentio.ModelTokenProber`) sends `model` instead of the cheapest one; `claude.ProbeModel` resolves an alias Claude Code accepts (`opus`, `sonnet`, `haiku`, `fable`, a `[1m]` suffix dropped) to its model id, and `""` is the default probe model. A model the API refuses is `agentio.ErrModelRefused`, never a login failure.

<!-- snippet: examples/claudeusage/main.go#probe -->
```go
// probe reads a token login's usage from the rate-limit headers of one
// minimal /v1/messages call. It is the Quota door: it spends a few tokens
// on the real API, so run it once, never in a loop. An API key has no plan
// windows and is answered with ErrBilledPerToken without a request.
func probe(ctx context.Context, w io.Writer, a *claude.Agent) error {
	p := a.Prober()
	u, err := p.TokenUsage(ctx, agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken("sk-ant-oat01-EXAMPLE")})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "probe usage (%s):\n", u.Source)
	for _, win := range u.Windows {
		fmt.Fprintf(w, "  %-16s %3.0f%%  resets %s\n", win.Label, win.Percent, win.ResetsAt.UTC().Format(time.RFC3339))
	}
	if _, err := p.TokenUsage(ctx, agentio.TokenCredential{Kind: agentio.TokenAPIKey, Token: agentio.NewToken("sk-ant-api03-EXAMPLE")}); errors.Is(err, agentio.ErrBilledPerToken) {
		fmt.Fprintln(w, "api key:", err)
	}
	half := http.Header{}
	half.Set(hdr5hUtil, probe5hUtil)
	if _, err := claude.ParseProbeHeaders(half); errors.Is(err, claude.ErrProbeHeaders) {
		fmt.Fprintln(w, "half a header pair:", err)
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudeusage
```

```text
live usage (server, billing subscription), highest window 6%
  Current session    6%  class short resets 2026-10-28T12:59:59Z
  All models         2%  class week  resets 2026-11-04T05:59:59Z
  Fable              0%  class week  resets 2026-11-04T06:00:00Z
  wire row: kind session, group session, severity normal, active true, label "Current session"
  wire row: kind weekly_all, group weekly, severity normal, active false, label "All models"
  wire row: kind weekly_scoped, group weekly, severity normal, active false, label "Fable"
  empty body: claude: usage response has no limits
cached usage (cache), observed 2026-10-28T08:38:37Z (387h0m0s old), 3 windows
extra usage: enabled false, utilization set false, unknown keys 10
probe usage (probe):
  Current session   31%  resets 2026-11-13T15:00:00Z
  All models         7%  resets 2026-11-17T12:00:00Z
api key: agentio: billed per token: no usage windows
half a header pair: claude: probe response has no usable rate-limit headers: anthropic-ratelimit-unified-5h pair incomplete
```

## Pitfalls

- **Live usage is rate limited.** [`Usage` makes one request and never retries](ds:block?id=claude-agent-get-8w45njgb); a 429 is `agentio.ErrRateLimited` (its message names the [`Retry-After`](ds:block?id=ratelimiterror-pbyjx7ce) delay when the server sends one). Cache the result and fall back to `CachedUsage`; never poll.
- **The probe spends quota.** [`TokenUsage` is the Quota door](ds:block?id=claude-quota-door-tokenusage-j2fw66uu): a real request on the account's plan. Call it once per token when the user asks, never in a loop.
- **Expired logins are refused, not refreshed.** [`Usage` and `Subscription` return `agentio.ErrExpired`](ds:block?id=claude-agent-usablecredential-e8k4sjep) before any request when the stored login is past its expiry; run `claude` in that home to refresh it.
- **A setup-token cannot read account endpoints.** The usage and profile endpoints answer 403 `oauth_scope_insufficient` for it, [mapped to `agentio.ErrScopeLimited`](ds:block?id=claude-agent-get-8w45njgb); use the probe for token logins.
- **Cached usage can be days old.** `ObservedAt` is when Claude Code fetched it, not when you read it; the fixture's copy is 16 days older than the clock.
- **Percent units differ by source.** Usage windows always carry 0–100 ([the probe converts its 0..1 headers](ds:block?id=claude-parseprobeheaders-63ksqmwv)). The stream's `rate_limit_event` keeps the wire fraction: [`RateLimitWindow.Utilization`](ds:block?id=claude-ratelimitwindow-zmkhyjn8) 0.52 means 52%.
- **[A nil `Host.HTTP` is `ErrUnsupported`.](ds:block?id=claude-agent-httpclient-e4beyzpg)** `Usage`, `Subscription` and `TokenUsage` never fall back to a default client; set `Host.HTTP` (`HostFromOS` does) or expect `errors.Is(err, agentio.ErrUnsupported)`.
- **[401/403 is `agentio.ErrUnauthorized`.](ds:block?id=claude-agent-get-8w45njgb)** Error text never repeats the token: [the library replaces it with `[redacted]` if a body echoes it](ds:block?id=claude-redactin-wfbs8jc7).

## Related

- [Claude credentials and tokens](credential.md), [Claude account and identity](account.md), [Claude stats](stats.md), [Claude stream decoding](stream.md) (`rate_limit_event`)
- Neutral view: [Usage and subscription](../capabilities/usage.md), [The doors](../capabilities/doors.md#step-2--spend-quota-through-a-prober), [Logins and tokens](../capabilities/logins.md#step-3--probe-a-token)
- Example: [`examples/claudeusage`](../../examples/claudeusage/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
