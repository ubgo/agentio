package codex

import (
	"path/filepath"
)

// File and directory names inside a Codex home, verbatim from a real
// codex-cli 0.162.1 home and upstream source (CODEX_API.md §3).
const (
	fileConfig            = "config.toml"
	fileProfileSuffix     = ".config.toml"
	fileAgentsMD          = "AGENTS.md"
	dirRules              = "rules"
	fileHooks             = "hooks.json"
	fileAuth              = "auth.json"
	fileCredentials       = ".credentials.json"
	fileInstallationID    = "installation_id"
	dirSessions           = "sessions"
	dirArchivedSessions   = "archived_sessions"
	fileSessionIndex      = "session_index.jsonl"
	fileHistory           = "history.jsonl"
	dirShellSnapshots     = "shell_snapshots"
	dirAttachments        = "attachments"
	fileStateDB           = "state_5.sqlite"
	fileThreadHistoryDB   = "thread_history_1.sqlite"
	fileGoalsDB           = "goals_1.sqlite"
	fileQueueDB           = "queue_1.sqlite"
	fileMemoriesDB        = "memories_1.sqlite"
	fileLogsDB            = "logs_2.sqlite"
	fileModelsCache       = "models_cache.json"
	fileVersionCache      = "version.json"
	dirSkills             = "skills"
	dirVendorSkills       = "vendor_imports/skills"
	dirPlugins            = "plugins"
	dirPluginCache        = "plugins/cache"
	dirMemories           = "memories"
	dirCache              = "cache"
	dirLog                = "log"
	dirTmp                = "tmp"
	fileDesktopState      = ".codex-global-state.json"
	fileDesktopDB         = "sqlite/codex-dev.db"
	fileIPCSocket         = "ipc/ipc.sock"
	fileChatProcesses     = "process_manager/chat_processes.json"
	dirComputerUse        = "computer-use"
	dirBrowser            = "browser"
	dirVisualizations     = "visualizations"
	dirThreadWriterLocks  = "thread-writer-locks"
	markerPersonality     = ".personality_migration"
	markerSandbox         = ".sandbox_migration"
	markerAppServerState  = ".app-server-state-reconciled-v1"
	fileTranscriptHistory = "transcription-history.jsonl"
)

// Paths names every file and directory Codex uses in one home, so callers
// never hard-code a name. Paths are computed, not checked; Inventory says
// which exist.
type Paths struct {
	Home string

	// config and instructions
	Config   string // config.toml
	AgentsMD string // AGENTS.md (user-level instructions)
	Rules    string // rules/ (execpolicy *.rules)
	Hooks    string // hooks.json

	// identity
	Auth           string // auth.json (0600)
	Credentials    string // .credentials.json (keyring fallback store)
	InstallationID string // installation_id

	// conversations
	Sessions             string // sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl
	ArchivedSessions     string // archived_sessions/
	SessionIndex         string // session_index.jsonl (titles)
	History              string // history.jsonl (prompt history)
	TranscriptionHistory string // transcription-history.jsonl (dictation)
	ShellSnapshots       string // shell_snapshots/
	Attachments          string // attachments/

	// SQLite; directory = CODEX_SQLITE_HOME / sqlite_home, default Home
	SQLiteHome      string
	StateDB         string // state_5.sqlite
	ThreadHistoryDB string // thread_history_1.sqlite
	GoalsDB         string // goals_1.sqlite
	QueueDB         string // queue_1.sqlite
	MemoriesDB      string // memories_1.sqlite
	LogsDB          string // logs_2.sqlite

	// catalogs and caches
	ModelsCache  string // models_cache.json
	VersionCache string // version.json
	Skills       string // skills/ (+ skills/.system bundled)
	VendorSkills string // vendor_imports/skills
	Plugins      string // plugins/
	PluginCache  string // plugins/cache/<marketplace>/<plugin>/<version>/
	Memories     string // memories/
	Cache        string // cache/
	Log          string // log/ (or config log_dir)
	Tmp          string // tmp/

	// desktop app files: listed so callers know they exist, never parsed
	// (no recorded shape)
	DesktopState      string // .codex-global-state.json
	DesktopDB         string // sqlite/codex-dev.db
	IPCSocket         string // ipc/ipc.sock
	ChatProcesses     string // process_manager/chat_processes.json
	ComputerUse       string // computer-use/
	Browser           string // browser/
	Visualizations    string // visualizations/
	ThreadWriterLocks string // thread-writer-locks/

	// Markers are migration markers (presence = done).
	Markers []string
}

