package agentio

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Spec is an agent described as data (SPEC §5.6). A config-only agent is
// nothing but a Spec; built-ins build their Core from their own Spec too,
// so launch, home env and discovery are data for every agent. A Spec can
// only describe L1 plus the data-driven capabilities (process
// classification, login command, home links, binary version, install
// version from package metadata, L1 doctor); usage, identity and
// conversations need code.
//
// Every field has a snake_case JSON tag, so a program can decode a Spec
// from its own config (agx: an agents.<id>.spec block) with encoding/json.
// Enums (Autonomy keys, TokenKind) decode from their wire names; Validate
// is the check to run after decoding.
type Spec struct {
	ID ID `json:"id"`
	// Name is the display name.
	Name string `json:"name"`
	// Binary is the executable name looked up on PATH.
	Binary string `json:"binary"`
	// HomeEnv selects a non-default home; "" = one home only. By default its
	// value is the home dir itself (CLAUDE_CONFIG_DIR, CODEX_HOME).
	HomeEnv string `json:"home_env,omitempty"`
	// HomeEnvNamesParent says HomeEnv names the dir that CONTAINS the home,
	// not the home: Gemini CLI reads GEMINI_CLI_HOME and uses
	// $GEMINI_CLI_HOME/.gemini. Every home is then <parent>/<DefaultHome>
	// (DefaultHome must be one path element), the variable is set to the
	// parent, and a home whose base is not DefaultHome cannot be selected.
	HomeEnvNamesParent bool `json:"home_env_names_parent,omitempty"`
	// DefaultHome is relative to Host.UserHome, e.g. ".codex".
	DefaultHome string `json:"default_home,omitempty"`
	// ExtraHomes is a glob relative to Host.UserHome, e.g. ".claude-*"; "" = none.
	ExtraHomes string `json:"extra_homes,omitempty"`
	// HomeMarkers are entries that make a dir a real home; empty = any dir.
	HomeMarkers []string `json:"home_markers,omitempty"`
	// NewArgs and ResumeArgs are argv templates with {args} (where the
	// computed args go; appended when absent) and {id} (resume id).
	NewArgs    []string `json:"new_args,omitempty"`
	ResumeArgs []string `json:"resume_args,omitempty"`
	// ModelArgs ({model}) and AddDirArgs ({dir}, repeated per dir) are
	// optional templates; absent = ErrUnsupported for that request field.
	ModelArgs  []string `json:"model_args,omitempty"`
	AddDirArgs []string `json:"add_dir_args,omitempty"`
	// PromptArgs ({prompt}) passes an initial prompt; absent = Prompt is
	// ErrUnsupported. Added beyond SPEC §5.6 so a config agent can take one.
	PromptArgs []string `json:"prompt_args,omitempty"`
	// AutonomyArgs are the args for each autonomy level. A level without an
	// entry is refused (ErrUnsupported), never mapped to a guess; an entry
	// with no args is allowed and adds nothing (pi never asks, so its only
	// honest level is "full": {"full": []}). AutonomyDefault needs no entry
	// (no args); AutonomyFull still needs Target.AllowDangerous.
	AutonomyArgs map[Autonomy][]string `json:"autonomy_args,omitempty"`
	// HeadlessArgs make a launch non-interactive with machine-readable
	// output (Gemini: ["--output-format", "stream-json"]); absent = Headless
	// is ErrUnsupported.
	HeadlessArgs []string `json:"headless_args,omitempty"`
	// TokenEnv says which variable carries a token login, per token kind and
	// provider; no entry for the requested (kind, provider) = the token login
	// is ErrUnsupported. See TokenEnv for the scrub rule.
	TokenEnv []TokenEnv `json:"token_env,omitempty"`
	// VersionArgs enables BinaryVersion through a Runner (e.g. ["--version"]).
	VersionArgs []string `json:"version_args,omitempty"`
	// Packages are the npm package names the binary may come from
	// ("@google/gemini-cli"). With one set, Install reads the version from
	// that package's package.json next to the resolved binary, without
	// running it (InstallFromPath also knows Homebrew Cellar and native
	// version dirs).
	Packages []string `json:"packages,omitempty"`
	// LoginArgs enables LoginCommander (browser flow only for config agents).
	LoginArgs []string `json:"login_args,omitempty"`
	// LogoutArgs is the logout argv; empty = LogoutCommand returns a zero Command.
	// Added beyond SPEC §5.6 because LoginCommander needs a logout command.
	LogoutArgs []string `json:"logout_args,omitempty"`
	// SharedHomeFiles enables HomeLinker: entries of the default home a new
	// home reaches through a symlink.
	SharedHomeFiles []string `json:"shared_home_files,omitempty"`
	// CredentialEnv are the inherited variables that would override a
	// home's STORED login. A launch on the home's own login scrubs them; a
	// token login scrubs only the ones that would override that token (see
	// TokenEnv.Scrub). A variable that IS an agent's login mechanism (pi's
	// provider keys, a Gemini API-key home's GEMINI_API_KEY) does not belong
	// here: it would be removed from the user who set it on purpose.
	CredentialEnv []string `json:"credential_env,omitempty"`
	// APIBillingEnv are variables that switch the agent to per-token billing.
	APIBillingEnv []string `json:"api_billing_env,omitempty"`
	// Process recognises a process row as this agent.
	Process ProcessMatch `json:"process,omitzero"`
}

