package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/ubgo/agentio"
)

// Hook handler keys, verbatim from config/src/hook_config.rs at
// rust-v0.162.1 (HookHandlerConfig serde renames).
const (
	hookKeyType              = "type"
	hookKeyCommand           = "command"
	hookKeyCommandWindows    = "commandWindows"
	hookKeyCommandWindowsAlt = "command_windows"
	hookKeyTimeout           = "timeout"
	hookKeyAsync             = "async"
	hookKeyStatusMessage     = "statusMessage"
	hookKeyContextLimit      = "additionalContextLimit"
	hookKeyServer            = "server"
	hookKeyTool              = "tool"
	hookKeyInput             = "input"
	hookKeyMatcher           = "matcher"
	hookKeyHooks             = "hooks"
	hookKeyDescription       = "description"
)

// HookConfig returns the [hooks] table of <home>/config.toml (events and
// the hooks.state table); a zero Hooks when the file or table is absent.
// Disk · Read.
func (a *Agent) HookConfig(h Home) (Hooks, error) {
	c, err := a.ConfigFile(h)
	if err != nil || c.Hooks == nil {
		return Hooks{}, err
	}
	return *c.Hooks, nil
}

// HookState returns the hooks.state table of <home>/config.toml, keyed
// "<hooks file>:<event_snake>:<group>:<handler>". Disk · Read.
func (a *Agent) HookState(h Home) (map[string]HookState, error) {
	hk, err := a.HookConfig(h)
	return hk.State, err
}

// HooksFile is <home>/hooks.json (hook_config.rs HooksFile): an optional
// description and the same event → matcher groups as [hooks].
type HooksFile struct {
	Path        string
	Description Opt[string]
	Events      map[HookEvent][]HookMatcherGroup
	Unknown     agentio.Unknown // top-level keys besides description/hooks
}

type hooksFileWire struct {
	Description *string                              `json:"description"`
	Hooks       map[HookEvent][]hookMatcherGroupWire `json:"hooks"`
}

type hookMatcherGroupWire struct {
	Matcher *string           `json:"matcher"`
	Hooks   []json.RawMessage `json:"hooks"`
}

type hookHandlerWire struct {
	Type                   HookHandlerType `json:"type"`
	Command                *string         `json:"command"`
	CommandWindows         *string         `json:"commandWindows"`
	CommandWindowsAlt      *string         `json:"command_windows"`
	Timeout                *float64        `json:"timeout"`
	Async                  *bool           `json:"async"`
	StatusMessage          *string         `json:"statusMessage"`
	AdditionalContextLimit *int64          `json:"additionalContextLimit"`
	Server                 *string         `json:"server"`
	Tool                   *string         `json:"tool"`
	Input                  json.RawMessage `json:"input"`
}

func hookHandlerKnownKeys() []string {
	return []string{hookKeyType, hookKeyCommand, hookKeyCommandWindows, hookKeyCommandWindowsAlt, hookKeyTimeout,
		hookKeyAsync, hookKeyStatusMessage, hookKeyContextLimit, hookKeyServer, hookKeyTool, hookKeyInput}
}

// ParseHooksFile decodes hooks.json bytes. A handler that does not decode
// keeps its type (if readable) and raw keys in UnknownJSON plus a warning;
// the rest of the file still reads. Pure.
func ParseHooksFile(b []byte, path string) (HooksFile, agentio.Warnings, error) {
	var w hooksFileWire
	if err := json.Unmarshal(b, &w); err != nil {
		return HooksFile{}, nil, fmt.Errorf("codex: decode %s: %w", fileHooks, err)
	}
	hf := HooksFile{Path: path}
	if w.Description != nil {
		hf.Description = agentio.Some(*w.Description)
	}
	hf.Unknown, _ = agentio.ExtractUnknown(b, []string{hookKeyDescription, hookKeyHooks})
	var warns agentio.Warnings
	for ev, groups := range w.Hooks {
		if hf.Events == nil {
			hf.Events = map[HookEvent][]HookMatcherGroup{}
		}
		for _, g := range groups {
			mg := HookMatcherGroup{}
			if g.Matcher != nil {
				mg.Matcher = agentio.Some(*g.Matcher)
			}
			for _, raw := range g.Hooks {
				hh, ok := decodeHookHandlerJSON(raw)
				if !ok {
					warns.Add(agentio.WarnTypeChanged, path, string(ev), "a hook handler did not decode; kept raw")
				}
				mg.Hooks = append(mg.Hooks, hh)
			}
			hf.Events[ev] = append(hf.Events[ev], mg)
		}
	}
	return hf, warns, nil
}

