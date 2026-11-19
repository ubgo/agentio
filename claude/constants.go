// Package claude is the typed API of Claude Code: its install, homes, paths,
// settings, global state, MCP servers, memory files, skills, subagents,
// plugins, hooks, identity, usage, conversations, prompt history, live
// sessions, local stats, the launch builder and the stream decoder.
//
// Everything reads through an agentio.Host, so a test can point the agent at
// a fixture home. Nothing here refreshes, writes or prints a login: refreshing
// rotates the refresh token and logs the owning Claude Code install out.
// Reads that run the claude binary sit behind Runner, quota calls behind
// Prober and disk writes behind Writer (SPEC §4).
package claude

import "time"

// Rule: no bare strings for any on-disk name, keychain service, env var or
// wire value Claude Code defines. These are Claude Code's own conventions as
// recorded at 2.1.323; if a version renames one, it changes here only.

// Agent identity.
const (
	// Binary is the executable name looked up on PATH.
	Binary = "claude"
	// displayName is what Name() reports.
	displayName = "Claude Code"
)

// Home layout.
const (
	// defaultDirName is the config dir used when CLAUDE_CONFIG_DIR is absent.
	defaultDirName = ".claude"
	// extraHomesGlob matches additional config dirs (~/.claude-work, …).
	extraHomesGlob = ".claude-*"
	// globalConfigFile holds account metadata and per-project state. The
	// default home keeps it one level up, at ~/.claude.json.
	globalConfigFile       = ".claude.json"
	settingsFile           = "settings.json"
	localSettingsFile      = "settings.local.json"
	credentialsFile        = ".credentials.json"
	memoryFileName         = "CLAUDE.md"
	localMemoryFile        = "CLAUDE.local.md"
	agentsMDFile           = "AGENTS.md"
	keybindingsFile        = "keybindings.json"
	legacyMCPFile          = "mcp.json"
	projectMCPFile         = ".mcp.json"
	worktreeInclude        = ".worktreeinclude"
	projectsDir            = "projects"
	historyFile            = "history.jsonl"
	sessionsDir            = "sessions"
	jobsDir                = "jobs"
	jobStateFile           = "state.json"
	jobTimelineFile        = "timeline.jsonl"
	daemonDir              = "daemon"
	teamsDir               = "teams"
	teamConfigFile         = "config.json"
	tasksDir               = "tasks"
	rulesDir               = "rules"
	skillsDir              = "skills"
	skillFile              = "SKILL.md"
	commandsDir            = "commands"
	agentsDir              = "agents"
	outputStylesDir        = "output-styles"
	workflowsDir           = "workflows"
	agentMemoryDir         = "agent-memory"
	themesDir              = "themes"
	pluginsDir             = "plugins"
	fileHistoryDir         = "file-history"
	sessionEnvDir          = "session-env"
	shellSnapshotsDir      = "shell-snapshots"
	pasteCacheDir          = "paste-cache"
	plansDir               = "plans"
	debugDir               = "debug"
	backupsDir             = "backups"
	telemetryDir           = "telemetry"
	cacheDir               = "cache"
	stateDir               = "state"
	feedbackDir            = "feedback"
	usageDataDir           = "usage-data"
	uploadsDir             = "uploads"
	devModsDir             = "dev-mods"
	ideDir                 = "ide"
	chromeDir              = "chrome"
	statsCacheFile         = "stats-cache.json"
	policyLimitsFile       = "policy-limits.json"
	remoteSettingsFile     = "remote-settings.json"
	syncedAtSuffix         = ".synced-at"
	stampSuffix            = ".stamp.json"
	daemonLockFile         = "daemon.lock"
	daemonLogFile          = "daemon.log"
	daemonStatusFile       = "daemon.status.json"
	daemonRosterFile       = "roster.json"
	daemonAuthStatusFile   = "daemon-auth-status.json"
	daemonAuthCooldownFile = "daemon-auth-cooldown"
	lastUpdateFile         = ".last-update-result.json"
	lastCleanupFile        = ".last-cleanup"
	mcpNeedsAuthFile       = "mcp-needs-auth-cache.json"
	dotClaudeDir           = ".claude"
	memoryDir              = "memory"
	memoryIndexFile        = "MEMORY.md"
	subagentsDir           = "subagents"
	metaSuffix             = ".meta.json"
	transcriptExt          = ".jsonl"
	jsonExt                = ".json"
	markdownExt            = ".md"
	keyExt                 = ".key"
	gitDir                 = ".git"
	changelogFile          = "changelog.md"
	modelCatalogDir        = "model-catalog"
	shellSnapshotPrefix    = "snapshot-"
	debugLatest            = "latest"
	configBackupPrefix     = ".claude.json."
)

