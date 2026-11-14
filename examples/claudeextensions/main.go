// Command claudeextensions lists what extends Claude Code in a home and a
// project: MCP servers, memory (instruction) files, skills, slash commands,
// workflows, plugins and marketplaces, subagents, output styles, and the
// local data Claude Code keeps beside them (file checkpoints, shell
// snapshots, IDE locks, debug logs, disk usage). It also builds the vendor
// commands that change MCP servers and plugins, without running them.
//
// It never reads the real machine: run copies the 2.1.325 fixture home into
// a temp dir, adds the files the fixture lacks, and points a hand-built Host
// at it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/frontmatter"
)

// Fixture locations, relative to the examples module root (where
// `go run ./claudeextensions` runs).
const (
	defaultFixture = "../claude/testdata/homes/2.1.325/userhome"
	defaultStream  = "../claude/testdata/stream/2.1.325/result.json"
)

// Values the example uses.
const (
	fixtureProject = "project"
	fixtureUser    = "user"
	pluginID       = claude.PluginID("demo-plugin@claude-plugins-official")
	historyFolder  = "/work/proj12"
	sessionID      = "f85efbdb-fc2f-1e8e-7748-9f915edc8e2d"
	checkpointSess = "s1"
	dirPerm        = 0o700
	filePerm       = 0o600
	homeTilde      = "~"
)

// fixedNow is the Host clock.
func fixedNow() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) }

func main() {
	fixture := flag.String("fixture", defaultFixture, "fixture user home to copy")
	stream := flag.String("result", defaultStream, "recorded --output-format json result")
	flag.Parse()
	if err := run(os.Stdout, *fixture, *stream); err != nil {
		fmt.Fprintln(os.Stderr, "claudeextensions:", err)
		os.Exit(1)
	}
}

