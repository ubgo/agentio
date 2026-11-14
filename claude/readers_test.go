package claude

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/khanakia/agentcli/agenttest"
)

// newReadersAgent copies the fixture into a temp user home and returns the
// agent, its default home and the fixture project.
func newReadersAgent(t *testing.T, fixture string, env ...string) (*Agent, Home, Project) {
	t.Helper()
	host := agenttest.NewHost(t)
	agenttest.CopyFixture(t, fixture, host.UserHome)
	host.Env = append(host.Env, env...)
	a := New(host)
	p, err := a.Project(filepath.Join(host.UserHome, "project"))
	if err != nil {
		t.Fatal(err)
	}
	return a, a.DefaultHome(), p
}

func TestReadersPlugins(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, h, p := newReadersAgent(t, fixture)
		agenttest.WriteFile(t, p.Dir, ".claude/settings.local.json", `{"enabledPlugins": {"demo-plugin@claude-plugins-official": true}}`)
		ps, err := a.Plugins(h, &p)
		if err != nil || len(ps) != 1 {
			t.Fatalf("Plugins = %v, %v", ps, err)
		}
		pl := ps[0]
		if pl.ID != "demo-plugin@claude-plugins-official" || pl.Version != "1.0.0" || !pl.Enabled || !pl.ProjectEnabled || pl.Scope != PluginScopeUser {
			t.Fatalf("plugin = %+v", pl)
		}
		if filepath.Base(pl.DataDir) != "demo-plugin-claude-plugins-official" {
			t.Fatalf("DataDir = %s", pl.DataDir)
		}
		ms, err := a.Marketplaces(h)
		if err != nil || len(ms) != 1 || ms[0].Source.Kind != MarketplaceGitHub || ms[0].Declared {
			t.Fatalf("Marketplaces = %+v, %v", ms, err)
		}
		if bl, err := a.PluginBlocklist(h); err != nil || bl.FetchedAt != nil {
			t.Fatalf("blocklist = %+v, %v", bl, err)
		}
		dir := t.TempDir()
		agenttest.WriteFile(t, dir, ".claude-plugin/plugin.json", `{"name":"x","version":"1","skills":"./skills","agents":["a"],"homepage":"h"}`)
		m, err := a.PluginManifest(dir)
		if err != nil || m.Name != "x" || len(m.Skills) != 1 || len(m.Agents) != 1 || m.Unknown["homepage"] == nil {
			t.Fatalf("manifest = %+v, %v", m, err)
		}
	})
}

func TestReadersPluginsUnknownVersion(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, h, _ := newReadersAgent(t, fixture)
		agenttest.WriteFile(t, h.Dir(), "plugins/installed_plugins.json", `{"version": 3, "plugins": {}}`)
		if _, err := a.Plugins(h, nil); err == nil || !strings.Contains(err.Error(), "version 3") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestReadersSkills(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, h, p := newReadersAgent(t, fixture)
		ss, err := a.Skills(h, &p)
		if err != nil || len(ss) != 2 {
			t.Fatalf("Skills = %+v, %v", ss, err)
		}
		s := ss[0]
		if s.Name != "demo-skill" || s.Scope != SkillScopeUser || s.ArgumentHint != "[target]" || s.Model != "opus" {
			t.Fatalf("skill = %+v", s)
		}
		if !strings.HasPrefix(s.Description, "A fixture skill folded") || len(s.AllowedTools) != 2 || s.UserInvocable == nil || !*s.UserInvocable {
			t.Fatalf("skill fields = %+v", s)
		}
		if s.Metadata["owner"] != "fixture" || s.Metadata["tier"] != "2" || string(s.Unknown["license"]) != `"MIT"` {
			t.Fatalf("metadata/unknown = %v %v", s.Metadata, s.Unknown)
		}
		if ss[1].Name != "proj-skill" || ss[1].Scope != SkillScopeProject {
			t.Fatalf("project skill = %+v", ss[1])
		}
		cs, err := a.SlashCommands(h, &p)
		if err != nil || len(cs) != 1 || cs[0].Name != "hello" || cs[0].Description != "Say hello" {
			t.Fatalf("commands = %+v, %v", cs, err)
		}
		agenttest.WriteFile(t, h.Dir(), "workflows/ship.js", "export default {}")
		ws, err := a.Workflows(h, &p)
		if err != nil || len(ws) != 1 || ws[0].Name != "ship" {
			t.Fatalf("workflows = %+v, %v", ws, err)
		}
	})
}

func TestReadersPluginSkills(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, h, _ := newReadersAgent(t, fixture)
		plug := t.TempDir()
		agenttest.WriteFile(t, plug, "skills/plug-skill/SKILL.md", "---\nname: plug-skill\ndescription: from a plugin\n---\n")
		agenttest.WriteFile(t, h.Dir(), "plugins/installed_plugins.json",
			`{"version":2,"plugins":{"p@m":[{"scope":"user","installPath":"`+plug+`","version":"1"}]}}`)
		ss, err := a.Skills(h, nil)
		if err != nil {
			t.Fatal(err)
		}
		var found bool
		for _, s := range ss {
			if s.Name == "plug-skill" && s.Scope == SkillScopePlugin && s.Plugin == "p@m" {
				found = true
			}
		}
		if !found {
			t.Fatalf("plugin skill missing: %+v", ss)
		}
	})
}

