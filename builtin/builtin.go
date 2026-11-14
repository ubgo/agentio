// Package builtin assembles the typed set of agents agentio knows: the
// built-in vendors (Claude Code, Codex, Gemini CLI, pi) plus config-defined agents described
// by agentio.Spec values. It is its own module because it must import every
// vendor module; a Claude-only caller imports claude directly and never pulls
// Codex's dependencies (SPEC §3, §5.7).
package builtin

import (
	"fmt"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
	"github.com/ubgo/agentio/gemini"
	"github.com/ubgo/agentio/pi"
)

// Agents is the typed set of agents on one Host. Vendor fields are concrete
// types so L3 calls are compile-checked; Custom holds config-only agents in
// config order. There is no package-level registry and no default agent.
type Agents struct {
	Claude *claude.Agent
	Codex  *codex.Agent
	Gemini *gemini.Agent
	Pi     *pi.Agent
	Custom []agentio.Agent
}

// New builds the built-in agents on h and one config-only agent per custom
// Spec. A custom Spec whose ID equals a built-in's, or repeats an earlier
// custom ID, is an error, never an override. Pure: nothing is read from disk.
func New(h agentio.Host, custom ...agentio.Spec) (Agents, error) {
	out := Agents{Claude: claude.New(h), Codex: codex.New(h), Gemini: gemini.New(h), Pi: pi.New(h)}
	seen := map[agentio.ID]bool{claude.ID: true, codex.ID: true, gemini.ID: true, pi.ID: true}
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

// All returns the neutral view of every agent: the built-ins (Claude,
// Codex, Gemini CLI, pi), then Custom. Pure.
func (a Agents) All() []agentio.Agent {
	all := make([]agentio.Agent, 0, len(a.Custom)+builtinCount)
	if a.Claude != nil {
		all = append(all, a.Claude.Neutral())
	}
	if a.Codex != nil {
		all = append(all, a.Codex.Neutral())
	}
	if a.Gemini != nil {
		all = append(all, a.Gemini.Neutral())
	}
	if a.Pi != nil {
		all = append(all, a.Pi.Neutral())
	}
	return append(all, a.Custom...)
}

// builtinCount is the number of built-in vendor agents in All.
const builtinCount = 4

// ByID finds an agent by ID; false when no agent has it. Pure.
func (a Agents) ByID(id agentio.ID) (agentio.Agent, bool) {
	for _, ag := range a.All() {
		if ag.ID() == id {
			return ag, true
		}
	}
	return nil, false
}
