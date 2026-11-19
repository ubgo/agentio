package claude

import (
	"errors"
	"fmt"

	"github.com/ubgo/agentio"
)

// Claude reads turns from a transcript path a caller holds (CapTurnFiles).
var (
	_ agentio.TurnFileReader  = neutral{}
	_ agentio.BadLineReporter = neutral{}
)

// TurnsAt is TurnsFile. Disk · Read.
func (n neutral) TurnsAt(path string, from agentio.Offset, fn func(agentio.Turn) error) (agentio.Offset, error) {
	return TurnsFile(path, from, fn)
}

// TurnsAtReporting is TurnsFileReporting. Disk · Read.
func (n neutral) TurnsAtReporting(path string, from agentio.Offset, fn func(agentio.Turn) error, bad func(agentio.BadLine)) (agentio.Offset, error) {
	return TurnsFileReporting(path, from, fn, bad)
}

// TranscriptOwner is Agent.TranscriptAt with the TurnFileReader error
// contract: a path outside a Claude projects dir is ErrNotFound (it still
// matches ErrForeignHome). Pure.
func (n neutral) TranscriptOwner(path string) (agentio.TranscriptRef, error) {
	h, id, err := n.a.TranscriptAt(path)
	if errors.Is(err, agentio.ErrForeignHome) {
		return agentio.TranscriptRef{}, fmt.Errorf("%w: %w", agentio.ErrNotFound, err)
	}
	if err != nil {
		return agentio.TranscriptRef{}, err
	}
	return agentio.TranscriptRef{Agent: ID, Home: h.Neutral(), ConversationID: id}, nil
}
