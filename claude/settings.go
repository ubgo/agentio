package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// SettingsScope is a settings layer.
type SettingsScope string

// Settings layers, highest precedence first.
const (
	ScopeManaged SettingsScope = "managed"
	ScopeCLI     SettingsScope = "cli"
	ScopeLocal   SettingsScope = "local"
	ScopeProject SettingsScope = "project"
	ScopeUser    SettingsScope = "user"
)

// IsKnown reports a documented layer. cost: Pure.
func (s SettingsScope) IsKnown() bool {
	switch s {
	case ScopeManaged, ScopeCLI, ScopeLocal, ScopeProject, ScopeUser:
		return true
	}
	return false
}

// SourceKind names where a value came from.
type SourceKind string

// Source kinds.
const (
	SourceManagedFile   SourceKind = "managed-file"
	SourceManagedDropIn SourceKind = "managed-dropin"
	SourceMDMPlist      SourceKind = "mdm-plist"
	SourceRemote        SourceKind = "remote"
	SourceUser          SourceKind = "user"
	SourceProject       SourceKind = "project"
	SourceLocal         SourceKind = "local"
	SourceCLI           SourceKind = "cli"
	SourceGlobalConfig  SourceKind = "global-config"
	SourcePlugin        SourceKind = "plugin"
	SourceBuiltin       SourceKind = "builtin"
)

// IsKnown reports a documented source kind. cost: Pure.
func (k SourceKind) IsKnown() bool {
	switch k {
	case SourceManagedFile, SourceManagedDropIn, SourceMDMPlist, SourceRemote, SourceUser, SourceProject, SourceLocal,
		SourceCLI, SourceGlobalConfig, SourcePlugin, SourceBuiltin:
		return true
	}
	return false
}

// Source names where a value came from, for diagnostics.
type Source struct {
	Kind SourceKind
	Path string
}

// Effort is an effort level (open set: help + docs).
type Effort string

// Known effort levels.
const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
	EffortXHigh  Effort = "xhigh"
	EffortMax    Effort = "max"
)

// IsKnown reports a documented effort level. cost: Pure.
func (e Effort) IsKnown() bool {
	switch e {
	case EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax:
		return true
	}
	return false
}

// UpdateChannel is autoUpdatesChannel.
type UpdateChannel string

// Update channels (help + disk).
const (
	ChannelLatest UpdateChannel = "latest"
	ChannelStable UpdateChannel = "stable"
)

// IsKnown reports a documented channel. cost: Pure.
func (c UpdateChannel) IsKnown() bool { return c == ChannelLatest || c == ChannelStable }

// ModelSetting is one entry of modelSettings.
type ModelSetting struct {
	EffortLevel Effort          `json:"effortLevel,omitempty"`
	Unknown     agentio.Unknown `json:"-"`
}

// StatusLine is the statusLine setting.
type StatusLine struct {
	Type    string          `json:"type"`
	Command string          `json:"command"`
	Padding *int            `json:"padding,omitempty"`
	Unknown agentio.Unknown `json:"-"`
}

// Attribution is the attribution setting.
type Attribution struct {
	Commit     *string         `json:"commit,omitempty"`
	PR         *string         `json:"pr,omitempty"`
	SessionURL *bool           `json:"sessionUrl,omitempty"`
	Unknown    agentio.Unknown `json:"-"`
}

// Sandbox is the sandbox setting: the switches the library reports; every
// nested filesystem/network/credentials key stays raw in Unknown.
type Sandbox struct {
	Enabled                  *bool           `json:"enabled,omitempty"`
	FailIfUnavailable        *bool           `json:"failIfUnavailable,omitempty"`
	AutoAllowBashIfSandboxed *bool           `json:"autoAllowBashIfSandboxed,omitempty"`
	AllowUnsandboxedCommands *bool           `json:"allowUnsandboxedCommands,omitempty"`
	ExcludedCommands         []string        `json:"excludedCommands,omitempty"`
	Unknown                  agentio.Unknown `json:"-"`
}

// WorktreeSettings is the worktree setting.
type WorktreeSettings struct {
	BaseRef            string          `json:"baseRef,omitempty"`
	BgIsolation        string          `json:"bgIsolation,omitempty"`
	Location           string          `json:"location,omitempty"`
	SparsePaths        []string        `json:"sparsePaths,omitempty"`
	SymlinkDirectories []string        `json:"symlinkDirectories,omitempty"`
	Unknown            agentio.Unknown `json:"-"`
}

