# pick

Chooses the agent home with the most headroom with `agentio/pick`: reads each home's usage, builds a `pick.Candidate` per home, picks with `pick.Best`, prints `Result.Explain()` and every skipped home with its typed reason, and builds the launch command on the chosen home with `Agent.Launch`. It runs the same choice on live usage (`agentio.UsageOf`, one request per home) and on cached usage (`agentio.CachedUsageOf`, disk only, with the reading's age), then with a custom policy that keeps busy homes in reserve.

The homes are temp copies of the fixtures: the Claude personal and work homes (the example gives the work home a placeholder login) and the Codex home, whose login has expired at the example's fixed clock. An in-process fake usage API answers per login (personal busy, work quiet), so nothing leaves the process and no quota is spent.

## Run

```sh
cd examples && go run ./pick
```

## Output

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

## Test

```sh
cd examples && go test ./pick/
```

The test runs the same steps against the fake API and fixture copies and checks the choice, the skips, the launch and the custom policy.

## Guides

- [Pick the home with the most headroom](../../docs/capabilities/pick.md)
- [Usage and subscription](../../docs/capabilities/usage.md), [Live or cached usage](../../docs/choosing/live-or-cached-usage.md)
- [Feature index](../../docs/reference/features.md)
