// Command claudesettings walks the read-only side of the claude module on a
// copy of the 2.1.325 fixture home: the agent and its homes, the install,
// paths, every settings layer and their merge, the environment in effect,
// the global state file (.claude.json), permissions and policy limits.
//
// It never reads the real machine: run copies the fixture into a temp dir,
// adds a fake native install and a few files the fixture lacks, and points a
// hand-built Host at it.
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
)

// defaultFixture is the fixture user home, relative to the examples module
// root (where `go run ./claudesettings` runs).
const defaultFixture = "../claude/testdata/homes/2.1.325/userhome"

// Fixture layout and fake values the example adds or relies on.
const (
	fixtureProject  = "project"    // the fixture's project folder under the user home
	fixtureUser     = "user"       // OS login name the Host reports
	nativeVersion   = "2.1.325"    // version dir name of the fake native install
	nativeLocalBin  = ".local/bin" // where the native installer links claude
	nativeVersions  = ".local/share/claude/versions"
	stampFile       = "policy-limits.json.stamp.json"
	backupName      = ".claude.json.backup.1794000000000"
	backupsDir      = "backups"
	historyFolder   = "/work/proj12" // a folder with a conversation in the fixture
	linuxGOOS       = "linux"        // the OS a modelled real host reports
	unixSystemRoot  = "/"            // the system root HostFromOS sets on unix
	privateDirPerm  = 0o700
	privateFilePerm = 0o600
	execPerm        = 0o700
	homeTilde       = "~"
)

// fixedNow is the clock the Host reports, so output never depends on today.
func fixedNow() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) }

func main() {
	fixture := flag.String("fixture", defaultFixture, "fixture user home to copy")
	flag.Parse()
	if err := run(os.Stdout, *fixture); err != nil {
		fmt.Fprintln(os.Stderr, "claudesettings:", err)
		os.Exit(1)
	}
}

// run copies fixture into a temp user home and prints every section.
func run(w io.Writer, fixture string) error {
	tmp, err := os.MkdirTemp("", "claudesettings-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }() // temp copy; nothing to keep
	userHome := filepath.Join(tmp, "home")   // CopyTree refuses an existing dst
	if err := agentio.CopyTree(fixture, userHome); err != nil {
		return err
	}
	if err := seed(userHome); err != nil {
		return err
	}
	host := fixtureHost(userHome)
	p := printer{w: w, home: userHome}
	steps := []func(printer, agentio.Host) error{
		showAgent, showHomes, showProject, showNeutral, showInstall, showPaths, showSettingsLayers, showEffectiveSettings,
		showManagedSettings, showEnv, showGlobalConfig, showProjectState,
		showConfigBackups, showPermissions, showRules, showPolicy,
	}
	for _, step := range steps {
		if err := step(p, host); err != nil {
			return err
		}
	}
	return nil
}

// fixtureHost is a Host whose every input points at the temp user home: no
// real env, no network (a transport that refuses), a fixed clock.
func fixtureHost(userHome string) agentio.Host {
	return agentio.Host{
		UserHome: userHome,
		User:     fixtureUser,
		Env: []string{
			"HOME=" + userHome,
			"PATH=" + filepath.Join(userHome, nativeLocalBin),
		},
		Now:  fixedNow,
		HTTP: &http.Client{Transport: noNetwork{}},
	}
}

// noNetwork fails every request: nothing in this example may go online.
type noNetwork struct{}

// errNoNetwork is what noNetwork returns.
var errNoNetwork = errors.New("claudesettings: network disabled in this example")

// RoundTrip refuses every request, so a read that would reach the network
// fails loudly instead of leaving the machine.
func (noNetwork) RoundTrip(*http.Request) (*http.Response, error) { return nil, errNoNetwork }

// seed adds what the fixture does not carry: a fake native install (a
// versions file and the ~/.local/bin/claude symlink the installer makes),
// a policy stamp and a .claude.json backup.
func seed(userHome string) error {
	versions := filepath.Join(userHome, nativeVersions)
	bin := filepath.Join(userHome, nativeLocalBin)
	for _, d := range []string{versions, bin, filepath.Join(userHome, ".claude", backupsDir)} {
		if err := os.MkdirAll(d, privateDirPerm); err != nil {
			return err
		}
	}
	exe := filepath.Join(versions, nativeVersion)
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), execPerm); err != nil {
		return err
	}
	if err := os.Symlink(exe, filepath.Join(bin, claude.Binary)); err != nil {
		return err
	}
	files := map[string]string{
		filepath.Join(".claude", stampFile):              `{"v":1,"kind":"org","identity":"acct-1","sha":"abc","confirmed_at":"2026-11-01T10:00:00Z","hipaa_seen":false}`,
		filepath.Join(".claude", backupsDir, backupName): `{}`,
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(userHome, rel), []byte(body), privateFilePerm); err != nil {
			return err
		}
	}
	return nil
}

