package gemini

// Names recorded from @google/gemini-cli 0.68.0 (GEMINI_API.md). Each
// constant is the vendor's own spelling; the source line is in the API doc.
const (
	// Binary is the executable npm, Homebrew and the SEA build install.
	Binary = "gemini"
	// displayName is how the vendor names the product.
	displayName = "Gemini CLI"
	// Package is the npm package; its package.json holds the version.
	Package = "@google/gemini-cli"
	// defaultDirName is the home dir under the user home (GEMINI_DIR,
	// core Storage).
	defaultDirName = ".gemini"
)

// Environment variables Gemini CLI reads.
const (
	// HomeEnv names the dir that CONTAINS .gemini (core homedir():
	// process.env.GEMINI_CLI_HOME || os.homedir()), not .gemini itself.
	HomeEnv = "GEMINI_CLI_HOME"
	// APIKeyEnv is the Gemini API key (auth type gemini-api-key).
	APIKeyEnv = "GEMINI_API_KEY"
	// GoogleAPIKeyEnv is the Vertex AI express-mode key (auth type vertex-ai).
	GoogleAPIKeyEnv = "GOOGLE_API_KEY"
	// UseVertexEnv selects vertex-ai when settings name no auth type.
	UseVertexEnv = "GOOGLE_GENAI_USE_VERTEXAI"
	// UseGCAEnv selects the Google login and, with CloudAccessTokenEnv,
	// replaces the cached login with a raw access token.
	UseGCAEnv = "GOOGLE_GENAI_USE_GCA"
	// CloudAccessTokenEnv is a raw Google access token; with UseGCAEnv it
	// bypasses oauth_creds.json entirely.
	CloudAccessTokenEnv = "GOOGLE_CLOUD_ACCESS_TOKEN"
	// AppCredentialsEnv names an ADC file tried after oauth_creds.json; it
	// is used silently when the home's own file is invalid.
	AppCredentialsEnv = "GOOGLE_APPLICATION_CREDENTIALS"
	// AuthOverrideEnv overrides security.auth.selectedType in memory (the
	// CLI's own parent-to-child relaunch channel).
	AuthOverrideEnv = "GEMINI_CLI_AUTH_OVERRIDE"
	// SystemSettingsEnv overrides the system settings path.
	SystemSettingsEnv = "GEMINI_CLI_SYSTEM_SETTINGS_PATH"
	// SystemDefaultsEnv overrides the system defaults path.
	SystemDefaultsEnv = "GEMINI_CLI_SYSTEM_DEFAULTS_PATH"
	// TrustedFoldersEnv overrides the trustedFolders.json path.
	TrustedFoldersEnv = "GEMINI_CLI_TRUSTED_FOLDERS_PATH"
	// NoRelaunchEnv is set by the parent process on its relaunched child,
	// so the child is the session process.
	NoRelaunchEnv = "GEMINI_CLI_NO_RELAUNCH"
)

// Files and dirs in a home (core Storage) and a workspace.
const (
	settingsFile         = "settings.json"
	oauthCredsFile       = "oauth_creds.json"
	googleAccountsFile   = "google_accounts.json"
	projectsFile         = "projects.json"
	trustedFoldersFile   = "trustedFolders.json"
	tmpDir               = "tmp"
	historyDir           = "history"
	chatsDir             = "chats"
	projectRootFile      = ".project_root"
	extensionsDir        = "extensions"
	extensionManifest    = "gemini-extension.json"
	extensionEnablement  = "extension-enablement.json"
	mcpEnablementFile    = "mcp-server-enablement.json"
	commandsDir          = "commands"
	skillsDir            = "skills"
	skillFile            = "SKILL.md"
	agentsDotDir         = ".agents"
	defaultContextFile   = "GEMINI.md"
	gitDir               = ".git"
	systemDefaultsFile   = "system-defaults.json"
	sessionFilePrefix    = "session-"
	sessionFileExt       = ".jsonl"
	legacySessionFileExt = ".json"
)

// System settings dirs per OS (cli settings getSystemSettingsPath).
const (
	systemDirLinux   = "/etc/gemini-cli"
	systemDirDarwin  = "/Library/Application Support/GeminiCli"
	systemDirWindows = `C:\ProgramData\gemini-cli`
)

// Flags (gemini --help, 0.68.0).
const (
	flagModel             = "--model"
	flagPrompt            = "--prompt"
	flagPromptInteractive = "--prompt-interactive"
	flagResume            = "--resume"
	flagIncludeDirs       = "--include-directories"
	flagApprovalMode      = "--approval-mode"
	flagOutputFormat      = "--output-format"
	flagVersion           = "--version"
	outputStreamJSON      = "stream-json"
)

// Spec template placeholders (agentio.Spec).
const (
	placeholderArgs   = "{args}"
	placeholderID     = "{id}"
	placeholderModel  = "{model}"
	placeholderDir    = "{dir}"
	placeholderPrompt = "{prompt}"
)

// Process match strings: the CLI runs as node with the bin path as
// argv[1] (the npm shim .../bin/gemini as invoked, or a path inside the
// package).
const (
	argvPackage    = "@google/gemini-cli"
	argPromptShort = "-p"
)

// Parse bounds: never read a transcript whole.
const (
	// headLimit bounds the head read of a session file: the metadata line
	// and the first user message are at the top.
	headLimit = 64 << 10
	// tailLimit bounds the tail read for the newest $set summary and
	// lastUpdated, as the Claude reader does for titles.
	tailLimit = 256 << 10
	// maxScanLine is the longest JSONL line the scanner accepts.
	maxScanLine = 4 << 20
	// titleRunes caps a title taken from a user message.
	titleRunes = 120
)
