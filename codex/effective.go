package codex

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ubgo/agentio"
)

// LayerKind names a config layer (config/src/config_layer_source.rs).
type LayerKind string

// LayerKind values, lowest precedence first.
const (
	LayerPackaged      LayerKind = "packaged"
	LayerMDM           LayerKind = "mdm"
	LayerSystem        LayerKind = "system"
	LayerCloud         LayerKind = "cloud"
	LayerUser          LayerKind = "user"
	LayerLegacyProfile LayerKind = "legacy-profile"
	LayerUserProfile   LayerKind = "user-profile"
	LayerProject       LayerKind = "project"
	LayerSession       LayerKind = "session"
	LayerLegacyManaged LayerKind = "legacy-managed"
)

// IsKnown reports a known layer kind. Pure.
func (k LayerKind) IsKnown() bool {
	switch k {
	case LayerPackaged, LayerMDM, LayerSystem, LayerCloud, LayerUser, LayerLegacyProfile, LayerUserProfile,
		LayerProject, LayerSession, LayerLegacyManaged:
		return true
	}
	return false
}

// System config paths (unix; loader/mod.rs).
const (
	systemConfigPath  = "/etc/codex/config.toml"
	managedConfigPath = "/etc/codex/managed_config.toml"
	// reasonUntrusted marks a project layer Codex ignores.
	reasonUntrusted = "folder is not trusted"
	reasonNotRead   = "not readable by the library"
)

// Layer is one config source in precedence order.
type Layer struct {
	Kind     LayerKind
	File     string
	Exists   bool
	Disabled string // why Codex ignores it ("folder is not trusted"); "" = applied
}

// EffectiveQuery selects the layers Effective merges.
type EffectiveQuery struct {
	Dir           string     // cwd for project layers and trust
	Profile       string     // v2 profile name (`-p`)
	Overrides     []Override // the -c list the caller will pass
	IncludeSystem bool       // read /etc/codex/config.toml and managed_config.toml
}

// EffectiveConfig is the merged config plus where each key came from.
type EffectiveConfig struct {
	Config       Config
	Origins      map[string]LayerKind // dotted key → winning layer
	Layers       []Layer              // low → high
	NotEvaluated []LayerKind          // mdm, cloud: present upstream, not readable here
	Warnings     agentio.Warnings
}

// projectDeniedRoots are keys Codex refuses in project layers
// (loader/mod.rs:89-102).
func projectDeniedRoots() []string {
	return []string{"openai_base_url", "chatgpt_base_url", "apps_mcp_product_sku", "responses_api_metadata",
		"model_provider", "model_providers", "notify", "profile", "profiles", "otel"}
}

// realtime URL keys are denied by prefix/suffix.
const (
	deniedRealtimePrefix = "experimental_realtime_"
	deniedRealtimeSuffix = "_url"
)

func projectDenied(root string) bool {
	if strings.HasPrefix(root, deniedRealtimePrefix) && strings.HasSuffix(root, deniedRealtimeSuffix) {
		return true
	}
	for _, d := range projectDeniedRoots() {
		if d == root {
			return true
		}
	}
	return false
}

// layerEntries is one parsed layer.
type layerEntries struct {
	layer   Layer
	entries []tomlEntry
}

func readLayer(kind LayerKind, path string) (layerEntries, error) {
	le := layerEntries{layer: Layer{Kind: kind, File: path}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return le, nil
	}
	if err != nil {
		return le, fmt.Errorf("codex: read %s: %w", path, err)
	}
	le.layer.Exists = true
	entries, err := parseTOML(b)
	if err != nil {
		return le, fmt.Errorf("codex: %s: %w", path, err)
	}
	le.entries = entries
	return le, nil
}

