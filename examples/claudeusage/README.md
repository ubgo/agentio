# claudeusage

Reads Claude plan usage four ways: the live usage endpoint (`Agent.Usage`, `Usage`, `Limit.Label`, `ParseUsage`, `ErrNoLimits`, `WithBaseURL`), the copy Claude Code cached in `.claude.json` (`Agent.CachedUsage`), the extra-usage object (`ExtraUsageOf`) and a token probe (`Prober.TokenUsage`, `ParseProbeHeaders`, `ErrProbeHeaders`, `agentio.ErrBilledPerToken`).

Every network call goes to a local `httptest` server through `Agent.WithBaseURL`: the fake usage endpoint serves the body the fixture home `claude/testdata/homes/2.1.325/userhome` cached, the fake `/v1/messages` answers with rate-limit headers. Nothing reaches the vendor and no quota is spent.

## Run

```sh
cd examples && go run ./claudeusage
```

`go test ./claudeusage/` runs the same code and checks key output lines.

## Output

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

## Guides

- [Claude plan usage](../../docs/claude/usage.md)
- [Feature index](../../docs/reference/features.md)
