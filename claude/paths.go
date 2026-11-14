package claude

import (
	"path/filepath"
)

// Paths are the files and dirs of one home, computed purely from the home
// dir and the user home, so callers never hard-code a name.
type Paths struct {
	Home                                                                                                                                                    string
	GlobalConfig                                                                                                                                            string // default: ~/.claude.json; other homes: <home>/.claude.json
	Settings                                                                                                                                                string
	Credentials                                                                                                                                             string // often absent on macOS: the keychain holds the login
	Memory                                                                                                                                                  string
	Rules, Skills, Commands, Agents, OutputStyles, Workflows, AgentMemory, Themes                                                                           string
	Keybindings                                                                                                                                             string
	MCPLegacy                                                                                                                                               string // <home>/mcp.json: observed on disk, not in docs
	Projects                                                                                                                                                string
	History                                                                                                                                                 string
	Sessions                                                                                                                                                string
	Jobs, Daemon                                                                                                                                            string
	Teams, Tasks                                                                                                                                            string
	FileHistory, SessionEnv, ShellSnapshots, PasteCache, Plans, Debug, Backups, Telemetry, Cache, State, Feedback, UsageData, Uploads, DevMods, IDE, Chrome string
	Plugins                                                                                                                                                 PluginPaths
	StatsCache                                                                                                                                              string
	PolicyLimits                                                                                                                                            string
	RemoteSettings                                                                                                                                          string
	DaemonLock, DaemonLog, DaemonStatus, DaemonAuthStatus, DaemonAuthCooldown                                                                               string
	LastUpdateResult, LastCleanup, MCPNeedsAuthCache                                                                                                        string
}

// PluginPaths are the files under <home>/plugins.
type PluginPaths struct {
	Dir, Installed, KnownMarketplaces, Blocklist, Cache, Marketplaces, Data, Store, Synced, CatalogCache, DirectoryCache string
}

// ProjectPaths are the Claude files of one project folder.
type ProjectPaths struct {
	Memory                                                                []string // ./CLAUDE.md, ./.claude/CLAUDE.md, ./CLAUDE.local.md, ./AGENTS.md
	Settings                                                              string
	LocalSettings                                                         string // <RepoRoot>/.claude/settings.local.json (Dir when no repo)
	MCP                                                                   string
	WorktreeInclude                                                       string
	Rules, Skills, Commands, Agents, OutputStyles, Workflows, AgentMemory string
	HistoryDir                                                            string // <home>/projects/<EncodePath(Dir)>
	AutoMemory                                                            string // HistoryDir/memory
}

// SystemPaths are the managed (admin) locations for Host.GOOS under
// Host.SystemRoot. Every field is "" when the Host has no system locations
// (no GOOS or SystemRoot, the fixture default); readers skip "" paths.
type SystemPaths struct {
	ManagedDir                                                string
	ManagedSettings, ManagedDropIn, ManagedMCP, ManagedMemory string
}

