package main

import (
	"reflect"
	"sort"
	"strconv"

	"github.com/khanakia/agentcli"
	"github.com/khanakia/agentcli/claude"
)

// claudeCompat compares the on-disk Claude Code version with claude.Compat(). Disk · Read.
func claudeCompat(inst *agentcli.Install) *CompatReport {
	c := claude.Compat()
	r := &CompatReport{TestedMin: claude.TestedMin, TestedMax: claude.TestedMax}
	if inst != nil {
		r.Installed = inst.Version.String()
		r.Status = c.Check(inst.Version)
	}
	return r
}

// claudeHome fills the Claude L3 parts of a home report: named paths and a
// value-free settings summary (file list, key counts, unknown key names, the
// effective model and permission mode). Settings env values are never printed.
// Disk · Read.
func claudeHome(hr *HomeReport, c *claude.Agent, n agentcli.Home, dir string) {
	h, err := c.HomeOf(n)
	if err != nil {
		hr.Errors = addErr(hr.Errors, "home", err)
		return
	}
	hr.Paths = pathRows(reflect.ValueOf(c.Paths(h)))
	var proj *claude.Project
	if p, err := c.Project(dir); err == nil {
		proj = &p
	}
	eff, err := c.Settings(h, proj, nil)
	if err != nil {
		hr.Errors = addErr(hr.Errors, "settings", err)
		return
	}
	s := &SettingsSummary{Notes: map[string]string{}}
	unknown := map[string]bool{}
	for _, l := range eff.Layers {
		if !l.Present {
			continue
		}
		s.Files = append(s.Files, string(l.Scope)+" "+l.Source.Path)
		for _, k := range l.Settings.Unknown.Keys() {
			unknown[k] = true
		}
		if l.Err != nil {
			hr.Errors = addErr(hr.Errors, "settings "+l.Source.Path, l.Err)
		}
	}
	for k := range unknown {
		s.UnknownKeys = append(s.UnknownKeys, k)
	}
	sort.Strings(s.UnknownKeys)
	s.Keys, s.KeyUnit = len(eff.Origin), "top-level"
	m := eff.Merged
	if m.Model != "" {
		s.Notes["model"] = m.Model
	}
	if m.Permissions != nil {
		if m.Permissions.DefaultMode != "" {
			s.Notes["permissions.defaultMode"] = string(m.Permissions.DefaultMode)
		}
		s.Notes["permissions.allow/deny"] = strconv.Itoa(len(m.Permissions.Allow)) + "/" + strconv.Itoa(len(m.Permissions.Deny))
	}
	if len(m.Hooks) > 0 {
		s.Notes["hooks events"] = strconv.Itoa(len(m.Hooks))
	}
	if len(m.Env) > 0 {
		s.Notes["env vars (names only)"] = strconv.Itoa(len(m.Env))
	}
	if m.StatusLine != nil {
		s.Notes["statusLine"] = "set"
	}
	hr.Settings = s
}
