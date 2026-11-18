// Package pi is the typed API of the pi coding agent (npm
// @earendil-works/pi-coding-agent, formerly @mariozechner/pi-coding-agent):
// its install, agent dirs (homes), settings, sessions, instruction files,
// MCP servers, skills and prompt templates, plus the commands that launch
// or resume it. Every shape is recorded from pi 1.5.2 (live where a run
// without a login could produce it, otherwise from the shipped source and
// docs, marked source-derived in the API notes).
//
// pi has no plan or quota and no permission prompts: usage windows are
// unsupported with that reason, and the only autonomy levels are
// read-only (the documented tool allowlist) and full.
package pi

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ubgo/agentio"
)

// ID is pi's agent id.
const ID agentio.ID = "pi"

// Agent is pi on one Host. Like claude.Agent it is not an agentio.Agent
// itself: Neutral() returns the L1/L2 view, so vendor methods keep their
// rich types.
type Agent struct {
	host agentio.Host
	// core is the shared L1 implementation built from Spec(): homes, env
	// rule, PATH lookup, Command assembly.
	core *agentio.Core
}

// New returns the pi agent for h. Pure; no error, no I/O. Spec() is a
// constant that TestSpecValidates proves valid, so NewCore cannot fail
// here; a nil core would only follow an edit that breaks that test.
func New(h agentio.Host) *Agent {
	core, err := agentio.NewCore(Spec(), h)
	if err != nil {
		core = nil
	}
	return &Agent{host: h, core: core}
}

// Host returns the host the agent reads through. Pure.
func (a *Agent) Host() agentio.Host { return a.host }

// ID returns pi.ID. Pure.
func (a *Agent) ID() agentio.ID { return ID }

// Name returns "pi". Pure.
func (a *Agent) Name() string { return displayName }

// homeMarkers are the entries that make a dir a pi agent dir. pi writes
// auth.json and models-store.json on every start, even for --help, and
// creates sessions/ on the first run in a folder (observed live, a fresh
// HOME); settings.json is written only by /settings and friends, so it is a
// marker for older homes but never the only one.
func homeMarkers() []string {
	return []string{authFile, modelsStoreFile, settingsFile, sessionsDir}
}

// HomeMarkers lists the marker entries of an agent dir. Pure.
func HomeMarkers() []string { return homeMarkers() }

// genericHomeMarkers are the markers other agents' homes hold too:
// settings.json (Claude Code, Gemini CLI), sessions/ (Claude Code, Codex)
// and auth.json (Codex). Alone they mark a home only in ~/.pi/agent or the
// PI_CODING_AGENT_DIR folder; a folder found elsewhere needs
// models-store.json, which pi writes on every start.
func genericHomeMarkers() []string { return []string{settingsFile, sessionsDir, authFile} }

// tokenEnv maps a provider to the variable pi reads its API key from
// (node_modules/@earendil-works/pi-ai/dist/env-api-keys.js:68-120 in pi
// 1.5.2). Only providers with exactly one key variable are listed: a
// provider whose key comes from several variables or ambient credentials
// (amazon-bedrock, google-vertex ADC) has no single variable to set. The
// Scrub list is empty: another provider's variable never overrides this
// login, and pi reads a stored auth.json credential before any variable.
func tokenEnv() []agentio.TokenEnv {
	pairs := []struct{ provider, env string }{
		{"anthropic", "ANTHROPIC_API_KEY"},
		{"openai", "OPENAI_API_KEY"},
		{"google", "GEMINI_API_KEY"},
		{"google-vertex", "GOOGLE_CLOUD_API_KEY"},
		{"azure", "AZURE_OPENAI_API_KEY"},
		{"deepseek", "DEEPSEEK_API_KEY"},
		{"groq", "GROQ_API_KEY"},
		{"cerebras", "CEREBRAS_API_KEY"},
		{"xai", "XAI_API_KEY"},
		{"openrouter", "OPENROUTER_API_KEY"},
		{"mistral", "MISTRAL_API_KEY"},
		{"github-copilot", "COPILOT_GITHUB_TOKEN"},
		{"huggingface", "HF_TOKEN"},
		{"fireworks", "FIREWORKS_API_KEY"},
		{"together", "TOGETHER_API_KEY"},
		{"zai", "ZAI_API_KEY"},
		{"nvidia", "NVIDIA_API_KEY"},
		{"vercel-ai-gateway", "AI_GATEWAY_API_KEY"},
	}
	out := make([]agentio.TokenEnv, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, agentio.TokenEnv{Kind: agentio.TokenAPIKey, Provider: p.provider, Env: p.env, Scrub: []string{}})
	}
	return out
}

// autonomyArgs are pi's honest autonomy levels. pi never asks before a
// tool call (PKG/docs/security.md:3), so "full" adds nothing and ask /
// accept-edits are refused. "read-only" is the allowlist pi --help names
// "Read-only mode (no file modifications possible)", plus --no-mcp and
// --no-extensions because MCP and extension tools stay enabled under
// --tools otherwise (PKG/docs/cli.md:113-158).
func autonomyArgs() map[agentio.Autonomy][]string {
	return map[agentio.Autonomy][]string{
		agentio.AutonomyReadOnly: {flagTools, joinList(readOnlyTools()), flagNoMCP, flagNoExtensions},
		agentio.AutonomyFull:     {},
	}
}

