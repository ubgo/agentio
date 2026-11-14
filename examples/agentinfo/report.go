package main

import (
	"time"

	"github.com/ubgo/agentio"
)

// Report is the full typed output of agentinfo (--json prints it as is).
// Personal values (email, org) are agentio.Personal and marshal redacted;
// tokens never reach this type.
type Report struct {
	Generated    time.Time
	Dir          string
	ProcessError string `json:",omitempty"`
	Agents       []AgentReport
}

// AgentReport is everything known about one agent.
type AgentReport struct {
	ID                 agentio.ID
	Name               string
	Binary             string `json:",omitempty"`
	Supported          []agentio.Capability
	Install            *agentio.Install `json:",omitempty"`
	InstallError       string           `json:",omitempty"`
	Compat             *CompatReport    `json:",omitempty"`
	BinaryVersion      string           `json:",omitempty"`
	BinaryVersionError string           `json:",omitempty"`
	Homes              []HomeReport
	HomesError         string `json:",omitempty"`
	Processes          []ProcessRow
}

// CompatReport compares the on-disk installed version with the vendor
// module's tested range.
type CompatReport struct {
	TestedMin, TestedMax string
	Installed            string
	Status               agentio.CompatStatus
}

// HomeReport is everything known about one home of one agent.
type HomeReport struct {
	Dir              string
	Name             string
	Default          bool
	Paths            []PathRow `json:",omitempty"`
	Settings         *SettingsSummary
	Profiles         []ProfileRow  `json:",omitempty"`
	ModelProviders   []ProviderRow `json:",omitempty"`
	Hooks            []HookRow     `json:",omitempty"`
	ConfigLayers     []agentio.ConfigLayer
	ProjectTrust     agentio.Trust
	Defaults         *agentio.Defaults `json:",omitempty"`
	MCPServers       []agentio.MCPServer
	InstructionFiles []agentio.InstructionFile
	Skills           []agentio.Skill
	Plugins          []agentio.Plugin
	Identity         *agentio.Identity `json:",omitempty"`
	Expired          bool
	CachedUsage      *agentio.Usage `json:",omitempty"`
	CachedUsageAge   time.Duration  `json:",omitempty"` // now minus CachedUsage.ObservedAt
	LiveUsage        *agentio.Usage `json:",omitempty"`
	Conversations    []agentio.Conversation
	LiveSessions     []agentio.LiveSession
	Checks           []agentio.Check
	Errors           []SectionError `json:",omitempty"`
}

// PathRow is one named vendor path of a home and whether it exists.
type PathRow struct {
	Name   string
	Path   string
	Exists bool
}

// SettingsSummary is a short, value-free summary of a home's settings.
type SettingsSummary struct {
	Files       []string          // settings files that exist, in precedence order
	Keys        int               // modelled (typed) keys set across those files, counted per KeyUnit
	KeyUnit     string            // what Keys counts: "top-level" (Claude settings) or "leaf" (Codex TOML)
	RawKeys     int               // leaf keys kept as raw text in sub-trees not typed yet
	UnknownKeys []string          // keys the library does not model (names only)
	Notes       map[string]string // short non-personal facts (model, permission mode, …)
}

// SectionError is a per-section read failure.
type SectionError struct {
	Section string
	Error   string
}

// ProcessRow is one running process classified as this agent.
type ProcessRow struct {
	PID, PPID int
	Start     time.Time
	Exe       string
	Kind      agentio.ProcessKind
	Session   bool
}

// ProfileRow is one config profile (Codex: legacy [profiles.X] or a v2 file).
type ProfileRow struct {
	Name     string
	Kind     string
	Path     string
	Selected bool
}

// ProviderRow is one configured model provider.
type ProviderRow struct {
	ID      string
	Name    string
	BaseURL string
	WireAPI string
}

// HookRow counts the hook handlers configured for one event in one source file.
type HookRow struct {
	Source   string
	Event    string
	Groups   int
	Handlers int
}
