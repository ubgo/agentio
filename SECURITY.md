# Security Policy

agentio reads the local state of coding-agent CLIs (Claude Code, OpenAI Codex): their homes, settings, logins, usage and conversations. Credentials pass through it, so a leak or an unexpected write is a security bug. This page says how to report one and which guarantees the library makes about secrets.

## Reporting a vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Report them privately:

- Use **[GitHub Security Advisories](https://github.com/ubgo/agentio/security/advisories/new)** (preferred), or
- Email **khanakia@gmail.com** with the details.

Please include:

- A description of the vulnerability and its impact.
- The module and function involved (`agentio`, `claude`, `codex`, `codex/state`, `builtin`).
- Steps to reproduce, ideally a Go test against a fixture home or a temp dir.
- Affected module version(s) and OS.

Never include a real token, credential file, keychain entry or transcript in a report. Replace every value; the shape is enough.

You will get an acknowledgement within a few days and updates while a fix is prepared. Please allow a reasonable window for a fix before any public disclosure; reporters are credited in the advisory unless they ask otherwise.

## What counts as a vulnerability

Any of these is in scope:

- A token, API key, MCP server env value or personal value (email, account id, org name) appearing in argv, an error message, `String()` / `fmt` output, JSON, or `slog` output.
- Any library code path that refreshes, writes, copies or prints a vendor login.
- Any write to a file a vendor CLI owns that did not go through a `Writer` door, or that did not back the target up first.
- A read of `$HOME`, the environment, the keychain, the clock or the network that bypasses `Host`.
- A vendor binary run without the caller asking for it through a `Runner` door.
- A network call other than a GET, outside the `Prober` door.

## Secret handling guarantees

These are library invariants. Each one is guarded by a test that fails when the guard is removed (`internal/archtest`, the root tests and the `agenttest` conformance suite).

- **Never refreshes, writes or prints an OAuth token.** Refreshing rotates the refresh token and logs the vendor CLI out, so agentio only reads logins. It never mints, refreshes, copies or stores them.
- **Secrets travel as redacting types.** `agentio.Token` (credentials), `agentio.EnvValue` (MCP server env values) and `agentio.Personal` (emails, names, account and org ids) print `[redacted]` from `String`, `GoString`, every `fmt` verb, `MarshalJSON` and `LogValue`. The value is an unexported field; it leaves the type only through an explicit `Reveal()`, which archtest confines to the few places that need it (an HTTP auth header, decoding a credential blob the keychain returned).
- **Tokens never go on argv.** A token login is applied to `Command.Env` only, the one place a child process needs it, and the vendor's other credential variables are scrubbed. `Command.Redacted()` masks every secret-bearing env entry before you print a command. `MCPAddCommand` refuses a server with env values or headers (`ErrUnsupported`) instead of putting them on the `mcp add` argv.
- **Read-only by default.** Readers and launch builders never write. Writing anything a vendor CLI owns is a separate, explicit method on the `Writer` door.
- **Every write backs up first.** A `Writer` method that changes an existing file copies it to `<file>.agentio-bak-<UTC stamp>` (never overwriting an earlier backup) and returns an `agentio.Backup` listing what it copied. Moves back up every source item and roll renames back on failure. Linking shared files into a home only creates symlinks where nothing exists and never replaces a file, so it has nothing to back up.
- **No hidden state.** Nothing reads `$HOME`, `os.Environ`, the keychain, the clock or the network except through `agentio.Host`. `agentio.HostFromOS()` is the one function that touches the real machine; tests build a `Host` from fixtures. There are no package-level variables and no global registry.
- **Costly calls sit behind doors.** Running the vendor binary (`Runner`), spending model quota (`Prober`) and writing (`Writer`) are each reached only through their own door, so a loop that uses the plain agent and the read accessors cannot spawn a vendor process, spend quota or write. Keychain reads only look entries up; they never write or list values.
- **Usage endpoints are never retried in a loop.** A 429 maps to `ErrRateLimited`; callers cache, never poll.

## Supported versions

agentio is pre-1.0. Security fixes land in the latest version of each affected module (the root, `claude`, `codex`, `codex/state`, `builtin` are tagged separately) as a PATCH release. Older versions are not patched; upgrade to stay supported.

| Version | Supported |
|---|---|
| latest of each module | ✅ |
| older | ❌ |
