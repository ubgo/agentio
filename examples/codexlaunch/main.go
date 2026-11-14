// Command codexlaunch builds Codex command lines without running them:
// interactive start, resume, fork, headless exec and review, the vendor's
// own archive/delete/queue/doctor/server commands, the version gate on
// launch flags, and the decoder for `codex exec --json` output (fed from a
// recorded stream, not a live process).
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/codex"
)

// Layout facts.
const (
	codexDir    = ".codex"
	otherDir    = "work-codex" // a second, non-default home
	fixtureUser = "user"
	dirPerm     = os.FileMode(0o700)
	filePerm    = os.FileMode(0o600)
	execPerm    = os.FileMode(0o700)
	threadID    = "0199e1a0-0000-7000-8000-000000000001"
	oldVersion  = "0.150.0" // older than --no-daemon (0.156.0)
)

// execStream is `codex exec --json` stdout recorded from codex 0.162.1
// (values replaced), including a diagnostic line and a future event type.
const execStream = `{"type":"thread.started","thread_id":"th-1"}
{"type":"turn.started"}
{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"done"}}
{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"ls","aggregated_output":"a","exit_code":0,"status":"completed"}}
warning: not json
{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":3,"reasoning_output_tokens":1}}
{"type":"future.event","x":1}
`

func main() {
	flag.Parse()
	if err := run(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "codexlaunch:", err)
		os.Exit(1)
	}
}

// run builds a temp user home with two Codex homes and a placeholder
// codex executable on PATH (never run), then prints every section.
func run(w io.Writer) error {
	userHome, err := os.MkdirTemp("", "codexlaunch-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(userHome) }()
	for _, d := range []string{codexDir, otherDir} {
		if err := os.MkdirAll(filepath.Join(userHome, d), dirPerm); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(userHome, d, "config.toml"), nil, filePerm); err != nil {
			return err
		}
	}
	bin := filepath.Join(userHome, "bin")
	if err := os.MkdirAll(bin, dirPerm); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(bin, codex.Binary), []byte("#!/bin/sh\nexit 1\n"), execPerm); err != nil {
		return err
	}
	host := agentio.Host{
		UserHome:  userHome,
		User:      fixtureUser,
		Env:       []string{"HOME=" + userHome, "PATH=" + bin},
		Now:       agenttest.FixedNow,
		Secrets:   agenttest.NewFakeSecrets(),
		Processes: &agenttest.FakeProcesses{},
	}
	a := codex.New(host)
	other, err := a.HomeAt(filepath.Join(userHome, otherDir))
	if err != nil {
		return err
	}
	steps := []func(io.Writer, *codex.Agent, codex.Home) error{
		startAndResume, execHeadless, vendorCommands, unavailableFeature, decodeExec, neutralEvents,
	}
	for _, step := range steps {
		if err := step(w, a, other); err != nil {
			return err
		}
	}
	return nil
}

// show prints a Command as `codex <args>` plus the CODEX_HOME it sets,
// and any build warnings. The binary path is a temp dir, so only its
// base name is printed; an argument with a space is quoted.
func show(w io.Writer, label string, c agentio.Command) {
	args := make([]string, len(c.Args))
	for i, arg := range c.Args {
		args[i] = arg
		if strings.Contains(arg, " ") {
			args[i] = strconv.Quote(arg)
		}
	}
	fmt.Fprintf(w, "%-12s %s %s", label, filepath.Base(c.Path), strings.Join(args, " "))
	if v, ok := agentio.EnvLookup(c.Env, codex.EnvHome); ok {
		fmt.Fprintf(w, "  [CODEX_HOME=%s]", filepath.Base(v))
	}
	for _, wn := range c.Warnings {
		fmt.Fprintf(w, "  [warning %s]", wn.Kind)
	}
	fmt.Fprintln(w)
}

