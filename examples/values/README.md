# values

The small value types: error sentinels and rate limits (`errors.go`), `Opt[T]`, `Unknown` keys and `Warnings` (`values.go`), `Version` parsing and comparison (`versions.go`), closed and open enums (`enums.go`).

Everything is Pure: no file, network or process.

## Run

```sh
cd examples && go run ./values
```

## Output

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
opt: theme="dark" set=true
opt: verbose set=false or(true)=true
opt: retries set=false (null reads as unset)
opt: json={"theme":"dark","verbose":false}
unknown: modelled=[model effort]
unknown: keys=[fastMode theme]
unknown: theme raw={"name":"dark"}
unknown: none left is nil=true
warnings: clean read is nil=true
warning: type-changed: settings.json: cleanupPeriodDays: expected a number
warning: unknown-record: history.jsonl: record type "snapshot" skipped
warnings: has type-changed=true has untested-version=false
warnings: json={"kind":"type-changed","file":"settings.json","key":"cleanupPeriodDays","detail":"expected a number"}
parse "2.1.325 (Claude Code)": ok=true version="2.1.325" major=2 pre=""
parse "codex-cli 0.162.1": ok=true version="0.162.1" major=0 pre=""
parse "v1.2": ok=true version="1.2.0" major=1 pre=""
parse "1.0.0-beta.2": ok=true version="1.0.0-beta.2" major=1 pre="beta.2"
parse "x86_64 build 7": ok=true version="7.0.0" major=7 pre=""
parse "no version here": ok=false version="" major=0 pre=""
must: 2.1.323 typo-is-zero=true
sorted: [0.162.1 1.0.0-alpha 1.0.0-beta.2 1.0.0-beta.11 1.0.0]
compare: 2.1.325 vs 2.1.300 = 1 (at least minimum: true)
unknown: IsZero=true String=""
parse autonomy: accept-edits ok=true
parse autonomy yolo: ok=false
json autonomy: read-only
json autonomy yolo: agentio: unknown autonomy "yolo"
json out: {"autonomy":"full"}
out of range: autonomy(42)
install method npm: known=true
install method mise: known=false
autonomy values: [default read-only ask accept-edits full]
cost classes: [pure disk process network quota]
process kinds: [cli exec app-server mcp-server desktop bridge launcher command]
capabilities: 32, first installer
```

## Test

```sh
cd examples && go test ./values/
```

The test runs the same `run` function against the fixture and checks the key output lines.

## Guides

- [Errors](../../docs/core/errors.md)
- [Optional values, unknown keys and warnings](../../docs/core/values.md)
- [Versions](../../docs/core/versions.md)
- [Enums](../../docs/core/enums.md)
- [Feature index](../../docs/reference/features.md)
