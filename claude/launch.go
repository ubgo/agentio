package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/ubgo/agentio"
)

// Flags and subcommands at 2.1.323 (`claude --help`, verbatim).
const (
	flagPrint                  = "-p"
	flagOutputFormat           = "--output-format"
	flagInputFormat            = "--input-format"
	flagIncludePartialMessages = "--include-partial-messages"
	flagIncludeHookEvents      = "--include-hook-events"
	flagReplayUserMessages     = "--replay-user-messages"
	flagForwardSubagentText    = "--forward-subagent-text"
	flagVerbose                = "--verbose"
	flagJSONSchema             = "--json-schema"
	flagMaxBudgetUSD           = "--max-budget-usd"
	flagNoSessionPersistence   = "--no-session-persistence"
	flagModel                  = "--model"
	flagFallbackModel          = "--fallback-model"
	flagEffort                 = "--effort"
	flagAgent                  = "--agent"
	flagAgents                 = "--agents"
	flagPermissionMode         = "--permission-mode"
	flagPermissionPrompts      = "--permission-prompts"
	flagDangerouslySkip        = "--dangerously-skip-permissions"
	flagAllowDangerouslySkip   = "--allow-dangerously-skip-permissions"
	flagAllowedTools           = "--allowedTools"
	flagDisallowedTools        = "--disallowedTools"
	flagTools                  = "--tools"
	flagAddDir                 = "--add-dir"
	flagMCPConfig              = "--mcp-config"
	flagStrictMCPConfig        = "--strict-mcp-config"
	flagSettings               = "--settings"
	flagSettingSources         = "--setting-sources"
	flagSystemPrompt           = "--system-prompt"
	flagAppendSystemPrompt     = "--append-system-prompt"
	flagSystemPromptFile       = "--system-prompt-file"
	flagAppendSystemPromptFile = "--append-system-prompt-file"
	flagSystemPromptSnapshot   = "--system-prompt-snapshot"
	flagExcludeDynamicSections = "--exclude-dynamic-system-prompt-sections"
	flagPluginDir              = "--plugin-dir"
	flagPluginURL              = "--plugin-url"
	flagBare                   = "--bare"
	flagSafeMode               = "--safe-mode"
	flagRestricted             = "--restricted"
	flagDisableSlashCommands   = "--disable-slash-commands"
	flagResume                 = "--resume"
	flagContinue               = "--continue"
	flagForkSession            = "--fork-session"
	flagSessionID              = "--session-id"
	flagName                   = "--name"
	flagFromPR                 = "--from-pr"
	flagWorktree               = "--worktree"
	flagTmux                   = "--tmux"
	flagBackground             = "--bg"
	flagDebug                  = "--debug"
	flagDebugFile              = "--debug-file"
	flagIDE                    = "--ide"
	flagChrome                 = "--chrome"
	flagNoChrome               = "--no-chrome"
	flagBrief                  = "--brief"
	flagAutocompact            = "--autocompact"
	flagRemoteControl          = "--remote-control"
	flagBetas                  = "--betas"
	flagFile                   = "--file"
	flagVersion                = "--version"
	flagJSON                   = "--json"
	flagAll                    = "--all"
	flagAvailable              = "--available"
	flagDryRun                 = "--dry-run"
	flagScope                  = "-s"
	flagEmail                  = "--email"
	flagClaudeAI               = "--claudeai"
	flagConsole                = "--console"
	flagSSO                    = "--sso"
	subAuth                    = "auth"
	subLogin                   = "login"
	subLogout                  = "logout"
	subStatus                  = "status"
	subSetupToken              = "setup-token"
	subPlugin                  = "plugin"
	subList                    = "list"
	subMarketplace             = "marketplace"
	subInstall                 = "install"
	subEnable                  = "enable"
	subDisable                 = "disable"
	subAdd                     = "add"
	subUpdate                  = "update"
	subMCP                     = "mcp"
	subAddJSON                 = "add-json"
	subRemove                  = "remove"
	subResetProjectChoices     = "reset-project-choices"
	subAgents                  = "agents"
	subAutoMode                = "auto-mode"
	subConfig                  = "config"
	subDefaults                = "defaults"
	subPurge                   = "purge"
	subAttach                  = "attach"
	subStop                    = "stop"
	subLogs                    = "logs"
	listSep                    = ","
)

