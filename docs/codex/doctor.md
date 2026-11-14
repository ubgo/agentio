# Codex doctor

`Checks` runs a disk-only health check of one Codex home: binary on `PATH`, home present, home env, API billing env, installed version against the tested range, `config.toml` parse, login presence and expiry, v2 profile files that collide with legacy profiles, and a stale update cache. Use it for a status page or a `doctor` subcommand of your own; it never runs `codex`.

## Before you start

- Module `github.com/ubgo/agentio/codex`; a `codex.Agent` and `codex.Home` from [The Codex agent](agent.md).
- The same checks are available to agent-agnostic code through `agentio.DoctorOf(a.Neutral())`.

The snippet comes from [`examples/codexconfig`](../../examples/codexconfig/).

## Step 1 — Run the checks

Each `agentio.Check` has an `ID` (neutral IDs like `binary-found`, `home`, `version-tested`, `login`, plus Codex's `codex.CheckConfigParse` (`config`), `codex.CheckProfileConflict` (`profile-conflict`), `codex.CheckStaleUpdate` (`update-cache`), `codex.CheckKeyring` (`keyring-login`)), a `Status` (`ok`, `info`, `warn`, `fail`), a `Message`, and an optional `Fix` command.

<!-- snippet: examples/codexconfig/hooks.go#checks -->
```go
// runChecks runs the disk-only doctor: it never calls a Runner method.
func runChecks(w io.Writer, m machine) {
	a := codex.New(m.host)
	for _, c := range a.Checks(context.Background(), a.DefaultHome()) {
		msg := strings.ReplaceAll(c.Message, m.userHome, "~")
		fmt.Fprintf(w, "%-5s %-18s %s\n", c.Status, c.ID, msg)
		if c.Fix != nil {
			fmt.Fprintf(w, "      fix: %s\n", argv(*c.Fix))
		}
	}
}
```

## Try it

```sh
cd examples && go run ./codexconfig
```

The `== doctor` section (the fixture clock is past the fixture token's expiry, and the fixture has a legacy profile):

```text
== doctor
ok    binary-found       ~/npm/bin/codex
ok    home               ~/.codex
ok    version-tested     0.162.1 is tested
ok    config             ~/.codex/config.toml
info  login              access token expired at 2026-09-21T14:13:20Z; codex refreshes it on its next run
warn  profile-conflict   legacy profiles in config.toml: ollama-launch; `codex -p NAME` (profile v2, since 0.131.0) refuses while a legacy [profiles.NAME] table or `profile = "NAME"` exists — move each into <home>/NAME.config.toml and remove the legacy table/selector
info  update-cache       version.json latest 0.156.1 is older than installed 0.162.1 (stale cache, not an update)
```

## Pitfalls

- **Never a Runner call.** The version check uses the version from `package.json` (as [`Install`](install.md#step-1--read-the-install) reads it); with no npm-family install the version check warns instead of running `codex --version`.
- **An expired access token is `info`, not a failure.** Codex refreshes it on its next run; the library never refreshes it, because refreshing rotates the refresh token.
- **Keyring homes cost one keychain read.** For `cli_auth_credentials_store = "keyring"` or `"auto"`, the login check reads the OS keyring through `Host.Secrets`; a failed or unparsable read reports under `keyring-login`.
- **The check list varies with the machine.** Checks that do not apply are left out, so match on `ID`, not on position or count.
- **The vendor has its own doctor.** `DoctorCommand(h, json)` builds `codex doctor [--json]`, which may touch the network and auth when run ([launch](launch.md#step-3--build-archive-delete-and-queue-commands)).

## Related

- [Doctor (neutral)](../capabilities/doctor.md), [Compat](../compat/compat.md)
- [Codex install and updates](install.md), [Codex config.toml](config.md#step-3--read-profiles), [Codex auth](auth-usage.md)
- [Claude doctor](../claude/doctor.md)
- [Choosing guides](../choosing/README.md), [feature index](../reference/features.md)