// Paths computes the paths of h. cost: Pure.
func (a *Agent) Paths(h Home) Paths {
	d := h.Dir()
	j := func(n string) string { return filepath.Join(d, n) }
	pd := j(pluginsDir)
	pj := func(n string) string { return filepath.Join(pd, n) }
	return Paths{
		Home: d, GlobalConfig: a.globalConfigPath(h), Settings: j(settingsFile), Credentials: j(credentialsFile),
		Memory: j(memoryFileName), Rules: j(rulesDir), Skills: j(skillsDir), Commands: j(commandsDir), Agents: j(agentsDir),
		OutputStyles: j(outputStylesDir), Workflows: j(workflowsDir), AgentMemory: j(agentMemoryDir), Themes: j(themesDir),
		Keybindings: j(keybindingsFile), MCPLegacy: j(legacyMCPFile), Projects: j(projectsDir), History: j(historyFile),
		Sessions: j(sessionsDir), Jobs: j(jobsDir), Daemon: j(daemonDir), Teams: j(teamsDir), Tasks: j(tasksDir),
		FileHistory: j(fileHistoryDir), SessionEnv: j(sessionEnvDir), ShellSnapshots: j(shellSnapshotsDir), PasteCache: j(pasteCacheDir),
		Plans: j(plansDir), Debug: j(debugDir), Backups: j(backupsDir), Telemetry: j(telemetryDir), Cache: j(cacheDir), State: j(stateDir),
		Feedback: j(feedbackDir), UsageData: j(usageDataDir), Uploads: j(uploadsDir), DevMods: j(devModsDir), IDE: j(ideDir), Chrome: j(chromeDir),
		Plugins: PluginPaths{
			Dir: pd, Installed: pj(installedPluginsFile), KnownMarketplaces: pj(knownMarketplacesFile), Blocklist: pj(blocklistFile),
			Cache: pj(pluginCacheDir), Marketplaces: pj(pluginMarketplacesDir), Data: pj(pluginDataDir), Store: pj(pluginStoreDir),
			Synced: pj(pluginSyncedDir), CatalogCache: pj(pluginCatalogCacheFile), DirectoryCache: pj(pluginDirCacheFile),
		},
		StatsCache: j(statsCacheFile), PolicyLimits: j(policyLimitsFile), RemoteSettings: j(remoteSettingsFile),
		DaemonLock: j(daemonLockFile), DaemonLog: j(daemonLogFile), DaemonStatus: j(daemonStatusFile),
		DaemonAuthStatus: j(daemonAuthStatusFile), DaemonAuthCooldown: j(daemonAuthCooldownFile),
		LastUpdateResult: j(lastUpdateFile), LastCleanup: j(lastCleanupFile), MCPNeedsAuthCache: j(mcpNeedsAuthFile),
	}
}

// HistoryDir is where Claude Code keeps conversations started in dir. cost: Pure.
func (a *Agent) HistoryDir(h Home, dir string) string {
	return filepath.Join(h.Dir(), projectsDir, EncodePath(filepath.Clean(dir)))
}

// ProjectPaths computes the project-side paths of p for home h. cost: Pure.
func (a *Agent) ProjectPaths(h Home, p Project) ProjectPaths {
	dc := filepath.Join(p.Dir, dotClaudeDir)
	j := func(n string) string { return filepath.Join(dc, n) }
	hist := a.HistoryDir(h, p.Dir)
	return ProjectPaths{
		Memory: []string{
			filepath.Join(p.Dir, memoryFileName), j(memoryFileName),
			filepath.Join(p.Dir, localMemoryFile), filepath.Join(p.Dir, agentsMDFile),
		},
		Settings:      j(settingsFile),
		LocalSettings: filepath.Join(p.localRoot(), dotClaudeDir, localSettingsFile),
		MCP:           filepath.Join(p.Dir, projectMCPFile), WorktreeInclude: filepath.Join(p.Dir, worktreeInclude),
		Rules: j(rulesDir), Skills: j(skillsDir), Commands: j(commandsDir), Agents: j(agentsDir),
		OutputStyles: j(outputStylesDir), Workflows: j(workflowsDir), AgentMemory: j(agentMemoryDir),
		HistoryDir: hist, AutoMemory: filepath.Join(hist, memoryDir),
	}
}

// SystemPaths returns the managed locations for Host.GOOS under
// Host.SystemRoot, or the zero SystemPaths when the Host has neither (a
// fixture host never sees the test machine's /etc). Never reads
// runtime.GOOS. cost: Pure.
func (a *Agent) SystemPaths() SystemPaths {
	rel := managedDirLinux
	switch a.host.GOOS {
	case goosDarwin:
		rel = managedDirDarwin
	case goosWindows:
		rel = managedDirWindows
	}
	dir, ok := a.host.SystemPath(rel)
	if !ok {
		return SystemPaths{}
	}
	return SystemPaths{
		ManagedDir: dir, ManagedSettings: filepath.Join(dir, managedSettingsFile),
		ManagedDropIn: filepath.Join(dir, managedDropInDir), ManagedMCP: filepath.Join(dir, managedMCPFile),
		ManagedMemory: filepath.Join(dir, memoryFileName),
	}
}

// EncodePath maps a working directory to its history folder name under
// <home>/projects: every byte that is not an ASCII letter or digit becomes
// '-' (invariant 4; agx verified 158/158 real folders). cost: Pure.
func EncodePath(dir string) string {
	b := []byte(dir)
	for i, c := range b {
		if !isASCIIAlnum(c) {
			b[i] = '-'
		}
	}
	return string(b)
}

func isASCIIAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}
