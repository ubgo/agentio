// Command agentinfo prints everything agentcli can read about every built-in
// coding agent on this machine: install, compat, homes, paths, settings, MCP
// servers, instruction files, skills, plugins, identity (personal values
// redacted), cached usage, recent conversations, live sessions and running
// agent processes.
//
// By default it only reads disk and the process table. It never runs a vendor
// binary unless --runner is given and never calls a vendor endpoint unless
// --network is given.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/khanakia/agentcli"
	"github.com/khanakia/agentcli/builtin"
)

// recentConversations is how many conversations per home are listed.
const recentConversations = 5

// options are the command-line switches.
type options struct {
	json    bool // print the full typed report as JSON
	runner  bool // allow Runner calls (runs the vendor binary: may touch login)
	network bool // allow Network reads (vendor usage endpoint: may 429)
	dir     string
}

func main() {
	var o options
	flag.BoolVar(&o.json, "json", false, "print the full typed report as JSON")
	flag.BoolVar(&o.runner, "runner", false, "also run the vendor binary for its version (may touch the login)")
	flag.BoolVar(&o.network, "network", false, "also fetch live usage from the vendor (one request per home; may 429)")
	flag.StringVar(&o.dir, "dir", "", "project folder for per-project reads (default: current directory)")
	flag.Parse()
	if err := run(context.Background(), o, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "agentinfo:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, o options, w io.Writer) error {
	host, err := agentcli.HostFromOS()
	if err != nil {
		return err
	}
	if o.dir == "" {
		if o.dir, err = os.Getwd(); err != nil {
			return err
		}
	}
	agents, err := builtin.New(host)
	if err != nil {
		return err
	}
	rep := Report{Generated: host.Now().UTC(), Dir: o.dir}
	procs, perr := host.Processes.Processes(ctx)
	rep.ProcessError = errText(perr)
	for _, a := range agents.All() {
		rep.Agents = append(rep.Agents, inspect(ctx, host, a, agents, procs, o))
	}
	if o.json {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(rep)
	}
	printText(w, rep)
	return nil
}

// inspect gathers one agent's report. Each section records its own error so
// one unreadable file never hides the rest.
func inspect(ctx context.Context, host agentcli.Host, a agentcli.Agent, all builtin.Agents, procs []agentcli.Process, o options) AgentReport {
	r := AgentReport{ID: a.ID(), Name: a.Name(), Supported: agentcli.Supported(a)}
	if p, err := a.LookPath(); err == nil {
		r.Binary = p
	}
	if in, ok := agentcli.InstallerOf(a); ok {
		inst, err := in.Install()
		r.Install, r.InstallError = &inst, errText(err)
		if err != nil {
			r.Install = nil
		}
	}
	vendorReport(&r, all, o)
	if o.runner {
		if bv, ok := agentcli.BinaryVersionOf(a); ok {
			v, err := bv.BinaryVersion(ctx)
			r.BinaryVersion, r.BinaryVersionError = v.String(), errText(err)
		}
	}
	homes, err := a.Homes()
	r.HomesError = errText(err)
	for _, h := range homes {
		r.Homes = append(r.Homes, inspectHome(ctx, host, a, all, h, o))
	}
	if pc, ok := agentcli.ProcessClassifierOf(a); ok {
		for _, p := range procs {
			if info, ok := pc.ClassifyProcess(p); ok {
				r.Processes = append(r.Processes, ProcessRow{PID: p.PID, PPID: p.PPID, Start: p.Start, Exe: p.Exe, Kind: info.Kind, Session: info.Session})
			}
		}
	}
	return r
}

func inspectHome(ctx context.Context, host agentcli.Host, a agentcli.Agent, all builtin.Agents, h agentcli.Home, o options) HomeReport {
	hr := HomeReport{Dir: h.Dir(), Name: h.Name(), Default: h.IsDefault()}
	vendorHomeReport(&hr, all, a.ID(), h, o.dir)
	if c, ok := agentcli.ConfigOf(a); ok {
		layers, err := c.ConfigLayers(h, o.dir)
		hr.ConfigLayers, hr.Errors = layers, addErr(hr.Errors, "config-layers", err)
		trust, err := c.ProjectTrust(h, o.dir)
		hr.ProjectTrust, hr.Errors = trust, addErr(hr.Errors, "project-trust", err)
		def, err := c.Defaults(h, o.dir)
		hr.Defaults, hr.Errors = &def, addErr(hr.Errors, "defaults", err)
	}
	if m, ok := agentcli.MCPServersOf(a); ok {
		v, err := m.MCPServers(h, o.dir)
		hr.MCPServers, hr.Errors = v, addErr(hr.Errors, "mcp", err)
	}
	if in, ok := agentcli.InstructionFilesOf(a); ok {
		v, err := in.InstructionFiles(h, o.dir)
		hr.InstructionFiles, hr.Errors = v, addErr(hr.Errors, "instructions", err)
	}
	if s, ok := agentcli.SkillsOf(a); ok {
		v, err := s.Skills(h, o.dir)
		hr.Skills, hr.Errors = v, addErr(hr.Errors, "skills", err)
	}
	if p, ok := agentcli.PluginsOf(a); ok {
		v, err := p.Plugins(h, o.dir)
		hr.Plugins, hr.Errors = v, addErr(hr.Errors, "plugins", err)
	}
	if id, ok := agentcli.IdentityOf(a); ok {
		v, err := id.Identity(h)
		if err == nil {
			hr.Identity = &v
			hr.Expired = v.Expired(host.Now())
		}
		hr.Errors = addErr(hr.Errors, "identity", err)
	}
	if cu, ok := agentcli.CachedUsageOf(a); ok {
		u, found, err := cu.CachedUsage(h)
		if found && err == nil {
			hr.CachedUsage = &u
			// The vendor refreshes this copy on its own schedule (OD-10), so
			// the reading's age is part of the answer.
			if !u.ObservedAt.IsZero() {
				hr.CachedUsageAge = host.Now().Sub(u.ObservedAt).Round(time.Minute)
			}
		}
		hr.Errors = addErr(hr.Errors, "cached-usage", err)
	}
	if o.network {
		if ur, ok := agentcli.UsageOf(a); ok {
			u, err := ur.Usage(ctx, h)
			if err == nil {
				hr.LiveUsage = &u
			}
			hr.Errors = addErr(hr.Errors, "usage", err)
		}
	}
	if cl, ok := agentcli.ConversationsOf(a); ok {
		v, err := cl.Conversations(ctx, h, agentcli.ConversationQuery{Limit: recentConversations})
		hr.Conversations, hr.Errors = v, addErr(hr.Errors, "conversations", err)
	}
	if ls, ok := agentcli.LiveSessionsOf(a); ok {
		v, err := ls.LiveSessions(h)
		hr.LiveSessions, hr.Errors = v, addErr(hr.Errors, "live-sessions", err)
	}
	if d, ok := agentcli.DoctorOf(a); ok {
		hr.Checks = d.Checks(ctx, h)
	}
	return hr
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func addErr(list []SectionError, section string, err error) []SectionError {
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
