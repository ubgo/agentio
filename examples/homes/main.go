// Command homes shows how agentio finds and names agent homes (config dirs
// such as ~/.claude and ~/.claude-work): the default home, discovery by
// marker files, the home an environment variable selects, opening a home by
// folder, and the owner check that stops one agent's home reaching another.
// It reads the 2.1.325 fixture user home in this repo; nothing on the real
// machine is touched.
//
//	cd examples && go run ./homes
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
)

// defaultUserHome is the fixture user home, relative to the examples dir.
const defaultUserHome = "../claude/testdata/homes/2.1.325/userhome"

// Folders under the fixture user home.
const (
	workHomeRel   = ".claude-work"
	projectDirRel = "project"
	// ciHomeDir is a folder a caller names directly (CLAUDE_CONFIG_DIR
	// pointing outside the user home); it does not need to exist.
	ciHomeDir = "/srv/agents/.claude-ci"
)

// settingsFileName is a file inside a Claude home, joined with Home.Path.
const settingsFileName = "settings.json"

// toyID is the ID of the made-up agent used to show Core.HomeAt.
const toyID agentio.ID = "toy"

func main() {
	userHome := flag.String("home", defaultUserHome, "fixture user home to read")
	flag.Parse()
	if err := run(os.Stdout, *userHome); err != nil {
		fmt.Fprintln(os.Stderr, "homes:", err)
		os.Exit(1)
	}
}

// run prints every home step against the fixture user home. Disk · Read.
func run(w io.Writer, userHome string) error {
	abs, err := filepath.Abs(userHome)
	if err != nil {
		return err
	}
	p := printer{w: w, repl: strings.NewReplacer(abs, "~")}
	h := agentio.Host{UserHome: abs, User: "user"}
	a := claude.New(h)
	defaultHome(p, a)
	if err := listHomes(p, a.Neutral()); err != nil {
		return err
	}
	// The same agent on a Host whose base env selects the work home.
	h.Env = []string{claude.ConfigDirEnv + "=" + filepath.Join(abs, workHomeRel)}
	homeFromEnv(p, claude.New(h).Neutral())
	if err := isHome(p, a.Neutral(), abs); err != nil {
		return err
	}
	if err := openByFolder(p, a, h); err != nil {
		return err
	}
	return checkOwner(p, a, codex.New(h).Neutral())
}

// snippet:start default-home
// defaultHome prints the default home and the Home accessors. A Home is
// built only by an agent (or NewHome in vendor code), so it always knows
// which agent owns it. Pure.
func defaultHome(p printer, a *claude.Agent) {
	def := a.Neutral().DefaultHome()
	p.line(fmt.Sprintf("default: agent=%s name=%s default=%t", def.Agent(), def.Name(), def.IsDefault()))
	p.line(fmt.Sprintf("default: dir=%s", def.Dir()))
	p.line(fmt.Sprintf("default: path=%s", def.Path(settingsFileName)))
	p.line(fmt.Sprintf("default: name is DefaultHomeName: %t", def.Name() == agentio.DefaultHomeName))
	var zero agentio.Home
	p.line(fmt.Sprintf("zero home: IsZero=%t", zero.IsZero()))
}

// snippet:end default-home

// snippet:start list-homes
// listHomes lists homes on disk through the neutral Agent: the default
// first, then extra homes (~/.claude-*) sorted, then the env-selected one.
// Only folders holding a home marker count. Disk · Read.
func listHomes(p printer, a agentio.Agent) error {
	homes, err := a.Homes()
	if err != nil {
		return fmt.Errorf("homes: %w", err)
	}
	for _, h := range homes {
		p.line(fmt.Sprintf("homes: %s name=%s default=%t", h, h.Name(), h.IsDefault()))
	}
	return nil
}

// snippet:end list-homes

// snippet:start home-from-env
// homeFromEnv prints the home the Host's base env selects through the
// agent's home variable (CLAUDE_CONFIG_DIR). false when it is absent or
// empty. Pure: it reads Host.Env, never os.Getenv.
func homeFromEnv(p printer, a agentio.Agent) {
	h, ok := a.HomeFromEnv()
	p.line(fmt.Sprintf("from env: ok=%t name=%s dir=%s", ok, h.Name(), h.Dir()))
}

// snippet:end home-from-env

// snippet:start is-home
// isHome asks whether a folder is a home of this agent: it must exist and
// hold at least one marker (settings.json, projects/, .credentials.json,
// .claude.json for Claude). A missing folder is (false, nil). Disk · Read.
func isHome(p printer, a agentio.Agent, userHome string) error {
	for _, rel := range []string{workHomeRel, projectDirRel, "missing"} {
		ok, err := a.IsHome(filepath.Join(userHome, rel))
		if err != nil {
			return err
		}
		p.line(fmt.Sprintf("is home: %s=%t", rel, ok))
	}
	return nil
}

// snippet:end is-home

// snippet:start open-by-folder
// openByFolder builds a Home for a folder the caller names. HomeAt is Pure:
// it never checks the disk, so it also opens a home that does not exist
// yet. A relative folder resolves against Host.UserHome; the default folder
// gives back the default home.
func openByFolder(p printer, a *claude.Agent, h agentio.Host) error {
	for _, dir := range []string{workHomeRel, ciHomeDir, ".claude"} {
		home := a.HomeAt(dir)
		p.line(fmt.Sprintf("home at %s: name=%s default=%t dir=%s", dir, home.Name(), home.IsDefault(), home.Dir()))
	}
	// Core.HomeAt does the same for any agent described by a Spec.
	core, err := agentio.NewCore(agentio.Spec{ID: toyID, Name: "Toy", Binary: "toy", HomeEnv: "TOY_HOME", DefaultHome: ".toy"}, h)
	if err != nil {
		return err
	}
	toy := core.HomeAt(".toy-staging")
	p.line(fmt.Sprintf("core home at .toy-staging: %s name=%s", toy, toy.Name()))
	return nil
}

// snippet:end open-by-folder

// snippet:start check-owner
// checkOwner shows the guard every neutral method applies: a home of
// another agent is refused with ErrForeignHome instead of being read as if
// it were this agent's. Pure.
func checkOwner(p printer, a *claude.Agent, other agentio.Agent) error {
	codexHome := other.DefaultHome()
	err := agentio.CheckOwner(claude.ID, codexHome)
	p.line(fmt.Sprintf("check owner: foreign=%t err=%v", errors.Is(err, agentio.ErrForeignHome), err))
	// The vendor conversion applies the same check.
	if _, err := a.HomeOf(codexHome); !errors.Is(err, agentio.ErrForeignHome) {
		return fmt.Errorf("HomeOf(codex home) = %v, want ErrForeignHome", err)
	}
	own := a.Neutral().DefaultHome()
	p.line(fmt.Sprintf("check owner: own home err=%v", agentio.CheckOwner(claude.ID, own)))
	return nil
}

// snippet:end check-owner

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