// OutputFormat is --output-format.
type OutputFormat string

// Output formats.
const (
	OutputText       OutputFormat = "text"
	OutputJSON       OutputFormat = "json"
	OutputStreamJSON OutputFormat = "stream-json"
)

// InputFormat is --input-format.
type InputFormat string

// Input formats.
const (
	InputText       InputFormat = "text"
	InputStreamJSON InputFormat = "stream-json"
)

// LaunchOptions are Claude's launch flags (CLAUDE_API §25). Zero values
// emit nothing. Flags not modelled (rare / cloud) go through ExtraArgs.
type LaunchOptions struct {
	Prompt                 string
	Print                  bool
	OutputFormat           OutputFormat
	InputFormat            InputFormat
	IncludePartialMessages bool
	IncludeHookEvents      bool
	ReplayUserMessages     bool
	ForwardSubagentText    bool
	Verbose                bool
	JSONSchema             agentio.Raw
	MaxBudgetUSD           *float64
	NoSessionPersistence   bool

	Model          string
	FallbackModels []string
	Effort         Effort
	Agent          string
	Agents         map[string]SubagentDef

	PermissionMode                  PermissionMode
	PermissionPrompts               string
	DangerouslySkipPermissions      bool
	AllowDangerouslySkipPermissions bool
	AllowedTools, DisallowedTools   []string
	// Tools: nil = default; &[]string{} = "" (no tools).
	Tools *[]string

	AddDirs         []string
	MCPConfigs      []string
	StrictMCPConfig bool
	Settings        string
	SettingSources  []SettingsScope

	SystemPrompt, AppendSystemPrompt         string
	SystemPromptFile, AppendSystemPromptFile string
	SystemPromptSnapshot                     string
	ExcludeDynamicSystemPromptSections       bool
	PluginDirs, PluginURLs                   []string
	Bare, SafeMode, Restricted               bool
	DisableSlashCommands                     bool

	// Resume is an id, a name or a transcript path.
	Resume      string
	Continue    bool
	ForkSession bool
	SessionID   string
	Name        string
	FromPR      string
	// Worktree: nil = no flag; &"" = --worktree with no name.
	Worktree      *string
	Tmux          string
	Background    bool
	Debug         string
	DebugFile     string
	IDE           bool
	Chrome        bool
	NoChrome      bool
	Brief         bool
	Autocompact   string
	RemoteControl *string
	Betas         []string
	Files         []string
	// ExtraArgs are appended last, verbatim.
	ExtraArgs []string
}

// ErrInvalidLaunch means a LaunchOptions combination Claude Code rejects.
var ErrInvalidLaunch = errors.New("claude: invalid launch options")

// argBuilder collects argv and the features each flag needs.
type argBuilder struct {
	args  []string
	feats []Feature
}

func (b *argBuilder) flag(f Feature, args ...string) {
	b.args = append(b.args, args...)
	b.feats = append(b.feats, f)
}

func (b *argBuilder) str(f Feature, flag, v string) {
	if v != "" {
		b.flag(f, flag, v)
	}
}

func (b *argBuilder) on(f Feature, flag string, v bool) {
	if v {
		b.flag(f, flag)
	}
}

func (b *argBuilder) list(f Feature, flag string, vs []string) {
	if len(vs) > 0 {
		b.flag(f, append([]string{flag}, vs...)...)
	}
}

// validate rejects combinations Claude Code refuses.
func (o LaunchOptions) validate() error {
	switch {
	case o.OutputFormat != "" && !o.Print:
		return fmt.Errorf("%w: --output-format needs -p", ErrInvalidLaunch)
	case o.InputFormat != "" && !o.Print:
		return fmt.Errorf("%w: --input-format needs -p", ErrInvalidLaunch)
	case o.IncludePartialMessages && o.OutputFormat != OutputStreamJSON:
		return fmt.Errorf("%w: --include-partial-messages needs --output-format stream-json", ErrInvalidLaunch)
	case o.Background && o.Print:
		return fmt.Errorf("%w: --bg cannot run with -p", ErrInvalidLaunch)
	case o.Resume != "" && o.Continue:
		return fmt.Errorf("%w: --resume and --continue are exclusive", ErrInvalidLaunch)
	case o.Chrome && o.NoChrome:
		return fmt.Errorf("%w: --chrome and --no-chrome are exclusive", ErrInvalidLaunch)
	}
	return nil
}

