package codex

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"time"

	"github.com/ubgo/agentio"
)

// Config is one config.toml layer, typed after upstream ConfigToml
// (config/src/config_toml.rs at rust-v0.162.1). Every field is
// presence-tracked: an absent key is unset, never its zero value (SPEC §5.8).
// Sections the library does not model yet are TOMLRaw sub-trees; any other
// key lands in Unknown with its source text.
type Config struct {
	// model
	Model                      Opt[string]
	ModelProvider              Opt[string] // key into ModelProviders or a built-in ("openai", "ollama", "lmstudio")
	ModelReasoningEffort       Opt[ReasoningEffort]
	PlanModeReasoningEffort    Opt[ReasoningEffort]
	ModelReasoningSummary      Opt[ReasoningSummary]
	ModelVerbosity             Opt[Verbosity]
	ModelContextWindow         Opt[int64]
	ModelAutoCompactTokenLimit Opt[int64]
	ModelCatalogJSON           Opt[string]
	ModelInstructionsFile      Opt[string]
	ReviewModel                Opt[string]
	ServiceTier                Opt[string] // free-form upstream: "default" | "priority" | "flex" | legacy "fast"
	Personality                Opt[Personality]
	OSSProvider                Opt[string]
	OpenAIBaseURL              Opt[string]
	ChatGPTBaseURL             Opt[string]

	// safety
	ApprovalPolicy Opt[ApprovalPolicy]
	// ApprovalGranular is the `approval_policy = { granular = {...} }`
	// table, kept raw (no sample seen).
	ApprovalGranular       TOMLRaw
	ApprovalsReviewer      Opt[ApprovalsReviewer]
	SandboxMode            Opt[SandboxMode]
	SandboxWorkspaceWrite  *SandboxWorkspaceWrite
	DefaultPermissions     Opt[string]        // permissions profile name; ":" prefix = built-in
	Permissions            map[string]TOMLRaw // [permissions.<name>] kept raw (network/filesystem details untyped)
	ShellEnvironmentPolicy *ShellEnvironmentPolicy
	AllowLoginShell        Opt[bool]

	// login
	ForcedLoginMethod         Opt[LoginMethod]
	ForcedChatGPTWorkspaceIDs []string // string or array upstream; normalised to a list
	CLIAuthCredentialsStore   Opt[CredentialStore]
	MCPOAuthCredentialsStore  Opt[OAuthStore]

	// tables
	ModelProviders map[string]ModelProvider
	Profiles       map[string]LegacyProfile // [profiles.*] (legacy, read-only: Codex keeps them only for old configs)
	Profile        Opt[string]              // root `profile = "…"` (legacy selector)
	MCPServers     map[string]MCPServer
	Projects       map[string]Project     // key = absolute folder
	Plugins        map[string]PluginEntry // key "<plugin>@<marketplace>"
	Marketplaces   map[string]MarketplaceEntry
	Skills         *SkillsConfig
	Features       map[string]FeatureSetting // bool or {enabled = …, …}
	Hooks          *Hooks

	// instructions
	Instructions                Opt[string]
	DeveloperInstructions       Opt[string]
	CompactPrompt               Opt[string]
	ProjectDocMaxBytes          Opt[int64]
	ProjectDocFallbackFilenames []string
	ProjectRootMarkers          []string // default [".git"]; an empty list disables the parent walk
	// ProjectRootMarkersSet tells "absent" (default .git) from "set to []".
	ProjectRootMarkersSet bool

	// misc
	Notify                  []string // argv of a notifier; the JSON payload is appended as the last arg
	History                 *HistoryConfig
	Memories                TOMLRaw // [memories] kept raw
	Agents                  *AgentsConfig
	Goals                   *GoalsConfig
	WebSearch               Opt[WebSearchMode]
	FileOpener              Opt[FileOpener]
	HideAgentReasoning      Opt[bool]
	ShowRawAgentReasoning   Opt[bool]
	CheckForUpdateOnStartup Opt[bool]
	Analytics               Opt[bool] // [analytics] enabled
	Feedback                Opt[bool] // [feedback] enabled
	SQLiteHome              Opt[string]
	LogDir                  Opt[string]
	TUI                     TOMLRaw // [tui] kept raw (keymaps, notifications, model_availability_nux, …)
	Desktop                 TOMLRaw // opaque by upstream design

	// Unknown holds every other key with its source text.
	Unknown UnknownTOML

	// counts is filled by the decoder; read through KeyCount.
	counts KeyCounts
}

// KeyCount reports how many leaf keys the decoded file(s) held and how
// they were placed (typed, kept raw, unknown). Nested tables and
// array-of-tables elements count per leaf. Pure.
func (c Config) KeyCount() KeyCounts { return c.counts }

// SandboxWorkspaceWrite is [sandbox_workspace_write].
type SandboxWorkspaceWrite struct {
	WritableRoots       []string
	NetworkAccess       Opt[bool]
	ExcludeTmpdirEnvVar Opt[bool]
	ExcludeSlashTmp     Opt[bool]
	Unknown             UnknownTOML
}

// ShellEnvironmentPolicy is [shell_environment_policy]. `set` values may be
// secrets, so only their names are kept.
type ShellEnvironmentPolicy struct {
	Inherit                Opt[EnvInherit]
	IgnoreDefaultExcludes  Opt[bool]
	ExperimentalUseProfile Opt[bool]
	Exclude                []string
	IncludeOnly            []string
	SetKeys                []string // names only
	Unknown                UnknownTOML
}

