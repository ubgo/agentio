package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/frontmatter"
)

// Skill and plugin file names (CODEX_API.md §11-§12).
const (
	fileSkillMD         = "SKILL.md"
	dirSystemSkills     = ".system"
	dirProjectCodex     = ".codex"
	filePluginManifest  = ".codex-plugin/plugin.json"
	fileRemoteInstall   = ".codex-remote-plugin-install.json"
	frontmatterMaxBytes = 8 << 10
	frontmatterName     = "name"
	frontmatterDesc     = "description"
	pluginKeySep        = "@"
)

// Codex skill scopes: the neutral ones plus "vendor" (curated imports).
const (
	SkillScopeVendor agentio.SkillScope = "vendor"
)

// Skill is one SKILL.md, front matter only.
type Skill struct {
	Name, Description, Dir string
	Scope                  agentio.SkillScope
	Enabled                bool
	Plugin                 string
	Extra                  map[string]string // other top-level front-matter scalars
}

// Neutral maps onto agentio.Skill. Pure.
func (s Skill) Neutral() agentio.Skill {
	en := s.Enabled
	return agentio.Skill{Name: s.Name, Description: s.Description, Dir: s.Dir, Plugin: s.Plugin, Scope: s.Scope, Enabled: &en}
}

// readFrontMatter parses the front matter of the first 8 KiB of r with
// the root frontmatter package (stdlib only, so no YAML dependency). A block cut by
// the bound or a syntax problem yields an empty Doc, never an error.
func readFrontMatter(r io.Reader) frontmatter.Doc {
	b, err := io.ReadAll(io.LimitReader(r, frontmatterMaxBytes))
	if err != nil {
		return frontmatter.Doc{}
	}
	doc, _, err := frontmatter.Parse(b)
	if err != nil {
		return frontmatter.Doc{}
	}
	return doc
}

func readSkill(dir string, scope agentio.SkillScope, plugin string) (Skill, bool) {
	f, err := os.Open(filepath.Join(dir, fileSkillMD))
	if err != nil {
		return Skill{}, false
	}
	// Read-only file: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	fm := readFrontMatter(f)
	s := Skill{Name: fm.String(frontmatterName), Description: strings.TrimSpace(fm.String(frontmatterDesc)), Dir: dir, Scope: scope, Enabled: true, Plugin: plugin}
	if s.Name == "" {
		s.Name = filepath.Base(dir)
	}
	for _, k := range fm.Keys() {
		if k == frontmatterName || k == frontmatterDesc {
			continue
		}
		if v := fm.String(k); v != "" {
			if s.Extra == nil {
				s.Extra = map[string]string{}
			}
			s.Extra[k] = v
		}
	}
	return s, true
}

// skillsIn lists every <root>/<name>/SKILL.md (one level).
func skillsIn(root string, scope agentio.SkillScope, plugin string) []Skill {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() || e.Name() == dirSystemSkills {
			continue
		}
		if s, ok := readSkill(filepath.Join(root, e.Name()), scope, plugin); ok {
			out = append(out, s)
		}
	}
	return out
}

// Skills lists user skills (<home>/skills), bundled system skills
// (skills/.system), project skills (.codex/skills from dir up to the
// project root), curated vendor imports and skills shipped in installed
// plugins. `[[skills.config]]` entries with enabled=false mark a skill (by
// path or name) disabled. Front matter only (first 8 KiB). Disk · Read.
func (a *Agent) Skills(h Home, dir string) ([]Skill, error) {
	p := a.Paths(h)
	var out []Skill
	out = append(out, skillsIn(p.Skills, agentio.SkillUser, "")...)
	out = append(out, skillsIn(filepath.Join(p.Skills, dirSystemSkills), agentio.SkillSystem, "")...)
	if dir != "" {
		cfg, _, _ := readConfigPath(p.Config)
		root, _ := projectRoot(dir, cfg)
		for _, d := range dirsRootToCwd(root, dir) {
			out = append(out, skillsIn(filepath.Join(d, dirProjectCodex, dirSkills), agentio.SkillProject, "")...)
		}
	}
	out = append(out, skillsIn(p.VendorSkills, SkillScopeVendor, "")...)
	plugins, err := a.Plugins(h)
	if err != nil {
		return nil, err
	}
	for _, pl := range plugins {
		out = append(out, skillsIn(filepath.Join(pl.Dir, dirSkills), agentio.SkillPlugin, pl.Name)...)
	}
	cfg, _, err := readConfigPath(p.Config)
	if err != nil {
		return nil, err
	}
	if cfg.Skills != nil {
		for i := range out {
			for _, sc := range cfg.Skills.Config {
				if !sc.Enabled.Set {
					continue
				}
				if (sc.Path.Set && filepath.Clean(sc.Path.V) == filepath.Clean(out[i].Dir)) ||
					(sc.Path.Set && filepath.Clean(sc.Path.V) == filepath.Join(out[i].Dir, fileSkillMD)) ||
					(sc.Name.Set && sc.Name.V == out[i].Name) {
					out[i].Enabled = sc.Enabled.V
				}
			}
		}
	}
	return out, nil
}

