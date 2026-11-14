# Doctor checks

A doctor check is one line of a health report for a home: is the binary on `PATH`, is the home really a home, is the installed version one the library was tested against, is anyone logged in. The `Doctor` capability runs every check that needs only the disk; `agentio.VersionCheck` is the shared `version-tested` check you can call on any version. Show the result in a `doctor` command or before a launch that would fail.

## Before you start

- Modules: `github.com/ubgo/agentio` plus the agents; `claude.Compat()` and `codex.Compat()` for the tested ranges.
- The homes on disk, and the Host's `PATH` for the binary check.
- Doctor checks never run the vendor binary. Checks that need it (such as `claude auth status`) live on the vendor Runner; see [Doors](doors.md).

[`examples/accessors`](../../examples/accessors/) runs the checks on copies of the fixture homes with a fake install on `PATH`.

## Step 1 — Run the checks for a home

`DoctorOf(a)` returns a `Doctor`; `Checks(ctx, home)` returns `[]agentio.Check`. Each check has an `ID` (a `CheckID`), a `Status` (a `CheckStatus`: `skip`, `ok`, `info`, `warn`, `fail`), a `Message`, and an optional `Fix`: a `*Command` that would fix it, for you to offer, never run silently.

<!-- snippet: examples/accessors/doctor.go#doctor -->
```go
// printDoctor runs the disk-only doctor checks of every home. A Fix, when
// set, is a Command the caller may offer to run; nothing here runs it.
func printDoctor(ctx context.Context, w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	for _, a := range agents {
		d, ok := agentio.DoctorOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return err
		}
		for _, home := range homes {
			for _, c := range d.Checks(ctx, home) {
				line := fmt.Sprintf("%s %s %-5s %-15s %s", a.ID(), home.Name(), c.Status, c.ID, tidy(h, c.Message))
				if c.Fix != nil {
					line += " (fix: " + c.Fix.String() + ")"
				}
				fmt.Fprintln(w, line)
			}
		}
	}
	return nil
}
```

The shared `CheckID`s are `binary-found`, `home`, `home-env`, `api-billing-env`, `version-tested` and `login`; `CheckIDValues()` lists them. Vendors add their own (`stale-live-sessions`, `retention`, `profile-conflict`, ...), so `CheckID` is an open set: `IsKnown()` is false for a vendor ID, and you should still show it. Every agent has a doctor, including config-defined ones, which run the L1 checks from their Spec.

## Step 2 — Check the installed version

`agentio.VersionCheck(compat, installed)` compares a version with a vendor's tested range (`agentio.Compat`, from `claude.Compat()` or `codex.Compat()`): `ok` inside the range, `warn` for newer, older or unknown. The built-in doctors already include it; call it yourself when you have a version from elsewhere, for example from `BinaryVersionOf`.

<!-- snippet: examples/accessors/doctor.go#versioncheck -->
```go
// printVersionCheck compares the installed version (read from disk, never
// by running the binary) with each vendor module's tested range.
func printVersionCheck(w io.Writer, agents builtin.Agents) {
	if inst, err := agents.Claude.Install(); err == nil {
		c := agentio.VersionCheck(claude.Compat(), inst.Version)
		fmt.Fprintf(w, "claude %s %s\n", c.Status, c.Message)
	}
	if inst, err := agents.Codex.Install(); err == nil {
		c := agentio.VersionCheck(codex.Compat(), inst.Version)
		fmt.Fprintf(w, "codex  %s %s\n", c.Status, c.Message)
	}
	old := agentio.VersionCheck(claude.Compat(), agentio.MustVersion(untestedClaude))
	fmt.Fprintf(w, "claude %s %s\n", old.Status, old.Message)
	unknown := agentio.VersionCheck(claude.Compat(), agentio.Version{})
	fmt.Fprintf(w, "claude %s %s\n", unknown.Status, unknown.Message)
}
```

## Try it

```sh
cd examples && go run ./accessors
```

The doctor and version check sections of the output:

```text
== doctor ==
claude personal ok    binary-found    /opt/fake/bin/claude
claude personal ok    home            ~/.claude
claude personal ok    version-tested  2.1.325 is tested
claude personal ok    login           login in file
claude personal info  stale-live-sessions 2 sessions/*.json entries whose process is gone
claude personal warn  retention       transcripts kept 30 days; last cleanup at 2026-11-13T19:30:55Z is after the host clock (clock skew)
claude personal info  history-size    history.jsonl 0 MiB; 2 project history dirs
claude work ok    binary-found    /opt/fake/bin/claude
claude work ok    home            ~/.claude-work
claude work ok    version-tested  2.1.325 is tested
claude work warn  login           no login found in the credentials file or keychain
claude work info  stale-live-sessions 2 sessions/*.json entries whose process is gone
claude work info  retention       transcripts kept 30 days
claude work info  history-size    history.jsonl 0 MiB; 1 project history dirs
codex personal ok    binary-found    /opt/fake/bin/codex
codex personal ok    home            ~/.codex
codex personal ok    version-tested  0.162.1 is tested
codex personal ok    config          ~/.codex/config.toml
codex personal info  login           access token expired at 2026-09-21T14:13:20Z; codex refreshes it on its next run
codex personal warn  profile-conflict legacy profiles in config.toml: ollama-launch; `codex -p NAME` (profile v2, since 0.131.0) refuses while a legacy [profiles.NAME] table or `profile = "NAME"` exists — move each into <home>/NAME.config.toml and remove the legacy table/selector
codex personal info  update-cache    version.json latest 0.156.1 is older than installed 0.162.1 (stale cache, not an update)
== version check ==
claude ok 2.1.325 is tested
codex  ok 0.162.1 is tested
claude warn 2.0.0 is untested (tested 2.1.323..2.1.325)
claude warn installed version unknown; tested 2.1.323..2.1.325
```

The fixture's `.last-cleanup` stamp is later than the example's fixed clock, so `retention` warns about clock skew instead of printing an age; on a real machine the clock is later than the stamp.

## Pitfalls

- **`Checks` returns no error.** A failure to read something becomes a check with status `fail` or `warn`; a home of another agent becomes a `home` check that fails. Always print every check.
- **Statuses are not all problems.** `info` lines (stale session entries, retention, a set home variable) are facts; reserve red for `fail` and yellow for `warn`.
- **Version checks use the version on disk.** If `Install` cannot read a version (brew cask, unknown layout), `version-tested` warns "installed version unknown". Run `BinaryVersionOf` (a Process call that may touch the login) only if the user asks, then call `VersionCheck` with its answer.
- **`home-env` is informational.** It means the home variable (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`) is set in the base environment; launches of the default home remove it, so it does not leak into child processes.
- **`api-billing-env` warns on names only.** Variables such as `ANTHROPIC_API_KEY` switch the vendor CLI to per-token billing; the check names them and never prints a value.
- **`Fix` is data.** It is a `Command` like any launch; run it attached to a terminal if the user accepts.

## Related

- [Install, version and identity](install-identity.md), [Shared capabilities and accessors](accessors.md).
- [Compat and version ranges](../compat/compat.md), [Versions](../core/versions.md).
- Vendor pages: [Claude doctor](../claude/doctor.md), [Codex doctor](../codex/doctor.md).
- [Config-defined agents](fromspec.md#step-3--launch-it): the L1 checks a Spec gives.
- [Choosing guides](../choosing/README.md). Example: [`examples/accessors`](../../examples/accessors/). Feature index: [features](../reference/features.md).