// ModelProvider is [model_providers.<id>]. Secret-bearing keys
// (experimental_bearer_token) are reported by presence only.
type ModelProvider struct {
	Name, BaseURL, EnvKey, EnvKeyInstructions, ModelCatalogURL Opt[string]
	WireAPI                                                    Opt[WireAPI]
	// QueryParams and HTTPHeaders are RedactedString upstream: names only,
	// values left empty. EnvHTTPHeaders maps header → env var name.
	QueryParams, HTTPHeaders, EnvHTTPHeaders   map[string]string
	RequestMaxRetries, StreamMaxRetries        Opt[int64]
	StreamIdleTimeout, WebsocketConnectTimeout Opt[time.Duration] // *_ms keys
	RequiresOpenAIAuth, SupportsWebsockets     Opt[bool]
	SupportsStandaloneWebSearch                Opt[bool]
	HasExperimentalBearerToken                 bool // value never loaded
	Auth, AWS, GatewayOAuth, Capabilities      TOMLRaw
	Unknown                                    UnknownTOML
}

// MCPServer is [mcp_servers.<name>] (config/src/mcp_types.rs). Env values
// are never loaded into the struct: EnvKeys lists names, and the value is
// reachable only through Agent.MCPServerEnvValue as a redacting EnvValue.
type MCPServer struct {
	// stdio
	Command Opt[string]
	Args    []string
	Cwd     Opt[string]
	EnvKeys []string    // names only
	EnvVars []MCPEnvVar // env_vars: names forwarded from the parent env

	// http
	URL               Opt[string]
	BearerTokenEnvVar Opt[string]
	HasBearerToken    bool              // inline bearer_token present; value never loaded
	HTTPHeaderNames   []string          // http_headers names only
	EnvHTTPHeaders    map[string]string // header → env var name
	Auth              Opt[MCPAuth]
	OAuthResource     Opt[string]
	Scopes            []string
	OAuth             TOMLRaw // [mcp_servers.x.oauth] kept raw (client_secret never typed)

	// control
	Enabled, Required, SupportsParallelToolCalls Opt[bool]
	StartupTimeout, ToolTimeout                  Opt[time.Duration] // startup_timeout_sec|_ms, tool_timeout_sec
	EnabledTools, DisabledTools                  []string
	DefaultToolsApprovalMode                     Opt[ToolApproval]
	StartupReadiness                             Opt[MCPReadiness]
	Tools                                        TOMLRaw
	Unknown                                      UnknownTOML
}

// MCPEnvVar is one env_vars entry: a bare name or {name, source}.
type MCPEnvVar struct {
	Name   string
	Source string
}

// MCPTransport values of a Codex server (no sse/ws in Codex config).
const (
	transportStdio = "stdio"
	transportHTTP  = "http"
)

// Transport is "http" when a URL is set, else "stdio". Pure.
func (s MCPServer) Transport() agentio.MCPTransport {
	if s.URL.Set {
		return agentio.MCPTransport(transportHTTP)
	}
	return agentio.MCPTransport(transportStdio)
}

// Project is [projects."<dir>"].
type Project struct {
	TrustLevel Opt[TrustLevel]
	Unknown    UnknownTOML
}

// PluginEntry is [plugins."<plugin>@<marketplace>"].
type PluginEntry struct {
	Enabled    Opt[bool]
	MCPServers TOMLRaw // per-plugin MCP server toggles, kept raw
	Unknown    UnknownTOML
}

// MarketplaceEntry is [marketplaces.<name>] (types.rs MarketplaceConfig).
type MarketplaceEntry struct {
	LastUpdated, LastRevision, SourceType, Source, Ref Opt[string]
	SparsePaths                                        []string
	Unknown                                            UnknownTOML
}

// SkillsConfig is [skills].
type SkillsConfig struct {
	BundledEnabled      Opt[bool]
	IncludeInstructions Opt[bool]
	MaxContextTokens    Opt[int64]
	Config              []SkillConfigEntry
	Unknown             UnknownTOML
}

// SkillConfigEntry is one `[[skills.config]]` row.
type SkillConfigEntry struct {
	Path, Name Opt[string]
	Enabled    Opt[bool]
}

// FeatureSetting is one [features] entry: `name = bool` or a table whose
// `enabled` key is the switch; other keys stay raw in Extra.
type FeatureSetting struct {
	Enabled bool
	Extra   TOMLRaw
}

// Hooks is [hooks] in config.toml (config/src/hook_config.rs HooksToml):
// event matcher groups plus the hooks.state table.
type Hooks struct {
	Events map[HookEvent][]HookMatcherGroup
	State  map[string]HookState // hooks.state "<file>:<event_snake>:<group>:<handler>"
}

// HookState is one hooks.state entry (HookStateToml): Codex records the
// hash of a hook the user trusted, and can disable one.
type HookState struct {
	Enabled     Opt[bool]
	TrustedHash Opt[string]
}

// HookMatcherGroup is one `[[hooks.<Event>]]` element.
type HookMatcherGroup struct {
	Matcher Opt[string]
	Hooks   []HookHandler
}

// HookHandler is one `[[hooks.<Event>.hooks]]` element (HookHandlerConfig,
// tagged on `type`). Fields not used by a Type stay unset.
type HookHandler struct {
	Type HookHandlerType
	// command
	Command                Opt[string]
	CommandWindows         Opt[string]        // commandWindows (alias command_windows)
	Timeout                Opt[time.Duration] // `timeout`, seconds
	Async                  Opt[bool]
	StatusMessage          Opt[string]
	AdditionalContextLimit Opt[int64] // tokens; 0 disables spilling
	// mcp_tool
	Server, Tool Opt[string]
	Input        TOMLRaw     // mcp_tool input table (config.toml)
	InputJSON    agentio.Raw // mcp_tool input object (hooks.json)
	// Unknown keeps unmodelled keys: config.toml source text here,
	// hooks.json raw values in UnknownJSON.
	Unknown     UnknownTOML
	UnknownJSON agentio.Unknown
}

