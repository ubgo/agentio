// Command doors shows the three doors agentio puts in front of anything
// that is not a plain disk read: the Runner (runs the vendor binary), the
// Prober (spends a little quota) and the Writer (changes vendor files, with
// a backup first), plus the HomeLinker write door. The binary runs are
// canned, the probe hits an in-process fake API and every write lands in a
// temporary copy of the fixture home.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
)

// Values used by the steps.
const (
	sampleOAuthToken = "sk-ant-" + "oat01-EXAMPLE-NOT-A-TOKEN"
	newHomeDir       = ".claude-new"
	themeKey         = "theme"
	themeValue       = `"dark"`
)

func main() {
	if err := runMain(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "doors:", err)
		os.Exit(1)
	}
}

// runMain wires the fakes and a temp copy of the fixture, then runs.
func runMain(w io.Writer) error {
	srv := fakeProbeAPI()
	defer srv.Close()
	tmp, err := os.MkdirTemp("", "agentio-doors-")
	if err != nil {
		return err
	}
	// The temp dir holds only copies; removing it is cleanup.
	defer func() { _ = os.RemoveAll(tmp) }()
	exec := &recordingExec{}
	h, err := setup(tmp, exec, srv.Client())
	if err != nil {
		return err
	}
	return run(context.Background(), w, h, exec, srv.URL)
}

// run prints each door step, then the commands that reached Host.Exec.
func run(ctx context.Context, w io.Writer, h agentio.Host, exec *recordingExec, baseURL string) error {
	steps := []struct {
		title string
		fn    func() error
	}{
		{"runner", func() error { return useRunners(ctx, w, h) }},
		{"prober", func() error { return useProber(ctx, w, h, baseURL) }},
		{"writer", func() error { return useWriter(w, h) }},
		{"home linker", func() error { return linkHome(w, h) }},
	}
	for _, s := range steps {
		fmt.Fprintf(w, "== %s ==\n", s.title)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.title, err)
		}
	}
	fmt.Fprintln(w, "== host.exec calls ==")
	for _, c := range exec.Calls() {
		fmt.Fprintln(w, c)
	}
	return nil
}

// snippet:start runner
// useRunners runs `--version` through each vendor Runner and through the
// root Runner of a Spec-built core. Holding a Runner is the opt-in: no read
// method of the library ever runs a binary.
func useRunners(ctx context.Context, w io.Writer, h agentio.Host) error {
	cv, err := claude.New(h).Runner().BinaryVersion(ctx)
	if err != nil {
		return err
	}
	xv, err := codex.New(h).Runner().BinaryVersion(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "claude %s, codex %s\n", cv, xv)

	// The root Runner works for any Spec with VersionArgs.
	core, err := agentio.NewCore(agentio.Spec{ID: "aider", Name: "Aider", Binary: "aider", VersionArgs: []string{"--version"}}, h)
	if err != nil {
		return err
	}
	av, err := core.Runner().Version(ctx)
	if err != nil {
		return err
	}
	cmd, err := core.Command(core.DefaultHome().Target(), "", []string{"--help"}, nil)
	if err != nil {
		return err
	}
	res, err := core.Runner().Run(ctx, cmd, nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "aider %s; `%s` exited %d\n", av, cmd, res.ExitCode)
	return nil
}

// snippet:end runner

// snippet:start prober
// useProber reads a token's plan usage from one minimal API request. It
// spends quota, so it lives behind its own door; call it once on demand.
func useProber(ctx context.Context, w io.Writer, h agentio.Host, baseURL string) error {
	p := claude.New(h).WithBaseURL(baseURL).Prober()
	cred := agentio.TokenCredential{Kind: agentio.TokenOAuthToken, Token: agentio.NewToken(sampleOAuthToken)}
	u, err := p.TokenUsage(ctx, cred)
	if err != nil {
		return err
	}
	for _, win := range u.Windows {
		fmt.Fprintf(w, "%s %.0f%% resets %s\n", win.Label, win.Percent, win.ResetsAt.Format("15:04 MST"))
	}
	return nil
}

// snippet:end prober

// snippet:start writer
// useWriter changes one top-level key of the default home's settings.json.
// The Writer backs the file up first and returns where the copy went; other
// keys keep their values (the file is rewritten indented). Codex's Writer exists but has no methods yet:
// Codex config changes are vendor commands.
func useWriter(w io.Writer, h agentio.Host) error {
	c := claude.New(h)
	b, err := c.Writer().SetUserSetting(c.DefaultHome(), claude.SettingsPatch{
		Set: map[string]agentio.Raw{themeKey: agentio.Raw(themeValue)},
	})
	if err != nil {
		return err
	}
	for _, e := range b.Entries {
		fmt.Fprintf(w, "backed up %s\n       to %s\n", tidy(h, e.Original), tidy(h, e.Copy))
	}
	s, err := c.UserSettings(c.DefaultHome())
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "theme now %q\n", s.Settings.Theme)
	_ = codex.New(h).Writer() // no methods yet
	return nil
}

// snippet:end writer

// snippet:start linker
// linkHome plans and makes the symlinks that let a new home share the
// default home's instructions, skills and settings. Planning is a read;
// applying only creates symlinks and never replaces an existing file.
func linkHome(w io.Writer, h agentio.Host) error {
	c := claude.New(h)
	linker, ok := agentio.HomeLinkerOf(c.Neutral())
	if !ok {
		return fmt.Errorf("claude has no home linker")
	}
	def := c.DefaultHome().Neutral()
	fresh := c.HomeAt(filepath.Join(h.UserHome, newHomeDir)).Neutral()
	links, err := linker.PlanHomeLinks(def, fresh)
	if err != nil {
		return err
	}
	if err := linker.ApplyHomeLinks(links); err != nil {
		return err
	}
	for _, l := range links {
		fmt.Fprintf(w, "%s -> %s (existed %t)\n", tidy(h, l.To), tidy(h, l.From), l.Exists)
	}
	again, err := linker.PlanHomeLinks(def, fresh)
	if err != nil {
		return err
	}
	for _, l := range again {
		fmt.Fprintf(w, "replan %s exists=%t\n", l.Name, l.Exists)
	}
	return nil
}

// snippet:end linker
