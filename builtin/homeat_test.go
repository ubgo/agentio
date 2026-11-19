package builtin

import (
	"path/filepath"
	"testing"

	"github.com/ubgo/agentio/agenttest"
)

// TestNeutralHomeAt: every built-in and config agent maps its default dir
// to its default home and another folder to a non-default home it owns,
// through the neutral Agent alone.
func TestNeutralHomeAt(t *testing.T) {
	h := agenttest.NewHost(t)
	a, err := New(h, sampleSpec("sample"))
	if err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(h.UserHome, "alice-homes", "work")
	for _, ag := range a.All() {
		def := ag.DefaultHome()
		if got := ag.HomeAt(def.Dir()); !got.IsDefault() || got.Dir() != def.Dir() || got.Agent() != ag.ID() {
			t.Errorf("%s: HomeAt(default dir) = %+v", ag.ID(), got)
		}
		got := ag.HomeAt(other)
		if got.IsDefault() || got.Dir() != other || got.Agent() != ag.ID() {
			t.Errorf("%s: HomeAt(%s) = %+v", ag.ID(), other, got)
		}
	}
}
