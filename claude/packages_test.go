package claude_test

import (
	"slices"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// TestSpecNamesNPMPackage: the spec carries the npm package so callers
// can print an install hint without vendor knowledge.
func TestSpecNamesNPMPackage(t *testing.T) {
	if !slices.Contains(claude.Spec().Packages, "@anthropic-ai/claude-code") {
		t.Fatalf("Packages = %v", claude.Spec().Packages)
	}
}

// TestCachedUsageNote: Claude declares why a cached reading may be
// missing or old, through the neutral optional interface.
func TestCachedUsageNote(t *testing.T) {
	a := claude.New(agentio.Host{}).Neutral()
	r, ok := agentio.CachedUsageOf(a)
	if !ok {
		t.Fatal("no cached usage reader")
	}
	n, ok := r.(agentio.CachedUsageNoter)
	if !ok || n.CachedUsageNote() == "" {
		t.Fatal("no cached-usage note")
	}
}
