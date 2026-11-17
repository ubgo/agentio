package codex_test

import (
	"slices"
	"testing"

	"github.com/ubgo/agentio/codex"
)

// TestSpecNamesNPMPackage: the spec carries the npm package so callers
// can print an install hint without vendor knowledge.
func TestSpecNamesNPMPackage(t *testing.T) {
	if !slices.Contains(codex.Spec().Packages, "@openai/codex") {
		t.Fatalf("Packages = %v", codex.Spec().Packages)
	}
}