// build renders the argv and the features it needs.
func (o LaunchOptions) build() (argBuilder, error) {
	var b argBuilder
	if err := o.validate(); err != nil {
		return b, err
	}
	b.on(FeatureFlagPrint, flagPrint, o.Print)
	if o.OutputFormat == OutputStreamJSON {
		b.flag(FeatureFlagStreamJSON, flagOutputFormat, string(o.OutputFormat))
	} else {
		b.str(FeatureFlagOutputFormat, flagOutputFormat, string(o.OutputFormat))
	}
	b.str(FeatureFlagInputFormat, flagInputFormat, string(o.InputFormat))
	b.on(FeatureFlagIncludePartialMessages, flagIncludePartialMessages, o.IncludePartialMessages)
	b.on(FeatureFlagIncludeHookEvents, flagIncludeHookEvents, o.IncludeHookEvents)
	b.on(FeatureFlagReplayUserMessages, flagReplayUserMessages, o.ReplayUserMessages)
	b.on(FeatureFlagForwardSubagentText, flagForwardSubagentText, o.ForwardSubagentText)
	b.on(FeatureFlagVerbose, flagVerbose, o.Verbose)
	if len(o.JSONSchema) > 0 {
		b.flag(FeatureFlagJSONSchema, flagJSONSchema, string(o.JSONSchema))
	}
	if o.MaxBudgetUSD != nil {
		b.flag(FeatureFlagMaxBudgetUSD, flagMaxBudgetUSD, strconv.FormatFloat(*o.MaxBudgetUSD, 'f', -1, 64))
	}
	b.on(FeatureFlagNoSessionPersistence, flagNoSessionPersistence, o.NoSessionPersistence)
	b.str(FeatureFlagModel, flagModel, o.Model)
	if len(o.FallbackModels) > 0 {
		f := FeatureFlagFallbackModel
		if !o.Print {
			f = FeatureFallbackModelInteractive
		}
		b.flag(f, flagFallbackModel, strings.Join(o.FallbackModels, listSep))
	}
	b.str(FeatureFlagEffort, flagEffort, string(o.Effort))
	b.str(FeatureFlagAgent, flagAgent, o.Agent)
	if len(o.Agents) > 0 {
		js, err := EncodeAgentsFlag(o.Agents)
		if err != nil {
			return b, err
		}
		b.flag(FeatureFlagAgents, flagAgents, js)
	}
	if o.PermissionMode != "" {
		f := FeatureFlagPermissionMode
		if o.PermissionMode == ModeManual {
			f = FeaturePermissionModeManual
		}
		b.flag(f, flagPermissionMode, string(o.PermissionMode))
	}
	b.str(FeatureFlagPermissionPrompts, flagPermissionPrompts, o.PermissionPrompts)
	b.on(FeatureFlagDangerouslySkipPermissions, flagDangerouslySkip, o.DangerouslySkipPermissions)
	b.on(FeatureFlagAllowDangerouslySkipPermissions, flagAllowDangerouslySkip, o.AllowDangerouslySkipPermissions)
	b.list(FeatureFlagAllowedTools, flagAllowedTools, o.AllowedTools)
	b.list(FeatureFlagDisallowedTools, flagDisallowedTools, o.DisallowedTools)
	if o.Tools != nil {
		b.flag(FeatureFlagTools, flagTools, strings.Join(*o.Tools, listSep))
	}
	for _, d := range o.AddDirs {
		b.flag(FeatureFlagAddDir, flagAddDir, d)
	}
	b.list(FeatureFlagMCPConfig, flagMCPConfig, o.MCPConfigs)
	b.on(FeatureFlagStrictMCPConfig, flagStrictMCPConfig, o.StrictMCPConfig)
	b.str(FeatureFlagSettings, flagSettings, o.Settings)
	if len(o.SettingSources) > 0 {
		parts := make([]string, len(o.SettingSources))
		for i, s := range o.SettingSources {
			parts[i] = string(s)
		}
		b.flag(FeatureFlagSettingSources, flagSettingSources, strings.Join(parts, listSep))
	}
	b.str(FeatureFlagSystemPrompt, flagSystemPrompt, o.SystemPrompt)
	b.str(FeatureFlagAppendSystemPrompt, flagAppendSystemPrompt, o.AppendSystemPrompt)
	b.str(FeatureFlagSystemPromptFile, flagSystemPromptFile, o.SystemPromptFile)
	b.str(FeatureFlagAppendSystemPromptFile, flagAppendSystemPromptFile, o.AppendSystemPromptFile)
	b.str(FeatureFlagSystemPromptSnapshot, flagSystemPromptSnapshot, o.SystemPromptSnapshot)
	b.on(FeatureFlagExcludeDynamicSystemPromptSections, flagExcludeDynamicSections, o.ExcludeDynamicSystemPromptSections)
	for _, d := range o.PluginDirs {
		f := FeatureFlagPluginDir
		if strings.HasSuffix(d, zipExt) {
			f = FeaturePluginDirZip
		}
		b.flag(f, flagPluginDir, d)
	}
	for _, u := range o.PluginURLs {
		b.flag(FeatureFlagPluginURL, flagPluginURL, u)
	}
	b.on(FeatureFlagBare, flagBare, o.Bare)
	b.on(FeatureFlagSafeMode, flagSafeMode, o.SafeMode)
	b.on(FeatureFlagRestricted, flagRestricted, o.Restricted)
	b.on(FeatureFlagDisableSlashCommands, flagDisableSlashCommands, o.DisableSlashCommands)
	b.str(FeatureFlagResume, flagResume, o.Resume)
	b.on(FeatureFlagContinue, flagContinue, o.Continue)
	b.on(FeatureFlagForkSession, flagForkSession, o.ForkSession)
	if o.SessionID != "" {
		f := FeatureFlagSessionID
		if o.ForkSession {
			f = FeatureSessionIDWithFork
		}
		b.flag(f, flagSessionID, o.SessionID)
	}
	b.str(FeatureFlagName, flagName, o.Name)
	b.str(FeatureFlagFromPR, flagFromPR, o.FromPR)
	if o.Worktree != nil {
		if *o.Worktree == "" {
			b.flag(FeatureFlagWorktree, flagWorktree)
		} else {
			b.flag(FeatureFlagWorktree, flagWorktree, *o.Worktree)
		}
	}
	b.str(FeatureFlagTmux, flagTmux, o.Tmux)
	b.on(FeatureFlagBackground, flagBackground, o.Background)
	b.str(FeatureFlagDebug, flagDebug, o.Debug)
	b.str(FeatureFlagDebugFile, flagDebugFile, o.DebugFile)
	b.on(FeatureFlagIDE, flagIDE, o.IDE)
	b.on(FeatureFlagChrome, flagChrome, o.Chrome)
	b.on(FeatureFlagNoChrome, flagNoChrome, o.NoChrome)
	b.on(FeatureFlagBrief, flagBrief, o.Brief)
	b.str(FeatureFlagAutocompact, flagAutocompact, o.Autocompact)
	if o.RemoteControl != nil {
		if *o.RemoteControl == "" {
			b.flag(FeatureFlagRemoteControl, flagRemoteControl)
		} else {
			b.flag(FeatureFlagRemoteControl, flagRemoteControl, *o.RemoteControl)
		}
	}
	b.list(FeatureFlagBetas, flagBetas, o.Betas)
	for _, f := range o.Files {
		b.flag(FeatureFlagFile, flagFile, f)
	}
	b.args = append(b.args, o.ExtraArgs...)
	if o.Prompt != "" {
		// "--" keeps a prompt starting with '-' from being read as a flag.
		b.args = append(b.args, argsEnd, o.Prompt)
	}
	return b, nil
}

