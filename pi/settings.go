package pi

import (
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"reflect"

	"github.com/ubgo/agentio"
)

// ProjectTrust is settings defaultProjectTrust (global only): what pi does
// with project-local resources it has no saved decision for.
type ProjectTrust string

// ProjectTrust values (PKG/schemas/settings.schema.json; default "ask").
const (
	TrustAsk    ProjectTrust = "ask"
	TrustAlways ProjectTrust = "always"
	TrustNever  ProjectTrust = "never"
)

// ProjectTrustValues lists every value. Pure.
func ProjectTrustValues() []ProjectTrust { return []ProjectTrust{TrustAsk, TrustAlways, TrustNever} }

// CacheWarming is settings cacheWarming (global only). "idle", the
// default, keeps the prompt cache warm between runs for up to 30 minutes
// and spends tokens doing so (PKG/docs/settings.md:21).
type CacheWarming string

// CacheWarming values (settings schema).
const (
	CacheWarmingOff       CacheWarming = "off"
	CacheWarmingStreaming CacheWarming = "streaming"
	CacheWarmingIdle      CacheWarming = "idle"
)

// CacheWarmingValues lists every value. Pure.
func CacheWarmingValues() []CacheWarming {
	return []CacheWarming{CacheWarmingOff, CacheWarmingStreaming, CacheWarmingIdle}
}

// Settings is one settings.json (global <agent-dir>/settings.json or
// project .pi/settings.json). The keys agentio uses are typed; every other
// key (pi's schema has additionalProperties: true and about 80 keys) is
// kept raw in Unknown and written back by MarshalJSON, so a round trip
// never drops a key. A modelled key whose value changed type moves to
// Unknown with a warning instead of failing the file.
type Settings struct {
	DefaultProvider        agentio.Opt[string]        `json:"defaultProvider,omitzero"`
	DefaultModel           agentio.Opt[string]        `json:"defaultModel,omitzero"`
	DefaultThinkingLevel   agentio.Opt[ThinkingLevel] `json:"defaultThinkingLevel,omitzero"`
	EnabledModels          []string                   `json:"enabledModels,omitempty"`
	DefaultTools           []string                   `json:"defaultTools,omitempty"`
	DefaultProjectTrust    agentio.Opt[ProjectTrust]  `json:"defaultProjectTrust,omitzero"`
	SessionDir             agentio.Opt[string]        `json:"sessionDir,omitzero"`
	CacheWarming           agentio.Opt[CacheWarming]  `json:"cacheWarming,omitzero"`
	EnableInstallTelemetry agentio.Opt[bool]          `json:"enableInstallTelemetry,omitzero"`
	Theme                  agentio.Opt[string]        `json:"theme,omitzero"`
	LastChangelogVersion   agentio.Opt[string]        `json:"lastChangelogVersion,omitzero"`
	// Packages entries are a source string or an object with resource
	// filters (settings schema: (string | object)[]), kept raw.
	Packages   agentio.Raw `json:"packages,omitempty"`
	Extensions []string    `json:"extensions,omitempty"`
	Skills     []string    `json:"skills,omitempty"`
	Prompts    []string    `json:"prompts,omitempty"`
	Themes     []string    `json:"themes,omitempty"`
	// Unknown are the keys not modelled above, raw.
	Unknown agentio.Unknown `json:"-"`
}

// settingsWire is Settings without its methods, for the plain decoder.
type settingsWire Settings

