# Changelog

All notable changes to agentio are recorded in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Each Go module is tagged on its own (`v0.1.1`, `claude/v0.1.1`, `codex/v0.1.1`, `codex/state/v0.1.1`, `gemini/v0.1.1`, `pi/v0.1.1`, `opencode/v0.1.0`, `opencode/store/v0.1.0`, `builtin/v0.1.1`); an entry names the module it affects when it is not the whole library.

## [Unreleased]

## [0.1.4] - 2026-11-21

Patch release of `claude/v0.1.4` and `builtin/v0.1.3` (requires `claude/v0.1.4`). The other modules are unchanged and keep their tags.

### Added

- Claude Code 2.1.330 (module `claude`): `TestedMax` is 2.1.330 with a fixture home at `claude/testdata/homes/2.1.330`, so 2.1.330 no longer warns `untested-version`. The `claude --help` text did not change. A drift run on real 2.1.329 and 2.1.330 homes found shapes first written by 2.1.329 that the 2.1.329 sample had not seen; the fixture carries each of them with made-up values.
- `RecordDataFlowRecords` (module `claude`): the `data-flow-records` transcript record type (batches of tool calls and what they touched, written since 2.1.329), so it is a known type instead of an `unknown-record` drift finding. The `dataFlowOn` and `attributionPlugin` keys on message records and `failedHookErrorIndexes` on `stop_hook_summary` records stay outside the typed views; the records decode as before.
- `ModelSetting.AutoCompactWindow` (module `claude`): the per-model `autoCompactWindow` inside `modelSettings`, which used to land in `Unknown`.
- Feature row `FeatureFullSignInLinkEnv` (module `claude`): `CLAUDE_CODE_FULL_SIGN_IN_LINK`, added in 2.1.330, also listed by `KnownEnv`.
- `scripts/claude-bump.sh` and `task compat:claude -- <version>`: the mechanical part of supporting a new Claude Code release. It saves the CHANGELOG entries since `TestedMax`, diffs `claude --help`, runs the drift check against the last saved report and saves the next one, and prints the env vars, flags and settings keys the CHANGELOG names that the library does not know. `--apply` raises `TestedMax`, creates the fixture home from the previous one and moves the tested range in docs and example outputs. See [Drift check](docs/compat/drift.md#supporting-a-new-claude-code-version).

### Fixed

- `claude.ParseRecord` no longer drops a transcript line whose `timestamp` is missing, empty or not a time: the record decodes with a nil `Timestamp`. Claude Code has written such records (2.1.330 fixed `--resume` failing on them); before, `TranscriptReader`, `TranscriptTail` and the title scan skipped them, so a real message could disappear from a read.

## [0.1.3] - 2026-11-20

Patch release of the root module `v0.1.2` and `claude/v0.1.3` (requires root `v0.1.2`). The other modules are unchanged and keep their tags.

### Added

- Per-model windows in the token probe (modules root, `claude`): `ParseProbeHeaders` reads every `anthropic-ratelimit-unified-<window>-utilization/-reset/-status` header, not only `5h` and `7d`, so the per-model weekly window `7d_oi` (seen when the probe asks with Fable 5.2) and any future window come back; a broken unknown window is skipped. `agentio.Window` gains `ID` (the header name), `Status` (`agentio.WindowRejected` = refused now) and `Binding` (named by `representative-claim`).
- `agentio.ModelTokenProber` and `claude.Prober.TokenUsageModel(ctx, cred, model)`: probe with the model a session uses, so its scoped windows show; `claude.ProbeModel` resolves Claude Code's aliases (`opus`, `sonnet`, `haiku`, `fable`, `[1m]` dropped). A refused model is the new `agentio.ErrModelRefused`.

## [0.1.2] - 2026-11-20

Patch release of `claude/v0.1.2` (Claude Code 2.1.328 and 2.1.329 tested) and `builtin/v0.1.2` (requires `claude/v0.1.2`). The other modules are unchanged and keep their v0.1.1 / v0.1.0 tags.

### Added

- Claude Code 2.1.329 (module `claude`): `TestedMax` is 2.1.329 with a fixture home at `claude/testdata/homes/2.1.329`, so 2.1.328 and 2.1.329 no longer warn `untested-version`. A drift run on real 2.1.328 and 2.1.329 homes found no new record type, enum value or file shape; the only transcript key not seen in earlier records, `apiErrorIsTransient` on an API-error assistant record (first written by 2.1.328), sits outside the typed views and the record still decodes. The fixture carries such a record.
- Feature rows from the 2.1.328 CHANGELOG (module `claude`): `FeatureHookOnFailureMCPTool` (`"onFailure": "block"` on `mcp_tool` hooks; `HookHandler.OnFailure` already reads it) and `FeatureDisableDeferredToolResumeEnv` (`CLAUDE_CODE_DISABLE_DEFERRED_TOOL_RESUME`, also listed by `KnownEnv`).

## [0.1.1] - 2026-11-19

Patch release of every module: `github.com/ubgo/agentio` (`v0.1.1`), `claude/v0.1.1`, `codex/v0.1.1`, `codex/state/v0.1.1`, `gemini/v0.1.1`, `pi/v0.1.1` and `builtin/v0.1.1`, plus the first release of `opencode/v0.1.0` and `opencode/store/v0.1.0`.

### Added

- Process table facts in the same single read on macOS, Linux and Windows (module root): `Process.CommandLine` (the raw, unsplit command line), `RSS`, `TTY` (none on Windows) and `CPUTime`, plus `Process.CommandString`.
- Script matching for npm installs (module root): `ProcessMatch.ScriptBasenames`, `ScriptContains`, `Interpreters` (`DefaultInterpreters`: node, nodejs, bun, deno), `ScriptIndex`, `MatchesBasename` and `ProcessBase`; exclusions and substring matches also test the raw command line, so a path with a space (`Application Support/Claude`) is excluded on macOS; a Windows `.exe`/`.cmd`/`.bat` ending is ignored when matching a basename.
- Process sessions (modules root, `claude`, `codex`, `gemini`, `pi`, `opencode`): `ProcessSessionReader`, `ProcessSessionsOf`, `FindProcessSession` and `ProcessSession` join a running process to its home, conversation, transcript, version, status and last activity, with a `JoinConfidence` (exact from Claude's `sessions/<pid>.json`, argv from a resume flag, inferred from the working directory and start time). Helpers `Core.ProcessHome`, `ArgValue`, `IsUUID` and `Ancestors` (the parent chain of a pid).
- Stop plans (modules root and every agent): `StopPlanner`, `StopPlannerOf`, `StopPlan`, `StopSignal`, `DefaultStopGrace` and `Core.BaseStopPlan` say which pids to signal, the grace period, whether forcing is safe, the resume command, and refuse non-session processes (Codex app-server and mcp-server, `claude mcp serve`); Claude background sessions stop through `claude stop <id>`, Gemini CLI's relaunch pair is stopped child first, and a Windows host gets no graceful signal. Data only: agentio sends no signal.
- Turn files (modules root, `claude`, `codex`, `gemini`, `pi`, `opencode`): `TurnFileReader`, `TurnFilesOf` and `TranscriptRef` read turns from a transcript path (`TurnsAt`) and map a path to its home and conversation (`TranscriptOwner`); `codex.Agent.TurnsAt`/`RolloutOwner`, `gemini.Agent.TurnsAt`/`ChatOwner`/`MessageTokens` (Gemini chats get a turn reader for the first time), `pi.Agent.TurnsAt`/`SessionOwner`.
- `Turn.ID` (the vendor's stable message id) on every turn reader, and `Turn.Interim` for progress messages; Codex `Phase` (`PhaseCommentary`, `PhaseFinalAnswer`) on `EventMsg` and `TurnItem` (module `codex`).
- Command hooks (modules root, `claude`, `codex`, `gemini`): `HookInstaller`, `HooksOf`, `HookKind`, `HookEntry`, `HookPlan`, `HookCall` and `ErrInvalidHook`; Claude writes the home's `settings.json`, Codex `hooks.json` (never `config.toml`; feature flags are reported as manual steps), Gemini CLI the `hooks` key of `settings.json`. Installing only appends its own entries, keeps every other byte of the file (other hooks, keys, order, comments), backs the file up first and is idempotent; removing drops only matching commands. `claude.Writer.EnsureHook`, `codex.Writer.EnsureHook`/`RemoveHooks`, a new `gemini.Writer`, typed `codex.HookInput` and `gemini.HookInput` (from the vendors' documented shapes, not yet from a recorded run), `claude.HookInput.Raw`.
- `agentio.SessionWaiting`: a session blocked on the user (a permission prompt or dialog).
- Module `opencode` (opencode 1.18.30, stdlib only): data dir under `XDG_DATA_HOME`, install, legacy JSON sessions and turns with a message-count cursor, launch and resume, process classification and process sessions, stop plans, doctor; registered in `builtin` after pi. Guide `docs/opencode/README.md` and `examples/opencode`.
- Module `opencode/store` (pure-Go SQLite): reads `opencode.db` read-only: sessions with token and cost totals, conversations, messages and turns in a stable order.
- `agenttest`: golden turn helpers (`GoldenTurn`, `Golden`, `CheckTurns` with `-update`, `ReadTurns`, `GrowingTurns`) and `HookMerge`, which checks every merge guarantee against a hook installer.
- Guides `docs/capabilities/process-sessions.md` and `docs/capabilities/hooks.md`; examples `examples/processsessions` and `examples/hooks`.
- Batch process-session join (modules root, `claude`): `FindProcessSessions(ctx, agents, table)` returns sessions by pid, asking an agent's `ProcessSessionBatchReader` (`ProcessSessionBatchOf`) once for all its rows; `claude.Agent.ProcessSessions` reads every start environment in one call and lists the homes once. `ProcessEnvBatchReader` (`HostFromOS`: one `ps -E` run on macOS, `/proc` on Linux), `Core.ProcessHomes`, `Process.Cwd` (a cwd the caller already looked up; every agent's reader reuses it) and `Host.ProcessCwd`. `agenttest.FakeProcesses` implements `EnvBatch` and counts host calls (`TableReads`, `CwdCalls`, `EnvCalls`, `EnvBatchCalls`).
- `Host.ProcessesAlive(ctx, []ProcessRef, ...AliveOption)` re-checks many recorded processes with one table read; `ProcessAlive` uses it. `ExactStart()` requires the exact recorded start time instead of the two-second tolerance (module root).
- `agenttest.FakeProcesses` is safe for concurrent use; `Counts()` returns a `ProcessCalls` snapshot (module root).
- `Command.ShellLine()` and `StopPlan.ResumeLine()`: the line to print for a person, with the home variable of a non-default home (`CLAUDE_CONFIG_DIR=<home> claude --resume <id>`) and `cd <dir> &&`, never a token or any other env value (module root).
- `ProcessKind` value `command`: a short subcommand that is never a session (module root).
- `agentio.BadLine`, `BadLineReporter` and `TurnsAtReporting(r, path, from, fn, bad)`: report the transcript lines a turn reader skipped as undecodable; `claude.TurnsFileReporting`, `codex.Agent.TurnsAtReporting` and both neutral views implement it (modules root, `claude`, `codex`).
- `agentio.TrimLineEnd` (module root).
- `HookPlan.Existing` (every command hook the file already holds, with its neutral kind) and `HookPlan.Matching(match)` (what a `RemoveHooks` with the same match would drop), filled by the Claude, Codex and Gemini installers (modules root, `claude`, `codex`, `gemini`).
- `HomeAt(dir)` on the neutral `agentio.Agent` interface; opencode's neutral view uses its data-dir rule (modules root, `opencode`).
- `codex.TurnContext.Personality` and `TurnContext.Unknown`; `codex.ErrInvalidLine` (module `codex`).
- `claude.SubagentRun.HasMeta`; `SubagentRuns` also lists `agent-<id>.jsonl` transcripts without a meta file (module `claude`).
- Gemini turns carry `Dir` from the chat's `tmp/<slug>/.project_root` (module `gemini`).

### Changed

- `claude.SessionStatus.Neutral` maps `waiting` to `agentio.SessionWaiting` (it was `SessionIdle`) (module `claude`).
- `Offset` is documented as an opaque, agent-defined cursor (a byte offset for JSONL transcripts, a message count for opencode); store and pass back the value a read returns.
- Codex turn readers emit commentary as `Interim` turns without tokens, pair `token_count` with the final answer, and hold back an answer whose usage has not been written yet: the returned cursor stays on its line, so a live reader gets the turn once, with tokens. A session that crashes before writing another line keeps its last answer held back (module `codex`).
- `claude.Writer.AddHook` and `RemoveHooks` keep every other byte of `settings.json` instead of re-marshalling it (module `claude`).
- A Codex rollout line that is not valid JSON no longer ends `TurnsAt` with an error: it is skipped, like Claude's reader, and reported through `TurnsAtReporting` (module `codex`).
- `Turn.Raw` (and `claude.Record.Raw`) is the line without its terminator for every agent: Claude and pi kept the trailing `\n` (or `\r\n`) (modules `claude`, `pi`).
- No `codex mcp …` row is a session any more: `codex mcp serve` and a bare `codex mcp` classify as `mcp-server`, `codex mcp list|add|remove|…` as `command`, and stop plans refuse them (module `codex`). Every `claude mcp …` row other than `mcp serve` is `command`, not a CLI session (module `claude`).
- `npm install @openai/codex` and `npm install @anthropic-ai/claude-code` are no longer classified as agent processes; the pi basename matches only a process titled `pi` or run by node (modules `codex`, `claude`, `pi`).

## [0.1.0] - 2026-11-18

First release of every module: `github.com/ubgo/agentio` (`v0.1.0`), `claude/v0.1.0`, `codex/v0.1.0`, `codex/state/v0.1.0`, `gemini/v0.1.0`, `pi/v0.1.0` and `builtin/v0.1.0`.

### Added

- Modules `github.com/ubgo/agentio` (neutral core, typed capability accessors, `agenttest`), `claude` (Claude Code), `codex` (Codex), `codex/state` (Codex SQLite stores, read-only) and `builtin` (every built-in agent plus config-defined agents).
- `codex.Agent.WithBaseURL` and `codex.DefaultBaseURL`: send Codex usage calls to a test server or proxy without a custom transport, the same mechanism as `claude.Agent.WithBaseURL` (module `codex`).
- `agentio/pick` (root module, stdlib only): `pick.Best` chooses the home with the most headroom from `pick.Candidate`s (home, usage, read error, billing) with a `pick.Policy` (default `MaxPercent`, agx's auto-pick), skipping errored and per-token homes with typed reasons; ties keep input order; `Result.Explain` gives one line and the age of cached readings with `WithNow`. The core never imports it (architecture test). Guide `docs/capabilities/pick.md` and `examples/pick`.
- Platform rules by `Host.GOOS` (module root): `Host.LookPath`/`LookPathAll` and `LookPathAllFor` (`;` and `PATHEXT` on Windows, no exec bit there), `EnvLookupFor`, `ApplyEnvFor`, `NewCommandFor`, `Host.NewCommand`, `Host.ApplyEnv` (env names case-insensitive on Windows), `PathListSeparatorFor`, `OSLinux`/`OSDarwin`/`OSWindows`, `ShimTarget` (npm/pnpm launcher shims), `StripBOM`, `SymlinkPrivilegeError` (Windows without Developer Mode), and `InstallVolta`.
- `HostFromOS` on Windows: a process table from one PowerShell `Get-CimInstance Win32_Process` run (CommandLineToArgvW splitting); on Linux a keyring reader through `secret-tool` (Secret Service). Windows Credential Manager and process working directories return `ErrUnsupported` with the reason.
- `claude.Runner.ManagedRegistry` reads the Windows managed-settings policy registry values (`HKLM`/`HKCU\SOFTWARE\Policies\ClaudeCode`) with `reg.exe`; `SourceRegistryHKLM` and `SourceRegistryHKCU` (module `claude`).
- Install detection for npm `.cmd` shims, pnpm shims, nvm, bun (also `.exe` on Windows), Volta, Homebrew formulas (`Cellar`) and Claude's copied native `claude.exe`; Codex `UpdateCommand` handles Volta and Homebrew formulas (modules `claude`, `codex`).
- `agenttest.ExeName`, `SymlinkOrSkip` and `SkipOnWindows` for tests that must mean the same on every OS; `scripts/workspace.sh` writes a `go.work` with version-specific replaces so CI runners resolve the private sibling modules locally.
- `examples/agentinfo -fixtures`, `task check:windows` (Windows vet and test compile, part of `task check`), `task test:distros` (Debian, Alpine, Fedora and Arch containers; manual), and a manual-only `.github/workflows/platforms.yml` for Linux, macOS and Windows runners.
- A platforms guide (`docs/platforms.md`) and a README support table.
- Launch option registry (modules root, `claude`, `codex`): every `LaunchOptions` field has a snake_case key; `LaunchOptionKeys` lists each with its kind, allowed values, flag and feature gate, derived by running the real argv builder per field; `ParseLaunchOption` (`key=value`), `DecodeLaunchOptions` (strict typed block, unknown keys list the valid ones) and `LaunchOptionsSchema` (closed JSON Schema). Root: `OptionKey`, `DeriveOptionKeys`, `FindOptionKey`, `OptionValueJSON`, `SetOption`, `DecodeOptions`, `OptionsSchema`, and a JSON Schema builder (`JSONSchemaFor`, `SchemaNode`).
- Claude settings round trip (module `claude`): `Settings` and every nested settings type write their unknown keys back on `json.Marshal`; `ValidateSettings` type-checks a merged settings object and lists unknown keys (dotted when nested); `SettingsKeys`, `SettingsSchema`; `LaunchSettings` writes a 0600 `settings-<run id>.json` into a caller's dir or returns inline JSON for `--settings`; `EffortValues`, `PermissionModeValues`, `UpdateChannelValues`, `OutputFormatValues`, `InputFormatValues`, `CLISettingSources`.
- Codex config key table (module `codex`): `ConfigKeys` lists every typed `config.toml` path with its kind and enum values; `CheckOverride` decodes a `-c` override through the config decoder and reports typed, unlisted value, raw, unknown or type error.
- Live credential capability: `CapLiveCredential`, `LiveCredentialer`, `LiveCredentialPlan` and `LiveCredentialOf`; Claude plans `apiKeyHelper` with `CLAUDE_CODE_API_KEY_HELPER_TTL_MS` (`claude.Agent.LiveCredential`), Codex declares why it has none. Token kind `api-key-helper` (`TokenAPIKeyHelper`): Claude passes it as settings `apiKeyHelper` (`claude.HelperSettings`). `TokenProviderLister` lets an agent name the providers a token login needs.
- Config-defined agents can launch with autonomy, headless output and token logins (module root): `Spec.AutonomyArgs`, `HeadlessArgs`, `TokenEnv` (per kind and provider, with its own scrub list), `HomeEnvNamesParent` (Gemini CLI's `GEMINI_CLI_HOME` names the folder that contains the home) and `Packages`; every `Spec` field has a snake_case JSON tag; `TokenCredential.Provider`.
- Config-defined agents with `token_env` have `TokenLogin` (module root): `TokenLoginOf` was false for every config-defined agent although `Launch` honoured `token_env`. `TokenKinds` lists the kinds `token_env` names, `DetectTokenKind` never guesses, `TokenEnv` refuses an undescribed login as `Launch` does, and a spec whose `token_env` names a provider is a `TokenProviderLister`. Without `token_env` the reason is "spec has no token_env". `examples/fromspec` shows a token launch.
- Install version from package metadata without running the binary (module root): `InstallFromPath` (npm, pnpm, Bun, Volta, Homebrew Cellar, native `versions/<v>`); config-defined agents now have an `Installer`.
- Run-file cleanup (module root): `Host.ProcessAlive` returns `Liveness` (alive, ended, unknown) for a recorded pid, start time and start-env entry; `ProcessEnvReader` (HostFromOS reads a process's start env on Linux and macOS); `agenttest.FakeProcesses.Envs`.
- Launch config capability (modules root, `claude`, `codex`): `CapLaunchConfig`, `LaunchConfigurer` and `LaunchConfigOf` build a launch with typed vendor options and per-launch settings through the neutral Agent (`LaunchConfigured(t, r, LaunchConfig{Options, Settings, Delivery, RunDir, RunID})`): options overlay the request mapping and a key the request already sets is refused naming both; Claude settings are type-checked and delivered by `LaunchSettings` (an api-key-helper login merges into them), Codex settings become `-c` overrides checked by `CheckOverride` (`codex.SettingsOverrides`, `codex.OverrideFloat`); unknown keys warn (`WarnUnknownKey`) and type errors are refused. `OptionKey.Concern` and `SettingsCheck.Concerns` name the neutral concern a key overlaps; `SettingsDelivery`, `Concern`, `OverlayOptions`, `CheckOptionsObject`, `MergeSettings`, `ClosestKey`, `SchemaPaths` and `IsEmptyJSON` in the root. `DecodeOptions` now names the key and expected kind for a value of the wrong JSON type. Gemini CLI, pi and config-defined agents declare why they have none.
- Module `pi`: the pi coding agent (`@earendil-works/pi-coding-agent` 1.5.2, also the deprecated `@mariozechner/pi-coding-agent`) as a built-in agent: install, agent dir and `PI_CODING_AGENT_DIR`, sessions and turns, per-session token cost, settings, trust, logins without values, instruction files, `mcp.json`, skills, prompt templates, typed launch options, doctor; registered in `builtin`. Guide `docs/pi/README.md` and `examples/pi`.
- Guide `docs/capabilities/launch-config.md` and `examples/launchconfig` for the option registry, settings, live credential, Codex overrides and run files.
- `OptionKey.Path` (modules root, `claude`, `codex`): an option whose value names a file or folder on this machine, set by the struct tag `agentio:"path"` (Claude `add_dirs`, `mcp_configs`, `settings`, `system_prompt_file`, `append_system_prompt_file`, `plugin_dirs`, `debug_file`; Codex `cd`, `add_dirs`, `images`), so a launcher expands `~` in those values only. `DeriveOptionKeys` refuses the tag on a value that is not text and an unknown `agentio` tag.
- Claude Code 2.1.327 (module `claude`): `TestedMax` is 2.1.327 with a fixture home at `claude/testdata/homes/2.1.327`; `StatusWaiting` and `LiveSession.WaitingFor` for a session waiting on a dialog; transcript record type `RecordForkContextRef` (the first record of a subagent transcript); feature row `FeatureMCPAddJSONStdin` (`claude mcp add-json <name> -`, since 2.1.327).

### Changed

- `claude.Agent.MCPAddCommand`'s refusal of a server with env values or headers now gives the real reason: agentio passes the server JSON on argv because `agentio.Command` has no stdin, although Claude Code 2.1.327 can read it from stdin (module `claude`).
- `OverlayOptions` (module root): an option list that lands where `LaunchRequest.Args` go (Claude's `extra_args`) is no longer refused next to raw args; both are kept, the option's list first, then the raw args. Conflict errors name the request part in plain words (`option "model" conflicts with the request's model`) instead of Go field names (`LaunchRequest.Model`).
- `Command.Redacted` and the `Command` JSON judge each env value by its source (module root): a Token or `EnvValue` value and a credential variable the build scrubbed and then set again are masked; a plain value the build set with `EnvSet` is shown whatever its name (`CLAUDE_CODE_API_KEY_HELPER_TTL_MS=300000` was masked because its name contains `KEY`); only values inherited unchanged from the base env, whose source is unknown, are still masked by name (`TOKEN`, `KEY`, `SECRET`, `PASSWORD`, `PASSWD`, `CREDENTIAL`, `AUTH`).
- `ErrDangerousNotAllowed` reads `agentio: full autonomy needs explicit consent` (was `… needs Target.AllowDangerous`): the text is shown to a launcher's user, so it names no Go field. The sentinel is the same value (module root).
- Every vendor launch-refusal sentinel also matches `agentio.ErrInvalidOption` (`NewLaunchSentinel`, module root): `claude.ErrInvalidLaunch` (for example `--resume and --continue are exclusive`), `pi.ErrInvalidLaunch`, `codex.ErrInvalidOption`, `ErrNoSessionID`, `ErrBadOverride` and `ErrProfileConflict` were plain sentinels, so a caller classifying by agentio's sentinel saw a refused combination as an unknown failure. Messages are unchanged.
- Codex `LaunchConfigured` with a `codex_profile` option reads the home's `config.toml` and refuses a legacy profile of that name (`ErrProfileConflict`), instead of building with the warning `--profile conflict with a legacy profile not checked (no config given)`, which a launcher could do nothing about. The rich builder's warning without `ProfileCheck` now reads `--profile NAME not checked against the legacy [profiles] tables in config.toml; Codex refuses it when one has that name` (module `codex`).
- Claude's neutral option keys (`LaunchConfigurer.OptionKeys`, `OptionsSchema`, `CheckOptions`, `LaunchConfigured`) no longer list `settings`: `LaunchConfig.Settings` is the neutral path's one settings channel, so a launcher's own settings key never duplicates it. `LaunchOptionKeys` and the rich builder keep the option (module `claude`).
- Home markers (all modules): `IsHome` no longer takes a file name other tools write too as proof of a home in a folder found elsewhere. `Spec.GenericHomeMarkers` lists them (Claude `settings.json`, `projects`; Gemini CLI `settings.json`, `tmp`; pi `settings.json`, `sessions`, `auth.json`; Codex `config.toml`, `auth.json`, `sessions`, `version.json`): alone they count only in the default home, an `ExtraHomes` hit or the home variable's folder. A folder with only `settings.json` was a Claude, Gemini CLI and pi home at once, and a pi home was also a Codex home. `Core.HasHomeMarker` counts every marker for a caller-named home (doctor home check, Codex `HomeAt`); `Validate` refuses a generic marker that is not a home marker, or a spec whose markers are all generic.
- Credential scrubbing follows the login: a launch on the home's own login scrubs `Spec.CredentialEnv`, a token login scrubs only the variables that outrank that token, so a variable that is the login mechanism (pi's provider keys, a Gemini API-key home's `GEMINI_API_KEY`) is kept.
- The free `agentio.LookPath` and `LookPathAll` use the running OS's rules (`LookPathAllFor` with `runtime.GOOS`): on Windows they now try `PATHEXT` and need no exec bit; Windows PATH rules accept rooted (`\tools`) and quoted entries; home dirs and PATH lookups use each OS's own path syntax instead of the runner's `filepath`.
- Every launch scrubs the vendor's inherited credential variables (`Spec.CredentialEnv`), for a home login too, before `Target.ExtraEnv` is applied: an inherited `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN` or `CLAUDE_CODE_OAUTH_TOKEN` no longer overrides a Claude home's login. Codex's `CredentialEnv` is now only `CODEX_API_KEY` (it beats the home login in `codex exec`); `OPENAI_API_KEY` is kept because Codex auth does not read it and a model provider may name it as its key.
- `Host` has `GOOS` and `SystemRoot`, filled only by `HostFromOS` (`Host.SystemPath` joins a system path under the root; `agenttest.WithSystem` sets both). Claude `SystemPaths`, `ManagedSettings`, the managed `CLAUDE.md` in `MemoryFiles`, managed MCP servers, `Runner.ManagedPlist` and Codex's `/etc/codex` layers resolve through them instead of `runtime.GOOS` and absolute paths; a fixture or hand-built Host has no system locations, so the machine's `/etc` never changes its output.
- `HistoryMover.MoveHistory` returns `agentio.HistoryMove{Moved, Backup}` instead of a bare `Backup`, so a folder without history (`Moved` false) is no longer indistinguishable from a move; `claude.Writer.MoveHistory` returns the same type instead of `(moved, backup, err)`.
- `HomeLink` carries the `Agent` that planned it (`agentio.CheckLinksOwner` checks a plan).
- Config layers are listed highest precedence first for every vendor and the neutral `ConfigReader.ConfigLayers` (documented on the interface): Codex `ConfigLayers` and `EffectiveConfig.Layers` now match Claude, which already listed the winning layer first (module `codex`).
- `state.Open` returns only a `*state.DB`: it needs just the paths of the stores you query, and a store without a path fails with `*state.NoPathError` (`state.ErrNoPath`) when used instead of `Open` failing "no path for goals_1.sqlite" (module `codex/state`).
- One API shape for both vendors: `codex.FeatureValues` is renamed `codex.Features` (as `claude.Features`); `UpdateCommand` is a method `UpdateCommand() (agentio.Command, error)` on both agents that resolves the binary or package manager on `Host.Env` `PATH` (codex reads the install method from disk; the old `codex.UpdateCommand(method)` returned a bare `npm`/`bun` name); `claude.ParseVersion` and `codex.ParseVersion` return `(agentio.Version, bool)` like `agentio.ParseVersion`.

### Fixed

- `codex/state` opens SQLite through a valid `file:` URI on Windows (`file:///C:/...`); a drive path was rendered as an invalid URI authority and every read failed (module `codex/state`).
- Install detection, the `settings.local.json` repo-root-is-home check and the memory-import "external" check compare symlink-resolved paths on both sides: a copied native `claude.exe` behind a symlinked parent dir (macOS `/var` → `/private/var`, a home reached through a link) was reported as an unknown install (module `claude`).
- A home variable holding a rooted path without a drive on Windows (`\work\h`) resolves onto the user home's drive, as Windows does, instead of under the user home.
- `claude.EncodePath` now matches Claude Code exactly (2.1.325 bundle): one `-` per UTF-16 unit (it replaced each UTF-8 byte), and a key over 200 characters is cut and suffixed with a base-36 hash, so history of non-ASCII and long paths is found (module `claude`).
- `settings.local.json` is read from the start folder on Windows and when the repo root is the user home, as Claude Code does (module `claude`).
- On a linux or windows Host, Claude credentials come only from `.credentials.json`; the keychain is not consulted there (module `claude`).
- The OS runner resolves a bare `Command.Path` on the Command's own `PATH` (with `PATHEXT` on Windows) instead of the parent process's, so `Runner.BinaryVersion` finds a codex that only the Host's PATH has.
- Vendor JSON, TOML and JSONL readers accept a UTF-8 BOM (Windows editors) and CRLF.
- The Codex doctor's `api-billing-env` no longer warns on `OPENAI_API_KEY` (Codex auth never reads it); it names `CODEX_API_KEY`, and `OPENAI_API_KEY` only when a model provider's `env_key` names it (module `codex`).
- `ParseVersion` only starts a version at a word boundary: `x86_64 build 7` parses as `7.0.0`, no longer `64.0.0`.
- Claude usage, profile and token-probe calls return `ErrUnsupported` when `Host.HTTP` is nil instead of building a default client that reached the real network (module `claude`).
- User-facing strings no longer name private design ids: the Claude model-lister Unsupported reason dropped "(OD-15)" and the Codex accept-edits reason "(OD-14)"; an architecture test fails on any string literal naming one. Code comments no longer name them either: each states the reason instead, and the same test covers comments in non-test code.
- `claude.Runner.BackgroundAgents` keeps keys a newer Claude Code adds in `BackgroundAgent.Unknown` instead of dropping them (module `claude`).
- `state.Turn` encodes without an `Error` key when `error_json` is NULL instead of `"Error":null` (module `codex/state`).
- The Claude doctor `retention` check warns about clock skew when the `.last-cleanup` stamp is after the host clock, instead of printing a negative age ("last cleanup -7 h ago").
- `claude.Writer.MoveHistory` refuses while a Claude session runs in the source folder (or one of its conversations is live), with a typed `*claude.LiveSessionError` matching `agentio.ErrMoveBlocked` and the blocker text `PlanMove` lists.
- `claude.Writer.ApplyHomeLinks` and `Core.ApplyHomeLinks` refuse links planned for another agent's homes with `ErrForeignHome`, as `ApplyMove` does.

### Tooling

- `.golangci.yml` (golangci-lint v2, default linters) and `task lint`, now part of `task check`; every module reports 0 issues. The only exclusion is errcheck on `fmt.Fprint*` output in `examples/`.

### Documentation

- Live credential: `LiveCredentialPlan` and the launch-config guide say the command must never print an empty value (Claude Code 2.1.327 exits after three failed helper runs, with no fallback to the home's own login) and describe the checked refresh behaviour (retries after a 401, stale-while-revalidate timed refresh).
- A documentation site under `docs/`: a learning path from the first `Host` to a production setup, a "start from your goal" table, and guides for the core, the shared capabilities, every Claude Code and Codex area, testing, and compatibility. Every guide has complete snippets, the real output of a runnable example, and pitfalls taken from the code and tests.
- Choosing guides (`docs/choosing/`): concrete vendor type or neutral accessor, which door (Runner, Prober, Writer), live or cached usage, built-in or config-defined agent, which module to import, and file writes or vendor commands.
- A feature index (`docs/reference/features.md`) listing every feature with its package, the guide step that teaches it and an example that calls it.
- An architecture diagram (`docs/assets/architecture.svg`).
- Runnable examples under `examples/`, one per area, each with a README showing its real output and a test that runs it on the repository's fixture homes, fakes and local test servers (no real account, binary or network). `examples/agentinfo` and `examples/drift` gained READMEs and fixture-home tests, and `drift` a `-fixtures` flag.
- Docs bound to code with docsync (`ds`): behavioural sentences in `README.md` and the guides cite the declaration that implements them (`ds:block` links to remote `ds:def` ids defined in a comment block in `README.md`), each acknowledged only after it was checked against the code; `.ds/` is committed. `task docs:check` runs `ds check` after `internal/docscheck` and `task check` runs `task docs:check`; a manual-only (`workflow_dispatch`) docsync workflow is in `.github/workflows/docsync.yml`.
- A docs checker (`internal/docscheck`, run by `task docs:check`) that fails on broken links and anchors, hard-wrapped Markdown, local paths, attribution lines, and snippets that differ from the example source they are copied from; `task docs:snippets` rewrites snippet blocks from source.
- Repository files: README, CONTRIBUTING, SECURITY, CODE_OF_CONDUCT, LICENSE, issue and pull request templates, and `llms.txt`.

[Unreleased]: https://github.com/ubgo/agentio/compare/claude/v0.1.2...HEAD
[0.1.2]: https://github.com/ubgo/agentio/compare/claude/v0.1.1...claude/v0.1.2
[0.1.1]: https://github.com/ubgo/agentio/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/ubgo/agentio/releases/tag/v0.1.0