// Argv markers.
const (
	argsEnd = "--"
	zipExt  = ".zip"
)

// Args renders the flags without version checks. cost: Pure.
func (o LaunchOptions) Args() ([]string, error) {
	b, err := o.build()
	return b.args, err
}

// Launch builds the command for t. Default home: CLAUDE_CONFIG_DIR is
// removed from the env (invariant 2); others set it. Each emitted flag is
// checked against t.Version: provably unavailable → *FeatureUnavailableError;
// unknown version → built with a warning; outside Compat() → built with an
// "untested" warning (SPEC §5.8). cost: Pure.
func (a *Agent) Launch(t agentio.Target, o LaunchOptions) (agentio.Command, error) {
	return a.launch(t, o, nil)
}

func (a *Agent) launch(t agentio.Target, o LaunchOptions, tokenEnv []agentio.EnvChange) (agentio.Command, error) {
	b, err := o.build()
	if err != nil {
		return agentio.Command{}, err
	}
	for _, f := range b.feats {
		if err := checkFeature(f, t.Version); err != nil {
			return agentio.Command{}, err
		}
	}
	argv := slices.Concat(t.ExtraArgs, b.args)
	cmd, err := a.core.Command(t, "", argv, tokenEnv)
	if err != nil {
		return agentio.Command{}, err
	}
	cmd.Warnings = append(cmd.Warnings, versionWarnings(t.Version)...)
	return cmd, nil
}

