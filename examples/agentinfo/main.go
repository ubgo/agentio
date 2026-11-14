// Command agentinfo prints everything agentio can read about every built-in
// coding agent on this machine: install, compat, homes, paths, settings, MCP
// servers, instruction files, skills, plugins, identity (personal values
// redacted), cached usage, recent conversations, live sessions and running
// agent processes.
//
// By default it only reads disk and the process table. It never runs a vendor
// binary unless --runner is given and never calls a vendor endpoint unless
// --network is given. -fixtures reads the fixture homes checked into this
// repository instead of this machine (same output on every OS; the platform
// workflow and the distro images run it).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/builtin"
	"github.com/ubgo/agentio/codex"
)

// recentConversations is how many conversations per home are listed.
const recentConversations = 5

// options are the command-line switches.
type options struct {
	json    bool // print the full typed report as JSON
	runner  bool // allow Runner calls (runs the vendor binary: may touch login)
	network bool // allow Network reads (vendor usage endpoint: may 429)
	dir     string
	// fixtures reads the repository fixture homes in place (read-only)
	// instead of this machine.
	fixtures bool
}

func main() {
	var o options
	flag.BoolVar(&o.json, "json", false, "print the full typed report as JSON")
	flag.BoolVar(&o.runner, "runner", false, "also run the vendor binary for its version (may touch the login)")
	flag.BoolVar(&o.network, "network", false, "also fetch live usage from the vendor (one request per home; may 429)")
	flag.StringVar(&o.dir, "dir", "", "project folder for per-project reads (default: current directory)")
	flag.BoolVar(&o.fixtures, "fixtures", false, "read the fixture homes in this repository instead of this machine (run from examples/)")
	flag.Parse()
	if err := runOS(context.Background(), o, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "agentinfo:", err)
		os.Exit(1)
	}
}

// runOS is the command-line default: it reads this machine through
// agentio.HostFromOS (the one place that touches the real machine) and
// uses the current directory when --dir is not given.
func runOS(ctx context.Context, o options, w io.Writer) error {
	if o.fixtures {
		host, dir, err := repoFixtureHost(repoFromExamples)
		if err != nil {
			return err
		}
		if o.dir == "" {
			o.dir = dir
		}
		return run(ctx, host, o, w)
	}
	host, err := agentio.HostFromOS()
	if err != nil {
		return err
	}
	if o.dir == "" {
		if o.dir, err = os.Getwd(); err != nil {
			return err
		}
	}
	return run(ctx, host, o, w)
}

