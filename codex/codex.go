// Package codex is the typed API of OpenAI Codex CLI on one machine:
// install, homes, config.toml layers and profiles, AGENTS.md, auth.json
// identity, usage, rollouts, prompt history, skills, plugins, execpolicy
// rules, models, processes, launch builders and the `exec --json`
// decoder (docsi/CODEX_API.md). SQLite stores live in the codex/state
// sub-module so callers of this package never pull a SQLite driver.
//
// Read-only by default: nothing here writes a Codex-owned file, refreshes a
// login or runs the codex binary, except through the Runner door
// (SPEC §4). Every Codex subcommand can refresh and rewrite auth.json, so
// every Runner method is labelled MayTouchLogin.
package codex

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// ID is Codex's agent ID.
const ID agentio.ID = "codex"

// Spec facts (agx provider/codex/codex.go, codex --help at 0.162.1).
const (
	// Binary is the Codex CLI executable name.
	Binary = "codex"
	// displayName is the agent's display name.
	displayName = "Codex"
	// EnvHome selects a non-default Codex home; ABSENT for the default.
	EnvHome = "CODEX_HOME"
	// EnvSQLiteHome moves the SQLite files out of the home (state/src/lib.rs:133).
	EnvSQLiteHome = "CODEX_SQLITE_HOME"
	// EnvAPIKey is the OpenAI key variable. Codex auth does not read it
	// (rust-v0.162.1 login/src/auth/manager.rs load_auth reads only
	// CODEX_API_KEY), but a model provider or MCP server may name it as its
	// key, so a launch keeps it; it still marks per-token billing.
	EnvAPIKey = "OPENAI_API_KEY"
	// EnvCodexAPIKey takes precedence over the home's stored login in
	// `codex exec` (load_auth with enable_codex_api_key_env, exec/src/lib.rs),
	// so every launch scrubs an inherited one (invariant 9).
	EnvCodexAPIKey = "CODEX_API_KEY"
	// defaultHomeDir is relative to Host.UserHome.
	defaultHomeDir = ".codex"
	// npmPackage is the npm launcher's package name (process matching).
	npmPackage = "@openai/codex"
)

// spec placeholders understood by agentio templates.
const (
	tplArgs = "{args}"
	tplID   = "{id}"
)

// Spec is the data Codex's L1 core is built from: binary, home variable,
// default home, markers (§2), resume template, process match. Pure.
func Spec() agentio.Spec {
	return agentio.Spec{
		ID:          ID,
		Name:        displayName,
		Binary:      Binary,
		HomeEnv:     EnvHome,
		DefaultHome: defaultHomeDir,
		HomeMarkers: []string{fileConfig, fileAuth, dirSessions, fileStateDB, fileVersionCache, fileInstallationID},
		NewArgs:     []string{tplArgs},
		ResumeArgs:  []string{subResume, tplArgs, tplID},
		VersionArgs: []string{flagVersion},
		LoginArgs:   []string{subLogin},
		LogoutArgs:  []string{subLogout},
		// Codex has no shared-file convention between homes; nothing to link.
		SharedHomeFiles: nil,
		// Only the variable that overrides a home login; see EnvAPIKey.
		CredentialEnv: []string{EnvCodexAPIKey},
		APIBillingEnv: []string{EnvAPIKey, EnvCodexAPIKey},
		Process: agentio.ProcessMatch{
			Basenames:    []string{Binary},
			ArgvContains: []string{npmPackage},
		},
	}
}

// Agent is the Codex implementation, built once per Host. It is not an
// agentio.Agent: Neutral returns the L1/L2 view (SPEC §5.3).
type Agent struct {
	host agentio.Host
	core *agentio.Core
	// baseURL is the ChatGPT backend origin for usage calls;
	// WithBaseURL points it at an httptest server or a proxy.
	baseURL string
}

// New builds the Codex agent. Nothing is read at construction. Pure.
func New(h agentio.Host) *Agent {
	core, err := agentio.NewCore(Spec(), h)
	if err != nil {
		// Spec() is a constant this package controls and a test validates;
		// a failure here is a programming error in this file.
		panic("codex: invalid built-in spec: " + err.Error())
	}
	return &Agent{host: h, core: core, baseURL: DefaultBaseURL}
}

// WithBaseURL returns a copy that sends network calls to base (an origin
// such as an httptest server URL, a trailing slash is dropped) instead of
// chatgpt.com/backend-api, the same mechanism as claude.Agent.WithBaseURL,
// so tests and proxies need no custom transport. cost: Pure.
func (a *Agent) WithBaseURL(base string) *Agent {
	c := *a
	c.baseURL = strings.TrimRight(base, "/")
	return &c
}

// ID returns codex.ID. Pure.
func (a *Agent) ID() agentio.ID { return ID }

// Name returns the display name. Pure.
func (a *Agent) Name() string { return displayName }