// layers resolves the ordered layer list for q (files read once).
func (a *Agent) layers(h Home, q EffectiveQuery) ([]layerEntries, agentio.Warnings, error) {
	p := a.Paths(h)
	var out []layerEntries
	if q.IncludeSystem {
		le, err := readLayer(LayerSystem, systemConfigPath)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, le)
	}
	user, err := readLayer(LayerUser, p.Config)
	if err != nil {
		return nil, nil, err
	}
	out = append(out, user)
	userCfg, warns, _ := decodeEntries(user.entries, p.Config)
	if q.Profile == "" && userCfg.Profile.Set {
		// legacy selector: root `profile = "X"` applies [profiles.X] on top of the root
		name := userCfg.Profile.V
		le := layerEntries{layer: Layer{Kind: LayerLegacyProfile, File: p.Config, Exists: true}}
		for _, e := range user.entries {
			if len(e.path) >= 3 && e.path[0] == keyProfiles && e.path[1] == name {
				le.entries = append(le.entries, tomlEntry{path: e.path[2:], val: e.val, line: e.line})
			}
		}
		out = append(out, le)
	}
	if q.Profile != "" {
		le, err := readLayer(LayerUserProfile, p.ProfileConfig(q.Profile))
		if err != nil {
			return nil, nil, err
		}
		out = append(out, le)
	}
	if q.Dir != "" {
		trusted := false
		if t, ok := userCfg.TrustOf(q.Dir); ok && t == TrustTrusted {
			trusted = true
		}
		root, _ := projectRoot(q.Dir, userCfg)
		for _, d := range dirsRootToCwd(root, q.Dir) {
			path := filepath.Join(d, dirProjectCodex, fileConfig)
			le, err := readLayer(LayerProject, path)
			if err != nil {
				return nil, nil, err
			}
			if !le.layer.Exists {
				continue
			}
			if !trusted {
				le.layer.Disabled, le.entries = reasonUntrusted, nil
			} else {
				kept := le.entries[:0]
				for _, e := range le.entries {
					if !projectDenied(e.path[0]) {
						kept = append(kept, e)
					}
				}
				le.entries = kept
			}
			out = append(out, le)
		}
	}
	if len(q.Overrides) > 0 {
		le := layerEntries{layer: Layer{Kind: LayerSession, Exists: true}}
		for _, ov := range q.Overrides {
			// a -c path is already TOML dotted-key syntax (possibly quoted)
			src := []byte(ov.Path + " = " + string(ov.Value))
			entries, err := parseTOML(src)
			if err != nil {
				return nil, nil, fmt.Errorf("codex: override %s: %w", ov.Path, err)
			}
			le.entries = append(le.entries, entries...)
		}
		out = append(out, le)
	}
	if q.IncludeSystem {
		le, err := readLayer(LayerLegacyManaged, managedConfigPath)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, le)
	}
	return out, warns, nil
}

func decodeEntries(entries []tomlEntry, file string) (Config, agentio.Warnings, error) {
	c, warns := decodeEntriesInto(entries, file)
	return c, warns, nil
}

// Effective merges the layers Codex would apply for q: system (opt-in),
// user config.toml, the legacy [profiles.X] selected by root `profile`
// (only without a v2 profile), the v2 profile file, trusted project
// layers from the project root down to q.Dir (denied keys dropped), and
// -c overrides. Later layers win per key; tables merge. MDM and cloud
// layers are never read (NotEvaluated). Disk · Read.
func (a *Agent) Effective(h Home, q EffectiveQuery) (EffectiveConfig, error) {
	layers, warns, err := a.layers(h, q)
	if err != nil {
		return EffectiveConfig{}, err
	}
	out := EffectiveConfig{Origins: map[string]LayerKind{}, NotEvaluated: []LayerKind{LayerMDM, LayerCloud}, Warnings: warns}
	order := []string{}
	merged := map[string]tomlEntry{}
	for _, le := range layers {
		out.Layers = append(out.Layers, le.layer)
		for _, e := range le.entries {
			k := e.key()
			if _, seen := merged[k]; !seen {
				order = append(order, k)
			}
			merged[k] = e
			out.Origins[k] = le.layer.Kind
		}
	}
	entries := make([]tomlEntry, 0, len(order))
	for _, k := range order {
		entries = append(entries, merged[k])
	}
	cfg, w, _ := decodeEntries(entries, "")
	out.Config = cfg
	out.Warnings = append(out.Warnings, w...)
	return out, nil
}

