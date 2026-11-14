// Command compat shows the version side of agentio: the vendor version
// range each module was tested against (Compat, TestedMin, TestedMax),
// where an installed version falls in it (Compat.Check, Compat.Warning),
// the per-feature version table generated from features.tsv (Features,
// FeatureValues, FeatureInfo, FeatureSpan, FeatureSource), and the typed
// FeatureUnavailableError a launch builder returns when the installed
// version provably lacks a flag.
//
// Cost: Pure. Everything here is computed from tables compiled into the
// vendor modules and a Host value built in memory; nothing is read from
// disk, no binary is run and no network is used.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
)

// Example installed versions: one inside each tested range, one newer, one
// older, and one too old for the gated flag the example asks for.
const (
	claudeOld      = "2.1.40"
	listFrom       = "2.1.0"
	claudeNewer    = "2.1.400"
	codexOld       = "0.150.0"
	exampleUser    = "user"
	exampleHome    = "/home/user"
	examplePath    = "/nonexistent-bin"
	envHomePrefix  = "HOME="
	envPathPrefix  = "PATH="
	unknownVersion = ""
)

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "compat:", err)
		os.Exit(1)
	}
}

// run prints every section; it touches nothing outside the process.
func run(w io.Writer) error {
	printRanges(w)
	printChecks(w)
	printFeatures(w)
	printSpans(w)
	return launchGates(w)
}

// snippet:start ranges
// printRanges prints the tested range of each vendor module. TestedMin and
// TestedMax are string constants; Compat() parses them into versions.
func printRanges(w io.Writer) {
	fmt.Fprintf(w, "claude tested %s..%s\n", claude.TestedMin, claude.TestedMax)
	fmt.Fprintf(w, "codex  tested %s..%s\n", codex.TestedMin, codex.TestedMax)
	c := claude.Compat()
	fmt.Fprintf(w, "claude.Compat() = {Min: %s, Max: %s}\n", c.Min, c.Max)
}

// snippet:end ranges

// snippet:start check
// printChecks places a few installed versions against the Claude range.
// Outside the range is a warning, never a refusal; an unknown (zero)
// version is CompatUnknown.
func printChecks(w io.Writer) {
	c := claude.Compat()
	for _, s := range []string{claude.TestedMax, claudeNewer, claudeOld, unknownVersion} {
		// ok is ignored on purpose: unknownVersion fails to parse and
		// yields the zero Version, which is exactly the case shown.
		v, _ := agentio.ParseVersion(s)
		status := c.Check(v)
		line := fmt.Sprintf("check %-9q -> %s", s, status)
		if warn, ok := c.Warning(v); ok {
			line += fmt.Sprintf(" (warning %s)", warn.Kind)
		}
		fmt.Fprintln(w, line)
	}
}

// snippet:end check

// snippet:start features
// printFeatures counts the rows of both feature tables and prints the
// Claude rows gated since listFrom. A row with no known bound is a flag the
// builder emits but cannot gate.
func printFeatures(w io.Writer) {
	gated, total := 0, 0
	for _, f := range claude.Features() {
		total++
		span := claude.FeatureInfo(f)
		if !span.Known() {
			continue
		}
		gated++
		if span.Since.Compare(agentio.MustVersion(listFrom)) >= 0 {
			fmt.Fprintf(w, "claude %-28s since %-8s %s\n", f, span.Since, claude.FeatureSource(f))
		}
	}
	fmt.Fprintf(w, "claude features: %d rows, %d with a known version\n", total, gated)
	gated, total = 0, 0
	for _, f := range codex.FeatureValues() {
		total++
		if codex.FeatureInfo(f).Known() {
			gated++
		}
	}
	fmt.Fprintf(w, "codex features: %d rows, %d with a known version\n", total, gated)
}

// snippet:end features

// snippet:start spans
// printSpans asks one feature's span about three versions. In is true
// unless the version is provably outside the span; an unknown version is
// always in (build and warn, never refuse on a guess).
func printSpans(w io.Writer) {
	span := claude.FeatureInfo(claude.FeatureFlagWorktree)
	for _, s := range []string{claudeOld, claude.TestedMax, unknownVersion} {
		v, _ := agentio.ParseVersion(s) // zero Version for unknownVersion, on purpose
		fmt.Fprintf(w, "%s in %-9q: %t\n", claude.FeatureFlagWorktree, s, span.In(v))
	}
}

// snippet:end spans

// exampleHost is an in-memory Host: launch builders read only Env (for the
// PATH lookup and the home variable rule), so no fixture is needed.
func exampleHost() agentio.Host {
	return agentio.Host{
		UserHome: exampleHome,
		User:     exampleUser,
		Env:      []string{envHomePrefix + exampleHome, envPathPrefix + examplePath},
	}
}

// snippet:start gates
// launchGates builds a launch that asks for a version-gated flag with an
// old Target.Version. The builder refuses with a typed error that matches
// agentio.ErrFeatureUnavailable; with a newer, untested version it builds
// the command and attaches a warning instead.
func launchGates(w io.Writer) error {
	h := exampleHost()

	ca := claude.New(h)
	t := ca.DefaultHome().Target()
	t.Version = agentio.MustVersion(claudeOld)
	worktree := ""
	_, err := ca.Launch(t, claude.LaunchOptions{Worktree: &worktree})
	var cerr *claude.FeatureUnavailableError
	if !errors.As(err, &cerr) {
		return fmt.Errorf("claude launch on %s: want FeatureUnavailableError, got %v", claudeOld, err)
	}
	fmt.Fprintf(w, "claude launch: %v\n", err)
	fmt.Fprintf(w, "  feature=%s installed=%s since=%s is ErrFeatureUnavailable=%t\n",
		cerr.Feature, cerr.Installed, cerr.Since, errors.Is(err, agentio.ErrFeatureUnavailable))

	t.Version = agentio.MustVersion(claudeNewer)
	cmd, err := ca.Launch(t, claude.LaunchOptions{Worktree: &worktree})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "claude %s: built, untested warning=%t\n", claudeNewer, cmd.Warnings.Has(agentio.WarnUntestedVersion))

	xa := codex.New(h)
	xt := xa.DefaultHome().Target()
	xt.Version = agentio.MustVersion(codexOld)
	_, err = xa.New(xt, codex.LaunchOptions{NoDaemon: true})
	var xerr *codex.FeatureUnavailableError
	if !errors.As(err, &xerr) {
		return fmt.Errorf("codex launch on %s: want FeatureUnavailableError, got %v", codexOld, err)
	}
	fmt.Fprintf(w, "codex launch: %v\n", err)
	return nil
}

// snippet:end gates
