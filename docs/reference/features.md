# Feature index

Every user-facing feature of agentio, grouped by area, with the guide that teaches it and a runnable example that calls it. **Feature** names the API (types, functions, methods) a caller uses; fine-grained helpers that only make sense together share one row. **Package** is the import path under `github.com/ubgo/agentio` (`agentio` is the root module). **Guide** links the guide page and the exact step that covers the feature. **Example** links a program in `examples/` whose code calls that API; "—" means no example calls it yet.

Areas follow the library's layers: L1 is the neutral core every agent shares, L2 the shared capabilities reached through typed accessors (`agentio.UsageOf` and friends), the doors keep process runs, quota spending and writes behind an explicit opt-in, and L3 is the full typed API of each vendor module (`claude`, `codex`, `codex/state`).


## L1 core

| Feature | Package | Guide | Example |
|---|---|---|---|
| Install the module(s) you need (`go get`, module layout, which module pulls which dependency) | `agentio`, `claude`, `codex`, `codex/state`, `builtin` | [Install agentio](../getting-started/install.md#step-1--add-the-module-you-need) | — |
| Quick start: `HostFromOS` → `claude.New` → `Install` → `Homes` | `agentio`, `claude` | [Your first agent](../getting-started/first-agent.md#step-1--build-a-host) | [quickstart](../../examples/quickstart/), [drift](../../examples/drift/) |
| `HostFromOS` — the one constructor that reads the real machine | `agentio` | [Host: the only door to the machine](../core/host.md#step-1--build-a-host-from-the-os) | [quickstart](../../examples/quickstart/), [agentinfo](../../examples/agentinfo/) |
| `Host` built by hand (`UserHome`, `User`, `Env`, `Now`, `HTTP`, `Secrets`, `Processes`, `Exec`, `GOOS`, `SystemRoot`) and `Host.Getenv`, `Host.SystemPath`, `EnvLookup` | `agentio` | [Host: the only door to the machine](../core/host.md#step-2--build-a-host-by-hand) | [quickstart](../../examples/quickstart/) |
| Host inputs: `SecretStore`, `ProcessLister`, `ProcessRunner`, `ProcessResult` | `agentio` | [Host: the only door to the machine](../core/host.md#step-2--build-a-host-by-hand) | [doors](../../examples/doors/), [testing](../../examples/testing/) |
| `LookPath`, `LookPathAll`, `Agent.LookPath` (PATH walk, never a shell) | `agentio` | [Host: the only door to the machine](../core/host.md#step-3--find-a-binary-on-path) | [quickstart](../../examples/quickstart/), [agentinfo](../../examples/agentinfo/) |
| `Agent` interface (`ID`, `Name`, `Spec`, `LookPath`, `DefaultHome`, `HomeFromEnv`, `Homes`, `IsHome`, `Launch`) | `agentio` | [Agent, Spec and Core](../core/agent-and-spec.md#step-1--use-the-agent-interface) | [quickstart](../../examples/quickstart/), [agentinfo](../../examples/agentinfo/) |
| `ID`, `ParseID`, `ErrInvalidID` | `agentio` | [Agent, Spec and Core](../core/agent-and-spec.md#step-1--use-the-agent-interface) | [quickstart](../../examples/quickstart/) |
| `Spec` (data that describes an agent), `Spec.Validate`, `ProcessMatch` | `agentio` | [Agent, Spec and Core](../core/agent-and-spec.md#step-2--read-an-agents-spec) | [quickstart](../../examples/quickstart/) |
| `NewCore` and `Core` (Spec-driven L1 core: homes, env, `Command`, `Launch`, `LoginCommand`, `Checks`) | `agentio` | [Agent, Spec and Core](../core/agent-and-spec.md#step-3--build-a-core-from-a-spec) | [quickstart](../../examples/quickstart/) |
| `Home`, `NewHome`, `DefaultHomeName`, `Home.Dir` / `Name` / `IsDefault` / `Path` / `IsZero` | `agentio` | [Homes](../core/homes.md#step-1--get-the-default-home) | [homes](../../examples/homes/) |
| `Agent.Homes`, `Agent.HomeFromEnv`, `Agent.IsHome` (home discovery by markers) | `agentio` | [Homes](../core/homes.md#step-2--list-every-home) | [homes](../../examples/homes/) |
| `Core.HomeAt` / vendor `HomeAt` (open a home by folder) | `agentio` | [Homes](../core/homes.md#step-3--open-a-home-by-folder) | [homes](../../examples/homes/) |
| `CheckOwner`, `Core.Owns`, `ErrForeignHome` | `agentio` | [Homes](../core/homes.md#step-4--check-who-owns-a-home) | [homes](../../examples/homes/) |
| `Target`, `Home.Target` | `agentio` | [Targets and commands](../core/launch.md#step-1--build-a-target-from-a-home) | [launch](../../examples/launch/), [claudelaunch](../../examples/claudelaunch/), [codexlaunch](../../examples/codexlaunch/) |
| `LaunchRequest` and `Agent.Launch` (neutral launch / resume) | `agentio` | [Targets and commands](../core/launch.md#step-2--launch-through-the-neutral-agent) | [launch](../../examples/launch/) |
| `Autonomy`, `CheckAutonomy`, `ErrDangerousNotAllowed` | `agentio` | [Targets and commands](../core/launch.md#step-3--check-the-autonomy-level) | [launch](../../examples/launch/) |
| `Command`, `NewCommand`, `Command.Redacted`, `Command.SecretEnvNames`, `Warnings` | `agentio` | [Targets and commands](../core/launch.md#step-2--launch-through-the-neutral-agent) | [launch](../../examples/launch/) |
| `EnvChange`, `EnvSet`, `EnvSetSecret`, `EnvUnset`, `ApplyEnv`, `HomeEnvChange`, `ScrubEnv` | `agentio` | [Targets and commands](../core/launch.md#step-4--change-the-child-environment) | [launch](../../examples/launch/) |
| `Token`, `NewToken`, `Token.Reveal`, `Token.LogValue` (redacting secret) | `agentio` | [Tokens and redacted values](../core/secrets.md#step-1--wrap-a-secret-in-a-token) | [launch](../../examples/launch/) |
| `TokenCredential`, `EnvSetSecret` (token only in `Command.Env`) | `agentio` | [Tokens and redacted values](../core/secrets.md#step-2--put-a-token-in-a-child-env) | [launch](../../examples/launch/) |
| `EnvValue`, `NewEnvValue`, `Personal`, `NewPersonal` (redacted values) | `agentio` | [Tokens and redacted values](../core/secrets.md#step-3--redact-personal-values) | [launch](../../examples/launch/) |
| Error sentinels (`ErrNotLoggedIn`, `ErrExpired`, `ErrUnauthorized`, `ErrScopeLimited`, `ErrBilledPerToken`, `ErrUnsupported`, `ErrNotFound`, `ErrNotInstalled`, `ErrUnknownAgent`, `ErrInvalidSpec`, `ErrMoveBlocked`, `ErrUnknownVersion`, `ErrFeatureUnavailable`), `UnsupportedError` | `agentio` | [Errors](../core/errors.md#step-1--match-a-sentinel) | [values](../../examples/values/), [drift](../../examples/drift/) |
| `ErrRateLimited`, `RateLimitError`, `RetryAfter` | `agentio` | [Errors](../core/errors.md#step-2--handle-rate-limits) | [values](../../examples/values/) |
| `Opt[T]`, `Some`, `Scalar` (presence-tracked fields) | `agentio` | [Optional values, unknown keys and warnings](../core/values.md#step-1--read-an-optional-field) | [values](../../examples/values/) |
| `Unknown`, `Unknown.Keys`, `ExtractUnknown`, `ExtractUnknownFor`, `JSONKeys`, `Raw` | `agentio` | [Optional values, unknown keys and warnings](../core/values.md#step-2--find-unknown-keys) | [values](../../examples/values/), [agentinfo](../../examples/agentinfo/) |
| `Warning`, `Warnings` (`Add`, `Has`), `WarningKind` | `agentio` | [Optional values, unknown keys and warnings](../core/values.md#step-3--collect-warnings) | [values](../../examples/values/), [compat](../../examples/compat/), [drift](../../examples/drift/) |
| `Version`, `ParseVersion`, `MustVersion` | `agentio` | [Versions](../core/versions.md#step-1--parse-a-version) | [values](../../examples/values/), [drift](../../examples/drift/) |
| `Version.Compare`, `Version.IsZero` | `agentio` | [Versions](../core/versions.md#step-2--compare-versions) | [values](../../examples/values/) |
| `Backup`, `BackupEntry`, `BackupPath`, `BackupFile`, `Backup.Merge`, `Backup.IsEmpty`, `CopyTree` | `agentio` | [Backups, moves and home links](../core/backup-move-link.md#step-1--back-up-a-file) | [backupmove](../../examples/backupmove/) |
| `MoveRequest`, `MovePlan`, `MoveItem` (planning a history move) | `agentio` | [Backups, moves and home links](../core/backup-move-link.md#step-2--plan-a-move) | [backupmove](../../examples/backupmove/) |
| `ApplyMovePlan` | `agentio` | [Backups, moves and home links](../core/backup-move-link.md#step-3--apply-a-move-plan) | [backupmove](../../examples/backupmove/) |
| `HomeLink` (with `Agent`), `PlanHomeLinks`, `ApplyHomeLinks`, `CheckLinksOwner`, `Core.PlanHomeLinks`, `Core.ApplyHomeLinks` | `agentio` | [Backups, moves and home links](../core/backup-move-link.md#step-4--plan-and-apply-home-links) | [backupmove](../../examples/backupmove/) |
| Closed-set enums with `XValues()` / `ParseX()` / `String()` (`Access`, `Autonomy`, `Billing`, `Capability`, `CheckStatus`, `CompatStatus`, `ConfigScope`, `CostClass`, `CredentialSource`, `InstructionScope`, `LoginFlow`, `Role`, `RunEventType`, `SessionStatus`, `TokenKind`, `Trust`, `UsageSource`, `WarningKind`, `WindowClass`) and open string enums (`InstallMethod`, `MCPTransport`, `ProcessKind`, `SessionKind`, `SkillScope`, `CheckID`) with `IsKnown` | `agentio` | [Enums](../core/enums.md#step-1--parse-an-enum-from-text) | [values](../../examples/values/) |

## L2 capabilities and accessors

| Feature | Package | Guide | Example |
|---|---|---|---|
| Loop over agents with typed accessors (no type assertions) | `agentio` | [Shared capabilities and accessors](../capabilities/accessors.md#step-1--loop-over-agents-with-an-accessor) | [accessors](../../examples/accessors/), [agentinfo](../../examples/agentinfo/), [testing](../../examples/testing/) |
| `Capability`, `Supported` | `agentio` | [Shared capabilities and accessors](../capabilities/accessors.md#step-2--list-supported-capabilities) | [accessors](../../examples/accessors/), [agentinfo](../../examples/agentinfo/) |
| `Unsupported`, `UnsupportedReasoner`, `CapabilityGate` | `agentio` | [Shared capabilities and accessors](../capabilities/accessors.md#step-3--ask-why-a-capability-is-missing) | [accessors](../../examples/accessors/) |
| `Capability.Cost`, `Cost`, `CostClass`, `Access` | `agentio` | [Shared capabilities and accessors](../capabilities/accessors.md#step-4--check-what-a-capability-costs) | [accessors](../../examples/accessors/) |
| `Installer` · `InstallerOf` (`Install`, `InstallMethod`) | `agentio` | [Install, version and identity](../capabilities/install-identity.md#step-1--read-the-install) | [accessors](../../examples/accessors/), [agentinfo](../../examples/agentinfo/) |
| `BinaryVersion` · `BinaryVersionOf` (Process door, may touch login) | `agentio` | [Install, version and identity](../capabilities/install-identity.md#step-2--run-the-binary-for-its-version) | [accessors](../../examples/accessors/), [agentinfo](../../examples/agentinfo/) |
| `IdentityReader` · `IdentityOf` (`Identity`, `Identity.Expired`) | `agentio` | [Install, version and identity](../capabilities/install-identity.md#step-3--read-the-signed-in-identity) | [accessors](../../examples/accessors/), [agentinfo](../../examples/agentinfo/) |
| `CachedUsageReader` · `CachedUsageOf` | `agentio` | [Usage and subscription](../capabilities/usage.md#step-1--read-cached-usage) | [usage](../../examples/usage/), [agentinfo](../../examples/agentinfo/) |
| `UsageReader` · `UsageOf` (`Usage`, `Window`, `WindowClass`, `Usage.MaxPercent`, `UsageSource`, `Billing`) | `agentio` | [Usage and subscription](../capabilities/usage.md#step-2--read-live-usage) | [usage](../../examples/usage/), [claudeusage](../../examples/claudeusage/), [agentinfo](../../examples/agentinfo/) |
| `SubscriptionReader` · `SubscriptionOf` (`Subscription`) | `agentio` | [Usage and subscription](../capabilities/usage.md#step-3--read-the-subscription) | [usage](../../examples/usage/) |
| `LoginCommander` · `LoginCommanderOf` (`LoginFlow`) | `agentio` | [Logins and tokens](../capabilities/logins.md#step-1--build-a-login-command) | [usage](../../examples/usage/) |
| `TokenLogin` · `TokenLoginOf` (`TokenKind`, `CredentialSource`) | `agentio` | [Logins and tokens](../capabilities/logins.md#step-2--launch-with-a-token) | [usage](../../examples/usage/) |
| `TokenProber` · `TokenProberOf` (Quota door) | `agentio` | [Logins and tokens](../capabilities/logins.md#step-3--probe-a-token) | [usage](../../examples/usage/), [claudeusage](../../examples/claudeusage/) |
| `ConversationLister` · `ConversationsOf` (`Conversation`, `ConversationQuery`) | `agentio` | [Conversations, turns and prompt history](../capabilities/conversations.md#step-1--list-conversations) | [conversations](../../examples/conversations/), [agentinfo](../../examples/agentinfo/) |
| `TurnReader` · `TurnsOf` (`Turn`, `Offset`, `Role`, `TokenCounts`) | `agentio` | [Conversations, turns and prompt history](../capabilities/conversations.md#step-2--read-turns-from-an-offset) | [conversations](../../examples/conversations/) |
| `PromptHistoryReader` · `PromptHistoryOf` (`PromptEntry`, `PromptQuery`) | `agentio` | [Conversations, turns and prompt history](../capabilities/conversations.md#step-3--read-prompt-history) | [conversations](../../examples/conversations/) |
| `HistoryChecker` · `HistoryCheckerOf` | `agentio` | [Conversations, turns and prompt history](../capabilities/conversations.md#step-4--check-and-move-history) | [conversations](../../examples/conversations/) |
| `HistoryMover` · `HistoryMoverOf` (returns `HistoryMove`) and `ConversationMover` · `ConversationMoverOf` (Write doors) | `agentio` | [Conversations, turns and prompt history](../capabilities/conversations.md#step-4--check-and-move-history) | [conversations](../../examples/conversations/) |
| `LiveSessionLister` · `LiveSessionsOf` (`LiveSession`, `SessionStatus`, `SessionKind`) | `agentio` | [Live sessions and processes](../capabilities/live-sessions.md#step-1--list-live-sessions) | [livesessions](../../examples/livesessions/), [agentinfo](../../examples/agentinfo/) |
| `ProcessClassifier` · `ProcessClassifierOf`, `ClassifyProcess` (`Process`, `ProcessInfo`, `ProcessKind`) | `agentio` | [Live sessions and processes](../capabilities/live-sessions.md#step-2--classify-a-process) | [livesessions](../../examples/livesessions/), [agentinfo](../../examples/agentinfo/) |
| `FindLiveSession` | `agentio` | [Live sessions and processes](../capabilities/live-sessions.md#step-3--find-the-session-of-a-pid) | [livesessions](../../examples/livesessions/) |
| `ConfigReader` · `ConfigOf` (`ConfigLayer`, `ConfigScope`, `Defaults`, `Trust`) | `agentio` | [Config, MCP, instructions, skills, plugins and models](../capabilities/config.md#step-1--read-config-layers) | [accessors](../../examples/accessors/), [agentinfo](../../examples/agentinfo/) |
| `MCPLister` · `MCPServersOf` (`MCPServer`, `MCPTransport`) | `agentio` | [Config, MCP, instructions, skills, plugins and models](../capabilities/config.md#step-2--list-mcp-servers) | [accessors](../../examples/accessors/), [agentinfo](../../examples/agentinfo/) |
| `InstructionFiles` · `InstructionFilesOf` (`InstructionFile`, `InstructionScope`) | `agentio` | [Config, MCP, instructions, skills, plugins and models](../capabilities/config.md#step-3--list-instruction-files) | [accessors](../../examples/accessors/), [agentinfo](../../examples/agentinfo/) |
| `SkillLister` · `SkillsOf` (`Skill`, `SkillScope`) and `PluginLister` · `PluginsOf` (`Plugin`) | `agentio` | [Config, MCP, instructions, skills, plugins and models](../capabilities/config.md#step-4--list-skills-and-plugins) | [accessors](../../examples/accessors/), [agentinfo](../../examples/agentinfo/) |
| `ModelLister` · `ModelsOf` (`Model`; Codex only) | `agentio` | [Config, MCP, instructions, skills, plugins and models](../capabilities/config.md#step-5--list-models) | [accessors](../../examples/accessors/) |
| `RunEvents` · `RunEventsOf` (`RunDecoder`, `RunEvent`, `RunEventType`) | `agentio` | [Run events](../capabilities/run-events.md#step-1--decode-a-headless-run) | [runevents](../../examples/runevents/) |
| `Doctor` · `DoctorOf` (`Check`, `CheckID`, `CheckStatus`) | `agentio` | [Doctor checks](../capabilities/doctor.md#step-1--run-the-checks-for-a-home) | [accessors](../../examples/accessors/), [agentinfo](../../examples/agentinfo/) |
| `VersionCheck` (version-tested check) | `agentio` | [Doctor checks](../capabilities/doctor.md#step-2--check-the-installed-version) | [accessors](../../examples/accessors/) |

## Doors: Runner, Prober, Writer

| Feature | Package | Guide | Example |
|---|---|---|---|
| Runner door: `claude.Agent.Runner()`, `codex.Agent.Runner()`, root `Core.Runner` / `Runner.Run` / `Runner.Version` | `agentio`, `claude`, `codex` | [Doors: Runner, Prober and Writer](../capabilities/doors.md#step-1--run-the-vendor-binary-through-a-runner) | [doors](../../examples/doors/) |
| Prober door: `claude.Agent.Prober()` (`Prober.TokenUsage`) | `claude` | [Doors: Runner, Prober and Writer](../capabilities/doors.md#step-2--spend-quota-through-a-prober) | [doors](../../examples/doors/), [claudeusage](../../examples/claudeusage/) |
| Writer door: `claude.Agent.Writer()`, `codex.Agent.Writer()` (no methods yet), every write returns a `Backup` | `claude`, `codex` | [Doors: Runner, Prober and Writer](../capabilities/doors.md#step-3--write-through-a-writer) | [doors](../../examples/doors/) |
| `HomeLinker` · `HomeLinkerOf` (Write door for shared home files) | `agentio` | [Doors: Runner, Prober and Writer](../capabilities/doors.md#step-4--link-shared-home-files) | [doors](../../examples/doors/) |

## Built-in and config-defined agents

| Feature | Package | Guide | Example |
|---|---|---|---|
| `builtin.New`, `builtin.Agents` (typed `Claude`, `Codex`, `Custom`), `Agents.All` | `builtin` | [Built-in agents](../capabilities/builtin.md#step-1--build-every-agent) | [accessors](../../examples/accessors/), [agentinfo](../../examples/agentinfo/) |
| `Agents.ByID` | `builtin` | [Built-in agents](../capabilities/builtin.md#step-2--find-an-agent-by-id) | [accessors](../../examples/accessors/) |
| Config-defined agents passed to `builtin.New(h, specs...)` | `builtin` | [Built-in agents](../capabilities/builtin.md#step-3--add-config-defined-agents) | [accessors](../../examples/accessors/) |
| Describe a plain agent as a `Spec` (binary, home env, resume args, process match) | `agentio` | [Config-defined agents](../capabilities/fromspec.md#step-1--describe-the-agent-as-a-spec) | [fromspec](../../examples/fromspec/) |
| `FromSpec` | `agentio` | [Config-defined agents](../capabilities/fromspec.md#step-2--build-it-with-fromspec) | [fromspec](../../examples/fromspec/) |
| Launch a config-defined agent (`Launch`, `LoginCommand`, `Checks` from the Spec) | `agentio` | [Config-defined agents](../capabilities/fromspec.md#step-3--launch-it) | [fromspec](../../examples/fromspec/) |

## Compat and drift

| Feature | Package | Guide | Example |
|---|---|---|---|
| `TestedMin` / `TestedMax`, `claude.Compat`, `codex.Compat`, `Compat` | `claude`, `codex`, `agentio` | [Tested vendor versions](../compat/compat.md#step-1--read-the-tested-range) | [compat](../../examples/compat/), [agentinfo](../../examples/agentinfo/) |
| `Compat.Check`, `Compat.Warning`, `CompatStatus` | `agentio` | [Tested vendor versions](../compat/compat.md#step-2--check-the-installed-version) | [compat](../../examples/compat/), [agentinfo](../../examples/agentinfo/) |
| `claude.Feature` / `codex.Feature`, `claude.Features`, `codex.FeatureValues`, `FeatureSource`, `features.tsv` | `claude`, `codex` | [Per-feature version table](../compat/features.md#step-1--list-features) | [compat](../../examples/compat/), [drift](../../examples/drift/) |
| `FeatureInfo`, `FeatureSpan` (`In`, `Known`) | `claude`, `codex`, `agentio` | [Per-feature version table](../compat/features.md#step-2--check-a-feature-against-a-version) | [compat](../../examples/compat/) |
| `claude.FeatureUnavailableError`, `codex.FeatureUnavailableError`, `ErrFeatureUnavailable` | `claude`, `codex` | [Per-feature version table](../compat/features.md#step-3--handle-featureunavailableerror) | [compat](../../examples/compat/) |
| Drift check (`task drift`, `examples/drift`): unknown keys, unknown enum values, unknown records, missing flags | `examples` | [Drift check](../compat/drift.md#step-1--run-the-drift-check) | [drift](../../examples/drift/) |
| Drift findings and kinds | `examples` | [Drift check](../compat/drift.md#step-2--read-the-findings) | [drift](../../examples/drift/) |
| Drift baseline comparison (`-baseline`, `-claude-help`) | `examples` | [Drift check](../compat/drift.md#step-3--compare-with-a-baseline) | [drift](../../examples/drift/) |

## Testing: agenttest and fixtures

| Feature | Package | Guide | Example |
|---|---|---|---|
| `agenttest.NewHost`, `HostOption` (`WithEnv`, `WithUserHome`, `WithNow`, `WithHTTP`, `WithSecrets`, `WithProcesses`, `WithExec`, `WithSystem`), `FixedNow` | `agenttest` | [Fakes and fixture hosts](../testing/agenttest.md#step-1--build-a-fixture-host) | [testing](../../examples/testing/) |
| `FakeSecrets`, `NewFakeSecrets`, `SecretKey`, `FakeProcesses` | `agenttest` | [Fakes and fixture hosts](../testing/agenttest.md#step-2--fake-secrets-and-processes) | [testing](../../examples/testing/) |
| `FakeExec`, `FailExec`, `ErrForbidden`, `ErrFakeNotFound` | `agenttest` | [Fakes and fixture hosts](../testing/agenttest.md#step-3--fake-the-vendor-binary) | [testing](../../examples/testing/) |
| `FailingHTTP`, `WithHTTP` + `httptest` | `agenttest` | [Fakes and fixture hosts](../testing/agenttest.md#step-4--point-http-at-a-test-server) | [testing](../../examples/testing/) |
| `agenttest.Conformance` | `agenttest` | [The conformance suite](../testing/conformance.md#step-1--run-conformance-on-an-agent) | [testing](../../examples/testing/) |
| Fixture homes (`claude/testdata/homes/<ver>/userhome`, `codex/testdata/homes/<ver>`) | `claude`, `codex` | [Fixture homes](../testing/fixtures.md#step-1--use-a-checked-in-fixture-home) | [testing](../../examples/testing/), [quickstart](../../examples/quickstart/), [claudesettings](../../examples/claudesettings/) |
| `CopyFixture`, `WriteFile` | `agenttest` | [Fixture homes](../testing/fixtures.md#step-2--copy-a-fixture-before-writing) | [testing](../../examples/testing/) |
| Injecting a `Host` into your own code; testing a loop over agents | `agenttest` | [Testing your own code](../testing/your-code.md#step-1--inject-a-host) | [testing](../../examples/testing/) |

## L3 claude: agent, install, paths

| Feature | Package | Guide | Example |
|---|---|---|---|
| `claude.New`, `ID`, `Spec`, `Binary`, `ConfigDirEnv` | `claude` | [The Claude agent](../claude/agent.md#step-1--create-the-agent) | [claudesettings](../../examples/claudesettings/) |
| `claude.Home`, `HomeAt`, `HomeOf`, `DefaultHome`, `HomeFromEnv`, `Homes`, `IsHome`, `HomeMarkers`, `HomeMarkersIn` | `claude` | [The Claude agent](../claude/agent.md#step-2--pick-a-home) | [claudesettings](../../examples/claudesettings/), [agentinfo](../../examples/agentinfo/) |
| `Agent.Project`, `Project` | `claude` | [The Claude agent](../claude/agent.md#step-3--open-a-project) | [claudesettings](../../examples/claudesettings/), [agentinfo](../../examples/agentinfo/) |
| `Agent.Neutral` (and `Neutral()` on rich types) | `claude` | [The Claude agent](../claude/agent.md#step-4--get-the-neutral-view) | [claudesettings](../../examples/claudesettings/) |
| `Agent.Install`, `Install`, install method constants, `ParseVersion` | `claude` | [Claude install and updates](../claude/install.md#step-1--read-the-install) | [claudesettings](../../examples/claudesettings/), [drift](../../examples/drift/) |
| `Agent.LastUpdate`, `UpdateResult`, `Agent.Changelog` | `claude` | [Claude install and updates](../claude/install.md#step-2--read-the-last-update) | [claudesettings](../../examples/claudesettings/), [drift](../../examples/drift/) |
| `UpdateCommand`, `InstallCommand` | `claude` | [Claude install and updates](../claude/install.md#step-3--build-install-and-update-commands) | [claudesettings](../../examples/claudesettings/) |
| `Agent.Paths`, `Paths`, `PluginPaths` | `claude` | [Claude paths](../claude/paths.md#step-1--get-home-paths) | [claudesettings](../../examples/claudesettings/), [agentinfo](../../examples/agentinfo/) |
| `Agent.ProjectPaths`, `ProjectPaths`, `Agent.SystemPaths`, `SystemPaths` | `claude` | [Claude paths](../claude/paths.md#step-2--get-project-paths) | [claudesettings](../../examples/claudesettings/) |
| `EncodePath`, `Agent.HistoryDir` | `claude` | [Claude paths](../claude/paths.md#step-3--encode-a-history-folder) | [claudesettings](../../examples/claudesettings/) |

## L3 claude: settings, config, permissions, policy

| Feature | Package | Guide | Example |
|---|---|---|---|
| `UserSettings`, `ProjectSettings`, `LocalSettings`, `SettingsLayer`, `Settings`, `ParseSettings` | `claude` | [Claude settings](../claude/settings.md#step-1--read-one-settings-layer) | [claudesettings](../../examples/claudesettings/), [drift](../../examples/drift/) |
| `Agent.Settings`, `EffectiveSettings` | `claude` | [Claude settings](../claude/settings.md#step-2--read-the-effective-settings) | [claudesettings](../../examples/claudesettings/), [agentinfo](../../examples/agentinfo/) |
| `ManagedSettings`, `RemoteSettings`, `Runner.ManagedPlist` | `claude` | [Claude settings](../claude/settings.md#step-3--read-managed-and-remote-settings) | [claudesettings](../../examples/claudesettings/), [drift](../../examples/drift/) |
| `EnvInEffect`, `EnvSetting`, `KnownEnv`, `EnvVar` | `claude` | [Claude settings](../claude/settings.md#step-4--list-environment-variables-in-effect) | [claudesettings](../../examples/claudesettings/) |
| `GlobalConfig`, `ParseGlobalConfig`, `Account` (`OAuthAccount`) | `claude` | [Claude global state (.claude.json)](../claude/global-config.md#step-1--read-the-global-config) | [claudesettings](../../examples/claudesettings/), [drift](../../examples/drift/) |
| `ProjectState`, `CachedUsageBody` | `claude` | [Claude global state (.claude.json)](../claude/global-config.md#step-2--read-one-projects-state) | [claudesettings](../../examples/claudesettings/) |
| `ConfigBackups` | `claude` | [Claude global state (.claude.json)](../claude/global-config.md#step-3--list-config-backups) | [claudesettings](../../examples/claudesettings/) |
| `Agent.Permissions`, `EffectivePermissions`, `Permissions`, `PermissionMode`, `ScopedRule` | `claude` | [Claude permissions](../claude/permissions.md#step-1--read-the-effective-permissions) | [claudesettings](../../examples/claudesettings/) |
| `ParseRule`, `Rule` | `claude` | [Claude permissions](../claude/permissions.md#step-2--parse-a-rule) | [claudesettings](../../examples/claudesettings/) |
| `PolicyLimits`, `Restriction`, `PolicyStamp` | `claude` | [Claude policy limits](../claude/policy.md#step-1--read-policy-limits) | [claudesettings](../../examples/claudesettings/), [drift](../../examples/drift/) |

## L3 claude: hooks, statusline, extensions

| Feature | Package | Guide | Example |
|---|---|---|---|
| `Agent.Hooks`, `ScopedHook`, `Hooks`, `HookMatcher`, `HookHandler`, `HookEvents`, `HookEvent.SupportsMatcher`, `HookMatcher.Matches` | `claude` | [Claude hooks](../claude/hooks.md#step-1--list-configured-hooks) | [claudehooks](../../examples/claudehooks/), [drift](../../examples/drift/) |
| `DecodeHookInput`, `HookInput` | `claude` | [Claude hooks](../claude/hooks.md#step-2--decode-hook-input) | [claudehooks](../../examples/claudehooks/) |
| `HookOutput`, `HookSpecificOutput`, `HookOutput.Encode` | `claude` | [Claude hooks](../claude/hooks.md#step-3--reply-from-a-hook) | [claudehooks](../../examples/claudehooks/) |
| `StatusLine` setting | `claude` | [Claude status line](../claude/statusline.md#step-1--read-the-status-line-setting) | [claudehooks](../../examples/claudehooks/) |
| `DecodeStatusLineInput`, `StatusLineInput` (`StatusLineModel`, `StatusLineWorkspace`, `StatusLineCost`, `StatusLineOutputStyle`) | `claude` | [Claude status line](../claude/statusline.md#step-2--decode-status-line-input) | [claudehooks](../../examples/claudehooks/) |
| `MCPServers`, `MCPServer`, `ParseMCPConfig`, `MCPOAuth` | `claude` | [Claude MCP servers](../claude/mcp.md#step-1--list-mcp-servers) | [claudeextensions](../../examples/claudeextensions/), [drift](../../examples/drift/) |
| `MCPNeedsAuth` | `claude` | [Claude MCP servers](../claude/mcp.md#step-2--find-servers-that-need-auth) | [claudeextensions](../../examples/claudeextensions/) |
| `MCPListCommand`, `MCPAddCommand`, `MCPRemoveCommand`, `MCPResetProjectChoicesCommand` | `claude` | [Claude MCP servers](../claude/mcp.md#step-3--build-add-and-remove-commands) | [claudeextensions](../../examples/claudeextensions/) |
| `MemoryFiles`, `MemoryFile` | `claude` | [Claude memory files](../claude/memory.md#step-1--list-memory-files) | [claudeextensions](../../examples/claudeextensions/) |
| `AutoMemory` | `claude` | [Claude memory files](../claude/memory.md#step-2--read-auto-memory) | [claudeextensions](../../examples/claudeextensions/) |
| `ParseImports`, `Import` | `claude` | [Claude memory files](../claude/memory.md#step-3--follow-imports) | [claudeextensions](../../examples/claudeextensions/) |
| `Agent.Skills`, `Skill` | `claude` | [Claude skills, slash commands and workflows](../claude/skills.md#step-1--list-skills) | [claudeextensions](../../examples/claudeextensions/), [drift](../../examples/drift/) |
| `SlashCommands`, `SlashCommand` | `claude` | [Claude skills, slash commands and workflows](../claude/skills.md#step-2--list-slash-commands) | [claudeextensions](../../examples/claudeextensions/), [drift](../../examples/drift/) |
| `Workflows`, `Workflow` | `claude` | [Claude skills, slash commands and workflows](../claude/skills.md#step-3--list-workflows) | [claudeextensions](../../examples/claudeextensions/) |
| `ParseFrontmatter` and the `frontmatter` package (`Parse`, `ParseYAML`, `Split`, `Doc`, `Field`, `Value`, `Kind`) | `claude`, `agentio/frontmatter` | [Claude skills, slash commands and workflows](../claude/skills.md#step-4--parse-frontmatter) | [claudeextensions](../../examples/claudeextensions/) |
| `Agent.Plugins`, `InstalledPlugin`, `PluginID`, `PluginID.Split` | `claude` | [Claude plugins and marketplaces](../claude/plugins.md#step-1--list-installed-plugins) | [claudeextensions](../../examples/claudeextensions/), [drift](../../examples/drift/) |
| `Marketplaces`, `Marketplace`, `MarketplaceDecl`, `MarketplaceSource`, `PluginBlocklist`, `Blocklist` | `claude` | [Claude plugins and marketplaces](../claude/plugins.md#step-2--list-marketplaces-and-the-blocklist) | [claudeextensions](../../examples/claudeextensions/), [drift](../../examples/drift/) |
| `PluginManifest` | `claude` | [Claude plugins and marketplaces](../claude/plugins.md#step-3--read-a-plugin-manifest) | [claudeextensions](../../examples/claudeextensions/) |
| `PluginInstallCommand`, `PluginEnableCommand`, `PluginDisableCommand`, `MarketplaceAddCommand`, `MarketplaceUpdateCommand` | `claude` | [Claude plugins and marketplaces](../claude/plugins.md#step-4--build-plugin-commands) | [claudeextensions](../../examples/claudeextensions/) |
| `SubagentDefs`, `SubagentDef` | `claude` | [Claude subagents](../claude/subagents.md#step-1--list-subagent-definitions) | [claudeextensions](../../examples/claudeextensions/), [drift](../../examples/drift/) |
| `SubagentRuns`, `SubagentRun`, `SubagentStats` | `claude` | [Claude subagents](../claude/subagents.md#step-2--list-subagent-runs) | [claudeextensions](../../examples/claudeextensions/) |
| `EncodeAgentsFlag` | `claude` | [Claude subagents](../claude/subagents.md#step-3--pass-subagents-on-the-command-line) | [claudeextensions](../../examples/claudeextensions/) |
| `OutputStyles`, `OutputStyle` | `claude` | [Claude output styles](../claude/output-styles.md#step-1--list-output-styles) | [claudeextensions](../../examples/claudeextensions/), [drift](../../examples/drift/) |

## L3 claude: local data

| Feature | Package | Guide | Example |
|---|---|---|---|
| `FileHistory`, `Checkpoint` | `claude` | [Claude local data](../claude/local-data.md#step-1--read-file-history) | [claudeextensions](../../examples/claudeextensions/) |
| `ShellSnapshots`, `ShellSnapshot`, `LastCleanup` | `claude` | [Claude local data](../claude/local-data.md#step-2--read-shell-snapshots-and-cleanup) | [claudeextensions](../../examples/claudeextensions/) |
| `IDELocks`, `IDELock`, `DebugLogs`, `DebugLog` | `claude` | [Claude local data](../claude/local-data.md#step-3--read-ide-locks-and-debug-logs) | [claudeextensions](../../examples/claudeextensions/), [drift](../../examples/drift/) |
| `DataUsage`, `DataEntry`, `ModelCatalogFiles` | `claude` | [Claude local data](../claude/local-data.md#step-4--read-data-usage-and-the-changelog) | [claudeextensions](../../examples/claudeextensions/) |

## L3 claude: writer

| Feature | Package | Guide | Example |
|---|---|---|---|
| `Writer.SetUserSetting`, `Writer.SetProjectSetting`, `SettingsPatch` | `claude` | [Claude writer: settings and hooks](../claude/writer.md#step-1--patch-a-settings-file) | [claudehooks](../../examples/claudehooks/) |
| `Writer.AddHook` | `claude` | [Claude writer: settings and hooks](../claude/writer.md#step-2--add-a-hook) | [claudehooks](../../examples/claudehooks/) |
| `Writer.RemoveHooks` | `claude` | [Claude writer: settings and hooks](../claude/writer.md#step-3--remove-hooks) | [claudehooks](../../examples/claudehooks/) |
| `Writer.MoveHistory`, `Writer.ApplyMove`, `Writer.ApplyHomeLinks`, `ErrHistoryExists`, `LiveSessionError` | `claude` | [Claude writer: settings and hooks](../claude/writer.md#step-4--move-history) | [claudehooks](../../examples/claudehooks/) |

## L3 claude: credentials, account, usage, stats

| Feature | Package | Guide | Example |
|---|---|---|---|
| `Agent.Credential`, `CredentialContext`, `Credential`, `Credential.Expired`, `KeychainServices`, `CredentialSummary` | `claude` | [Claude credentials and tokens](../claude/credential.md#step-1--read-the-credential) | [claudeaccount](../../examples/claudeaccount/) |
| `TokenKinds`, `DetectTokenKind`, `TokenLifetime`, `SetupTokenLifetime`, `ErrUnusableToken` | `claude` | [Claude credentials and tokens](../claude/credential.md#step-2--detect-a-tokens-kind) | [claudeaccount](../../examples/claudeaccount/) |
| `TokenEnv` | `claude` | [Claude credentials and tokens](../claude/credential.md#step-3--build-a-token-environment) | [claudeaccount](../../examples/claudeaccount/) |
| `LoginCommand` (`LoginMode`), `LogoutCommand`, `SetupTokenCommand` | `claude` | [Claude credentials and tokens](../claude/credential.md#step-4--build-login-commands) | [claudeaccount](../../examples/claudeaccount/) |
| `Agent.Identity`, `Identity` | `claude` | [Claude account and identity](../claude/account.md#step-1--read-the-identity) | [claudeaccount](../../examples/claudeaccount/) |
| `Account`, `OAuthAccount`, `ParseProfile`, `PlanLabel`, `Subscription`, `ErrNoOrganization` | `claude` | [Claude account and identity](../claude/account.md#step-2--read-the-oauth-account) | [claudeaccount](../../examples/claudeaccount/) |
| `Runner.AuthStatus`, `AuthStatus`, `ParseAuthStatus` | `claude` | [Claude account and identity](../claude/account.md#step-3--read-auth-status-through-the-runner) | [claudeaccount](../../examples/claudeaccount/) |
| `Agent.Usage`, `Usage`, `Limit`, `Limit.Label`, `ParseUsage`, `WithBaseURL`, `DefaultBaseURL`, `ErrNoLimits` | `claude` | [Claude plan usage](../claude/usage.md#step-1--read-live-usage) | [claudeusage](../../examples/claudeusage/) |
| `Agent.CachedUsage`, `CachedUsage` | `claude` | [Claude plan usage](../claude/usage.md#step-2--read-cached-usage) | [claudeusage](../../examples/claudeusage/) |
| `ExtraUsageOf`, `ExtraUsage` | `claude` | [Claude plan usage](../claude/usage.md#step-3--read-extra-usage) | [claudeusage](../../examples/claudeusage/) |
| `Prober.TokenUsage`, `ParseProbeHeaders`, `ErrProbeHeaders` | `claude` | [Claude plan usage](../claude/usage.md#step-4--probe-a-tokens-usage) | [claudeusage](../../examples/claudeusage/) |
| `Stats`, `ParseStats`, `DailyActivity`, `DailyTokens`, `ModelUsage`, `LongestSession` | `claude` | [Claude stats](../claude/stats.md#step-1--read-the-stats-cache) | [claudeconversations](../../examples/claudeconversations/), [drift](../../examples/drift/) |

## L3 claude: conversations, transcripts, history

| Feature | Package | Guide | Example |
|---|---|---|---|
| `Agent.Conversations`, `Conversation` | `claude` | [Claude conversations](../claude/conversations.md#step-1--list-conversations) | [claudeconversations](../../examples/claudeconversations/), [drift](../../examples/drift/) |
| `Agent.Conversation`, `HasHistory`, `ErrAmbiguousID` | `claude` | [Claude conversations](../claude/conversations.md#step-2--open-one-conversation) | [claudeconversations](../../examples/claudeconversations/) |
| `Agent.Turns`, `TurnsFile` | `claude` | [Claude conversations](../claude/conversations.md#step-3--read-turns) | [claudeconversations](../../examples/claudeconversations/) |
| `Agent.PlanMove` | `claude` | [Claude conversations](../claude/conversations.md#step-4--plan-a-move) | [claudeconversations](../../examples/claudeconversations/) |
| `OpenTranscript`, `OpenTranscriptFile`, `TranscriptAt`, `TranscriptReader`, `ParseRecord`, `Record`, `RecordTypes`, `ErrInternalRecord`, `ErrWrongRecordType` | `claude` | [Claude transcripts](../claude/transcript.md#step-1--open-a-transcript) | [claudeconversations](../../examples/claudeconversations/) |
| `TranscriptTail`, `TranscriptTailFile`, `Record.As*` (`AssistantRecord`, `UserRecord`, `Title`, `LastPrompt`, …), `Content.Text` | `claude` | [Claude transcripts](../claude/transcript.md#step-2--read-the-tail) | [claudeconversations](../../examples/claudeconversations/), [drift](../../examples/drift/) |
| `LastAssistant` | `claude` | [Claude transcripts](../claude/transcript.md#step-3--read-the-last-assistant-reply) | [claudeconversations](../../examples/claudeconversations/) |
| `SessionCost` (`CostState`), `PRLinks` (`PRLink`), `InternalRecords` | `claude` | [Claude transcripts](../claude/transcript.md#step-4--read-cost-and-pr-links) | [claudeconversations](../../examples/claudeconversations/) |
| `PromptHistory`, `HistoryQuery`, `HistoryEntry`, `ParseHistoryLine` | `claude` | [Claude prompt history](../claude/history.md#step-1--read-prompt-history) | [claudeconversations](../../examples/claudeconversations/), [drift](../../examples/drift/) |
| `PastedContent`, `Paste` | `claude` | [Claude prompt history](../claude/history.md#step-2--read-pasted-content) | [claudeconversations](../../examples/claudeconversations/) |

## L3 claude: sessions, stream, launch, runner, doctor

| Feature | Package | Guide | Example |
|---|---|---|---|
| `LiveSessions`, `LiveSessionsIn`, `IsLive`, `LiveSession`, `SessionStatus` | `claude` | [Claude live sessions and background work](../claude/sessions.md#step-1--list-live-sessions) | [claudesessions](../../examples/claudesessions/), [drift](../../examples/drift/) |
| `ClassifyProcess`, `Agent.Processes`, `LiveProcess` | `claude` | [Claude live sessions and background work](../claude/sessions.md#step-2--list-agent-processes) | [claudesessions](../../examples/claudesessions/) |
| `Jobs`, `JobTimeline`, `Job`, `JobEvent`, `Daemon`, `DaemonStatus`, `AttachCommand`, `StopCommand`, `LogsCommand` | `claude` | [Claude live sessions and background work](../claude/sessions.md#step-3--read-jobs-and-the-daemon) | [claudesessions](../../examples/claudesessions/), [drift](../../examples/drift/) |
| `Teams`, `Team`, `TaskLists`, `TaskList`, `TaskItem` | `claude` | [Claude live sessions and background work](../claude/sessions.md#step-4--read-teams-and-task-lists) | [claudesessions](../../examples/claudesessions/), [drift](../../examples/drift/) |
| `NewStreamDecoder`, `StreamDecoder.Next`, `StreamMessage.As*` (`StreamInit`, `StreamAssistantMessage`, `StreamResult`, `RateLimitEvent`, `APIRetry`, …), `ErrWrongStreamMessage` | `claude` | [Claude stream decoding](../claude/stream.md#step-1--decode-a-stream-json-run) | [claudestream](../../examples/claudestream/) |
| `DecodeJSONResult` | `claude` | [Claude stream decoding](../claude/stream.md#step-2--decode-a-json-result) | [claudestream](../../examples/claudestream/) |
| `StreamMessage.Neutral` (to `agentio.RunEvent`) | `claude` | [Claude stream decoding](../claude/stream.md#step-3--convert-to-neutral-run-events) | [claudestream](../../examples/claudestream/) |
| `Agent.Launch`, `LaunchOptions`, `LaunchOptions.Args`, `ErrInvalidLaunch` | `claude` | [Claude launch and resume](../claude/launch.md#step-1--build-a-launch-command) | [claudelaunch](../../examples/claudelaunch/) |
| Resume / continue with `LaunchOptions` | `claude` | [Claude launch and resume](../claude/launch.md#step-2--resume-a-conversation) | [claudelaunch](../../examples/claudelaunch/) |
| `TargetFor` (fills `Target.Version`) | `claude` | [Claude launch and resume](../claude/launch.md#step-3--fill-the-target-version) | [claudelaunch](../../examples/claudelaunch/) |
| `PurgeCommand` | `claude` | [Claude launch and resume](../claude/launch.md#step-1--build-a-launch-command) | [claudelaunch](../../examples/claudelaunch/) |
| `Runner.BinaryVersion` | `claude` | [Claude Runner](../claude/runner.md#step-1--read-the-binary-version) | [claudelaunch](../../examples/claudelaunch/) |
| `Runner.Plugins` (`RunnerPlugin`), `Runner.Marketplaces` (`RunnerMarketplace`) | `claude` | [Claude Runner](../claude/runner.md#step-2--list-plugins-and-marketplaces) | [claudelaunch](../../examples/claudelaunch/) |
| `Runner.BackgroundAgents` (`BackgroundAgent`), `Runner.AutoModeConfig`, `Runner.AutoModeDefaults` | `claude` | [Claude Runner](../claude/runner.md#step-3--read-background-agents-and-auto-mode) | [claudelaunch](../../examples/claudelaunch/) |
| `Agent.Doctor`, `DoctorOptions`, Claude `Check*` IDs | `claude` | [Claude doctor](../claude/doctor.md#step-1--run-the-checks) | [claudelaunch](../../examples/claudelaunch/) |

## L3 codex: agent, install, paths, config

| Feature | Package | Guide | Example |
|---|---|---|---|
| `codex.New`, `ID`, `Spec`, `HomeEnv`, `InstallationID` | `codex` | [The Codex agent](../codex/agent.md#step-1--create-the-agent) | [codexconfig](../../examples/codexconfig/) |
| `codex.Home`, `HomeAt`, `HomeOf`, `DefaultHome`, `HomeFromEnv`, `Homes`, `IsHome` | `codex` | [The Codex agent](../codex/agent.md#step-2--pick-a-home) | [codexconfig](../../examples/codexconfig/), [agentinfo](../../examples/agentinfo/) |
| `Agent.Neutral` | `codex` | [The Codex agent](../codex/agent.md#step-3--get-the-neutral-view) | [codexconfig](../../examples/codexconfig/) |
| `Agent.Install`, `Install`, `ParseVersion`, `UpdateCommand` | `codex` | [Codex install and updates](../codex/install.md#step-1--read-the-install) | [codexconfig](../../examples/codexconfig/), [drift](../../examples/drift/) |
| `UpdateCache` | `codex` | [Codex install and updates](../codex/install.md#step-2--read-the-update-cache) | [codexconfig](../../examples/codexconfig/) |
| `Runner.BinaryVersion`, `Runner.LoginStatus`, `Runner.Features`, `ParseFeatureList`, `Runner.BundledModels` | `codex` | [Codex install and updates](../codex/install.md#step-3--run-the-binary-through-the-runner) | [codexconfig](../../examples/codexconfig/) |
| `Agent.Paths`, `PathsWith`, `Paths`, `Paths.ProfileConfig` | `codex` | [Codex paths and inventory](../codex/paths.md#step-1--get-home-paths) | [codexconfig](../../examples/codexconfig/), [agentinfo](../../examples/agentinfo/) |
| `Inventory`, `InventoryEntry` | `codex` | [Codex paths and inventory](../codex/paths.md#step-2--list-the-home-inventory) | [codexconfig](../../examples/codexconfig/) |
| `ConfigFile`, `ConfigFileWarnings`, `ParseConfig`, `Config`, `Config.KeyCount`, `TOMLRaw`, `UnknownTOML` | `codex` | [Codex config.toml](../codex/config.md#step-1--read-configtoml) | [codexconfig](../../examples/codexconfig/), [agentinfo](../../examples/agentinfo/) |
| `Effective`, `EffectiveQuery`, `EffectiveConfig`, `ConfigLayers`, `Layer`, `Config.TrustOf`, `Config.FeatureOverrides` | `codex` | [Codex config.toml](../codex/config.md#step-2--read-the-effective-config) | [codexconfig](../../examples/codexconfig/) |
| `Profiles`, `ProfileFiles`, `ProfileInfo`, `Config.LegacyProfileNames`, `Config.ProfileConflict` | `codex` | [Codex config.toml](../codex/config.md#step-3--read-profiles) | [codexconfig](../../examples/codexconfig/), [agentinfo](../../examples/agentinfo/) |
| Codex config enums (`ApprovalPolicy`, `SandboxMode`, `ReasoningEffort`, `ReasoningSummary`, `Verbosity`, `WebSearchMode`, `TrustLevel`, `AuthMode`, …) as open string types with `IsKnown`; `Opt[T]` alias | `codex` | [Codex config.toml](../codex/config.md#step-1--read-configtoml) | [codexconfig](../../examples/codexconfig/), [agentinfo](../../examples/agentinfo/) |
| `Override`, `OverrideString`, `OverrideBool`, `OverrideInt`, `OverrideStrings`, `ParseOverride` | `codex` | [Codex config.toml](../codex/config.md#step-4--build-config-overrides) | [codexconfig](../../examples/codexconfig/) |
| `MCPServers`, `MCPServerEntry`, `MCPServerEnvValue`, `MCPServer.Transport` | `codex` | [Codex MCP servers](../codex/mcp.md#step-1--list-mcp-servers) | [codexconfig](../../examples/codexconfig/) |
| `MCPAddCommand` (`MCPAddSpec`), `MCPRemoveCommand` | `codex` | [Codex MCP servers](../codex/mcp.md#step-2--build-add-and-remove-commands) | [codexconfig](../../examples/codexconfig/) |
| `InstructionFiles`, `InstructionFile`, `ProjectRoot` | `codex` | [Codex instructions, skills and plugins](../codex/instructions-skills.md#step-1--list-instruction-files) | [codexconfig](../../examples/codexconfig/) |
| `Agent.Skills`, `Skill` | `codex` | [Codex instructions, skills and plugins](../codex/instructions-skills.md#step-2--list-skills) | [codexconfig](../../examples/codexconfig/) |
| `Agent.Plugins`, `Plugin`, `Marketplaces`, `Marketplace`, `PluginListCommand` | `codex` | [Codex instructions, skills and plugins](../codex/instructions-skills.md#step-3--list-plugins-and-marketplaces) | [codexconfig](../../examples/codexconfig/) |
| `HookConfig`, `HookState`, `HooksFile`, `ParseHooksFile`, `HookMatcherGroup`, `HookEvent` | `codex` | [Codex hooks and rules](../codex/hooks-rules.md#step-1--read-hook-config) | [codexconfig](../../examples/codexconfig/), [agentinfo](../../examples/agentinfo/) |
| `DecodeHookInput`, `DecodeNotify`, `HookInput`, `NotifyPayload` | `codex` | [Codex hooks and rules](../codex/hooks-rules.md#step-2--decode-hook-input-and-notify-payloads) | [codexconfig](../../examples/codexconfig/) |
| `Rules`, `ParseRules`, `PrefixRule`, `RuleDecision`, `RuleParseIssue`, `ParseSessionSource` | `codex` | [Codex hooks and rules](../codex/hooks-rules.md#step-3--read-execpolicy-rules) | [codexconfig](../../examples/codexconfig/) |
| `Agent.Checks` (doctor) | `codex` | [Codex doctor](../codex/doctor.md#step-1--run-the-checks) | [codexconfig](../../examples/codexconfig/) |

## L3 codex: auth, usage, conversations, sessions, launch

| Feature | Package | Guide | Example |
|---|---|---|---|
| `Agent.Identity`, `IdentityContext`, `ParseAuth`, `ParsePlan`, `Plan`, `KeyringAccount`, `KeyringEntry`, `KeyringError` | `codex` | [Codex auth, identity and usage](../codex/auth-usage.md#step-1--read-the-identity) | [codexaccount](../../examples/codexaccount/), [drift](../../examples/drift/) |
| `Agent.Usage`, `Agent.WithBaseURL`, `DefaultBaseURL`, `UsageEndpoint`, `ParseUsage`, `Usage`, `UsageWindow`, `Agent.Subscription`, `Subscription` | `codex` | [Codex auth, identity and usage](../codex/auth-usage.md#step-2--read-live-usage) | [codexaccount](../../examples/codexaccount/) |
| `Agent.CachedUsage` (rollout `token_count`) | `codex` | [Codex auth, identity and usage](../codex/auth-usage.md#step-3--read-cached-usage) | [codexaccount](../../examples/codexaccount/) |
| `LoginCommand`, `LogoutCommand` | `codex` | [Codex auth, identity and usage](../codex/auth-usage.md#step-4--build-login-commands) | [codexaccount](../../examples/codexaccount/) |
| `Agent.Conversations`, `ConversationQuery`, `Conversation`, `Titles`, `Agent.Conversation`, `HasHistory`, `MoveUnsupportedReason` | `codex` | [Codex conversations and history](../codex/conversations.md#step-1--list-conversations) | [codexconversations](../../examples/codexconversations/), [drift](../../examples/drift/) |
| `Agent.Turns` | `codex` | [Codex conversations and history](../codex/conversations.md#step-2--read-turns) | [codexconversations](../../examples/codexconversations/) |
| `Agent.PromptHistory`, `PromptEntry` | `codex` | [Codex conversations and history](../codex/conversations.md#step-3--read-prompt-history) | [codexconversations](../../examples/codexconversations/) |
| `Agent.Models`, `ParseModelCatalog`, `ModelCatalog`, `ModelCatalog.Validate`, `ModelInfo` | `codex` | [Codex conversations and history](../codex/conversations.md#step-4--list-models) | [codexconversations](../../examples/codexconversations/) |
| `OpenRollout`, `OpenRolloutAt`, `RolloutReader`, `ParseRecord`, `Record`, `SessionMeta` | `codex` | [Codex rollout files](../codex/rollout.md#step-1--open-a-rollout) | [codexconversations](../../examples/codexconversations/) |
| `RolloutTail` | `codex` | [Codex rollout files](../codex/rollout.md#step-2--read-the-tail) | [codexconversations](../../examples/codexconversations/) |
| `ConversationTokens`, `TokenCounts` | `codex` | [Codex rollout files](../codex/rollout.md#step-3--sum-conversation-tokens) | [codexconversations](../../examples/codexconversations/) |
| `LiveSessions`, `IsLive`, `LiveConversation` | `codex` | [Codex live sessions and processes](../codex/sessions.md#step-1--list-live-sessions) | [codexsessions](../../examples/codexsessions/) |
| `ThreadLocks`, `ThreadLock`, `ParseThreadLockName` | `codex` | [Codex live sessions and processes](../codex/sessions.md#step-2--read-thread-writer-locks) | [codexsessions](../../examples/codexsessions/) |
| `ClassifyProcess`, `Processes`, `LiveProcess`, `ChatProcesses`, `ChatProcess`, `DaemonSocket` | `codex` | [Codex live sessions and processes](../codex/sessions.md#step-3--list-processes-and-chat-processes) | [codexsessions](../../examples/codexsessions/) |
| `Agent.New`, `Resume`, `ResumeLast`, `Fork`, `LaunchOptions`, `TargetFor` | `codex` | [Codex launch, resume and exec](../codex/launch.md#step-1--start-and-resume) | [codexlaunch](../../examples/codexlaunch/) |
| `Exec`, `ExecResume`, `ExecOptions`, `Review`, `ReviewTarget` | `codex` | [Codex launch, resume and exec](../codex/launch.md#step-2--run-exec-headless) | [codexlaunch](../../examples/codexlaunch/) |
| `Archive`, `Unarchive`, `Delete`, `Queue`, `DoctorCommand`, `MCPServer`, `AppServer`, `FeatureCommand` | `codex` | [Codex launch, resume and exec](../codex/launch.md#step-3--build-archive-delete-and-queue-commands) | [codexlaunch](../../examples/codexlaunch/) |
| `codex.FeatureUnavailableError` in launch builders | `codex` | [Codex launch, resume and exec](../codex/launch.md#step-4--handle-unavailable-features) | [codexlaunch](../../examples/codexlaunch/) |
| `NewExecDecoder`, `ExecDecoder.Next`, `ParseExecEvent`, `ExecEvent`, `ExecItem`, `ExecEventVersion` | `codex` | [Codex exec event decoding](../codex/exec.md#step-1--decode-exec-events) | [codexlaunch](../../examples/codexlaunch/) |
| `NewRunDecoder` (to `agentio.RunEvent`) | `codex` | [Codex exec event decoding](../codex/exec.md#step-2--convert-to-neutral-run-events) | [codexlaunch](../../examples/codexlaunch/) |

## L3 codex/state

| Feature | Package | Guide | Example |
|---|---|---|---|
| `state.Open`, `DB`, `DB.Path`, `DB.Close`, `DB.SchemaVersion`, `File`, `ErrSchemaTooNew`, `SchemaTooNewError` | `codex/state` | [Codex SQLite state (codex/state)](../codex/state.md#step-1--open-the-state-database) | [codexstate](../../examples/codexstate/) |
| `DB.Threads`, `DB.Thread`, `Thread`, `ThreadStatusFilter`, `Thread.Conversation`, `DB.SubAgents`, `SpawnEdge`, `DB.Projects` | `codex/state` | [Codex SQLite state (codex/state)](../codex/state.md#step-2--list-threads) | [codexstate](../../examples/codexstate/) |
| `DB.Turns`, `Turn`, `DB.Items`, `Item`, `ItemType` | `codex/state` | [Codex SQLite state (codex/state)](../codex/state.md#step-3--read-turns-and-items) | [codexstate](../../examples/codexstate/) |
| `DB.Goal`, `DB.Queue`, `DB.Memories`, `DB.MemoryJobs`, `DB.Logs`, `LogQuery`, `LogLine` | `codex/state` | [Codex SQLite state (codex/state)](../codex/state.md#step-4--read-goals-queue-memories-and-logs) | [codexstate](../../examples/codexstate/) |