// versionWarnings adds the unknown-version and untested-version warnings.
func versionWarnings(v agentio.Version) agentio.Warnings {
	if v.IsZero() {
		return agentio.Warnings{{Kind: agentio.WarnVersionUnknown, Detail: "installed Claude Code version unknown; flags not checked"}}
	}
	if w, ok := Compat().Warning(v); ok {
		return agentio.Warnings{w}
	}
	return nil
}

// TargetFor fills a target's Version from the on-disk install, never by
// running the binary. cost: Disk · Read.
func (a *Agent) TargetFor(h Home) (agentio.Target, error) {
	t := h.Target()
	inst, err := a.Install()
	if err != nil && !errors.Is(err, agentio.ErrNotInstalled) {
		return t, err
	}
	t.Version = inst.Version
	return t, nil
}

// launchFromRequest maps the neutral request onto LaunchOptions
// (SPEC §5.4 table). A neutral level with no exact Claude mode is refused.
func launchFromRequest(r agentio.LaunchRequest) (LaunchOptions, error) {
	o := LaunchOptions{Prompt: r.Prompt, Resume: r.ResumeID, Model: r.Model, AddDirs: r.AddDirs, ExtraArgs: r.Args}
	switch r.Autonomy {
	case agentio.AutonomyDefault:
	case agentio.AutonomyReadOnly:
		o.PermissionMode = ModePlan
	case agentio.AutonomyAsk:
		o.PermissionMode = ModeManual
	case agentio.AutonomyAcceptEdits:
		o.PermissionMode = ModeAcceptEdits
	case agentio.AutonomyFull:
		o.DangerouslySkipPermissions = true
	default:
		return o, agentio.UnsupportedError(ID, "autonomy "+r.Autonomy.String(), "no matching Claude permission mode")
	}
	if r.Headless {
		o.Print, o.OutputFormat, o.Verbose = true, OutputStreamJSON, true
	}
	return o, nil
}

// launchRequest is the neutral Launch: Autonomy rules, token login env,
// then the rich builder.
func (a *Agent) launchRequest(t agentio.Target, r agentio.LaunchRequest) (agentio.Command, error) {
	if err := agentio.CheckAutonomy(t, r.Autonomy); err != nil {
		return agentio.Command{}, err
	}
	o, err := launchFromRequest(r)
	if err != nil {
		return agentio.Command{}, err
	}
	var tokenEnv []agentio.EnvChange
	if r.Credential != nil {
		if tokenEnv, err = a.TokenEnv(*r.Credential); err != nil {
			return agentio.Command{}, err
		}
	}
	cmd, err := a.launch(t, o, tokenEnv)
	if err != nil {
		return agentio.Command{}, err
	}
	cmd.Dir = r.Dir
	return cmd, nil
}

