package gemini

import "github.com/ubgo/agentio"

// Gemini reads turns from a chat file path a caller holds (CapTurnFiles).
// CapTurns stays unsupported: a rewind removes messages that an
// append-only turn callback has already reported (Agent.TurnsAt).
var _ agentio.TurnFileReader = neutral{}

// TurnsAt is Agent.TurnsAt. Disk · Read.
func (n neutral) TurnsAt(path string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	return n.a.TurnsAt(path, from, fn)
}

// TranscriptOwner is Agent.ChatOwner. Disk · Read.
func (n neutral) TranscriptOwner(path string) (agentio.TranscriptRef, error) {
	h, id, err := n.a.ChatOwner(path)
	if err != nil {
		return agentio.TranscriptRef{}, err
	}
	return agentio.TranscriptRef{Agent: ID, Home: h.Neutral(), ConversationID: id}, nil
}