// ConfigLayers lists the files that apply to h and dir in precedence
// order, including disabled (untrusted) project layers. Disk · Read.
func (a *Agent) ConfigLayers(h Home, dir string) ([]Layer, error) {
	layers, _, err := a.layers(h, EffectiveQuery{Dir: dir, IncludeSystem: true})
	if err != nil {
		return nil, err
	}
	out := make([]Layer, 0, len(layers)+2)
	for _, le := range layers {
		out = append(out, le.layer)
	}
	return out, nil
}

// ProfileFiles lists the v2 profile names: <home>/<name>.config.toml.
// Disk · Read.
func (a *Agent) ProfileFiles(h Home) ([]string, error) {
	entries, err := os.ReadDir(h.Dir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("codex: read %s: %w", h.Dir(), err)
	}
	var out []string
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), fileProfileSuffix)
		if !ok || name == "" || e.IsDir() {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// MCPServerEntry is one effective MCP server with its name and source.
type MCPServerEntry struct {
	Name   string
	Layer  LayerKind
	Source string // file it came from
	MCPServer
}

// Neutral maps onto agentio.MCPServer (env names only). Pure.
func (s MCPServerEntry) Neutral() agentio.MCPServer {
	out := agentio.MCPServer{
		Name: s.Name, Transport: s.Transport(), Scope: layerScope(s.Layer),
		Command: s.Command.V, Args: s.Args, URL: s.URL.V, EnvNames: s.EnvKeys, Source: s.Source,
	}
	for _, v := range s.EnvVars {
		addName(&out.EnvNames, v.Name)
	}
	if s.Enabled.Set {
		en := s.Enabled.V
		out.Enabled = &en
	}
	return out
}

func layerScope(k LayerKind) agentio.ConfigScope {
	switch k {
	case LayerSystem, LayerMDM, LayerCloud, LayerLegacyManaged:
		return agentio.ConfigManaged
	case LayerUser:
		return agentio.ConfigUser
	case LayerUserProfile, LayerLegacyProfile:
		return agentio.ConfigProfile
	case LayerProject:
		return agentio.ConfigProject
	case LayerSession:
		return agentio.ConfigCLI
	}
	return agentio.ConfigScopeUnknown
}

// MCPServers lists the effective [mcp_servers] for dir (user config plus
// trusted project layers), sorted by name. Listing never spawns a server.
// Disk · Read.
func (a *Agent) MCPServers(h Home, dir string) ([]MCPServerEntry, error) {
	eff, err := a.Effective(h, EffectiveQuery{Dir: dir})
	if err != nil {
		return nil, err
	}
	files := map[LayerKind]string{}
	for _, l := range eff.Layers {
		if l.Exists && l.Disabled == "" {
			files[l.Kind] = l.File
		}
	}
	var out []MCPServerEntry
	for name, s := range eff.Config.MCPServers {
		layer := LayerUser
		prefix := dottedKey([]string{keyMCPServers, name}) + "."
		for k, kind := range eff.Origins {
			if strings.HasPrefix(k, prefix) {
				layer = kind
				break
			}
		}
		out = append(out, MCPServerEntry{Name: name, Layer: layer, Source: files[layer], MCPServer: s})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ErrNoEnvValue means the server or env key is not in config.toml.
var ErrNoEnvValue = errors.New("codex: mcp server env key not found")

// MCPServerEnvValue reads one mcp_servers.<server>.env.<key> value from
// <home>/config.toml as a redacting EnvValue (never logged or printed).
// Disk · Read.
func (a *Agent) MCPServerEnvValue(h Home, server, key string) (agentio.EnvValue, error) {
	b, err := os.ReadFile(a.Paths(h).Config)
	if err != nil {
		return agentio.EnvValue{}, fmt.Errorf("codex: read %s: %w", fileConfig, err)
	}
	entries, err := parseTOML(b)
	if err != nil {
		return agentio.EnvValue{}, err
	}
	for _, e := range entries {
		if len(e.path) == 4 && e.path[0] == keyMCPServers && e.path[1] == server && e.path[2] == "env" && e.path[3] == key && e.val.kind == tomlString {
			return agentio.NewEnvValue(e.val.str), nil
		}
	}
	return agentio.EnvValue{}, fmt.Errorf("%w: %s.%s", ErrNoEnvValue, server, key)
}
