package agentio

// Agent is the L1 core every agent has, including a config-only agent
// described by a Spec (SPEC §5.3). Vendor agents (*claude.Agent,
// *codex.Agent) are not Agents; they hand out their neutral view with
// Neutral(), which keeps vendor method names free for rich vendor types.
// Shared capabilities are reached through the typed accessors (UsageOf, …),
// never through type assertions in caller code.
type Agent interface {
	// ID is Pure.
	ID() ID
	// Name is the display name ("Claude Code"). Pure.
	Name() string
	// Spec is the data this agent's core is built from. Pure.
	Spec() Spec
	// LookPath finds the binary by walking PATH from Host.Env, never a
	// shell (invariant 12). Disk · Read.
	LookPath() (string, error)
	// DefaultHome is Pure.
	DefaultHome() Home
	// HomeFromEnv is the home Host.Env selects through Spec.HomeEnv. Pure.
	HomeFromEnv() (Home, bool)
	// HomeAt is the Home value for a folder the caller names (a --home
	// flag): the default home when dir is the default dir, else a
	// non-default home named from the folder; relative dirs resolve
	// against Host.UserHome. It does not check the folder exists or is
	// a home (IsHome does). Pure.
	HomeAt(dir string) Home
	// Homes lists homes on disk, default first, only dirs with markers. Disk · Read.
	Homes() ([]Home, error)
	// IsHome reports whether dir is a home of this agent. Disk · Read.
	IsHome(dir string) (bool, error)
	// Launch builds the command for one invocation. Pure.
	Launch(t Target, r LaunchRequest) (Command, error)
}

// LaunchRequest is the launch subset every agent maps (SPEC §5.4). A field
// the agent cannot express returns ErrUnsupported, never a silent drop.
type LaunchRequest struct {
	// Dir is the working directory; "" = the caller's.
	Dir string
	// Prompt is the initial prompt; "" = interactive, no prompt.
	Prompt string
	// ResumeID resumes a conversation; "" = new.
	ResumeID string
	// Model is the model; "" = vendor default.
	Model string
	// AddDirs are extra directories the agent may access.
	AddDirs []string
	// Autonomy is the permission level; AutonomyDefault = no flag.
	Autonomy Autonomy
	// Headless asks for a non-interactive, machine-readable event stream.
	Headless bool
	// Credential is a token login; nil = the home's own login.
	Credential *TokenCredential
	// Args are appended last, verbatim.
	Args []string
}

// UnsupportedReasoner is implemented by neutral adapters that can say why a
// capability is missing ("conversations are indexed in SQLite with
// absolute paths"). Read by Unsupported.
type UnsupportedReasoner interface {
	UnsupportedReason(c Capability) (reason string, declared bool)
}

// CapabilityGate is implemented by agents whose capability set depends on
// data (a config-only agent has LoginCommander only when Spec.LoginArgs is
// set). Accessors consult it after the type assertion; an agent without it
// has every capability its type implements.
type CapabilityGate interface {
	HasCapability(c Capability) bool
}
