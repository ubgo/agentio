# Usage and subscription

Plan usage is how much of each rate-limit window an account has used: the five-hour session window, the weekly windows, and so on. agentio has three ways to read it: `CachedUsageReader` reads the copy the vendor CLI left on disk (free, safe in a loop), `UsageReader` asks the vendor once (network, may 429), and `SubscriptionReader` reports the plan itself. All three return neutral types, so one status bar can show Claude Code and Codex side by side.

## Before you start

- Modules: `github.com/ubgo/agentio`, `github.com/ubgo/agentio/claude`, `github.com/ubgo/agentio/codex` (or `builtin`).
- Cached usage needs only the home on disk.
- Live usage and the Claude subscription need a logged-in, unexpired home and `Host.HTTP`. In tests, point the Claude agent at an `httptest` server with `claude.New(h).WithBaseURL(srv.URL)`.

[`examples/usage`](../../examples/usage/) copies the fixture homes into a temp dir, starts an in-process fake vendor API serving the recorded response shapes, and routes every request of its HTTP client to that server, so nothing leaves the process.

## Step 1 — Read cached usage

`CachedUsage(home)` returns `(Usage, found, error)`. `found` is false when the vendor has not left a reading in that home; that is not an error. Claude keeps the last usage response in `.claude.json`; Codex records rate limits in its session transcripts.

<!-- snippet: examples/usage/main.go#cached -->
```go
// printCachedUsage reads the usage each vendor CLI left on disk: no network,
// safe in a loop. The reading is as old as the vendor's last refresh, so the
// age (Host clock minus ObservedAt) is part of the answer.
func printCachedUsage(w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	for _, a := range agents {
		cu, ok := agentio.CachedUsageOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return err
		}
		for _, home := range homes {
			u, found, err := cu.CachedUsage(home)
			if err != nil {
				return err
			}
			if !found {
				fmt.Fprintf(w, "%s %s: no cached usage\n", a.ID(), home.Name())
				continue
			}
			age := h.Now().Sub(u.ObservedAt).Round(time.Minute)
			fmt.Fprintf(w, "%s %s: max %.0f%% source=%s age=%s\n", a.ID(), home.Name(), u.MaxPercent(), u.Source, age)
			printWindows(w, u)
		}
	}
	return nil
}
```

`Usage.ObservedAt` is when the vendor fetched or recorded the reading, and `Usage.Source` says where it came from (`cache`, `transcript`). Show the age: a cached reading from two weeks ago is not today's usage.

## Step 2 — Read live usage

`Usage(ctx, home)` makes one request with the home's stored login. It never refreshes a token: an expired login returns `ErrExpired` before any request is sent.

<!-- snippet: examples/usage/main.go#live -->
```go
// printLiveUsage asks each vendor once for the default home's usage. This
// is a network read that may 429: call it on demand, cache the answer,
// never poll. An expired login is refused before any request.
func printLiveUsage(ctx context.Context, w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		ur, ok := agentio.UsageOf(a)
		if !ok {
			continue
		}
		u, err := ur.Usage(ctx, a.DefaultHome())
		switch {
		case errors.Is(err, agentio.ErrExpired):
			fmt.Fprintf(w, "%s: login expired; run the vendor CLI once to refresh it\n", a.ID())
			continue
		case errors.Is(err, agentio.ErrRateLimited):
			fmt.Fprintf(w, "%s: rate limited; keep the last reading\n", a.ID())
			continue
		case err != nil:
			return err
		}
		fmt.Fprintf(w, "%s: max %.0f%% billing=%s source=%s\n", a.ID(), u.MaxPercent(), u.Billing, u.Source)
		printWindows(w, u)
	}
	return nil
}
```

The neutral `agentio.Usage` carries:

- `Windows []Window`: each with `Length` (a `time.Duration`, 0 when the vendor does not say), `Scope` (a model or surface, "" for all usage), `Label`, `Percent`, `ResetsAt` (nil for no scheduled reset) and `VendorKind` (the raw kind, such as `five_hour`).
- `Window.Class()`: a `WindowClass` derived from `Length` (`short` under a day, `day`, `week`, `month`, `unknown` for 0).
- `MaxPercent()`: the highest percent across windows; any window at 100% blocks the account.
- `Billing`: `subscription`, `per-token` or `unknown`. `per-token` means there are no windows at all: an answer, not a failure.
- `Source`: `server`, `cache`, `transcript` or `probe`. `Raw` keeps the vendor body for vendor decoders such as `claude.ExtraUsageOf`.

