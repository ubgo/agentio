package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/frontmatter"
)

// Front matter keys of a SKILL.md and the read bound.
const (
	fmName           = "name"
	fmDescription    = "description"
	frontmatterBytes = 8 << 10
	// skillsSettingsKey is the settings section holding skills.disabled.
	skillsSettingsKey = "skills"
	// overrideNegate starts a negated extension-enablement.json rule.
	overrideNegate = "!"
)

// ExtensionManifest is gemini-extension.json (cli extension loader). Only
// name and version are required; every other key is kept in Unknown.
type ExtensionManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// ContextFileName is a string or a list; default GEMINI.md.
	ContextFileName StringList                 `json:"contextFileName,omitempty"`
	MCPServers      map[string]MCPServerConfig `json:"mcpServers,omitempty"`
	ExcludeTools    []string                   `json:"excludeTools,omitempty"`
	Unknown         agentio.Unknown            `json:"-"`
}

// UnmarshalJSON keeps unknown keys. cost: Pure.
func (m *ExtensionManifest) UnmarshalJSON(b []byte) error {
	type wire ExtensionManifest
	var w wire
	err := json.Unmarshal(b, &w)
	u, uerr := agentio.ExtractUnknownFor(b, reflect.TypeFor[wire]())
	*m, m.Unknown = ExtensionManifest(w), u
	return errors.Join(err, uerr)
}

// Extension is one installed extension.
type Extension struct {
	Manifest ExtensionManifest
	// Dir is the extension dir (<home>/extensions/<name>).
	Dir string
	// Scope is user (the home) or project (<dir>/.gemini/extensions).
	Scope agentio.ConfigScope
	// Enabled applies extension-enablement.json to the project dir the
	// caller asked about; true when no rule matches (the CLI default).
	Enabled bool
}

// extensionOverrides is one extension-enablement.json entry:
// {"overrides": ["<path glob>", "!<path glob>"]}; the last matching rule
// wins (cli ExtensionEnablementManager).
type extensionOverrides struct {
	Overrides []string `json:"overrides"`
}

// Extensions lists the extensions of h and, when dir is set, dir's
// .gemini/extensions, with enablement for dir (the user's home dir when
// dir is ""). An unreadable manifest is skipped: the CLI skips it too.
// cost: Disk · Read.
func (a *Agent) Extensions(h Home, dir string) ([]Extension, error) {
	enable, err := readEnablement(h.Path(extensionsDir, extensionEnablement))
	if err != nil {
		return nil, err
	}
	where := dir
	if where == "" {
		where = a.host.UserHome
	}
	out := extensionsIn(h.Path(extensionsDir), agentio.ConfigUser, enable, where)
	if dir != "" {
		ws := filepath.Join(dir, defaultDirName, extensionsDir)
		if !samePath(ws, h.Path(extensionsDir)) {
			out = append(out, extensionsIn(ws, agentio.ConfigProject, enable, where)...)
		}
	}
	return out, nil
}

// readEnablement reads extension-enablement.json; missing = no rules.
// cost: Disk · Read.
func readEnablement(p string) (map[string]extensionOverrides, error) {
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("gemini: read %s: %w", p, err)
	}
	var out map[string]extensionOverrides
	if err := json.Unmarshal(agentio.StripBOM(data), &out); err != nil {
		return nil, fmt.Errorf("gemini: decode %s: %w", p, err)
	}
	return out, nil
}

// extensionsIn reads every <root>/<name>/gemini-extension.json.
// cost: Disk · Read.
func extensionsIn(root string, scope agentio.ConfigScope, enable map[string]extensionOverrides, where string) []Extension {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []Extension
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		d := filepath.Join(root, e.Name())
		data, err := os.ReadFile(filepath.Join(d, extensionManifest))
		if err != nil {
			continue
		}
		var m ExtensionManifest
		if json.Unmarshal(StripJSONComments(agentio.StripBOM(data)), &m) != nil || m.Name == "" {
			continue
		}
		out = append(out, Extension{Manifest: m, Dir: d, Scope: scope, Enabled: enabledFor(enable[m.Name], where)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.Name < out[j].Manifest.Name })
	return out
}

// enabledFor applies an extension's overrides to dir as the CLI does
// (cli ExtensionEnablementManager.isEnabled): each rule is a path glob,
// "!" disables and a plain rule enables, a trailing "*" covers subdirs;
// dir is matched with forward slashes and a leading and trailing "/"; the
// last match wins; no match = enabled. cost: Pure.
func enabledFor(o extensionOverrides, dir string) bool {
	enabled := true
	target := withSlashes(dir)
	for _, rule := range o.Overrides {
		disable := strings.HasPrefix(rule, overrideNegate)
		if globRegexp(strings.TrimPrefix(rule, overrideNegate)).MatchString(target) {
			enabled = !disable
		}
	}
	return enabled
}