// snippet:start start
// startAndResume builds interactive commands. TargetFor reads the
// installed version from disk (here unknown: the placeholder binary has
// no package.json), so the builder warns instead of gating flags.
func startAndResume(w io.Writer, a *codex.Agent, other codex.Home) error {
	t, err := a.TargetFor(a.DefaultHome())
	if err != nil {
		return err
	}
	opts := codex.LaunchOptions{
		Prompt:   "fix the build",
		Model:    "gpt-5.5",
		Sandbox:  codex.ModeWorkspaceWrite,
		Approval: codex.ApprovalOnRequest,
		AddDirs:  []string{"/work/shared"},
		Overrides: []codex.Override{
			codex.OverrideString("model_reasoning_effort", "high"),
		},
	}
	c, err := a.New(t, opts)
	if err != nil {
		return err
	}
	show(w, "new", c)

	// Pin the version when you know it: no warning, and flags are gated.
	t.Version = agentio.MustVersion(codex.TestedMax)
	r, err := a.Resume(t, threadID, codex.LaunchOptions{})
	if err != nil {
		return err
	}
	show(w, "resume", r)
	last, err := a.ResumeLast(t, codex.LaunchOptions{}, true)
	if err != nil {
		return err
	}
	show(w, "resume last", last)
	f, err := a.Fork(t, threadID, codex.LaunchOptions{Prompt: "try another way"})
	if err != nil {
		return err
	}
	show(w, "fork", f)

	// A non-default home sets CODEX_HOME; the default home removes it.
	o, err := a.New(other.Target(), codex.LaunchOptions{})
	if err != nil {
		return err
	}
	show(w, "other home", o)

	if _, err := a.Resume(t, "", codex.LaunchOptions{}); errors.Is(err, codex.ErrNoSessionID) {
		fmt.Fprintln(w, "resume without an id: ErrNoSessionID")
	}
	if _, err := a.New(t, codex.LaunchOptions{BypassAll: true}); errors.Is(err, agentio.ErrDangerousNotAllowed) {
		fmt.Fprintln(w, "bypass without Target.AllowDangerous: ErrDangerousNotAllowed")
	}

	// ProfileCheck refuses a v2 profile name that a legacy [profiles]
	// table also uses, before Codex does.
	legacy, _, err := codex.ParseConfig([]byte("[profiles.work]\nmodel = \"o3\"\n"), "config.toml")
	if err != nil {
		return err
	}
	if _, err := a.New(t, codex.LaunchOptions{Profile: "work", ProfileCheck: &legacy}); errors.Is(err, codex.ErrProfileConflict) {
		fmt.Fprintln(w, "profile work: ErrProfileConflict")
	}
	return nil
}

// snippet:end start

// snippet:start exec
// execHeadless builds `codex exec` runs. Building is pure; RUNNING the
// command spends quota. In exec, Approval renders as -c approval_policy.
func execHeadless(w io.Writer, a *codex.Agent, _ codex.Home) error {
	t := a.DefaultHome().Target()
	t.Version = agentio.MustVersion(codex.TestedMax)
	e, err := a.Exec(t, codex.ExecOptions{
		LaunchOptions:     codex.LaunchOptions{PromptFromStdin: true, Approval: codex.ApprovalNever, Sandbox: codex.ModeReadOnly},
		JSON:              true,
		OutputLastMessage: "/tmp/last.txt",
		SkipGitRepoCheck:  true,
	})
	if err != nil {
		return err
	}
	show(w, "exec", e)
	er, err := a.ExecResume(t, threadID, codex.ExecOptions{LaunchOptions: codex.LaunchOptions{Prompt: "continue"}, JSON: true})
	if err != nil {
		return err
	}
	show(w, "exec resume", er)
	rv, err := a.Review(t, codex.ReviewTarget{Base: "main", Title: "Release review"}, codex.ExecOptions{JSON: true})
	if err != nil {
		return err
	}
	show(w, "review", rv)
	return nil
}

// snippet:end exec

