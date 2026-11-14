package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Runner is the Process door: reads that run the claude binary through
// Host.Exec, with the home env rule applied and no TTY. Every method is
// labelled "may touch login": vendor subcommands can refresh or rewrite
// the login, so they are never assumed side-effect free. Nothing in the library calls a Runner method implicitly.
type Runner struct{ a *Agent }

// Runner returns the Process door. cost: Pure.
func (a *Agent) Runner() *Runner { return &Runner{a: a} }

// plutilBin converts the macOS managed preferences plist to JSON; the
// library runs plutil, never claude, to read it, so no login is touched.
const (
	plutilBin        = "/usr/bin/plutil"
	plutilConvert    = "-convert"
	plutilJSONFormat = "json"
	plutilOutput     = "-o"
	plutilStdout     = "-"
	// managedPlistRel is relative to Host.SystemRoot (darwin only).
	managedPlistRel = "Library/Managed Preferences/com.anthropic.claudecode.plist"
)

// run executes claude with args for h and returns stdout; a non-zero exit
// is an error carrying a stderr snippet.
func (r *Runner) run(ctx context.Context, h Home, args ...string) ([]byte, error) {
	if r.a.host.Exec == nil {
		return nil, agentio.UnsupportedError(ID, "run claude", "the host has no process runner")
	}
	cmd := r.a.command(h, args...)
	res, err := r.a.host.Exec.Run(ctx, cmd, nil)
	if err != nil {
		return nil, fmt.Errorf("claude: run %s: %w", cmd.String(), err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("claude: %s exited %d: %s", cmd.String(), res.ExitCode, snippet(bytes.TrimSpace(res.Stderr)))
	}
	return res.Stdout, nil
}

// BinaryVersion runs `claude --version` for the default home.
// cost: Process · Read; login-safe only after the fixture test (SPEC §4).
func (r *Runner) BinaryVersion(ctx context.Context) (agentio.Version, error) {
	out, err := r.run(ctx, r.a.DefaultHome(), flagVersion)
	if err != nil {
		return agentio.Version{}, err
	}
	v, ok := ParseVersion(string(out))
	if !ok {
		return agentio.Version{}, fmt.Errorf("claude: no version in %s output %q", flagVersion, snippet(bytes.TrimSpace(out)))
	}
	return v, nil
}

// AuthMethod is auth status authMethod ("claude.ai" seen).
type AuthMethod string

// APIProvider is auth status apiProvider ("firstParty" seen).
type APIProvider string

// AuthStatus is `claude auth status --json`.
type AuthStatus struct {
	LoggedIn            bool             `json:"loggedIn"`
	AuthMethod          AuthMethod       `json:"authMethod"`
	APIProvider         APIProvider      `json:"apiProvider"`
	AnalyticsDisabled   bool             `json:"analyticsDisabled"`
	ProjectsDirectory   string           `json:"projectsDirectory"`
	ConfigDirectory     string           `json:"configDirectory"`
	LoginExpiresAt      *time.Time       `json:"loginExpiresAt"`
	Email               agentio.Personal `json:"email"`
	OrgID               agentio.Personal `json:"orgId"`
	OrgName             agentio.Personal `json:"orgName"`
	SubscriptionType    string           `json:"subscriptionType"`
	PlanTier            string           `json:"planTier"`
	StoredClaudeAILogin bool             `json:"storedClaudeAiLogin"`
	Unknown             agentio.Unknown  `json:"-"`
}

// ParseAuthStatus decodes auth status JSON. cost: Pure.
func ParseAuthStatus(b []byte) (AuthStatus, error) {
	var s AuthStatus
	u, _, err := decodeLenient("auth status", b, reflect.TypeFor[AuthStatus](), func(d []byte) error { s = AuthStatus{}; return json.Unmarshal(d, &s) })
	s.Unknown = u
	return s, err
}

// AuthStatus runs `claude auth status --json` for h. cost: Process · Read ·
// MAY TOUCH LOGIN (creates ~/.claude.json if missing per docs; refresh
// unverified, so it is treated as able to rewrite the login).
func (r *Runner) AuthStatus(ctx context.Context, h Home) (AuthStatus, error) {
	out, err := r.run(ctx, h, subAuth, subStatus, flagJSON)
	if err != nil {
		return AuthStatus{}, err
	}
	return ParseAuthStatus(out)
}

type wirePluginListItem struct {
	ID             PluginID    `json:"id"`
	Version        string      `json:"version"`
	Scope          PluginScope `json:"scope"`
	Enabled        bool        `json:"enabled"`
	InstallPath    string      `json:"installPath"`
	InstalledAt    time.Time   `json:"installedAt"`
	LastUpdated    time.Time   `json:"lastUpdated"`
	ProjectEnabled bool        `json:"projectEnabled"`
}

// RunnerPlugin is one row of `claude plugin list --json`.
type RunnerPlugin struct {
	ID                       PluginID
	Version                  string
	Scope                    PluginScope
	Enabled, ProjectEnabled  bool
	InstallPath              string
	InstalledAt, LastUpdated time.Time
}

// Plugins runs `claude plugin list --json [--available]`. cost: Process ·
// Read · may touch login.
func (r *Runner) Plugins(ctx context.Context, h Home, available bool) ([]RunnerPlugin, error) {
	args := []string{subPlugin, subList, flagJSON}
	if available {
		args = append(args, flagAvailable)
	}
	out, err := r.run(ctx, h, args...)
	if err != nil {
		return nil, err
	}
	var w []wirePluginListItem
	if err := json.Unmarshal(out, &w); err != nil {
		return nil, fmt.Errorf("claude: decode plugin list: %w", err)
	}
	res := make([]RunnerPlugin, len(w))
	for i, p := range w {
		res[i] = RunnerPlugin{ID: p.ID, Version: p.Version, Scope: p.Scope, Enabled: p.Enabled, ProjectEnabled: p.ProjectEnabled,
			InstallPath: p.InstallPath, InstalledAt: p.InstalledAt, LastUpdated: p.LastUpdated}
	}
	return res, nil
}

// RunnerMarketplace is one row of `claude plugin marketplace list --json`.
type RunnerMarketplace struct {
	Name            string                `json:"name"`
	Source          MarketplaceSourceKind `json:"source"`
	Repo            string                `json:"repo"`
	InstallLocation string                `json:"installLocation"`
}

// Marketplaces runs `claude plugin marketplace list --json`. cost: Process ·
// Read · may touch login.
func (r *Runner) Marketplaces(ctx context.Context, h Home) ([]RunnerMarketplace, error) {
	out, err := r.run(ctx, h, subPlugin, subMarketplace, subList, flagJSON)
	if err != nil {
		return nil, err
	}
	var w []RunnerMarketplace
	if err := json.Unmarshal(out, &w); err != nil {
		return nil, fmt.Errorf("claude: decode marketplace list: %w", err)
	}
	return w, nil
}

// BackgroundAgent is one row of `claude agents --json`.
type BackgroundAgent struct {
	ID        string          `json:"id"`
	PID       int             `json:"pid"`
	SessionID string          `json:"sessionId"`
	Name      string          `json:"name"`
	Cwd       string          `json:"cwd"`
	Kind      string          `json:"kind"`
	State     string          `json:"state"`
	Status    string          `json:"status"`
	StartedAt millis          `json:"startedAt"`
	Unknown   agentio.Unknown `json:"-"`
}

// BackgroundAgents runs `claude agents --json [--all]`. cost: Process ·
// Read · may touch login.
func (r *Runner) BackgroundAgents(ctx context.Context, h Home, all bool) ([]BackgroundAgent, error) {
	args := []string{subAgents, flagJSON}
	if all {
		args = append(args, flagAll)
	}
	out, err := r.run(ctx, h, args...)
	if err != nil {
		return nil, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("claude: decode agents: %w", err)
	}
	// Each entry decodes leniently so keys a newer Claude Code adds land in
	// Unknown instead of being dropped (invariant 13).
	w := make([]BackgroundAgent, 0, len(items))
	for _, it := range items {
		var ba BackgroundAgent
		unknown, _, err := decodeLenient(agentsSource, it, reflect.TypeFor[BackgroundAgent](), func(d []byte) error { ba = BackgroundAgent{}; return json.Unmarshal(d, &ba) })
		if err != nil {
			return nil, err
		}
		ba.Unknown = unknown
		w = append(w, ba)
	}
	return w, nil
}

// agentsSource names `claude agents --json` output in decode errors.
const agentsSource = "claude agents --json"

// AutoModeConfig runs `claude auto-mode config`; Raw until a sample is
// recorded. cost: Process · Read · may touch login.
func (r *Runner) AutoModeConfig(ctx context.Context, h Home) (agentio.Raw, error) {
	out, err := r.run(ctx, h, subAutoMode, subConfig)
	return agentio.Raw(out), err
}

// AutoModeDefaults runs `claude auto-mode defaults`; Raw until sampled.
// cost: Process · Read · may touch login.
func (r *Runner) AutoModeDefaults(ctx context.Context, h Home) (agentio.Raw, error) {
	out, err := r.run(ctx, h, subAutoMode, subDefaults)
	return agentio.Raw(out), err
}

// ManagedPlist converts the macOS managed preferences plist to JSON with
// plutil and returns it as a managed layer; nil when the plist is absent
// or the Host is not darwin with a SystemRoot. Does not run claude, so no
// login risk. cost: Process · Read.
func (r *Runner) ManagedPlist(ctx context.Context) (*SettingsLayer, error) {
	if r.a.host.GOOS != goosDarwin {
		return nil, nil
	}
	managedPlistPath, ok := r.a.host.SystemPath(managedPlistRel)
	if !ok || !fileExists(managedPlistPath) {
		return nil, nil
	}
	if r.a.host.Exec == nil {
		return nil, agentio.UnsupportedError(ID, "managed plist", "the host has no process runner")
	}
	cmd := r.a.host.NewCommand(plutilBin, []string{plutilConvert, plutilJSONFormat, plutilOutput, plutilStdout, managedPlistPath}, "")
	res, err := r.a.host.Exec.Run(ctx, cmd, nil)
	if err != nil {
		return nil, fmt.Errorf("claude: plutil: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("claude: plutil exited %d", res.ExitCode)
	}
	s, err := ParseSettings(managedPlistPath, res.Stdout)
	l := &SettingsLayer{Scope: ScopeManaged, Source: Source{Kind: SourceMDMPlist, Path: managedPlistPath}, Present: true, Settings: s, Err: err}
	return l, err
}

// Windows managed-settings registry (managed settings docs, "Where each
// mechanism stores the policy"), read with reg.exe from System32.
const (
	regExeRel       = "Windows/System32/reg.exe"
	regQuery        = "query"
	regValueFlag    = "/v"
	regValueName    = "Settings"
	regKeyHKLM      = `HKLM\SOFTWARE\Policies\ClaudeCode`
	regKeyHKCU      = `HKCU\SOFTWARE\Policies\ClaudeCode`
	regTypePrefix   = "REG_"
	regTypeSZ       = "REG_SZ"
	regTypeExpandSZ = "REG_EXPAND_SZ"
)

// ManagedRegistry reads the Windows policy registry values that carry
// managed settings JSON, HKLM first, then HKCU, and returns each present
// one as a managed layer (Source.Kind SourceRegistryHKLM or
// SourceRegistryHKCU, Source.Path the key). Claude Code applies HKCU only
// when no admin document (remote settings, MDM or HKLM, managed files) is
// present; the caller decides with the layers in hand. nil on a Host that
// is not windows with a SystemRoot (reg.exe lives in System32 under it).
// Runs reg.exe, never claude, so no login risk. cost: Process · Read.
func (r *Runner) ManagedRegistry(ctx context.Context) ([]SettingsLayer, error) {
	if r.a.host.GOOS != agentio.OSWindows {
		return nil, nil
	}
	regExe, ok := r.a.host.SystemPath(regExeRel)
	if !ok {
		return nil, nil
	}
	if r.a.host.Exec == nil {
		return nil, agentio.UnsupportedError(ID, "managed registry", "the host has no process runner")
	}
	var out []SettingsLayer
	for _, k := range []struct {
		key  string
		kind SourceKind
	}{{regKeyHKLM, SourceRegistryHKLM}, {regKeyHKCU, SourceRegistryHKCU}} {
		cmd := r.a.host.NewCommand(regExe, []string{regQuery, k.key, regValueFlag, regValueName}, "")
		res, err := r.a.host.Exec.Run(ctx, cmd, nil)
		if err != nil {
			return out, fmt.Errorf("claude: reg query %s: %w", k.key, err)
		}
		if res.ExitCode != 0 {
			continue // no such key or value: that source is absent
		}
		val, ok := parseRegValue(res.Stdout, regValueName)
		if !ok {
			continue
		}
		s, perr := ParseSettings(k.key, []byte(val))
		out = append(out, SettingsLayer{Scope: ScopeManaged, Source: Source{Kind: k.kind, Path: k.key}, Present: true, Settings: s, Err: perr})
	}
	return out, nil
}

// parseRegValue finds `<name>    REG_SZ    <data>` (or REG_EXPAND_SZ) in
// `reg query` output and returns the data with its inner spaces kept. Pure.
func parseRegValue(out []byte, name string) (string, bool) {
	for _, line := range strings.Split(string(agentio.StripBOM(out)), "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		rest, ok := strings.CutPrefix(line, name)
		if !ok || len(rest) == 0 || (rest[0] != ' ' && rest[0] != '\t') {
			continue
		}
		rest = strings.TrimSpace(rest)
		typ, data, _ := strings.Cut(rest, "    ")
		if typ != regTypeSZ && typ != regTypeExpandSZ {
			if strings.HasPrefix(typ, regTypePrefix) {
				return "", false
			}
			continue
		}
		return strings.TrimSpace(data), true
	}
	return "", false
}