// MarketplaceDecl is one extraKnownMarketplaces entry.
type MarketplaceDecl struct {
	Source  MarketplaceSource `json:"source"`
	Unknown agentio.Unknown   `json:"-"`
}

// Settings is one settings file, typed for the keys the library reads
// (OD-17); every other key is kept in Unknown. Presence-tracked: absent
// scalars are nil pointers or empty strings, absent lists and maps nil.
type Settings struct {
	Schema                            string                     `json:"$schema,omitempty"`
	Model                             string                     `json:"model,omitempty"`
	FallbackModel                     []string                   `json:"fallbackModel,omitempty"`
	EffortLevel                       Effort                     `json:"effortLevel,omitempty"`
	MaxEffortLevel                    Effort                     `json:"maxEffortLevel,omitempty"`
	ModelSettings                     map[string]ModelSetting    `json:"modelSettings,omitempty"`
	AvailableModels                   []string                   `json:"availableModels,omitempty"`
	Permissions                       *Permissions               `json:"permissions,omitempty"`
	Hooks                             Hooks                      `json:"hooks,omitempty"`
	DisableAllHooks                   *bool                      `json:"disableAllHooks,omitempty"`
	StatusLine                        *StatusLine                `json:"statusLine,omitempty"`
	Env                               map[string]string          `json:"env,omitempty"`
	EnabledPlugins                    map[PluginID]bool          `json:"enabledPlugins,omitempty"`
	ExtraKnownMarketplaces            map[string]MarketplaceDecl `json:"extraKnownMarketplaces,omitempty"`
	EnabledMcpjsonServers             []string                   `json:"enabledMcpjsonServers,omitempty"`
	DisabledMcpjsonServers            []string                   `json:"disabledMcpjsonServers,omitempty"`
	EnableAllProjectMcpServers        *bool                      `json:"enableAllProjectMcpServers,omitempty"`
	OutputStyle                       string                     `json:"outputStyle,omitempty"`
	AutoUpdatesChannel                UpdateChannel              `json:"autoUpdatesChannel,omitempty"`
	MinimumVersion                    string                     `json:"minimumVersion,omitempty"`
	CleanupPeriodDays                 *int                       `json:"cleanupPeriodDays,omitempty"`
	APIKeyHelper                      string                     `json:"apiKeyHelper,omitempty"`
	Theme                             string                     `json:"theme,omitempty"`
	TUI                               string                     `json:"tui,omitempty"`
	EditorMode                        string                     `json:"editorMode,omitempty"`
	Verbose                           *bool                      `json:"verbose,omitempty"`
	SkipDangerousModePermissionPrompt *bool                      `json:"skipDangerousModePermissionPrompt,omitempty"`
	AutoMemoryEnabled                 *bool                      `json:"autoMemoryEnabled,omitempty"`
	AutoMemoryDirectory               string                     `json:"autoMemoryDirectory,omitempty"`
	ClaudeMdExcludes                  []string                   `json:"claudeMdExcludes,omitempty"`
	Agent                             string                     `json:"agent,omitempty"`
	Sandbox                           *Sandbox                   `json:"sandbox,omitempty"`
	Attribution                       *Attribution               `json:"attribution,omitempty"`
	Worktree                          *WorktreeSettings          `json:"worktree,omitempty"`
	Unknown                           agentio.Unknown            `json:"-"`
	// Warnings lists keys whose type changed (invariant 17); never a value.
	Warnings agentio.Warnings `json:"-"`
}

// ParseSettings decodes one settings body; a key whose type changed lands
// in Unknown with a warning, the rest still decodes. cost: Pure.
func ParseSettings(file string, data []byte) (Settings, error) {
	var s Settings
	unknown, warns, err := decodeLenient(file, data, reflect.TypeFor[Settings](), func(b []byte) error {
		s = Settings{}
		return json.Unmarshal(b, &s)
	})
	if err != nil {
		return Settings{}, err
	}
	s.Unknown, s.Warnings = unknown, warns
	return s, nil
}

// SettingsLayer is one layer of the hierarchy.
type SettingsLayer struct {
	Scope    SettingsScope
	Source   Source
	Present  bool
	Settings Settings
	// Err is a parse error kept per layer, like Claude's "Settings Warning".
	Err error
	raw map[string]json.RawMessage
}

// EffectiveSettings is the hierarchy and its merge.
type EffectiveSettings struct {
	Layers []SettingsLayer // highest first
	Merged Settings
	// Origin maps each top-level key to the layer that set it (the highest
	// one for keys that merge).
	Origin map[string]SettingsScope
}

