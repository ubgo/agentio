package opencode

// Names recorded from opencode 1.18.30 (the compiled binary's bundled
// source and --help) and from the files it wrote on a real machine. Each
// constant is the vendor's own spelling.
const (
	// Binary is the executable every install method puts on PATH: the
	// native installer (~/.opencode/bin/opencode), the npm package's bin
	// and Homebrew.
	Binary = "opencode"
	// displayName is how the vendor names the product.
	displayName = "opencode"
	// Package is the npm package; its package.json holds the version.
	Package = "opencode-ai"
	// appDir is the per-app folder opencode puts under each XDG base dir
	// (Global.Path: data, config, state and cache are <base>/opencode).
	appDir = "opencode"
)

// Environment variables opencode reads (bundled source of 1.18.30).
const (
	// XDGDataHomeEnv is the XDG base data dir; opencode's data dir (the
	// home: storage/, opencode.db, auth.json, log/) is $XDG_DATA_HOME/opencode,
	// else ~/.local/share/opencode, on every OS. It is a base dir every XDG
	// tool shares, not a variable of opencode's own.
	XDGDataHomeEnv = "XDG_DATA_HOME"
	// XDGConfigHomeEnv is the XDG base config dir; opencode's global config
	// dir is $XDG_CONFIG_HOME/opencode, else ~/.config/opencode.
	XDGConfigHomeEnv = "XDG_CONFIG_HOME"
	// DBEnv overrides the database file: ":memory:", an absolute path, or
	// a path relative to the data dir.
	DBEnv = "OPENCODE_DB"
	// DisableChannelDBEnv ("1" or "true") makes a non-release build use
	// opencode.db instead of opencode-<channel>.db.
	DisableChannelDBEnv = "OPENCODE_DISABLE_CHANNEL_DB"
	// ConfigEnv names an extra config file merged over the global one.
	ConfigEnv = "OPENCODE_CONFIG"
	// ConfigDirEnv names an extra config dir (agents, commands, plugins).
	ConfigDirEnv = "OPENCODE_CONFIG_DIR"
	// ConfigContentEnv is inline config JSON merged over the files.
	ConfigContentEnv = "OPENCODE_CONFIG_CONTENT"
	// DisableAutoUpdateEnv turns off opencode's self-update.
	DisableAutoUpdateEnv = "OPENCODE_DISABLE_AUTOUPDATE"
)

// Files and dirs in the data dir.
const (
	// dbFile is the SQLite database of every session since 1.2.0 (release
	// builds; see DatabasePath for channel builds and OPENCODE_DB).
	dbFile = "opencode.db"
	// memoryDB is the OPENCODE_DB value that keeps the database in memory.
	memoryDB = ":memory:"
	// storageDir is the legacy JSON tree, written until 1.2.0 migrated it.
	storageDir = "storage"
	// storage subfolders: session/<projectID>/<sessionID>.json,
	// message/<sessionID>/<messageID>.json, part/<messageID>/<partID>.json,
	// project/<projectID>.json.
	sessionDir = "session"
	messageDir = "message"
	partDir    = "part"
	projectDir = "project"
	// migrationFile holds the number of the last storage migration run.
	migrationFile = "migration"
	// authFile holds provider credentials (never read by this package).
	authFile = "auth.json"
	jsonExt  = ".json"
)

// Default XDG base dirs, relative to the user home.
const (
	defaultDataBase   = ".local/share"
	defaultConfigBase = ".config"
)

// Release channels whose database is opencode.db; any other channel
// (a dev or local build) writes opencode-<channel>.db.
const (
	channelLatest = "latest"
	channelBeta   = "beta"
	channelProd   = "prod"
	channelDBStem = "opencode-"
	channelDBExt  = ".db"
)

// Id prefixes opencode gives its records (Identifier.ascending).
const (
	sessionIDPrefix = "ses_"
	messageIDPrefix = "msg_"
)

// Flags (opencode --help and opencode run --help, 1.18.30).
const (
	flagModel    = "--model"
	flagSession  = "--session"
	flagSessionS = "-s"
	flagContinue = "--continue"
	flagFork     = "--fork"
	flagPrompt   = "--prompt"
	flagAuto     = "--auto"
	flagFormat   = "--format"
	flagDir      = "--dir"
	flagVersion  = "--version"
	formatJSON   = "json"
	cmdRun       = "run"
	// argEnd ends option parsing, so a prompt that starts with '-' stays a
	// positional message.
	argEnd = "--"
)

// Spec template placeholders (agentio.Spec).
const (
	placeholderArgs   = "{args}"
	placeholderID     = "{id}"
	placeholderModel  = "{model}"
	placeholderPrompt = "{prompt}"
)

// Message roles and part types (message-v2 schema).
const (
	roleUser      = "user"
	roleAssistant = "assistant"
	partText      = "text"
	// finishToolCalls is the finish reason of an assistant message whose
	// step ended in tool calls: the agent keeps working after it.
	finishToolCalls = "tool-calls"
	// modeCompaction marks the assistant message that summarises a
	// compacted conversation (summary: true).
	modeCompaction = "compaction"
)
