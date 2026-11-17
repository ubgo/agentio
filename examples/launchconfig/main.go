// Command launchconfig shows the typed launch configuration a launcher
// such as agx builds on: the launch option registry (validate
// `--option key=value` and a profile's typed block), Claude settings that
// round-trip unknown keys and go to a 0600 per-launch file, a live
// credential plan (apiKeyHelper), Codex `-c` overrides checked against
// the config key table, and run-file cleanup with Host.ProcessAlive.
// Everything is pure or writes into a temp dir; no agent runs.
//
//	cd examples && go run ./launchconfig
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
	"github.com/ubgo/agentio/codex"
)

// installedClaude is the version the launch is built for: flags are
// checked against it (features.tsv).
const installedClaude = "2.1.325"

// runID names this launch's files, as agx's run folder id would.
const runID = "20261114-120000-ab12"

func main() {
	if err := runMain(os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "launchconfig:", err)
		os.Exit(1)
	}
}

// runMain runs every step with a temp dir for the run files.
// Disk · Write (temp dir).
func runMain(w io.Writer) error {
	tmp, err := os.MkdirTemp("", "agentio-launchconfig-")
	if err != nil {
		return err
	}
	// The temp dir holds only this run's files; removing it is cleanup.
	defer func() { _ = os.RemoveAll(tmp) }()
	return run(w, tmp)
}

// runDirToken stands for the run dir inside argv before the command is
// rendered: it is shell-safe, so Command.String does not quote it.
const runDirToken = "RUNDIR"

// displayCommand renders cmd with the run dir shown as "<run dir>/" on
// every OS. Command.String quotes for a POSIX shell, so a Windows path
// (backslashes) comes back quoted and escaped and a plain text replace of
// the raw dir no longer matches; the dir is replaced in each argument
// first, with a separator-neutral token, then rendered. Pure.
func displayCommand(cmd agentio.Command, runDir string) string {
	return displayCommandSep(cmd, runDir, string(os.PathSeparator))
}

// displayCommandSep is displayCommand with an explicit path separator, so
// a test can render a Windows path on any OS. Pure.
func displayCommandSep(cmd agentio.Command, runDir, sep string) string {
	c := cmd
	c.Args = make([]string, len(cmd.Args))
	for i, a := range cmd.Args {
		a = strings.ReplaceAll(a, runDir+sep, runDirToken+"/")
		c.Args[i] = strings.ReplaceAll(a, runDir, runDirToken)
	}
	return strings.ReplaceAll(c.String(), runDirToken, "<run dir>")
}

// run prints each step; runDir receives the per-launch settings file.
func run(w io.Writer, runDir string) error {
	h := agentio.Host{UserHome: runDir, User: "user"}
	a := claude.New(h)
	fmt.Fprintln(w, "== claude options ==")
	o, err := claudeOptions(w)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "== claude settings ==")
	settingsArg, err := claudeSettings(w, runDir)
	if err != nil {
		return err
	}
	o.Settings = settingsArg
	t := a.DefaultHome().Target()
	t.Version = agentio.MustVersion(installedClaude)
	cmd, err := a.Launch(t, o)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, "command:", displayCommand(cmd, runDir))
	fmt.Fprintln(w, "== live credential ==")
	if err := liveCredential(w, a.Neutral()); err != nil {
		return err
	}
	fmt.Fprintln(w, "== codex options and overrides ==")
	if err := codexConfig(w); err != nil {
		return err
	}
	fmt.Fprintln(w, "== run files ==")
	return runFiles(w)
}

// snippet:start options
// claudeOptions validates a profile's typed block and one --option on top
// of it through the registry, then shows what three keys map to.
func claudeOptions(w io.Writer) (claude.LaunchOptions, error) {
	keys, err := claude.LaunchOptionKeys()
	if err != nil {
		return claude.LaunchOptions{}, err
	}
	fmt.Fprintf(w, "%d option keys\n", len(keys))
	for _, name := range []string{"effort", "mcp_configs", "append_system_prompt_file"} {
		k, err := agentio.FindOptionKey(keys, name)
		if err != nil {
			return claude.LaunchOptions{}, err
		}
		fmt.Fprintf(w, "  %-26s %-8s flag=%s since=%s values=%v\n", k.Key, k.Kind, k.Flag, orDash(k.Gate.Since), k.Values)
	}
	profile := []byte(`{"effort": "high", "append_system_prompt_file": "~/prompts/work.md", "mcp_configs": ["~/mcp/github.json"]}`)
	o, err := claude.DecodeLaunchOptions(profile)
	if err != nil {
		return o, err
	}
	if err := claude.ParseLaunchOption(&o, "effort", "max"); err != nil {
		return o, err
	}
	fmt.Fprintf(w, "effort=%s (the --option beat the profile)\n", o.Effort)
	_, err = claude.DecodeLaunchOptions([]byte(`{"effort": "high", "codex_profile": "deep"}`))
	fmt.Fprintln(w, "refused:", firstClause(err))
	return o, nil
}

// snippet:end options

