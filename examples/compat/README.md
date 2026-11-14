# compat

Shows the version side of agentio, all Pure (no disk, no process, no network):

- the tested vendor ranges: `claude.TestedMin`/`TestedMax`, `codex.TestedMin`/`TestedMax`, `claude.Compat()`;
- where an installed version falls: `Compat.Check` (`CompatInRange`, `CompatNewer`, `CompatOlder`, `CompatUnknown`) and `Compat.Warning`;
- the per-feature tables generated from `features.tsv`: `claude.Features`, `codex.FeatureValues`, `FeatureInfo`, `FeatureSource`, `FeatureSpan.Known` and `FeatureSpan.In`;
- the typed `claude.FeatureUnavailableError` / `codex.FeatureUnavailableError` a launch builder returns for a flag the installed version provably lacks, matching `agentio.ErrFeatureUnavailable`, and the untested-version warning for a newer version.

## Run it

```sh
cd examples && go run ./compat
```

```text
claude tested 2.1.323..2.1.325
codex  tested 0.162.1..0.162.1
claude.Compat() = {Min: 2.1.323, Max: 2.1.325}
check "2.1.325" -> in-range
check "2.1.400" -> newer (warning untested-version)
check "2.1.40"  -> older (warning untested-version)
check ""        -> unknown (warning version-unknown)
claude --from-pr                    since 2.1.27   Claude Code CHANGELOG.md 2.1.27
claude --worktree                   since 2.1.49   Claude Code CHANGELOG.md 2.1.49
claude --name                       since 2.1.76   Claude Code CHANGELOG.md 2.1.76
claude --plugin-dir .zip            since 2.1.128  Claude Code CHANGELOG.md 2.1.128 (behaviour)
claude --fallback-model interactive since 2.1.166  Claude Code CHANGELOG.md 2.1.166 (behaviour; print-only before)
claude --restricted                 since 2.1.248  Claude Code CHANGELOG.md 2.1.248
claude features: 67 rows, 19 with a known version
codex features: 39 rows, 9 with a known version
--worktree in "2.1.40" : false
--worktree in "2.1.325": true
--worktree in ""       : true
claude launch: claude: --worktree needs Claude Code >= 2.1.49 (installed 2.1.40)
  feature=--worktree installed=2.1.40 since=2.1.49 is ErrFeatureUnavailable=true
claude 2.1.400: built, untested warning=true
codex launch: codex 0.150.0: --no-daemon needs codex >= 0.156.0
```

Test: `cd examples && go test ./compat`.

## Guides

- [Tested vendor versions](../../docs/compat/compat.md)
- [Per-feature version table](../../docs/compat/features.md)
- [Feature index](../../docs/reference/features.md#compat-and-drift)
