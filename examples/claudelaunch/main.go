// Command claudelaunch builds Claude Code commands and runs the read-only
// health checks: launch and resume commands with the home rule applied,
// the target version read from disk, flags a version lacks, the Runner
// door (reads that run the claude binary) and Doctor.
//
// Nothing is executed. The "installed" claude is an empty file in a temp
// dir laid out like a native install (~/.local/share/claude/versions/<v>),
// so the version is read from the path; the Runner door goes to a canned
// process runner. Homes come from the checked-in 2.1.325 fixture.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// defaultFixture is the fixture user home, relative to the examples module.
const defaultFixture = "../claude/testdata/homes/2.1.325/userhome"

// Fixture facts.
const (
	installedVersion = "2.1.325"
	conversationID   = "3f474440-903e-5410-8bfb-d537b89f9547"
	workHomeName     = ".claude-work"
	projectSubdir    = "project"
	// oldVersion predates --worktree (added in 2.1.49).
	oldVersion = "2.1.40"
	// futureVersion is newer than the tested range.
	futureVersion = "2.1.400"
)

// File modes for the fake install.
const (
	dirMode = 0o755
	exeMode = 0o755
)

// cannedOutput is what the fake process runner answers, keyed by argv line;
// the JSON keeps the recorded shapes with placeholder values.
func cannedOutput() map[string]string {
	return map[string]string{
		"claude --version":                      "2.1.325 (Claude Code)\n",
		"claude auth status --json":             `{"loggedIn": true, "authMethod": "claude.ai", "apiProvider": "firstParty", "subscriptionType": "max", "planTier": "max_20x"}`,
		"claude plugin list --json":             `[{"id": "demo@example-market", "version": "1.2.0", "scope": "user", "enabled": true, "installPath": "/home/user/.claude/plugins/cache/demo", "installedAt": "2026-11-01T10:00:00Z", "lastUpdated": "2026-11-10T10:00:00Z", "projectEnabled": false}]`,
		"claude plugin marketplace list --json": `[{"name": "example-market", "source": "github", "repo": "example/market", "installLocation": "/home/user/.claude/plugins/marketplaces/example-market"}]`,
		"claude agents --json --all":            `[{"id": "ab12", "pid": 4242, "sessionId": "11111111-0000-4000-8000-000000000001", "name": "fix-tests", "cwd": "/work/proj4", "kind": "bg", "state": "blocked", "status": "waiting", "startedAt": 1794597025793}]`,
		"claude auto-mode config":               `{"enabled": true}`,
		"claude auto-mode defaults":             `{"enabled": false}`,
	}
}

// cannedExec is a process runner that answers known argv lines and never
// starts a process.
type cannedExec map[string]string

// errUnexpectedCommand reports an argv line cannedExec has no answer for.
var errUnexpectedCommand = errors.New("unexpected command")

// Run returns the canned stdout. Pure.
func (c cannedExec) Run(_ context.Context, cmd agentio.Command, _ io.Reader) (agentio.ProcessResult, error) {
	out, ok := c[cmd.String()]
	if !ok {
		return agentio.ProcessResult{}, fmt.Errorf("%w: %s", errUnexpectedCommand, cmd.String())
	}
	return agentio.ProcessResult{Stdout: []byte(out)}, nil
}

// fixtureNow is the fixed clock: just after the fixture login expired.
func fixtureNow() time.Time { return time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC) }

func main() {
	fixture := flag.String("fixture", defaultFixture, "Claude fixture user home to read")
	flag.Parse()
	tmp, err := os.MkdirTemp("", "claudelaunch-")
	if err == nil {
		err = run(context.Background(), os.Stdout, *fixture, tmp)
		// Best-effort cleanup of our own temp dir: run's error is the one
		// worth reporting, and a leftover dir under $TMPDIR is harmless.
		_ = os.RemoveAll(tmp)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "claudelaunch:", err)
		os.Exit(1)
	}
}

// fakeInstall lays out tmp like a native install and returns the PATH dir:
// tmp/bin/claude -> tmp/.local/share/claude/versions/<v>.
func fakeInstall(tmp string) (string, error) {
	versions := filepath.Join(tmp, ".local", "share", "claude", "versions")
	bin := filepath.Join(tmp, "bin")
	for _, d := range []string{versions, bin} {
		if err := os.MkdirAll(d, dirMode); err != nil {
			return "", err
		}
	}
	exe := filepath.Join(versions, installedVersion)
	if err := os.WriteFile(exe, nil, exeMode); err != nil {
		return "", err
	}
	return bin, os.Symlink(exe, filepath.Join(bin, claude.Binary))
}

