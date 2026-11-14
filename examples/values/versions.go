package main

import (
	"fmt"
	"slices"

	"github.com/ubgo/agentio"
)

// snippet:start parse-version
// parseVersions reads the version out of real `--version` output shapes.
// ParseVersion scans for the first dotted number, so product text around
// it does not matter; ok is false when there is no number. Pure.
func parseVersions(p printer) error {
	for _, s := range []string{
		"2.1.325 (Claude Code)",
		"codex-cli 0.162.1",
		"v1.2",
		"1.0.0-beta.2",
		"x86_64 build 7",
		"no version here",
	} {
		v, ok := agentio.ParseVersion(s)
		p.line(fmt.Sprintf("parse %q: ok=%t version=%q major=%d pre=%q", s, ok, v, v.Major, v.Pre))
	}
	// MustVersion is for constants known at compile time; a typo gives the
	// zero (unknown) Version, never a panic.
	p.line(fmt.Sprintf("must: %s typo-is-zero=%t", agentio.MustVersion("2.1.323"), agentio.MustVersion("two").IsZero()))
	return nil
}

// snippet:end parse-version

// snippet:start compare-versions
// compareVersions orders versions semver-style: a release sorts after its
// pre-releases, numeric pre-release parts compare as numbers. The zero
// Version means "unknown", which callers treat as build-and-warn. Pure.
func compareVersions(p printer) error {
	vs := []agentio.Version{
		agentio.MustVersion("1.0.0"),
		agentio.MustVersion("1.0.0-beta.11"),
		agentio.MustVersion("1.0.0-beta.2"),
		agentio.MustVersion("0.162.1"),
		agentio.MustVersion("1.0.0-alpha"),
	}
	slices.SortFunc(vs, agentio.Version.Compare)
	p.line(fmt.Sprintf("sorted: %v", vs))
	installed, minimum := agentio.MustVersion("2.1.325"), agentio.MustVersion("2.1.300")
	p.line(fmt.Sprintf("compare: 2.1.325 vs 2.1.300 = %d (at least minimum: %t)", installed.Compare(minimum), installed.Compare(minimum) >= 0))
	var unknown agentio.Version
	p.line(fmt.Sprintf("unknown: IsZero=%t String=%q", unknown.IsZero(), unknown.String()))
	return nil
}

// snippet:end compare-versions