// printer writes lines with the temp user home shown as "~", so the output
// is the same on every run.
type printer struct {
	w    io.Writer
	home string
}

// rel shows the home folder as ~ in s.
func (p printer) rel(s string) string { return strings.ReplaceAll(s, p.home, homeTilde) }

// line prints one line (callers format it with fmt.Sprintf) with the home
// folder shown as ~.
// A write error on the output (a closed pipe) is dropped: the example has
// nothing better to do with it than stop printing.
func (p printer) line(s string) {
	_, _ = fmt.Fprintln(p.w, p.rel(s))
}

// section prints a blank line and a "== title" heading.
func (p printer) section(title string) { p.line("\n== " + title) }

// snippet:start agent
// showAgent creates the agent and prints its identity constants.
func showAgent(p printer, host agentio.Host) error {
	p.section("agent")
	a := claude.New(host) // Pure: no I/O, never fails
	p.line(fmt.Sprintf("id=%s name=%q binary=%s home env=%s", a.ID(), a.Name(), claude.Binary, claude.ConfigDirEnv))
	p.line(fmt.Sprintf("spec binary=%s home markers=%v", a.Spec().Binary, a.Spec().HomeMarkers))
	return nil
}

// snippet:end agent

// snippet:start homes
// showHomes lists the homes on the machine and picks one by dir.
func showHomes(p printer, host agentio.Host) error {
	a := claude.New(host)
	homes, err := a.Homes() // ~/.claude first, then every ~/.claude-<x> with a marker
	if err != nil {
		return err
	}
	for _, h := range homes {
		p.line(fmt.Sprintf("home %s name=%s default=%t", h.Dir(), h.Name(), h.IsDefault()))
	}
	work := a.HomeAt(filepath.Join(host.UserHome, ".claude-work"))
	isHome, err := a.IsHome(work.Dir())
	if err != nil {
		return err
	}
	markers, err := a.HomeMarkersIn(work.Dir())
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("HomeAt %s is a home=%t markers=%v (of %v)", work, isHome, markers, claude.HomeMarkers()))
	if _, ok := a.HomeFromEnv(); !ok {
		p.line(fmt.Sprintf("HomeFromEnv: %s is absent, so the default home is in use", claude.ConfigDirEnv))
	}
	if _, err := a.HomeOf(agentio.NewHome("codex", host.UserHome, true, "x")); errors.Is(err, agentio.ErrForeignHome) {
		p.line(fmt.Sprintf("HomeOf refuses a Codex home: %v", err))
	}
	return nil
}

// snippet:end homes

// snippet:start project
// showProject opens the fixture project: its dir and git root.
func showProject(p printer, host agentio.Host) error {
	a := claude.New(host)
	proj, err := a.Project(filepath.Join(host.UserHome, fixtureProject))
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("project dir=%s repo root=%q", proj.Dir, proj.RepoRoot))
	return nil
}

// snippet:end project

// snippet:start neutral
// showNeutral gets the agentio.Agent view and a neutral home handle.
func showNeutral(p printer, host agentio.Host) error {
	a := claude.New(host)
	n := a.Neutral() // the agentio.Agent view for code that loops over agents
	home := a.DefaultHome().Neutral()
	p.line(fmt.Sprintf("neutral id=%s default home=%s agent of home=%s", n.ID(), n.DefaultHome(), home.Agent()))
	return nil
}

