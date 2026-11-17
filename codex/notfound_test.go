package codex_test

import (
	"errors"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/codex"
)

// TestConversationNotFoundIsAgentioNotFound: the ConversationLister
// contract (ErrNotFound when absent) holds for Codex.
func TestConversationNotFoundIsAgentioNotFound(t *testing.T) {
	if !errors.Is(codex.ErrConversationNotFound, agentio.ErrNotFound) {
		t.Fatal("ErrConversationNotFound does not wrap agentio.ErrNotFound")
	}
}