// snippet:start settings
// claudeSettings type-checks merged settings (unknown keys kept, listed
// for a warning) and writes them to this run's 0600 file for --settings.
func claudeSettings(w io.Writer, runDir string) (string, error) {
	merged := []byte(`{"model": "opus", "env": {"CLAUDE_CODE_API_KEY_HELPER_TTL_MS": "10000"}, "spinnerTipsEnabled": false, "permissions": {"allow": ["Bash(ls:*)"], "futureRule": true}}`)
	check, err := claude.ValidateSettings(merged)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(w, "ok=%t unknown keys (kept, warned): %v\n", check.OK(), check.UnknownKeys)
	bad, err := claude.ValidateSettings([]byte(`{"cleanupPeriodDays": "thirty"}`))
	if err != nil {
		return "", err
	}
	fmt.Fprintf(w, "type error: %s %s\n", bad.TypeErrors[0].Key, bad.TypeErrors[0].Detail)
	path, err := claude.LaunchSettings(merged, claude.SettingsViaFile, runDir, runID)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(w, "settings file: <run dir>/%s mode=%v\n", strings.TrimPrefix(path, runDir+string(os.PathSeparator)), fi.Mode().Perm())
	return path, nil
}

// snippet:end settings

// snippet:start live
// liveCredential plans a switchable session: Claude re-runs the helper
// command every Refresh, after the variables that outrank it are removed.
func liveCredential(w io.Writer, a agentio.Agent) error {
	lc, ok := agentio.LiveCredentialOf(a)
	if !ok {
		reason, _ := agentio.Unsupported(a, agentio.CapLiveCredential)
		fmt.Fprintln(w, "not switchable:", reason)
		return nil
	}
	plan, err := lc.LiveCredential("/usr/local/bin/agx credential", 0)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "settings %s\nenv %v\nscrub %v\nrefresh %s\n", plan.Settings, plan.Env, plan.Scrub, plan.Refresh)
	return nil
}

// snippet:end live

// snippet:start codex
// codexConfig validates a Codex typed block and checks -c overrides
// against the config key table through the real config decoder.
func codexConfig(w io.Writer) error {
	o, err := codex.DecodeLaunchOptions([]byte(`{"codex_profile": "deep", "search": true}`))
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "codex profile=%s search=%t\n", o.Profile, o.Search)
	for _, ov := range []codex.Override{
		codex.OverrideString("model_reasoning_effort", "high"),
		codex.OverrideString("model_reasoning_effort", "turbo"),
		codex.OverrideBool("sandbox_workspace_write.network_access", true),
		codex.OverrideBool("model", true),
		codex.OverrideString("tui.theme", "dark"),
		codex.OverrideString("brand_new_key", "x"),
	} {
		c, err := codex.CheckOverride(ov)
		if err != nil {
			return err
		}
		fmt.Fprintln(w, strings.TrimRight(fmt.Sprintf("  -c %-46s %-14s %s", ov, c.Status, c.Detail), " "))
	}
	return nil
}

// snippet:end codex

// snippet:start alive
// runFiles decides which run folders agx clean may delete: the recorded
// pid, start time and AGX_RUN_ID must still match a live process.
func runFiles(w io.Writer) error {
	started := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	h := agentio.Host{Processes: fixtureProcesses{
		{agentio.Process{PID: 4242, Start: started}, []string{"AGX_RUN_ID=" + runID}},
		{agentio.Process{PID: 5151, Start: started}, []string{"AGX_RUN_ID=someone-else"}},
	}}
	for _, pid := range []int{4242, 5151, 6060} {
		state, err := h.ProcessAlive(context.Background(), pid, started, "AGX_RUN_ID", runID)
		fmt.Fprintf(w, "pid %d: %s (err=%v)\n", pid, state, err)
	}
	return nil
}

// snippet:end alive

// fixtureProcess is one process with its start environment: what
// HostFromOS reads from /proc (linux) or ps (darwin).
type fixtureProcess struct {
	proc agentio.Process
	env  []string
}

// fixtureProcesses is a fixed process table.
type fixtureProcesses []fixtureProcess

// Processes lists the table. Pure.
func (f fixtureProcesses) Processes(context.Context) ([]agentio.Process, error) {
	out := make([]agentio.Process, len(f))
	for i, p := range f {
		out[i] = p.proc
	}
	return out, nil
}

// Cwd is unknown for every pid. Pure.
func (f fixtureProcesses) Cwd(context.Context, int) (string, error) { return "", os.ErrNotExist }

// Env returns pid's start environment. Pure.
func (f fixtureProcesses) Env(_ context.Context, pid int) ([]string, error) {
	for _, p := range f {
		if p.proc.PID == pid {
			return p.env, nil
		}
	}
	return nil, os.ErrNotExist
}

// orDash prints "-" for an unknown version.
func orDash(v agentio.Version) string {
	if v.IsZero() {
		return "-"
	}
	return v.String()
}

// firstClause keeps an error up to its valid-keys list, for one line.
func firstClause(err error) string {
	s, _, _ := strings.Cut(fmt.Sprint(err), " (valid:")
	return s
}
