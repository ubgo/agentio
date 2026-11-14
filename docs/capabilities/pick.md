# Pick the home with the most headroom

When a user has several accounts (a personal and a work Claude home, a Codex home), a launcher should start the one with the most room left. `agentio/pick` makes that choice from usage you already read: it scores every home with a policy, skips the ones it cannot compare, and returns the winner plus a one-line explanation. Use it for an "auto" account in a launcher, a dashboard's suggestion, or a CI job that should not land on a capped account.

The package is policy, not facts: it reads nothing and no core package imports it. The default policy is agx's auto-pick: a home's score is the highest percent across its windows, because any single window at its cap blocks work; the lowest score wins; ties keep your order.

## Before you start

- Module `github.com/ubgo/agentio` (the `pick` package is in the root module, stdlib only), plus the vendor modules for the agents you read usage from.
- A Host with `HTTP` set for live usage (`HostFromOS` does), or nothing beyond disk for cached usage.
- Decide how fresh the numbers must be: live usage costs one network request per home and may answer 429; cached usage costs nothing but can be days old ([Live or cached usage](../choosing/live-or-cached-usage.md)).

The snippets come from [`examples/pick`](../../examples/pick/), which runs on fixture homes with an in-process usage server: the Claude personal home is busy (77% in one window), the work home is quiet (9%), and the Codex login has expired.

## Step 1 — Build a candidate per home

A [`pick.Candidate`](ds:block?id=pick-candidate-373dhag3) is a home, its `Usage` and the error from reading it. Keep the error on the candidate instead of dropping the home: pick reports it as the reason the home was skipped. Set `Candidate.Billing` to `agentio.BillingPerToken` when you know a home bills per token (a profile setting); a per-token home has no windows to compare and is skipped.

<!-- snippet: examples/pick/main.go#live -->
```go
// liveCandidates reads live usage for every home of every agent that has
// a UsageReader: one network request per home, which may 429, so call it
// once per user action and never in a loop. A read error is kept on the
// candidate (pick skips it with the reason). owners maps a home back to
// its agent, to launch it once chosen.
func liveCandidates(ctx context.Context, agents []agentio.Agent) ([]pick.Candidate, map[agentio.Home]agentio.Agent, error) {
	var cands []pick.Candidate
	owners := map[agentio.Home]agentio.Agent{}
	for _, a := range agents {
		ur, ok := agentio.UsageOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return nil, nil, err
		}
		for _, home := range homes {
			u, err := ur.Usage(ctx, home)
			cands = append(cands, pick.Candidate{Home: home, Usage: u, Err: err})
			owners[home] = a
		}
	}
	return cands, owners, nil
}
```

## Step 2 — Choose and explain

[`pick.Best(cands)`](ds:block?id=pick-best-8t8m6qaj) skips a candidate with a usage error or per-token billing (the error is the reason when both apply), scores the rest with the policy ([`MaxPercent`](ds:block?id=pick-maxpercent-score-vrqt3y7v) by default), and returns a `pick.Result`: `Chosen` (the lowest score; the first in your order on a tie), `Others` (best first, ties in your order) and `Skipped` (your order). Each [`pick.Skip`](ds:block?id=pick-skip-6g3huthr) has a typed `Reason` (`SkipUsageError`, `SkipPerToken`, `SkipPolicy`) and the `Err` behind it. When nothing is eligible, the error matches `pick.ErrNoCandidate` and lists every reason.

[`Result.Explain()`](ds:block?id=pick-result-explain-8z8jycgs) is one line for a log or a status bar: `auto → work (max 9%) over personal (max 77%)`. When the candidates come from several agents, homes are named `claude:work`, `codex:personal`.

<!-- snippet: examples/pick/main.go#explain -->
```go
// printResult prints the one-line explanation, then every skipped home
// with its typed reason and cause.
func printResult(w io.Writer, res pick.Result) {
	fmt.Fprintln(w, res.Explain())
	for _, s := range res.Skipped {
		fmt.Fprintf(w, "  skipped %s:%s (%s): %v\n", s.Home.Agent(), s.Home.Name(), s.Reason, s.Err)
	}
}
```

## Step 3 — Launch on the chosen home

`Result.Chosen.Home` is an `agentio.Home`, so its `Target()` launches on that account: the launch builder applies the home rule (`CLAUDE_CONFIG_DIR` set for a non-default home, absent for the default one).

<!-- snippet: examples/pick/main.go#launch -->
```go
// launchChosen builds the launch command on the chosen home: its Target
// carries the home, so the child gets the right CLAUDE_CONFIG_DIR (or none
// for the default home). The command is printed, never run.
func launchChosen(w io.Writer, res pick.Result, owners map[agentio.Home]agentio.Agent, tidy *strings.Replacer) error {
	a := owners[res.Chosen.Home]
	cmd, err := a.Launch(res.Chosen.Home.Target(), agentio.LaunchRequest{Prompt: "fix the failing tests"})
	if err != nil {
		return err
	}
	home, _ := agentio.EnvLookup(cmd.Env, claude.ConfigDirEnv)
	fmt.Fprintf(w, "launch: %s (%s=%s)\n", cmd, claude.ConfigDirEnv, tidy.Replace(home))
	return nil
}
```

