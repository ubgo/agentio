package hookjson

import (
	"fmt"

	"github.com/ubgo/agentio"
)

// This file is the neutral glue the three vendor HookInstallers share:
// validating the wanted entries, splitting them into present and missing
// against the file bytes, and checking a plan before it is applied.

// Resolve validates want for one agent and fills each Event from its Kind
// (a caller's Event is replaced, so a plan always names the vendor's
// spelling). A kind event does not map is an agentio.ErrUnsupported error
// carrying reason; an empty command is agentio.ErrInvalidHook. Repeated
// entries are kept once, in first-seen order. Pure.
func Resolve(agent agentio.ID, want []agentio.HookEntry, event func(agentio.HookKind) (string, bool), reason string) ([]agentio.HookEntry, error) {
	var out []agentio.HookEntry
	seen := map[agentio.HookEntry]bool{}
	for _, w := range want {
		ev, ok := event(w.Kind)
		if !ok {
			return nil, agentio.UnsupportedError(agent, "hook kind "+w.Kind.String(), reason)
		}
		if w.Command == "" {
			return nil, fmt.Errorf("%w: %s %s: empty command", agentio.ErrInvalidHook, agent, w.Kind)
		}
		e := agentio.HookEntry{Kind: w.Kind, Event: ev, Command: w.Command}
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out, nil
}

// Split partitions entries into those b already holds (a command hook with
// the same event and command, in any group) and those it lacks. Pure.
func Split(b []byte, entries []agentio.HookEntry, opt Options) (present, missing []agentio.HookEntry, err error) {
	hs, err := Handlers(b, opt)
	if err != nil {
		return nil, nil, err
	}
	have := map[[2]string]bool{}
	for _, h := range hs {
		if h.IsCommand {
			have[[2]string{h.Event, h.Command}] = true
		}
	}
	for _, e := range entries {
		if have[[2]string{e.Event, e.Command}] {
			present = append(present, e)
		} else {
			missing = append(missing, e)
		}
	}
	return present, missing, nil
}

// Existing lists every command hook b holds as entries, in file order,
// with Kind from kindOf (agentio.HookUnknown for an event without one).
// Pure.
func Existing(b []byte, opt Options, kindOf func(event string) agentio.HookKind) ([]agentio.HookEntry, error) {
	hs, err := Handlers(b, opt)
	if err != nil {
		return nil, err
	}
	var out []agentio.HookEntry
	for _, h := range hs {
		if h.IsCommand {
			out = append(out, agentio.HookEntry{Kind: kindOf(h.Event), Event: h.Event, Command: h.Command})
		}
	}
	return out, nil
}

// AppendEntries appends one matcher group with one command handler per
// entry, in order. Pure.
func AppendEntries(b []byte, entries []agentio.HookEntry, opt Options) ([]byte, error) {
	for _, e := range entries {
		g, err := CommandGroup(e.Command)
		if err != nil {
			return nil, err
		}
		if b, err = AppendGroup(b, e.Event, g, opt); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// CheckPlan refuses a plan of another agent (agentio.ErrForeignHome) or
// one whose File is not the hook file of its home (agentio.ErrInvalidHook),
// so ApplyHooks only ever writes the file PlanHooks read. Pure.
func CheckPlan(p agentio.HookPlan, agent agentio.ID, file string) error {
	if p.Agent != agent {
		return fmt.Errorf("%w: %s hook plan", agentio.ErrForeignHome, p.Agent)
	}
	if p.File != file {
		return fmt.Errorf("%w: plan file %s is not %s's hook file %s", agentio.ErrInvalidHook, p.File, agent, file)
	}
	for _, e := range p.Add {
		if e.Event == "" || e.Command == "" {
			return fmt.Errorf("%w: %s: entry without event or command", agentio.ErrInvalidHook, agent)
		}
	}
	return nil
}

// DropCommands is the RemoveHandlers predicate of a neutral RemoveHooks:
// command hooks whose command match reports true. Pure.
func DropCommands(match func(command string) bool) func(Handler) bool {
	return func(h Handler) bool { return h.IsCommand && match(h.Command) }
}
