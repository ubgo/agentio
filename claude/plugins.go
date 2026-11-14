package claude

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// PluginID is "<plugin>@<marketplace>".
type PluginID string

// Split returns the plugin and marketplace parts. cost: Pure.
func (id PluginID) Split() (plugin, marketplace string) {
	p, m, _ := strings.Cut(string(id), pluginIDSep)
	return p, m
}

// MarketplaceSourceKind is a marketplace source type (github verified; rest docs).
type MarketplaceSourceKind string

// Marketplace source kinds.
const (
	MarketplaceGitHub    MarketplaceSourceKind = "github"
	MarketplaceGit       MarketplaceSourceKind = "git"
	MarketplaceURL       MarketplaceSourceKind = "url"
	MarketplaceDirectory MarketplaceSourceKind = "directory"
	MarketplaceCommand   MarketplaceSourceKind = "command"
)

// IsKnown reports a documented kind. cost: Pure.
func (k MarketplaceSourceKind) IsKnown() bool {
	switch k {
	case MarketplaceGitHub, MarketplaceGit, MarketplaceURL, MarketplaceDirectory, MarketplaceCommand:
		return true
	}
	return false
}

// MarketplaceSource is where a marketplace comes from.
type MarketplaceSource struct {
	Kind    MarketplaceSourceKind `json:"source"`
	Repo    string                `json:"repo,omitempty"`
	URL     string                `json:"url,omitempty"`
	Path    string                `json:"path,omitempty"`
	Unknown agentio.Unknown       `json:"-"`
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (s *MarketplaceSource) UnmarshalJSON(b []byte) error {
	type plain MarketplaceSource
	var v plain
	u, _, err := decodeLenient("source", b, reflect.TypeFor[plain](), func(d []byte) error { v = plain{}; return json.Unmarshal(d, &v) })
	*s = MarketplaceSource(v)
	s.Unknown = u
	return err
}

// InstalledPlugin is one installed_plugins.json record, with enablement
// from the merged settings.
type InstalledPlugin struct {
	ID          PluginID
	Scope       PluginScope
	InstallPath string
	Version     string
	InstalledAt time.Time
	LastUpdated time.Time
	// Enabled: enabledPlugins says true in the merged settings.
	Enabled bool
	// ProjectEnabled: a project or local layer enables it.
	ProjectEnabled bool
	DataDir        string
	Unknown        agentio.Unknown
}

// installedPluginsVersion is the only installed_plugins.json format recorded.
const installedPluginsVersion = 2

type wireInstalledPlugins struct {
	Version int                            `json:"version"`
	Plugins map[PluginID][]json.RawMessage `json:"plugins"`
}

type wireInstallRecord struct {
	Scope       PluginScope `json:"scope"`
	InstallPath string      `json:"installPath"`
	Version     string      `json:"version"`
	InstalledAt time.Time   `json:"installedAt"`
	LastUpdated time.Time   `json:"lastUpdated"`
}

// pluginDataName is the per-plugin data dir name: <plugin>-<marketplace>.
func pluginDataName(id PluginID) string {
	p, m := id.Split()
	if m == "" {
		return p
	}
	return p + "-" + m
}

// Plugins lists installed plugins (installed_plugins.json version 2) with
// Enabled from the merged enabledPlugins. Another file version is
// ErrUnknownVersion. cost: Disk · Read.
func (a *Agent) Plugins(h Home, p *Project) ([]InstalledPlugin, error) {
	pp := a.Paths(h).Plugins
	data, found, err := readFileBounded(pp.Installed, maxJSONFileBytes)
	if err != nil || !found {
		return nil, err
	}
	var w wireInstalledPlugins
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("claude: decode %s: %w", pp.Installed, err)
	}
	if w.Version != installedPluginsVersion {
		return nil, fmt.Errorf("%w: %s version %d", agentio.ErrUnknownVersion, pp.Installed, w.Version)
	}
	enabled := map[PluginID]bool{}
	projectEnabled := map[PluginID]bool{}
	if eff, err := a.Settings(h, p, nil); err == nil {
		enabled = eff.Merged.EnabledPlugins
		for _, l := range eff.Layers {
			if l.Scope == ScopeProject || l.Scope == ScopeLocal {
				for id, on := range l.Settings.EnabledPlugins {
					projectEnabled[id] = projectEnabled[id] || on
				}
			}
		}
	}
	ids := make([]PluginID, 0, len(w.Plugins))
	for id := range w.Plugins {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var out []InstalledPlugin
	for _, id := range ids {
		for _, raw := range w.Plugins[id] {
			var r wireInstallRecord
			unknown, _, err := decodeLenient(installedPluginsFile, raw, reflect.TypeFor[wireInstallRecord](), func(b []byte) error { r = wireInstallRecord{}; return json.Unmarshal(b, &r) })
			if err != nil {
				continue
			}
			out = append(out, InstalledPlugin{
				ID: id, Scope: r.Scope, InstallPath: r.InstallPath, Version: r.Version, InstalledAt: r.InstalledAt, LastUpdated: r.LastUpdated,
				Enabled: enabled[id], ProjectEnabled: projectEnabled[id], DataDir: filepath.Join(pp.Data, pluginDataName(id)), Unknown: unknown,
			})
		}
	}
	return out, nil
}

// Marketplace is one known or declared marketplace.
type Marketplace struct {
	Name            string
	Source          MarketplaceSource
	InstallLocation string
	LastUpdated     time.Time
	// Declared: known only through extraKnownMarketplaces (not cloned yet).
	Declared bool
}

type wireKnownMarketplace struct {
	Source          MarketplaceSource `json:"source"`
	InstallLocation string            `json:"installLocation"`
	LastUpdated     time.Time         `json:"lastUpdated"`
}

// Marketplaces lists known_marketplaces.json plus marketplaces declared in
// user settings extraKnownMarketplaces, sorted by name. cost: Disk · Read.
func (a *Agent) Marketplaces(h Home) ([]Marketplace, error) {
	pp := a.Paths(h).Plugins
	byName := map[string]Marketplace{}
	data, found, err := readFileBounded(pp.KnownMarketplaces, maxJSONFileBytes)
	if err != nil {
		return nil, err
	}
	if found {
		var w map[string]wireKnownMarketplace
		if err := json.Unmarshal(data, &w); err != nil {
			return nil, fmt.Errorf("claude: decode %s: %w", pp.KnownMarketplaces, err)
		}
		for n, m := range w {
			byName[n] = Marketplace{Name: n, Source: m.Source, InstallLocation: m.InstallLocation, LastUpdated: m.LastUpdated}
		}
	}
	if l, err := a.UserSettings(h); err == nil {
		for n, d := range l.Settings.ExtraKnownMarketplaces {
			if _, ok := byName[n]; !ok {
				byName[n] = Marketplace{Name: n, Source: d.Source, Declared: true}
			}
		}
	}
	out := make([]Marketplace, 0, len(byName))
	for _, m := range byName {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Blocklist is plugins/blocklist.json.
type Blocklist struct {
	FetchedAt *time.Time
	Plugins   agentio.Raw // entry shape not sampled; kept raw
	Unknown   agentio.Unknown
}

type wireBlocklist struct {
	FetchedAt *time.Time      `json:"fetchedAt"`
	Plugins   json.RawMessage `json:"plugins"`
}

// PluginBlocklist reads plugins/blocklist.json; zero when absent.
// cost: Disk · Read.
func (a *Agent) PluginBlocklist(h Home) (Blocklist, error) {
	p := a.Paths(h).Plugins.Blocklist
	data, found, err := readFileBounded(p, maxJSONFileBytes)
	if err != nil || !found {
		return Blocklist{}, err
	}
	var w wireBlocklist
	unknown, _, err := decodeLenient(blocklistFile, data, reflect.TypeFor[wireBlocklist](), func(b []byte) error { return json.Unmarshal(b, &w) })
	if err != nil {
		return Blocklist{}, err
	}
	return Blocklist{FetchedAt: w.FetchedAt, Plugins: rawOrNil(w.Plugins), Unknown: unknown}, nil
}

// PluginManifest is <plugin>/.claude-plugin/plugin.json. Component fields
// accept a string or a list (docs); shape not recorded on this disk.
type PluginManifest struct {
	Name, Version, Description string
	Skills, Agents, Commands   []string
	Hooks, MCPServers          []string
	OutputStyles               []string
	Unknown                    agentio.Unknown
}

// stringList decodes a JSON string or string array; anything else is nil.
type stringList []string

// UnmarshalJSON accepts "x" or ["x", …]; objects (inline configs) yield nil.
func (s *stringList) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*s = stringList{one}
		return nil
	}
	var many []string
	if json.Unmarshal(b, &many) == nil {
		*s = many
		return nil
	}
	*s = nil
	return nil
}

type wireManifest struct {
	Name         string     `json:"name"`
	Version      string     `json:"version"`
	Description  string     `json:"description"`
	Skills       stringList `json:"skills"`
	Agents       stringList `json:"agents"`
	Commands     stringList `json:"commands"`
	Hooks        stringList `json:"hooks"`
	MCPServers   stringList `json:"mcpServers"`
	OutputStyles stringList `json:"outputStyles"`
}

// PluginManifest reads installPath/.claude-plugin/plugin.json.
// cost: Disk · Read.
func (a *Agent) PluginManifest(installPath string) (PluginManifest, error) {
	p := filepath.Join(installPath, pluginManifestDir, pluginManifestFile)
	data, found, err := readFileBounded(p, maxJSONFileBytes)
	if err != nil {
		return PluginManifest{}, err
	}
	if !found {
		return PluginManifest{}, fmt.Errorf("%w: %s", agentio.ErrNotFound, p)
	}
	var w wireManifest
	unknown, _, err := decodeLenient(pluginManifestFile, data, reflect.TypeFor[wireManifest](), func(b []byte) error { return json.Unmarshal(b, &w) })
	if err != nil {
		return PluginManifest{}, err
	}
	return PluginManifest{Name: w.Name, Version: w.Version, Description: w.Description, Skills: w.Skills, Agents: w.Agents,
		Commands: w.Commands, Hooks: w.Hooks, MCPServers: w.MCPServers, OutputStyles: w.OutputStyles, Unknown: unknown}, nil
}