// HookHandlerType is a hook handler's `type`.
type HookHandlerType string

// HookHandlerType values (hook_config.rs at rust-v0.162.1).
const (
	HookTypeCommand HookHandlerType = "command"
	HookTypeMCPTool HookHandlerType = "mcp_tool"
	HookTypePrompt  HookHandlerType = "prompt"
	HookTypeAgent   HookHandlerType = "agent"
)

// IsKnown reports a known handler type. Pure.
func (t HookHandlerType) IsKnown() bool {
	return t == HookTypeCommand || t == HookTypeMCPTool || t == HookTypePrompt || t == HookTypeAgent
}

// HistoryConfig is [history].
type HistoryConfig struct {
	Persistence Opt[HistoryPersistence]
	MaxBytes    Opt[int64]
}

// AgentsConfig is [agents] (subagents).
type AgentsConfig struct {
	Enabled                        Opt[bool]
	MaxConcurrentThreadsPerSession Opt[int64]
	MaxDepth                       Opt[int64]
	DefaultSubagentModel           Opt[string]
	DefaultSubagentReasoningEffort Opt[ReasoningEffort]
	JobMaxRuntime                  Opt[time.Duration] // job_max_runtime_seconds
	InterruptMessage               Opt[bool]
	Unknown                        UnknownTOML
}

// GoalsConfig is [goals].
type GoalsConfig struct {
	MaxGoalTokenBudget Opt[int64]
	Unknown            UnknownTOML
}

// LegacyProfile is one legacy [profiles.<name>] table (profile_toml.rs).
// The keys a launch depends on are typed; the rest stay in Unknown. Two keys
// upstream only allows at the root were seen inside a profile on a real Mac
// (openai_base_url, forced_login_method); they are typed here too
// (OpenAIBaseURL, ForcedLoginMethod) and never an error.
type LegacyProfile struct {
	Model                   Opt[string]
	ServiceTier             Opt[string]
	ModelProvider           Opt[string]
	ApprovalPolicy          Opt[ApprovalPolicy]
	ApprovalsReviewer       Opt[ApprovalsReviewer]
	SandboxMode             Opt[SandboxMode]
	ModelReasoningEffort    Opt[ReasoningEffort]
	PlanModeReasoningEffort Opt[ReasoningEffort]
	ModelReasoningSummary   Opt[ReasoningSummary]
	ModelVerbosity          Opt[Verbosity]
	ModelCatalogJSON        Opt[string]
	Personality             Opt[Personality]
	ChatGPTBaseURL          Opt[string]
	ModelInstructionsFile   Opt[string]
	WebSearch               Opt[WebSearchMode]
	OSSProvider             Opt[string]
	// OpenAIBaseURL and ForcedLoginMethod are root-only upstream (a strict
	// ConfigProfile rejects them) but appear in real legacy profiles; typed
	// so callers see them, never an error.
	OpenAIBaseURL     Opt[string]
	ForcedLoginMethod Opt[LoginMethod]
	Features          map[string]FeatureSetting
	Unknown           UnknownTOML
}

// ParseConfig decodes config.toml bytes. file names the source in
// warnings. A syntax error fails the file; a key whose type changed only
// degrades that key (kept in Unknown, Warning added). Pure.
func ParseConfig(src []byte, file string) (Config, agentio.Warnings, error) {
	entries, err := parseTOML(src)
	if err != nil {
		return Config{}, nil, err
	}
	c, warns := decodeEntriesInto(entries, file)
	return c, warns, nil
}

// decodeEntriesInto decodes flattened entries into a Config and tallies
// KeyCount (Typed = Total − Raw − Unknown).
func decodeEntriesInto(entries []tomlEntry, file string) (Config, agentio.Warnings) {
	d := newTOMLDecoder(file)
	var c Config
	for _, e := range entries {
		d.scope(&c.Unknown, 0)
		decodeConfigEntry(d, &c, e)
	}
	d.counts.Total = len(entries)
	d.counts.Typed = d.counts.Total - d.counts.Raw - d.counts.Unknown
	c.counts = d.counts
	return c, d.warns
}

// ConfigFile reads <home>/config.toml only (no layers). An absent file is
// an empty Config. Disk · Read.
func (a *Agent) ConfigFile(h Home) (Config, error) {
	c, _, err := a.ConfigFileWarnings(h)
	return c, err
}

// ConfigFileWarnings is ConfigFile plus the drift warnings of the decode.
// Disk · Read.
func (a *Agent) ConfigFileWarnings(h Home) (Config, agentio.Warnings, error) {
	return readConfigPath(a.Paths(h).Config)
}

func readConfigPath(path string) (Config, agentio.Warnings, error) {
	b, err := readFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil, nil
	}
	if err != nil {
		return Config{}, nil, fmt.Errorf("codex: read %s: %w", path, err)
	}
	return ParseConfig(b, path)
}

