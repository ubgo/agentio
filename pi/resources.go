package pi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/frontmatter"
)

// InstructionFiles lists the instruction files pi loads for dir, in load
// order (resource-loader.js:129-215, :959-980):
//   - the agent dir's context file, then one per directory from the
//     filesystem root down to dir; in each directory only the first of
//     AGENTS.override.md, AGENTS.md, AGENTS.MD, CLAUDE.md, CLAUDE.MD counts,
//     and context files load whether or not the project is trusted;
//   - SYSTEM.md and APPEND_SYSTEM.md: the project's .pi/<name> when the
//     project is trusted and has one, else the agent dir's (never both).
//
// Disk · Read (stat only).
func (a *Agent) InstructionFiles(h Home, dir string) ([]agentio.InstructionFile, error) {
	var out []agentio.InstructionFile
	if f, ok := firstContextFile(h.Dir()); ok {
		f.Scope = agentio.InstructionUser
		out = append(out, f)
	}
	abs, err := absDir(dir)
	if err != nil {
		return nil, err
	}
	if abs != "" {
		var chain []string
		for d := abs; ; d = filepath.Dir(d) {
			chain = append(chain, d)
			if filepath.Dir(d) == d {
				break
			}
		}
		slices.Reverse(chain)
		for _, d := range chain {
			if f, ok := firstContextFile(d); ok && !slices.ContainsFunc(out, func(o agentio.InstructionFile) bool { return o.Path == f.Path }) {
				f.Scope = agentio.InstructionProject
				out = append(out, f)
			}
		}
	}
	trusted := false
	if abs != "" {
		t, err := a.ProjectTrust(h, abs)
		if err != nil {
			return nil, err
		}
		trusted = t == agentio.TrustTrusted
	}
	for _, name := range []string{systemFile, appendFile} {
		if trusted {
			p := filepath.Join(abs, projectDirName, name)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				out = append(out, agentio.InstructionFile{Path: p, Scope: agentio.InstructionProject, Kind: name, Bytes: fi.Size()})
				continue
			}
		}
		p := filepath.Join(h.Dir(), name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			out = append(out, agentio.InstructionFile{Path: p, Scope: agentio.InstructionUser, Kind: name, Bytes: fi.Size()})
		}
	}
	return out, nil
}

// firstContextFile is the first existing context file in d. Disk · Read.
func firstContextFile(d string) (agentio.InstructionFile, bool) {
	for _, name := range contextFileNames() {
		p := filepath.Join(d, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return agentio.InstructionFile{Path: p, Kind: name, Bytes: fi.Size()}, true
		}
	}
	return agentio.InstructionFile{}, false
}

// MCPServerConfig is one mcp.json server (PKG/docs/mcp.md). Env and
// header values may hold secrets or "!command" values, so only their
// names are kept.
type MCPServerConfig struct {
	Name    string
	Type    string
	Command string
	Args    []string
	Cwd     string
	URL     string
	// EnvNames and HeaderNames are names only.
	EnvNames, HeaderNames []string
	// Enabled is nil when the entry does not say (pi connects it).
	Enabled     *bool
	Description string
	Scope       agentio.ConfigScope
	Source      string
}

// mcpServerWire is one server entry as read.
type mcpServerWire struct {
	Type        string                     `json:"type"`
	Command     string                     `json:"command"`
	Args        []string                   `json:"args"`
	Cwd         string                     `json:"cwd"`
	URL         string                     `json:"url"`
	Env         map[string]json.RawMessage `json:"env"`
	Headers     map[string]json.RawMessage `json:"headers"`
	Enabled     *bool                      `json:"enabled"`
	Description string                     `json:"description"`
}

// mcpFileWire is mcp.json.
type mcpFileWire struct {
	MCPServers map[string]mcpServerWire `json:"mcpServers"`
}

// readMCPFile reads one mcp.json; absent = nil. Disk · Read.
func readMCPFile(p string, scope agentio.ConfigScope) (map[string]MCPServerConfig, error) {
	data, found, err := readFileBounded(p, maxJSONFileBytes)
	if err != nil || !found {
		return nil, err
	}
	var w mcpFileWire
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("pi: decode %s: %w", p, err)
	}
	out := map[string]MCPServerConfig{}
	for name, s := range w.MCPServers {
		out[name] = MCPServerConfig{
			Name: name, Type: s.Type, Command: s.Command, Args: s.Args, Cwd: s.Cwd, URL: s.URL,
			EnvNames: sortedKeys(s.Env), HeaderNames: sortedKeys(s.Headers), Enabled: s.Enabled,
			Description: s.Description, Scope: scope, Source: p,
		}
	}
	return out, nil
}

