// Command quickstart is the first program to read: it builds a Host, creates
// the Claude Code agent on it, reads the install from disk and lists the
// homes. By default every input is a fixture: the user home is the 2.1.325
// fixture home in this repo and PATH points at a fake npm install written to
// a temp dir, so nothing on the real machine is read and no binary runs.
//
//	cd examples && go run ./quickstart          # fixture home (default)
//	cd examples && go run ./quickstart -os      # your real machine (read-only)
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// defaultUserHome is the fixture user home, relative to the examples dir
// (where `go run ./quickstart` runs).
const defaultUserHome = "../claude/testdata/homes/2.1.325/userhome"

// fakeVersion is the version written into the fake npm package.json; Install
// reads it from disk, never by running the binary.
const fakeVersion = "2.1.325"

// Fake npm layout, the shape `npm install -g @anthropic-ai/claude-code`
// leaves: <prefix>/bin/claude -> ../lib/node_modules/@anthropic-ai/claude-code/cli.js.
const (
	npmPackageRel = "lib/node_modules/@anthropic-ai/claude-code"
	npmEntryFile  = "cli.js"
	binDirName    = "bin"
	// shadowDirName holds a second claude later on PATH (a stale copy).
	shadowDirName = "old/bin"
)

// File modes: the fake binary must be executable for a PATH walk to count it.
const (
	dirPerm  os.FileMode = 0o755
	execPerm os.FileMode = 0o755
	filePerm os.FileMode = 0o644
)

// fixtureUser is the OS login name of the fixture Host.
const fixtureUser = "user"

// httpTimeout bounds HTTP calls; this program makes none, but a Host built
// by hand should never carry a client without a timeout.
const httpTimeout = 30 * time.Second

func main() {
	userHome := flag.String("home", defaultUserHome, "fixture user home to read")
	useOS := flag.Bool("os", false, "read the real machine through HostFromOS instead of the fixture")
	flag.Parse()
	var err error
	if *useOS {
		err = runOS(os.Stdout)
	} else {
		err = run(os.Stdout, *userHome)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "quickstart:", err)
		os.Exit(1)
	}
}

// run builds a fixture Host over userHome with a fake npm install on PATH,
// then prints what the library sees. Disk · Read on the fixture, Disk ·
// Write only inside a temp dir it removes.
func run(w io.Writer, userHome string) error {
	abs, err := filepath.Abs(userHome)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "agentio-quickstart-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	// Resolve symlinks (macOS /var -> /private/var) so RealPath, which
	// Install resolves too, still starts with tmp.
	if tmp, err = filepath.EvalSymlinks(tmp); err != nil {
		return err
	}
	binDir, err := fakeNPMInstall(tmp, fakeVersion)
	if err != nil {
		return err
	}
	shadowDir := filepath.Join(tmp, shadowDirName)
	if err := writeExecutable(filepath.Join(shadowDir, claude.Binary)); err != nil {
		return err
	}
	pathList := strings.Join([]string{binDir, shadowDir}, string(os.PathListSeparator))
	h := fixtureHost(abs, pathList)
	p := printer{w: w, repl: strings.NewReplacer(tmp, "$TMP", abs, "~")}
	hostEnv(p, h)
	if err := report(p, h); err != nil {
		return err
	}
	findBinary(p, pathList)
	if err := notInstalled(p, abs); err != nil {
		return err
	}
	if err := describe(p, neutralClaude(h)); err != nil {
		return err
	}
	if err := readSpec(p, neutralClaude(h)); err != nil {
		return err
	}
	return buildCore(p, h)
}

// runOS prints the same report for the real machine. Disk · Read only.
func runOS(w io.Writer) error {
	h, err := osHost()
	if err != nil {
		return err
	}
	return report(printer{w: w, repl: strings.NewReplacer(h.UserHome, "~")}, h)
}

// snippet:start host-from-os
// osHost is the one constructor that reads the real machine: $HOME,
// os.Environ(), the real clock, the keychain, the process table and an
// os/exec runner. Disk · Read (it only resolves the home dir).
func osHost() (agentio.Host, error) {
	h, err := agentio.HostFromOS()
	if err != nil {
		return agentio.Host{}, fmt.Errorf("host: %w", err)
	}
	return h, nil
}

// snippet:end host-from-os

// snippet:start build-host
// fixtureHost builds a Host by hand. Every field is an input the library
// would otherwise take from the machine; a nil Secrets, Processes or Exec
// means "none": keychain reads find nothing, no process is ever listed or
// run. Pure.
func fixtureHost(userHome, pathList string) agentio.Host {
	return agentio.Host{
		UserHome: userHome,
		User:     fixtureUser,
		// Env is the base env every child command starts from. PATH here is
		// what LookPath walks; no shell is ever asked.
		Env:  []string{"PATH=" + pathList, "HOME=" + userHome},
		Now:  func() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) },
		HTTP: &http.Client{Timeout: httpTimeout},
	}
}

// snippet:end build-host