// Config keys (verbatim upstream spelling).
const (
	keyModelProviders = "model_providers"
	keyProfiles       = "profiles"
	keyMCPServers     = "mcp_servers"
	keyProjects       = "projects"
	keyPlugins        = "plugins"
	keyMarketplaces   = "marketplaces"
	keyFeatures       = "features"
	keyHooks          = "hooks"
	keyHooksState     = "state"
	keyPermissions    = "permissions"
	keyEnabled        = "enabled"
	keyGranular       = "granular"
	keyApprovalPolicy = "approval_policy"
)

func decodeConfigEntry(d *tomlDecoder, c *Config, e tomlEntry) {
	p := e.path
	if len(p) == 1 && decodeProfileScalar(d, profileFields{
		model: &c.Model, serviceTier: &c.ServiceTier, modelProvider: &c.ModelProvider,
		approval: &c.ApprovalPolicy, reviewer: &c.ApprovalsReviewer, sandbox: &c.SandboxMode,
		effort: &c.ModelReasoningEffort, planEffort: &c.PlanModeReasoningEffort,
		summary: &c.ModelReasoningSummary, verbosity: &c.ModelVerbosity, catalog: &c.ModelCatalogJSON,
		personality: &c.Personality, chatgptBase: &c.ChatGPTBaseURL, instructionsFile: &c.ModelInstructionsFile,
		webSearch: &c.WebSearch, ossProvider: &c.OSSProvider,
	}, e) {
		return
	}
	switch p[0] {
	case keyApprovalPolicy:
		// table form: approval_policy = { granular = {...} }
		if len(p) >= 2 && p[1] == keyGranular {
			c.ApprovalPolicy = agentio.Some(ApprovalGranular)
			d.raw(&c.ApprovalGranular, e, 2)
			return
		}
	case keyModelProviders:
		if len(p) >= 3 {
			if c.ModelProviders == nil {
				c.ModelProviders = map[string]ModelProvider{}
			}
			mp := c.ModelProviders[p[1]]
			d.scope(&mp.Unknown, 2)
			decodeModelProvider(d, &mp, e)
			c.ModelProviders[p[1]] = mp
			return
		}
	case keyProfiles:
		if len(p) >= 3 {
			if c.Profiles == nil {
				c.Profiles = map[string]LegacyProfile{}
			}
			lp := c.Profiles[p[1]]
			d.scope(&lp.Unknown, 2)
			decodeLegacyProfile(d, &lp, e)
			c.Profiles[p[1]] = lp
			return
		}
	case keyMCPServers:
		if len(p) >= 3 {
			if c.MCPServers == nil {
				c.MCPServers = map[string]MCPServer{}
			}
			s := c.MCPServers[p[1]]
			d.scope(&s.Unknown, 2)
			decodeMCPServer(d, &s, e)
			c.MCPServers[p[1]] = s
			return
		}
	case keyProjects:
		if len(p) >= 3 {
			if c.Projects == nil {
				c.Projects = map[string]Project{}
			}
			pr := c.Projects[p[1]]
			d.scope(&pr.Unknown, 2)
			if len(p) == 3 && p[2] == "trust_level" {
				enumOpt(d, &pr.TrustLevel, e)
			} else {
				d.keep(e)
			}
			c.Projects[p[1]] = pr
			return
		}
	case keyPlugins:
		if len(p) >= 3 {
			if c.Plugins == nil {
				c.Plugins = map[string]PluginEntry{}
			}
			pe := c.Plugins[p[1]]
			d.scope(&pe.Unknown, 2)
			switch {
			case len(p) == 3 && p[2] == keyEnabled:
				d.boolean(&pe.Enabled, e)
			case p[2] == keyMCPServers:
				d.raw(&pe.MCPServers, e, 3)
			default:
				d.keep(e)
			}
			c.Plugins[p[1]] = pe
			return
		}
	case keyMarketplaces:
		if len(p) >= 3 {
			if c.Marketplaces == nil {
				c.Marketplaces = map[string]MarketplaceEntry{}
			}
			m := c.Marketplaces[p[1]]
			d.scope(&m.Unknown, 2)
			decodeMarketplace(d, &m, e)
			c.Marketplaces[p[1]] = m
			return
		}
	case keyFeatures:
		if len(p) >= 2 {
			decodeFeature(d, &c.Features, e)
			return
		}
	case keyHooks:
		if len(p) >= 2 {
			if c.Hooks == nil {
				c.Hooks = &Hooks{}
			}
			decodeHooks(d, c.Hooks, e)
			return
		}
	case keyPermissions:
		if len(p) >= 3 {
			if c.Permissions == nil {
				c.Permissions = map[string]TOMLRaw{}
			}
			r := c.Permissions[p[1]]
			d.raw(&r, e, 2)
			c.Permissions[p[1]] = r
			return
		}
	case "sandbox_workspace_write":
		if len(p) == 2 {
			if c.SandboxWorkspaceWrite == nil {
				c.SandboxWorkspaceWrite = &SandboxWorkspaceWrite{}
			}
			w := c.SandboxWorkspaceWrite
			d.scope(&w.Unknown, 1)
			switch p[1] {
			case "writable_roots":
				d.strList(&w.WritableRoots, e, false)
			case "network_access":
				d.boolean(&w.NetworkAccess, e)
			case "exclude_tmpdir_env_var":
				d.boolean(&w.ExcludeTmpdirEnvVar, e)
			case "exclude_slash_tmp":
				d.boolean(&w.ExcludeSlashTmp, e)
			default:
				d.keep(e)
			}
			return
		}
	case "shell_environment_policy":
		if len(p) >= 2 {
			if c.ShellEnvironmentPolicy == nil {
				c.ShellEnvironmentPolicy = &ShellEnvironmentPolicy{}
			}
			s := c.ShellEnvironmentPolicy
			d.scope(&s.Unknown, 1)
			switch {
			case p[1] == "set" && len(p) == 3:
				addName(&s.SetKeys, p[2])
			case len(p) != 2:
				d.keep(e)
			case p[1] == "inherit":
				enumOpt(d, &s.Inherit, e)
			case p[1] == "ignore_default_excludes":
				d.boolean(&s.IgnoreDefaultExcludes, e)
			case p[1] == "experimental_use_profile":
				d.boolean(&s.ExperimentalUseProfile, e)
			case p[1] == "exclude":
				d.strList(&s.Exclude, e, false)
			case p[1] == "include_only":
				d.strList(&s.IncludeOnly, e, false)
			default:
				d.keep(e)
			}
			return
		}
	case "skills":
		if len(p) >= 2 {
			if c.Skills == nil {
				c.Skills = &SkillsConfig{}
			}
			decodeSkillsConfig(d, c.Skills, e)
			return
		}
	case "history":
		if len(p) == 2 {
			if c.History == nil {
				c.History = &HistoryConfig{}
			}
			switch p[1] {
			case "persistence":
				enumOpt(d, &c.History.Persistence, e)
			case "max_bytes":
				d.integer(&c.History.MaxBytes, e)
			default:
				d.keep(e)
			}
			return
		}
	case "agents":
		if len(p) == 2 {
			if c.Agents == nil {
				c.Agents = &AgentsConfig{}
			}
			decodeAgents(d, c.Agents, e)
			return
		}
	case "goals":
		if len(p) == 2 {
			if c.Goals == nil {
				c.Goals = &GoalsConfig{}
			}
			d.scope(&c.Goals.Unknown, 1)
			if p[1] == "max_goal_token_budget" {
				d.integer(&c.Goals.MaxGoalTokenBudget, e)
			} else {
				d.keep(e)
			}
			return
		}
	case "analytics", "feedback":
		if len(p) == 2 && p[1] == keyEnabled {
			if p[0] == "analytics" {
				d.boolean(&c.Analytics, e)
			} else {
				d.boolean(&c.Feedback, e)
			}
			return
		}
	case "memories":
		if len(p) >= 2 {
			d.raw(&c.Memories, e, 1)
			return
		}
	case "tui":
		d.raw(&c.TUI, e, 1)
		return
	case "desktop":
		d.raw(&c.Desktop, e, 1)
		return
	}
	if len(p) != 1 {
		d.keep(e)
		return
	}
	switch p[0] {
	case "model_context_window":
		d.integer(&c.ModelContextWindow, e)
	case "model_auto_compact_token_limit":
		d.integer(&c.ModelAutoCompactTokenLimit, e)
	case "review_model":
		d.str(&c.ReviewModel, e)
	case "openai_base_url":
		d.str(&c.OpenAIBaseURL, e)
	case "default_permissions":
		d.str(&c.DefaultPermissions, e)
	case "allow_login_shell":
		d.boolean(&c.AllowLoginShell, e)
	case "forced_login_method":
		enumOpt(d, &c.ForcedLoginMethod, e)
	case "forced_chatgpt_workspace_id":
		d.strList(&c.ForcedChatGPTWorkspaceIDs, e, true)
	case "cli_auth_credentials_store":
		enumOpt(d, &c.CLIAuthCredentialsStore, e)
	case "mcp_oauth_credentials_store":
		enumOpt(d, &c.MCPOAuthCredentialsStore, e)
	case "profile":
		d.str(&c.Profile, e)
	case "instructions":
		d.str(&c.Instructions, e)
	case "developer_instructions":
		d.str(&c.DeveloperInstructions, e)
	case "compact_prompt":
		d.str(&c.CompactPrompt, e)
	case "project_doc_max_bytes":
		d.integer(&c.ProjectDocMaxBytes, e)
	case "project_doc_fallback_filenames":
		d.strList(&c.ProjectDocFallbackFilenames, e, false)
	case "project_root_markers":
		d.strList(&c.ProjectRootMarkers, e, false)
		c.ProjectRootMarkersSet = e.val.kind == tomlArray
	case "notify":
		d.strList(&c.Notify, e, false)
	case "file_opener":
		enumOpt(d, &c.FileOpener, e)
	case "hide_agent_reasoning":
		d.boolean(&c.HideAgentReasoning, e)
	case "show_raw_agent_reasoning":
		d.boolean(&c.ShowRawAgentReasoning, e)
	case "check_for_update_on_startup":
		d.boolean(&c.CheckForUpdateOnStartup, e)
	case "sqlite_home":
		d.str(&c.SQLiteHome, e)
	case "log_dir":
		d.str(&c.LogDir, e)
	default:
		d.keep(e)
	}
}