// MarshalJSON writes the modelled keys merged with Unknown (a modelled
// key wins over an Unknown key of the same name). Pure.
func (s Settings) MarshalJSON() ([]byte, error) {
	known, err := json.Marshal(settingsWire(s))
	if err != nil {
		return nil, err
	}
	if len(s.Unknown) == 0 {
		return known, nil
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(known, &merged); err != nil {
		return nil, err
	}
	out := maps.Clone(map[string]json.RawMessage(s.Unknown))
	maps.Copy(out, merged)
	return json.Marshal(out)
}

// ParseSettings decodes a settings.json body leniently: unknown keys go to
// Unknown, a modelled key of the wrong type goes to Unknown with a
// WarnTypeChanged warning. file only labels warnings and errors. Pure.
func ParseSettings(file string, data []byte) (Settings, agentio.Warnings, error) {
	var w settingsWire
	unknown, warns, err := decodeLenient(file, agentio.StripBOM(data), reflect.TypeFor[settingsWire](), func(b []byte) error {
		w = settingsWire{}
		return json.Unmarshal(b, &w)
	})
	if err != nil {
		return Settings{}, warns, err
	}
	s := Settings(w)
	s.Unknown = unknown
	return s, warns, nil
}

// ReadSettings reads h's settings.json; found is false (zero Settings, nil
// error) when pi has not written it yet, which is the normal state of a
// fresh agent dir. Disk · Read.
func (a *Agent) ReadSettings(h Home) (s Settings, found bool, warns agentio.Warnings, err error) {
	return readSettingsFile(a.Paths(h).Settings)
}

// ReadProjectSettings reads <dir>/.pi/settings.json (pi only applies it
// once the project is trusted). Disk · Read.
func (a *Agent) ReadProjectSettings(dir string) (s Settings, found bool, warns agentio.Warnings, err error) {
	abs, err := absDir(dir)
	if err != nil {
		return Settings{}, false, nil, err
	}
	return readSettingsFile(filepath.Join(abs, projectDirName, settingsFile))
}

// readSettingsFile reads and parses one settings file. Disk · Read.
func readSettingsFile(p string) (Settings, bool, agentio.Warnings, error) {
	data, found, err := readFileBounded(p, maxJSONFileBytes)
	if err != nil {
		return Settings{}, found, nil, fmt.Errorf("pi: read %s: %w", p, err)
	}
	if !found {
		return Settings{}, false, nil, nil
	}
	s, warns, err := ParseSettings(p, data)
	return s, true, warns, err
}

// SettingsKeys lists the settings.json keys this package types; every
// other key round-trips through Unknown. Pure.
func SettingsKeys() []string { return agentio.JSONKeys(reflect.TypeFor[settingsWire]()) }

// ConfigLayers lists the settings files that apply to h and dir, highest
// precedence first: the project .pi/settings.json (applied only once the
// project is trusted, PKG/docs/configuration.md:3), then the agent dir's
// settings.json. pi merges them with the project winning
// (settings-manager.js:19-33). Disk · Read (stat only).
func (a *Agent) ConfigLayers(h Home, dir string) ([]agentio.ConfigLayer, error) {
	var out []agentio.ConfigLayer
	abs, err := absDir(dir)
	if err != nil {
		return nil, err
	}
	if abs != "" {
		p := filepath.Join(abs, projectDirName, settingsFile)
		out = append(out, agentio.ConfigLayer{Scope: agentio.ConfigProject, Path: p, Exists: fileExists(p)})
	}
	p := a.Paths(h).Settings
	out = append(out, agentio.ConfigLayer{Scope: agentio.ConfigUser, Path: p, Exists: fileExists(p)})
	return out, nil
}

// Defaults are the effective model and autonomy for h in dir: the
// project's defaultModel when the project is trusted, else the agent
// dir's. A model is "provider/id" when defaultProvider is set, matching
// what --model accepts. Autonomy is always AutonomyDefault: pi has no
// permission setting. Disk · Read.
func (a *Agent) Defaults(h Home, dir string) (agentio.Defaults, error) {
	user, _, _, err := a.ReadSettings(h)
	if err != nil {
		return agentio.Defaults{}, err
	}
	eff := user
	if dir != "" {
		trust, err := a.ProjectTrust(h, dir)
		if err != nil {
			return agentio.Defaults{}, err
		}
		if trust == agentio.TrustTrusted {
			proj, found, _, err := a.ReadProjectSettings(dir)
			if err != nil {
				return agentio.Defaults{}, err
			}
			if found {
				eff = mergeModel(user, proj)
			}
		}
	}
	return agentio.Defaults{Model: modelOf(eff)}, nil
}

// mergeModel applies the project's provider and model over the user's,
// key by key (pi's deep merge replaces scalars). Pure.
func mergeModel(user, proj Settings) Settings {
	out := user
	if proj.DefaultProvider.Set {
		out.DefaultProvider = proj.DefaultProvider
	}
	if proj.DefaultModel.Set {
		out.DefaultModel = proj.DefaultModel
	}
	return out
}

// modelOf renders the default model as --model accepts it. Pure.
func modelOf(s Settings) string {
	m, ok := s.DefaultModel.Get()
	if !ok || m == "" {
		return ""
	}
	if p, ok := s.DefaultProvider.Get(); ok && p != "" {
		return p + "/" + m
	}
	return m
}