// run copies fixture into a temp user home, seeds it and prints every
// section. result is a recorded `--output-format json` document.
func run(w io.Writer, fixture, result string) error {
	tmp, err := os.MkdirTemp("", "claudeextensions-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }() // temp copy; nothing to keep
	userHome := filepath.Join(tmp, "home")   // CopyTree refuses an existing dst
	if err := agentio.CopyTree(fixture, userHome); err != nil {
		return err
	}
	a := claude.New(fixtureHost(userHome))
	proj, err := a.Project(filepath.Join(userHome, fixtureProject))
	if err != nil {
		return err
	}
	if err := seed(a, proj); err != nil {
		return err
	}
	p := printer{w: w, home: userHome}
	steps := []func(printer, *claude.Agent, claude.Project) error{
		showMCP, showMCPNeedsAuth, showMCPCommands,
		showMemory, showAutoMemory, showImports,
		showSkills, showSlashCommands, showWorkflows, showFrontmatter,
		showPlugins, showMarketplaces, showManifest, showPluginCommands,
		showSubagentDefs, showSubagentRuns, showAgentsFlag, showOutputStyles,
		showFileHistory, showSnapshots, showIDEAndDebug, showDataUsage,
	}
	for _, step := range steps {
		if err := step(p, a, proj); err != nil {
			return err
		}
	}
	return showSubagentStats(p, result)
}

// fixtureHost points every Host input at the temp user home; no network.
func fixtureHost(userHome string) agentio.Host {
	return agentio.Host{
		UserHome: userHome,
		User:     fixtureUser,
		Env:      []string{"HOME=" + userHome, "PATH="},
		Now:      fixedNow,
		HTTP:     &http.Client{Transport: noNetwork{}},
	}
}

// noNetwork fails every request: nothing in this example may go online.
type noNetwork struct{}

// errNoNetwork is what noNetwork returns.
var errNoNetwork = errors.New("claudeextensions: network disabled in this example")

// RoundTrip refuses every request, so a read that would reach the network
// fails loudly instead of leaving the machine.
func (noNetwork) RoundTrip(*http.Request) (*http.Response, error) { return nil, errNoNetwork }

// seed writes the files the fixture home lacks, in the shapes Claude Code
// writes them: a plugin with a manifest and components, auto memory, output
// styles, workflows, a subagent run, the MCP needs-auth cache and the local
// data directories.
func seed(a *claude.Agent, proj claude.Project) error {
	h := a.DefaultHome()
	home := h.Dir()
	plugin := filepath.Join(home, "plugins", "cache", "claude-plugins-official", "demo-plugin", "1.0.0")
	files := map[string]string{
		// The fixture's installed_plugins.json names a path on another
		// machine; point it at a plugin dir inside this temp home.
		filepath.Join(home, "plugins", "installed_plugins.json"):                                    `{"version": 2, "plugins": {"` + string(pluginID) + `": [{"scope": "user", "installPath": "` + plugin + `", "version": "1.0.0", "installedAt": "2026-11-01T10:00:00.000Z", "lastUpdated": "2026-11-02T10:00:00.000Z"}]}}`,
		filepath.Join(plugin, ".claude-plugin", "plugin.json"):                                      `{"name": "demo-plugin", "version": "1.0.0", "description": "A demo plugin", "skills": "./skills", "agents": ["./agents/helper.md"], "homepage": "https://example.com"}`,
		filepath.Join(plugin, "skills", "plug-skill", "SKILL.md"):                                   "---\nname: plug-skill\ndescription: Skill shipped by a plugin\n---\n",
		filepath.Join(plugin, "commands", "deploy.md"):                                              "---\ndescription: Deploy the app\n---\nDeploy.\n",
		filepath.Join(home, "plugins", "blocklist.json"):                                            `{"fetchedAt": "2026-11-13T10:00:00Z", "plugins": []}`,
		filepath.Join(home, "notes-by-another-tool.txt"):                                            "made by another tool\n",
		filepath.Join(home, "output-styles", "terse.md"):                                            "---\ndescription: Short answers\nkeep-coding-instructions: true\n---\nBe terse.\n",
		filepath.Join(proj.Dir, ".claude", "output-styles", "teach.md"):                             "---\nname: Teacher\ndescription: Explains every step\n---\nExplain.\n",
		filepath.Join(home, "workflows", "release.js"):                                              "export default {}\n",
		filepath.Join(proj.Dir, ".claude", "workflows", "triage.js"):                                "export default {}\n",
		filepath.Join(home, "mcp-needs-auth-cache.json"):                                            `{"fixture-http": {"timestamp": 1794000000000}}`,
		filepath.Join(a.ProjectPaths(h, proj).AutoMemory, "MEMORY.md"):                              "- [Build](build.md): how the build runs\n",
		filepath.Join(a.ProjectPaths(h, proj).AutoMemory, "build.md"):                               "Run task build.\n",
		filepath.Join(a.HistoryDir(h, historyFolder), sessionID, "subagents", "agent-a1.meta.json"): `{"agentType":"Explore","description":"Find the config loader","isBuiltIn":true,"spawnDepth":1,"permissionMode":"default","futureKey":1}`,
		filepath.Join(home, "file-history", checkpointSess, "abc123@v2"):                            "previous file body",
		filepath.Join(home, "shell-snapshots", "snapshot-zsh-1794000000000-abcd.sh"):                "# shell state\n",
		filepath.Join(home, "ide", "41234.lock"):                                                    `{"pid":5,"workspaceFolders":["/work/project"],"ideName":"VS Code","transport":"ws","authToken":"fixture-secret"}`,
		filepath.Join(home, "debug", "s1.txt"):                                                      "debug log\n",
		filepath.Join(home, "cache", "changelog.md"):                                                "# Changelog\n",
		filepath.Join(home, "cache", "model-catalog", "models.json"):                                "{}",
	}
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(body), filePerm); err != nil {
			return err
		}
	}
	// debug/latest points at the newest log, as Claude Code keeps it.
	return os.Symlink(filepath.Join(home, "debug", "s1.txt"), filepath.Join(home, "debug", "latest"))
}

// printer writes lines with the temp user home shown as "~".
type printer struct {
	w    io.Writer
	home string
}

// line prints one line (callers format it with fmt.Sprintf) with the home
// folder shown as ~.
// A write error on the output (a closed pipe) is dropped: the example has
// nothing better to do with it than stop printing.
func (p printer) line(s string) {
	_, _ = fmt.Fprintln(p.w, strings.ReplaceAll(s, p.home, homeTilde))
}

// section prints a blank line and a "== title" heading.
func (p printer) section(title string) { p.line("\n== " + title) }