// profileFields points at the keys shared by the root and a legacy profile.
type profileFields struct {
	model, serviceTier, modelProvider, catalog, chatgptBase, instructionsFile, ossProvider *Opt[string]
	approval                                                                               *Opt[ApprovalPolicy]
	reviewer                                                                               *Opt[ApprovalsReviewer]
	sandbox                                                                                *Opt[SandboxMode]
	effort, planEffort                                                                     *Opt[ReasoningEffort]
	summary                                                                                *Opt[ReasoningSummary]
	verbosity                                                                              *Opt[Verbosity]
	personality                                                                            *Opt[Personality]
	webSearch                                                                              *Opt[WebSearchMode]
}

// decodeProfileScalar decodes a key shared by root and profile; false when
// the key is not one of them.
func decodeProfileScalar(d *tomlDecoder, f profileFields, e tomlEntry) bool {
	switch e.path[len(e.path)-1] {
	case "model":
		d.str(f.model, e)
	case "service_tier":
		d.str(f.serviceTier, e)
	case "model_provider":
		d.str(f.modelProvider, e)
	case keyApprovalPolicy:
		enumOpt(d, f.approval, e)
	case "approvals_reviewer":
		enumOpt(d, f.reviewer, e)
	case "sandbox_mode":
		enumOpt(d, f.sandbox, e)
	case "model_reasoning_effort":
		enumOpt(d, f.effort, e)
	case "plan_mode_reasoning_effort":
		enumOpt(d, f.planEffort, e)
	case "model_reasoning_summary":
		enumOpt(d, f.summary, e)
	case "model_verbosity":
		enumOpt(d, f.verbosity, e)
	case "model_catalog_json":
		d.str(f.catalog, e)
	case "personality":
		enumOpt(d, f.personality, e)
	case "chatgpt_base_url":
		d.str(f.chatgptBase, e)
	case "model_instructions_file":
		d.str(f.instructionsFile, e)
	case "web_search":
		enumOpt(d, f.webSearch, e)
	case "oss_provider":
		d.str(f.ossProvider, e)
	default:
		return false
	}
	return true
}

