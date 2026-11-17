package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
)

// ID is Claude Code's agent id.
const ID agentio.ID = "claude"

// Agent is Claude Code on one Host. It is not an agentio.Agent itself:
// Neutral() returns the L1/L2 view, so vendor methods keep their rich types
// (c.Usage returns claude.Usage, the neutral view's Usage returns
// agentio.Usage) without two methods fighting over one name (SPEC §5.3).
type Agent struct {
	host agentio.Host
	// core is the shared L1 implementation built from Spec(): homes, env
	// rule, PATH lookup, Command assembly.
	core *agentio.Core
	// baseURL is the API origin for usage, profile and probe calls;
	// WithBaseURL points it at an httptest server.
	baseURL string
}

// New returns the Claude agent for h. cost: Pure; no error, no I/O. Spec()
// is a constant that TestSpecValidates proves valid, so NewCore cannot fail
// here; a nil core would only follow an edit that breaks that test.
func New(h agentio.Host) *Agent {
	core, err := agentio.NewCore(Spec(), h)
	if err != nil {
		core = nil
	}
	return &Agent{host: h, core: core, baseURL: DefaultBaseURL}
}

// WithBaseURL returns a copy that sends network calls to base instead of
// api.anthropic.com. Tests use it with httptest. cost: Pure.
func (a *Agent) WithBaseURL(base string) *Agent {
	c := *a
	c.baseURL = strings.TrimRight(base, "/")
	return &c
}

// Host returns the host the agent reads through. cost: Pure.
func (a *Agent) Host() agentio.Host { return a.host }

// ID returns claude.ID. cost: Pure.
func (a *Agent) ID() agentio.ID { return ID }

// Name returns "Claude Code". cost: Pure.
func (a *Agent) Name() string { return displayName }

// homeMarkers are entries whose presence makes a ~/.claude-* directory a
// real home; they exclude look-alikes such as ~/.claude-worktrees.
func homeMarkers() []string {
	return []string{settingsFile, projectsDir, credentialsFile, globalConfigFile}
}

// HomeMarkers lists the marker entries of a home. cost: Pure.
func HomeMarkers() []string { return homeMarkers() }

// sharedHomeFiles are linked from ~/.claude into a new home: the user's own
// instructions, skills and settings. Logins, history and caches stay per home.
func sharedHomeFiles() []string { return []string{memoryFileName, skillsDir, settingsFile} }

// credentialEnv are the variables that take precedence over a home's stored
// login; every launch (home login or token login) scrubs inherited ones
// (invariant 9, agentio.Core.EnvChanges).
func credentialEnv() []string { return []string{OAuthTokenEnv, APIKeyEnv, AuthTokenEnv} }

// apiBillingEnv are the variables that switch Claude Code to per-token billing.
func apiBillingEnv() []string {
	return []string{APIKeyEnv, AuthTokenEnv, UseBedrockEnv, UseVertexEnv, UseFoundryEnv}
}

// NPMPackage is the npm package Claude Code installs from; it is in Spec
// so a caller can print an install hint ("npm i -g <package>") without
// knowing the vendor.
const NPMPackage = "@anthropic-ai/claude-code"

// Spec returns the data Claude's core is built from. cost: Pure.
func Spec() agentio.Spec {
	return agentio.Spec{
		ID: ID, Name: displayName, Binary: Binary,
		HomeEnv: ConfigDirEnv, DefaultHome: defaultDirName, ExtraHomes: extraHomesGlob,
		HomeMarkers:     homeMarkers(),
		NewArgs:         []string{placeholderArgs},
		ResumeArgs:      []string{flagResume, placeholderID, placeholderArgs},
		ModelArgs:       []string{flagModel, placeholderModel},
		AddDirArgs:      []string{flagAddDir, placeholderDir},
		PromptArgs:      []string{placeholderPrompt},
		VersionArgs:     []string{flagVersion},
		Packages:        []string{NPMPackage},
		LoginArgs:       []string{subAuth, subLogin},
		LogoutArgs:      []string{subAuth, subLogout},
		SharedHomeFiles: sharedHomeFiles(),
		CredentialEnv:   credentialEnv(),
		APIBillingEnv:   apiBillingEnv(),
		Process: agentio.ProcessMatch{
			Basenames: []string{Binary},
			Exclude:   []string{excludeDesktopApp, excludeChromeHost, excludeDesktopSupport},
		},
	}
}

// Spec returns claude.Spec(). cost: Pure.
func (a *Agent) Spec() agentio.Spec { return Spec() }

