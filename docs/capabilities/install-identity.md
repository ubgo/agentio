# Install, version and identity

Three capabilities answer "is this agent here, which version, and who is signed in": `Installer` reads the install from disk, `BinaryVersion` runs `<binary> --version`, and `IdentityReader` reads a home's login state without the network. Reach them through `agentio.InstallerOf`, `agentio.BinaryVersionOf` and `agentio.IdentityOf` when you loop over agents, for a status line, a doctor screen or a picker that shows which account each home uses.

## Before you start

- Modules: `github.com/ubgo/agentio` plus the agents (`builtin`, or `claude` / `codex`).
- A Host whose `Env` holds the `PATH` to search. The library walks `PATH` itself; it never asks a shell.
- `BinaryVersion` needs `Host.Exec`. `HostFromOS` fills it with an `os/exec` runner; tests use `agenttest.FakeExec` or a canned runner like the one in [`examples/accessors`](../../examples/accessors/).
- `Identity` reads files in the home (and the macOS keychain for Claude through `Host.Secrets`).

The example lays out a fake native Claude Code install and a fake npm Codex install in a temp dir, each reached through a symlink in `bin/`, and answers `--version` from a canned runner, so no real binary is touched.

## Step 1 — Read the install

`Install()` finds the binary on `PATH`, resolves symlinks, recognises the install method from the real path (npm, bun, pnpm, brew, native, standalone) and reads the version from the package metadata on disk. It never runs the binary.

<!-- snippet: examples/accessors/install.go#install -->
```go
// printInstalls reads what is on disk for each agent's binary: the PATH hit,
// the real path behind the symlink, the install method and the version from
// package metadata. Disk · Read; the binary is never run.
func printInstalls(w io.Writer, h agentio.Host, agents []agentio.Agent) {
	for _, a := range agents {
		in, ok := agentio.InstallerOf(a)
		if !ok {
			continue
		}
		inst, err := in.Install()
		if err != nil {
			// ErrNotInstalled is an answer, not a failure of the loop.
			fmt.Fprintf(w, "%s: %v\n", a.ID(), err)
			continue
		}
		fmt.Fprintf(w, "%s: %s -> %s method=%s version=%s\n",
			a.ID(), tidy(h, inst.Path), tidy(h, inst.RealPath), inst.Method, inst.Version)
	}
}
```

`agentio.Install` has `Path` (the first PATH hit), `RealPath`, `Method` (an `InstallMethod`; unknown values are kept, `IsKnown` reports false), `Version` (zero when nothing on disk says) and `Shadowed`, the other PATH entries with the same name that never run. The vendor types (`claude.Install`, `codex.Install`) add more, such as Claude's update channel or Codex's package root; see [Claude install](../claude/install.md) and [Codex install](../codex/install.md).

## Step 2 — Run the binary for its version

When the disk does not know the version (a brew cask, a standalone build), run the binary. `BinaryVersionOf` is a separate accessor on purpose: it is the Process door for this one call, with cost `process read (may touch login)`.

<!-- snippet: examples/accessors/install.go#binaryversion -->
```go
// printBinaryVersions runs `<binary> --version` through Host.Exec. This is
// a Process call that may touch the login, so it has its own accessor and a
// caller reaches it only on purpose.
func printBinaryVersions(ctx context.Context, w io.Writer, agents []agentio.Agent) {
	for _, a := range agents {
		bv, ok := agentio.BinaryVersionOf(a)
		if !ok {
			continue
		}
		v, err := bv.BinaryVersion(ctx)
		if err != nil {
			fmt.Fprintf(w, "%s: %v\n", a.ID(), err)
			continue
		}
		fmt.Fprintf(w, "%s: %s\n", a.ID(), v)
	}
}
```

The call builds the command for the default home, runs it through `Host.Exec` and parses the first version in stdout (stderr as a fallback). A config-defined agent has it only when its Spec sets `VersionArgs`.

## Step 3 — Read the signed-in identity

`Identity(home)` reports `LoggedIn`, the account `Email` and `Org`, a display `PlanLabel`, the `Billing` kind, where the credential came from (`Credential`: file, keychain, keyring, env) and the access credential's `ExpiresAt`. It is never a network call; the conformance suite checks that.