// snippet:start host-getenv
// hostEnv reads the Host's base env, never the process env. Getenv and
// EnvLookup return the LAST entry of a name (what a child process sees)
// and tell "K=" (present, empty) apart from absent. Pure.
func hostEnv(p printer, h agentio.Host) {
	_, hasPath := h.Getenv("PATH")
	p.line(fmt.Sprintf("host: user=%s PATH set=%t", h.User, hasPath))
	env := []string{"MODE=a", "MODE=b", "EMPTY="}
	mode, _ := agentio.EnvLookup(env, "MODE")
	empty, emptySet := agentio.EnvLookup(env, "EMPTY")
	_, missingSet := agentio.EnvLookup(env, "MISSING")
	p.line(fmt.Sprintf("host: MODE=%s EMPTY=%q set=%t MISSING set=%t", mode, empty, emptySet, missingSet))
}

// snippet:end host-getenv

// report creates the Claude agent and prints its install and homes.
func report(p printer, h agentio.Host) error {
	a := newAgent(h)
	if err := readInstall(p, a); err != nil {
		return err
	}
	if err := listHomes(p, a); err != nil {
		return err
	}
	neutralView(p, a)
	return nil
}

// snippet:start create-agent
// newAgent creates Claude Code on h. New is Pure: no I/O and no error, so
// it is safe to call per request.
func newAgent(h agentio.Host) *claude.Agent {
	return claude.New(h)
}

// snippet:end create-agent

// snippet:start read-install
// readInstall reports how claude is installed. Disk · Read: PATH walk,
// symlinks, package.json and ~/.claude.json; it never runs the binary.
func readInstall(p printer, a *claude.Agent) error {
	inst, err := a.Install()
	if errors.Is(err, agentio.ErrNotInstalled) {
		p.line("install: claude is not on PATH")
		return nil
	}
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}
	p.line(fmt.Sprintf("install: path=%s", inst.Path))
	p.line(fmt.Sprintf("install: real=%s", inst.RealPath))
	p.line(fmt.Sprintf("install: method=%s version=%s", inst.Method, inst.Version))
	for _, s := range inst.Shadowed {
		p.line(fmt.Sprintf("install: shadowed=%s", s))
	}
	return nil
}

// snippet:end read-install

// snippet:start list-homes
// listHomes prints every Claude home: ~/.claude first, then each
// ~/.claude-<name> that holds a home marker, then CLAUDE_CONFIG_DIR's.
// Disk · Read.
func listHomes(p printer, a *claude.Agent) error {
	homes, err := a.Homes()
	if err != nil {
		return fmt.Errorf("homes: %w", err)
	}
	for _, h := range homes {
		p.line(fmt.Sprintf("home: name=%s default=%t dir=%s", h.Name(), h.IsDefault(), h.Dir()))
	}
	return nil
}

// snippet:end list-homes

// snippet:start neutral-view
// neutralView prints the vendor-neutral view: the agentio.Agent every
// agent shares, for code that loops over agents. Pure.
func neutralView(p printer, a *claude.Agent) {
	n := a.Neutral()
	p.line(fmt.Sprintf("neutral: id=%s name=%q binary=%s", n.ID(), n.Name(), n.Spec().Binary))
}

// snippet:end neutral-view

// snippet:start find-binary
// findBinary walks a PATH value the way Agent.LookPath does: the first hit
// is what runs, later hits are shadowed copies. Disk · Read.
func findBinary(p printer, pathList string) {
	if first, err := agentio.LookPath(pathList, claude.Binary); err == nil {
		p.line(fmt.Sprintf("lookpath: first=%s", first))
	}
	for i, hit := range agentio.LookPathAll(pathList, claude.Binary) {
		p.line(fmt.Sprintf("lookpath: hit[%d]=%s", i, hit))
	}
}

// snippet:end find-binary

// snippet:start not-installed
// notInstalled shows the answer for a machine without claude: Install and
// LookPath return an error matching agentio.ErrNotInstalled. Disk · Read.
func notInstalled(p printer, userHome string) error {
	a := claude.New(fixtureHost(userHome, ""))
	_, err := a.LookPath()
	if !errors.Is(err, agentio.ErrNotInstalled) {
		return fmt.Errorf("empty PATH: got %v, want ErrNotInstalled", err)
	}
	p.line(fmt.Sprintf("empty PATH: %v", err))
	return nil
}

// snippet:end not-installed

// fakeNPMInstall writes the npm global layout under prefix and returns its
// bin dir. Disk · Write inside prefix only.
func fakeNPMInstall(prefix, version string) (string, error) {
	pkg := filepath.Join(prefix, npmPackageRel)
	if err := os.MkdirAll(pkg, dirPerm); err != nil {
		return "", err
	}
	manifest := `{"name":"@anthropic-ai/claude-code","version":"` + version + `"}`
	if err := os.WriteFile(filepath.Join(pkg, "package.json"), []byte(manifest), filePerm); err != nil {
		return "", err
	}
	entry := filepath.Join(pkg, npmEntryFile)
	if err := writeExecutable(entry); err != nil {
		return "", err
	}
	binDir := filepath.Join(prefix, binDirName)
	if err := os.MkdirAll(binDir, dirPerm); err != nil {
		return "", err
	}
	return binDir, os.Symlink(entry, filepath.Join(binDir, claude.Binary))
}

// writeExecutable writes an empty executable file; it is never run, only
// found by the PATH walk.
func writeExecutable(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return err
	}
	return os.WriteFile(path, nil, execPerm)
}

// printer writes lines with machine-specific path prefixes replaced, so the
// output is the same on every machine.
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