func decodeLegacyProfile(d *tomlDecoder, lp *LegacyProfile, e tomlEntry) {
	r := rest(e, 2)
	if r[0] == keyFeatures && len(r) >= 2 {
		decodeFeatureAt(d, &lp.Features, e, 3)
		return
	}
	if len(r) == 1 && decodeProfileScalar(d, profileFields{
		model: &lp.Model, serviceTier: &lp.ServiceTier, modelProvider: &lp.ModelProvider,
		approval: &lp.ApprovalPolicy, reviewer: &lp.ApprovalsReviewer, sandbox: &lp.SandboxMode,
		effort: &lp.ModelReasoningEffort, planEffort: &lp.PlanModeReasoningEffort,
		summary: &lp.ModelReasoningSummary, verbosity: &lp.ModelVerbosity, catalog: &lp.ModelCatalogJSON,
		personality: &lp.Personality, chatgptBase: &lp.ChatGPTBaseURL, instructionsFile: &lp.ModelInstructionsFile,
		webSearch: &lp.WebSearch, ossProvider: &lp.OSSProvider,
	}, e) {
		return
	}
	switch {
	case len(r) == 1 && r[0] == "openai_base_url":
		d.str(&lp.OpenAIBaseURL, e)
	case len(r) == 1 && r[0] == "forced_login_method":
		enumOpt(d, &lp.ForcedLoginMethod, e)
	default:
		d.keep(e)
	}
}

func decodeModelProvider(d *tomlDecoder, mp *ModelProvider, e tomlEntry) {
	r := rest(e, 2)
	switch r[0] {
	case "query_params":
		if len(r) == 2 {
			if mp.QueryParams == nil {
				mp.QueryParams = map[string]string{}
			}
			mp.QueryParams[r[1]] = ""
			return
		}
	case "http_headers":
		if len(r) == 2 {
			// header values can be credentials: keep names only
			if mp.HTTPHeaders == nil {
				mp.HTTPHeaders = map[string]string{}
			}
			mp.HTTPHeaders[r[1]] = ""
			return
		}
	case "env_http_headers":
		if len(r) == 2 {
			d.strMap(&mp.EnvHTTPHeaders, r[1], e)
			return
		}
	case "auth":
		d.raw(&mp.Auth, e, 3)
		return
	case "aws":
		d.raw(&mp.AWS, e, 3)
		return
	case "gateway_oauth":
		d.raw(&mp.GatewayOAuth, e, 3)
		return
	case "capabilities":
		d.raw(&mp.Capabilities, e, 3)
		return
	}
	if len(r) != 1 {
		d.keep(e)
		return
	}
	switch r[0] {
	case "name":
		d.str(&mp.Name, e)
	case "base_url":
		d.str(&mp.BaseURL, e)
	case "env_key":
		d.str(&mp.EnvKey, e)
	case "env_key_instructions":
		d.str(&mp.EnvKeyInstructions, e)
	case "model_catalog_url":
		d.str(&mp.ModelCatalogURL, e)
	case "wire_api":
		enumOpt(d, &mp.WireAPI, e)
	case "request_max_retries":
		d.integer(&mp.RequestMaxRetries, e)
	case "stream_max_retries":
		d.integer(&mp.StreamMaxRetries, e)
	case "stream_idle_timeout_ms":
		d.duration(&mp.StreamIdleTimeout, e, time.Millisecond)
	case "websocket_connect_timeout_ms":
		d.duration(&mp.WebsocketConnectTimeout, e, time.Millisecond)
	case "requires_openai_auth":
		d.boolean(&mp.RequiresOpenAIAuth, e)
	case "supports_websockets":
		d.boolean(&mp.SupportsWebsockets, e)
	case "supports_standalone_web_search":
		d.boolean(&mp.SupportsStandaloneWebSearch, e)
	case "experimental_bearer_token":
		mp.HasExperimentalBearerToken = true
	default:
		d.keep(e)
	}
}

