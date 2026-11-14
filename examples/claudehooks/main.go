// Command claudehooks shows the hook side of the claude module: listing the
// hooks configured in every settings layer, a hook program that decodes its
// stdin and replies, a status-line program, and the Writer door that patches
// settings, adds and removes hooks and moves history, always backing the
// file up first.
//
// Modes:
//
//	go run ./claudehooks                         demo on a temp copy of the fixture home
//	go run ./claudehooks hook < input.json       act as a Claude Code hook
//	go run ./claudehooks statusline < input.json act as a status-line command
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// defaultFixture is the fixture user home, relative to the examples module
// root (where `go run ./claudehooks` runs).
const defaultFixture = "../claude/testdata/homes/2.1.325/userhome"

// Command-line modes.
const (
	modeHook       = "hook"
	modeStatusLine = "statusline"
)

// Values the example uses.
const (
	fixtureProject  = "project"
	fixtureUser     = "user"
	workHome        = ".claude-work"
	ourHookCommand  = "/usr/local/bin/audit-hook" // the hook this example adds, then removes
	ourHookTimeout  = 5 * time.Second
	statusLineCmd   = "/usr/local/bin/my-statusline"
	movedFrom       = "/work/proj12" // has history in the default home
	movedTo         = "/work/proj12-renamed"
	movedAcrossDir  = "/work/proj13" // moved from the default home to the work home
	dangerousPrefix = "rm -rf"
	homeTilde       = "~"
	bakMarker       = ".agentio-bak-"
)

// Sample hook inputs in Claude Code's documented shape (values replaced).
// They are constants, not an embedded file system, because the repo allows
// no package-level vars; testdata/ holds the same JSON for piping into the
// hook and statusline modes, and TestInputsMatchTestdata keeps them equal.
const (
	preToolUseInput = `{"session_id":"0b5e3a8e-0000-4000-8000-000000000001","transcript_path":"/home/user/.claude/projects/-work-project/0b5e3a8e-0000-4000-8000-000000000001.jsonl","cwd":"/work/project","permission_mode":"default","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"rm -rf /work/project/build","description":"Clean the build dir"},"tool_use_id":"toolu_01","effort":{"level":"high"},"new_field":true}`
	statusLineInput = `{"session_id":"0b5e3a8e-0000-4000-8000-000000000001","transcript_path":"/home/user/.claude/projects/-work-project/0b5e3a8e-0000-4000-8000-000000000001.jsonl","cwd":"/work/project/src","version":"2.1.325","model":{"id":"claude-opus-5-5","display_name":"Opus"},"workspace":{"current_dir":"/work/project/src","project_dir":"/work/project"},"output_style":{"name":"default"},"cost":{"total_cost_usd":0.4213,"total_duration_ms":95000,"total_api_duration_ms":41000,"total_lines_added":12,"total_lines_removed":3},"exceeds_200k_tokens":false}`
)

// fixedNow is the Host clock; backup names carry it, so output is stable.
func fixedNow() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) }

func main() {
	fixture := flag.String("fixture", defaultFixture, "fixture user home to copy (demo mode)")
	flag.Parse()
	var err error
	switch flag.Arg(0) {
	case modeHook:
		err = hookMain(os.Stdin, os.Stdout)
	case modeStatusLine:
		err = statusLineMain(os.Stdin, os.Stdout)
	default:
		err = run(os.Stdout, *fixture)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "claudehooks:", err)
		os.Exit(1)
	}
}