// TokenEnv maps one token login (kind, provider) to the variable the agent
// reads it from.
type TokenEnv struct {
	// Kind is the login kind: oauth-token, api-key or api-key-helper.
	Kind TokenKind `json:"kind"`
	// Provider narrows the entry for agents whose variable depends on the
	// model provider (pi: anthropic -> ANTHROPIC_API_KEY, openai ->
	// OPENAI_API_KEY). "" = any provider; a request without a provider
	// matches only when exactly one entry has its kind.
	Provider string `json:"provider,omitempty"`
	// Env is the variable that receives the token.
	Env string `json:"env"`
	// Scrub are the inherited variables that would override this token and
	// are removed for this login only. nil = Spec.CredentialEnv minus Env
	// (Claude: an OAuth-token login removes ANTHROPIC_API_KEY and
	// ANTHROPIC_AUTH_TOKEN, which outrank it); an empty, non-nil list
	// removes nothing.
	Scrub []string `json:"scrub,omitempty"`
}

// ProcessMatch is how a process row is recognised as an agent.
type ProcessMatch struct {
	// Basenames match argv[0] or Exe basename: "claude", "codex".
	Basenames []string `json:"basenames,omitempty"`
	// ArgvContains match a substring of any argv element: "@openai/codex".
	ArgvContains []string `json:"argv_contains,omitempty"`
	// Exclude substrings of Exe or any argv element veto a match:
	// "Claude.app", "claude-in-chrome".
	Exclude []string `json:"exclude,omitempty"`
}

// IsZero reports an empty matcher (the agent classifies nothing). Pure.
func (m ProcessMatch) IsZero() bool {
	return len(m.Basenames) == 0 && len(m.ArgvContains) == 0
}

// Match reports whether p is this agent's process. Pure.
func (m ProcessMatch) Match(p Process) bool {
	if m.IsZero() {
		return false
	}
	fields := append([]string{p.Exe}, p.Argv...)
	for _, ex := range m.Exclude {
		for _, f := range fields {
			if ex != "" && strings.Contains(f, ex) {
				return false
			}
		}
	}
	var names []string
	if p.Exe != "" {
		names = append(names, filepath.Base(p.Exe))
	}
	if len(p.Argv) > 0 && p.Argv[0] != "" {
		names = append(names, filepath.Base(p.Argv[0]))
	}
	for _, n := range names {
		if slices.Contains(m.Basenames, n) {
			return true
		}
	}
	for _, sub := range m.ArgvContains {
		for _, a := range p.Argv {
			if sub != "" && strings.Contains(a, sub) {
				return true
			}
		}
	}
	return false
}

// Template placeholders.
const (
	placeholderArgs   = "{args}"
	placeholderID     = "{id}"
	placeholderModel  = "{model}"
	placeholderDir    = "{dir}"
	placeholderPrompt = "{prompt}"
)

