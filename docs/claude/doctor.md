# Claude doctor

`a.Doctor(ctx, h, p, opts)` runs health checks for one Claude home (and optionally one project) and returns them as `[]agentio.Check`: is the binary found and not shadowed, is the version tested, is there a login and has it expired, do the settings files parse, are enabled plugins installed, are project MCP servers waiting for approval, and more. Use it for a `doctor` command, a setup wizard, or a support bundle. It reads disk only, unless you hand it a Runner.

## Before you start

- `go get github.com/ubgo/agentio/claude`.
- A `Host` with `Env` (for `PATH`), `Now` (expiry and retention checks), `Secrets` (login check) and `Processes` (stale session check). Missing inputs skip or soften the checks that need them.
- Optionally a project from `a.Project(dir)` for project-level checks.

The snippet comes from [`examples/claudelaunch`](../../examples/claudelaunch/). It reads the checked-in 2.1.325 fixture home with a fake native install on `PATH` and a clock set just after the fixture login expired, and passes a Runner whose process runner is a canned fake.

## Step 1 — Run the checks

Each `agentio.Check` has an `ID` (`agentio.CheckID`), a `Status` (`agentio.CheckOK`, `CheckInfo`, `CheckWarn`, `CheckFail`, or `CheckSkip`) and a `Message` that names files and counts, never values. The shared checks come first (`binary-found`, `home`, `home-env`, `api-billing-env`, `version-tested`, `login`; `home-env` and `api-billing-env` only when a variable is set), then Claude's own:

| ID | Constant | Status when it fires |
|---|---|---|
| `binary-shadowed` | `claude.CheckBinaryShadowed` | warn: other `claude` entries later on `PATH` |
| `shell-function` | `claude.CheckShellFunction` | info: `claude` is also a shell function in the newest shell snapshot |
| `version-min` | `claude.CheckVersionMin` | fail: installed version below the settings' `minimumVersion` |
| `home-env-default` | `claude.CheckHomeEnvDefault` | warn: `CLAUDE_CONFIG_DIR` names the default home (Claude Code then looks logged out) |
| `login-expired` | `claude.CheckLoginExpired` | warn: the stored login's expiry has passed |
| `settings-parse` | `claude.CheckSettingsParse` | fail: a settings file does not parse; warn: keys changed type or an unknown hook event |
| `global-config-parse` | `claude.CheckGlobalConfigParse` | fail / warn: `.claude.json` does not parse or keys changed type |
| `global-config-size` | `claude.CheckGlobalConfigSize` | warn: `.claude.json` above 8 MiB |
| `mcp-project-pending` | `claude.CheckMCPProjectPending` | info: `.mcp.json` servers not approved yet (needs a project) |
| `plugins-missing` | `claude.CheckPluginsMissing` | warn: plugins enabled in settings but not installed |
| `stale-live-sessions` | `claude.CheckStaleLiveSessions` | info: `sessions/*.json` entries whose process is gone |
| `retention` | `claude.CheckRetention` | info: transcript retention days and last cleanup; warn when cleanup is overdue |
| `history-size` | `claude.CheckHistorySize` | info: `history.jsonl` size and number of project history dirs |
| `auth-status` | `claude.CheckAuthStatus` | only with `DoctorOptions.Runner`: result of `claude auth status` |

`claude.DoctorOptions{Runner: a.Runner()}` opts into the one check that runs the binary; with the zero `DoctorOptions` no check runs `claude`. The neutral [`agentio.DoctorOf`](../capabilities/doctor.md#step-1--run-the-checks-for-a-home) returns the disk-only checks for any agent.

<!-- snippet: examples/claudelaunch/main.go#doctor -->
```go
// doctor computes the health checks from disk reads. With a Runner in
// DoctorOptions it also runs `claude auth status`; without one it never
// runs the binary. Messages carry names and counts, never values.
func doctor(ctx context.Context, w io.Writer, a *claude.Agent, h claude.Home, dir string, s scrub) error {
	p, err := a.Project(dir)
	if err != nil {
		return err
	}
	for _, c := range a.Doctor(ctx, h, &p, claude.DoctorOptions{Runner: a.Runner()}) {
		fmt.Fprintf(w, "check %-20s %-5s %s\n", c.ID, c.Status, s.path(c.Message))
	}
	return nil
}
```

## Try it

```sh
cd examples && go run ./claudelaunch
```

The lines for this page (machine paths are shown as `~` and `<tmp>` by the example):

```text
check binary-found         ok    <tmp>/bin/claude
check home                 ok    ~/.claude
check version-tested       ok    2.1.325 is tested
check login                ok    login in file
check login-expired        warn  login expired at 2026-11-13T20:56:01Z; run claude in this home to refresh it
check retention            info  transcripts kept 30 days; last cleanup 4 h ago
check history-size         info  history.jsonl 0 MiB; 2 project history dirs
check auth-status          ok    auth status: logged in (claude.ai)
```

## Pitfalls

- **Only firing checks are listed.** Most checks appear only when they find something; their absence means "fine" (for example no `plugins-missing` or `home-env` line). `binary-found`, `home`, `version-tested`, `login` and `retention` always appear; `history-size` whenever `history.jsonl` exists.
- **Doctor never fails as a call.** It returns checks, not an error; a check that cannot read its input is skipped. Inspect `Status`, not the slice length.
- **The clock matters.** With no `Host.Now`, expiry is never reported and the retention message has no "last cleanup"; the example sets a clock after the fixture login's expiry, which is why `login-expired` fires.
- **The auth-status check may touch the login.** It runs `claude auth status`; pass a Runner only when the user agreed to run the binary.
- **Project checks need a project.** `mcp-project-pending` and project-level plugin and settings checks use `p`; pass nil to check the home only.
- **Messages may name paths.** They never carry tokens, emails or settings values, but they do contain file paths; scrub them before sharing a report outside the machine (the example replaces the home and temp dirs).

## Related

- [Claude install and updates](install.md), [Claude credentials and tokens](credential.md), [Claude settings](settings.md), [Claude live sessions](sessions.md), [Claude Runner](runner.md)
- Neutral view: [Doctor checks](../capabilities/doctor.md)
- Example: [`examples/claudelaunch`](../../examples/claudelaunch/)
- [Feature index](../reference/features.md), [Choosing guides](../choosing/README.md)
