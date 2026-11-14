// Command launch builds launch commands without running anything: a Target
// from a home, a neutral LaunchRequest turned into an agentio.Command, the
// autonomy guard, and the child environment rules (home variable, secret
// values, scrubbed credentials). secrets.go adds the redacting Token,
// EnvValue and Personal types. Commands are data; this program only prints
// them, redacted.
//
//	cd examples && go run ./launch
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// defaultUserHome is the fixture user home, relative to the examples dir.
const defaultUserHome = "../claude/testdata/homes/2.1.325/userhome"

// Launch inputs used throughout.
const (
	workHomeRel  = ".claude-work"
	projectDir   = "/work/project"
	extraDir     = "/work/shared-lib"
	modelName    = "sonnet"
	promptText   = "fix the failing tests"
	resumeID     = "f85efbdb-fc2f-1e8e-7748-9f915edc8e2d"
	installedVer = "2.1.325"
	// profileEnvName is a variable a caller's profile adds to every launch.
	profileEnvName = "TEAM"
)

// envPrefixes are the variables printed from a built env; the rest of the
// base env is the caller's and not interesting here.
func envPrefixes() []string {
	return []string{claude.ConfigDirEnv, claude.OAuthTokenEnv, claude.APIKeyEnv, profileEnvName}
}

func main() {
	userHome := flag.String("home", defaultUserHome, "fixture user home")
	flag.Parse()
	if err := run(os.Stdout, *userHome); err != nil {
		fmt.Fprintln(os.Stderr, "launch:", err)
		os.Exit(1)
	}
}

