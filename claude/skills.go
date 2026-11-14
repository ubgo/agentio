package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/khanakia/agentcli"
	"github.com/khanakia/agentcli/frontmatter"
)

// SkillScope is where a skill, command, subagent or output style comes from.
type SkillScope string

// Skill scopes (CLAUDE_API §8).
const (
	SkillScopeUser    SkillScope = "user"
	SkillScopeProject SkillScope = "project"
	SkillScopePlugin  SkillScope = "plugin"
	SkillScopeManaged SkillScope = "managed"
	SkillScopeBundled SkillScope = "bundled"
	SkillScopeSynced  SkillScope = "synced"
	SkillScopeAddDir  SkillScope = "add-dir"
)

// IsKnown reports a documented scope. cost: Pure.
func (s SkillScope) IsKnown() bool {
	switch s {
	case SkillScopeUser, SkillScopeProject, SkillScopePlugin, SkillScopeManaged, SkillScopeBundled, SkillScopeSynced, SkillScopeAddDir:
		return true
	}
	return false
}

// Frontmatter read bounds: listers read only the head of a file, never a
// whole skill body.
const (
	frontmatterReadBytes = 64 << 10
	promptReadBytes      = 256 << 10
	workflowExt          = ".js"
)

// Skill frontmatter keys (docs).
const (
	fmName                   = "name"
	fmDescription            = "description"
	fmWhenToUse              = "when_to_use"
	fmArgumentHint           = "argument-hint"
	fmArguments              = "arguments"
	fmDisableModelInvocation = "disable-model-invocation"
	fmUserInvocable          = "user-invocable"
	fmAllowedTools           = "allowed-tools"
	fmDisallowedTools        = "disallowed-tools"
	fmModel                  = "model"
	fmEffort                 = "effort"
	fmContext                = "context"
	fmAgent                  = "agent"
	fmBackground             = "background"
	fmHooks                  = "hooks"
	fmPaths                  = "paths"
	fmShell                  = "shell"
	fmMetadata               = "metadata"
)

// readFront reads the frontmatter (and the head of the body) of a file,
// bounded by limit.
func readFront(path string, limit int64) (frontmatter.Doc, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return frontmatter.Doc{}, nil, err
	}
	// Read-only: a close error cannot lose data.
	defer func() { _ = f.Close() }()
	buf := make([]byte, limit)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return frontmatter.Doc{}, nil, nil // empty file
	}
	d, body, perr := frontmatter.Parse(buf[:n])
	if perr != nil {
		return frontmatter.Doc{}, body, fmt.Errorf("claude: frontmatter %s: %w", path, perr)
	}
	return d, body, nil
}

// valueJSON re-encodes a frontmatter value as JSON for Unknown.
func valueJSON(v frontmatter.Value) json.RawMessage {
	var b []byte
	switch v.Kind {
	case frontmatter.KindList:
		b, _ = json.Marshal(v.List)
	case frontmatter.KindMap:
		m := map[string]json.RawMessage{}
		if v.Map != nil {
			for _, f := range v.Map.Fields {
				m[f.Key] = valueJSON(f.Value)
			}
		}
		b, _ = json.Marshal(m)
	default:
		b, _ = json.Marshal(v.Text)
	}
	return b
}

// docUnknown keeps the keys of d that known does not list.
func docUnknown(d frontmatter.Doc, known []string) agentcli.Unknown {
	var u agentcli.Unknown
	for _, k := range d.Keys() {
		if slices.Contains(known, k) {
			continue
		}
		v, _ := d.Get(k)
		if u == nil {
			u = agentcli.Unknown{}
		}
		u[k] = valueJSON(v)
	}
	return u
}

// docBool converts a present YAML boolean to a pointer.
func docBool(d frontmatter.Doc, key string) *bool {
	if v, ok := d.Bool(key); ok {
		return &v
	}
	return nil
}

// docStringMap reads a nested map of scalars.
func docStringMap(d frontmatter.Doc, key string) map[string]string {
	m := d.Map(key)
	if len(m.Fields) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, f := range m.Fields {
		if f.Value.Kind == frontmatter.KindScalar {
			out[f.Key] = f.Value.Text
		}
	}
	return out
}

// docHooks decodes a `hooks` frontmatter map through its JSON form.
func docHooks(d frontmatter.Doc) Hooks {
	v, ok := d.Get(fmHooks)
	if !ok || v.Kind != frontmatter.KindMap {
		return nil
	}
	var h Hooks
	if json.Unmarshal(valueJSON(v), &h) != nil {
		return nil
	}
	return h
}

// Skill is one SKILL.md (frontmatter only) or command file.
type Skill struct {
	Name, Description, WhenToUse, ArgumentHint string
	Scope                                      SkillScope
	Plugin                                     PluginID
	Dir, File                                  string
	UserInvocable, DisableModelInvocation      *bool
	AllowedTools, DisallowedTools              []string
	Model                                      string
	Effort                                     Effort
	Context                                    string
	Agent                                      string
	Background                                 *bool
	Paths                                      []string
	Shell                                      string
	Hooks                                      Hooks
	Metadata                                   map[string]string
	// Unknown holds other frontmatter keys, re-encoded to JSON.
	Unknown agentcli.Unknown
	// Err is a frontmatter parse error; the entry is still listed.
	Err error
}

