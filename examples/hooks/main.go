// Command hooks installs a capture command as hooks into a Claude Code, a
// Codex and a Gemini CLI home through the neutral agentio.HookInstaller:
// plan, apply with a backup, a second install that writes nothing,
// decoding one payload per agent, and a removal that gives the original
// file back byte for byte. pi has no hook installer and says why. Each
// home is a temp folder seeded with another tool's hooks; nothing reads
// the real machine, runs a binary or touches the network.
package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
	"github.com/ubgo/agentio/gemini"
	"github.com/ubgo/agentio/pi"
)

// captureCommand is the hook this example installs and removes; a fake
// path, nothing runs it.
const captureCommand = "/usr/local/bin/capture hook"

// homeTilde replaces the temp user home in printed paths.
const homeTilde = "~"

// Seed files: each holds another tool's hook on an event the capture
// command also uses, plus unrelated keys. Gemini's has a comment.
const (
	claudeSeed = `{
  "model": "opus",
  "hooks": {
    "Stop": [
      {"hooks": [{"type": "command", "command": "/usr/local/bin/other-tool stop", "timeout": 5}]}
    ]
  },
  "theme": "dark"
}
`
	codexSeed = `{
  "description": "team hooks",
  "hooks": {
    "UserPromptSubmit": [
      {"hooks": [{"type": "command", "command": "/usr/local/bin/other-tool notify"}]}
    ]
  }
}
`
	geminiSeed = `// user settings
{
  "ui": { "theme": "GitHub" },
  "hooks": {
    "AfterAgent": [
      { "hooks": [ { "type": "command", "command": "/usr/local/bin/other-tool stop" } ] }
    ]
  }
}
`
)

// Sample payloads, values replaced. Claude's follows the recorded shape;
// Codex's and Gemini's are unverified (typed from the vendor schema and
// docs, never recorded).
const (
	claudePayload = `{"session_id":"0b5e3a8e-0000-4000-8000-000000000001","transcript_path":"/home/user/.claude/projects/-work-app/0b5e3a8e-0000-4000-8000-000000000001.jsonl","cwd":"/work/app","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"add a test"}`
	codexPayload  = `{"session_id":"019a0000-0000-7000-8000-000000000001","transcript_path":null,"cwd":"/work/app","hook_event_name":"Stop","model":"gpt-5.5","permission_mode":"default","last_assistant_message":"Added the test.","stop_hook_active":false,"turn_id":"turn-1"}`
	geminiPayload = `{"session_id":"7c0e0000-0000-4000-8000-000000000001","transcript_path":"/home/user/.gemini/tmp/app/chats/session-1.json","cwd":"/work/app","hook_event_name":"AfterAgent","timestamp":"2026-11-19T10:00:00.000Z","prompt":"add a test","prompt_response":"Added the test.","stop_hook_active":false}`
)

// fixedNow is the Host clock; backup names carry it.
func fixedNow() time.Time { return time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC) }

func main() {
	if err := runMain(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "hooks:", err)
		os.Exit(1)
	}
}

// runMain builds the temp homes and runs every step. Disk · Write (temp
// dir only).
func runMain(w io.Writer) error {
	tmp, err := os.MkdirTemp("", "agentio-hooks-")
	if err != nil {
		return err
	}
	// The temp dir holds only seeds; removing it is cleanup.
	defer func() { _ = os.RemoveAll(tmp) }()
	if tmp, err = filepath.EvalSymlinks(tmp); err != nil {
		return err
	}
	host := agentio.Host{
		UserHome: tmp,
		Env:      []string{"HOME=" + tmp, "PATH="},
		Now:      fixedNow,
		HTTP:     &http.Client{Transport: noNetwork{}},
	}
	agents := []agentio.Agent{claude.New(host).Neutral(), codex.New(host).Neutral(), gemini.New(host).Neutral(), pi.New(host).Neutral()}
	seeds := map[agentio.ID]string{claude.ID: claudeSeed, codex.ID: codexSeed, gemini.ID: geminiSeed}
	payloads := map[agentio.ID]string{claude.ID: claudePayload, codex.ID: codexPayload, gemini.ID: geminiPayload}
	p := func(s string) { _, _ = fmt.Fprintln(w, strings.ReplaceAll(s, tmp, homeTilde)) }
	for _, a := range agents {
		p("== " + a.Name())
		inst, ok := agentio.HooksOf(a)
		if !ok {
			reason, _ := agentio.Unsupported(a, agentio.CapHooks)
			p("no hook installer: " + reason)
			continue
		}
		if err := seed(inst, a.DefaultHome(), seeds[a.ID()]); err != nil {
			return err
		}
		if err := install(p, inst, a.DefaultHome()); err != nil {
			return fmt.Errorf("%s: %w", a.ID(), err)
		}
		if err := decode(p, inst, payloads[a.ID()]); err != nil {
			return fmt.Errorf("%s: %w", a.ID(), err)
		}
		if err := uninstall(p, inst, a.DefaultHome(), seeds[a.ID()]); err != nil {
			return fmt.Errorf("%s: %w", a.ID(), err)
		}
	}
	return nil
}