// run copies fixture into a temp user home and prints every section.
func run(w io.Writer, fixture string) error {
	tmp, err := os.MkdirTemp("", "claudehooks-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }() // temp copy; nothing to keep
	userHome := filepath.Join(tmp, "home")   // CopyTree refuses an existing dst
	if err := agentio.CopyTree(fixture, userHome); err != nil {
		return err
	}
	a := claude.New(fixtureHost(userHome))
	p := printer{w: w, home: userHome}
	steps := []func(printer, *claude.Agent) error{
		showHooks, showMatchers, showHookProgram, showStatusLine,
		patchSettings, addHook, removeHooks, moveHistory, moveAcrossHomes, linkHome,
	}
	for _, step := range steps {
		if err := step(p, a); err != nil {
			return err
		}
	}
	return nil
}

// fixtureHost points every Host input at the temp user home; no network.
func fixtureHost(userHome string) agentio.Host {
	return agentio.Host{
		UserHome: userHome,
		User:     fixtureUser,
		Env:      []string{"HOME=" + userHome, "PATH="},
		Now:      fixedNow,
		HTTP:     &http.Client{Transport: noNetwork{}},
	}
}

// noNetwork fails every request: nothing in this example may go online.
type noNetwork struct{}

// errNoNetwork is what noNetwork returns.
var errNoNetwork = errors.New("claudehooks: network disabled in this example")

// RoundTrip refuses every request, so a read that would reach the network
// fails loudly instead of leaving the machine.
func (noNetwork) RoundTrip(*http.Request) (*http.Response, error) { return nil, errNoNetwork }

// printer writes lines with the temp user home shown as "~".
type printer struct {
	w    io.Writer
	home string
}

// line prints one line (callers format it with fmt.Sprintf) with the home
// folder shown as ~.
// A write error on the output (a closed pipe) is dropped: the example has
// nothing better to do with it than stop printing.
func (p printer) line(s string) {
	_, _ = fmt.Fprintln(p.w, strings.ReplaceAll(s, p.home, homeTilde))
}

// section prints a blank line and a "== title" heading.
func (p printer) section(title string) { p.line("\n== " + title) }

// project opens the fixture project under the user home.
func project(a *claude.Agent) (claude.Project, error) {
	return a.Project(filepath.Join(a.Host().UserHome, fixtureProject))
}

// snippet:start hooks
// showHooks lists every hook handler in effect, with the layer it came from,
// counted per event so the output stays short.
func showHooks(p printer, a *claude.Agent) error {
	p.section("configured hooks")
	proj, err := project(a)
	if err != nil {
		return err
	}
	hooks, err := a.Hooks(a.DefaultHome(), &proj)
	if err != nil {
		return err
	}
	type key struct {
		scope   claude.SettingsScope
		event   claude.HookEvent
		matcher string
	}
	count := map[key]int{}
	var keys []key
	for _, h := range hooks {
		k := key{h.Scope, h.Event, h.Matcher}
		if count[k] == 0 {
			keys = append(keys, k)
		}
		count[k]++
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].scope != keys[j].scope {
			return keys[i].scope < keys[j].scope
		}
		if keys[i].event != keys[j].event {
			return keys[i].event < keys[j].event
		}
		return keys[i].matcher < keys[j].matcher
	})
	for _, k := range keys {
		p.line(fmt.Sprintf("%-7s %-17s matcher=%-20q handlers=%d", k.scope, k.event, k.matcher, count[k]))
	}
	first := hooks[0].Handler
	p.line(fmt.Sprintf("first handler: type=%s command=%s timeout=%s", first.Type, first.Command, first.Timeout))
	return nil
}

// snippet:end hooks

// snippet:start matchers
// showMatchers checks which events filter on a matcher and how a matcher
// string is applied to a tool name.
func showMatchers(p printer, _ *claude.Agent) error {
	p.section("events and matchers")
	var withMatcher []claude.HookEvent
	for _, ev := range claude.HookEvents() {
		if ev.SupportsMatcher() {
			withMatcher = append(withMatcher, ev)
		}
	}
	p.line(fmt.Sprintf("documented events=%d, %d take a matcher, e.g. %v", len(claude.HookEvents()), len(withMatcher), withMatcher[:3]))
	p.line(fmt.Sprintf("Stop takes a matcher: %t", claude.HookStop.SupportsMatcher()))
	for _, c := range []struct{ matcher, tool string }{
		{"Write|Edit|MultiEdit", "Edit"},
		{"Write|Edit|MultiEdit", "NotebookEdit"},
		{"mcp__.*__write.*", "mcp__fs__write_file"},
		{"", "Bash"},
	} {
		m := claude.HookMatcher{Matcher: c.matcher}
		p.line(fmt.Sprintf("matcher %-22q tool %-20s -> %t", c.matcher, c.tool, m.Matches(c.tool)))
	}
	return nil
}

// snippet:end matchers

// snippet:start hookprogram
// hookMain is a complete PreToolUse hook: it decodes the input Claude Code
// writes on stdin and denies Bash commands that start with "rm -rf".
// Anything else gets no output, which lets the normal permission flow run.
func hookMain(stdin io.Reader, stdout io.Writer) error {
	in, err := claude.DecodeHookInput(stdin)
	if err != nil {
		return err
	}
	if in.HookEventName != claude.HookPreToolUse || in.ToolName != "Bash" {
		return nil
	}
	var bash struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(in.ToolInput, &bash); err != nil {
		return err
	}
	if !strings.HasPrefix(bash.Command, dangerousPrefix) {
		return nil
	}
	out := claude.HookOutput{
		HookSpecificOutput: &claude.HookSpecificOutput{
			HookEventName:            claude.HookPreToolUse,
			PermissionDecision:       "deny",
			PermissionDecisionReason: "recursive delete is blocked by policy",
		},
	}
	return out.Encode(stdout)
}

// snippet:end hookprogram