// Spec returns the data pi's core is built from. CredentialEnv is empty
// on purpose: pi authenticates BY the provider variables, and a stored
// auth.json credential "owns the provider: ambient/env is consulted only
// when nothing is stored" (pi-ai/dist/auth/resolve.js:4-39), so no
// variable overrides a home's stored login and none may be scrubbed.
// Pure.
func Spec() agentio.Spec {
	return agentio.Spec{
		ID: ID, Name: displayName, Binary: Binary,
		HomeEnv: HomeEnv, DefaultHome: defaultHomeDir,
		HomeMarkers:        homeMarkers(),
		GenericHomeMarkers: genericHomeMarkers(),
		NewArgs:            []string{placeholderArgs},
		ResumeArgs:         []string{flagSession, placeholderID, placeholderArgs},
		ModelArgs:          []string{flagModel, placeholderModel},
		PromptArgs:         []string{endOfOptions, placeholderPrompt},
		AutonomyArgs:       autonomyArgs(),
		HeadlessArgs:       []string{flagMode, string(ModeJSON)},
		TokenEnv:           tokenEnv(),
		VersionArgs:        []string{flagVersion},
		Packages:           []string{PackageName, legacyPackageName},
		// pi sets process.title = "pi" (PKG/dist/cli/setup.js:4), which
		// overwrites argv on linux: the row's argv0 is "pi" and its exe is
		// node, so only the basename can match; the package path never
		// appears in argv.
		Process: agentio.ProcessMatch{Basenames: []string{Binary}},
	}
}

// Spec returns pi.Spec(). Pure.
func (a *Agent) Spec() agentio.Spec { return Spec() }

// Home is one pi agent dir. It wraps the neutral handle so every pi
// method is compile-checked against pi.
type Home struct{ h agentio.Home }

// Neutral returns the shared handle. Pure.
func (h Home) Neutral() agentio.Home { return h.h }

// Dir is the agent dir. Pure.
func (h Home) Dir() string { return h.h.Dir() }

// IsDefault reports the default home (~/.pi/agent, PI_CODING_AGENT_DIR
// absent). Pure.
func (h Home) IsDefault() bool { return h.h.IsDefault() }

// Name is the suggested name ("personal" for the default). Pure.
func (h Home) Name() string { return h.h.Name() }

// Target is a plain launch target for this home. Pure.
func (h Home) Target() agentio.Target { return h.h.Target() }

// String is "pi:<dir>". Pure.
func (h Home) String() string { return h.h.String() }

// MarshalJSON encodes the neutral handle. Pure.
func (h Home) MarshalJSON() ([]byte, error) { return h.h.MarshalJSON() }

// wrap converts a neutral home of this agent.
func wrap(h agentio.Home) Home { return Home{h: h} }

// HomeAt returns the home at a caller-named dir (any PI_CODING_AGENT_DIR);
// relative dirs resolve against UserHome. Pure.
func (a *Agent) HomeAt(dir string) Home { return wrap(a.core.HomeAt(dir)) }

// HomeOf converts a neutral home; ErrForeignHome for another agent's. Pure.
func (a *Agent) HomeOf(n agentio.Home) (Home, error) {
	if err := agentio.CheckOwner(ID, n); err != nil {
		return Home{}, err
	}
	return Home{h: n}, nil
}

// DefaultHome is ~/.pi/agent. Pure.
func (a *Agent) DefaultHome() Home { return wrap(a.core.DefaultHome()) }

// HomeFromEnv returns the home PI_CODING_AGENT_DIR in Host.Env selects;
// false when absent or empty (pi falls back to the default, config.js:511).
// Pure.
func (a *Agent) HomeFromEnv() (Home, bool) {
	h, ok := a.core.HomeFromEnv()
	return wrap(h), ok
}

// Homes lists ~/.pi/agent when it is a home, then the PI_CODING_AGENT_DIR
// home when not listed. pi has no naming convention for extra agent dirs,
// so nothing is globbed. Disk · Read.
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

// IsHome reports whether dir holds any home marker. Disk · Read.
func (a *Agent) IsHome(dir string) (bool, error) { return a.core.IsHome(dir) }

// LookPath finds pi by walking PATH from Host.Env, never a shell.
// Disk · Read.
func (a *Agent) LookPath() (string, error) { return a.core.LookPath() }

// Paths are the files of one agent dir.
type Paths struct {
	Dir, Settings, Auth, Models, ModelsStore, MCP, Trust string
	Sessions, Skills, Prompts, Extensions                string
	System, AppendSystem                                 string
}

// Paths lists h's files (they may not exist). Pure.
func (a *Agent) Paths(h Home) Paths {
	d := h.Dir()
	return Paths{
		Dir: d, Settings: filepath.Join(d, settingsFile), Auth: filepath.Join(d, authFile),
		Models: filepath.Join(d, modelsFile), ModelsStore: filepath.Join(d, modelsStoreFile),
		MCP: filepath.Join(d, mcpFile), Trust: filepath.Join(d, trustFile),
		Sessions: filepath.Join(d, sessionsDir), Skills: filepath.Join(d, skillsDir),
		Prompts: filepath.Join(d, promptsDir), Extensions: filepath.Join(d, extensionsDir),
		System: filepath.Join(d, systemFile), AppendSystem: filepath.Join(d, appendFile),
	}
}

// Install finds pi on PATH and reads its version from the package.json
// of either npm package above the resolved binary; it never runs pi.
// Disk · Read.
func (a *Agent) Install() (agentio.Install, error) { return a.core.Install() }

// absDir resolves a project dir; "" stays "".
func absDir(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("pi: project %s: %w", dir, err)
	}
	return abs, nil
}

// isDir reports an existing directory (following symlinks). Disk · Read.
func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
