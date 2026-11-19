// Package gemini is agentio's typed API for Google's Gemini CLI
// (@google/gemini-cli): install and version from package metadata, homes
// (GEMINI_CLI_HOME names the dir that contains .gemini), settings with
// every key kept, MCP servers, instruction files, skills and extensions,
// the Google login state, conversations per project, and the launch
// command with its approval modes.
//
// Every shape is recorded from 0.68.0 (live where a file is created
// without a login, source-derived from the bundled CLI otherwise; see
// GEMINI_API.md). The package never logs in, never refreshes or prints a
// token, and writes nothing a vendor CLI owns.
package gemini

import (
	"context"
	"fmt"

	"github.com/ubgo/agentio"
)

// ID is Gemini CLI's agent id.
const ID agentio.ID = "gemini"

// Agent is Gemini CLI on one Host. Like claude.Agent it is not an
// agentio.Agent itself: Neutral() returns the L1/L2 view, so vendor
// methods keep their rich types.
type Agent struct {
	host agentio.Host
	// core is the shared L1 implementation built from Spec(): homes, the
	// parent-dir home variable, PATH lookup, Command assembly.
	core *agentio.Core
}

// New returns the Gemini agent for h. cost: Pure; no I/O. Spec() is a
// constant that TestSpecValidates proves valid, so NewCore cannot fail
// here; a nil core would only follow an edit that breaks that test.
func New(h agentio.Host) *Agent {
	core, err := agentio.NewCore(Spec(), h)
	if err != nil {
		core = nil
	}
	return &Agent{host: h, core: core}
}

// Host returns the host the agent reads through. cost: Pure.
func (a *Agent) Host() agentio.Host { return a.host }

// ID returns gemini.ID. cost: Pure.
func (a *Agent) ID() agentio.ID { return ID }

// Name returns "Gemini CLI". cost: Pure.
func (a *Agent) Name() string { return displayName }

// homeMarkers are entries the CLI writes into a home: projects.json on the
// first run of any command (observed live), settings.json once the user
// picks an auth method, the login caches, and tmp/ for project state.
func homeMarkers() []string {
	return []string{settingsFile, projectsFile, oauthCredsFile, googleAccountsFile, tmpDir}
}

// HomeMarkers lists the marker entries of a home. cost: Pure.
func HomeMarkers() []string { return homeMarkers() }

// genericHomeMarkers are the markers other tools write too: settings.json
// (Claude Code, pi) and tmp/. Alone they mark a home only in ~/.gemini or
// the GEMINI_CLI_HOME folder; a folder found elsewhere needs projects.json
// (written by any command) or a login cache.
func genericHomeMarkers() []string { return []string{settingsFile, tmpDir} }

// sharedHomeFiles are linked from ~/.gemini into a new home: the user's
// own instructions, custom commands, skills and extensions. settings.json
// stays per home because it holds security.auth.selectedType (each home's
// login method); logins, projects and chats stay per home too.
func sharedHomeFiles() []string {
	return []string{defaultContextFile, commandsDir, skillsDir, extensionsDir}
}

// credentialEnv are the inherited variables that override a home's STORED
// login (recorded from the auth code, GEMINI_API.md §4):
//   - GOOGLE_CLOUD_ACCESS_TOKEN with GOOGLE_GENAI_USE_GCA replaces the
//     cached Google login with a raw access token;
//   - GOOGLE_APPLICATION_CREDENTIALS is tried right after oauth_creds.json
//     and wins silently when the home's file is invalid;
//   - GEMINI_CLI_AUTH_OVERRIDE overrides the home's selectedType.
//
// GEMINI_API_KEY is never listed: the home's selectedType wins over it, and
// for an API-key home it IS the login, so removing it would break a user
// who set it on purpose.
func credentialEnv() []string {
	return []string{CloudAccessTokenEnv, UseGCAEnv, AppCredentialsEnv, AuthOverrideEnv}
}

// apiBillingEnv are the variables that put a launch on a pay-per-token key
// when the home names no auth type.
func apiBillingEnv() []string { return []string{APIKeyEnv, GoogleAPIKeyEnv, UseVertexEnv} }

// autonomyArgs maps each agentio level onto --approval-mode (choices in
// 0.68.0 --help: default, auto_edit, yolo, plan). AutonomyDefault adds no
// flag, so settings general.defaultApprovalMode still applies.
func autonomyArgs() map[agentio.Autonomy][]string {
	return map[agentio.Autonomy][]string{
		agentio.AutonomyReadOnly:    {flagApprovalMode, string(ApprovalPlan)},
		agentio.AutonomyAsk:         {flagApprovalMode, string(ApprovalDefault)},
		agentio.AutonomyAcceptEdits: {flagApprovalMode, string(ApprovalAutoEdit)},
		agentio.AutonomyFull:        {flagApprovalMode, string(ApprovalYolo)},
	}
}

// Provider names for TokenCredential.Provider: a Gemini API key is the
// default (empty provider); a Vertex AI express key is "vertex".
const (
	ProviderGemini = ""
	ProviderVertex = "vertex"
)

// tokenEnv maps token logins onto variables. The scrub list of each is
// nil, so a token login removes credentialEnv: those variables would beat
// a key only through selectedType oauth-personal, which a token launch
// must not fall back to.
func tokenEnv() []agentio.TokenEnv {
	return []agentio.TokenEnv{
		{Kind: agentio.TokenAPIKey, Provider: ProviderGemini, Env: APIKeyEnv},
		{Kind: agentio.TokenAPIKey, Provider: ProviderVertex, Env: GoogleAPIKeyEnv},
	}
}