// sortedKeys lists a map's keys, sorted. Pure.
func sortedKeys(m map[string]json.RawMessage) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// MCPConfigs lists the MCP servers pi would configure for h in dir,
// sorted by name: the agent dir's mcp.json, with a trusted project's
// .pi/mcp.json replacing a same-name entry, or, for an entry without
// command, url and type, overriding only its enabled flag (PKG/docs/
// mcp.md "Configure servers"). Never connects. Disk · Read.
func (a *Agent) MCPConfigs(h Home, dir string) ([]MCPServerConfig, error) {
	user, err := readMCPFile(a.Paths(h).MCP, agentio.ConfigUser)
	if err != nil {
		return nil, err
	}
	merged := map[string]MCPServerConfig{}
	for k, v := range user {
		merged[k] = v
	}
	if dir != "" {
		abs, err := absDir(dir)
		if err != nil {
			return nil, err
		}
		trust, err := a.ProjectTrust(h, abs)
		if err != nil {
			return nil, err
		}
		if trust == agentio.TrustTrusted {
			proj, err := readMCPFile(filepath.Join(abs, projectDirName, mcpFile), agentio.ConfigProject)
			if err != nil {
				return nil, err
			}
			for k, p := range proj {
				base, ok := merged[k]
				if ok && p.Command == "" && p.URL == "" && p.Type == "" {
					if p.Enabled != nil {
						base.Enabled = p.Enabled
					}
					merged[k] = base
					continue
				}
				merged[k] = p
			}
		}
	}
	out := make([]MCPServerConfig, 0, len(merged))
	for _, v := range merged {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// MCPServers is the neutral view: command selects stdio, url streamable
// HTTP (pi rejects SSE). Disk · Read.
func (a *Agent) MCPServers(h Home, dir string) ([]agentio.MCPServer, error) {
	cfgs, err := a.MCPConfigs(h, dir)
	if err != nil {
		return nil, err
	}
	out := make([]agentio.MCPServer, 0, len(cfgs))
	for _, c := range cfgs {
		tr := agentio.MCPStdio
		if c.URL != "" {
			tr = agentio.MCPHTTP
		}
		out = append(out, agentio.MCPServer{Name: c.Name, Transport: tr, Scope: c.Scope, Command: c.Command, Args: c.Args, URL: c.URL, EnvNames: c.EnvNames, Enabled: c.Enabled, Source: c.Source})
	}
	return out, nil
}

// Skill frontmatter keys (Agent Skills, PKG/docs/skills.md).
const (
	fmName        = "name"
	fmDescription = "description"
)

// Skills lists SKILL.md skills pi discovers for h in dir: the agent
// dir's skills/ and ~/.agents/skills/ (user), then a trusted project's
// .pi/skills/ (project). Discovery is recursive (PKG/docs/skills.md:9-75);
// the name falls back to the folder name. Settings skills paths and
// package skills are not followed. Disk · Read.
func (a *Agent) Skills(h Home, dir string) ([]agentio.Skill, error) {
	roots := []struct {
		dir   string
		scope agentio.SkillScope
	}{
		{filepath.Join(h.Dir(), skillsDir), agentio.SkillUser},
		{filepath.Join(a.host.UserHome, ".agents", skillsDir), agentio.SkillUser},
	}
	if dir != "" {
		abs, err := absDir(dir)
		if err != nil {
			return nil, err
		}
		trust, err := a.ProjectTrust(h, abs)
		if err != nil {
			return nil, err
		}
		if trust == agentio.TrustTrusted {
			roots = append(roots, struct {
				dir   string
				scope agentio.SkillScope
			}{filepath.Join(abs, projectDirName, skillsDir), agentio.SkillProject})
		}
	}
	var out []agentio.Skill
	for _, r := range roots {
		if !isDir(r.dir) {
			continue
		}
		err := filepath.WalkDir(r.dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Name() != skillFile {
				return nil
			}
			out = append(out, readSkill(p, r.scope))
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("pi: skills %s: %w", r.dir, err)
		}
	}
	return out, nil
}

// readSkill reads a SKILL.md's name and description. Disk · Read.
func readSkill(p string, scope agentio.SkillScope) agentio.Skill {
	s := agentio.Skill{Name: filepath.Base(filepath.Dir(p)), Dir: filepath.Dir(p), Scope: scope}
	data, found, err := readFileBounded(p, maxJSONFileBytes)
	if err != nil || !found {
		return s
	}
	doc, _, err := frontmatter.Parse(data)
	if err != nil {
		return s
	}
	if n := doc.String(fmName); n != "" {
		s.Name = n
	}
	s.Description = doc.String(fmDescription)
	return s
}

// PromptTemplate is one prompt template: a markdown file whose name is
// the /name command (PKG/docs/prompt-templates.md:1-30).
type PromptTemplate struct {
	Name, Description, Path string
	Scope                   agentio.SkillScope
}

// PromptTemplates lists the agent dir's prompts/*.md (user) and, for a
// trusted project, .pi/prompts/*.md (project). Disk · Read.
func (a *Agent) PromptTemplates(h Home, dir string) ([]PromptTemplate, error) {
	roots := []struct {
		dir   string
		scope agentio.SkillScope
	}{{filepath.Join(h.Dir(), promptsDir), agentio.SkillUser}}
	if dir != "" {
		abs, err := absDir(dir)
		if err != nil {
			return nil, err
		}
		if t, err := a.ProjectTrust(h, abs); err != nil {
			return nil, err
		} else if t == agentio.TrustTrusted {
			roots = append(roots, struct {
				dir   string
				scope agentio.SkillScope
			}{filepath.Join(abs, projectDirName, promptsDir), agentio.SkillProject})
		}
	}
	var out []PromptTemplate
	for _, r := range roots {
		entries, err := os.ReadDir(r.dir)
		if isNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("pi: prompts %s: %w", r.dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), markdownExt) {
				continue
			}
			p := filepath.Join(r.dir, e.Name())
			t := PromptTemplate{Name: strings.TrimSuffix(e.Name(), markdownExt), Path: p, Scope: r.scope}
			if data, found, err := readFileBounded(p, maxJSONFileBytes); err == nil && found {
				if doc, _, err := frontmatter.Parse(data); err == nil {
					t.Description = doc.String(fmDescription)
				}
			}
			out = append(out, t)
		}
	}
	return out, nil
}
