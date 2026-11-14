// Command fromspec describes a coding agent agentio has no code for as a
// plain agentio.Spec, builds it with agentio.FromSpec and uses it like a
// built-in: homes, capabilities, launch commands, login command, process
// classification and doctor checks. Everything runs in a temporary folder
// with an inert fake binary; nothing is executed.
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

// The made-up agent "Acme Code": binary acme, homes ~/.acme and ~/.acme-*,
// ACME_HOME selects a non-default home, ACME_API_KEY switches to per-token
// billing.
const acmeID agentio.ID = "acme"

// Acme Code names and the values the launch steps use.
const (
	acmeBinary    = "acme"
	acmeHomeEnv   = "ACME_HOME"
	acmeKeyEnv    = "ACME_API_KEY"
	acmeConfig    = "config.yml"
	acmeDefault   = ".acme"
	acmeWork      = ".acme-work"
	exampleConvID = "c-42"
	exampleModel  = "acme-large"
	examplePrompt = "fix the build"
)

// Temp layout and permissions.
const (
	homeDir  = "home"
	binDir   = "bin"
	dirPerm  = 0o700
	filePerm = 0o600
	execPerm = 0o755
)

// snippet:start spec
// acmeSpec describes Acme Code as data: binary, homes, argv templates
// ({args}, {id}, {model}, {prompt}), version and login args, files a new
// home shares with the default one, credential variables and how to
// recognise its process.
func acmeSpec() agentio.Spec {
	return agentio.Spec{
		ID:              acmeID,
		Name:            "Acme Code",
		Binary:          acmeBinary,
		HomeEnv:         acmeHomeEnv,
		DefaultHome:     acmeDefault,
		ExtraHomes:      ".acme-*",
		HomeMarkers:     []string{acmeConfig},
		NewArgs:         []string{"{args}"},
		ResumeArgs:      []string{"--resume", "{id}", "{args}"},
		ModelArgs:       []string{"--model", "{model}"},
		PromptArgs:      []string{"--message", "{prompt}"},
		VersionArgs:     []string{"--version"},
		LoginArgs:       []string{"login"},
		LogoutArgs:      []string{"logout"},
		SharedHomeFiles: []string{acmeConfig},
		CredentialEnv:   []string{acmeKeyEnv},
		APIBillingEnv:   []string{acmeKeyEnv},
		Process:         agentio.ProcessMatch{Basenames: []string{acmeBinary}, Exclude: []string{"acme-desktop"}},
	}
}

// snippet:end spec

func main() {
	if err := runMain(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fromspec:", err)
		os.Exit(1)
	}
}

// runMain lays out the temp machine and runs every step.
func runMain(w io.Writer) error {
	tmp, err := os.MkdirTemp("", "agentio-fromspec-")
	if err != nil {
		return err
	}
	// The temp dir is the whole example machine; removing it is cleanup.
	defer func() { _ = os.RemoveAll(tmp) }()
	h, err := setup(tmp)
	if err != nil {
		return err
	}
	return run(context.Background(), w, h)
}

// setup creates two Acme homes and an inert acme binary under tmp and
// returns a Host on them with ACME_API_KEY set. Disk · Write.
func setup(tmp string) (agentio.Host, error) {
	home := filepath.Join(tmp, homeDir)
	bin := filepath.Join(tmp, binDir)
	for _, d := range []string{acmeDefault, acmeWork} {
		if err := os.MkdirAll(filepath.Join(home, d), dirPerm); err != nil {
			return agentio.Host{}, err
		}
		if err := os.WriteFile(filepath.Join(home, d, acmeConfig), []byte("model: acme-small\n"), filePerm); err != nil {
			return agentio.Host{}, err
		}
	}
	if err := os.MkdirAll(bin, dirPerm); err != nil {
		return agentio.Host{}, err
	}
	if err := os.WriteFile(filepath.Join(bin, acmeBinary), []byte("#!/bin/sh\nexit 0\n"), execPerm); err != nil {
		return agentio.Host{}, err
	}
	return agentio.Host{
		UserHome: home,
		User:     "user",
		Env:      []string{"HOME=" + home, "PATH=" + bin, acmeKeyEnv + "=set-in-example"},
		Now:      func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) },
	}, nil
}

// tidy replaces the temp machine's folders in s. Pure.
func tidy(h agentio.Host, s string) string {
	return strings.NewReplacer(h.UserHome, "~", filepath.Join(filepath.Dir(h.UserHome), binDir), "/opt/bin").Replace(s)
}

// run prints each step.
func run(ctx context.Context, w io.Writer, h agentio.Host) error {
	fmt.Fprintln(w, "== validate ==")
	validate(w)
	fmt.Fprintln(w, "== build ==")
	a, err := build(w, h)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "== launch ==")
	if err := launch(w, h, a); err != nil {
		return err
	}
	fmt.Fprintln(w, "== login, process, doctor ==")
	return extras(ctx, w, h, a)
}