// snippet:start mcp
// showMCP lists every file-configured MCP server with its scope and
// approval. Listing never starts a server; env and header values print
// redacted.
func showMCP(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("MCP servers")
	servers, err := a.MCPServers(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, s := range servers {
		p.line(fmt.Sprintf("%-13s scope=%-7s transport=%-5s approval=%q env=%v headers=%v", s.Name, s.Scope, s.Transport, s.Approval, s.Env, s.Headers))
	}
	parsed, err := claude.ParseMCPConfig([]byte(`{"mcpServers": {"docs": {"type": "http", "url": "https://mcp.example.com/docs", "oauth": {"clientId": "docs-cli", "callbackPort": 8765}}}}`))
	if err != nil {
		return err
	}
	docs := parsed["docs"]
	p.line(fmt.Sprintf("ParseMCPConfig: docs transport=%s oauth client=%s port=%d", docs.Transport, docs.OAuth.ClientID, docs.OAuth.CallbackPort))
	return nil
}

// snippet:end mcp

// snippet:start needsauth
// showMCPNeedsAuth reads which servers Claude Code saw asking for a login.
func showMCPNeedsAuth(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("MCP servers needing auth")
	seen, err := a.MCPNeedsAuth(a.DefaultHome()) // nil when the cache file is absent
	if err != nil {
		return err
	}
	for name, at := range seen {
		p.line(fmt.Sprintf("%s needs auth since %s", name, at.UTC().Format(time.RFC3339)))
	}
	return nil
}

// snippet:end needsauth

// snippet:start mcpcommands
// showMCPCommands builds the `claude mcp` commands that change servers.
// They are data: print them, or run them yourself.
func showMCPCommands(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("MCP commands")
	h := a.DefaultHome()
	add, err := a.MCPAddCommand(h, proj.Dir, claude.MCPServer{
		Name: "docs", Scope: claude.MCPScopeUser, Transport: claude.TransportHTTP, URL: "https://mcp.example.com/docs",
	})
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("add:    %s", add))
	p.line(fmt.Sprintf("remove: %s", a.MCPRemoveCommand(h, proj.Dir, claude.MCPScopeUser, "docs")))
	p.line(fmt.Sprintf("list:   %s", a.MCPListCommand(h, proj.Dir)))
	p.line(fmt.Sprintf("reset:  %s", a.MCPResetProjectChoicesCommand(h, proj.Dir)))
	_, err = a.MCPAddCommand(h, proj.Dir, claude.MCPServer{
		Name: "db", Scope: claude.MCPScopeUser, Command: "db-mcp",
		Env: map[string]agentio.EnvValue{"DB_PASSWORD": agentio.NewEnvValue("hunter2")},
	})
	p.line(fmt.Sprintf("server with env values refused: %v", err))
	return nil
}

// snippet:end mcpcommands

// snippet:start memory
// showMemory lists the instruction files Claude Code loads for the project,
// in load order, with the @imports each one names.
func showMemory(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("memory files")
	files, err := a.MemoryFiles(a.DefaultHome(), proj, false) // false: list imports, do not expand them
	if err != nil {
		return err
	}
	for _, f := range files {
		p.line(fmt.Sprintf("%-12s %s size=%d paths=%v imports=%d", f.Kind, f.Path, f.Size, f.PathsGlob, len(f.Imports)))
	}
	return nil
}

// snippet:end memory

// snippet:start automemory
// showAutoMemory lists the notes Claude Code saved for this project itself.
func showAutoMemory(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("auto memory")
	notes, err := a.AutoMemory(a.DefaultHome(), proj)
	if err != nil {
		return err
	}
	// The folder is <home>/projects/<EncodePath(project dir)>/memory; the
	// encoded name holds the temp dir, so it is shown symbolically.
	dir := a.ProjectPaths(a.DefaultHome(), proj).AutoMemory
	p.line(fmt.Sprintf("dir=%s", strings.Replace(dir, claude.EncodePath(proj.Dir), "<EncodePath(project dir)>", 1)))
	for _, n := range notes {
		p.line(fmt.Sprintf("%s %s size=%d", n.Kind, filepath.Base(n.Path), n.Size))
	}
	return nil
}

