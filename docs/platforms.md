# Platforms: macOS, Linux and Windows

agentio runs on macOS, Linux (glibc and musl: Ubuntu, Debian, Fedora, Arch, Alpine) and Windows 10/11. Every feature either works on an OS or returns a typed `agentio.ErrUnsupported` whose message says why. Every rule that differs by OS follows `Host.GOOS`, not the OS the program was built for, so one test suite proves the Windows rules on a Linux machine and the other way round.

## Before you start

- Build a Host with [`agentio.HostFromOS`](ds:block?id=hostfromos-yvmqkxwe): it sets `GOOS` from `runtime.GOOS` and `SystemRoot` to `/` on unix or the system drive (`C:\`) on Windows.
- A Host you build yourself with no `GOOS` uses the rules of the OS the program was built for and has no system locations ([Host](core/host.md)).

## Support table

| Feature | macOS | Linux | Windows |
|---|---|---|---|
| Find the binary on `PATH` | `:`-separated, exec bit | same | `;`-separated, `PATHEXT` extensions (`claude.exe`, `codex.cmd`), no exec bit |
| Environment names | case-sensitive | case-sensitive | case-insensitive (`Path` is `PATH`); edits remove every spelling |
| Default homes | `~/.claude`, `~/.codex` | same | `%USERPROFILE%\.claude`, `%USERPROFILE%\.codex` |
| Default home rule | `CLAUDE_CONFIG_DIR` / `CODEX_HOME` absent | same | same, removed in any letter case |
| Claude login | keychain, then `.credentials.json` | `<home>/.credentials.json` only | `<home>\.credentials.json` only |
| Codex keyring login | keychain (`security`) | Secret Service through `secret-tool`; `ErrUnsupported` when it is not installed or no D-Bus session answers | `ErrUnsupported` (Credential Manager needs `CredReadW`); `auto` store falls back to `auth.json` |
| Claude managed settings | `/Library/Application Support/ClaudeCode/`, managed plist (`Runner.ManagedPlist`) | `/etc/claude-code/` | `C:\Program Files\ClaudeCode\`, policy registry `HKLM`/`HKCU\SOFTWARE\Policies\ClaudeCode` (`Runner.ManagedRegistry`) |
| Codex system config | `/etc/codex/config.toml`, `managed_config.toml` | same | `%ProgramData%\OpenAI\Codex\config.toml`; no system managed file |
| Claude `settings.local.json` | repo root | repo root | the start folder (beside `.claude/settings.json`) |
| Claude history key | every UTF-16 unit not `[A-Za-z0-9]` → `-`, cut at 200 with a hash | same | same (`C:\Users\x\proj` → `C--Users-x-proj`) |
| Process table | one `ps` run | `/proc` read directly (no `ps`: works on Alpine and minimal images) | one PowerShell `Get-CimInstance Win32_Process` run |
| Process working directory | `lsof` | `/proc/<pid>/cwd` | `ErrUnsupported` (only in the process's memory); live sessions still match through their session files |
| Install detection | native, npm, nvm, bun, pnpm, Volta, Homebrew cask/formula, standalone | same (Homebrew on Linux: formula) | native `claude.exe`, npm `.cmd` shims in `%APPDATA%\npm`, bun `.exe`, pnpm and Volta |
| Home links (symlinks) | `os.Symlink` | same | needs Developer Mode or an elevated process, else `*agentio.SymlinkPrivilegeError` (matches `ErrUnsupported`) |
| `Command.String()` | POSIX sh quoting, display only | same | same: not PowerShell or cmd quoting; run `Path`/`Args` directly |
| Text files | UTF-8, LF | same | a UTF-8 BOM and CRLF line ends are accepted everywhere |

## Step 1 — Let the Host pick the rules

[`Host.LookPath`](ds:block?id=host-lookpath-4k46e3ag) walks `Host.Env` `PATH` under `Host.GOOS`: on Windows it splits on `;`, tries every `PATHEXT` extension for a bare name and needs no exec bit, so `codex` finds `%APPDATA%\npm\codex.cmd`. [`Host.Getenv`](ds:block?id=host-getenv-qyjtfrc3) compares names case-insensitively on Windows, and [`Host.NewCommand`](ds:block?id=host-newcommand-pu5aa4ej) builds a child env where an edit removes every spelling of the name, so the default home rule unsets `codex_home` as well as `CODEX_HOME`. The pure forms take the OS as an argument: `LookPathAllFor`, `EnvLookupFor`, `ApplyEnvFor`, `NewCommandFor`, `PathListSeparatorFor`.

```go
// Rules for Windows, wherever this runs: PATHEXT finds the npm shim.
h := agentio.Host{GOOS: agentio.OSWindows, Env: []string{`Path=C:\Users\u\AppData\Roaming\npm`, "PATHEXT=.COM;.EXE;.BAT;.CMD"}}
p, err := h.LookPath("codex") // C:\Users\u\AppData\Roaming\npm\codex.cmd on that machine
```

## Step 2 — Detect the install on each OS

`claude.Agent.Install` and `codex.Agent.Install` resolve the PATH hit through symlinks and launcher shims. npm on Windows and pnpm everywhere write a small script instead of a symlink; [`agentio.ShimTarget`](ds:block?id=shimtarget-ubxq8m56) reads it and returns the package entry it runs, so the version still comes from that package's `package.json`. bun on Windows (`~/.bun/bin/codex.exe`) and Volta (`~/.volta/bin`, `%LOCALAPPDATA%\Volta\bin`) are recognised by their bin dir, and [`agentio.InstallVolta`](ds:block?id=installmethodvalues-ru5s5eaq) is a known method. `UpdateCommand` resolves the package manager the same way (`npm.cmd` on Windows); a Homebrew formula upgrades without `--cask`, Volta with `volta install`.

## Step 3 — Read logins, keyrings and managed settings

Claude Code keeps its login in the macOS keychain and in `<home>/.credentials.json` on Linux and Windows (Claude Code authentication docs, "Credential management"), so on a linux or windows Host the library never asks `Host.Secrets` for Claude. Codex can keep its login in the OS keyring; `HostFromOS` reads it with `security` on macOS and `secret-tool` on Linux. On Windows, or on Linux without `secret-tool` or a Secret Service, the read is `ErrUnsupported` with the reason, and a Codex `auto` store falls back to `auth.json` as Codex itself does.

Managed settings follow the managed settings docs per OS. On Windows the policy can also sit in the registry: `claude.Runner.ManagedRegistry(ctx)` runs `reg.exe query` for `HKLM` and `HKCU\SOFTWARE\Policies\ClaudeCode` (value `Settings`) and returns each present one as a managed layer. It runs `reg.exe`, never `claude`.

## Step 4 — Test each OS

- **Any machine:** `task check` runs every module's tests, including the per-OS matrix tests (they set `Host.GOOS` and use the Windows fixture homes under `claude/testdata/windows/` and `codex/testdata/windows/`, with backslash paths, CRLF and BOM files and `.cmd` shims), and compiles every package and test binary for Windows (`task check:windows`).
- **Linux distributions:** `task test:distros` runs the tests, `agentinfo -fixtures`, and `agentinfo` on an empty and a seeded home in Debian, Alpine (musl), Fedora and Arch containers. It needs Docker and network and is not part of `task check`.
- **Windows and macOS for real:** `.github/workflows/platforms.yml` runs the tests and `agentinfo -fixtures` on GitHub's Windows, macOS and Linux runners. It is manual (`workflow_dispatch`) because Actions minutes are billed on this private repository.
- **By hand:** `cd examples && go run ./agentinfo -fixtures` prints the same report on every OS; `go run ./agentinfo` reads your machine.

## Pitfalls

- **A Host without `GOOS` uses the build OS's rules.** A hand-built Host on Windows gets `;` and `PATHEXT`, the same Host on Linux gets `:`; set `GOOS` to model another OS.
- **`Command.String()` is POSIX only.** It is for display; never paste it into PowerShell or `cmd.exe`. Run a Command through `exec.Cmd` with `Path`, `Args`, `Env` and `Dir`, which needs no quoting on any OS.
- **The OS runner resolves a bare `Command.Path` on the Command's own `PATH`** (with `PATHEXT` on Windows), not the parent process's. A `.cmd` shim is started through Windows' batch handling; that path has not been run on a real Windows by this repository's tests (the platforms workflow does that on demand).
- **Process working directories are unknown on Windows.** `Processes.Cwd` is `ErrUnsupported`; `codex.Agent.Processes` leaves `Cwd` empty and Claude live sessions use the cwd recorded in their session files.
- **A Codex keyring login on Windows is `ErrUnsupported`, not "logged out".** Branch on `errors.Is(err, agentio.ErrUnsupported)` and show the reason.
- **Claude's Windows native install layout is inferred.** A copied `%USERPROFILE%\.local\bin\claude.exe` is reported as native with the newest version under `~/.local/share/claude/versions`; that layout comes from the Linux and macOS installer and has not been observed on Windows.
- **Locked files on Windows.** A settings write is a temp file plus rename; if another program holds the file open without sharing, the rename fails and the error is returned (the backup was already made). Retry when the vendor CLI is idle.
- **Symlinks on Windows need Developer Mode.** Without it `ApplyHomeLinks` returns a `*agentio.SymlinkPrivilegeError`; there is no copy fallback, because a copied `settings.json` would stop following the default home.
- **The ownership exception of `settings.local.json` is not checked.** Claude Code also keeps the file in the start folder when the repo root is not owned by the user; the Host has no owner reader.

## Related

- [Host](core/host.md), [Homes](core/homes.md), [Targets and commands](core/launch.md)
- [Claude install](claude/install.md), [Codex install](codex/install.md), [Claude paths](claude/paths.md), [Codex auth and usage](codex/auth-usage.md)
- [Backups, moves and home links](core/backup-move-link.md), [Testing your own code](testing/your-code.md)
- [Feature index](reference/features.md)