func decodeHookHandlerJSON(raw json.RawMessage) (HookHandler, bool) {
	var hh HookHandler
	hh.UnknownJSON, _ = agentio.ExtractUnknown(raw, hookHandlerKnownKeys())
	var w hookHandlerWire
	if err := json.Unmarshal(raw, &w); err != nil {
		all, _ := agentio.ExtractUnknown(raw, nil)
		hh.UnknownJSON = all
		return hh, false
	}
	hh.Type = w.Type
	optStr := func(dst *Opt[string], v *string) {
		if v != nil {
			*dst = agentio.Some(*v)
		}
	}
	optStr(&hh.Command, w.Command)
	optStr(&hh.CommandWindows, w.CommandWindowsAlt)
	optStr(&hh.CommandWindows, w.CommandWindows)
	optStr(&hh.StatusMessage, w.StatusMessage)
	optStr(&hh.Server, w.Server)
	optStr(&hh.Tool, w.Tool)
	if w.Timeout != nil {
		hh.Timeout = agentio.Some(time.Duration(*w.Timeout * float64(time.Second)))
	}
	if w.Async != nil {
		hh.Async = agentio.Some(*w.Async)
	}
	if w.AdditionalContextLimit != nil {
		hh.AdditionalContextLimit = agentio.Some(*w.AdditionalContextLimit)
	}
	if len(w.Input) > 0 {
		hh.InputJSON = agentio.Raw(w.Input)
	}
	return hh, true
}

// HooksFile reads <home>/hooks.json; found=false when absent. Disk · Read.
func (a *Agent) HooksFile(h Home) (HooksFile, bool, error) {
	path := a.Paths(h).Hooks
	b, err := readFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return HooksFile{}, false, nil
	}
	if err != nil {
		return HooksFile{}, false, fmt.Errorf("codex: read %s: %w", fileHooks, err)
	}
	hf, _, err := ParseHooksFile(b, path)
	if err != nil {
		return HooksFile{}, false, err
	}
	return hf, true, nil
}

// ProfileKind says which profile system a profile belongs to.
type ProfileKind string

// ProfileKind values.
const (
	// ProfileLegacy is a [profiles.<name>] table (or the root `profile =`
	// selector) in config.toml.
	ProfileLegacy ProfileKind = "legacy"
	// ProfileV2 is a <home>/<name>.config.toml file (Codex >= 0.131.0).
	ProfileV2 ProfileKind = "v2"
)

// IsKnown reports a known kind. Pure.
func (k ProfileKind) IsKnown() bool { return k == ProfileLegacy || k == ProfileV2 }

// ProfileInfo is one profile of a home.
type ProfileInfo struct {
	Name string
	Kind ProfileKind
	Path string // config.toml for legacy, <name>.config.toml for v2
	// Selected: the root `profile =` names it (legacy selector).
	Selected bool
	// HasTable: a [profiles.<name>] table exists (legacy only; a selector
	// may name a profile without a table).
	HasTable bool
}

// Profiles lists legacy profiles (sorted) then v2 profile files (sorted).
// A name in both lists is the conflict Codex refuses (`codex -p NAME`,
// loader/mod.rs:303). Disk · Read.
func (a *Agent) Profiles(h Home) ([]ProfileInfo, error) {
	c, err := a.ConfigFile(h)
	if err != nil {
		return nil, err
	}
	cfgPath := a.Paths(h).Config
	var out []ProfileInfo
	for _, name := range c.LegacyProfileNames() {
		_, table := c.Profiles[name]
		out = append(out, ProfileInfo{Name: name, Kind: ProfileLegacy, Path: cfgPath,
			Selected: c.Profile.Set && c.Profile.V == name, HasTable: table})
	}
	v2, err := a.ProfileFiles(h)
	if err != nil {
		return nil, err
	}
	for _, name := range v2 {
		out = append(out, ProfileInfo{Name: name, Kind: ProfileV2, Path: a.Paths(h).ProfileConfig(name)})
	}
	return out, nil
}
