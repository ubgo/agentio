package claude_test

import (
	"slices"
	"testing"

	"github.com/ubgo/agentio/claude"
)

// TestSpecNamesNPMPackage: the spec carries the npm package so callers
// can print an install hint without vendor knowledge.
func TestSpecNamesNPMPackage(t *testing.T) {
	if !slices.Contains(claude.Spec().Packages, "@anthropic-ai/claude-code") {
		t.Fatalf("Packages = %v", claude.Spec().Packages)
	}
}