func decodeMCPServer(d *tomlDecoder, s *MCPServer, e tomlEntry) {
	r := rest(e, 2)
	switch r[0] {
	case "env":
		if len(r) == 2 {
			addName(&s.EnvKeys, r[1])
			return
		}
	case "http_headers":
		if len(r) == 2 {
			addName(&s.HTTPHeaderNames, r[1])
			return
		}
	case "env_http_headers":
		if len(r) == 2 {
			d.strMap(&s.EnvHTTPHeaders, r[1], e)
			return
		}
	case "env_vars":
		decodeEnvVars(d, s, e, r)
		return
	case "oauth":
		d.raw(&s.OAuth, e, 3)
		return
	case "tools":
		d.raw(&s.Tools, e, 3)
		return
	}
	if len(r) != 1 {
		d.keep(e)
		return
	}
	switch r[0] {
	case "command":
		d.str(&s.Command, e)
	case "args":
		d.strList(&s.Args, e, false)
	case "cwd":
		d.str(&s.Cwd, e)
	case "url":
		d.str(&s.URL, e)
	case "bearer_token_env_var":
		d.str(&s.BearerTokenEnvVar, e)
	case "bearer_token":
		s.HasBearerToken = true
	case "auth":
		enumOpt(d, &s.Auth, e)
	case "oauth_resource":
		d.str(&s.OAuthResource, e)
	case "scopes":
		d.strList(&s.Scopes, e, false)
	case keyEnabled:
		d.boolean(&s.Enabled, e)
	case "required":
		d.boolean(&s.Required, e)
	case "supports_parallel_tool_calls":
		d.boolean(&s.SupportsParallelToolCalls, e)
	case "startup_timeout_sec":
		d.duration(&s.StartupTimeout, e, time.Second)
	case "startup_timeout_ms":
		d.duration(&s.StartupTimeout, e, time.Millisecond)
	case "tool_timeout_sec":
		d.duration(&s.ToolTimeout, e, time.Second)
	case "enabled_tools":
		d.strList(&s.EnabledTools, e, false)
	case "disabled_tools":
		d.strList(&s.DisabledTools, e, false)
	case "default_tools_approval_mode":
		enumOpt(d, &s.DefaultToolsApprovalMode, e)
	case "startup_readiness":
		enumOpt(d, &s.StartupReadiness, e)
	default:
		d.keep(e)
	}
}

// decodeEnvVars handles `env_vars = ["A", {name = "B", source = "…"}]`:
// strings arrive as one array entry, tables as flattened env_vars.<i>.key.
func decodeEnvVars(d *tomlDecoder, s *MCPServer, e tomlEntry, r []string) {
	if len(r) == 1 {
		var names []string
		d.strList(&names, e, false)
		for _, n := range names {
			s.EnvVars = append(s.EnvVars, MCPEnvVar{Name: n})
		}
		return
	}
	if len(r) != 3 {
		d.keep(e)
		return
	}
	idx, err := strconv.Atoi(r[1])
	if err != nil || e.val.kind != tomlString {
		d.keep(e)
		return
	}
	for len(s.EnvVars) <= idx {
		s.EnvVars = append(s.EnvVars, MCPEnvVar{})
	}
	switch r[2] {
	case "name":
		s.EnvVars[idx].Name = e.val.str
	case "source":
		s.EnvVars[idx].Source = e.val.str
	default:
		d.keep(e)
	}
}

func decodeMarketplace(d *tomlDecoder, m *MarketplaceEntry, e tomlEntry) {
	r := rest(e, 2)
	if len(r) != 1 {
		d.keep(e)
		return
	}
	switch r[0] {
	case "last_updated":
		d.str(&m.LastUpdated, e)
	case "last_revision":
		d.str(&m.LastRevision, e)
	case "source_type":
		d.str(&m.SourceType, e)
	case "source":
		d.str(&m.Source, e)
	case "ref":
		d.str(&m.Ref, e)
	case "sparse_paths":
		d.strList(&m.SparsePaths, e, false)
	default:
		d.keep(e)
	}
}

func decodeFeature(d *tomlDecoder, dst *map[string]FeatureSetting, e tomlEntry) {
	decodeFeatureAt(d, dst, e, 2)
}

// decodeFeatureAt decodes features.<name>[.key] where the name is part n-1.
func decodeFeatureAt(d *tomlDecoder, dst *map[string]FeatureSetting, e tomlEntry, n int) {
	if *dst == nil {
		*dst = map[string]FeatureSetting{}
	}
	name := e.path[n-1]
	fs := (*dst)[name]
	r := rest(e, n)
	switch {
	case len(r) == 0 && e.val.kind == tomlBool:
		fs.Enabled = e.val.b
	case len(r) == 0:
		d.mismatch(e, tomlBool)
		return
	case len(r) == 1 && r[0] == keyEnabled && e.val.kind == tomlBool:
		fs.Enabled = e.val.b
	default:
		d.raw(&fs.Extra, e, n)
	}
	(*dst)[name] = fs
}

