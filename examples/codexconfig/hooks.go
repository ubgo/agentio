package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ubgo/agentio/codex"
)

// showHooksRules covers docs/codex/hooks-rules.md.
func showHooksRules(w io.Writer, m machine) error {
	if err := readHooks(w, m); err != nil {
		return err
	}
	if err := decodePayloads(w); err != nil {
		return err
	}
	return readRules(w, m)
}

// snippet:start hooks
// readHooks reads the [hooks] table of config.toml, its hooks.state
// trust records, and <home>/hooks.json.
func readHooks(w io.Writer, m machine) error {
	a := codex.New(m.host)
	h := a.DefaultHome()
	hk, err := a.HookConfig(h)
	if err != nil {
		return err
	}
	events := make([]string, 0, len(hk.Events))
	for ev := range hk.Events {
		events = append(events, string(ev))
	}
	sort.Strings(events)
	for _, name := range events {
		for _, g := range hk.Events[codex.HookEvent(name)] {
			for _, hd := range g.Hooks {
				fmt.Fprintf(w, "config hook %s matcher=%q type=%s command=%q timeout=%v\n",
					name, g.Matcher.V, hd.Type, hd.Command.V, hd.Timeout.V)
			}
		}
	}

	state, err := a.HookState(h)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s := state[k]
		fmt.Fprintf(w, "state %s enabled-set=%v enabled=%v\n", k, s.Enabled.Set, s.Enabled.V)
	}

	hf, found, err := a.HooksFile(h)
	if err != nil || !found {
		return err
	}
	fmt.Fprintf(w, "hooks.json %q events=%d\n", hf.Description.V, len(hf.Events))

	// ParseHooksFile is the same decoder for bytes you already hold.
	parsed, warns, err := codex.ParseHooksFile([]byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"/bin/done"}]}]}}`), "inline.json")
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "parsed Stop groups=%d warnings=%d known=%v\n",
		len(parsed.Events[codex.HookStop]), len(warns), codex.HookStop.IsKnown())
	return nil
}

// snippet:end hooks

// snippet:start payloads
// decodePayloads reads what Codex hands a hook program on stdin and a
// notify program as its last argument.
func decodePayloads(w io.Writer) error {
	in, err := codex.DecodeHookInput(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"shell"}`))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "hook input event=%s raw bytes=%d\n", in.Event, len(in.Raw))

	n, err := codex.DecodeNotify(`{"type":"agent-turn-complete","turn-id":"t1"}`)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "notify type=%s\n", n.Type)
	return nil
}

// snippet:end payloads

// snippet:start rules
// readRules parses execpolicy *.rules files: prefix_rule calls become
// typed rules; anything else is reported as an issue, never evaluated.
func readRules(w io.Writer, m machine) error {
	a := codex.New(m.host)
	rules, issues, err := a.Rules(a.DefaultHome(), m.project)
	if err != nil {
		return err
	}
	for _, r := range rules {
		fmt.Fprintf(w, "rule %v -> %s (%s:%d)\n", r.Pattern, r.Decision, m.rel(r.File), r.Line)
	}
	for _, is := range issues {
		fmt.Fprintf(w, "issue %s:%d %s\n", m.rel(is.File), is.Line, is.Detail)
	}

	// ParseRules never fails outright: a statement it cannot read comes back
	// as an issue, so an empty result must be checked before indexing.
	parsed, parseIssues := codex.ParseRules([]byte(`prefix_rule(pattern=["rm"], decision="forbidden")`), "inline.rules")
	if len(parsed) == 0 {
		return fmt.Errorf("inline rule not parsed: %v", parseIssues)
	}
	fmt.Fprintf(w, "parsed %v -> %s\n", parsed[0].Pattern, parsed[0].Decision)

	// threads.source text in the SQLite index is a string or a JSON object.
	fmt.Fprintf(w, "source %s %s\n", codex.ParseSessionSource("cli"),
		codex.ParseSessionSource(`{"subagent":{"thread_spawn":{"depth":1}}}`))
	return nil
}

// snippet:end rules

// showDoctor covers docs/codex/doctor.md.
func showDoctor(w io.Writer, m machine) error {
	runChecks(w, m)
	return nil
}

// snippet:start checks
// runChecks runs the disk-only doctor: it never calls a Runner method.
func runChecks(w io.Writer, m machine) {
	a := codex.New(m.host)
	for _, c := range a.Checks(context.Background(), a.DefaultHome()) {
		msg := strings.ReplaceAll(c.Message, m.userHome, "~")
		fmt.Fprintf(w, "%-5s %-18s %s\n", c.Status, c.ID, msg)
		if c.Fix != nil {
			fmt.Fprintf(w, "      fix: %s\n", argv(*c.Fix))
		}
	}
}

// snippet:end checks