// scrub hides machine paths in printed text.
type scrub struct{ home, tmp string }

// path replaces the user home with ~ and the temp dir with <tmp>.
func (s scrub) path(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, s.home, "~"), s.tmp, "<tmp>")
}

// run builds the commands and runs the health checks on the fixture home
// with a fake install under tmp and a canned process runner: commands are
// printed, never executed.
func run(ctx context.Context, w io.Writer, fixture, tmp string) error {
	abs, err := filepath.Abs(fixture)
	if err != nil {
		return err
	}
	bin, err := fakeInstall(tmp)
	if err != nil {
		return err
	}
	host := agentio.Host{UserHome: abs, Env: []string{"HOME=" + abs, "PATH=" + bin}, Now: fixtureNow, Exec: cannedExec(cannedOutput())}
	a := claude.New(host)
	s := scrub{home: abs, tmp: tmp}
	def, work := a.DefaultHome(), a.HomeAt(workHomeName)
	t, err := targetFor(w, a, def)
	if err != nil {
		return err
	}
	for _, step := range []func() error{
		func() error { return buildLaunch(w, a, t) },
		func() error { return resume(w, a, t, work) },
		func() error { return unavailable(w, a, def) },
		func() error { return runner(ctx, w, a, def) },
		func() error { return doctor(ctx, w, a, def, filepath.Join(abs, projectSubdir), s) },
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// show prints a built command: argv, whether the home variable is in the
// child env, and any build warnings.
func show(w io.Writer, label string, c agentio.Command) {
	_, homeSet := agentio.EnvLookup(c.Env, claude.ConfigDirEnv)
	fmt.Fprintf(w, "%s: %s\n  %s set: %t, warnings: %d\n", label, c, claude.ConfigDirEnv, homeSet, len(c.Warnings))
	for _, warn := range c.Warnings {
		fmt.Fprintf(w, "  warning %s: %s\n", warn.Kind, warn.Detail)
	}
}

// snippet:start target
// targetFor fills Target.Version from the install on disk (the versioned
// path of a native install), never by running claude --version.
func targetFor(w io.Writer, a *claude.Agent, h claude.Home) (agentio.Target, error) {
	t, err := a.TargetFor(h)
	if err != nil {
		return t, err
	}
	fmt.Fprintf(w, "target: home %s, version %s\n", t.Home.Name(), t.Version)
	return t, nil
}

// snippet:end target

// snippet:start launch
// buildLaunch turns LaunchOptions into a Command (data, never run). The
// prompt goes last after "--"; a combination Claude Code rejects is
// ErrInvalidLaunch before anything is built.
func buildLaunch(w io.Writer, a *claude.Agent, t agentio.Target) error {
	cmd, err := a.Launch(t, claude.LaunchOptions{
		Model:          "opus",
		PermissionMode: claude.ModePlan,
		AddDirs:        []string{"/work/shared"},
		Prompt:         "-explain the build",
	})
	if err != nil {
		return err
	}
	show(w, "launch", cmd)
	args, err := claude.LaunchOptions{Print: true, OutputFormat: claude.OutputStreamJSON, Verbose: true, IncludePartialMessages: true}.Args()
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "headless args: %s\n", strings.Join(args, " "))
	if _, err := a.Launch(t, claude.LaunchOptions{OutputFormat: claude.OutputJSON}); errors.Is(err, claude.ErrInvalidLaunch) {
		fmt.Fprintln(w, "invalid:", err)
	}
	show(w, "purge", a.PurgeCommand("/work/old-project", true))
	return nil
}

// snippet:end launch

// snippet:start resume
// resume builds the three ways back into a conversation: by id, the most
// recent one in the folder, and a fork under a new id. Another home gets
// CLAUDE_CONFIG_DIR in the child env; the default home never does.
func resume(w io.Writer, a *claude.Agent, t agentio.Target, work claude.Home) error {
	byID, err := a.Launch(t, claude.LaunchOptions{Resume: conversationID})
	if err != nil {
		return err
	}
	show(w, "resume", byID)
	last, err := a.Launch(t, claude.LaunchOptions{Continue: true})
	if err != nil {
		return err
	}
	show(w, "continue", last)
	wt := work.Target()
	wt.Version = t.Version
	fork, err := a.Launch(wt, claude.LaunchOptions{Resume: conversationID, ForkSession: true, SessionID: "22222222-0000-4000-8000-000000000002"})
	if err != nil {
		return err
	}
	show(w, "fork in work home", fork)
	if _, err := a.Launch(t, claude.LaunchOptions{Resume: conversationID, Continue: true}); errors.Is(err, claude.ErrInvalidLaunch) {
		fmt.Fprintln(w, "invalid:", err)
	}
	return nil
}

// snippet:end resume

// snippet:start unavailable
// unavailable shows the version check every flag goes through: a version
// that provably lacks a flag is a *FeatureUnavailableError; an unknown
// version or one outside the tested range builds with a warning.
func unavailable(w io.Writer, a *claude.Agent, h claude.Home) error {
	worktree := ""
	opts := claude.LaunchOptions{Worktree: &worktree}
	old := h.Target()
	old.Version = agentio.MustVersion(oldVersion)
	_, err := a.Launch(old, opts)
	var fe *claude.FeatureUnavailableError
	if errors.As(err, &fe) {
		fmt.Fprintf(w, "unavailable: %v (feature %s, since %s, matches ErrFeatureUnavailable: %t)\n",
			err, fe.Feature, fe.Since, errors.Is(err, agentio.ErrFeatureUnavailable))
	}
	unknown, err := a.Launch(h.Target(), opts)
	if err != nil {
		return err
	}
	show(w, "unknown version", unknown)
	future := h.Target()
	future.Version = agentio.MustVersion(futureVersion)
	untested, err := a.Launch(future, opts)
	if err != nil {
		return err
	}
	show(w, "untested version", untested)
	return nil
}

// snippet:end unavailable

// snippet:start version
// runnerVersion runs `claude --version` through the Runner door: a Process
// read, labelled "may touch login", so nothing calls it implicitly.
func runnerVersion(ctx context.Context, w io.Writer, r *claude.Runner) error {
	v, err := r.BinaryVersion(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "runner version: %s\n", v)
	return nil
}

// snippet:end version

// snippet:start plugins
// runnerPlugins lists plugins and marketplaces as the claude binary reports
// them (`plugin list --json`, `plugin marketplace list --json`).
func runnerPlugins(ctx context.Context, w io.Writer, r *claude.Runner, h claude.Home) error {
	plugins, err := r.Plugins(ctx, h, false)
	if err != nil {
		return err
	}
	for _, p := range plugins {
		fmt.Fprintf(w, "runner plugin %s v%s (%s), enabled %t, installed %s\n", p.ID, p.Version, p.Scope, p.Enabled, p.InstalledAt.Format("2006-01-02"))
	}
	markets, err := r.Marketplaces(ctx, h)
	if err != nil {
		return err
	}
	for _, m := range markets {
		fmt.Fprintf(w, "runner marketplace %s: %s %s\n", m.Name, m.Source, m.Repo)
	}
	return nil
}

// snippet:end plugins

// snippet:start agents
// runnerAgents lists background agents (`agents --json --all`) and reads
// the auto-mode config and defaults, which stay raw JSON until a sample of
// their shape is recorded.
func runnerAgents(ctx context.Context, w io.Writer, r *claude.Runner, h claude.Home) error {
	bg, err := r.BackgroundAgents(ctx, h, true)
	if err != nil {
		return err
	}
	for _, b := range bg {
		fmt.Fprintf(w, "background agent %s %q: pid %d, state %s, cwd %s\n", b.ID, b.Name, b.PID, b.State, b.Cwd)
	}
	cfg, err := r.AutoModeConfig(ctx, h)
	if err != nil {
		return err
	}
	defs, err := r.AutoModeDefaults(ctx, h)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "auto mode config %s, defaults %s\n", cfg, defs)
	return nil
}

// snippet:end agents

// runner runs the three Runner steps against the canned process runner.
func runner(ctx context.Context, w io.Writer, a *claude.Agent, h claude.Home) error {
	r := a.Runner()
	if err := runnerVersion(ctx, w, r); err != nil {
		return err
	}
	if err := runnerPlugins(ctx, w, r, h); err != nil {
		return err
	}
	return runnerAgents(ctx, w, r, h)
}

// snippet:start doctor
// doctor computes the health checks from disk reads. With a Runner in
// DoctorOptions it also runs `claude auth status`; without one it never
// runs the binary. Messages carry names and counts, never values.
func doctor(ctx context.Context, w io.Writer, a *claude.Agent, h claude.Home, dir string, s scrub) error {
	p, err := a.Project(dir)
	if err != nil {
		return err
	}
	for _, c := range a.Doctor(ctx, h, &p, claude.DoctorOptions{Runner: a.Runner()}) {
		fmt.Fprintf(w, "check %-20s %-5s %s\n", c.ID, c.Status, s.path(c.Message))
	}
	return nil
}

// snippet:end doctor
