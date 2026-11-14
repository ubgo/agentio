// Command pi walks the typed pi API on a copy of the 1.5.2 fixture agent
// dir: the install (version from package.json, never running pi), homes,
// sessions with their title, model, turns and token cost, a settings.json
// round trip that keeps unknown keys, the launch commands for each
// autonomy level and a provider token login, the doctor and the
// capability matrix with the reason for every gap. Everything runs on temp
// copies: no pi binary, account, network or quota.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/pi"
)

// defaultFixture is the fixture user home, relative to the examples module.
const defaultFixture = "../pi/testdata/homes/1.5.2/userhome"

// Example constants.
const (
	installedVersion = "1.5.2"
	fixtureUser      = "fixture"
	homeTilde        = "~"
	dirPerm          = 0o700
	exePerm          = 0o755
	filePerm         = 0o600
	// exampleSettings is a settings.json pi could have written: two typed
	// keys and two this library does not model (kept raw).
	exampleSettings = `{"defaultProvider":"anthropic","defaultModel":"claude-sonnet","compaction":{"enabled":false},"hideThinkingBlock":true}`
	// exampleKey is an obvious placeholder, never a real key.
	exampleKey = "sk-ant-" + "api03-EXAMPLE-NOT-A-KEY"
)

func main() {
	fixture := flag.String("fixture", defaultFixture, "fixture user home to copy")
	flag.Parse()
	if err := runMain(os.Stdout, *fixture); err != nil {
		fmt.Fprintln(os.Stderr, "pi:", err)
		os.Exit(1)
	}
}

// runMain copies the fixture into a temp dir and runs every section.
// Disk · Write (temp dir).
func runMain(w io.Writer, fixture string) error {
	tmp, err := os.MkdirTemp("", "agentio-pi-")
	if err != nil {
		return err
	}
	// The temp dir holds only copies; removing it is cleanup.
	defer func() { _ = os.RemoveAll(tmp) }()
	h, err := setup(tmp, fixture)
	if err != nil {
		return err
	}
	return run(context.Background(), w, h)
}

// setup copies fixture to <tmp>/home and lays out an npm install of pi in
// <tmp>/npm (package.json plus the bin symlink npm makes). Symlinks are
// resolved first (macOS /var -> /private/var) so printed paths shorten to
// ~. Disk · Write.
func setup(tmp, fixture string) (agentio.Host, error) {
	tmp, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		return agentio.Host{}, err
	}
	home := filepath.Join(tmp, "home")
	if err := agentio.CopyTree(fixture, home); err != nil {
		return agentio.Host{}, err
	}
	pkg := filepath.Join(home, "npm", "lib", "node_modules", "@earendil-works", "pi-coding-agent")
	cli := filepath.Join(pkg, "dist", "bundle", "cli.js")
	bin := filepath.Join(home, "npm", "bin")
	for _, d := range []string{filepath.Dir(cli), bin} {
		if err := os.MkdirAll(d, dirPerm); err != nil {
			return agentio.Host{}, err
		}
	}
	manifest := `{"name":"` + pi.PackageName + `","version":"` + installedVersion + `","piConfig":{"configDir":".pi"}}`
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(manifest), filePerm); err != nil {
		return agentio.Host{}, err
	}
	if err := os.WriteFile(cli, []byte("#!/usr/bin/env node\n"), exePerm); err != nil {
		return agentio.Host{}, err
	}
	if err := os.Symlink(cli, filepath.Join(bin, pi.Binary)); err != nil {
		return agentio.Host{}, err
	}
	return agentio.Host{
		UserHome: home,
		User:     fixtureUser,
		Env:      []string{"HOME=" + home, "PATH=" + bin, "OPENAI_API_KEY=" + exampleKey},
	}, nil
}

// printer prints lines with the temp user home shortened to ~.
type printer struct {
	w    io.Writer
	home string
}

// line prints one line. Disk · Write (the writer).
func (p printer) line(s string) {
	_, _ = fmt.Fprintln(p.w, strings.ReplaceAll(s, p.home, homeTilde))
}

// run prints every section for pi on h.
func run(ctx context.Context, w io.Writer, h agentio.Host) error {
	a := pi.New(h)
	p := printer{w: w, home: h.UserHome}
	steps := []func() error{
		func() error { return install(p, a) },
		func() error { return homes(p, a) },
		func() error { return sessions(p, a) },
		func() error { return settings(p, a) },
		func() error { return launch(p, a) },
		func() error { doctor(ctx, p, a); return nil },
		func() error { capabilities(p, a); return nil },
	}
	for _, s := range steps {
		if err := s(); err != nil {
			return err
		}
	}
	return nil
}

// snippet:start install
// install reads pi's install from disk: the PATH hit, the file it links
// to, and the version from the npm package.json above it. pi never runs.
func install(p printer, a *pi.Agent) error {
	p.line("== install ==")
	in, err := a.Install()
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("install: %s -> %s method=%s version=%s tested=%s..%s", in.Path, in.RealPath, in.Method, in.Version, pi.TestedMin, pi.TestedMax))
	return nil
}

// snippet:end install

// snippet:start homes
// homes lists the agent dirs: ~/.pi/agent, plus the one PI_CODING_AGENT_DIR
// names (it names the agent dir itself).
func homes(p printer, a *pi.Agent) error {
	p.line("== homes ==")
	hs, err := a.Homes()
	if err != nil {
		return err
	}
	for _, h := range hs {
		p.line(fmt.Sprintf("home: %s default=%t dir=%s", h.Name(), h.IsDefault(), h.Dir()))
	}
	return nil
}

