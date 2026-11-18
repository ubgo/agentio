# claudelaunch

Builds Claude Code commands and runs the read-only checks: launch commands (`Agent.Launch`, `LaunchOptions`, `LaunchOptions.Args`, `ErrInvalidLaunch`, `PurgeCommand`), resume, continue and fork, the target version from disk (`TargetFor`), flags a version lacks (`FeatureUnavailableError`, version warnings), the Runner door (`BinaryVersion`, `Plugins`, `Marketplaces`, `BackgroundAgents`, `AutoModeConfig`, `AutoModeDefaults`) and `Agent.Doctor`.

Nothing is executed. The installed `claude` is an empty file in a temp dir laid out like a native install (`<tmp>` in the output), so the version is read from its path; the Runner door goes to a canned process runner keyed by argv. Homes come from the fixture `claude/testdata/homes/2.1.325/userhome` (`~` in the output); the clock is set just after the fixture login expired.

## Run

```sh
cd examples && go run ./claudelaunch
```

`go test ./claudelaunch/` runs the same code and checks key output lines.

## Output

```text
target: home personal, version 2.1.325
launch: claude --model opus --permission-mode plan --add-dir /work/shared -- "-explain the build"
  CLAUDE_CONFIG_DIR set: false, warnings: 0
headless args: -p --output-format stream-json --include-partial-messages --verbose
invalid: claude: invalid launch options: --output-format needs -p
purge: claude purge /work/old-project --dry-run
  CLAUDE_CONFIG_DIR set: false, warnings: 0
resume: claude --resume 3f474440-903e-5410-8bfb-d537b89f9547
  CLAUDE_CONFIG_DIR set: false, warnings: 0
continue: claude --continue
  CLAUDE_CONFIG_DIR set: false, warnings: 0
fork in work home: claude --resume 3f474440-903e-5410-8bfb-d537b89f9547 --fork-session --session-id 22222222-0000-4000-8000-000000000002
  CLAUDE_CONFIG_DIR set: true, warnings: 0
invalid: claude: invalid launch options: --resume and --continue are exclusive
unavailable: claude: --worktree needs Claude Code >= 2.1.49 (installed 2.1.40) (feature --worktree, since 2.1.49, matches ErrFeatureUnavailable: true)
unknown version: claude --worktree
  CLAUDE_CONFIG_DIR set: false, warnings: 1
  warning version-unknown: installed Claude Code version unknown; flags not checked
untested version: claude --worktree
  CLAUDE_CONFIG_DIR set: false, warnings: 1
  warning untested-version: installed 2.1.400 is outside tested 2.1.323..2.1.327
runner version: 2.1.325
runner plugin demo@example-market v1.2.0 (user), enabled true, installed 2026-11-01
runner marketplace example-market: github example/market
background agent ab12 "fix-tests": pid 4242, state blocked, cwd /work/proj4
auto mode config {"enabled": true}, defaults {"enabled": false}
check binary-found         ok    <tmp>/bin/claude
check home                 ok    ~/.claude
check version-tested       ok    2.1.325 is tested
check login                ok    login in file
check login-expired        warn  login expired at 2026-11-13T20:56:01Z; run claude in this home to refresh it
check retention            info  transcripts kept 30 days; last cleanup 4 h ago
check history-size         info  history.jsonl 0 MiB; 2 project history dirs
check auth-status          ok    auth status: logged in (claude.ai)
```

## Guides

- [Claude launch and resume](../../docs/claude/launch.md)
- [Claude Runner](../../docs/claude/runner.md)
- [Claude doctor](../../docs/claude/doctor.md)
- [Feature index](../../docs/reference/features.md)