func skillKnownKeys() []string {
	return []string{fmName, fmDescription, fmWhenToUse, fmArgumentHint, fmDisableModelInvocation, fmUserInvocable,
		fmAllowedTools, fmDisallowedTools, fmModel, fmEffort, fmContext, fmAgent, fmBackground, fmHooks, fmPaths, fmShell, fmMetadata}
}

// skillFromDoc fills a Skill from parsed frontmatter.
func skillFromDoc(d frontmatter.Doc, file, fallbackName string, scope SkillScope, plugin PluginID) Skill {
	s := Skill{
		Name: d.String(fmName), Description: d.String(fmDescription), WhenToUse: d.String(fmWhenToUse),
		ArgumentHint: d.String(fmArgumentHint), Scope: scope, Plugin: plugin, Dir: filepath.Dir(file), File: file,
		UserInvocable: docBool(d, fmUserInvocable), DisableModelInvocation: docBool(d, fmDisableModelInvocation),
		AllowedTools: d.List(fmAllowedTools), DisallowedTools: d.List(fmDisallowedTools),
		Model: d.String(fmModel), Effort: Effort(d.String(fmEffort)), Context: d.String(fmContext), Agent: d.String(fmAgent),
		Background: docBool(d, fmBackground), Paths: d.List(fmPaths), Shell: d.String(fmShell),
		Hooks: docHooks(d), Metadata: docStringMap(d, fmMetadata), Unknown: docUnknown(d, skillKnownKeys()),
	}
	if s.Name == "" {
		s.Name = fallbackName
	}
	return s
}

// SlashCommand is a commands/*.md file; named so it never collides with
// agentcli.Command.
type SlashCommand struct{ Skill }

// Workflow is a workflows/*.js file that becomes /<name>.
type Workflow struct {
	Name, File string
	Scope      SkillScope
}

// skillRoot is one directory that holds skills, commands or agents.
type skillRoot struct {
	dir    string
	scope  SkillScope
	plugin PluginID
}

// roots lists the user, project and plugin dirs named sub (skills,
// commands, agents, output-styles), in precedence order.
func (a *Agent) roots(h Home, p *Project, sub string) []skillRoot {
	out := []skillRoot{{dir: filepath.Join(h.Dir(), sub), scope: SkillScopeUser}}
	if p != nil {
		out = append(out, skillRoot{dir: filepath.Join(p.Dir, dotClaudeDir, sub), scope: SkillScopeProject})
	}
	if plugins, err := a.Plugins(h, p); err == nil {
		for _, pl := range plugins {
			if pl.InstallPath != "" {
				out = append(out, skillRoot{dir: filepath.Join(pl.InstallPath, sub), scope: SkillScopePlugin, plugin: pl.ID})
			}
		}
	}
	return out
}

// sortedEntries reads a dir sorted by name; a missing dir is empty.
func sortedEntries(dir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: list %s: %w", dir, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// isDirFollow reports a dir, following a symlink (skills are often symlinked).
func isDirFollow(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// Skills lists <home>/skills/*/SKILL.md, project .claude/skills and each
// installed plugin's skills dir; frontmatter only. cost: Disk · Read.
func (a *Agent) Skills(h Home, p *Project) ([]Skill, error) {
	var out []Skill
	for _, r := range a.roots(h, p, skillsDir) {
		entries, err := sortedEntries(r.dir)
		if err != nil {
			return out, err
		}
		for _, e := range entries {
			dir := filepath.Join(r.dir, e.Name())
			if strings.HasPrefix(e.Name(), ".") || !isDirFollow(dir) {
				continue
			}
			file := filepath.Join(dir, skillFile)
			if !fileExists(file) {
				continue
			}
			d, _, perr := readFront(file, frontmatterReadBytes)
			s := skillFromDoc(d, file, e.Name(), r.scope, r.plugin)
			s.Err = perr
			out = append(out, s)
		}
	}
	return out, nil
}

// SlashCommands lists commands/*.md in the user, project and plugin dirs.
// cost: Disk · Read.
func (a *Agent) SlashCommands(h Home, p *Project) ([]SlashCommand, error) {
	var out []SlashCommand
	for _, r := range a.roots(h, p, commandsDir) {
		entries, err := sortedEntries(r.dir)
		if err != nil {
			return out, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), markdownExt) {
				continue
			}
			file := filepath.Join(r.dir, e.Name())
			d, _, perr := readFront(file, frontmatterReadBytes)
			s := skillFromDoc(d, file, strings.TrimSuffix(e.Name(), markdownExt), r.scope, r.plugin)
			s.Name = strings.TrimSuffix(e.Name(), markdownExt) // commands are named by file
			s.Err = perr
			out = append(out, SlashCommand{Skill: s})
		}
	}
	return out, nil
}

// Workflows lists workflows/*.js in the user and project dirs.
// cost: Disk · Read.
func (a *Agent) Workflows(h Home, p *Project) ([]Workflow, error) {
	dirs := []skillRoot{{dir: filepath.Join(h.Dir(), workflowsDir), scope: SkillScopeUser}}
	if p != nil {
		dirs = append(dirs, skillRoot{dir: filepath.Join(p.Dir, dotClaudeDir, workflowsDir), scope: SkillScopeProject})
	}
	var out []Workflow
	for _, r := range dirs {
		entries, err := sortedEntries(r.dir)
		if err != nil {
			return out, err
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), workflowExt) {
				out = append(out, Workflow{Name: strings.TrimSuffix(e.Name(), workflowExt), File: filepath.Join(r.dir, e.Name()), Scope: r.scope})
			}
		}
	}
	return out, nil
}
