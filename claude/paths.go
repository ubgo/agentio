package claude

import (
	"path/filepath"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
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
	LocalSettings                                                         string // <RepoRoot>/.claude/settings.local.json (Dir outside a repo, on Windows, or when RepoRoot is the home dir)
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
		LocalSettings: filepath.Join(a.localRoot(p), dotClaudeDir, localSettingsFile),
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
// <home>/projects, exactly as Claude Code does on every OS (the shipped
// 2.1.325 bundle: replace(/[^a-zA-Z0-9]/g, "-"), cut at 200, hash suffix):
//   - every UTF-16 code unit that is not an ASCII letter or digit becomes
//     '-', so "é" is one '-' and an emoji two (JavaScript strings are
//     UTF-16), and a Windows path keeps its drive letter
//     (C:\Users\x\proj → C--Users-x-proj);
//   - a key longer than maxKeyLen units is cut to maxKeyLen and gets "-" plus
//     base 36 of the absolute Java-style string hash of the original path.
//
// Invariant 4; agx verified 158/158 short real folders. cost: Pure.
func EncodePath(dir string) string {
	units := utf16.Encode([]rune(dir))
	b := make([]byte, len(units))
	for i, u := range units {
		if u < utf8.RuneSelf && isASCIIAlnum(byte(u)) {
			b[i] = byte(u)
		} else {
			b[i] = '-'
		}
	}
	if len(b) <= maxKeyLen {
		return string(b)
	}
	h := int64(stringHash(units))
	if h < 0 {
		h = -h
	}
	return string(b[:maxKeyLen]) + keyHashSep + strconv.FormatInt(h, keyHashBase)
}

// History key limits (Claude Code's $H: SDe = 200, base 36 suffix).
const (
	maxKeyLen   = 200
	keyHashSep  = "-"
	keyHashBase = 36
)

// stringHash is JavaScript's `e = (e << 5) - e + charCodeAt(n) | 0` over
// the UTF-16 units: Java's String.hashCode, wrapping in int32.
func stringHash(units []uint16) int32 {
	var h int32
	for _, u := range units {
		h = (h << 5) - h + int32(u)
	}
	return h
}

func isASCIIAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}