// Spec returns Spec(). Pure.
func (a *Agent) Spec() agentio.Spec { return Spec() }

// LookPath finds the codex binary on Host.Env PATH (never a shell). Disk · Read.
func (a *Agent) LookPath() (string, error) { return a.core.LookPath() }

// Home is a Codex home; L3 methods take it so they are compile-checked
// against Codex.
type Home struct{ h agentio.Home }

// Neutral returns the agentio.Home. Pure.
func (h Home) Neutral() agentio.Home { return h.h }

// Dir is the home directory. Pure.
func (h Home) Dir() string { return h.h.Dir() }

// IsDefault reports ~/.codex: CODEX_HOME must be ABSENT in the child env. Pure.
func (h Home) IsDefault() bool { return h.h.IsDefault() }

// Name is the suggested short name. Pure.
func (h Home) Name() string { return h.h.Name() }

// Target is a plain launch target for h (version unknown). Pure.
func (h Home) Target() agentio.Target { return h.h.Target() }

// MarshalJSON writes the neutral home. Pure.
func (h Home) MarshalJSON() ([]byte, error) { return h.h.MarshalJSON() }

// HomeOf converts a neutral home; ErrForeignHome for another agent's. Pure.
func (a *Agent) HomeOf(n agentio.Home) (Home, error) {
	if err := agentio.CheckOwner(ID, n); err != nil {
		return Home{}, err
	}
	return Home{h: n}, nil
}

// DefaultHome is ~/.codex. Pure.
func (a *Agent) DefaultHome() Home { return Home{h: a.core.DefaultHome()} }

// HomeFromEnv is the home CODEX_HOME selects in Host.Env. Pure.
func (a *Agent) HomeFromEnv() (Home, bool) {
	h, ok := a.core.HomeFromEnv()
	return Home{h: h}, ok
}

// Homes lists the default home and the CODEX_HOME one, only dirs with a
// marker. Codex has no ~/.codex-* convention; other homes come from the
// caller (HomeAt). Disk · Read.
func (a *Agent) Homes() ([]Home, error) {
	hs, err := a.core.Homes()
	if err != nil {
		return nil, err
	}
	out := make([]Home, 0, len(hs))
	for _, h := range hs {
		out = append(out, Home{h: h})
	}
	return out, nil
}

// HomeAt returns a caller-named home (agx config); ErrNotFound when dir
// holds no Codex marker. Disk · Read.
func (a *Agent) HomeAt(dir string) (Home, error) {
	h := a.core.HomeAt(dir)
	ok, err := a.core.IsHome(h.Dir())
	if err != nil {
		return Home{}, err
	}
	if !ok {
		return Home{}, fmt.Errorf("%w: no Codex home marker in %s", agentio.ErrNotFound, h.Dir())
	}
	return Home{h: h}, nil
}

// IsHome reports whether dir holds any Codex marker. Disk · Read.
func (a *Agent) IsHome(dir string) (bool, error) { return a.core.IsHome(dir) }

// HomeEnv returns base with the home rule applied: CODEX_HOME removed for
// the default home, set otherwise (invariant 2). Pure.
func (a *Agent) HomeEnv(base []string, h Home) []string {
	c, ok := agentio.HomeEnvChange(EnvHome, h.h)
	if !ok {
		return base
	}
	return agentio.ApplyEnv(base, c)
}

// InstallationID reads <home>/installation_id (a uuid). Disk · Read.
func (a *Agent) InstallationID(h Home) (agentio.Personal, error) {
	b, err := os.ReadFile(a.Paths(h).InstallationID)
	if err != nil {
		return agentio.Personal{}, fmt.Errorf("codex: read %s: %w", fileInstallationID, err)
	}
	return agentio.NewPersonal(strings.TrimSpace(string(b))), nil
}

// Writer is the Disk · Write door. Codex v0.1 writes no Codex-owned file
// (OD-7): config changes are vendor Commands (FeatureCommand,
// MCPAddCommand), so the door exists but has no methods yet.
type Writer struct{ a *Agent }

// Writer returns the write door. Pure.
func (a *Agent) Writer() *Writer { return &Writer{a: a} }

// now is Host.Now, the only clock (no time.Now outside HostFromOS).
// A Host without a clock reads as the zero time, which makes every expiry
// check "unknown" rather than guessing.
func (a *Agent) now() time.Time {
	if a.host.Now == nil {
		return time.Time{}
	}
	return a.host.Now()
}

// plainCommand builds `codex <args>` on h's default target, ignoring a
// foreign-home error (the caller passed a codex.Home, so it cannot be).
func (a *Agent) plainCommand(h Home, args []string) agentio.Command {
	cmd, err := a.core.Command(h.Target(), "", args, nil)
	if err != nil {
		return agentio.Command{Path: Binary, Args: args}
	}
	return cmd
}

// lookupEnv reads a variable from an env list.
func lookupEnv(env []string, key string) (string, bool) { return agentio.EnvLookup(env, key) }