// snippet:start showhook
// showHookProgram feeds the sample PreToolUse input to hookMain and prints
// both what was decoded and what the hook replied.
func showHookProgram(p printer, _ *claude.Agent) error {
	p.section("hook program")
	in, err := claude.DecodeHookInput(strings.NewReader(preToolUseInput))
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("event=%s tool=%s mode=%s effort=%s unknown=%v", in.HookEventName, in.ToolName, in.PermissionMode, in.Effort.Level, in.Unknown.Keys()))
	var reply bytes.Buffer
	if err := hookMain(strings.NewReader(preToolUseInput), &reply); err != nil {
		return err
	}
	p.line(fmt.Sprintf("reply: %s", strings.TrimSpace(reply.String())))
	return nil
}

// snippet:end showhook

// snippet:start statusline
// statusLineMain is a complete status-line command: Claude Code runs it,
// writes the session state on stdin and shows the first line it prints.
func statusLineMain(stdin io.Reader, stdout io.Writer) error {
	in, err := claude.DecodeStatusLineInput(stdin)
	if err != nil {
		return err
	}
	dir := filepath.Base(in.Workspace.CurrentDir)
	line := fmt.Sprintf("[%s] %s", in.Model.DisplayName, dir)
	if in.Cost != nil { // absent before the first request
		line += fmt.Sprintf(" $%.2f +%d -%d", in.Cost.TotalCostUSD, in.Cost.TotalLinesAdded, in.Cost.TotalLinesRemoved)
	}
	_, err = fmt.Fprintln(stdout, line)
	return err
}

// snippet:end statusline

// snippet:start showstatus
// showStatusLine sets the statusLine setting through the Writer, reads it
// back, and runs statusLineMain on the sample input.
func showStatusLine(p printer, a *claude.Agent) error {
	p.section("status line")
	h := a.DefaultHome()
	setting, err := json.Marshal(claude.StatusLine{Type: "command", Command: statusLineCmd})
	if err != nil {
		return err
	}
	if _, err := a.Writer().SetUserSetting(h, claude.SettingsPatch{Set: map[string]agentio.Raw{"statusLine": setting}}); err != nil {
		return err
	}
	user, err := a.UserSettings(h)
	if err != nil {
		return err
	}
	if sl := user.Settings.StatusLine; sl != nil {
		p.line(fmt.Sprintf("setting: type=%s command=%s", sl.Type, sl.Command))
	}
	in, err := claude.DecodeStatusLineInput(strings.NewReader(statusLineInput))
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("input: model=%s project=%s style=%s unknown=%v", in.Model.ID, in.Workspace.ProjectDir, in.OutputStyle.Name, in.Unknown.Keys()))
	var line bytes.Buffer
	if err := statusLineMain(strings.NewReader(statusLineInput), &line); err != nil {
		return err
	}
	p.line(fmt.Sprintf("prints: %s", strings.TrimSpace(line.String())))
	return nil
}

// snippet:end showstatus

// snippet:start patch
// patchSettings changes top-level keys of the user and local settings files.
// Every other key is kept, and the old file is backed up first.
func patchSettings(p printer, a *claude.Agent) error {
	p.section("patch settings")
	w := a.Writer()
	b, err := w.SetUserSetting(a.DefaultHome(), claude.SettingsPatch{
		Set:    map[string]agentio.Raw{"model": agentio.Raw(`"sonnet"`)},
		Delete: []string{"tui"},
	})
	if err != nil {
		return err
	}
	printBackup(p, b)
	proj, err := project(a)
	if err != nil {
		return err
	}
	b, err = w.SetProjectSetting(proj, true, claude.SettingsPatch{ // true: settings.local.json
		Set: map[string]agentio.Raw{"outputStyle": agentio.Raw(`"Explanatory"`)},
	})
	if err != nil {
		return err
	}
	printBackup(p, b)
	user, err := a.UserSettings(a.DefaultHome())
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("user model=%s tui=%q theme=%s hooks kept=%d events", user.Settings.Model, user.Settings.TUI, user.Settings.Theme, len(user.Settings.Hooks)))
	if _, err := w.SetUserSetting(a.DefaultHome(), claude.SettingsPatch{Set: map[string]agentio.Raw{"model": agentio.Raw(`sonnet`)}}); err != nil {
		p.line(fmt.Sprintf("invalid JSON value refused: %v", err))
	}
	return nil
}

// snippet:end patch

// snippet:start addhook
// addHook appends a Stop hook to the user settings; existing hooks stay.
func addHook(p printer, a *claude.Agent) error {
	p.section("add a hook")
	h := a.DefaultHome()
	b, err := a.Writer().AddHook(h, nil, claude.ScopeUser, claude.HookStop, claude.HookMatcher{
		Hooks: []claude.HookHandler{{Type: claude.HookTypeCommand, Command: ourHookCommand, Timeout: ourHookTimeout}},
	})
	if err != nil {
		return err
	}
	printBackup(p, b)
	user, err := a.UserSettings(h)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("Stop matcher groups now=%d", len(user.Settings.Hooks[claude.HookStop])))
	return nil
}

