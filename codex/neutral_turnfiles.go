package codex

import "github.com/ubgo/agentio"

// Codex reads turns from a rollout path a caller holds (CapTurnFiles).
var _ agentio.TurnFileReader = neutral{}

// TurnsAt is Agent.TurnsAt. Disk · Read.
func (n neutral) TurnsAt(path string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	return n.a.TurnsAt(path, from, fn)
}

// TranscriptOwner is Agent.RolloutOwner. Disk · Read.
func (n neutral) TranscriptOwner(path string) (agentio.TranscriptRef, error) {
	h, id, err := n.a.RolloutOwner(path)
	if err != nil {
		return agentio.TranscriptRef{}, err
	}
	return agentio.TranscriptRef{Agent: ID, Home: h.Neutral(), ConversationID: id}, nil
}