// Home is one Claude config dir. It wraps the neutral handle so every L3
// method is compile-checked against Claude.
type Home struct{ h agentio.Home }

// Neutral returns the shared handle. cost: Pure.
func (h Home) Neutral() agentio.Home { return h.h }

// Dir is the config dir. cost: Pure.
func (h Home) Dir() string { return h.h.Dir() }

// IsDefault reports the default home (~/.claude, CLAUDE_CONFIG_DIR absent). cost: Pure.
func (h Home) IsDefault() bool { return h.h.IsDefault() }

// Name is the suggested name ("personal", "work" for ~/.claude-work). cost: Pure.
func (h Home) Name() string { return h.h.Name() }

// Target is a plain launch target for this home (Version zero). cost: Pure.
func (h Home) Target() agentio.Target { return h.h.Target() }

// String is "claude:<dir>". cost: Pure.
func (h Home) String() string { return h.h.String() }

// MarshalJSON encodes the neutral handle. cost: Pure.
func (h Home) MarshalJSON() ([]byte, error) { return h.h.MarshalJSON() }

// wrap converts a neutral home of this agent.
func wrap(h agentio.Home) Home { return Home{h: h} }

// HomeAt returns the home at a caller-named dir (any CLAUDE_CONFIG_DIR);
// relative dirs resolve against UserHome. cost: Pure.
func (a *Agent) HomeAt(dir string) Home { return wrap(a.core.HomeAt(dir)) }

// HomeOf converts a neutral home; ErrForeignHome for another agent's. cost: Pure.
func (a *Agent) HomeOf(n agentio.Home) (Home, error) {
	if err := agentio.CheckOwner(ID, n); err != nil {
		return Home{}, err
	}
	return Home{h: n}, nil
}

// DefaultHome is ~/.claude. cost: Pure.
func (a *Agent) DefaultHome() Home { return wrap(a.core.DefaultHome()) }

// HomeFromEnv returns the home CLAUDE_CONFIG_DIR in Host.Env selects;
// false when the variable is absent or empty. cost: Pure.
func (a *Agent) HomeFromEnv() (Home, bool) {
	h, ok := a.core.HomeFromEnv()
	return wrap(h), ok
}

// HomeMarkersIn lists which markers exist in dir. cost: Disk · Read.
func (a *Agent) HomeMarkersIn(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claude: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, nil
	}
	var out []string
	for _, m := range homeMarkers() {
		if _, err := os.Lstat(filepath.Join(dir, m)); err == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

// IsHome reports whether dir holds any home marker. cost: Disk · Read.
func (a *Agent) IsHome(dir string) (bool, error) { return a.core.IsHome(dir) }

// Homes lists ~/.claude first (when it is a home), then every ~/.claude-<x>
// holding a marker sorted, then the CLAUDE_CONFIG_DIR home when not listed.
// ~/.claude-worktrees (git worktrees) has no marker and is skipped.
// cost: Disk · Read (glob + marker stat).
func (a *Agent) Homes() ([]Home, error) {
	hs, err := a.core.Homes()
	if err != nil {
		return nil, err
	}
	out := make([]Home, len(hs))
	for i, h := range hs {
		out[i] = wrap(h)
	}
	return out, nil
}

// LookPath finds claude by walking PATH from Host.Env, never a shell (on
// the owner's machine `claude` is also a zsh function). cost: Disk · Read.
func (a *Agent) LookPath() (string, error) { return a.core.LookPath() }

// Project is the folder Claude runs in.
type Project struct {
	// Dir is the session's primary working directory.
	Dir string
	// RepoRoot is the git root, "" outside git; settings.local.json lives
	// there (docs, v2.1.211+).
	RepoRoot string
}

// Project resolves dir and walks up for .git. cost: Disk · Read.
func (a *Agent) Project(dir string) (Project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Project{}, fmt.Errorf("claude: project %s: %w", dir, err)
	}
	p := Project{Dir: abs}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, gitDir)); err == nil {
			p.RepoRoot = d
			break
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	return p, nil
}

// localRoot is where .claude/settings.local.json lives (settings docs,
// "Where Claude Code keeps the local file in a git repository"): the repo
// root, except that it stays with .claude/settings.json in Dir outside a
// repository, on Windows, and when the repo root is the user's home dir.
// The docs' third exception (a root or .git/.claude entry not owned by the
// user) is not checked: Host has no file-owner reader.
func (a *Agent) localRoot(p Project) string {
	if p.RepoRoot == "" || a.host.GOOS == agentio.OSWindows || samePath(p.RepoRoot, a.host.UserHome) {
		return p.Dir
	}
	return p.RepoRoot
}
