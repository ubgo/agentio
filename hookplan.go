package agentio

import (
	"errors"
	"io"
)

// ErrInvalidHook means a hook entry or plan PlanHooks / ApplyHooks refuses:
// an empty command, or a plan made for another agent's file.
var ErrInvalidHook = errors.New("agentio: invalid hook entry")

// HookEntry is one command hook: the neutral kind, the vendor event it
// maps to, and the shell command the agent runs.
type HookEntry struct {
	Kind HookKind
	// Event is the vendor's event name ("UserPromptSubmit", "Stop",
	// "BeforeAgent"); PlanHooks fills it from Kind.
	Event string
	// Command is the shell command line the agent runs, verbatim.
	Command string
}

// HookPlan is what installing a set of hooks into one home would change.
// It is data: PlanHooks reads, ApplyHooks writes exactly this plan.
type HookPlan struct {
	Agent ID
	Home  Home
	// File is the settings file the plan changes (Claude settings.json,
	// Codex hooks.json, Gemini settings.json).
	File string
	// Add are the entries ApplyHooks appends; existing hooks of any other
	// command are kept as they are, in their order.
	Add []HookEntry
	// Present are requested entries the file already holds (same event,
	// same command), so a second install changes nothing.
	Present []HookEntry
	// Manual are steps the library does not take for the hooks to run
	// (Codex: a feature flag in config.toml, which agentio keeps
	// read-only), each one line for the user.
	Manual []string
}

// IsNoop reports a plan with nothing to write. Pure.
func (p HookPlan) IsNoop() bool { return len(p.Add) == 0 }

// HookCall is one decoded hook invocation, the stdin payload an agent
// sends a hook command, mapped to neutral fields. Fields the agent does
// not send stay "".
type HookCall struct {
	Agent ID
	// Kind is the neutral kind of Event; HookUnknown for an event with
	// none.
	Kind HookKind
	// Event is the vendor event name from the payload.
	Event          string
	ConversationID string
	Dir            string
	TranscriptPath string
	// Prompt is the submitted prompt (prompt-submit).
	Prompt string
	// LastAssistantMessage is the answer just finished (turn-end), when
	// the agent sends it inline.
	LastAssistantMessage string
	PermissionMode       string
	Model                string
	// Raw is the whole payload as received.
	Raw Raw
}

// HookInstaller installs and decodes command hooks for an agent (an
// optional capability; HooksOf). Installing merges: a plan only appends
// its own entries, never replaces or reorders another hook, backs the file
// up first, and is idempotent.
type HookInstaller interface {
	// HookKinds lists the neutral kinds this agent can run a command at.
	// Pure.
	HookKinds() []HookKind
	// PlanHooks is Disk · Read: what installing want into h would add.
	// An entry whose Kind the agent does not support is an
	// ErrUnsupported error naming it.
	PlanHooks(h Home, want []HookEntry) (HookPlan, error)
	// ApplyHooks is Disk · Write: appends p.Add to p.File after a backup.
	// Entries that appeared since the plan was made are not added twice.
	// A no-op plan writes nothing and returns an empty Backup.
	ApplyHooks(p HookPlan) (Backup, error)
	// RemoveHooks is Disk · Write: removes every command hook in h's
	// hook file whose command match reports true, and nothing else (a
	// group left without handlers is removed with it). Zero removed
	// writes nothing.
	RemoveHooks(h Home, match func(command string) bool) (removed int, b Backup, err error)
	// DecodeHook is Pure (reads r): one hook payload.
	DecodeHook(r io.Reader) (HookCall, error)
}