// readLayer reads one settings file as a layer; a missing file is a layer
// with Present false.
func readLayer(scope SettingsScope, kind SourceKind, path string) SettingsLayer {
	l := SettingsLayer{Scope: scope, Source: Source{Kind: kind, Path: path}}
	data, found, err := readFileBounded(path, maxJSONFileBytes)
	if err != nil {
		l.Err = err
		return l
	}
	if !found {
		return l
	}
	l.Present = true
	l.Settings, l.Err = ParseSettings(path, data)
	if l.Err == nil {
		l.Err = json.Unmarshal(data, &l.raw)
	}
	return l
}

// UserSettings reads <home>/settings.json. cost: Disk · Read.
func (a *Agent) UserSettings(h Home) (SettingsLayer, error) {
	l := readLayer(ScopeUser, SourceUser, filepath.Join(h.Dir(), settingsFile))
	return l, l.Err
}

// ProjectSettings reads <project>/.claude/settings.json. cost: Disk · Read.
func (a *Agent) ProjectSettings(p Project) (SettingsLayer, error) {
	l := readLayer(ScopeProject, SourceProject, filepath.Join(p.Dir, dotClaudeDir, settingsFile))
	return l, l.Err
}

// LocalSettings reads .claude/settings.local.json at the repo root (start
// dir outside git). cost: Disk · Read.
func (a *Agent) LocalSettings(p Project) (SettingsLayer, error) {
	l := readLayer(ScopeLocal, SourceLocal, filepath.Join(p.localRoot(), dotClaudeDir, localSettingsFile))
	if !l.Present && p.RepoRoot != "" && p.RepoRoot != p.Dir {
		// Versions before 2.1.211 kept the file in the start dir; still read.
		old := readLayer(ScopeLocal, SourceLocal, filepath.Join(p.Dir, dotClaudeDir, localSettingsFile))
		if old.Present {
			return old, old.Err
		}
	}
	return l, l.Err
}

// ManagedSettings reads the remote cache, then the managed drop-in dir
// (sorted) and file under SystemPaths. Only present layers are returned; a
// Host without system locations returns only the remote cache.
// cost: Disk · Read.
func (a *Agent) ManagedSettings(h Home) ([]SettingsLayer, error) {
	sp := a.SystemPaths()
	var out []SettingsLayer
	var errs []error
	if l := readLayer(ScopeManaged, SourceRemote, filepath.Join(h.Dir(), remoteSettingsFile)); l.Present && len(l.raw) > 0 {
		out = append(out, l)
		errs = append(errs, l.Err)
	}
	if sp.ManagedDir == "" {
		return out, errors.Join(errs...)
	}
	if entries, err := os.ReadDir(sp.ManagedDropIn); err == nil {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), jsonExt) {
				names = append(names, e.Name())
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(names))) // later drop-ins win, so they come first
		for _, n := range names {
			l := readLayer(ScopeManaged, SourceManagedDropIn, filepath.Join(sp.ManagedDropIn, n))
			out = append(out, l)
			errs = append(errs, l.Err)
		}
	}
	if l := readLayer(ScopeManaged, SourceManagedFile, sp.ManagedSettings); l.Present {
		out = append(out, l)
		errs = append(errs, l.Err)
	}
	return out, errors.Join(errs...)
}

// RemoteSettings reads the server-managed cache and its sync time.
// cost: Disk · Read.
func (a *Agent) RemoteSettings(h Home) (SettingsLayer, time.Time, error) {
	p := filepath.Join(h.Dir(), remoteSettingsFile)
	l := readLayer(ScopeManaged, SourceRemote, p)
	var synced time.Time
	if data, found, err := readFileBounded(p+syncedAtSuffix, maxJSONFileBytes); err == nil && found {
		var ms millis
		if json.Unmarshal([]byte(strings.TrimSpace(string(data))), &ms) == nil {
			synced = ms.Time()
		}
	}
	return l, synced, l.Err
}