// PluginInterface is a plugin manifest's `interface` block.
type PluginInterface struct {
	DisplayName   string   `json:"displayName,omitempty"`
	Category      string   `json:"category,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	DefaultPrompt []string `json:"defaultPrompt,omitempty"`
}

// Plugin is one installed plugin: the newest version dir under
// plugins/cache/<marketplace>/<plugin>/ joined with config enablement.
type Plugin struct {
	Name, Marketplace, Version, Dir, Description string
	Enabled                                      bool
	MCPServersFile                               string
	Interface                                    PluginInterface
	RemoteID                                     string
	Unknown                                      agentio.Unknown
}

// Neutral maps onto agentio.Plugin. Pure.
func (p Plugin) Neutral() agentio.Plugin {
	return agentio.Plugin{Name: p.Name, Marketplace: p.Marketplace, Version: p.Version, Dir: p.Dir, Enabled: p.Enabled}
}

type pluginManifestWire struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	MCPServers  json.RawMessage `json:"mcpServers"`
	Interface   PluginInterface `json:"interface"`
}

func pluginManifestKnownKeys() []string {
	return []string{"name", "version", "description", "mcpServers", "interface"}
}

type remoteInstallWire struct {
	RemotePluginID string `json:"remote_plugin_id"`
}

// Plugins lists installed plugins: for each <marketplace>/<plugin> the
// version directory with the newest mtime (how Codex picks one is
// unverified), its manifest, and `[plugins."<plugin>@<marketplace>"]
// enabled`. Disk · Read.
func (a *Agent) Plugins(h Home) ([]Plugin, error) {
	p := a.Paths(h)
	cfg, _, err := readConfigPath(p.Config)
	if err != nil {
		return nil, err
	}
	markets, err := os.ReadDir(p.PluginCache)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("codex: read %s: %w", p.PluginCache, err)
	}
	var out []Plugin
	for _, m := range markets {
		if !m.IsDir() {
			continue
		}
		mdir := filepath.Join(p.PluginCache, m.Name())
		plugins, err := os.ReadDir(mdir)
		if err != nil {
			continue
		}
		for _, pl := range plugins {
			if !pl.IsDir() {
				continue
			}
			vdir, ok := newestSubdir(filepath.Join(mdir, pl.Name()))
			if !ok {
				continue
			}
			out = append(out, readPlugin(vdir, m.Name(), pl.Name(), cfg))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Marketplace != out[j].Marketplace {
			return out[i].Marketplace < out[j].Marketplace
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func newestSubdir(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var best string
	var bestMod time.Time
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best, bestMod = filepath.Join(dir, e.Name()), info.ModTime()
		}
	}
	return best, best != ""
}

func readPlugin(vdir, market, name string, cfg Config) Plugin {
	pl := Plugin{Name: name, Marketplace: market, Version: filepath.Base(vdir), Dir: vdir}
	if b, err := os.ReadFile(filepath.Join(vdir, filePluginManifest)); err == nil {
		var w pluginManifestWire
		if json.Unmarshal(b, &w) == nil {
			if w.Name != "" {
				pl.Name = w.Name
			}
			if w.Version != "" {
				pl.Version = w.Version
			}
			pl.Description, pl.Interface = w.Description, w.Interface
			var mcpFile string
			if json.Unmarshal(w.MCPServers, &mcpFile) == nil && mcpFile != "" {
				pl.MCPServersFile = filepath.Join(vdir, mcpFile)
			}
			pl.Unknown, _ = agentio.ExtractUnknown(b, pluginManifestKnownKeys())
		}
	}
	if b, err := os.ReadFile(filepath.Join(vdir, fileRemoteInstall)); err == nil {
		var w remoteInstallWire
		if json.Unmarshal(b, &w) == nil {
			pl.RemoteID = w.RemotePluginID
		}
	}
	if e, ok := cfg.Plugins[name+pluginKeySep+market]; ok && e.Enabled.Set {
		pl.Enabled = e.Enabled.V
	}
	return pl
}

// Marketplace is one [marketplaces.<name>] entry.
type Marketplace struct {
	Name         string
	SourceType   string // git | local
	Source       string
	Ref          string
	LastRevision string
	LastUpdated  *time.Time
}

// Marketplaces lists the configured marketplaces, sorted by name. Pure.
func (a *Agent) Marketplaces(c Config) []Marketplace {
	var out []Marketplace
	for name, m := range c.Marketplaces {
		mk := Marketplace{Name: name, SourceType: m.SourceType.V, Source: m.Source.V, Ref: m.Ref.V, LastRevision: m.LastRevision.V}
		if t := parseRFC3339(m.LastUpdated.V); !t.IsZero() {
			mk.LastUpdated = &t
		}
		out = append(out, mk)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// plugin subcommand words.
const (
	subPlugin     = "plugin"
	subList       = "list"
	flagJSON      = "--json"
	flagAvailable = "--available"
)

// PluginListCommand builds `codex plugin list --json [--available]` (the
// vendor reads remote catalogs over the network when run). Pure.
func (a *Agent) PluginListCommand(h Home, available bool) agentio.Command {
	args := []string{subPlugin, subList, flagJSON}
	if available {
		args = append(args, flagAvailable)
	}
	return a.plainCommand(h, args)
}

// trimBOM drops a UTF-8 byte-order mark.
func trimBOM(b []byte) []byte { return bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")) }