// snippet:end automemory

// snippet:start imports
// showImports parses @path lines from a memory body and shows how the
// listed user CLAUDE.md resolves its own imports.
func showImports(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("imports")
	body := "See @docs/build.md\n```\n@not-an-import.md\n```\n  @~/notes.md\nmail me at user@example.com\n"
	for _, imp := range claude.ParseImports([]byte(body)) {
		p.line(fmt.Sprintf("line %d raw=%s", imp.Line, imp.Raw))
	}
	files, err := a.MemoryFiles(a.DefaultHome(), proj, true) // true: follow imports, depth-capped
	if err != nil {
		return err
	}
	for _, f := range files {
		for _, imp := range f.Imports {
			p.line(fmt.Sprintf("%s imports %s -> %s exists=%t", filepath.Base(f.Path), imp.Raw, imp.Resolved, imp.Exists))
		}
	}
	return nil
}

// snippet:end imports

// snippet:start skills
// showSkills lists user, project and plugin skills from their frontmatter.
func showSkills(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("skills")
	skills, err := a.Skills(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, s := range skills {
		p.line(fmt.Sprintf("%-10s scope=%-7s plugin=%q model=%q tools=%v desc=%q", s.Name, s.Scope, s.Plugin, s.Model, s.AllowedTools, s.Description))
	}
	return nil
}

// snippet:end skills

// snippet:start commands
// showSlashCommands lists commands/*.md of the user, project and plugins.
func showSlashCommands(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("slash commands")
	cmds, err := a.SlashCommands(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, c := range cmds {
		p.line(fmt.Sprintf("/%s scope=%s hint=%q desc=%q", c.Name, c.Scope, c.ArgumentHint, c.Description))
	}
	return nil
}

// snippet:end commands

// snippet:start workflows
// showWorkflows lists workflows/*.js, each of which becomes /<name>.
func showWorkflows(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("workflows")
	flows, err := a.Workflows(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, f := range flows {
		p.line(fmt.Sprintf("/%s scope=%s file=%s", f.Name, f.Scope, f.File))
	}
	return nil
}

// snippet:end workflows

// snippet:start frontmatter
// showFrontmatter parses a SKILL.md the way the listers do, then reads the
// same bytes with the frontmatter package directly.
func showFrontmatter(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("frontmatter")
	data, err := os.ReadFile(filepath.Join(a.Paths(a.DefaultHome()).Skills, "demo-skill", "SKILL.md"))
	if err != nil {
		return err
	}
	doc, body, err := claude.ParseFrontmatter(data)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("keys=%v", doc.Keys()))
	p.line(fmt.Sprintf("description=%q", doc.String("description")))
	p.line(fmt.Sprintf("allowed-tools=%v owner=%s", doc.List("allowed-tools"), doc.Map("metadata").String("owner")))
	if v, ok := doc.Bool("user-invocable"); ok {
		p.line(fmt.Sprintf("user-invocable=%t", v))
	}
	p.line(fmt.Sprintf("body starts %q", strings.SplitN(strings.TrimSpace(string(body)), "\n", 2)[0]))

	front, _, ok := frontmatter.Split([]byte("---\ntools: Read, Grep\n---\nbody\n"))
	if !ok {
		return errors.New("no frontmatter block")
	}
	d, err := frontmatter.ParseYAML(front)
	if err != nil {
		return err
	}
	tools, _ := d.Get("tools")
	p.line(fmt.Sprintf("Split+ParseYAML: tools kind=%s text=%q List=%v", tools.Kind, tools.Text, d.List("tools")))
	whole, _, err := frontmatter.Parse([]byte("no frontmatter here\n"))
	p.line(fmt.Sprintf("Parse without a block: fields=%d err=%v", len(whole.Fields), err))
	return nil
}

// snippet:end frontmatter

// snippet:start plugins
// showPlugins lists installed plugins with their enablement.
func showPlugins(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("plugins")
	plugins, err := a.Plugins(a.DefaultHome(), &proj) // another file version is ErrUnknownVersion
	if err != nil {
		return err
	}
	for _, pl := range plugins {
		name, market := pl.ID.Split()
		p.line(fmt.Sprintf("%s (plugin=%s marketplace=%s) v%s scope=%s enabled=%t", pl.ID, name, market, pl.Version, pl.Scope, pl.Enabled))
		p.line(fmt.Sprintf("  install=%s data=%s", pl.InstallPath, pl.DataDir))
	}
	return nil
}

// snippet:end plugins

// snippet:start marketplaces
// showMarketplaces lists cloned and declared marketplaces and the blocklist.
func showMarketplaces(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("marketplaces and blocklist")
	h := a.DefaultHome()
	markets, err := a.Marketplaces(h)
	if err != nil {
		return err
	}
	for _, m := range markets {
		p.line(fmt.Sprintf("%s source=%s repo=%s declared=%t updated=%s", m.Name, m.Source.Kind, m.Source.Repo, m.Declared, m.LastUpdated.Format(time.RFC3339)))
	}
	bl, err := a.PluginBlocklist(h) // zero value when the file is absent
	if err != nil {
		return err
	}
	if bl.FetchedAt != nil {
		p.line(fmt.Sprintf("blocklist fetched %s entries=%s", bl.FetchedAt.Format(time.RFC3339), bl.Plugins))
	}
	return nil
}

// snippet:end marketplaces

// snippet:start manifest
// showManifest reads the installed plugin's .claude-plugin/plugin.json.
func showManifest(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("plugin manifest")
	plugins, err := a.Plugins(a.DefaultHome(), &proj)
	if err != nil || len(plugins) == 0 {
		return err
	}
	m, err := a.PluginManifest(plugins[0].InstallPath)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("name=%s version=%s skills=%v agents=%v unknown=%v", m.Name, m.Version, m.Skills, m.Agents, m.Unknown.Keys()))
	return nil
}

// snippet:end manifest

// snippet:start plugincommands
// showPluginCommands builds the `claude plugin` commands; nothing runs.
func showPluginCommands(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("plugin commands")
	h := a.DefaultHome()
	p.line(a.PluginInstallCommand(h, pluginID, claude.PluginScopeProject).String())
	p.line(a.PluginEnableCommand(h, pluginID).String())
	p.line(a.PluginDisableCommand(h, pluginID).String())
	p.line(a.MarketplaceAddCommand(h, "anthropics/claude-plugins-official").String())
	p.line(a.MarketplaceUpdateCommand(h, "claude-plugins-official").String())
	return nil
}

// snippet:end plugincommands

// snippet:start subagentdefs
// showSubagentDefs lists agents/*.md definitions.
func showSubagentDefs(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("subagent definitions")
	defs, err := a.SubagentDefs(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, d := range defs {
		turns := "unset"
		if d.MaxTurns != nil { // nil: the file sets no limit
			turns = fmt.Sprint(*d.MaxTurns)
		}
		p.line(fmt.Sprintf("%s scope=%s model=%s mode=%s tools=%v max turns=%s prompt=%q", d.Name, d.Scope, d.Model, d.PermissionMode, d.Tools, turns, d.Prompt))
	}
	return nil
}

// snippet:end subagentdefs

// snippet:start subagentruns
// showSubagentRuns lists the subagents one session started.
func showSubagentRuns(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("subagent runs")
	runs, err := a.SubagentRuns(a.DefaultHome(), historyFolder, sessionID)
	if err != nil {
		return err
	}
	for _, r := range runs {
		p.line(fmt.Sprintf("%s type=%s builtin=%t depth=%d mode=%s desc=%q unknown=%v", r.ID, r.AgentType, r.IsBuiltIn, r.SpawnDepth, r.PermissionMode, r.Description, r.Unknown.Keys()))
	}
	return nil
}

// snippet:end subagentruns

// snippet:start subagentstats
// showSubagentStats reads the subagent tally of a finished headless run.
func showSubagentStats(p printer, result string) error {
	p.section("subagent stats of a run")
	f, err := os.Open(result)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }() // read-only; a close error changes nothing
	res, err := claude.DecodeJSONResult(f)
	if err != nil {
		return err
	}
	if s := res.SubagentStats; s != nil {
		p.line(fmt.Sprintf("spawned=%d completed=%d failed=%d max depth=%d", s.Spawned, s.Completed, s.Failed, s.MaxDepth))
	}
	return nil
}

// snippet:end subagentstats

// snippet:start agentsflag
// showAgentsFlag renders subagent definitions as the --agents JSON, for a
// session that should get them without files on disk.
func showAgentsFlag(p printer, _ *claude.Agent, _ claude.Project) error {
	p.section("--agents flag")
	flagValue, err := claude.EncodeAgentsFlag(map[string]claude.SubagentDef{
		"tester": {Description: "Runs the tests", Prompt: "Run go test and report failures.", Tools: []string{"Bash", "Read"}, Model: "sonnet"},
	})
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("--agents %s", flagValue))
	return nil
}