// run prints the report for the agents on host. Every input comes through
// host, so a test runs it on fixture homes with fake process, keychain and
// HTTP inputs.
func run(ctx context.Context, host agentio.Host, o options, w io.Writer) error {
	agents, err := builtin.New(host)
	if err != nil {
		return err
	}
	rep := Report{Generated: host.Now().UTC(), Dir: o.dir}
	var procs []agentio.Process
	if host.Processes != nil {
		var perr error
		procs, perr = host.Processes.Processes(ctx)
		rep.ProcessError = errText(perr)
	}
	for _, a := range agents.All() {
		rep.Agents = append(rep.Agents, inspect(ctx, host, a, agents, procs, o))
	}
	if o.json {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	return printText(w, rep)
}

// inspect gathers one agent's report. Each section records its own error so
// one unreadable file never hides the rest.
func inspect(ctx context.Context, host agentio.Host, a agentio.Agent, all builtin.Agents, procs []agentio.Process, o options) AgentReport {
	r := AgentReport{ID: a.ID(), Name: a.Name(), Supported: agentio.Supported(a)}
	if p, err := a.LookPath(); err == nil {
		r.Binary = p
	}
	if in, ok := agentio.InstallerOf(a); ok {
		inst, err := in.Install()
		r.Install, r.InstallError = &inst, errText(err)
		if err != nil {
			r.Install = nil
		}
	}
	vendorReport(&r, all, o)
	if o.runner {
		if bv, ok := agentio.BinaryVersionOf(a); ok {
			v, err := bv.BinaryVersion(ctx)
			r.BinaryVersion, r.BinaryVersionError = v.String(), errText(err)
		}
	}
	homes, err := a.Homes()
	r.HomesError = errText(err)
	for _, h := range homes {
		r.Homes = append(r.Homes, inspectHome(ctx, host, a, all, h, o))
	}
	if pc, ok := agentio.ProcessClassifierOf(a); ok {
		for _, p := range procs {
			if info, ok := pc.ClassifyProcess(p); ok {
				r.Processes = append(r.Processes, ProcessRow{PID: p.PID, PPID: p.PPID, Start: p.Start, Exe: p.Exe, Kind: info.Kind, Session: info.Session})
			}
		}
	}
	return r
}

// inspectHome gathers one home's report through the shared accessors, then
// the vendor-only extras. Like inspect, every read records its own error.
func inspectHome(ctx context.Context, host agentio.Host, a agentio.Agent, all builtin.Agents, h agentio.Home, o options) HomeReport {
	hr := HomeReport{Dir: h.Dir(), Name: h.Name(), Default: h.IsDefault()}
	vendorHomeReport(&hr, all, a.ID(), h, o.dir)
	if c, ok := agentio.ConfigOf(a); ok {
		layers, err := c.ConfigLayers(h, o.dir)
		hr.ConfigLayers, hr.Errors = layers, addErr(hr.Errors, SectionConfigLayers, err)
		trust, err := c.ProjectTrust(h, o.dir)
		hr.ProjectTrust, hr.Errors = trust, addErr(hr.Errors, SectionProjectTrust, err)
		def, err := c.Defaults(h, o.dir)
		hr.Defaults, hr.Errors = &def, addErr(hr.Errors, SectionDefaults, err)
	}
	if m, ok := agentio.MCPServersOf(a); ok {
		v, err := m.MCPServers(h, o.dir)
		hr.MCPServers, hr.Errors = v, addErr(hr.Errors, SectionMCP, err)
	}
	if in, ok := agentio.InstructionFilesOf(a); ok {
		v, err := in.InstructionFiles(h, o.dir)
		hr.InstructionFiles, hr.Errors = v, addErr(hr.Errors, SectionInstructions, err)
	}
	if s, ok := agentio.SkillsOf(a); ok {
		v, err := s.Skills(h, o.dir)
		hr.Skills, hr.Errors = v, addErr(hr.Errors, SectionSkills, err)
	}
	if p, ok := agentio.PluginsOf(a); ok {
		v, err := p.Plugins(h, o.dir)
		hr.Plugins, hr.Errors = v, addErr(hr.Errors, SectionPlugins, err)
	}
	if id, ok := agentio.IdentityOf(a); ok {
		v, err := id.Identity(h)
		if err == nil {
			hr.Identity = &v
			hr.Expired = v.Expired(host.Now())
		}
		hr.Errors = addErr(hr.Errors, SectionIdentity, err)
	}
	if cu, ok := agentio.CachedUsageOf(a); ok {
		u, found, err := cu.CachedUsage(h)
		if found && err == nil {
			hr.CachedUsage = &u
			// The vendor refreshes this copy on its own schedule, so
			// the reading's age is part of the answer.
			if !u.ObservedAt.IsZero() {
				hr.CachedUsageAge = host.Now().Sub(u.ObservedAt).Round(time.Minute)
			}
		}
		hr.Errors = addErr(hr.Errors, SectionCachedUsage, err)
	}
	if o.network {
		if ur, ok := agentio.UsageOf(a); ok {
			u, err := ur.Usage(ctx, h)
			if err == nil {
				hr.LiveUsage = &u
			}
			hr.Errors = addErr(hr.Errors, SectionUsage, err)
		}
	}
	if cl, ok := agentio.ConversationsOf(a); ok {
		v, err := cl.Conversations(ctx, h, agentio.ConversationQuery{Limit: recentConversations})
		hr.Conversations, hr.Errors = v, addErr(hr.Errors, SectionConversations, err)
	}
	if ls, ok := agentio.LiveSessionsOf(a); ok {
		v, err := ls.LiveSessions(h)
		hr.LiveSessions, hr.Errors = v, addErr(hr.Errors, SectionLiveSessions, err)
	}
	if d, ok := agentio.DoctorOf(a); ok {
		hr.Checks = d.Checks(ctx, h)
	}
	return hr
}

// errText flattens an error for the report; nil is "" so the field is
// omitted from -json output.
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// addErr appends err under section; a nil err leaves list unchanged, so
// every read can be recorded with one call.
func addErr(list []SectionError, section Section, err error) []SectionError {
	if err == nil {
		return list
	}
	return append(list, SectionError{Section: section, Error: err.Error()})
}

// durationOrDash formats a window length; 0 means the vendor did not say.
func durationOrDash(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	return d.String()
}

// Repository fixtures as seen from `cd examples`: the Claude fixture is a
// whole user home, the Codex fixture one home selected with CODEX_HOME.
const (
	repoFromExamples = ".."
	repoClaudeHome   = "claude/testdata/homes/2.1.325/userhome"
	repoCodexHome    = "codex/testdata/homes/0.162.1"
	repoProjectDir   = "project"
	repoNoBinaries   = "PATH=/nonexistent-bin"
	repoFixtureUser  = "user"
)

// repoFixtureClock is the fixed time -fixtures reads at.
func repoFixtureClock() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

// repoFixtureHost is a Host over the fixture homes under repoRoot, read in
// place: an empty process table, no keychain, no HTTP, no runner, a fixed
// clock and no GOOS (so no system locations), which makes the report the
// same on macOS, Linux and Windows. It also returns the fixture project dir.
func repoFixtureHost(repoRoot string) (agentio.Host, string, error) {
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return agentio.Host{}, "", err
	}
	home := filepath.Join(root, filepath.FromSlash(repoClaudeHome))
	return agentio.Host{
		UserHome:  home,
		User:      repoFixtureUser,
		Env:       []string{"HOME=" + home, repoNoBinaries, codex.EnvHome + "=" + filepath.Join(root, filepath.FromSlash(repoCodexHome))},
		Now:       repoFixtureClock,
		Processes: noProcesses{},
	}, filepath.Join(home, repoProjectDir), nil
}

// noProcesses is an empty process table.
type noProcesses struct{}

// Processes returns no rows. Pure.
func (noProcesses) Processes(context.Context) ([]agentio.Process, error) { return nil, nil }

// Cwd knows no process. Pure.
func (noProcesses) Cwd(context.Context, int) (string, error) { return "", agentio.ErrNotFound }