// EncodeAgentsFlag renders the --agents JSON {"name": {"description", "prompt", …}}.
// cost: Pure.
func EncodeAgentsFlag(defs map[string]SubagentDef) (string, error) {
	type wire struct {
		Description string   `json:"description"`
		Prompt      string   `json:"prompt"`
		Tools       []string `json:"tools,omitempty"`
		Model       string   `json:"model,omitempty"`
	}
	m := make(map[string]wire, len(defs))
	for k, d := range defs {
		m[k] = wire{Description: d.Description, Prompt: d.Prompt, Tools: d.Tools, Model: d.Model}
	}
	b, err := json.Marshal(m)
	return string(b), err
}

// command builds a data-only Command for h (home rule applied).
func (a *Agent) command(h Home, args ...string) agentio.Command {
	cmd, err := a.core.Command(h.Target(), "", args, nil)
	if err != nil {
		return agentio.NewCommand(Binary, args, a.host.Env, "")
	}
	return cmd
}

// UpdateCommand is `claude update` for the default home with the claude
// binary resolved on Host PATH (a walk, never a shell; the same shape as
// codex.Agent.UpdateCommand). ErrNotInstalled when PATH has no claude.
// Never run by the library. cost: Disk · Read (PATH walk).
func (a *Agent) UpdateCommand() (agentio.Command, error) {
	path, err := a.LookPath()
	if err != nil {
		return agentio.Command{}, err
	}
	cmd := a.command(a.DefaultHome(), subUpdate)
	cmd.Path = path
	return cmd, nil
}

// InstallCommand is `claude install <target>` (stable, latest or a version). cost: Pure.
func (a *Agent) InstallCommand(target string) agentio.Command {
	return a.command(a.DefaultHome(), subInstall, target)
}

// LoginMode picks the login flow of `claude auth login`.
type LoginMode string

// Login modes (help).
const (
	LoginClaudeAI LoginMode = "claudeai"
	LoginConsole  LoginMode = "console"
	LoginSSO      LoginMode = "sso"
)

// LoginCommand is `claude auth login --claudeai|--console|--sso [--email e]`
// for h, with inherited credential variables scrubbed (as for every launch)
// so the new login is the one used. cost: Pure.
func (a *Agent) LoginCommand(h Home, mode LoginMode, email string) agentio.Command {
	args := []string{subAuth, subLogin}
	switch mode {
	case LoginConsole:
		args = append(args, flagConsole)
	case LoginSSO:
		args = append(args, flagSSO)
	default:
		args = append(args, flagClaudeAI)
	}
	if email != "" {
		args = append(args, flagEmail, email)
	}
	cmd, err := a.core.Command(h.Target(), "", args, nil)
	if err != nil {
		return a.command(h, args...)
	}
	return cmd
}

// LogoutCommand is `claude auth logout` for h. cost: Pure.
func (a *Agent) LogoutCommand(h Home) agentio.Command { return a.command(h, subAuth, subLogout) }

// SetupTokenCommand is `claude setup-token` for h. cost: Pure.
func (a *Agent) SetupTokenCommand(h Home) agentio.Command { return a.command(h, subSetupToken) }

// MCPListCommand is `claude mcp list` (data). Running it spawns every
// server for a health check, so the library never runs it. cost: Pure.
func (a *Agent) MCPListCommand(h Home, dir string) agentio.Command {
	c := a.command(h, subMCP, subList)
	c.Dir = dir
	return c
}