// snippet:end agentsflag

// snippet:start outputstyles
// showOutputStyles lists output-styles/*.md of the user, project and plugins.
func showOutputStyles(p printer, a *claude.Agent, proj claude.Project) error {
	p.section("output styles")
	styles, err := a.OutputStyles(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, s := range styles {
		keep := "unset"
		if s.KeepCodingInstructions != nil {
			keep = fmt.Sprint(*s.KeepCodingInstructions)
		}
		p.line(fmt.Sprintf("%-8s scope=%-7s keep coding instructions=%s desc=%q", s.Name, s.Scope, keep, s.Description))
	}
	return nil
}

// snippet:end outputstyles

// snippet:start filehistory
// showFileHistory lists the pre-edit copies one session saved.
func showFileHistory(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("file history")
	cps, err := a.FileHistory(a.DefaultHome(), checkpointSess)
	if err != nil {
		return err
	}
	for _, c := range cps {
		p.line(fmt.Sprintf("session=%s file hash=%s version=%d size=%d", c.Session, c.FileHash, c.Version, c.Size))
	}
	return nil
}

// snippet:end filehistory

// snippet:start snapshots
// showSnapshots lists shell snapshots and when cleanup last ran.
func showSnapshots(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("shell snapshots and cleanup")
	h := a.DefaultHome()
	snaps, err := a.ShellSnapshots(h) // oldest first
	if err != nil {
		return err
	}
	for _, s := range snaps {
		p.line(fmt.Sprintf("shell=%s created=%s", s.Shell, s.Created.UTC().Format(time.RFC3339)))
	}
	last, err := a.LastCleanup(h) // nil when .last-cleanup is absent
	if err != nil {
		return err
	}
	if last != nil {
		p.line(fmt.Sprintf("last cleanup %s", last.UTC().Format(time.RFC3339)))
	}
	return nil
}

// snippet:end snapshots

// snippet:start ide
// showIDEAndDebug reads IDE lock files and lists debug logs.
func showIDEAndDebug(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("IDE locks and debug logs")
	h := a.DefaultHome()
	locks, err := a.IDELocks(h)
	if err != nil {
		return err
	}
	for _, l := range locks {
		p.line(fmt.Sprintf("ide=%s port=%d pid=%d workspace=%v transport=%s unknown=%v", l.IDEName, l.Port, l.PID, l.Workspace, l.Transport, l.Unknown.Keys()))
	}
	logs, err := a.DebugLogs(h)
	if err != nil {
		return err
	}
	for _, l := range logs {
		p.line(fmt.Sprintf("debug session=%s size=%d latest=%t", l.Session, l.Size, l.Latest))
	}
	return nil
}

// snippet:end ide

// snippet:start datausage
// showDataUsage sizes every top-level entry of the home (this walks the
// whole tree: slow on a real home), plus the cached catalog and changelog.
func showDataUsage(p printer, a *claude.Agent, _ claude.Project) error {
	p.section("data usage")
	h := a.DefaultHome()
	entries, err := a.DataUsage(h)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
		if e.Foreign { // Claude Code neither documents nor creates it; never parsed
			p.line(fmt.Sprintf("foreign entry %s (%d bytes)", e.Name, e.Bytes))
		}
	}
	p.line(fmt.Sprintf("entries=%v", names))
	catalog, err := a.ModelCatalogFiles(h)
	if err != nil {
		return err
	}
	for _, f := range catalog {
		p.line(fmt.Sprintf("model catalog file %s", f))
	}
	changelog, err := a.Changelog(h)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("changelog first line %q", strings.SplitN(changelog, "\n", 2)[0]))
	return nil
}

// snippet:end datausage