## Step 4 — Pick from cached usage

For a loop, a status bar or a shell prompt, read the usage the vendor CLIs left on disk instead. It costs no request and no quota. Pass [`pick.WithNow(host.Now())`](ds:block?id=pick-withnow-n9dnp45x) so Explain says how old each cached reading is: `max 6%, cached 16d ago`. pick never reads the clock itself.

<!-- snippet: examples/pick/main.go#cached -->
```go
// errNoCachedUsage marks a home whose vendor CLI left no usage on disk.
var errNoCachedUsage = errors.New("no cached usage on disk")

// cachedCandidates reads the usage each vendor CLI left on disk: no
// network and no quota, safe in a loop or a status bar. Each reading keeps
// its Source and ObservedAt, so Explain can say how old it is.
func cachedCandidates(agents []agentio.Agent) ([]pick.Candidate, error) {
	var cands []pick.Candidate
	for _, a := range agents {
		cu, ok := agentio.CachedUsageOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return nil, err
		}
		for _, home := range homes {
			u, found, err := cu.CachedUsage(home)
			if err == nil && !found {
				err = errNoCachedUsage
			}
			cands = append(cands, pick.Candidate{Home: home, Usage: u, Err: err})
		}
	}
	return cands, nil
}
```

## Step 5 — Use your own policy

A [`pick.Policy`](ds:block?id=pick-policy-g29b2nb5) has a `Name` and a `Score(c Candidate) (score float64, ok bool, reason string)`: lower is better, and `ok=false` skips the candidate with `SkipPolicy` and your reason. Pass it with [`pick.WithPolicy`](ds:block?id=pick-withpolicy-vgrumqb5); Explain then names it (`auto [reserve] → work (reserve 9)`). This one keeps any home with a window at or over a limit in reserve:

<!-- snippet: examples/pick/main.go#policy -->
```go
// reserveLimit keeps a home out of the running once any window reaches it.
const reserveLimit = 50

// reserve is a custom pick.Policy: a home whose busiest window is at or
// over limit percent is refused (kept in reserve), the rest score like the
// default MaxPercent. Lower is better; ok=false skips with the reason.
type reserve struct{ limit float64 }

// Name labels the policy in Explain. Pure.
func (reserve) Name() string { return "reserve" }

// Score refuses a home at or over the limit. Pure.
func (r reserve) Score(c pick.Candidate) (float64, bool, string) {
	if m := c.Usage.MaxPercent(); m >= r.limit {
		return 0, false, fmt.Sprintf("a window is at %.0f%%, the reserve starts at %.0f%%", m, r.limit)
	}
	return c.Usage.MaxPercent(), true, ""
}
```

## Try it

```sh
cd examples && go run ./pick
```

```text
== live usage ==
auto → claude:work (max 9%) over claude:personal (max 77%)
  skipped codex:personal (usage-error): codex: access token expired at 2026-09-21T14:13:20Z (run codex to refresh it): agentio: login expired
launch: claude -- "fix the failing tests" (CLAUDE_CONFIG_DIR=~/.claude-work)
== cached usage ==
auto → claude:personal (max 6%, cached 16d ago) over codex:personal (max 12%, cached 30d ago)
  skipped claude:work (usage-error): no cached usage on disk
== custom policy ==
auto [reserve] → claude:work (reserve 9)
  skipped claude:personal (policy): pick: refused by policy: a window is at 77%, the reserve starts at 50%
  skipped codex:personal (usage-error): codex: access token expired at 2026-09-21T14:13:20Z (run codex to refresh it): agentio: login expired
== nothing eligible ==
no candidate: true
pick: no eligible home: claude:personal: pick: refused by policy: a window is at 77%, the reserve starts at 1%; claude:work: pick: refused by policy: a window is at 9%, the reserve starts at 1%; codex:personal: codex: access token expired at 2026-09-21T14:13:20Z (run codex to refresh it): agentio: login expired
```

## Pitfalls

- **Live usage spends requests, not quota, and may 429.** `agentio.UsageOf` makes one request per home; read it once per user action and never in a loop. A 429 arrives as the candidate's error (`agentio.ErrRateLimited`), so that home is skipped rather than guessed.
- **Never loop probes.** `TokenProberOf` spends real quota on every call; pick needs usage, not a probe.
- **Cached usage can be old.** It is as fresh as the vendor CLI's last refresh (16 and 30 days in the fixtures); pass `WithNow` so Explain shows the age, and prefer live usage when a wrong pick is expensive.
- **A home over 100% is still a candidate.** A window above its cap scores above 100 and loses to any home with room, but if every home is capped the least capped one still wins. Use a policy that refuses capped homes (like the reserve above) when you would rather not launch at all.
- **Ties keep your order.** Put the home you prefer first; pick never reorders equal scores.
- **No windows scores 0.** A subscription home whose usage has no windows looks idle and wins; per-token homes are skipped instead, because they report no windows by design.

## Related

- [Usage and subscription](usage.md), [Live or cached usage](../choosing/live-or-cached-usage.md)
- [Targets and commands](../core/launch.md) (launching the chosen home), [Homes](../core/homes.md)
- Example: [`examples/pick`](../../examples/pick/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
