package claude

import (
	"fmt"

	"github.com/khanakia/agentcli"
)

//go:generate go run ./internal/featuregen

// Feature is a Claude Code flag, file or behaviour whose availability
// depends on the installed version. String() is the vendor spelling. The
// table lives in features.tsv; features_gen.go is generated from it.
type Feature uint16

// featureKind names the type in out-of-range String output.
const featureKind = "Feature"

// String returns the vendor spelling ("--worktree"). cost: Pure.
func (f Feature) String() string {
	names := featureSpellings()
	if int(f) < len(names) {
		return names[f]
	}
	return fmt.Sprintf("%s(%d)", featureKind, uint16(f))
}

// MarshalText encodes the spelling. cost: Pure.
func (f Feature) MarshalText() ([]byte, error) { return []byte(f.String()), nil }

// Features lists every feature in table order. cost: Pure.
func Features() []Feature {
	n := len(featureSpellings())
	out := make([]Feature, 0, n-1)
	for i := 1; i < n; i++ {
		out = append(out, Feature(i))
	}
	return out
}

// FeatureSource names the CHANGELOG line behind a feature's bounds; "" when
// the bounds are unknown. cost: Pure.
func FeatureSource(f Feature) string { return featureSource(f) }

// TestedMin and TestedMax are the Claude Code versions this module was
// tested against: a fixture home for each lives in testdata/homes (SPEC
// §5.8). They move only when a fixture for a new version is added.
const (
	TestedMin = "2.1.323"
	TestedMax = "2.1.323"
)

// Compat parses the tested range. cost: Pure.
func Compat() agentcli.Compat {
	return agentcli.Compat{Min: agentcli.MustVersion(TestedMin), Max: agentcli.MustVersion(TestedMax)}
}

// FeatureUnavailableError reports that the installed version provably
// lacks a feature; it matches agentcli.ErrFeatureUnavailable.
type FeatureUnavailableError struct {
	Feature   Feature
	Installed agentcli.Version
	Since     agentcli.Version
	Until     agentcli.Version
}

// Error names the feature and the versions. cost: Pure.
func (e *FeatureUnavailableError) Error() string {
	switch {
	case !e.Since.IsZero() && e.Installed.Compare(e.Since) < 0:
		return fmt.Sprintf("claude: %s needs Claude Code >= %s (installed %s)", e.Feature, e.Since, e.Installed)
	case !e.Until.IsZero():
		return fmt.Sprintf("claude: %s was removed in Claude Code %s (installed %s)", e.Feature, e.Until, e.Installed)
	}
	return fmt.Sprintf("claude: %s is unavailable in Claude Code %s", e.Feature, e.Installed)
}

// Is matches agentcli.ErrFeatureUnavailable. cost: Pure.
func (e *FeatureUnavailableError) Is(target error) bool {
	return target == agentcli.ErrFeatureUnavailable
}

// checkFeature returns a FeatureUnavailableError when v provably lacks f.
func checkFeature(f Feature, v agentcli.Version) error {
	span := FeatureInfo(f)
	if span.In(v) {
		return nil
	}
	return &FeatureUnavailableError{Feature: f, Installed: v, Since: span.Since, Until: span.Until}
}
