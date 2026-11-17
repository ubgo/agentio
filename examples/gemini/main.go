// Command gemini walks the disk-only API of the gemini module on a temp
// copy of the checked-in 0.68.0 fixture home: homes (GEMINI_CLI_HOME
// names the dir that contains .gemini), the install read from package
// metadata, settings with every key kept, conversations per project,
// instruction files, MCP servers, skills and extensions, the login, the
// launch commands with their approval modes, and the capability matrix.
// Nothing reads the real machine, runs a binary or touches the network.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/gemini"
)

func main() {
	if err := runMain(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "gemini:", err)
		os.Exit(1)
	}
}

// runMain builds the fake machine in a temp dir and runs every step.
// Disk · Write (temp dir), then Disk · Read.
func runMain(w io.Writer) error {
	tmp, err := os.MkdirTemp("", "agentio-gemini-")
	if err != nil {
		return err
	}
	// The temp dir holds only copies; removing it is cleanup.
	defer func() { _ = os.RemoveAll(tmp) }()
	if tmp, err = filepath.EvalSymlinks(tmp); err != nil {
		return err
	}
	m, err := setup(tmp)
	if err != nil {
		return err
	}
	return run(context.Background(), w, m)
}

// run prints each step; every step is one guide section.
func run(ctx context.Context, w io.Writer, m machine) error {
	a := gemini.New(m.host)
	p := func(line string) { _, _ = fmt.Fprintln(w, m.tidy(line)) }
	for _, step := range []struct {
		title string
		fn    func() error
	}{
		{"homes", func() error { return homes(p, a, m) }},
		{"install", func() error { return install(p, a) }},
		{"settings", func() error { return settings(p, a, m) }},
		{"conversations", func() error { return conversations(ctx, p, a, m) }},
		{"instructions and mcp", func() error { return instructionsAndMCP(p, a, m) }},
		{"login", func() error { return login(p, a) }},
		{"launch", func() error { return launch(p, a) }},
		{"capabilities", func() error { capabilities(p, a); return nil }},
	} {
		p(fmt.Sprintf("== %s", step.title))
		if err := step.fn(); err != nil {
			return fmt.Errorf("%s: %w", step.title, err)
		}
	}
	return nil
}

// printer writes one output line (machine paths tidied).
type printer func(line string)

// snippet:start homes
// homes shows the default home and a second one selected through
// GEMINI_CLI_HOME, which names the dir that CONTAINS .gemini.
func homes(p printer, a *gemini.Agent, m machine) error {
	def := a.DefaultHome()
	p(fmt.Sprintf("default: %s name=%s", def.Dir(), def.Name()))
	work := a.HomeAt(filepath.Join(m.host.UserHome, "gemini-work", ".gemini"))
	cmd, err := a.Launch(work.Target(), agentio.LaunchRequest{})
	if err != nil {
		return err
	}
	parent, _ := agentio.EnvLookup(cmd.Env, gemini.HomeEnv)
	p(fmt.Sprintf("work: %s name=%s launches with %s=%s", work.Dir(), work.Name(), gemini.HomeEnv, parent))
	return nil
}

// snippet:end homes

// snippet:start install
// install reads the version from the npm package's package.json next to
// the resolved binary: no process runs.
func install(p printer, a *gemini.Agent) error {
	in, err := a.Install()
	if err != nil {
		return err
	}
	p(fmt.Sprintf("install: %s method=%s version=%s", filepath.Base(in.Path), in.Method, in.Version))
	p(fmt.Sprintf("tested range: %s..%s", gemini.TestedMin, gemini.TestedMax))
	return nil
}

// snippet:end install

// snippet:start settings
// settings lists the layers (highest first) and the merged values; keys
// agentio does not model are kept, so nothing is lost on a round trip.
func settings(p printer, a *gemini.Agent, m machine) error {
	h := a.DefaultHome()
	layers, err := a.Settings(h, m.project)
	if err != nil {
		return err
	}
	for _, l := range layers {
		p(fmt.Sprintf("layer %-8s exists=%-5t %s", l.Scope, l.Exists, l.Path))
	}
	user := layers[2].Settings
	p(fmt.Sprintf("user auth=%s approval=%s unknown keys=%v", user.Security.Auth.SelectedType, user.General.DefaultApprovalMode, user.Unknown.Keys()))
	e, err := a.EffectiveSettings(h, m.project)
	if err != nil {
		return err
	}
	d, err := a.Defaults(h, m.project)
	if err != nil {
		return err
	}
	p(fmt.Sprintf("effective model=%s (workspace wins) approval=%s -> autonomy %s", e.Model, e.ApprovalMode, d.Autonomy))
	return nil
}

// snippet:end settings