// Spec returns the data Gemini's core is built from. PromptArgs uses
// --prompt-interactive because LaunchRequest.Prompt is an initial prompt
// in an interactive session (--prompt makes the run headless); the neutral
// Launch switches to --prompt when Headless is set. cost: Pure.
func Spec() agentio.Spec {
	return agentio.Spec{
		ID: ID, Name: displayName, Binary: Binary,
		HomeEnv: HomeEnv, HomeEnvNamesParent: true, DefaultHome: defaultDirName,
		HomeMarkers:        homeMarkers(),
		GenericHomeMarkers: genericHomeMarkers(),
		NewArgs:            []string{placeholderArgs},
		ResumeArgs:         []string{flagResume, placeholderID, placeholderArgs},
		ModelArgs:          []string{flagModel, placeholderModel},
		AddDirArgs:         []string{flagIncludeDirs, placeholderDir},
		PromptArgs:         []string{flagPromptInteractive, placeholderPrompt},
		AutonomyArgs:       autonomyArgs(),
		HeadlessArgs:       []string{flagOutputFormat, outputStreamJSON},
		TokenEnv:           tokenEnv(),
		VersionArgs:        []string{flagVersion},
		Packages:           []string{Package},
		SharedHomeFiles:    sharedHomeFiles(),
		CredentialEnv:      credentialEnv(),
		APIBillingEnv:      apiBillingEnv(),
		// The standalone binary or npm shim is "gemini"; an npm install
		// runs as node with the shim (…/bin/gemini) or a path inside the
		// package (the bundle, a Homebrew libexec) as its script.
		Process: agentio.ProcessMatch{
			Basenames:       []string{Binary},
			ScriptBasenames: []string{Binary},
			ScriptContains:  []string{argvPackage},
		},
	}
}

// Spec returns gemini.Spec(). cost: Pure.
func (a *Agent) Spec() agentio.Spec { return Spec() }

// Home is one Gemini home dir (<parent>/.gemini). It wraps the neutral
// handle so every vendor method is compile-checked against Gemini.
type Home struct{ h agentio.Home }

// Neutral returns the shared handle. cost: Pure.
func (h Home) Neutral() agentio.Home { return h.h }

// Dir is the .gemini dir. cost: Pure.
func (h Home) Dir() string { return h.h.Dir() }

// IsDefault reports ~/.gemini (GEMINI_CLI_HOME absent). cost: Pure.
func (h Home) IsDefault() bool { return h.h.IsDefault() }

// Name is "personal" for the default home, else the parent dir's name
// (~/gemini-work/.gemini is "gemini-work"). cost: Pure.
func (h Home) Name() string { return h.h.Name() }

// Target is a plain launch target for this home. cost: Pure.
func (h Home) Target() agentio.Target { return h.h.Target() }

// Path joins elem under the home dir. cost: Pure.
func (h Home) Path(elem ...string) string { return h.h.Path(elem...) }

// String is "gemini:<dir>". cost: Pure.
func (h Home) String() string { return h.h.String() }

// MarshalJSON encodes the neutral handle. cost: Pure.
func (h Home) MarshalJSON() ([]byte, error) { return h.h.MarshalJSON() }

// HomeAt returns the home at dir, the .gemini dir itself (not its
// parent); relative dirs resolve against UserHome. cost: Pure.
func (a *Agent) HomeAt(dir string) Home { return Home{h: a.core.HomeAt(dir)} }

// HomeOf converts a neutral home; ErrForeignHome for another agent's.
// cost: Pure.
func (a *Agent) HomeOf(n agentio.Home) (Home, error) {
	if err := agentio.CheckOwner(ID, n); err != nil {
		return Home{}, err
	}
	return Home{h: n}, nil
}

// DefaultHome is ~/.gemini. cost: Pure.
func (a *Agent) DefaultHome() Home { return Home{h: a.core.DefaultHome()} }

// HomeFromEnv is the home GEMINI_CLI_HOME selects: $GEMINI_CLI_HOME/.gemini.
// cost: Pure.
func (a *Agent) HomeFromEnv() (Home, bool) {
	h, ok := a.core.HomeFromEnv()
	return Home{h: h}, ok
}

// Homes lists ~/.gemini and the GEMINI_CLI_HOME home, only dirs with a
// marker. Gemini has no ~/.gemini-* convention; other homes come from the
// caller (HomeAt). cost: Disk · Read.
func (a *Agent) Homes() ([]Home, error) {
	hs, err := a.core.Homes()
	if err != nil {
		return nil, fmt.Errorf("gemini: homes: %w", err)
	}
	out := make([]Home, 0, len(hs))
	for _, h := range hs {
		out = append(out, Home{h: h})
	}
	return out, nil
}

// IsHome reports whether dir holds any home marker. cost: Disk · Read.
func (a *Agent) IsHome(dir string) (bool, error) { return a.core.IsHome(dir) }

// LookPath finds gemini by walking PATH from Host.Env, never a shell.
// cost: Disk · Read.
func (a *Agent) LookPath() (string, error) { return a.core.LookPath() }

// Install finds gemini on PATH and reads its version from the package's
// package.json (npm, pnpm, Bun, Volta, Homebrew), never running it.
// cost: Disk · Read.
func (a *Agent) Install() (agentio.Install, error) { return a.core.Install() }

// Runner is the Process door: commands that run the gemini binary.
type Runner struct {
	agentio.Runner
}

// Runner returns the Process door. Holding it is the opt-in to running the
// binary (SPEC §4). cost: Pure.
func (a *Agent) Runner() Runner { return Runner{Runner: a.core.Runner()} }

// BinaryVersion runs `gemini --version`. cost: Process · Read.
func (r Runner) BinaryVersion(ctx context.Context) (agentio.Version, error) {
	return r.Version(ctx)
}
