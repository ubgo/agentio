package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
)

// Fixture locations inside the repository. The example never reads the real
// machine: it copies the recorded Claude Code 2.1.325 user home and the
// recorded Codex 0.162.1 home (as ~/.codex) into a temporary user home. The
// copy matters: instruction-file discovery walks up from the project folder,
// and inside the repository it would find the repository's own files.
const (
	claudeFixture = "claude/testdata/homes/2.1.325/userhome"
	codexFixture  = "codex/testdata/homes/0.162.1"
	codexHomeDir  = ".codex"
	userHomeDir   = "home"
	installDir    = "install"
	fixtureUser   = "user"
)

// Fake install layout: a native Claude Code version file and an npm Codex
// package, each reached through a symlink in bin/, the way the real
// installers lay them out.
const (
	claudeVersion  = "2.1.325"
	codexVersion   = "0.162.1"
	claudeNative   = ".local/share/claude/versions/" + claudeVersion
	codexPackage   = "lib/node_modules/@openai/codex"
	codexEntry     = codexPackage + "/bin/codex.js"
	binDir         = "bin"
	execPerm       = 0o755
	dirPerm        = 0o755
	filePerm       = 0o644
	versionFlag    = "--version"
	claudeBinary   = "claude"
	codexBinary    = "codex"
	codexPkgJSON   = `{"name":"@openai/codex","version":"` + codexVersion + `"}`
	fakeScriptBody = "#!/bin/sh\nexit 0\n"
)

// errNoExec is returned for any program the canned runner does not know.
var errNoExec = errors.New("example: this host runs no programs except the canned --version answers")

// fixtureNow is the fixed clock: every reading is stable across runs.
func fixtureNow() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

// repoRoot walks up from the working directory to the folder holding the
// fixtures, so `go run ./accessors` (from examples/) and `go test` (from
// examples/accessors/) both find them. Disk · Read.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, claudeFixture)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("example: %s not found above the working directory", claudeFixture)
		}
		dir = parent
	}
}

// fakeInstall lays out a fake native Claude Code and npm Codex under dir and
// returns the bin directory to put on PATH. The files are inert scripts; the
// example never runs them (Host.Exec is canned). Disk · Write.
func fakeInstall(dir string) (string, error) {
	bin := filepath.Join(dir, binDir)
	files := []struct {
		rel, body string
		perm      os.FileMode
	}{
		{claudeNative, fakeScriptBody, execPerm},
		{codexEntry, fakeScriptBody, execPerm},
		{filepath.Join(codexPackage, "package.json"), codexPkgJSON, filePerm},
	}
	for _, f := range files {
		p := filepath.Join(dir, f.rel)
		if err := os.MkdirAll(filepath.Dir(p), dirPerm); err != nil {
			return "", err
		}
		if err := os.WriteFile(p, []byte(f.body), f.perm); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(bin, dirPerm); err != nil {
		return "", err
	}
	links := map[string]string{claudeBinary: claudeNative, codexBinary: codexEntry}
	for name, target := range links {
		if err := os.Symlink(filepath.Join(dir, target), filepath.Join(bin, name)); err != nil {
			return "", err
		}
	}
	return bin, nil
}

// copyTree copies the regular files and directories of src into dst. The
// fixtures hold no symlinks. Disk · Write.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, dirPerm)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, filePerm)
	})
}

// setup builds the example machine under tmp: tmp/home is the user home
// (Claude fixture plus ~/.codex), tmp/install the fake install. Disk · Write.
func setup(tmp string) (agentio.Host, error) {
	root, err := repoRoot()
	if err != nil {
		return agentio.Host{}, err
	}
	// Resolve symlinks (macOS /var -> /private/var) so the install's
	// RealPath, which the library resolves, still starts with the fake
	// prefix and the home, and prints as /opt/fake and ~.
	if tmp, err = filepath.EvalSymlinks(tmp); err != nil {
		return agentio.Host{}, err
	}
	home := filepath.Join(tmp, userHomeDir)
	if err := copyTree(filepath.Join(root, claudeFixture), home); err != nil {
		return agentio.Host{}, err
	}
	if err := copyTree(filepath.Join(root, codexFixture), filepath.Join(home, codexHomeDir)); err != nil {
		return agentio.Host{}, err
	}
	bin, err := fakeInstall(filepath.Join(tmp, installDir))
	if err != nil {
		return agentio.Host{}, err
	}
	return fixtureHost(home, bin), nil
}

// fixtureHost is the Host every reading goes through: the copied homes, a
// PATH holding only the fake install, a fixed clock, no keychain entries,
// an empty process table, no HTTP client and a runner that only answers
// `--version`. Pure.
func fixtureHost(home, bin string) agentio.Host {
	return agentio.Host{
		UserHome:  home,
		User:      fixtureUser,
		Env:       []string{"HOME=" + home, "PATH=" + bin},
		Now:       fixtureNow,
		Secrets:   noSecrets{},
		Processes: staticProcesses{},
		Exec:      versionExec{},
	}
}

// tidy replaces temporary locations in s so printed output carries no
// machine-specific path: the user home becomes ~ and the fake install
// /opt/fake. Pure.
func tidy(h agentio.Host, s string) string {
	pairs := []string{h.UserHome, "~"}
	if bin, ok := h.Getenv("PATH"); ok {
		pairs = append([]string{filepath.Dir(bin), "/opt/fake"}, pairs...)
	}
	return strings.NewReplacer(pairs...).Replace(s)
}

// noSecrets is a keychain with no entries.
type noSecrets struct{}

// Get finds nothing. Pure.
func (noSecrets) Get(context.Context, string, string) (agentio.Token, bool, error) {
	return agentio.Token{}, false, nil
}

// staticProcesses is an empty process table.
type staticProcesses struct{}

// Processes returns no rows. Pure.
func (staticProcesses) Processes(context.Context) ([]agentio.Process, error) { return nil, nil }

// Cwd knows no process. Pure.
func (staticProcesses) Cwd(context.Context, int) (string, error) {
	return "", errNoExec
}

// versionExec answers `<binary> --version` with the fake install's version
// and refuses everything else, so a Runner call is visible but harmless.
type versionExec struct{}

// Run returns the canned version line. Pure.
func (versionExec) Run(_ context.Context, c agentio.Command, _ io.Reader) (agentio.ProcessResult, error) {
	if len(c.Args) != 1 || c.Args[0] != versionFlag {
		return agentio.ProcessResult{}, errNoExec
	}
	switch filepath.Base(c.Path) {
	case claudeBinary:
		return agentio.ProcessResult{Stdout: []byte(claudeVersion + " (Claude Code)\n")}, nil
	case codexBinary:
		return agentio.ProcessResult{Stdout: []byte("codex-cli " + codexVersion + "\n")}, nil
	}
	return agentio.ProcessResult{}, errNoExec
}
