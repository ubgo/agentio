package pi

// Names pi uses on disk and on the command line. Sources are the pinned
// @earendil-works/pi-coding-agent 1.5.2 install (PKG = its package dir);
// PI_API.md records each one with its file:line.
const (
	// Binary is the executable name on PATH (PKG/package.json "bin").
	Binary = "pi"
	// displayName is the product name.
	displayName = "pi"
	// HomeEnv relocates the agent dir; its value IS the agent dir (no
	// "agent" suffix is added, PKG/dist/config.js:510-516).
	HomeEnv = "PI_CODING_AGENT_DIR"
	// SessionDirEnv relocates session storage (PKG/dist/config.js:496);
	// a custom session dir is flat, every cwd in one folder.
	SessionDirEnv = "PI_CODING_AGENT_SESSION_DIR"
	// defaultHomeDir is ~/.pi/agent (PKG/dist/config.js:510-516).
	defaultHomeDir = ".pi/agent"
	// projectDirName is the project-level config dir (piConfig.configDir).
	projectDirName = ".pi"
	// PackageName is the official npm package; legacyPackageName is the
	// deprecated one it replaced after 0.73.1 (npm "deprecated" field).
	PackageName       = "@earendil-works/pi-coding-agent"
	legacyPackageName = "@mariozechner/pi-coding-agent"
)

// Files and dirs inside the agent dir (PKG/dist/config.js:522-552).
const (
	settingsFile    = "settings.json"
	authFile        = "auth.json"
	modelsFile      = "models.json"
	modelsStoreFile = "models-store.json"
	mcpFile         = "mcp.json"
	trustFile       = "trust.json"
	sessionsDir     = "sessions"
	skillsDir       = "skills"
	promptsDir      = "prompts"
	extensionsDir   = "extensions"
	systemFile      = "SYSTEM.md"
	appendFile      = "APPEND_SYSTEM.md"
	skillFile       = "SKILL.md"
	sessionExt      = ".jsonl"
	markdownExt     = ".md"
)

// Context file names, in the order pi tries them per directory: the first
// existing one wins (PKG/dist/core/resource-loader.js:130,185).
func contextFileNames() []string {
	return []string{"AGENTS.override.md", "AGENTS.md", "AGENTS.MD", "CLAUDE.md", "CLAUDE.MD"}
}

// Flags (PKG/docs/cli.md, pi --help 1.5.2).
const (
	flagSession            = "--session"
	flagSessionID          = "--session-id"
	flagContinue           = "--continue"
	flagFork               = "--fork"
	flagModel              = "--model"
	flagProvider           = "--provider"
	flagThinking           = "--thinking"
	flagPrint              = "--print"
	flagMode               = "--mode"
	flagTools              = "--tools"
	flagExcludeTools       = "--exclude-tools"
	flagNoMCP              = "--no-mcp"
	flagNoExtensions       = "--no-extensions"
	flagNoContextFiles     = "--no-context-files"
	flagName               = "--name"
	flagNoSession          = "--no-session"
	flagOffline            = "--offline"
	flagApprove            = "--approve"
	flagNoApprove          = "--no-approve"
	flagSystemPrompt       = "--system-prompt"
	flagAppendSystemPrompt = "--append-system-prompt"
	flagVersion            = "--version"
	// endOfOptions ends option parsing: everything after is a message, so
	// a prompt that starts with "-" is never read as a flag (pi --help:
	// `pi -p -- "- Summarize these points"`).
	endOfOptions = "--"
	// listSep joins list flag values (--tools read,grep).
	listSep = ","
)

// Spec template placeholders (agentio.Spec).
const (
	placeholderArgs   = "{args}"
	placeholderID     = "{id}"
	placeholderModel  = "{model}"
	placeholderPrompt = "{prompt}"
)

// Mode is --mode: how pi writes its output.
type Mode string

// Modes (pi --help: "text (default), json, or rpc").
const (
	ModeText Mode = "text"
	// ModeJSON writes JSONL events to stdout and exits (PKG/docs/json.md).
	ModeJSON Mode = "json"
	// ModeRPC reads JSONL commands on stdin until shutdown.
	ModeRPC Mode = "rpc"
)

// ModeValues lists every Mode. Pure.
func ModeValues() []Mode { return []Mode{ModeText, ModeJSON, ModeRPC} }

// ThinkingLevel is --thinking and settings defaultThinkingLevel.
type ThinkingLevel string

// Thinking levels (pi --help; settings schema default "medium").
const (
	ThinkingOff     ThinkingLevel = "off"
	ThinkingMinimal ThinkingLevel = "minimal"
	ThinkingLow     ThinkingLevel = "low"
	ThinkingMedium  ThinkingLevel = "medium"
	ThinkingHigh    ThinkingLevel = "high"
	ThinkingXHigh   ThinkingLevel = "xhigh"
	ThinkingMax     ThinkingLevel = "max"
)

// ThinkingLevelValues lists every level, lowest first. Pure.
func ThinkingLevelValues() []ThinkingLevel {
	return []ThinkingLevel{ThinkingOff, ThinkingMinimal, ThinkingLow, ThinkingMedium, ThinkingHigh, ThinkingXHigh, ThinkingMax}
}

// readOnlyTools is the tool allowlist pi --help advertises as "Read-only
// mode (no file modifications possible)": `pi --tools read,grep,find,ls`.
func readOnlyTools() []string { return []string{"read", "grep", "find", "ls"} }

// noPermissionModesReason is why pi refuses ask and accept-edits: pi "does
// not ask for approval before every tool call" (PKG/docs/security.md:3).
const noPermissionModesReason = "pi has no permission modes; it always runs without prompts"