// withSlashes is the CLI's ensureLeadingAndTrailingSlash. cost: Pure.
func withSlashes(dir string) string {
	s := filepath.ToSlash(strings.ReplaceAll(dir, `\`, "/"))
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	if !strings.HasSuffix(s, "/") {
		s += "/"
	}
	return s
}

// globRegexp is the CLI's globToRegex: every other character literal, and
// each "*", together with a "/" right before it, matching anything or
// nothing ("/a/*" matches "/a" and everything below it). A glob always
// compiles: every literal is quoted. cost: Pure.
func globRegexp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch {
		case glob[i] == '/' && i+1 < len(glob) && glob[i+1] == '*':
			b.WriteString("(/.*)?")
			i++
		case glob[i] == '*':
			b.WriteString("(.*)?")
		default:
			b.WriteString(regexp.QuoteMeta(glob[i : i+1]))
		}
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// Neutral maps an extension onto agentio.Plugin (an extension is Gemini's
// plugin; there is no marketplace). cost: Pure.
func (e Extension) Neutral() agentio.Plugin {
	return agentio.Plugin{Name: e.Manifest.Name, Version: e.Manifest.Version, Dir: e.Dir, Enabled: e.Enabled}
}

// Skill is one SKILL.md (front matter only).
type Skill struct {
	Name, Description, Dir string
	Scope                  agentio.SkillScope
	// Extension names the extension that ships it; "" for none.
	Extension string
	// Enabled is false when settings skills.disabled names it.
	Enabled bool
}

// Neutral maps onto agentio.Skill. cost: Pure.
func (s Skill) Neutral() agentio.Skill {
	en := s.Enabled
	return agentio.Skill{Name: s.Name, Description: s.Description, Dir: s.Dir, Plugin: s.Extension, Scope: s.Scope, Enabled: &en}
}

// skillsSection is the part of settings.skills agentio reads.
type skillsSection struct {
	Disabled []string `json:"disabled"`
}

// Skills lists skills from every dir the CLI reads (core skill loader):
// <home>/skills, <home's parent>/.agents/skills, and for dir its
// .gemini/skills and .agents/skills, plus each enabled extension's
// skills/. settings skills.disabled (user and workspace) marks a skill
// disabled. Front matter only (first 8 KiB). cost: Disk · Read.
func (a *Agent) Skills(h Home, dir string) ([]Skill, error) {
	disabled, err := a.disabledSkills(h, dir)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(h.Dir())
	var out []Skill
	out = append(out, skillsIn(h.Path(skillsDir), agentio.SkillUser, "")...)
	out = append(out, skillsIn(filepath.Join(parent, agentsDotDir, skillsDir), agentio.SkillUser, "")...)
	if dir != "" {
		for _, d := range []string{filepath.Join(dir, defaultDirName, skillsDir), filepath.Join(dir, agentsDotDir, skillsDir)} {
			if !samePath(d, h.Path(skillsDir)) && !samePath(d, filepath.Join(parent, agentsDotDir, skillsDir)) {
				out = append(out, skillsIn(d, agentio.SkillProject, "")...)
			}
		}
	}
	exts, err := a.Extensions(h, dir)
	if err != nil {
		return nil, err
	}
	for _, e := range exts {
		if e.Enabled {
			out = append(out, skillsIn(filepath.Join(e.Dir, skillsDir), agentio.SkillPlugin, e.Manifest.Name)...)
		}
	}
	for i := range out {
		out[i].Enabled = !slices.Contains(disabled, out[i].Name)
	}
	return out, nil
}

// disabledSkills collects skills.disabled from the user and workspace
// settings. cost: Disk · Read.
func (a *Agent) disabledSkills(h Home, dir string) ([]string, error) {
	paths := []string{h.UserSettingsPath()}
	if ws, ok := workspaceSettings(h, dir); ok {
		paths = append(paths, ws)
	}
	var out []string
	for _, p := range paths {
		f, err := ReadSettingsFile(agentio.ConfigUser, p)
		if err != nil {
			return nil, err
		}
		raw, ok := f.Settings.Unknown[skillsSettingsKey]
		if !ok {
			continue
		}
		var sec skillsSection
		if json.Unmarshal(raw, &sec) == nil {
			out = append(out, sec.Disabled...)
		}
	}
	return out, nil
}

// skillsIn lists every <root>/<name>/SKILL.md (one level). cost: Disk · Read.
func skillsIn(root string, scope agentio.SkillScope, ext string) []Skill {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		d := filepath.Join(root, e.Name())
		f, err := os.Open(filepath.Join(d, skillFile))
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(f, frontmatterBytes))
		_ = f.Close() // read-only: a close error cannot lose data
		if err != nil {
			continue
		}
		doc, _, err := frontmatter.Parse(b)
		if err != nil {
			doc = frontmatter.Doc{}
		}
		s := Skill{Name: doc.String(fmName), Description: strings.TrimSpace(doc.String(fmDescription)), Dir: d, Scope: scope, Extension: ext, Enabled: true}
		if s.Name == "" {
			s.Name = e.Name()
		}
		out = append(out, s)
	}
	return out
}
