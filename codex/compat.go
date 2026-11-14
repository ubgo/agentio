package codex

import (
	"fmt"

	"github.com/ubgo/agentio"
)

//go:generate go run ./internal/featuregen/gen

// Tested Codex range: moves only when a fixture home for that version is
// added to testdata (SPEC §5.8). One fixture home today (0.162.1).
const (
	TestedMin = "0.162.1"
	TestedMax = "0.162.1"
)

// Compat is the tested range as versions; a test proves both parse. Pure.
func Compat() agentio.Compat {
	return agentio.Compat{Min: agentio.MustVersion(TestedMin), Max: agentio.MustVersion(TestedMax)}
}

// Feature is a Codex flag, subcommand or file whose availability depends
// on the installed version. Values and spellings are generated from
// features.tsv (features_gen.go).
type Feature uint16

// MarshalText writes the vendor spelling. Pure.
func (f Feature) MarshalText() ([]byte, error) { return []byte(f.String()), nil }

// FeatureUnavailableError says the installed Codex provably lacks a
// feature; it matches errors.Is(err, agentio.ErrFeatureUnavailable).
type FeatureUnavailableError struct {
	Feature      Feature
	Installed    agentio.Version
	Since, Until agentio.Version
}

// Error names the feature, the installed version and the bound. Pure.
func (e *FeatureUnavailableError) Error() string {
	switch {
	case !e.Since.IsZero() && e.Installed.Compare(e.Since) < 0:
		return fmt.Sprintf("codex %s: %s needs codex >= %s", e.Installed, e.Feature, e.Since)
	case !e.Until.IsZero():
		return fmt.Sprintf("codex %s: %s was removed in codex %s", e.Installed, e.Feature, e.Until)
	}
	return fmt.Sprintf("codex %s: %s is unavailable", e.Installed, e.Feature)
}

// Is matches agentio.ErrFeatureUnavailable. Pure.
func (e *FeatureUnavailableError) Is(target error) bool {
	return target == agentio.ErrFeatureUnavailable
}

// FeatureSource names the release note behind f's bounds ("" when the
// bounds are unknown). Pure.
func FeatureSource(f Feature) string { return featureSource(f) }

// checkFeature returns a FeatureUnavailableError when t.Version provably
// lacks f; unknown versions pass (the builder adds a warning once).
func checkFeature(t agentio.Target, f Feature) error {
	span := FeatureInfo(f)
	if span.In(t.Version) {
		return nil
	}
	return &FeatureUnavailableError{Feature: f, Installed: t.Version, Since: span.Since, Until: span.Until}
}