// snippet:end neutral

// snippet:start install
// showInstall reads the install from disk, the last update and builds the
// install and update commands without running anything.
func showInstall(p printer, host agentio.Host) error {
	p.section("install")
	a := claude.New(host)
	inst, err := a.Install() // ErrNotInstalled when claude is not on PATH
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("path=%s real=%s method=%s version=%s", inst.Path, inst.RealPath, inst.Method, inst.Version))
	p.line(fmt.Sprintf("raw method=%q auto updates=%v protected for native=%t", inst.RawMethod, deref(inst.AutoUpdates), inst.ProtectedForNative))

	last, err := a.LastUpdate(a.DefaultHome()) // nil when the file is absent
	if err != nil {
		return err
	}
	if last != nil {
		p.line(fmt.Sprintf("last update %s: %s -> %s outcome=%s", last.At.Format(time.RFC3339), last.From, last.To, last.Outcome))
	}
	changelog, err := a.Changelog(a.DefaultHome()) // "" when cache/changelog.md is absent
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("cached changelog: %d bytes", len(changelog)))

	v, err := claude.ParseVersion("2.1.325 (Claude Code)")
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("ParseVersion -> %s", v))
	p.line(fmt.Sprintf("update command: %s", a.UpdateCommand()))
	p.line(fmt.Sprintf("install command: %s", a.InstallCommand("stable")))
	return nil
}

// snippet:end install

// snippet:start paths
// showPaths computes home, project and system paths and the history folder
// a directory maps to. Every call here is Pure.
func showPaths(p printer, host agentio.Host) error {
	p.section("paths")
	a := claude.New(host)
	h := a.DefaultHome()
	paths := a.Paths(h)
	p.line(fmt.Sprintf("global config=%s settings=%s", paths.GlobalConfig, paths.Settings))
	p.line(fmt.Sprintf("projects=%s sessions=%s", paths.Projects, paths.Sessions))
	p.line(fmt.Sprintf("plugins installed=%s", paths.Plugins.Installed))

	proj, err := a.Project(filepath.Join(host.UserHome, fixtureProject))
	if err != nil {
		return err
	}
	pp := a.ProjectPaths(h, proj)
	p.line(fmt.Sprintf("project settings=%s local=%s mcp=%s", pp.Settings, pp.LocalSettings, pp.MCP))
	p.line(fmt.Sprintf("project memory files=%v", pp.Memory))

	// Managed (admin) paths come from Host.GOOS and Host.SystemRoot. This
	// fixture host sets neither, so it has no managed locations and the
	// machine's /etc never changes the output; HostFromOS fills both.
	p.line(fmt.Sprintf("managed settings (fixture host)=%q", a.SystemPaths().ManagedSettings))
	real := host
	real.GOOS, real.SystemRoot = linuxGOOS, unixSystemRoot
	p.line(fmt.Sprintf("managed settings (linux host)=%s", claude.New(real).SystemPaths().ManagedSettings))

	p.line(fmt.Sprintf("EncodePath(%q) = %s", historyFolder, claude.EncodePath(historyFolder)))
	p.line(fmt.Sprintf("HistoryDir = %s", a.HistoryDir(h, historyFolder)))
	return nil
}

// snippet:end paths

// snippet:start layers
// showSettingsLayers reads the user, project and local layers one by one and
// parses a settings body held in memory.
func showSettingsLayers(p printer, host agentio.Host) error {
	p.section("settings layers")
	a := claude.New(host)
	proj, err := a.Project(filepath.Join(host.UserHome, fixtureProject))
	if err != nil {
		return err
	}
	user, err := a.UserSettings(a.DefaultHome())
	if err != nil {
		return err
	}
	project, err := a.ProjectSettings(proj)
	if err != nil {
		return err
	}
	local, err := a.LocalSettings(proj)
	if err != nil {
		return err
	}
	for _, l := range []claude.SettingsLayer{user, project, local} {
		p.line(fmt.Sprintf("%s present=%t file=%s model=%q unknown keys=%v", l.Scope, l.Present, l.Source.Path, l.Settings.Model, l.Settings.Unknown.Keys()))
	}

	s, err := claude.ParseSettings("inline.json", []byte(`{"model": 7, "theme": "dark"}`))
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("ParseSettings: theme=%q model kept in Unknown=%v warnings=%d", s.Theme, s.Unknown.Keys(), len(s.Warnings)))
	return nil
}

