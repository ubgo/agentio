// Command codexconfig walks the disk-only Codex API of the codex module on
// a copy of the checked-in fixture home: the agent and its homes, the
// install and update cache, paths and inventory, config.toml (file,
// effective layers, profiles, overrides), MCP servers, instruction files,
// skills, plugins, hooks, execpolicy rules and the doctor.
//
// It never touches the real machine: the Host is built by hand around a
// temp user home, a PATH holding a fake npm install of codex, and a canned
// process runner standing in for the codex binary.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// defaultFixture is the checked-in Codex home, relative to the examples module.
const defaultFixture = "../codex/testdata/homes/0.162.1"

// Fixture layout inside the temp user home.
const (
	codexDir     = ".codex"    // the default Codex home under the user home
	projectDir   = "work/proj" // a project folder with a .git marker
	npmPrefix    = "npm"       // a fake global npm prefix
	fixtureUser  = "user"      // the OS login name of the fake Host
	installedVer = "0.162.1"   // the version in the fake package.json
	dirPerm      = os.FileMode(0o700)
	filePerm     = os.FileMode(0o600)
	execPerm     = os.FileMode(0o700)
)

// fixtureNow is the fake clock: a fixed instant keeps the output stable.
func fixtureNow() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }

func main() {
	fixture := flag.String("fixture", defaultFixture, "Codex fixture home to copy")
	flag.Parse()
	if err := run(os.Stdout, *fixture); err != nil {
		fmt.Fprintln(os.Stderr, "codexconfig:", err)
		os.Exit(1)
	}
}

// run copies fixture into a temp user home and prints every section.
func run(w io.Writer, fixture string) error {
	userHome, err := os.MkdirTemp("", "codexconfig-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(userHome) }()
	env, err := buildMachine(userHome, fixture)
	if err != nil {
		return err
	}
	sections := []struct {
		title string
		fn    func(io.Writer, machine) error
	}{
		{"agent", showAgent},
		{"install", showInstall},
		{"paths", showPaths},
		{"config", showConfig},
		{"mcp", showMCP},
		{"instructions and skills", showInstructionsSkills},
		{"hooks and rules", showHooksRules},
		{"doctor", showDoctor},
	}
	for _, s := range sections {
		fmt.Fprintf(w, "== %s\n", s.title)
		if err := s.fn(w, env); err != nil {
			return fmt.Errorf("%s: %w", s.title, err)
		}
	}
	return nil
}

// machine is the fake machine every section reads.
type machine struct {
	host     agentio.Host
	userHome string
	project  string
}

// rel shortens p for printing: temp-dir prefixes differ per run.
func (m machine) rel(p string) string {
	if r, err := filepath.Rel(m.userHome, p); err == nil && !strings.HasPrefix(r, "..") {
		return "~/" + filepath.ToSlash(r)
	}
	return p
}

// buildMachine lays out the temp user home and returns its Host.
func buildMachine(userHome, fixture string) (machine, error) {
	if err := agentio.CopyTree(fixture, filepath.Join(userHome, codexDir)); err != nil {
		return machine{}, err
	}
	project := filepath.Join(userHome, projectDir)
	// Trust the project in the copied config.toml: Codex ignores a
	// project's .codex/ layer, rules and AGENTS.md in an untrusted folder.
	cfgPath := filepath.Join(userHome, codexDir, "config.toml")
	cfg, err := os.ReadFile(cfgPath)
	if err != nil {
		return machine{}, err
	}
	cfg = fmt.Appendf(cfg, "\n[projects.%q]\ntrust_level = \"trusted\"\n", project)
	if err := os.WriteFile(cfgPath, cfg, filePerm); err != nil {
		return machine{}, err
	}
	files := map[string]string{
		filepath.Join(project, ".git", "HEAD"):                                                        "ref: refs/heads/main\n",
		filepath.Join(project, "AGENTS.md"):                                                           "# Project rules\n",
		filepath.Join(project, ".codex", "config.toml"):                                               "model = \"project-model\"\n",
		filepath.Join(project, ".codex", "rules", "a.rules"):                                          `prefix_rule(pattern=["npm", "test"], decision="allow")` + "\n",
		filepath.Join(userHome, npmPrefix, "lib", "node_modules", "@openai", "codex", "package.json"): `{"name":"@openai/codex","version":"` + installedVer + `"}`,
	}
	for p, body := range files {
		if err := os.MkdirAll(filepath.Dir(p), dirPerm); err != nil {
			return machine{}, err
		}
		if err := os.WriteFile(p, []byte(body), filePerm); err != nil {
			return machine{}, err
		}
	}
	// The npm layout: bin/codex is a symlink into node_modules, the way a
	// real `npm install -g @openai/codex` lays it out.
	script := filepath.Join(userHome, npmPrefix, "lib", "node_modules", "@openai", "codex", "bin", "codex.js")
	if err := os.MkdirAll(filepath.Dir(script), dirPerm); err != nil {
		return machine{}, err
	}
	if err := os.WriteFile(script, []byte("#!/usr/bin/env node\n"), execPerm); err != nil {
		return machine{}, err
	}
	bin := filepath.Join(userHome, npmPrefix, "bin")
	if err := os.MkdirAll(bin, dirPerm); err != nil {
		return machine{}, err
	}
	if err := os.Symlink(script, filepath.Join(bin, "codex")); err != nil {
		return machine{}, err
	}
	host := agentio.Host{
		UserHome:  userHome,
		User:      fixtureUser,
		Env:       []string{"HOME=" + userHome, "PATH=" + bin},
		Now:       fixtureNow,
		Secrets:   agenttest.NewFakeSecrets(),
		Processes: &agenttest.FakeProcesses{},
		Exec:      cannedCodex{},
	}
	return machine{host: host, userHome: userHome, project: project}, nil
}

// cannedCodex stands in for the codex binary: it answers the argv the
// Runner sends with output recorded from codex 0.162.1.
type cannedCodex struct{}

// Canned stdout per argv (codex 0.162.1).
const (
	outVersion  = "codex-cli 0.162.1\n"
	outStatus   = "Logged in using an API key\n"
	outFeatures = "hooks                      stable             true\nunified_exec               under development  false\n"
	outModels   = `{"models":[{"slug":"gpt-6-luna","display_name":"GPT-6-Luna","visibility":"list","supported_in_api":true,"priority":1,"default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"medium"}],"context_window":272000}]}`
)

// Run returns canned output keyed by the argv after the binary. Pure.
func (cannedCodex) Run(_ context.Context, c agentio.Command, _ io.Reader) (agentio.ProcessResult, error) {
	out := map[string]string{
		"--version":              outVersion,
		"login status":           outStatus,
		"features list":          outFeatures,
		"debug models --bundled": outModels,
	}[strings.Join(c.Args, " ")]
	return agentio.ProcessResult{Stdout: []byte(out)}, nil
}
