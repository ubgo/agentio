# Live or cached usage

Both vendors show plan usage as windows (the 5-hour session, the week) with a percent used and a reset time. agentio can read that three ways: the copy the CLI last saved on disk (**cached**), one request to the vendor's usage endpoint with the home's stored login (**live**), or one minimal model request with a bare token whose reply headers carry the windows (**probe**). They differ in cost, freshness and what they need.

## Decision table

| | Cached | Live | Probe |
|---|---|---|---|
| Accessor | `agentio.CachedUsageOf(a)` | `agentio.UsageOf(a)` | `agentio.TokenProberOf(a)` |
| Vendor method | `claude.Agent.CachedUsage`, `codex.Agent.CachedUsage` | `claude.Agent.Usage`, `codex.Agent.Usage` | `claude.Agent.Prober().TokenUsage` |
| Cost | Disk · Read | Network · Read (may 429) | Quota: spends a little of the plan |
| Needs | the home on disk | [a stored, unexpired login in the home](ds:block?id=claude-agent-usablecredential-e8k4sjep) | a long-lived token (`TokenCredential`) |
| Claude Code source | [`cachedUsageUtilization`](ds:block?id=claude-agent-cachedusage-h2vr9pps) in `.claude.json` | the OAuth usage endpoint | rate-limit headers of one request |
| Codex source | last rate-limit snapshot in the newest rollout [(bounded tail read)](ds:block?id=codex-agent-cachedusage-8nmze7z6) | the `wham/usage` endpoint | ❌ Codex has no token login |
| Freshness | as old as the CLI's last fetch: hours or days (`ObservedAt`) | now | now |
| Safe in a loop or status line | ✅ | ❌ once per user action | ❌ never loop |
| `Usage.Source` | `UsageCache` (Claude), [`UsageTranscript`](ds:block?id=codex-usagefromsnapshot-8sff5ghq) (Codex) | [`UsageServer`](ds:block?id=codex-parseusage-67d5nukv) | [`UsageProbe`](ds:block?id=claude-parseprobeheaders-63ksqmwv) |
| Guide | [Usage: cached](../capabilities/usage.md#step-1--read-cached-usage) | [Usage: live](../capabilities/usage.md#step-2--read-live-usage) | [Logins: probe a token](../capabilities/logins.md#step-3--probe-a-token) |
| Example | [usage](../../examples/usage/), [claudeusage](../../examples/claudeusage/) | [usage](../../examples/usage/), [codexaccount](../../examples/codexaccount/) | [claudeusage](../../examples/claudeusage/), [doors](../../examples/doors/) |

## If this, then that

- If you draw a status line, a menu-bar meter or a table over every home, read cached usage. [It is free and never fails because of the network](ds:block?id=claude-agent-cachedusage-h2vr9pps).
- If the user clicks "refresh", read live usage once for that home and keep the result yourself; do not call it again until the user asks or your own cache expires.
- If the cached reading is missing (`found == false`) or older than you accept (compare `ObservedAt` with `Host.Now()`), fall back to live usage once.
- If live usage returns `agentio.ErrRateLimited`, show the cached reading and [the retry hint in the error](ds:block?id=ratelimiterror-pbyjx7ce); never retry in a loop.
- If it returns `agentio.ErrExpired`, the stored login is past its expiry and [no request was made](ds:block?id=claude-agent-usablecredential-e8k4sjep). Tell the user to run the CLI once in that home; agentio never refreshes a token.
- If it returns [`agentio.ErrBilledPerToken`](ds:block?id=codex-agent-usage-bnqunb32), the account pays per token and has no windows. That is an answer, not a failure.
- If it returns [`agentio.ErrScopeLimited`](ds:block?id=claude-agent-get-8w45njgb) (a Claude setup-token cannot read account endpoints), use the probe for that token instead.
- If you have only a token and no home (a CI runner), the probe is the only option, and [only for Claude Code](ds:block?id=claude-quota-door-tokenusage-j2fw66uu).

## Cached first, live on demand

```go
// usageNow prefers the free cached reading and asks the vendor only when
// the cache is missing or older than maxAge. A 429 is an answer to wait,
// never a login problem, so it is reported and not retried.
func usageNow(ctx context.Context, a agentio.Agent, home agentio.Home, now time.Time, maxAge time.Duration) (agentio.Usage, error) {
	if cu, ok := agentio.CachedUsageOf(a); ok {
		u, found, err := cu.CachedUsage(home) // Disk · Read
		if err != nil {
			return agentio.Usage{}, err
		}
		if found && now.Sub(u.ObservedAt) < maxAge {
			return u, nil
		}
	}
	live, ok := agentio.UsageOf(a)
	if !ok {
		return agentio.Usage{}, fmt.Errorf("%s: %w", a.ID(), agentio.ErrUnsupported)
	}
	u, err := live.Usage(ctx, home) // Network · Read: once, never in a tight loop
	switch {
	case errors.Is(err, agentio.ErrRateLimited):
		return agentio.Usage{}, fmt.Errorf("try later: %w", err)
	case errors.Is(err, agentio.ErrBilledPerToken):
		return agentio.Usage{Agent: a.ID(), Billing: agentio.BillingPerToken}, nil
	}
	return u, err
}
```

On the Claude fixture home the cached reading is older than a day and the stored login has expired, so this returns `agentio: login expired at 2026-11-13T20:56:01Z: run claude once in this home to refresh it` without sending a request.

## Pitfalls

- **Choosing an account from usage is `agentio/pick`.** Build a `pick.Candidate` per home from either kind of reading and call `pick.Best`; with cached readings pass `pick.WithNow` so the explanation shows their age ([Pick the home with the most headroom](../capabilities/pick.md)).
- **Cached readings can be very old.** [`ObservedAt`](ds:block?id=claude-agent-cachedusage-h2vr9pps) is when the CLI fetched or recorded it, not when you read it. Show the age.
- **Window labels are vendor text.** Group windows by [`Class()`](ds:block?id=window-class-sgsfcuv5) or `Length`, not by `Label`.
- **Codex's cached reading comes from a rollout.** A home with no recent conversation [has no snapshot](ds:block?id=codex-agent-cachedusage-8nmze7z6), and the windows are named [`primary`](ds:block?id=codex-vendorprimaryl-pjy6hvnt) / `secondary` rather than the server's `primary_window` / `secondary_window`.
- **Both vendors' live calls go through `WithBaseURL` in tests.** [`claude.Agent.WithBaseURL`](ds:block?id=claude-agent-withbaseurl-gud8dj5z) and [`codex.Agent.WithBaseURL`](ds:block?id=codex-agent-withbaseurl-exqfdthx) send requests to an `httptest` server (as `examples/usage` does), so no custom transport is needed.
- **Give the Host an HTTP client on purpose.** For tests, [`agenttest.FailingHTTP`](ds:block?id=agenttest-failinghttp-8vye27y9) makes any unexpected network call fail loudly instead of reaching the vendor.
- **Subscription can be a separate network read.** [`agentio.SubscriptionOf(a)`](ds:block?id=codex-agent-subscription-zk2v5smc) fetches the plan (Claude always; Codex only when the id_token on disk lacks the plan claim); the plan label in [`IdentityOf`](ds:block?id=claude-agent-identity-5vqz5ezp) comes from disk and is usually enough.

## Related

- [Usage and subscription](../capabilities/usage.md), [Claude plan usage](../claude/usage.md), [Codex auth, identity and usage](../codex/auth-usage.md), [Errors](../core/errors.md#step-2--handle-rate-limits)
- Other choices: [Which door](doors.md), [Concrete type or accessor](concrete-or-accessor.md)
- Examples: [usage](../../examples/usage/), [claudeusage](../../examples/claudeusage/), [codexaccount](../../examples/codexaccount/)
- [Feature index](../reference/features.md), [All choosing guides](README.md)
