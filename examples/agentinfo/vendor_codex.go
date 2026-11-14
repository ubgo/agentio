package main

import (
	"reflect"
	"sort"
	"strconv"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/codex"
)

// codexCompat compares the on-disk Codex version with codex.Compat(). Disk · Read.
func codexCompat(_ *codex.Agent, inst *agentio.Install) *CompatReport {
	c := codex.Compat()
	r := &CompatReport{TestedMin: codex.TestedMin, TestedMax: codex.TestedMax}
	if inst != nil {
		r.Installed = inst.Version.String()
		r.Status = c.Check(inst.Version)
	}
	return r
}

// codexHome fills the Codex L3 parts of a home report: named paths and a
// value-free config.toml summary. Disk · Read.
func codexHome(hr *HomeReport, x *codex.Agent, n agentio.Home) {
	h, err := x.HomeOf(n)
	if err != nil {
		hr.Errors = addErr(hr.Errors, "home", err)
		return
	}
	hr.Paths = pathRows(reflect.ValueOf(x.Paths(h)))
	cfg, warns, err := x.ConfigFileWarnings(h)
	if err != nil {
		hr.Errors = addErr(hr.Errors, "config.toml", err)
		return
	}
	s := &SettingsSummary{Notes: map[string]string{}}
	p := x.Paths(h)
	if fileExists(p.Config) {
		s.Files = append(s.Files, p.Config)
	}
	for k := range cfg.Unknown {
		s.UnknownKeys = append(s.UnknownKeys, k)
	}
	sort.Strings(s.UnknownKeys)
	if v, ok := cfg.Model.Get(); ok {
		s.Notes["model"] = v
	}
	if v, ok := cfg.ApprovalPolicy.Get(); ok {
		s.Notes["approval_policy"] = string(v)
	}
	if v, ok := cfg.SandboxMode.Get(); ok {
		s.Notes["sandbox_mode"] = string(v)
	}
	if v, ok := cfg.Profile.Get(); ok {
		s.Notes["profile"] = v
	}
	kc := cfg.KeyCount()
	s.Keys, s.RawKeys, s.KeyUnit = kc.Typed, kc.Raw, "leaf"
	if len(cfg.Unknown) != kc.Unknown {
		s.Notes["unknown leaf keys"] = strconv.Itoa(kc.Unknown)
	}
	codexProfiles(hr, x, h, cfg)
	codexHooks(hr, x, h, p.Config, cfg)
	for _, w := range warns {
		s.Notes["warning "+w.Key] = w.Kind.String()
	}
	hr.Settings = s
}

// codexProfiles lists legacy and v2 profiles and the model providers.
// Disk · Read.
func codexProfiles(hr *HomeReport, x *codex.Agent, h codex.Home, cfg codex.Config) {
	profiles, err := x.Profiles(h)
	hr.Errors = addErr(hr.Errors, "profiles", err)
	for _, p := range profiles {
		hr.Profiles = append(hr.Profiles, ProfileRow{Name: p.Name, Kind: string(p.Kind), Path: p.Path, Selected: p.Selected})
	}
	ids := make([]string, 0, len(cfg.ModelProviders))
	for id := range cfg.ModelProviders {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		mp := cfg.ModelProviders[id]
		hr.ModelProviders = append(hr.ModelProviders, ProviderRow{ID: id, Name: mp.Name.V, BaseURL: mp.BaseURL.V, WireAPI: string(mp.WireAPI.V)})
	}
}

// codexHooks counts hook handlers per event from config.toml and hooks.json.
// Disk · Read.
func codexHooks(hr *HomeReport, x *codex.Agent, h codex.Home, configPath string, cfg codex.Config) {
	add := func(src string, events map[codex.HookEvent][]codex.HookMatcherGroup) {
		names := make([]string, 0, len(events))
		for ev := range events {
			names = append(names, string(ev))
		}
		sort.Strings(names)
		for _, ev := range names {
			groups := events[codex.HookEvent(ev)]
			n := 0
			for _, g := range groups {
				n += len(g.Hooks)
			}
			hr.Hooks = append(hr.Hooks, HookRow{Source: src, Event: ev, Groups: len(groups), Handlers: n})
		}
	}
	if cfg.Hooks != nil {
		add(configPath, cfg.Hooks.Events)
	}
	hf, found, err := x.HooksFile(h)
	hr.Errors = addErr(hr.Errors, "hooks.json", err)
	if found {
		add(hf.Path, hf.Events)
	}
}