// Settings reads every layer for h (and p when set) plus cli (a --settings
// value the caller parsed) and merges them per the docs rules: lists merge
// across layers except fallbackModel and availableModels, objects env /
// enabledPlugins / modelSettings / extraKnownMarketplaces / hooks /
// permissions merge per key, everything else is won by the highest layer.
// A layer that fails to parse is kept with Err and skipped in the merge.
// cost: Disk · Read.
func (a *Agent) Settings(h Home, p *Project, cli *Settings) (EffectiveSettings, error) {
	var layers []SettingsLayer
	managed, _ := a.ManagedSettings(h) // per-layer errors stay on each layer
	layers = append(layers, managed...)
	if cli != nil {
		raw, err := json.Marshal(cli)
		l := SettingsLayer{Scope: ScopeCLI, Source: Source{Kind: SourceCLI}, Present: true, Settings: *cli, Err: err}
		if err == nil {
			l.Err = json.Unmarshal(raw, &l.raw)
		}
		layers = append(layers, l)
	}
	if p != nil {
		loc, _ := a.LocalSettings(*p)
		proj, _ := a.ProjectSettings(*p)
		layers = append(layers, loc, proj)
	}
	user, _ := a.UserSettings(h)
	layers = append(layers, user)

	merged := map[string]json.RawMessage{}
	origin := map[string]SettingsScope{}
	for i := len(layers) - 1; i >= 0; i-- { // lowest first, so higher layers override
		l := layers[i]
		if !l.Present || l.Err != nil {
			continue
		}
		for k, v := range l.raw {
			merged[k] = mergeKey(k, merged[k], v)
			origin[k] = l.Scope
		}
	}
	body, err := json.Marshal(merged)
	if err != nil {
		return EffectiveSettings{Layers: layers}, err
	}
	m, err := ParseSettings("merged settings", body)
	return EffectiveSettings{Layers: layers, Merged: m, Origin: origin}, err
}

// Settings keys whose merge is not "highest wins".
const (
	keyPermissions     = "permissions"
	keyHooks           = "hooks"
	keyEnv             = "env"
	keyEnabledPlugins  = "enabledPlugins"
	keyModelSettings   = "modelSettings"
	keyMarketplaces    = "extraKnownMarketplaces"
	keyFallbackModel   = "fallbackModel"
	keyAvailableModels = "availableModels"
)

// Keys where a stricter value in any layer wins over a higher layer's
// (docs settings: "a stricter lower value wins").
const (
	keyMaxEffortLevel      = "maxEffortLevel"
	keyCrossSessionInbound = "crossSessionInbound"
	keyAutoMode            = "autoMode"
	keyClassifyAllShell    = "classifyAllShell"
	inboundAccept          = "accept"
	inboundHold            = "hold"
	inboundRefuse          = "refuse"
)

// strictTopLevel maps each strict boolean key to its stricter value.
func strictTopLevel() map[string]bool {
	return map[string]bool{
		"disableClaudeAiConnectors": true, "isolatePeerMachines": true,
		"enableArtifact": false, "remoteControlAtStartup": false, "useAutoModeDuringPlan": false,
		"syncClaudeAiSkills": false, "syncClaudeAiPlugins": false,
	}
}

// strictPermissionKeys maps the strict booleans inside permissions.
func strictPermissionKeys() map[string]bool {
	return map[string]bool{
		"blockReadsOutsideWorkingDirectories": true, "blockWritesOutsideWorkingDirectories": true, "denyInsteadOfPrompting": true,
	}
}

// strictBools builds a mergeObjects strict hook from a key → stricter map.
func strictBools(keys map[string]bool) func(string, json.RawMessage, json.RawMessage) (json.RawMessage, bool) {
	return func(k string, lo, hi json.RawMessage) (json.RawMessage, bool) {
		strict, ok := keys[k]
		if !ok || lo == nil {
			return nil, false
		}
		return stricterBool(lo, hi, strict), true
	}
}

// stricterBool returns whichever side holds the stricter value; a non-bool
// side makes the higher layer win.
func stricterBool(lower, higher json.RawMessage, strict bool) json.RawMessage {
	var lo, hi bool
	if json.Unmarshal(lower, &lo) != nil || json.Unmarshal(higher, &hi) != nil {
		return higher
	}
	if lo == strict {
		return lower
	}
	return higher
}

// effortRank orders efforts for maxEffortLevel; unknown = -1 (higher wins).
func effortRank(e Effort) int {
	return slices.Index([]Effort{EffortLow, EffortMedium, EffortHigh, EffortXHigh, EffortMax}, e)
}

// lowerEffort keeps the lower maxEffortLevel.
func lowerEffort(lower, higher json.RawMessage) json.RawMessage {
	var lo, hi Effort
	if json.Unmarshal(lower, &lo) != nil || json.Unmarshal(higher, &hi) != nil {
		return higher
	}
	if l, h := effortRank(lo), effortRank(hi); l >= 0 && h >= 0 && l < h {
		return lower
	}
	return higher
}

