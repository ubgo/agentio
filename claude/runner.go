package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/ubgo/agentio"
)

// Runner is the Process door: reads that run the claude binary through
// Host.Exec, with the home env rule applied and no TTY. Every method is
// labelled "may touch login" (OD-5): vendor subcommands are never assumed
// side-effect free. Nothing in the library calls a Runner method implicitly.
type Runner struct{ a *Agent }

// Runner returns the Process door. cost: Pure.
func (a *Agent) Runner() *Runner { return &Runner{a: a} }

// plutilBin and the managed preferences domain (OD-9).
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
// unverified, OD-5).
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
	var w []BackgroundAgent
	if err := json.Unmarshal(out, &w); err != nil {
		return nil, fmt.Errorf("claude: decode agents: %w", err)
	}
	return w, nil
}

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
	cmd := agentio.NewCommand(plutilBin, []string{plutilConvert, plutilJSONFormat, plutilOutput, plutilStdout, managedPlistPath}, r.a.host.Env, "")
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