// noNetwork fails every request: nothing in this example may go online.
type noNetwork struct{}

// errNoNetwork is what noNetwork returns.
var errNoNetwork = errors.New("hooks: network disabled in this example")

// RoundTrip refuses every request.
func (noNetwork) RoundTrip(*http.Request) (*http.Response, error) { return nil, errNoNetwork }

// printer writes one output line with the temp home shown as ~.
type printer func(line string)

// want is the capture tool's request: a command at prompt submit and at
// turn end, whatever each agent calls those events.
func want() []agentio.HookEntry {
	return []agentio.HookEntry{
		{Kind: agentio.HookPromptSubmit, Command: captureCommand},
		{Kind: agentio.HookTurnEnd, Command: captureCommand},
	}
}

// seed writes the agent's hook file with another tool's hooks, at the
// path PlanHooks names. Disk · Write (temp dir).
func seed(inst agentio.HookInstaller, h agentio.Home, body string) error {
	p, err := inst.PlanHooks(h, want())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p.File), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p.File, []byte(body), 0o600)
}

// snippet:start install
// install plans the capture hooks, applies the plan (the file is backed up
// first and only the new entries are appended), then plans again: the
// second plan finds every entry Present and applying it writes nothing.
func install(p printer, inst agentio.HookInstaller, h agentio.Home) error {
	plan, err := inst.PlanHooks(h, want())
	if err != nil {
		return err
	}
	p("file " + plan.File)
	for _, e := range plan.Add {
		p(fmt.Sprintf("  add %s -> %s", e.Kind, e.Event))
	}
	for _, m := range plan.Manual {
		p("  manual: " + m)
	}
	b, err := inst.ApplyHooks(plan)
	if err != nil {
		return err
	}
	for _, e := range b.Entries {
		p("  backup " + e.Copy)
	}
	again, err := inst.PlanHooks(h, want())
	if err != nil {
		return err
	}
	b, err = inst.ApplyHooks(again)
	if err != nil {
		return err
	}
	p(fmt.Sprintf("  second install: present=%d noop=%t backup empty=%t", len(again.Present), again.IsNoop(), b.IsEmpty()))
	return nil
}

// snippet:end install

// snippet:start decode
// decode is what the installed command does with its stdin: one payload,
// decoded into the neutral call whatever agent sent it.
func decode(p printer, inst agentio.HookInstaller, payload string) error {
	c, err := inst.DecodeHook(strings.NewReader(payload))
	if err != nil {
		return err
	}
	p(fmt.Sprintf("  decoded %s (%s) conv=%.8s dir=%s prompt=%q answer=%q", c.Kind, c.Event, c.ConversationID, c.Dir, c.Prompt, c.LastAssistantMessage))
	return nil
}

// snippet:end decode

// snippet:start uninstall
// uninstall removes the capture command and nothing else: the other
// tool's hook, every other key and the file's layout come back as they
// were.
func uninstall(p printer, inst agentio.HookInstaller, h agentio.Home, original string) error {
	n, b, err := inst.RemoveHooks(h, func(command string) bool { return command == captureCommand })
	if err != nil {
		return err
	}
	plan, err := inst.PlanHooks(h, want())
	if err != nil {
		return err
	}
	now, err := os.ReadFile(plan.File)
	if err != nil {
		return err
	}
	p(fmt.Sprintf("  removed=%d backup=%t file restored=%t", n, !b.IsEmpty(), string(now) == original))
	return nil
}

// snippet:end uninstall