// snippet:end homes

// snippet:start sessions
// sessions lists the home's sessions (header, head and tail windows only),
// then reads one session's turns and token cost.
func sessions(p printer, a *pi.Agent) error {
	p.line("== sessions ==")
	h := a.DefaultHome()
	ss, err := a.Sessions(h, agentio.ConversationQuery{})
	if err != nil {
		return err
	}
	for _, s := range ss {
		p.line(fmt.Sprintf("session %s %q cwd=%s model=%s v%d", s.ID[:8], s.Title(), s.Cwd, s.Model, s.Version))
		p.line(fmt.Sprintf("  first prompt %q, last prompt %q", s.FirstPrompt, s.LastPrompt))
		var user, assistant int
		if _, err := a.Turns(h, s.ID, 0, func(t agentio.Turn) error {
			if t.Role == agentio.RoleUser {
				user++
			} else {
				assistant++
			}
			return nil
		}); err != nil {
			return err
		}
		p.line(fmt.Sprintf("  turns: %d user, %d assistant", user, assistant))
		u, err := a.SessionUsage(h, s.ID)
		if err != nil {
			return err
		}
		p.line(fmt.Sprintf("  usage: %d messages, %d input and %d output tokens, $%.4f", u.Messages, u.Tokens.Input, u.Tokens.Output, u.CostUSD))
	}
	return nil
}

// snippet:end sessions

// snippet:start settings
// settings parses a settings.json: typed keys come out typed, the rest is
// kept raw and written back, so a round trip loses nothing.
func settings(p printer, a *pi.Agent) error {
	p.line("== settings ==")
	h := a.DefaultHome()
	if err := os.WriteFile(a.Paths(h).Settings, []byte(exampleSettings), filePerm); err != nil {
		return err
	}
	s, found, warns, err := a.ReadSettings(h)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("settings: found=%t provider=%s model=%s unknown=%v warnings=%d", found, s.DefaultProvider.Or(""), s.DefaultModel.Or(""), s.Unknown.Keys(), len(warns)))
	out, err := json.Marshal(s)
	if err != nil {
		return err
	}
	p.line("round trip: " + string(out))
	d, err := a.Defaults(h, "")
	if err != nil {
		return err
	}
	p.line("default model: " + d.Model)
	return nil
}

// snippet:end settings

// snippet:start launch
// launch builds commands through the neutral view: a new interactive run
// with a prompt, a read-only headless run, a resume by session file, a
// provider token login, and the levels pi refuses.
func launch(p printer, a *pi.Agent) error {
	p.line("== launch ==")
	n := a.Neutral()
	def := n.DefaultHome().Target()
	cmd, err := n.Launch(def, agentio.LaunchRequest{Prompt: "fix the failing tests"})
	if err != nil {
		return err
	}
	p.line("new: " + cmd.String())
	cmd, err = n.Launch(def, agentio.LaunchRequest{Prompt: "review src/", Autonomy: agentio.AutonomyReadOnly, Headless: true})
	if err != nil {
		return err
	}
	p.line("read-only headless: " + cmd.String())
	ss, err := a.Sessions(a.DefaultHome(), agentio.ConversationQuery{Limit: 1})
	if err != nil || len(ss) == 0 {
		return fmt.Errorf("no session to resume: %w", err)
	}
	cmd, err = n.Launch(def, agentio.LaunchRequest{ResumeID: ss[0].Path, Dir: ss[0].Cwd})
	if err != nil {
		return err
	}
	p.line("resume: " + cmd.String())
	cred := &agentio.TokenCredential{Kind: agentio.TokenAPIKey, Provider: "anthropic", Token: agentio.NewToken(exampleKey)}
	cmd, err = n.Launch(def, agentio.LaunchRequest{Credential: cred})
	if err != nil {
		return err
	}
	red := cmd.Redacted()
	anth, _ := agentio.EnvLookup(red.Env, "ANTHROPIC_API_KEY")
	openai, _ := agentio.EnvLookup(red.Env, "OPENAI_API_KEY")
	p.line(fmt.Sprintf("token login: %s ANTHROPIC_API_KEY=%s OPENAI_API_KEY kept=%t", cmd, anth, openai != ""))
	for _, level := range []agentio.Autonomy{agentio.AutonomyAsk, agentio.AutonomyAcceptEdits} {
		_, err := n.Launch(def, agentio.LaunchRequest{Autonomy: level})
		p.line(fmt.Sprintf("%s: %v", level, err))
	}
	return nil
}

// snippet:end launch

// snippet:start doctor
// doctor runs the local checks: no network, pi never runs.
func doctor(ctx context.Context, p printer, a *pi.Agent) {
	p.line("== doctor ==")
	for _, c := range a.Checks(ctx, a.DefaultHome()) {
		p.line(fmt.Sprintf("%-15s %-5s %s", c.ID, c.Status, c.Message))
	}
}

// snippet:end doctor

// snippet:start capabilities
// capabilities prints what pi supports and why the rest is missing.
func capabilities(p printer, a *pi.Agent) {
	p.line("== capabilities ==")
	n := a.Neutral()
	p.line(fmt.Sprintf("supported: %v", agentio.Supported(n)))
	for _, c := range agentio.CapabilityValues() {
		if reason, ok := agentio.Unsupported(n, c); ok {
			p.line(fmt.Sprintf("lacks %s: %s", c, reason))
		}
	}
}

// snippet:end capabilities