// Validate checks a Spec: ID charset, Binary set, DefaultHome relative and
// set when HomeEnv is, every non-empty template holds its placeholder.
// Pure.
func (s Spec) Validate() error {
	if _, err := ParseID(string(s.ID)); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}
	if s.Binary == "" {
		return fmt.Errorf("%w: %s: binary is empty", ErrInvalidSpec, s.ID)
	}
	if s.HomeEnv != "" && s.DefaultHome == "" {
		return fmt.Errorf("%w: %s: home env %s needs a default home", ErrInvalidSpec, s.ID, s.HomeEnv)
	}
	if s.DefaultHome != "" && (filepath.IsAbs(s.DefaultHome) || !filepath.IsLocal(s.DefaultHome)) {
		return fmt.Errorf("%w: %s: default home %q must be relative to the user home", ErrInvalidSpec, s.ID, s.DefaultHome)
	}
	if s.ExtraHomes != "" {
		if _, err := filepath.Match(s.ExtraHomes, ""); err != nil || !filepath.IsLocal(s.ExtraHomes) {
			return fmt.Errorf("%w: %s: extra homes glob %q is invalid", ErrInvalidSpec, s.ID, s.ExtraHomes)
		}
	}
	if s.HomeEnvNamesParent && (s.HomeEnv == "" || strings.ContainsAny(s.DefaultHome, `/\`)) {
		return fmt.Errorf("%w: %s: home_env_names_parent needs home_env and a one-element default home, not %q", ErrInvalidSpec, s.ID, s.DefaultHome)
	}
	if err := s.validateTokenEnv(); err != nil {
		return err
	}
	for a := range s.AutonomyArgs {
		if _, ok := ParseAutonomy(a.String()); !ok {
			return fmt.Errorf("%w: %s: autonomy_args has an unknown level %d", ErrInvalidSpec, s.ID, a)
		}
	}
	for _, t := range []struct {
		name, placeholder string
		tpl               []string
	}{
		{"resume args", placeholderID, s.ResumeArgs},
		{"model args", placeholderModel, s.ModelArgs},
		{"add-dir args", placeholderDir, s.AddDirArgs},
		{"prompt args", placeholderPrompt, s.PromptArgs},
	} {
		if len(t.tpl) > 0 && !templateHas(t.tpl, t.placeholder) {
			return fmt.Errorf("%w: %s: %s %v lack %s", ErrInvalidSpec, s.ID, t.name, t.tpl, t.placeholder)
		}
	}
	return nil
}

// validateTokenEnv checks every TokenEnv entry: a known kind, a variable,
// and no two entries for the same (kind, provider). Pure.
func (s Spec) validateTokenEnv() error {
	type key struct {
		kind     TokenKind
		provider string
	}
	seen := map[key]bool{}
	for _, e := range s.TokenEnv {
		if e.Kind == TokenKindUnknown || e.Env == "" {
			return fmt.Errorf("%w: %s: token_env entry %+v needs a kind and an env", ErrInvalidSpec, s.ID, e)
		}
		k := key{e.Kind, e.Provider}
		if seen[k] {
			return fmt.Errorf("%w: %s: token_env has two entries for %s/%s", ErrInvalidSpec, s.ID, e.Kind, e.Provider)
		}
		seen[k] = true
	}
	return nil
}

// tokenEnvFor picks the TokenEnv entry for cred: an exact (kind, provider)
// match, else, for a credential without a provider, the only entry of its
// kind. ok false = no entry (or several without a provider to choose). Pure.
func (s Spec) tokenEnvFor(cred TokenCredential) (TokenEnv, bool) {
	var ofKind []TokenEnv
	for _, e := range s.TokenEnv {
		if e.Kind != cred.Kind {
			continue
		}
		if e.Provider == cred.Provider {
			return e, true
		}
		ofKind = append(ofKind, e)
	}
	if cred.Provider == "" && len(ofKind) == 1 {
		return ofKind[0], true
	}
	return TokenEnv{}, false
}

// tokenLogin is tokenEnvFor with the refusal both token paths share
// (Core.Launch and a spec agent's TokenLogin.TokenEnv), so they word a
// login token_env does not describe the same way: ErrUnsupported naming
// the kind and, when given, the provider. Pure.
func (s Spec) tokenLogin(cred TokenCredential) (TokenEnv, error) {
	if e, ok := s.tokenEnvFor(cred); ok {
		return e, nil
	}
	what := "token login " + cred.Kind.String()
	if cred.Provider != "" {
		what += " for provider " + cred.Provider
	}
	return TokenEnv{}, UnsupportedError(s.ID, what, specUndescribedReason)
}

// scrubFor is the list a token login through e removes: e.Scrub when set,
// else CredentialEnv without e.Env. Pure.
func (s Spec) scrubFor(e TokenEnv) []string {
	if e.Scrub != nil {
		return e.Scrub
	}
	out := make([]string, 0, len(s.CredentialEnv))
	for _, n := range s.CredentialEnv {
		if n != e.Env {
			out = append(out, n)
		}
	}
	return out
}

// templateHas reports a placeholder inside any element.
func templateHas(tpl []string, placeholder string) bool {
	for _, e := range tpl {
		if strings.Contains(e, placeholder) {
			return true
		}
	}
	return false
}

// expandTemplate replaces placeholder with value in every element.
func expandTemplate(tpl []string, placeholder, value string) []string {
	out := make([]string, len(tpl))
	for i, e := range tpl {
		out[i] = strings.ReplaceAll(e, placeholder, value)
	}
	return out
}

// spliceArgs puts args where {args} stands in tpl, or appends them.
func spliceArgs(tpl, args []string) []string {
	out := make([]string, 0, len(tpl)+len(args))
	spliced := false
	for _, e := range tpl {
		if e == placeholderArgs {
			out = append(out, args...)
			spliced = true
			continue
		}
		out = append(out, e)
	}
	if !spliced {
		out = append(out, args...)
	}
	return out
}
