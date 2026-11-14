package agentio

import (
	"fmt"
	"path/filepath"
)

// Home is one config dir of one agent (~/.claude, ~/.claude-work,
// ~/.codex). Fields are unexported so a Home always knows its owner: it is
// built only by an agent (or NewHome in a vendor package), and neutral
// methods return ErrForeignHome for another agent's home.
type Home struct {
	agent     ID
	dir       string
	isDefault bool
	name      string
}

// NewHome builds a Home. For vendor packages and Core; callers get homes
// from an agent (DefaultHome, Homes, HomeFromEnv). dir is cleaned. Pure.
func NewHome(agent ID, dir string, isDefault bool, name string) Home {
	if dir != "" {
		dir = filepath.Clean(dir)
	}
	return Home{agent: agent, dir: dir, isDefault: isDefault, name: name}
}

// Agent returns the owning agent's ID. Pure.
func (h Home) Agent() ID { return h.agent }

// Dir returns the absolute, cleaned directory. Pure.
func (h Home) Dir() string { return h.dir }

// IsDefault reports the default home: launched with the home variable
// ABSENT from the child env (invariant 2), never set to the default path.
// Pure.
func (h Home) IsDefault() bool { return h.isDefault }

// Name is a suggested short name ("default", "work" from ~/.claude-work). Pure.
func (h Home) Name() string { return h.name }

// IsZero reports a Home that was never built. Pure.
func (h Home) IsZero() bool { return h.agent == "" && h.dir == "" }

// String renders "agent:dir" for messages. Pure.
func (h Home) String() string { return string(h.agent) + ":" + h.dir }

// Path joins elem onto the home dir. Pure.
func (h Home) Path(elem ...string) string {
	return filepath.Join(append([]string{h.dir}, elem...)...)
}

// homeJSON is the JSON shape of a Home (fields are unexported in Go).
type homeJSON struct {
	Agent     ID     `json:"agent"`
	Dir       string `json:"dir"`
	IsDefault bool   `json:"is_default"`
	Name      string `json:"name"`
}

// MarshalJSON writes {agent, dir, is_default, name}. Pure.
func (h Home) MarshalJSON() ([]byte, error) {
	return marshalJSON(homeJSON{Agent: h.agent, Dir: h.dir, IsDefault: h.isDefault, Name: h.name})
}

// Target is one home plus the launch inputs that come from the caller's
// profile rather than from one invocation (SPEC §5.2).
func (h Home) Target() Target { return Target{Home: h} }

// CheckOwner returns ErrForeignHome when h is not a home of agent. Pure.
func CheckOwner(agent ID, h Home) error {
	if h.agent != agent {
		return fmt.Errorf("%w: %s home passed to %s", ErrForeignHome, h.agent, agent)
	}
	return nil
}

// Target is what a launch needs from the caller beyond one LaunchRequest.
type Target struct {
	Home Home
	// ExtraArgs go before the request's own args (profile args).
	ExtraArgs []string
	// ExtraEnv is applied onto Host.Env before the credential and home rules,
	// so it can never override the home variable or reintroduce a scrubbed
	// credential variable.
	ExtraEnv []EnvChange
	// Billing is the caller's billing hint for this target; BillingUnknown lets the agent decide.
	Billing Billing
	// AllowDangerous must be true for AutonomyFull.
	AllowDangerous bool
	// Version is the installed vendor version; zero = unknown (build and warn).
	Version Version
}