// ProfileConfig is the v2 profile file `<home>/<name>.config.toml` (§4.2).
// A method rather than the doc's func field so Paths stays printable. Pure.
func (p Paths) ProfileConfig(name string) string {
	return filepath.Join(p.Home, name+fileProfileSuffix)
}

// Paths returns every path of h, with SQLite and logs in the home. Use
// PathsWith when the config may move them. Pure.
func (a *Agent) Paths(h Home) Paths {
	return pathsFor(h.Dir(), "", "")
}

// PathsWith resolves sqlite_home and log_dir from c, and CODEX_SQLITE_HOME
// from Host.Env (it wins over sqlite_home, state/src/lib.rs:133). Pure.
func (a *Agent) PathsWith(h Home, c Config) Paths {
	sqlite := c.SQLiteHome.V
	if v, ok := a.host.Getenv(EnvSQLiteHome); ok && v != "" {
		sqlite = v
	}
	return pathsFor(h.Dir(), sqlite, c.LogDir.V)
}

func pathsFor(home, sqliteHome, logDir string) Paths {
	j := func(name string) string { return filepath.Join(home, name) }
	if sqliteHome == "" {
		sqliteHome = home
	}
	s := func(name string) string { return filepath.Join(sqliteHome, name) }
	log := j(dirLog)
	if logDir != "" {
		log = logDir
	}
	return Paths{
		Home:     home,
		Config:   j(fileConfig),
		AgentsMD: j(fileAgentsMD),
		Rules:    j(dirRules),
		Hooks:    j(fileHooks),

		Auth:           j(fileAuth),
		Credentials:    j(fileCredentials),
		InstallationID: j(fileInstallationID),

		Sessions:             j(dirSessions),
		ArchivedSessions:     j(dirArchivedSessions),
		SessionIndex:         j(fileSessionIndex),
		History:              j(fileHistory),
		TranscriptionHistory: j(fileTranscriptHistory),
		ShellSnapshots:       j(dirShellSnapshots),
		Attachments:          j(dirAttachments),

		SQLiteHome:      sqliteHome,
		StateDB:         s(fileStateDB),
		ThreadHistoryDB: s(fileThreadHistoryDB),
		GoalsDB:         s(fileGoalsDB),
		QueueDB:         s(fileQueueDB),
		MemoriesDB:      s(fileMemoriesDB),
		LogsDB:          s(fileLogsDB),

		ModelsCache:  j(fileModelsCache),
		VersionCache: j(fileVersionCache),
		Skills:       j(dirSkills),
		VendorSkills: j(dirVendorSkills),
		Plugins:      j(dirPlugins),
		PluginCache:  j(dirPluginCache),
		Memories:     j(dirMemories),
		Cache:        j(dirCache),
		Log:          log,
		Tmp:          j(dirTmp),

		DesktopState:      j(fileDesktopState),
		DesktopDB:         j(fileDesktopDB),
		IPCSocket:         j(fileIPCSocket),
		ChatProcesses:     j(fileChatProcesses),
		ComputerUse:       j(dirComputerUse),
		Browser:           j(dirBrowser),
		Visualizations:    j(dirVisualizations),
		ThreadWriterLocks: j(dirThreadWriterLocks),

		Markers: []string{j(markerPersonality), j(markerSandbox), j(markerAppServerState)},
	}
}