// run prints every launch step. Pure apart from resolving userHome.
func run(w io.Writer, userHome string) error {
	abs, err := filepath.Abs(userHome)
	if err != nil {
		return err
	}
	p := printer{w: w, repl: strings.NewReplacer(abs, "~")}
	// The base env already selects a home and carries an API key: the
	// launch rules below decide what the child really sees (the home rule
	// replaces the home variable, and every launch unsets the inherited key
	// so it cannot override the home's login).
	h := agentio.Host{UserHome: abs, User: "user", Env: []string{
		"PATH=/usr/bin",
		claude.ConfigDirEnv + "=" + filepath.Join(abs, ".claude-other"),
		claude.APIKeyEnv + "=sk-from-the-shell",
	}}
	a := claude.New(h)
	t := workTarget(a)
	steps := []func() error{
		func() error { return launchNeutral(p, a.Neutral(), t) },
		func() error { return launchDefault(p, a) },
		func() error { return autonomy(p, a.Neutral(), t) },
		func() error { return childEnv(p, a) },
		func() error { return wrapToken(p) },
		func() error { return tokenLaunch(p, a.Neutral(), t) },
		func() error { return personalValues(p) },
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// snippet:start target
// workTarget builds the Target for the work home: the home plus the inputs
// that come from the caller's profile rather than from one invocation.
// Pure.
func workTarget(a *claude.Agent) agentio.Target {
	t := a.HomeAt(workHomeRel).Target()
	t.ExtraArgs = []string{"--verbose"}
	t.ExtraEnv = []agentio.EnvChange{agentio.EnvSet(profileEnvName, "platform")}
	// The installed version, so the builder can check flags against it; a
	// zero Version means unknown: build and warn.
	t.Version = agentio.MustVersion(installedVer)
	return t
}

// snippet:end target

// snippet:start launch-neutral
// launchNeutral builds a new launch and a resume through the neutral Agent,
// the same call for every agent. Pure: nothing is looked up or run.
func launchNeutral(p printer, a agentio.Agent, t agentio.Target) error {
	cmd, err := a.Launch(t, agentio.LaunchRequest{
		Dir: projectDir, Prompt: promptText, Model: modelName, AddDirs: []string{extraDir},
	})
	if err != nil {
		return fmt.Errorf("launch: %w", err)
	}
	p.line("new: " + cmd.String())
	p.line("new: dir=" + cmd.Dir)
	printEnv(p, "new", cmd)
	p.line(fmt.Sprintf("new: warnings=%d", len(cmd.Warnings)))

	resume, err := a.Launch(t, agentio.LaunchRequest{Dir: projectDir, ResumeID: resumeID})
	if err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	p.line("resume: " + resume.String())
	return nil
}

// snippet:end launch-neutral

// snippet:start launch-default
// launchDefault launches the default home. The home variable is REMOVED
// from the child env, never set to ~/.claude: Claude Code keys its keychain
// entry on the variable, so setting it would make the home look logged
// out. The unknown version (zero) builds anyway and adds a warning. Pure.
func launchDefault(p printer, a *claude.Agent) error {
	cmd, err := a.Neutral().Launch(a.DefaultHome().Target(), agentio.LaunchRequest{})
	if err != nil {
		return err
	}
	p.line("default: " + cmd.String())
	printEnv(p, "default", cmd)
	for _, w := range cmd.Warnings {
		p.line("default: warning " + w.String())
	}
	return nil
}

// snippet:end launch-default

// snippet:start autonomy
// autonomy shows the one policy every builder shares: AutonomyFull (skip
// all permission prompts) needs Target.AllowDangerous. Pure.
func autonomy(p printer, a agentio.Agent, t agentio.Target) error {
	full := agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}
	_, err := a.Launch(t, full)
	p.line(fmt.Sprintf("autonomy full: refused=%t", errors.Is(err, agentio.ErrDangerousNotAllowed)))

	t.AllowDangerous = true
	if err := agentio.CheckAutonomy(t, agentio.AutonomyFull); err != nil {
		return err
	}
	cmd, err := a.Launch(t, full)
	if err != nil {
		return err
	}
	p.line("autonomy full allowed: " + cmd.String())

	cmd, err = a.Launch(t, agentio.LaunchRequest{Autonomy: agentio.AutonomyReadOnly})
	if err != nil {
		return err
	}
	p.line("autonomy read-only: " + cmd.String())
	return nil
}

// snippet:end autonomy

// snippet:start child-env
// childEnv builds an environment by hand with the same pieces the launch
// builders use. ApplyEnv never edits its input; each change first removes
// every entry of its name, then a set appends one. Pure.
func childEnv(p printer, a *claude.Agent) error {
	base := []string{"PATH=/usr/bin", "DEBUG=1", "DEBUG=2", claude.APIKeyEnv + "=sk-old"}
	changes := []agentio.EnvChange{agentio.EnvSet("DEBUG", "0"), agentio.EnvUnset("PATH")}
	// The home rule: unset for the default home, set for any other.
	if hc, ok := agentio.HomeEnvChange(claude.ConfigDirEnv, a.HomeAt(workHomeRel).Neutral()); ok {
		changes = append(changes, hc)
	}
	if hc, ok := agentio.HomeEnvChange(claude.ConfigDirEnv, a.DefaultHome().Neutral()); ok {
		p.line("home rule for default: " + hc.String())
	}
	// A token login scrubs the vendor's other credential variables first.
	changes = append(changes, agentio.ScrubEnv([]string{claude.APIKeyEnv, claude.AuthTokenEnv})...)
	changes = append(changes, agentio.EnvSetSecret(claude.OAuthTokenEnv, agentio.NewToken("sk-ant-oat01-example")))
	for _, c := range changes {
		p.line(fmt.Sprintf("change: op=%s %s", c.Op(), c))
	}
	env := agentio.ApplyEnv(base, changes...)
	p.line(fmt.Sprintf("base unchanged: %t", len(base) == 4))

	cmd := agentio.NewCommand(claude.Binary, []string{"--print"}, base, "", changes...)
	p.line(fmt.Sprintf("secret names: %v", cmd.SecretEnvNames()))
	p.line(fmt.Sprintf("redacted env: %v", cmd.Redacted().Env))
	js, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	p.line("json: " + string(js))
	p.line(fmt.Sprintf("raw env holds the token: %t", strings.Contains(strings.Join(env, " "), "sk-ant-oat01-example")))
	return nil
}

// snippet:end child-env

// printEnv prints the interesting variables of a built command, redacted.
func printEnv(p printer, label string, cmd agentio.Command) {
	for _, kv := range cmd.Redacted().Env {
		name, _, _ := strings.Cut(kv, "=")
		for _, want := range envPrefixes() {
			if name == want {
				p.line(label + ": env " + kv)
			}
		}
	}
	for _, want := range envPrefixes() {
		if _, ok := agentio.EnvLookup(cmd.Env, want); !ok {
			p.line(label + ": env -" + want)
		}
	}
}

// printer writes lines with the fixture path replaced by "~", so the output
// is the same on every machine.
type printer struct {
	w    io.Writer
	repl *strings.Replacer
}

// line prints one line with the home folder shown as ~.
// A write error on the output (a closed pipe) is dropped: the example has
// nothing better to do with it than stop printing.
func (p printer) line(s string) {
	_, _ = fmt.Fprintln(p.w, p.repl.Replace(s))
}