## Step 3 — Read the subscription

`Subscription(ctx, home)` returns the plan label and, where the vendor reports them, status, billing type and start date. Claude asks its profile endpoint (network); Codex answers from the claim in its stored login first and calls the network only when the claim is absent.

<!-- snippet: examples/usage/main.go#subscription -->
```go
// printSubscriptions reads the plan of each default home. Claude asks the
// profile endpoint (network); Codex answers from the login on disk first.
func printSubscriptions(ctx context.Context, w io.Writer, agents []agentio.Agent) error {
	for _, a := range agents {
		sr, ok := agentio.SubscriptionOf(a)
		if !ok {
			continue
		}
		s, err := sr.Subscription(ctx, a.DefaultHome())
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "%s: plan=%q status=%q billing-type=%q\n", a.ID(), s.Plan, s.Status, s.BillingType)
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./usage
```

The first three sections of the output:

```text
== cached usage ==
claude personal: max 6% source=cache age=387h21m0s
  Current session  short   6.0% resets 2026-10-28T12:59:59Z
  All models       week    2.0% resets 2026-11-04T05:59:59Z
  Fable            week    0.0% resets 2026-11-04T06:00:00Z
claude work: no cached usage
codex personal: max 12% source=transcript age=722h0m0s
  7d               week   12.0% resets 2026-09-21T14:13:20Z
== live usage ==
claude: max 42% billing=subscription source=server
  Current session  short  42.0% resets 2026-11-13T15:00:00Z
  All models       week   18.0% resets 2026-11-18T09:00:00Z
codex: login expired; run the vendor CLI once to refresh it
== subscription ==
claude: plan="Max (20x)" status="active" billing-type="stripe_subscription"
codex: plan="Plus" status="" billing-type=""
```

The fixture's Codex login is expired, so live usage is refused locally; the Codex subscription still answers because it comes from the stored login.

## Pitfalls

- **Never poll `Usage`.** It is a network read that may return `ErrRateLimited` (HTTP 429). Call it when the user asks, cache the result, and fall back to `CachedUsage` in loops and status lines. `agentio.RateLimitError` carries the server's retry time when it sent one.
- **Expired is local, not a 401.** An expired stored login returns `ErrExpired` without a request. The fix is to run the vendor CLI once in that home; agentio never refreshes a token because refreshing rotates the refresh token and logs the vendor CLI out.
- **`ErrNotLoggedIn`, `ErrUnauthorized`, `ErrScopeLimited`** are the other answers to expect: no login, a rejected token, and (Claude) a token that lacks the profile scope. Match them with `errors.Is`.
- **Cached readings can be very old.** The example's Claude reading is 16 days old. Compare `ObservedAt` with `Host.Now()` before you call it "current".
- **`found == false` is not an error.** A home that never fetched usage (the Claude work home here) has nothing cached.
- **Point both vendors at a test server with `WithBaseURL`.** `claude.Agent.WithBaseURL` and `codex.Agent.WithBaseURL` replace the API origin (`codex.DefaultBaseURL` is `https://chatgpt.com/backend-api`), so a test passes an `httptest` server's client as `Host.HTTP` and its URL as the base, with no custom transport; `examples/usage` does this for both.
- **Window labels come from the vendor.** Labels like `Fable` or `7d` are whatever the vendor sent; group by `Class()` or `Length`, not by label.

## Related

- [Logins and tokens](logins.md): login commands, token logins and the token probe (the fourth way to read usage).
- [Shared capabilities and accessors](accessors.md).
- Vendor pages: [Claude usage](../claude/usage.md) (extra usage, rich limits), [Claude account](../claude/account.md), [Codex auth and usage](../codex/auth-usage.md).
- [Errors](../core/errors.md) for `ErrExpired`, `ErrRateLimited` and friends.
- [Choosing guides](../choosing/README.md). Example: [`examples/usage`](../../examples/usage/). Feature index: [features](../reference/features.md).
