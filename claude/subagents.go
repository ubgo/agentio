package claude

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/frontmatter"
)

// SubagentDef is one subagent definition (agents/*.md frontmatter or an
// --agents entry).
type SubagentDef struct {
	Name, Description, Prompt string
	Scope                     SkillScope
	File                      string
	Tools, DisallowedTools    []string
	Skills                    []string
	Model                     string
	PermissionMode            PermissionMode
	MaxTurns                  *int
	Effort                    Effort
	Isolation, Color          string
	Background                *bool
	Memory                    string
	OmitClaudeMD              *bool
	MCPServers                map[string]MCPServer
	Hooks                     Hooks
	Unknown                   agentio.Unknown
	// Err is a frontmatter parse error; the entry is still listed.
	Err error
}

// subagentFilePrefix starts every subagent run file name (agent-<id>).
const subagentFilePrefix = "agent-"

// Subagent frontmatter keys (docs).
const (
	fmTools                = "tools"
	fmDisallowedToolsCamel = "disallowedTools"
	fmSkills               = "skills"
	fmPermissionMode       = "permissionMode"
	fmMaxTurns             = "maxTurns"
	fmIsolation            = "isolation"
	fmColor                = "color"
	fmMemory               = "memory"
	fmOmitClaudeMD         = "omitClaudeMd"
	fmMCPServers           = "mcpServers"
)

func subagentKnownKeys() []string {
	return []string{fmName, fmDescription, fmTools, fmDisallowedToolsCamel, fmSkills, fmModel, fmPermissionMode, fmMaxTurns,
		fmEffort, fmIsolation, fmColor, fmBackground, fmMemory, fmOmitClaudeMD, fmMCPServers, fmHooks}
}

// subagentFromDoc fills a definition from frontmatter and body.
func subagentFromDoc(d frontmatter.Doc, body []byte, file string, scope SkillScope) SubagentDef {
	s := SubagentDef{
		Name: d.String(fmName), Description: d.String(fmDescription), Prompt: strings.TrimSpace(string(body)),
		Scope: scope, File: file, Tools: d.List(fmTools), DisallowedTools: d.List(fmDisallowedToolsCamel), Skills: d.List(fmSkills),
		Model: d.String(fmModel), Effort: Effort(d.String(fmEffort)), Isolation: d.String(fmIsolation), Color: d.String(fmColor),
		Background: docBool(d, fmBackground), Memory: d.String(fmMemory), OmitClaudeMD: docBool(d, fmOmitClaudeMD),
		Hooks: docHooks(d), Unknown: docUnknown(d, subagentKnownKeys()),
	}
	_ = s.PermissionMode.UnmarshalText([]byte(d.String(fmPermissionMode)))
	if n, err := strconv.Atoi(d.String(fmMaxTurns)); err == nil {
		s.MaxTurns = &n
	}
	if v, ok := d.Get(fmMCPServers); ok && v.Kind == frontmatter.KindMap && v.Map != nil {
		raws := map[string]json.RawMessage{}
		for _, f := range v.Map.Fields {
			raws[f.Key] = valueJSON(f.Value)
		}
		s.MCPServers, _ = parseServerMap(raws, MCPScopeSession, file, nil)
	}
	if s.Name == "" {
		s.Name = strings.TrimSuffix(filepath.Base(file), markdownExt)
	}
	return s
}

// SubagentDefs lists agents/*.md of the user, project and plugin dirs.
// cost: Disk · Read (frontmatter + body, bounded).
func (a *Agent) SubagentDefs(h Home, p *Project) ([]SubagentDef, error) {
	var out []SubagentDef
	for _, r := range a.roots(h, p, agentsDir) {
		for _, f := range markdownIn(r.dir) {
			d, body, err := readFront(f, promptReadBytes)
			s := subagentFromDoc(d, body, f, r.scope)
			s.Err = err
			out = append(out, s)
		}
	}
	return out, nil
}

// SubagentRun is one subagent run of a session: a
// subagents/agent-<id>.meta.json with its transcript, or (older Claude
// Code, which wrote no meta file) a subagents/agent-<id>.jsonl alone, for
// which only ID and Transcript are set and HasMeta is false.
type SubagentRun struct {
	ID, AgentType, Name, Description, Model, TeamName, TaskKind string
	IsBuiltIn, NonInteractive, PlanModeRequired                 bool
	SpawnDepth                                                  int
	PermissionMode                                              PermissionMode
	Transcript                                                  string
	// HasMeta reports the run's .meta.json was read; false for a
	// transcript without one.
	HasMeta bool
	Unknown agentio.Unknown
}

type wireSubagentMeta struct {
	AgentType             string         `json:"agentType"`
	IsBuiltIn             bool           `json:"isBuiltIn"`
	Description           string         `json:"description"`
	Name                  string         `json:"name"`
	SpawnDepth            int            `json:"spawnDepth"`
	RequestNonInteractive bool           `json:"requestNonInteractive"`
	Model                 string         `json:"model"`
	TaskKind              string         `json:"taskKind"`
	TeamName              string         `json:"teamName"`
	PlanModeRequired      bool           `json:"planModeRequired"`
	PermissionMode        PermissionMode `json:"permissionMode"`
}

// SubagentRuns lists the subagent runs of one session started in dir, in
// name order: every agent-<id>.meta.json (an unreadable one is skipped),
// and every agent-<id>.jsonl transcript without a meta file (older
// Claude Code wrote none; HasMeta false). Files in subfolders (workflow
// journals) are not runs. cost: Disk · Read.
func (a *Agent) SubagentRuns(h Home, dir, sessionID string) ([]SubagentRun, error) {
	sub := filepath.Join(a.HistoryDir(h, dir), sessionID, subagentsDir)
	entries, err := sortedEntries(sub)
	if err != nil {
		return nil, err
	}
	metas := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), metaSuffix) {
			metas[strings.TrimSuffix(e.Name(), metaSuffix)] = true
		}
	}
	var out []SubagentRun
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if id, ok := strings.CutSuffix(e.Name(), transcriptExt); ok && strings.HasPrefix(id, subagentFilePrefix) && !metas[id] {
			out = append(out, SubagentRun{ID: id, Transcript: filepath.Join(sub, e.Name())})
			continue
		}
		if !strings.HasSuffix(e.Name(), metaSuffix) {
			continue
		}
		path := filepath.Join(sub, e.Name())
		data, found, err := readFileBounded(path, maxResponseBytes)
		if err != nil || !found {
			continue
		}
		var w wireSubagentMeta
		unknown, _, err := decodeLenient(e.Name(), data, reflect.TypeFor[wireSubagentMeta](), func(b []byte) error { w = wireSubagentMeta{}; return json.Unmarshal(b, &w) })
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(e.Name(), metaSuffix)
		out = append(out, SubagentRun{
			ID: id, AgentType: w.AgentType, Name: w.Name, Description: w.Description, Model: w.Model, TeamName: w.TeamName,
			TaskKind: w.TaskKind, IsBuiltIn: w.IsBuiltIn, NonInteractive: w.RequestNonInteractive, PlanModeRequired: w.PlanModeRequired,
			SpawnDepth: w.SpawnDepth, PermissionMode: w.PermissionMode, Transcript: filepath.Join(sub, id+transcriptExt), HasMeta: true, Unknown: unknown,
		})
	}
	return out, nil
}
