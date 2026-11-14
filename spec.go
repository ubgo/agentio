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
// classification, login command, home links, binary version, L1 doctor);
// usage, identity and conversations need code.
type Spec struct {
	ID ID
	// Name is the display name; Binary the executable name looked up on PATH.
	Name, Binary string
	// HomeEnv selects a non-default home; "" = one home only.
	HomeEnv string
	// DefaultHome is relative to Host.UserHome, e.g. ".codex".
	DefaultHome string
	// ExtraHomes is a glob relative to Host.UserHome, e.g. ".claude-*"; "" = none.
	ExtraHomes string
	// HomeMarkers are entries that make a dir a real home; empty = any dir.
	HomeMarkers []string
	// NewArgs and ResumeArgs are argv templates with {args} (where the
	// computed args go; appended when absent) and {id} (resume id).
	NewArgs, ResumeArgs []string
	// ModelArgs ({model}) and AddDirArgs ({dir}, repeated per dir) are
	// optional templates; absent = ErrUnsupported for that request field.
	ModelArgs, AddDirArgs []string
	// PromptArgs ({prompt}) passes an initial prompt; absent = Prompt is
	// ErrUnsupported. Added beyond SPEC §5.6 so a config agent can take one.
	PromptArgs []string
	// VersionArgs enables BinaryVersion through a Runner (e.g. ["--version"]).
	VersionArgs []string
	// LoginArgs enables LoginCommander (browser flow only for config agents).
	LoginArgs []string
	// LogoutArgs is the logout argv; empty = LogoutCommand returns a zero Command.
	// Added beyond SPEC §5.6 because LoginCommander needs a logout command.
	LogoutArgs []string
	// SharedHomeFiles enables HomeLinker: entries of the default home a new
	// home reaches through a symlink.
	SharedHomeFiles []string
	// CredentialEnv are the inherited variables that would override a
	// home's stored login; every launch scrubs them from the base env, and a
	// token login scrubs them again after Target.ExtraEnv (invariant 9).
	CredentialEnv []string
	// APIBillingEnv are variables that switch the agent to per-token billing.
	APIBillingEnv []string
	// Process recognises a process row as this agent.
	Process ProcessMatch
}

// ProcessMatch is how a process row is recognised as an agent.
type ProcessMatch struct {
	// Basenames match argv[0] or Exe basename: "claude", "codex".
	Basenames []string
	// ArgvContains match a substring of any argv element: "@openai/codex".
	ArgvContains []string
	// Exclude substrings of Exe or any argv element veto a match:
	// "Claude.app", "claude-in-chrome".
	Exclude []string
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