// snippet:start conversations
// conversations lists resumable chats, newest first, with the id
// `gemini --resume` takes; resume runs in the chat's project dir.
func conversations(ctx context.Context, p printer, a *gemini.Agent, m machine) error {
	h := a.DefaultHome()
	slug, _, err := a.ProjectSlug(h, m.project)
	if err != nil {
		return err
	}
	p(fmt.Sprintf("project slug=%s (Slugify of the base name: %s)", slug, gemini.Slugify("My Project")))
	ss, err := a.Sessions(ctx, h, m.project)
	if err != nil {
		return err
	}
	for _, s := range ss {
		p(fmt.Sprintf("  %s  %-36q legacy=%-5t updated %s", s.ID[:8], s.Title, s.Legacy, s.Updated.UTC().Format("2006-01-02T15:04Z")))
	}
	cmd, err := a.Launch(h.Target(), agentio.LaunchRequest{ResumeID: ss[0].ID, Dir: m.project})
	if err != nil {
		return err
	}
	p(fmt.Sprintf("resume: %s (in %s)", cmd, cmd.Dir))
	return nil
}

// snippet:end conversations

// snippet:start instructions
// instructionsAndMCP lists the GEMINI.md files loaded in a subdir (root
// first) and the MCP servers that apply, env names only.
func instructionsAndMCP(p printer, a *gemini.Agent, m machine) error {
	h := a.DefaultHome()
	files, err := a.InstructionFiles(h, filepath.Join(m.project, "sub"))
	if err != nil {
		return err
	}
	for _, f := range files {
		p(fmt.Sprintf("instructions %-7s %s", f.Scope, f.Path))
	}
	servers, err := a.MCPServers(h, m.project)
	if err != nil {
		return err
	}
	for _, s := range servers {
		n := s.Neutral()
		enabled := "-"
		if n.Enabled != nil {
			enabled = fmt.Sprint(*n.Enabled)
		}
		p(fmt.Sprintf("mcp %-15s %-5s scope=%-7s enabled=%-5s env=%v", n.Name, n.Transport, n.Scope, enabled, n.EnvNames))
	}
	skills, err := a.Skills(h, m.project)
	if err != nil {
		return err
	}
	for _, s := range skills {
		line := fmt.Sprintf("skill %-10s scope=%-7s enabled=%t", s.Name, s.Scope, s.Enabled)
		if s.Extension != "" {
			line += " extension=" + s.Extension
		}
		p(line)
	}
	return nil
}

// snippet:end instructions

// snippet:start login
// login reads the home's login from disk: the auth type, the cached
// Google login (email redacted) and its expiry. No token is printed.
func login(p printer, a *gemini.Agent) error {
	id, err := a.Identity(a.DefaultHome())
	if err != nil {
		return err
	}
	p(fmt.Sprintf("auth=%s logged-in=%t credential=%s email=%s expires=%s", id.AuthType, id.LoggedIn, id.Credential, id.Email, id.ExpiresAt.UTC().Format("2006-01-02")))
	return nil
}

// snippet:end login

// snippet:start launch
// launch builds commands; nothing is run. Autonomy maps onto
// --approval-mode, Headless onto --prompt plus stream-json, and a token
// login onto GEMINI_API_KEY.
func launch(p printer, a *gemini.Agent) error {
	t := a.DefaultHome().Target()
	t.AllowDangerous = true
	for _, r := range []agentio.LaunchRequest{
		{Prompt: "explain the parser"},
		{Autonomy: agentio.AutonomyReadOnly, Model: "gemini-2.5-flash"},
		{Autonomy: agentio.AutonomyFull, Headless: true, Prompt: "fix the failing tests", AddDirs: []string{"../lib"}},
	} {
		cmd, err := a.Launch(t, r)
		if err != nil {
			return err
		}
		p(fmt.Sprintf("launch: %s", cmd))
	}
	cmd, err := a.Launch(t, agentio.LaunchRequest{Credential: &agentio.TokenCredential{Kind: agentio.TokenAPIKey, Token: agentio.NewToken("example-key")}})
	if err != nil {
		return err
	}
	key, _ := agentio.EnvLookup(cmd.Env, gemini.APIKeyEnv)
	p(fmt.Sprintf("token launch sets %s (value redacted in output: %t)", gemini.APIKeyEnv, !strings.Contains(cmd.String(), key)))
	if _, err := a.Launch(a.DefaultHome().Target(), agentio.LaunchRequest{Autonomy: agentio.AutonomyFull}); err != nil {
		p(fmt.Sprintf("full without AllowDangerous: %v", err))
	}
	return nil
}

// snippet:end launch

// capabilities prints the neutral capability matrix with each reason.
func capabilities(p printer, a *gemini.Agent) {
	n := a.Neutral()
	p(fmt.Sprintf("supported: %v", agentio.Supported(n)))
	for _, c := range []agentio.Capability{agentio.CapUsage, agentio.CapLoginCommander, agentio.CapRunEvents} {
		r, _ := agentio.Unsupported(n, c)
		p(fmt.Sprintf("%s: %s", c, r))
	}
}