// snippet:end addhook

// snippet:start removehooks
// removeHooks drops the handler addHook added. keep returns false for the
// handlers to remove; a run that removes nothing writes nothing.
func removeHooks(p printer, a *claude.Agent) error {
	p.section("remove hooks")
	h := a.DefaultHome()
	keep := func(_ claude.HookEvent, hh claude.HookHandler) bool { return hh.Command != ourHookCommand }
	n, b, err := a.Writer().RemoveHooks(h, nil, claude.ScopeUser, keep)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("removed=%d", n))
	printBackup(p, b)
	n, b, err = a.Writer().RemoveHooks(h, nil, claude.ScopeUser, keep)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("second run removed=%d backup empty=%t", n, b.IsEmpty()))
	return nil
}

// snippet:end removehooks

// snippet:start movehistory
// moveHistory re-keys a folder's history after the folder was renamed, so
// `claude --resume` finds the old conversations from the new path.
func moveHistory(p printer, a *claude.Agent) error {
	p.section("move history")
	h := a.DefaultHome()
	res, err := a.Writer().MoveHistory(h, movedFrom, movedTo)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("moved=%t now at %s", res.Moved, a.HistoryDir(h, movedTo)))
	printBackup(p, res.Backup)
	res, err = a.Writer().MoveHistory(h, "/work/no-history", movedTo)
	p.line(fmt.Sprintf("folder without history: moved=%t err=%v", res.Moved, err))
	if err := os.MkdirAll(a.HistoryDir(h, movedFrom), 0o700); err != nil {
		return err
	}
	_, err = a.Writer().MoveHistory(h, movedFrom, movedTo)
	p.line(fmt.Sprintf("destination exists: ErrHistoryExists=%t", errors.Is(err, claude.ErrHistoryExists)))
	return nil
}

// snippet:end movehistory

// snippet:start moveacross
// moveAcrossHomes plans moving one folder's conversations from the default
// home to the work home, then applies the plan through the Writer.
func moveAcrossHomes(p printer, a *claude.Agent) error {
	p.section("move between homes")
	from := a.DefaultHome()
	to := a.HomeAt(filepath.Join(a.Host().UserHome, workHome))
	plan, err := a.PlanMove(agentio.MoveRequest{From: from.Neutral(), To: to.Neutral(), Dir: movedAcrossDir})
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("plan: %d conversation(s), %d item(s), blockers=%v", len(plan.Conversations), len(plan.Items), plan.Blockers))
	if len(plan.Blockers) > 0 {
		return nil
	}
	b, err := a.Writer().ApplyMove(plan)
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("applied, %d backup entr(ies); work home has history=%t", len(b.Entries), hasHistory(a, to, movedAcrossDir)))
	return nil
}

// snippet:end moveacross

// snippet:start link
// linkHome shares the default home's CLAUDE.md, skills and settings with
// the work home by symlink. Existing targets are never overwritten.
func linkHome(p printer, a *claude.Agent) error {
	p.section("link shared files")
	def := a.DefaultHome()
	work := a.HomeAt(filepath.Join(a.Host().UserHome, workHome))
	links, err := agentio.PlanHomeLinks(def.Neutral(), work.Neutral(), []string{"CLAUDE.md", "skills", "settings.json"})
	if err != nil {
		return err
	}
	for _, l := range links {
		p.line(fmt.Sprintf("plan %s -> %s exists=%t", l.To, l.From, l.Exists))
	}
	if err := a.Writer().ApplyHomeLinks(links); err != nil {
		return err
	}
	target, err := os.Readlink(filepath.Join(work.Dir(), "CLAUDE.md"))
	if err != nil {
		return err
	}
	p.line(fmt.Sprintf("work CLAUDE.md -> %s", target))
	return nil
}

// snippet:end link

// hasHistory reports whether dir has a history folder in h.
func hasHistory(a *claude.Agent, h claude.Home, dir string) bool {
	ok, err := a.HasHistory(h, dir)
	return err == nil && ok
}

// printBackup prints where each original was saved, with the stamp cut so
// the line names the file only.
func printBackup(p printer, b agentio.Backup) {
	if b.IsEmpty() {
		p.line("backup: none (nothing existed to save)")
		return
	}
	for _, e := range b.Entries {
		stamp := e.Copy[strings.Index(e.Copy, bakMarker)+len(bakMarker):]
		p.line(fmt.Sprintf("backup: %s saved as <file>%s%s", e.Original, bakMarker, stamp))
	}
}
