package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// snippet:start agent-interface
// describe works on any agent through the neutral interface: no type
// switch, no vendor import. Only Homes touches the disk.
func describe(p printer, a agentio.Agent) error {
	p.line(fmt.Sprintf("agent: id=%s name=%q", a.ID(), a.Name()))
	p.line(fmt.Sprintf("agent: default home=%s", a.DefaultHome().Dir()))
	if h, ok := a.HomeFromEnv(); ok {
		p.line("agent: env selects " + h.Dir())
	}
	homes, err := a.Homes()
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("agent: %d homes on disk", len(homes)))
	// IDs from config go through ParseID; built-in IDs are typed constants.
	if _, err := agentio.ParseID("Claude"); errors.Is(err, agentio.ErrInvalidID) {
		p.line("agent: ParseID(\"Claude\") refused: " + err.Error())
	}
	return nil
}

// snippet:end agent-interface

// snippet:start read-spec
// readSpec prints the data Claude's core is built from. A Spec is plain
// data: binary, home variable and folders, home markers, argv templates,
// credential variables and how to recognise the process. Pure.
func readSpec(p printer, a agentio.Agent) error {
	s := a.Spec()
	if err := s.Validate(); err != nil {
		return err
	}
	p.line(fmt.Sprintf("spec: binary=%s home env=%s default home=%s extra homes=%s", s.Binary, s.HomeEnv, s.DefaultHome, s.ExtraHomes))
	p.line(fmt.Sprintf("spec: markers=%v", s.HomeMarkers))
	p.line(fmt.Sprintf("spec: resume template=%v", s.ResumeArgs))
	p.line(fmt.Sprintf("spec: credential env=%v", s.CredentialEnv))
	for _, proc := range []agentio.Process{
		{PID: 1, Argv: []string{"/usr/local/bin/claude", "--resume", "x"}},
		{PID: 2, Exe: "/Applications/Claude.app/Contents/MacOS/claude"},
	} {
		p.line(fmt.Sprintf("spec: process exe=%q argv=%v matches=%t", proc.Exe, proc.Argv, s.Process.Match(proc)))
	}
	return nil
}

// snippet:end read-spec

// snippet:start build-core
// toySpec describes a made-up CLI entirely as data.
func toySpec() agentio.Spec {
	return agentio.Spec{
		ID: "toy", Name: "Toy Agent", Binary: "toy",
		HomeEnv: "TOY_HOME", DefaultHome: ".toy", ExtraHomes: ".toy-*",
		HomeMarkers: []string{"config.json"},
		NewArgs:     []string{"{args}"},
		ResumeArgs:  []string{"--resume", "{id}", "{args}"},
		ModelArgs:   []string{"--model", "{model}"},
		PromptArgs:  []string{"{prompt}"},
		LoginArgs:   []string{"login"},
	}
}

// buildCore binds a Spec to a Host and uses the L1 core every agent shares:
// launch from the templates, login command, doctor checks. NewCore
// validates the Spec first.
func buildCore(p printer, h agentio.Host) error {
	if _, err := agentio.NewCore(agentio.Spec{ID: "Toy"}, h); errors.Is(err, agentio.ErrInvalidSpec) {
		p.line("core: invalid spec refused: " + err.Error())
	}
	core, err := agentio.NewCore(toySpec(), h)
	if err != nil {
		return err
	}
	work := core.HomeAt(".toy-work")
	cmd, err := core.Launch(work.Target(), agentio.LaunchRequest{ResumeID: "abc", Model: "fast", Prompt: "hello"})
	if err != nil {
		return err
	}
	p.line("core: launch " + cmd.String())
	p.line("core: env " + strings.Join(onlyVar(cmd.Env, "TOY_HOME"), " "))
	_, err = core.Launch(work.Target(), agentio.LaunchRequest{Headless: true})
	p.line(fmt.Sprintf("core: headless unsupported=%t", errors.Is(err, agentio.ErrUnsupported)))
	login, err := core.LoginCommand(core.DefaultHome(), agentio.LoginBrowser)
	if err != nil {
		return err
	}
	p.line("core: login " + login.String())
	for _, c := range core.Checks(context.Background(), work) {
		p.line(fmt.Sprintf("core: check %s %s: %s", c.ID, c.Status, c.Message))
	}
	return nil
}

// snippet:end build-core

// onlyVar returns the entries of env for name.
func onlyVar(env []string, name string) []string {
	var out []string
	for _, kv := range env {
		if strings.HasPrefix(kv, name+"=") {
			out = append(out, kv)
		}
	}
	return out
}

// neutralClaude is the neutral view of Claude on h, for the Agent and Spec
// steps.
func neutralClaude(h agentio.Host) agentio.Agent { return claude.New(h).Neutral() }