// snippet:end layers

// snippet:start effective
// showEffectiveSettings merges every layer the way Claude Code does and
// says which layer won each key.
func showEffectiveSettings(p printer, host agentio.Host) error {
	p.section("effective settings")
	a := claude.New(host)
	proj, err := a.Project(filepath.Join(host.UserHome, fixtureProject))
	if err != nil {
		return err
	}
	eff, err := a.Settings(a.DefaultHome(), &proj, nil) // nil: no --settings value
	if err != nil {
		return err
	}
	for _, l := range eff.Layers {
		p.line(fmt.Sprintf("layer %s present=%t", l.Scope, l.Present))
	}
	m := eff.Merged
	p.line(fmt.Sprintf("model=%q from %s", m.Model, eff.Origin["model"]))
	p.line(fmt.Sprintf("default mode=%s allow=%v", m.Permissions.DefaultMode, m.Permissions.Allow))
	p.line(fmt.Sprintf("Stop hook groups=%d (user + project)", len(m.Hooks[claude.HookStop])))
	p.line(fmt.Sprintf("env=%v unknown keys=%v", m.Env, m.Unknown.Keys()))
	return nil
}

// snippet:end effective

// snippet:start managed
// showManagedSettings reads the admin layers and the server-managed cache.
func showManagedSettings(p printer, host agentio.Host) error {
	p.section("managed and remote settings")
	a := claude.New(host)
	h := a.DefaultHome()
	managed, err := a.ManagedSettings(h) // only present layers come back
	if err != nil {
		return err
	}
	for _, l := range managed {
		p.line(fmt.Sprintf("managed layer source=%s file=%s", l.Source.Kind, l.Source.Path))
	}
	remote, synced, err := a.RemoteSettings(h)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("remote present=%t synced=%s", remote.Present, synced.UTC().Format(time.RFC3339)))
	return nil
}

// snippet:end managed

// snippet:start env
// showEnv lists the variables Claude Code would see, with the layer that
// set each; values print redacted.
func showEnv(p printer, host agentio.Host) error {
	p.section("environment in effect")
	a := claude.New(host)
	proj, err := a.Project(filepath.Join(host.UserHome, fixtureProject))
	if err != nil {
		return err
	}
	env, err := a.EnvInEffect(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	for _, e := range env {
		p.line(fmt.Sprintf("%s=%s from=%s secret=%t", e.Name, e.Value, e.From, e.Secret))
	}
	var secrets []string
	for _, v := range claude.KnownEnv() {
		if v.Secret {
			secrets = append(secrets, v.Name)
		}
	}
	p.line(fmt.Sprintf("known variables=%d, secret ones=%v", len(claude.KnownEnv()), secrets))
	return nil
}

// snippet:end env

// snippet:start global
// showGlobalConfig reads .claude.json: install facts, the logged-in account
// (personal values print redacted) and the size of what stays unmodelled.
func showGlobalConfig(p printer, host agentio.Host) error {
	p.section("global config")
	a := claude.New(host)
	h := a.DefaultHome()
	g, err := a.GlobalConfig(h) // a missing file is the zero value with Path set
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("path=%s install method=%q startups=%d", g.Path, g.InstallMethod, g.NumStartups))
	p.line(fmt.Sprintf("projects=%d user MCP servers=%d unmodelled keys=%d", len(g.Projects), len(g.MCPServers), len(g.Unknown)))

	acct, err := a.Account(h) // nil when nobody is logged in
	if err != nil {
		return err
	}
	if acct != nil {
		p.line(fmt.Sprintf("account email=%s org type=%s billing=%s", acct.EmailAddress, acct.OrganizationType, acct.BillingType))
	}

	body, err := os.ReadFile(g.Path)
	if err != nil {
		return err
	}
	parsed, err := claude.ParseGlobalConfig(body) // the same decode, from bytes
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("ParseGlobalConfig: first start version=%s", parsed.FirstStartVersion))
	return nil
}

