package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
)

// timeLayout is how times print in the text view.
const timeLayout = "2006-01-02 15:04"

// printText writes the human-readable view of the report.
func printText(w io.Writer, r Report) {
	fmt.Fprintf(w, "agentinfo  %s  dir=%s\n", r.Generated.Format(time.RFC3339), r.Dir)
	if r.ProcessError != "" {
		fmt.Fprintf(w, "process table: %s\n", r.ProcessError)
	}
	for _, a := range r.Agents {
		printAgent(w, a)
	}
}

func printAgent(w io.Writer, a AgentReport) {
	fmt.Fprintf(w, "\n=== %s (%s) ===\n", a.Name, a.ID)
	if a.Install == nil {
		fmt.Fprintf(w, "install:   not installed %s\n", paren(a.InstallError))
	} else {
		in := a.Install
		fmt.Fprintf(w, "install:   %s -> %s  method=%s  version=%s\n", in.Path, in.RealPath, orDash(string(in.Method)), orDash(in.Version.String()))
		if len(in.Shadowed) > 0 {
			fmt.Fprintf(w, "shadowed:  %s\n", strings.Join(in.Shadowed, ", "))
		}
	}
	if c := a.Compat; c != nil {
		fmt.Fprintf(w, "compat:    %s (tested %s..%s, installed %s)\n", c.Status, c.TestedMin, c.TestedMax, orDash(c.Installed))
	}
	if a.BinaryVersion != "" || a.BinaryVersionError != "" {
		fmt.Fprintf(w, "binary --version: %s %s\n", a.BinaryVersion, paren(a.BinaryVersionError))
	}
	caps := make([]string, 0, len(a.Supported))
	for _, c := range a.Supported {
		caps = append(caps, c.String())
	}
	fmt.Fprintf(w, "supported: %s\n", strings.Join(caps, " "))
	if a.HomesError != "" {
		fmt.Fprintf(w, "homes: error %s\n", a.HomesError)
	}
	if len(a.Homes) == 0 {
		fmt.Fprintln(w, "homes:     none found")
	}
	for _, h := range a.Homes {
		printHome(w, h)
	}
	fmt.Fprintf(w, "\nprocesses (%d):\n", len(a.Processes))
	tw := tabwriter.NewWriter(w, 2, 2, 2, ' ', 0)
	for _, p := range a.Processes {
		fmt.Fprintf(tw, "  pid %d\tppid %d\t%s\tsession=%v\t%s\t%s\n", p.PID, p.PPID, p.Kind, p.Session, fmtTime(p.Start), p.Exe)
	}
	tw.Flush()
}