// decodeHooks handles hooks.<Event>.<i>.matcher,
// hooks.<Event>.<i>.hooks.<j>.<key> and hooks.state.<key>.<field>.
func decodeHooks(d *tomlDecoder, h *Hooks, e tomlEntry) {
	p := e.path
	if p[1] == keyHooksState {
		if len(p) != 4 {
			d.keep(e)
			return
		}
		if h.State == nil {
			h.State = map[string]HookState{}
		}
		t := h.State[p[2]]
		switch p[3] {
		case keyEnabled:
			d.boolean(&t.Enabled, e)
		case "trusted_hash":
			d.str(&t.TrustedHash, e)
		default:
			d.keep(e)
		}
		h.State[p[2]] = t
		return
	}
	if len(p) < 4 {
		d.keep(e)
		return
	}
	gi, err := strconv.Atoi(p[2])
	if err != nil {
		d.keep(e)
		return
	}
	if h.Events == nil {
		h.Events = map[HookEvent][]HookMatcherGroup{}
	}
	ev := HookEvent(p[1])
	groups := h.Events[ev]
	for len(groups) <= gi {
		groups = append(groups, HookMatcherGroup{})
	}
	g := &groups[gi]
	switch {
	case len(p) == 4 && p[3] == hookKeyMatcher:
		d.str(&g.Matcher, e)
	case len(p) >= 6 && p[3] == keyHooks:
		hi, err := strconv.Atoi(p[4])
		if err != nil {
			d.keep(e)
			break
		}
		for len(g.Hooks) <= hi {
			g.Hooks = append(g.Hooks, HookHandler{})
		}
		hh := &g.Hooks[hi]
		d.scope(&hh.Unknown, 5)
		if p[5] == hookKeyInput {
			d.raw(&hh.Input, e, 6)
			break
		}
		if len(p) != 6 {
			d.keep(e)
			break
		}
		switch p[5] {
		case hookKeyType:
			var t Opt[HookHandlerType]
			enumOpt(d, &t, e)
			hh.Type = t.V
		case hookKeyCommand:
			d.str(&hh.Command, e)
		case hookKeyCommandWindows, hookKeyCommandWindowsAlt:
			d.str(&hh.CommandWindows, e)
		case hookKeyTimeout:
			d.duration(&hh.Timeout, e, time.Second)
		case hookKeyStatusMessage:
			d.str(&hh.StatusMessage, e)
		case hookKeyAsync:
			d.boolean(&hh.Async, e)
		case hookKeyContextLimit:
			d.integer(&hh.AdditionalContextLimit, e)
		case hookKeyServer:
			d.str(&hh.Server, e)
		case hookKeyTool:
			d.str(&hh.Tool, e)
		default:
			d.keep(e)
		}
	default:
		d.keep(e)
	}
	h.Events[ev] = groups
}

func decodeSkillsConfig(d *tomlDecoder, s *SkillsConfig, e tomlEntry) {
	d.scope(&s.Unknown, 1)
	p := e.path
	switch {
	case len(p) == 3 && p[1] == "bundled" && p[2] == keyEnabled:
		d.boolean(&s.BundledEnabled, e)
	case len(p) == 2 && p[1] == "include_instructions":
		d.boolean(&s.IncludeInstructions, e)
	case len(p) == 2 && p[1] == "max_context_tokens":
		d.integer(&s.MaxContextTokens, e)
	case len(p) == 4 && p[1] == "config":
		i, err := strconv.Atoi(p[2])
		if err != nil {
			d.keep(e)
			return
		}
		for len(s.Config) <= i {
			s.Config = append(s.Config, SkillConfigEntry{})
		}
		sc := &s.Config[i]
		switch p[3] {
		case "path":
			d.str(&sc.Path, e)
		case "name":
			d.str(&sc.Name, e)
		case keyEnabled:
			d.boolean(&sc.Enabled, e)
		default:
			d.keep(e)
		}
	default:
		d.keep(e)
	}
}

func decodeAgents(d *tomlDecoder, a *AgentsConfig, e tomlEntry) {
	d.scope(&a.Unknown, 1)
	switch e.path[1] {
	case keyEnabled:
		d.boolean(&a.Enabled, e)
	case "max_concurrent_threads_per_session":
		d.integer(&a.MaxConcurrentThreadsPerSession, e)
	case "max_depth":
		d.integer(&a.MaxDepth, e)
	case "default_subagent_model":
		d.str(&a.DefaultSubagentModel, e)
	case "default_subagent_reasoning_effort":
		enumOpt(d, &a.DefaultSubagentReasoningEffort, e)
	case "job_max_runtime_seconds":
		d.duration(&a.JobMaxRuntime, e, time.Second)
	case "interrupt_message":
		d.boolean(&a.InterruptMessage, e)
	default:
		d.keep(e)
	}
}

// LegacyProfileNames lists [profiles.*] keys plus the root `profile`
// selector, sorted, deduplicated (read-only view, used to refuse a -p
// that collides with one). Pure.
func (c Config) LegacyProfileNames() []string {
	var out []string
	for name := range c.Profiles {
		out = append(out, name)
	}
	if c.Profile.Set && c.Profile.V != "" {
		addName(&out, c.Profile.V)
	}
	sort.Strings(out)
	return out
}

// FeatureOverrides is the [features] table as name → enabled. Pure.
func (c Config) FeatureOverrides() map[string]bool {
	if c.Features == nil {
		return nil
	}
	out := make(map[string]bool, len(c.Features))
	for k, v := range c.Features {
		out[k] = v.Enabled
	}
	return out
}

// ErrProfileConflict matches the loader/mod.rs:303 refusal: `--profile X`
// while config.toml also names X as a legacy profile.
var ErrProfileConflict = errors.New("codex: profile name is also a legacy [profiles] entry")

// ProfileConflict returns ErrProfileConflict (wrapped with the name) when
// name is used by a legacy [profiles.<name>] table or the root `profile`
// selector, which Codex 0.162.1 refuses alongside `--profile`. Pure.
func (c Config) ProfileConflict(name string) error {
	if name == "" {
		return nil
	}
	if _, ok := c.Profiles[name]; ok || (c.Profile.Set && c.Profile.V == name) {
		return fmt.Errorf("%w: %q (rename or remove the legacy profile, or use a different v2 profile name)", ErrProfileConflict, name)
	}
	return nil
}