// snippet:end global

// snippet:start projectstate
// showProjectState reads one folder's entry in .claude.json and the cached
// usage body Claude Code stored there.
func showProjectState(p printer, host agentio.Host) error {
	p.section("project state")
	a := claude.New(host)
	h := a.DefaultHome()
	st, err := a.ProjectState(h, "/work/proj2") // nil when Claude Code never ran there
	if err != nil {
		return err
	}
	if st != nil {
		p.line(fmt.Sprintf("dir=%s trusted=%t last session cost=$%.2f models=%d", st.Dir, st.TrustAccepted, st.Last.Cost, len(st.Last.ModelUsage)))
	}
	if never, err := a.ProjectState(h, "/never/used"); err == nil && never == nil {
		p.line("/never/used: no entry")
	}
	cached, err := a.CachedUsageBody(h) // raw body; parse it with claude.ParseUsage
	if err != nil {
		return err
	}
	if cached != nil {
		p.line(fmt.Sprintf("cached usage fetched %s, %d bytes", cached.FetchedAt.UTC().Format(time.RFC3339), len(cached.Body)))
	}
	return nil
}

// snippet:end projectstate

// snippet:start backups
// showConfigBackups lists the .claude.json backups Claude Code keeps.
func showConfigBackups(p printer, host agentio.Host) error {
	p.section("config backups")
	a := claude.New(host)
	backups, err := a.ConfigBackups(a.DefaultHome()) // newest name last
	if err != nil {
		return err
	}
	for _, b := range backups {
		p.line(fmt.Sprintf("backup %s", b))
	}
	return nil
}

// snippet:end backups

// snippet:start permissions
// showPermissions lists every rule of every layer and the winning mode.
func showPermissions(p printer, host agentio.Host) error {
	p.section("permissions")
	a := claude.New(host)
	proj, err := a.Project(filepath.Join(host.UserHome, fixtureProject))
	if err != nil {
		return err
	}
	perms, err := a.Permissions(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("default mode=%s set by %s", perms.DefaultMode, perms.ModeSource))
	for _, r := range perms.Rules {
		p.line(fmt.Sprintf("%s %s tool=%s specifier=%q", r.Scope, r.Kind, r.Rule.Tool, r.Rule.Specifier))
	}
	return nil
}

// snippet:end permissions

// snippet:start rules
// showRules parses rule strings the way they appear in settings files.
func showRules(p printer, _ agentio.Host) error {
	p.section("parse rules")
	for _, s := range []string{"Bash(git *)", "Read", "mcp__github__get_issue", "Bash(git *"} {
		r, err := claude.ParseRule(s)
		if err != nil {
			p.line(fmt.Sprintf("%q: %v", s, err))
			continue
		}
		p.line(fmt.Sprintf("%q -> tool=%s specifier=%q", s, r.Tool, r.Specifier))
	}
	return nil
}

// snippet:end rules

// snippet:start policy
// showPolicy reads the organisation's policy limits and their stamp.
func showPolicy(p printer, host agentio.Host) error {
	p.section("policy limits")
	a := claude.New(host)
	pl, err := a.PolicyLimits(a.DefaultHome()) // nil when the file is absent
	if err != nil {
		return err
	}
	if pl == nil {
		p.line("no policy limits")
		return nil
	}
	names := make([]string, 0, len(pl.Restrictions))
	for name := range pl.Restrictions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p.line(fmt.Sprintf("restriction %s allowed=%t", name, pl.Restrictions[name].Allowed))
	}
	p.line(fmt.Sprintf("defaults=%d", len(pl.Defaults)))
	if pl.Stamp != nil {
		p.line(fmt.Sprintf("stamp kind=%s identity=%s confirmed=%s", pl.Stamp.Kind, pl.Stamp.Identity, pl.Stamp.ConfirmedAt.Format(time.RFC3339)))
	}
	return nil
}

// snippet:end policy

// deref prints a presence-tracked bool: "unset" when the key is absent.
func deref(b *bool) string {
	if b == nil {
		return "unset"
	}
	return fmt.Sprint(*b)
}