func TestReadersMemory(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, h, p := newReadersAgent(t, fixture)
		ms, err := a.MemoryFiles(h, p, false)
		if err != nil {
			t.Fatal(err)
		}
		kinds := map[MemoryKind]int{}
		for _, m := range ms {
			kinds[m.Kind]++
		}
		if kinds[MemoryUser] != 1 || kinds[MemoryUserRule] != 1 || kinds[MemoryProject] != 1 {
			t.Fatalf("kinds = %v (%+v)", kinds, ms)
		}
		var user, rule MemoryFile
		for _, m := range ms {
			switch m.Kind {
			case MemoryUser:
				user = m
			case MemoryUserRule:
				rule = m
			}
		}
		if len(user.Imports) != 2 || !user.Imports[0].Exists || user.Imports[1].Exists || user.Imports[0].Line != 4 {
			t.Fatalf("imports = %+v", user.Imports)
		}
		if len(rule.PathsGlob) != 1 || rule.PathsGlob[0] != "src/**/*.go" {
			t.Fatalf("rule paths = %v", rule.PathsGlob)
		}
		followed, err := a.MemoryFiles(h, p, true)
		if err != nil {
			t.Fatal(err)
		}
		// rules/style.md is already listed as a user rule, so it is not repeated.
		if len(followed) != len(ms) {
			t.Fatalf("followed %d vs %d", len(followed), len(ms))
		}
		agenttest.WriteFile(t, a.ProjectPaths(h, p).AutoMemory, "MEMORY.md", "- [x](x.md)\n")
		am, err := a.AutoMemory(h, p)
		if err != nil || len(am) != 1 || am[0].Kind != MemoryAuto {
			t.Fatalf("auto = %+v, %v", am, err)
		}
	})
}

func TestReadersParseImports(t *testing.T) {
	imps := ParseImports([]byte("@a.md\n```\n@skip.md\n```\n  @~/b.md trailing\nemail@x.com\n"))
	if len(imps) != 2 || imps[0].Raw != "a.md" || imps[1].Raw != "~/b.md" || imps[1].Line != 5 {
		t.Fatalf("imports = %+v", imps)
	}
}

func TestReadersSubagents(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, h, p := newReadersAgent(t, fixture)
		ds, err := a.SubagentDefs(h, &p)
		if err != nil || len(ds) != 1 {
			t.Fatalf("defs = %+v, %v", ds, err)
		}
		d := ds[0]
		if d.Name != "reviewer" || d.Model != "sonnet" || d.PermissionMode != ModePlan || d.MaxTurns == nil || *d.MaxTurns != 12 ||
			len(d.Tools) != 3 || d.Prompt != "You review code." || d.Color != "blue" {
			t.Fatalf("def = %+v", d)
		}
		sub := filepath.Join(a.HistoryDir(h, "/work/proj1"), "sess-1", subagentsDir)
		agenttest.WriteFile(t, sub, "agent-a1.meta.json", `{"agentType":"Explore","isBuiltIn":true,"spawnDepth":1,"permissionMode":"default","futureKey":1}`)
		rs, err := a.SubagentRuns(h, "/work/proj1", "sess-1")
		if err != nil || len(rs) != 1 || rs[0].ID != "agent-a1" || rs[0].PermissionMode != ModeManual || rs[0].Unknown["futureKey"] == nil {
			t.Fatalf("runs = %+v, %v", rs, err)
		}
	})
}

func TestReadersOutputStyles(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, h, p := newReadersAgent(t, fixture)
		agenttest.WriteFile(t, p.Dir, ".claude/output-styles/terse.md", "---\ndescription: short\nkeep-coding-instructions: true\n---\nBe terse.\n")
		os, err := a.OutputStyles(h, &p)
		if err != nil || len(os) != 1 || os[0].Name != "terse" || os[0].Scope != SkillScopeProject || os[0].KeepCodingInstructions == nil {
			t.Fatalf("styles = %+v, %v", os, err)
		}
	})
}

func TestReadersStatusLine(t *testing.T) {
	in, err := DecodeStatusLineInput(strings.NewReader(`{"session_id":"s","model":{"id":"m","display_name":"M"},"workspace":{"current_dir":"/w"},"newKey":true}`))
	if err != nil || in.Model.DisplayName != "M" || in.Workspace.CurrentDir != "/w" || in.Unknown["newKey"] == nil {
		t.Fatalf("input = %+v, %v", in, err)
	}
}

func TestReadersEnv(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, h, p := newReadersAgent(t, fixture, APIKeyEnv+"=sk-ant-api-FIXTURE", "UNRELATED=1")
		es, err := a.EnvInEffect(h, &p)
		if err != nil {
			t.Fatal(err)
		}
		var proc, local bool
		for _, e := range es {
			if e.Name == APIKeyEnv && e.From == EnvFromProcess && e.Secret && e.Value.String() != "sk-ant-api-FIXTURE" {
				proc = true
			}
			if e.Name == "FIXTURE_FLAG" && e.From == ScopeLocal && e.Value.Reveal() == "1" {
				local = true
			}
			if e.Name == "UNRELATED" {
				t.Fatalf("unknown process var listed")
			}
		}
		if !proc || !local {
			t.Fatalf("env = %+v", es)
		}
		if len(KnownEnv()) == 0 || KnownEnv()[0].Name != ConfigDirEnv {
			t.Fatal("KnownEnv table")
		}
		if es[0].Value.String() == "1" {
			t.Fatal("env value not redacted")
		}
	})
}
