package claude

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/khanakia/agentcli"
)

// Local-data file name shapes.
const (
	// checkpointVersionSep splits "<hash>@v<n>" file-history names.
	checkpointVersionSep = "@v"
	// snapshotNameSep splits "snapshot-<shell>-<ms>-<rand>.sh".
	snapshotNameSep = "-"
	ideLockExt      = ".lock"
	debugLogExt     = ".txt"
	// maxChangelogBytes caps the cached changelog read.
	maxChangelogBytes = 4 << 20
)

// Checkpoint is one file-history entry (a pre-edit copy).
type Checkpoint struct {
	Session  string
	FileHash string
	Version  int
	Path     string
	Size     int64
}

// FileHistory lists file-history/<session>/ checkpoints. cost: Disk · Read.
func (a *Agent) FileHistory(h Home, session string) ([]Checkpoint, error) {
	if session == "" || strings.ContainsAny(session, `/\`) {
		return nil, fmt.Errorf("%w: session %q", agentcli.ErrNotFound, session)
	}
	dir := filepath.Join(h.Dir(), fileHistoryDir, session)
	entries, err := os.ReadDir(dir)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: list %s: %w", dir, err)
	}
	var out []Checkpoint
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		c := Checkpoint{Session: session, FileHash: e.Name(), Path: filepath.Join(dir, e.Name())}
		if hash, v, ok := strings.Cut(e.Name(), checkpointVersionSep); ok {
			c.FileHash = hash
			c.Version, _ = strconv.Atoi(v)
		}
		if info, err := e.Info(); err == nil {
			c.Size = info.Size()
		}
		out = append(out, c)
	}
	return out, nil
}

// ShellSnapshot is one shell-snapshots/ file.
type ShellSnapshot struct {
	Shell   string
	Created time.Time
	Path    string
}

// ShellSnapshots lists shell-snapshots/snapshot-<shell>-<ms>-<rand>.sh,
// oldest first. cost: Disk · Read.
func (a *Agent) ShellSnapshots(h Home) ([]ShellSnapshot, error) {
	dir := filepath.Join(h.Dir(), shellSnapshotsDir)
	entries, err := os.ReadDir(dir)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: list %s: %w", dir, err)
	}
	var out []ShellSnapshot
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), shellSnapshotPrefix)
		if e.IsDir() || !ok {
			continue
		}
		parts := strings.Split(strings.TrimSuffix(rest, filepath.Ext(rest)), snapshotNameSep)
		s := ShellSnapshot{Path: filepath.Join(dir, e.Name())}
		if len(parts) > 0 {
			s.Shell = parts[0]
		}
		if len(parts) > 1 {
			if ms, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				s.Created = millis(ms).Time()
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out, nil
}

// LastCleanup reads .last-cleanup (an ISO time); nil when absent.
// cost: Disk · Read.
func (a *Agent) LastCleanup(h Home) (*time.Time, error) {
	data, found, err := readFileBounded(filepath.Join(h.Dir(), lastCleanupFile), maxResponseBytes)
	if err != nil || !found {
		return nil, err
	}
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(data)))
	if err != nil {
		return nil, fmt.Errorf("claude: %s: %w", lastCleanupFile, err)
	}
	return &t, nil
}

// DataEntry is the size of one top-level entry of a home.
type DataEntry struct {
	Name  string
	Bytes int64
	// Foreign marks an entry Claude Code neither documents nor writes
	// (CLAUDE_API §29); it is never parsed.
	Foreign bool
}

// claudeEntries are the top-level names Claude Code documents or writes;
// everything else in a home is Foreign.
func claudeEntries() []string {
	return []string{globalConfigFile, settingsFile, localSettingsFile, credentialsFile, memoryFileName, keybindingsFile, legacyMCPFile,
		projectsDir, historyFile, sessionsDir, jobsDir, daemonDir, teamsDir, tasksDir, rulesDir, skillsDir, commandsDir, agentsDir,
		outputStylesDir, workflowsDir, agentMemoryDir, themesDir, pluginsDir, fileHistoryDir, sessionEnvDir, shellSnapshotsDir,
		pasteCacheDir, plansDir, debugDir, backupsDir, telemetryDir, cacheDir, stateDir, feedbackDir, usageDataDir, uploadsDir,
		devModsDir, ideDir, chromeDir, statsCacheFile, policyLimitsFile, remoteSettingsFile, daemonLockFile, daemonLogFile,
		daemonStatusFile, daemonAuthStatusFile, daemonAuthCooldownFile, lastUpdateFile, lastCleanupFile, mcpNeedsAuthFile,
		todosDir, imageCacheDir, downloadsDir}
}

// Legacy and observed Claude-written dirs not in Paths.
const (
	todosDir      = "todos"
	imageCacheDir = "image-cache"
	downloadsDir  = "downloads"
)

// isClaudeEntry reports a documented name, including the policy and
// remote-settings side files (<name>.signature.json, .stamp.json, .synced-at).
func isClaudeEntry(name string) bool {
	if slices.Contains(claudeEntries(), name) {
		return true
	}
	for _, base := range []string{policyLimitsFile, remoteSettingsFile} {
		if strings.HasPrefix(name, base+".") {
			return true
		}
	}
	return false
}

// DataUsage sums the size of every top-level entry of h (walking dirs, not
// following symlinks). projects/ can be GBs: this walks, it is slow.
// cost: Disk · Read.
func (a *Agent) DataUsage(h Home) ([]DataEntry, error) {
	entries, err := os.ReadDir(h.Dir())
	if err != nil {
		return nil, fmt.Errorf("claude: list %s: %w", h.Dir(), err)
	}
	out := make([]DataEntry, 0, len(entries))
	for _, e := range entries {
		de := DataEntry{Name: e.Name(), Foreign: !isClaudeEntry(e.Name())}
		_ = filepath.WalkDir(filepath.Join(h.Dir(), e.Name()), func(_ string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // unreadable entries are skipped, not fatal
			}
			if info, ierr := d.Info(); ierr == nil && info.Mode().IsRegular() {
				de.Bytes += info.Size()
			}
			return nil
		})
		out = append(out, de)
	}
	return out, nil
}

// IDELock is ide/<port>.lock. No sample on disk: the fields are docs
// knowledge, the rest stays in Unknown and the body in Raw.
type IDELock struct {
	Port      int
	PID       int
	Workspace []string
	IDEName   string
	Transport string
	Raw       agentcli.Raw
	Unknown   agentcli.Unknown
}

type wireIDELock struct {
	PID              int      `json:"pid"`
	WorkspaceFolders []string `json:"workspaceFolders"`
	IDEName          string   `json:"ideName"`
	Transport        string   `json:"transport"`
}

// IDELocks reads ide/*.lock. cost: Disk · Read.
func (a *Agent) IDELocks(h Home) ([]IDELock, error) {
	dir := filepath.Join(h.Dir(), ideDir)
	entries, err := os.ReadDir(dir)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: list %s: %w", dir, err)
	}
	var out []IDELock
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ideLockExt {
			continue
		}
		data, found, err := readFileBounded(filepath.Join(dir, e.Name()), maxResponseBytes)
		if err != nil || !found {
			continue
		}
		l := IDELock{Raw: agentcli.Raw(data)}
		l.Port, _ = strconv.Atoi(strings.TrimSuffix(e.Name(), ideLockExt))
		var w wireIDELock
		if unknown, _, err := decodeLenient(ideDir, data, reflect.TypeFor[wireIDELock](), func(b []byte) error { w = wireIDELock{}; return json.Unmarshal(b, &w) }); err == nil {
			l.PID, l.Workspace, l.IDEName, l.Transport, l.Unknown = w.PID, w.WorkspaceFolders, w.IDEName, w.Transport, unknown
		}
		out = append(out, l)
	}
	return out, nil
}

// DebugLog is one debug/<session>.txt.
type DebugLog struct {
	Session  string
	Path     string
	Size     int64
	Modified time.Time
	// Latest marks the file debug/latest points to.
	Latest bool
}

// DebugLogs lists debug/*.txt. cost: Disk · Read.
func (a *Agent) DebugLogs(h Home) ([]DebugLog, error) {
	dir := filepath.Join(h.Dir(), debugDir)
	entries, err := os.ReadDir(dir)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: list %s: %w", dir, err)
	}
	latest, _ := filepath.EvalSymlinks(filepath.Join(dir, debugLatest))
	var out []DebugLog
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != debugLogExt {
			continue
		}
		p := filepath.Join(dir, e.Name())
		l := DebugLog{Session: strings.TrimSuffix(e.Name(), debugLogExt), Path: p}
		if info, err := e.Info(); err == nil {
			l.Size, l.Modified = info.Size(), info.ModTime()
		}
		if real, err := filepath.EvalSymlinks(p); err == nil && latest != "" && real == latest {
			l.Latest = true
		}
		out = append(out, l)
	}
	return out, nil
}

// Changelog reads the cached cache/changelog.md (bounded 4 MiB); "" when
// absent. cost: Disk · Read.
func (a *Agent) Changelog(h Home) (string, error) {
	data, found, err := readFileBounded(filepath.Join(h.Dir(), cacheDir, changelogFile), maxChangelogBytes)
	if err != nil || !found {
		return "", err
	}
	return string(data), nil
}

// ModelCatalogFiles lists cache/model-catalog/ file paths; the shape is
// not recorded, so they are names only. cost: Disk · Read.
func (a *Agent) ModelCatalogFiles(h Home) ([]string, error) {
	dir := filepath.Join(h.Dir(), cacheDir, modelCatalogDir)
	entries, err := os.ReadDir(dir)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: list %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out, nil
}