// MCPAddCommand is `claude mcp add-json -s <scope> <name> <json>`: a
// vendor command, never a file edit, because Claude Code owns the file. The CLI takes the server JSON only on
// argv, so a server with env values or headers is refused: those are secrets
// (EnvValue) and must never ride in argv (invariant 8). Add such a server
// without them and set the values through Claude Code itself. cost: Pure.
func (a *Agent) MCPAddCommand(h Home, dir string, s MCPServer) (agentio.Command, error) {
	if len(s.Env) > 0 || len(s.Headers) > 0 {
		return agentio.Command{}, agentio.UnsupportedError(ID, "mcp add-json with env or headers",
			"claude mcp add-json takes the server only on argv; env values and headers are secrets and never go on argv")
	}
	type wire struct {
		Type    MCPTransport `json:"type,omitempty"`
		Command string       `json:"command,omitempty"`
		Args    []string     `json:"args,omitempty"`
		URL     string       `json:"url,omitempty"`
	}
	w := wire{Type: s.Transport, Command: s.Command, Args: s.Args, URL: s.URL}
	js, err := json.Marshal(w)
	if err != nil {
		return agentio.Command{}, err
	}
	scope := s.Scope
	if scope == "" {
		scope = MCPScopeLocal
	}
	c := a.command(h, subMCP, subAddJSON, flagScope, string(scope), s.Name, string(js))
	c.Dir = dir
	return c, nil
}

// MCPRemoveCommand is `claude mcp remove -s <scope> <name>`. cost: Pure.
func (a *Agent) MCPRemoveCommand(h Home, dir string, scope MCPScope, name string) agentio.Command {
	c := a.command(h, subMCP, subRemove, flagScope, string(scope), name)
	c.Dir = dir
	return c
}

// MCPResetProjectChoicesCommand is `claude mcp reset-project-choices`. cost: Pure.
func (a *Agent) MCPResetProjectChoicesCommand(h Home, dir string) agentio.Command {
	c := a.command(h, subMCP, subResetProjectChoices)
	c.Dir = dir
	return c
}

// PluginScope is where a plugin is installed.
type PluginScope string

// Plugin scopes.
const (
	PluginScopeUser    PluginScope = "user"
	PluginScopeProject PluginScope = "project"
	PluginScopeLocal   PluginScope = "local"
	PluginScopeSession PluginScope = "session"
)

// PluginInstallCommand is `claude plugin install <id> -s <scope>`. cost: Pure.
func (a *Agent) PluginInstallCommand(h Home, id PluginID, scope PluginScope) agentio.Command {
	return a.command(h, subPlugin, subInstall, string(id), flagScope, string(scope))
}

// PluginEnableCommand is `claude plugin enable <id>`. cost: Pure.
func (a *Agent) PluginEnableCommand(h Home, id PluginID) agentio.Command {
	return a.command(h, subPlugin, subEnable, string(id))
}

// PluginDisableCommand is `claude plugin disable <id>`. cost: Pure.
func (a *Agent) PluginDisableCommand(h Home, id PluginID) agentio.Command {
	return a.command(h, subPlugin, subDisable, string(id))
}

// MarketplaceAddCommand is `claude plugin marketplace add <source>`. cost: Pure.
func (a *Agent) MarketplaceAddCommand(h Home, source string) agentio.Command {
	return a.command(h, subPlugin, subMarketplace, subAdd, source)
}

// MarketplaceUpdateCommand is `claude plugin marketplace update [name]`. cost: Pure.
func (a *Agent) MarketplaceUpdateCommand(h Home, name string) agentio.Command {
	args := []string{subPlugin, subMarketplace, subUpdate}
	if name != "" {
		args = append(args, name)
	}
	return a.command(h, args...)
}

// PurgeCommand is `claude purge <dir> [--dry-run]`. cost: Pure.
func (a *Agent) PurgeCommand(dir string, dryRun bool) agentio.Command {
	args := []string{subPurge, dir}
	if dryRun {
		args = append(args, flagDryRun)
	}
	return a.command(a.DefaultHome(), args...)
}

// AttachCommand is `claude attach <id>`. cost: Pure.
func (a *Agent) AttachCommand(id string) agentio.Command {
	return a.command(a.DefaultHome(), subAttach, id)
}

// StopCommand is `claude stop <id>`. cost: Pure.
func (a *Agent) StopCommand(id string) agentio.Command {
	return a.command(a.DefaultHome(), subStop, id)
}

// LogsCommand is `claude logs <id>`. cost: Pure.
func (a *Agent) LogsCommand(id string) agentio.Command {
	return a.command(a.DefaultHome(), subLogs, id)
}