func printHome(w io.Writer, h HomeReport) {
	def := ""
	if h.Default {
		def = " (default)"
	}
	fmt.Fprintf(w, "\n--- home %s%s  name=%s\n", h.Dir, def, h.Name)
	if len(h.Paths) > 0 {
		fmt.Fprintln(w, "paths:")
		tw := tabwriter.NewWriter(w, 2, 2, 2, ' ', 0)
		for _, p := range h.Paths {
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", p.Name, p.Path, existsMark(p.Exists))
		}
		tw.Flush()
	}
	if s := h.Settings; s != nil {
		fmt.Fprintf(w, "settings:  %d files, %d modelled %s keys, %d kept raw, %d unknown keys %s\n", len(s.Files), s.Keys, s.KeyUnit, s.RawKeys, len(s.UnknownKeys), bracket(s.UnknownKeys))
		for _, f := range s.Files {
			fmt.Fprintf(w, "  file %s\n", f)
		}
		keys := make([]string, 0, len(s.Notes))
		for k := range s.Notes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, "  %s = %s\n", k, s.Notes[k])
		}
	}
	for _, p := range h.Profiles {
		sel := ""
		if p.Selected {
			sel = " (selected)"
		}
		fmt.Fprintf(w, "profile:   %-20s %-6s %s%s\n", p.Name, p.Kind, p.Path, sel)
	}
	for _, p := range h.ModelProviders {
		fmt.Fprintf(w, "provider:  %-20s name=%s base_url=%s wire_api=%s\n", p.ID, orDash(p.Name), orDash(p.BaseURL), orDash(p.WireAPI))
	}
	for _, hk := range h.Hooks {
		fmt.Fprintf(w, "hooks:     %-18s %d groups, %d handlers (%s)\n", hk.Event, hk.Groups, hk.Handlers, hk.Source)
	}
	for _, l := range h.ConfigLayers {
		fmt.Fprintf(w, "layer:     %-8s %s %s\n", l.Scope, l.Path, existsMark(l.Exists))
	}
	fmt.Fprintf(w, "trust:     %s\n", h.ProjectTrust)
	if d := h.Defaults; d != nil {
		fmt.Fprintf(w, "defaults:  model=%s autonomy=%s\n", orDash(d.Model), d.Autonomy)
	}
	fmt.Fprintf(w, "mcp (%d):\n", len(h.MCPServers))
	for _, m := range h.MCPServers {
		target := m.URL
		if target == "" {
			target = strings.TrimSpace(m.Command + " " + strings.Join(m.Args, " "))
		}
		fmt.Fprintf(w, "  %-20s %-6s %-8s %s env=%v  (%s)\n", m.Name, m.Transport, m.Scope, target, m.EnvNames, m.Source)
	}
	fmt.Fprintf(w, "instructions (%d):\n", len(h.InstructionFiles))
	for _, f := range h.InstructionFiles {
		fmt.Fprintf(w, "  %-9s %-10s %6dB %s\n", f.Scope, f.Kind, f.Bytes, f.Path)
	}
	fmt.Fprintf(w, "skills (%d):", len(h.Skills))
	for _, s := range h.Skills {
		name := s.Name
		if s.Plugin != "" {
			name = s.Plugin + ":" + s.Name
		}
		fmt.Fprintf(w, " %s[%s]", name, s.Scope)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "plugins (%d):\n", len(h.Plugins))
	for _, p := range h.Plugins {
		fmt.Fprintf(w, "  %s@%s %s enabled=%v\n", p.Name, p.Marketplace, orDash(p.Version), p.Enabled)
	}
	if id := h.Identity; id != nil {
		exp := "-"
		if id.ExpiresAt != nil {
			exp = fmtTime(*id.ExpiresAt)
		}
		fmt.Fprintf(w, "identity:  logged-in=%v email=%s org=%s plan=%s billing=%s credential=%s expires=%s expired=%v\n",
			id.LoggedIn, id.Email, id.Org, orDash(id.PlanLabel), id.Billing, id.Credential, exp, h.Expired)
	}
	if u := h.CachedUsage; u != nil {
		fmt.Fprintf(w, "cached usage (%s, as of %s, %s old):\n", u.Source, fmtTime(u.ObservedAt), fmtAge(h.CachedUsageAge))
		for _, win := range u.Windows {
			fmt.Fprintf(w, "  %-14s %-6s %5.1f%%  len=%s resets=%s\n", win.VendorKind, win.Class(), win.Percent, durationOrDash(win.Length), fmtTimePtr(win.ResetsAt))
		}
	} else {
		fmt.Fprintln(w, "cached usage: none on disk")
	}
	if u := h.LiveUsage; u != nil {
		fmt.Fprintf(w, "live usage (%s): %d windows\n", fmtTime(u.ObservedAt), len(u.Windows))
	}
	fmt.Fprintf(w, "conversations (latest %d):\n", len(h.Conversations))
	tw := tabwriter.NewWriter(w, 2, 2, 2, ' ', 0)
	for _, c := range h.Conversations {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", c.ID, fmtTime(c.Updated), clip(c.Title, 50), c.Dir)
	}
	tw.Flush()
	fmt.Fprintf(w, "live sessions (%d):\n", len(h.LiveSessions))
	for _, s := range h.LiveSessions {
		fmt.Fprintf(w, "  pid %d %s %s alive=%v status=%s since=%s %s\n", s.PID, s.ConversationID, s.Kind, s.Alive, s.Status, fmtTime(s.StatusSince), s.Dir)
	}
	if len(h.Checks) > 0 {
		fmt.Fprintln(w, "doctor:")
		for _, c := range h.Checks {
			fmt.Fprintf(w, "  %-5s %-22s %s\n", c.Status, c.ID, c.Message)
		}
	}
	for _, e := range h.Errors {
		fmt.Fprintf(w, "error %s: %s\n", e.Section, e.Error)
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func paren(s string) string {
	if s == "" {
		return ""
	}
	return "(" + s + ")"
}

func bracket(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return "[" + strings.Join(list, " ") + "]"
}

func existsMark(ok bool) string {
	if ok {
		return "ok"
	}
	return "missing"
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format(timeLayout)
}

func fmtTimePtr(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return fmtTime(*t)
}

// clip shortens a title for one table row.
func clip(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// fmtAge prints an age in the largest whole unit that fits (days, hours, minutes).
func fmtAge(d time.Duration) string {
	const day = 24 * time.Hour
	switch {
	case d <= 0:
		return "-"
	case d >= day:
		return fmt.Sprintf("%dd%dh", d/day, (d%day)/time.Hour)
	case d >= time.Hour:
		return fmt.Sprintf("%dh%dm", d/time.Hour, (d%time.Hour)/time.Minute)
	default:
		return fmt.Sprintf("%dm", d/time.Minute)
	}
}