// Plugin layout under <home>/plugins.
const (
	installedPluginsFile   = "installed_plugins.json"
	knownMarketplacesFile  = "known_marketplaces.json"
	blocklistFile          = "blocklist.json"
	pluginCacheDir         = "cache"
	pluginMarketplacesDir  = "marketplaces"
	pluginDataDir          = "data"
	pluginStoreDir         = "store"
	pluginSyncedDir        = "synced"
	pluginCatalogCacheFile = "plugin-catalog-cache.json"
	pluginDirCacheFile     = "plugin-directory-cache-v2.json"
	pluginManifestDir      = ".claude-plugin"
	pluginManifestFile     = "plugin.json"
	pluginIDSep            = "@"
)

// System (managed) locations per OS (docs "managed-settings"), relative to
// Host.SystemRoot and slash-separated: "/" + managedDirLinux is
// /etc/claude-code, `C:\` + managedDirWindows is C:\Program Files\ClaudeCode.
const (
	managedDirDarwin    = "Library/Application Support/ClaudeCode"
	managedDirLinux     = "etc/claude-code"
	managedDirWindows   = "Program Files/ClaudeCode"
	managedSettingsFile = "managed-settings.json"
	managedDropInDir    = "managed-settings.d"
	managedMCPFile      = "managed-mcp.json"
	goosDarwin          = "darwin"
	goosWindows         = "windows"
)

// Environment variables Claude Code reads.
const (
	// ConfigDirEnv selects a non-default home; ABSENT means the default home.
	ConfigDirEnv = "CLAUDE_CONFIG_DIR"
	// OAuthTokenEnv carries a long-lived `claude setup-token` token.
	OAuthTokenEnv = "CLAUDE_CODE_OAUTH_TOKEN"
	// APIKeyEnv carries a console API key (pay per token).
	APIKeyEnv = "ANTHROPIC_API_KEY"
	// AuthTokenEnv is the bearer token third-party backends use.
	AuthTokenEnv = "ANTHROPIC_AUTH_TOKEN"
	// Provider switches that also bill per token.
	UseBedrockEnv = "CLAUDE_CODE_USE_BEDROCK"
	UseVertexEnv  = "CLAUDE_CODE_USE_VERTEX"
	UseFoundryEnv = "CLAUDE_CODE_USE_FOUNDRY"
	BaseURLEnv    = "ANTHROPIC_BASE_URL"
	userEnv       = "USER"
)

// Keychain naming (agx keychain.go, verified 2.1.52+).
const (
	// keychainService is the bare service name; non-default homes append
	// "-" + the first keychainHashLen hex chars of sha256(config dir).
	keychainService = "Claude Code-credentials"
	keychainHashLen = 8
)

// Credential sources recorded on Credential.Source.
const (
	SourceKeychain CredentialSource = "keychain"
	SourceFile     CredentialSource = "file"
)

// Transcript bounds: ~/.claude/projects is ~2 GB on a busy machine, so files
// are never read whole: the cwd comes from the head, the title from the tail.
const (
	headScanBytes  = 256 << 10
	scanStartBytes = 64 << 10
	tailScanBytes  = 256 << 10
	// maxLineBytes caps one transcript line for the streaming reader.
	maxLineBytes = 16 << 20
	// maxJSONFileBytes caps any single JSON file read whole (~/.claude.json is a few MB).
	maxJSONFileBytes = 64 << 20
)

// Network endpoints (agx constants.go).
const (
	// DefaultBaseURL is the API origin; Agent.WithBaseURL overrides it (tests).
	DefaultBaseURL      = "https://api.anthropic.com"
	usagePath           = "/api/oauth/usage"
	profilePath         = "/api/oauth/profile"
	messagesPath        = "/v1/messages"
	oauthBetaHeader     = "anthropic-beta"
	oauthBetaValue      = "oauth-2025-04-20"
	userAgent           = "agentio"
	maxResponseBytes    = 1 << 20
	snippetMax          = 200
	headerAuthorization = "Authorization"
	headerAccept        = "Accept"
	headerUserAgent     = "User-Agent"
	headerContentType   = "Content-Type"
	bearerPrefix        = "Bearer "
	mimeJSON            = "application/json"
)

// liveTolerance is how far a process's start time may sit from the
// startedAt Claude Code records in sessions/<pid>.json and still count as the
// same process. Claude records startedAt a few seconds after exec (3.2 s
// measured on this server), and a reused pid starts minutes or days later.
const liveTolerance = 2 * time.Minute

// Spec template placeholders; they must equal the root's (agentio spec.go),
// which keeps them unexported.
const (
	placeholderArgs   = "{args}"
	placeholderID     = "{id}"
	placeholderModel  = "{model}"
	placeholderDir    = "{dir}"
	placeholderPrompt = "{prompt}"
)

// Process rows that carry "claude" but are not Claude Code (agentop rules):
// the desktop app, its Chrome native host, and its support dir.
const (
	excludeDesktopApp     = "Claude.app"
	excludeChromeHost     = "claude-in-chrome"
	excludeDesktopSupport = "Application Support/Claude"
)

// npmScriptDir is the package directory an npm install's script lives
// in (`node …/node_modules/@anthropic-ai/claude-code/cli.js`); the
// trailing "/" keeps `npm install @anthropic-ai/claude-code` out.
const npmScriptDir = NPMPackage + "/"