// snippet:start vendor
// vendorCommands builds the vendor's own management commands: Codex keeps
// rollouts, its SQLite index and config consistent itself, so changes go
// through these instead of file writes.
func vendorCommands(w io.Writer, a *codex.Agent, _ codex.Home) error {
	h := a.DefaultHome()
	show(w, "archive", a.Archive(h, threadID))
	show(w, "unarchive", a.Unarchive(h, threadID))
	show(w, "delete", a.Delete(h, threadID, true))
	show(w, "queue", a.Queue(h, threadID, "run the tests next"))
	show(w, "doctor", a.DoctorCommand(h, true))
	show(w, "mcp-server", a.MCPServer(h))
	show(w, "app-server", a.AppServer(h, "ws://127.0.0.1:8390"))
	show(w, "feature", a.FeatureCommand(h, "hooks", true))
	return nil
}

// snippet:end vendor

// snippet:start unavailable
// unavailableFeature asks for a flag the installed Codex provably lacks:
// the builder refuses with *FeatureUnavailableError, which also matches
// agentio.ErrFeatureUnavailable.
func unavailableFeature(w io.Writer, a *codex.Agent, _ codex.Home) error {
	t := a.DefaultHome().Target()
	t.Version = agentio.MustVersion(oldVersion)
	_, err := a.New(t, codex.LaunchOptions{NoDaemon: true})
	var fe *codex.FeatureUnavailableError
	if !errors.As(err, &fe) {
		return fmt.Errorf("expected a feature error, got %v", err)
	}
	fmt.Fprintf(w, "feature=%s installed=%s since=%s neutral match=%v\n",
		fe.Feature, fe.Installed, fe.Since, errors.Is(err, agentio.ErrFeatureUnavailable))
	span := codex.FeatureInfo(codex.FeatureFlagNoDaemon)
	fmt.Fprintf(w, "span known=%v since=%s\n", span.Known(), span.Since)
	return nil
}

// snippet:end unavailable

// snippet:start decode
// decodeExec reads `codex exec --json` stdout one event at a time. Blank
// and non-JSON lines are skipped; unknown types keep Type and Raw.
func decodeExec(w io.Writer, a *codex.Agent, _ codex.Home) error {
	fmt.Fprintf(w, "decoder written against codex %s\n", a.ExecEventVersion())
	d := codex.NewExecDecoder(strings.NewReader(execStream))
	for {
		ev, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch {
		case ev.Type == codex.ExecThreadStarted:
			fmt.Fprintf(w, "event %s thread=%s\n", ev.Type, ev.ThreadID)
		case ev.Item != nil && ev.Item.AgentMessage != nil:
			fmt.Fprintf(w, "event %s agent_message %q\n", ev.Type, ev.Item.AgentMessage.Text)
		case ev.Item != nil && ev.Item.Command != nil:
			fmt.Fprintf(w, "event %s command %q exit=%d\n", ev.Type, ev.Item.Command.Command, *ev.Item.Command.ExitCode)
		case ev.Usage != nil:
			fmt.Fprintf(w, "event %s input=%d output=%d\n", ev.Type, ev.Usage.Input, ev.Usage.Output)
		default:
			fmt.Fprintf(w, "event %s known=%v\n", ev.Type, ev.Type.IsKnown())
		}
	}
	one, err := codex.ParseExecEvent([]byte(`{"type":"turn.failed","error":{"message":"quota"}}`))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "parsed %s error=%q\n", one.Type, one.Error.Message)
	return nil
}

// snippet:end decode

// snippet:start neutral-events
// neutralEvents reads the same stream as agentio.RunEvent values, the
// shape code that drives several agents consumes.
func neutralEvents(w io.Writer, a *codex.Agent, _ codex.Home) error {
	d := a.NewRunDecoder(strings.NewReader(execStream))
	for {
		ev, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "run event %s", ev.Type)
		if ev.Tokens != nil {
			fmt.Fprintf(w, " reasoning=%d", ev.Tokens.Reasoning)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// snippet:end neutral-events
