// Package builtin assembles the typed set of agents agentio knows: the
// built-in vendors (Claude Code, Codex) plus config-defined agents described
// by agentio.Spec values. It is its own module because it must import every
// vendor module; a Claude-only caller imports claude directly and never pulls
// Codex's dependencies (SPEC §3, §5.7).
package builtin

import (
	"fmt"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
)

// Agents is the typed set of agents on one Host. Vendor fields are concrete
// types so L3 calls are compile-checked; Custom holds config-only agents in
// config order. There is no package-level registry and no default agent.
type Agents struct {
	Claude *claude.Agent
	Codex  *codex.Agent
	Custom []agentio.Agent
}

// New builds the built-in agents on h and one config-only agent per custom
// Spec. A custom Spec whose ID equals a built-in's, or repeats an earlier
// custom ID, is an error, never an override. Pure: nothing is read from disk.
func New(h agentio.Host, custom ...agentio.Spec) (Agents, error) {
	out := Agents{Claude: claude.New(h), Codex: codex.New(h)}
	seen := map[agentio.ID]bool{claude.ID: true, codex.ID: true}
	for _, s := range custom {
		if seen[s.ID] {
			return Agents{}, fmt.Errorf("builtin: agent id %q is already defined: %w", s.ID, agentio.ErrInvalidSpec)
		}
		a, err := agentio.FromSpec(s, h)
		if err != nil {
			return Agents{}, fmt.Errorf("builtin: custom agent %q: %w", s.ID, err)
		}
		seen[s.ID] = true
		out.Custom = append(out.Custom, a)
	}
	return out, nil
}

// All returns the neutral view of every agent: Claude, Codex, then Custom.
// Pure.
func (a Agents) All() []agentio.Agent {
	all := make([]agentio.Agent, 0, len(a.Custom)+2)
	if a.Claude != nil {
		all = append(all, a.Claude.Neutral())
	}
	if a.Codex != nil {
		all = append(all, a.Codex.Neutral())
	}
	return append(all, a.Custom...)
}

// ByID finds an agent by ID; false when no agent has it. Pure.
func (a Agents) ByID(id agentio.ID) (agentio.Agent, bool) {
	for _, ag := range a.All() {
		if ag.ID() == id {
			return ag, true
		}
	}
	return nil, false
}