// stricterInbound orders crossSessionInbound refuse > hold > accept.
func stricterInbound(lower, higher json.RawMessage) json.RawMessage {
	var lo, hi string
	if json.Unmarshal(lower, &lo) != nil || json.Unmarshal(higher, &hi) != nil {
		return higher
	}
	order := []string{inboundAccept, inboundHold, inboundRefuse}
	if l, h := slices.Index(order, lo), slices.Index(order, hi); l > h && h >= 0 {
		return lower
	}
	return higher
}

// permissionListKeys merge across layers inside permissions.
func permissionListKeys() []string { return []string{"allow", "ask", "deny", "additionalDirectories"} }

// mergeKey merges one top-level key: lower is the merged value so far (may
// be nil), higher the next layer's.
func mergeKey(key string, lower, higher json.RawMessage) json.RawMessage {
	if lower == nil {
		return higher
	}
	switch key {
	case keyEnv, keyEnabledPlugins, keyModelSettings, keyMarketplaces:
		return mergeObjects(lower, higher, nil, nil)
	case keyHooks:
		return mergeObjects(lower, higher, func(string) bool { return true }, nil)
	case keyPermissions:
		lists := permissionListKeys()
		return mergeObjects(lower, higher, func(k string) bool { return slices.Contains(lists, k) }, strictBools(strictPermissionKeys()))
	case keyFallbackModel, keyAvailableModels:
		return higher
	case keyMaxEffortLevel:
		return lowerEffort(lower, higher)
	case keyCrossSessionInbound:
		return stricterInbound(lower, higher)
	case keyAutoMode:
		return mergeObjects(lower, higher, nil, strictBools(map[string]bool{keyClassifyAllShell: true}))
	}
	if strict, ok := strictTopLevel()[key]; ok {
		return stricterBool(lower, higher, strict)
	}
	if l, ok := concatLists(lower, higher); ok {
		return l
	}
	return higher
}

// mergeObjects merges two JSON objects per key: higher wins, except keys
// for which concat returns true, whose lists are concatenated (lower first,
// duplicates dropped), and keys strict merges (stricter value wins). A
// non-object side makes higher win outright.
func mergeObjects(lower, higher json.RawMessage, concat func(string) bool, strict func(k string, lo, hi json.RawMessage) (json.RawMessage, bool)) json.RawMessage {
	var lo, hi map[string]json.RawMessage
	if json.Unmarshal(lower, &lo) != nil || json.Unmarshal(higher, &hi) != nil {
		return higher
	}
	for k, v := range hi {
		if strict != nil {
			if m, ok := strict(k, lo[k], v); ok {
				lo[k] = m
				continue
			}
		}
		if concat != nil && concat(k) {
			if l, ok := concatLists(lo[k], v); ok {
				lo[k] = l
				continue
			}
		}
		lo[k] = v
	}
	out, err := json.Marshal(lo)
	if err != nil {
		return higher
	}
	return out
}

// concatLists joins two JSON arrays, dropping exact duplicates; ok is false
// when either side is not an array.
func concatLists(lower, higher json.RawMessage) (json.RawMessage, bool) {
	if lower == nil {
		return higher, true
	}
	var lo, hi []json.RawMessage
	if json.Unmarshal(lower, &lo) != nil || json.Unmarshal(higher, &hi) != nil {
		return nil, false
	}
	seen := map[string]bool{}
	var out []json.RawMessage
	for _, v := range append(lo, hi...) {
		if !seen[string(v)] {
			seen[string(v)] = true
			out = append(out, v)
		}
	}
	b, err := json.Marshal(out)
	return b, err == nil
}

// SettingsPatch changes top-level keys of one settings file.
type SettingsPatch struct {
	Set    map[string]agentio.Raw
	Delete []string
}

// settingsPath resolves the file a scope writes to.
func (a *Agent) settingsPath(h Home, p *Project, s SettingsScope) (string, error) {
	switch s {
	case ScopeUser:
		return filepath.Join(h.Dir(), settingsFile), nil
	case ScopeProject, ScopeLocal:
		if p == nil {
			return "", fmt.Errorf("%w: %s settings need a project", agentio.ErrUnsupported, s)
		}
		if s == ScopeProject {
			return filepath.Join(p.Dir, dotClaudeDir, settingsFile), nil
		}
		return filepath.Join(p.localRoot(), dotClaudeDir, localSettingsFile), nil
	}
	return "", agentio.UnsupportedError(ID, "write "+string(s)+" settings", "only user, project and local settings files are writable")
}