// snippet:start validate
// validate shows the checks FromSpec runs: a template without its
// placeholder and an absolute default home are refused with ErrInvalidSpec.
func validate(w io.Writer) {
	bad := acmeSpec()
	bad.ResumeArgs = []string{"--resume"}
	fmt.Fprintln(w, bad.Validate())
	bad = acmeSpec()
	bad.DefaultHome = "/etc/acme"
	err := bad.Validate()
	fmt.Fprintln(w, err, errors.Is(err, agentio.ErrInvalidSpec))
}

// snippet:end validate

// snippet:start build
// build turns the Spec into an agentio.Agent bound to h and prints what it
// can do. A config-only agent has only the capabilities its Spec data
// enables; everything else declares why it is missing.
func build(w io.Writer, h agentio.Host) (agentio.Agent, error) {
	a, err := agentio.FromSpec(acmeSpec(), h)
	if err != nil {
		return nil, err
	}
	homes, err := a.Homes()
	if err != nil {
		return nil, err
	}
	for _, home := range homes {
		fmt.Fprintf(w, "home %s default=%t %s\n", home.Name(), home.IsDefault(), tidy(h, home.Dir()))
	}
	fmt.Fprintf(w, "supported: %v\n", agentio.Supported(a))
	reason, _ := agentio.Unsupported(a, agentio.CapUsage)
	fmt.Fprintf(w, "usage: %s\n", reason)

	// Capabilities follow the data: without LoginArgs there is no LoginCommander.
	noLogin := acmeSpec()
	noLogin.LoginArgs = nil
	b, err := agentio.FromSpec(noLogin, h)
	if err != nil {
		return nil, err
	}
	_, ok := agentio.LoginCommanderOf(b)
	reason, _ = agentio.Unsupported(b, agentio.CapLoginCommander)
	fmt.Fprintf(w, "without login args: login commander %t (%s)\n", ok, reason)
	return a, nil
}

// snippet:end build

// snippet:start launch
// launch builds commands from the templates. The default home's command
// removes ACME_HOME; another home's sets it. A request field the Spec has
// no template for is ErrUnsupported, never silently dropped.
func launch(w io.Writer, h agentio.Host, a agentio.Agent) error {
	homes, err := a.Homes()
	if err != nil {
		return err
	}
	def, work := homes[0], homes[1]
	requests := []struct {
		home agentio.Home
		req  agentio.LaunchRequest
	}{
		{def, agentio.LaunchRequest{}},
		{def, agentio.LaunchRequest{Model: exampleModel, Prompt: examplePrompt}},
		{work, agentio.LaunchRequest{ResumeID: exampleConvID}},
	}
	for _, r := range requests {
		cmd, err := a.Launch(r.home.Target(), r.req)
		if err != nil {
			return err
		}
		env, set := agentio.EnvLookup(cmd.Env, acmeHomeEnv)
		fmt.Fprintf(w, "%s: %s (%s set=%t %s)\n", r.home.Name(), cmd, acmeHomeEnv, set, tidy(h, env))
	}
	_, err = a.Launch(def.Target(), agentio.LaunchRequest{Headless: true})
	fmt.Fprintf(w, "headless: %v (unsupported=%t)\n", err, errors.Is(err, agentio.ErrUnsupported))
	_, err = a.Launch(def.Target(), agentio.LaunchRequest{Autonomy: agentio.AutonomyFull})
	fmt.Fprintf(w, "full autonomy: %v\n", err)
	return nil
}

// snippet:end launch

// snippet:start extras
// extras builds the login command, classifies two process rows and runs
// the L1 doctor checks; all of it comes from the Spec.
func extras(ctx context.Context, w io.Writer, h agentio.Host, a agentio.Agent) error {
	if lc, ok := agentio.LoginCommanderOf(a); ok {
		cmd, err := lc.LoginCommand(a.DefaultHome(), agentio.LoginBrowser)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "login: %s, logout: %s\n", cmd, lc.LogoutCommand(a.DefaultHome()))
		_, err = lc.LoginCommand(a.DefaultHome(), agentio.LoginDevice)
		fmt.Fprintf(w, "device login: %v\n", err)
	}
	rows := []agentio.Process{
		{PID: 10, Exe: "/opt/bin/acme", Argv: []string{"acme", "--resume", exampleConvID}},
		{PID: 11, Exe: "/opt/acme-desktop/acme", Argv: []string{"acme"}},
	}
	for _, p := range rows {
		info, ok := agentio.ClassifyProcess([]agentio.Agent{a}, p)
		fmt.Fprintf(w, "pid %d: match=%t kind=%s session=%t\n", p.PID, ok, info.Kind, info.Session)
	}
	if d, ok := agentio.DoctorOf(a); ok {
		for _, c := range d.Checks(ctx, a.DefaultHome()) {
			fmt.Fprintf(w, "check %-15s %-4s %s\n", c.ID, c.Status, tidy(h, c.Message))
		}
	}
	return nil
}

// snippet:end extras