<!-- snippet: examples/accessors/install.go#identity -->
```go
// printIdentities reads each home's login without the network. Email and
// Org are agentio.Personal and print redacted; Expired compares the stored
// expiry with the Host clock.
func printIdentities(w io.Writer, h agentio.Host, agents []agentio.Agent) error {
	for _, a := range agents {
		ir, ok := agentio.IdentityOf(a)
		if !ok {
			continue
		}
		homes, err := a.Homes()
		if err != nil {
			return err
		}
		for _, home := range homes {
			id, err := ir.Identity(home)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "%s %s: logged-in=%t email=%s plan=%q billing=%s credential=%s expired=%t\n",
				a.ID(), home.Name(), id.LoggedIn, id.Email, id.PlanLabel, id.Billing, id.Credential, id.Expired(h.Now()))
		}
	}
	return nil
}
```

`Email` and `Org` are `agentio.Personal`: they print, format, log and marshal as `[redacted]`. Call `Reveal()` only where the user asked to see them. `Expired(now)` is true only for a known expiry at or before `now`; pass `Host.Now()`, never `time.Now()`, so tests stay deterministic.

## Try it

```sh
cd examples && go run ./accessors
```

The install, binary version and identity sections of the output:

```text
== install ==
claude: /opt/fake/bin/claude -> /opt/fake/.local/share/claude/versions/2.1.325 method=native version=2.1.325
codex: /opt/fake/bin/codex -> /opt/fake/lib/node_modules/@openai/codex/bin/codex.js method=npm version=0.162.1
gemini: agentio: binary not found on PATH: gemini
pi: agentio: binary not found on PATH: pi
== binary version ==
claude: 2.1.325
codex: 0.162.1
gemini: agentio: gemini version: example: this host runs no programs except the canned --version answers
pi: agentio: pi version: example: this host runs no programs except the canned --version answers
== identity ==
claude personal: logged-in=true email=[redacted] plan="Max (20x)" billing=subscription credential=file expired=false
claude work: logged-in=false email= plan="" billing=unknown credential=none expired=false
codex personal: logged-in=true email=[redacted] plan="Plus" billing=subscription credential=file expired=true
```

The Claude work home has no login: that is an answer (`LoggedIn` false, nil error), not a failure. The Codex fixture's access token expired in September; Codex refreshes it on its next run, and agentio never does.

## Pitfalls

- **`Install` returns `ErrNotInstalled`** when the binary is not on the Host's `PATH`. Treat it as an answer in a loop, as the example does. Empty and relative `PATH` entries are skipped: running a binary from the current directory is never intended.
- **Find the real binary, not a shell function.** Some users wrap `claude` in a shell function or alias; `LookPath` walks `PATH` from `Host.Env` and finds the executable file, which is what a launched `Command` runs too.
- **`Install.Version` can be zero** (brew cask, unknown layout). Fall back to `BinaryVersionOf` only when the user wants the exact version: it runs the binary.
- **`BinaryVersion` may touch the login.** Vendor CLIs can refresh and rewrite the home's credentials when they start (Codex has been seen rewriting `auth.json`). Never call it in a loop over homes or on a timer.
- **No `Host.Exec`, no version.** A Host without a runner makes `BinaryVersion` fail with an error ("the host has no process runner"), never a guess.
- **`Identity` never refreshes a token.** An expired login reads as `Expired(now) == true`; the fix is to run the vendor CLI once in that home. Refreshing would rotate the refresh token and log the vendor CLI out.
- **Claude keychain lookups name the account first.** On macOS, `Host.User` is passed as the keychain account; stale entries for the same service exist on real machines. Set `Host.User` in hand-built Hosts.

## Related

- [Shared capabilities and accessors](accessors.md): the accessor pattern and costs.
- [Doors: Runner, Prober and Writer](doors.md): the vendor Runner behind `BinaryVersion`.
- [Doctor checks](doctor.md): `version-tested` compares the installed version with the tested range.
- Vendor pages: [Claude install](../claude/install.md), [Claude credential](../claude/credential.md), [Claude account](../claude/account.md), [Codex install](../codex/install.md), [Codex auth and usage](../codex/auth-usage.md).
- [Secrets and personal values](../core/secrets.md), [Versions](../core/versions.md).
- [Choosing guides](../choosing/README.md). Example: [`examples/accessors`](../../examples/accessors/). Feature index: [features](../reference/features.md).
