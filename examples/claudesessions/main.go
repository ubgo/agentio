// Command claudesessions shows what agentio knows about Claude Code work
// that is running now: the sessions/<pid>.json registry checked against the
// process table, Claude processes found in the process table, background
// jobs and their supervisor daemon, agent teams and task lists.
//
// The process table is an in-memory fake. The registry comes from the
// checked-in 2.1.325 fixture home; jobs, daemon, team and task files are
// written in their recorded shapes into a temp dir used as a second home.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/claude"
)

// defaultFixture is the fixture user home, relative to the examples module.
const defaultFixture = "../claude/testdata/homes/2.1.325/userhome"

// Fixture facts: registry entries of the default home.
const (
	busyPID        = 4242
	reusedPID      = 4243
	busySessionID  = "6d50c0ec-647e-18aa-84c7-04170288be89"
	busySessionCwd = "/work/proj4"
	mcpServePID    = 5001
	otherPID       = 5002
	jobID          = "ab12"
)

// busyStartedAt is sessions/4242.json startedAt; the live process started
// three seconds earlier (Claude records startedAt just after exec).
func busyStartedAt() time.Time { return time.UnixMilli(1794597025793).UTC() }

// startLag is how long before startedAt the busy process started.
const startLag = 3 * time.Second

// reuseGap is how much later the process now holding pid 4243 started: a
// different process that reused the pid.
const reuseGap = 48 * time.Hour

// File modes for the temp home.
const (
	dirMode  = 0o755
	fileMode = 0o644
)

// fixtureNow is the fixed clock.
func fixtureNow() time.Time { return time.Date(2026, 11, 13, 20, 0, 0, 0, time.UTC) }

func main() {
	fixture := flag.String("fixture", defaultFixture, "Claude fixture user home to read")
	flag.Parse()
	tmp, err := os.MkdirTemp("", "claudesessions-")
	if err == nil {
		err = run(context.Background(), os.Stdout, *fixture, tmp)
		// Best-effort cleanup of our own temp dir: run's error is the one
		// worth reporting, and a leftover dir under $TMPDIR is harmless.
		_ = os.RemoveAll(tmp)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "claudesessions:", err)
		os.Exit(1)
	}
}

// errNoCwd is returned for a pid the fake table has no cwd for.
var errNoCwd = errors.New("no cwd")

// fakeProcesses is a fixed process table with known working directories.
type fakeProcesses struct {
	list []agentio.Process
	cwds map[int]string
}

// Processes returns the fixed table. Pure.
func (f fakeProcesses) Processes(context.Context) ([]agentio.Process, error) { return f.list, nil }

// Cwd returns the recorded cwd of pid. Pure.
func (f fakeProcesses) Cwd(_ context.Context, pid int) (string, error) {
	if d, ok := f.cwds[pid]; ok {
		return d, nil
	}
	return "", errNoCwd
}

// processTable is what the fake ps shows: the busy session's process, a
// process that reused pid 4243 two days later, a `claude mcp serve` and an
// unrelated program.
func processTable() fakeProcesses {
	return fakeProcesses{
		list: []agentio.Process{
			{PID: busyPID, PPID: 1, Start: busyStartedAt().Add(-startLag), Exe: "/home/user/.local/share/claude/versions/2.1.325", Argv: []string{"claude"}},
			{PID: reusedPID, PPID: 1, Start: busyStartedAt().Add(reuseGap), Exe: "/usr/bin/python3", Argv: []string{"python3", "worker.py"}},
			{PID: mcpServePID, PPID: busyPID, Start: busyStartedAt(), Argv: []string{"claude", "mcp", "serve"}},
			{PID: otherPID, PPID: 1, Start: busyStartedAt(), Exe: "/usr/bin/node", Argv: []string{"node", "server.js"}},
		},
		cwds: map[int]string{busyPID: busySessionCwd},
	}
}

// backgroundFiles are jobs, daemon, team and task files in their recorded
// shapes (values are placeholders), relative to a home dir.
func backgroundFiles() map[string]string {
	return map[string]string{
		"jobs/" + jobID + "/state.json":     `{"state":"blocked","tempo":"blocked","detail":"waiting for review","needs":"approval","tokens":230135,"template":"bg","respawnFlags":["--model","opus"],"intent":"status","name":"fix-tests","sessionId":"11111111-0000-4000-8000-000000000001","cliVersion":"2.1.325","cwd":"/work/proj4","bgIsolation":"none","backend":"daemon","children":[{"id":"1","href":"https://github.com/example/project/pull/7","kind":"pr"}],"createdAt":"2026-11-13T18:00:00Z","updatedAt":"2026-11-13T19:00:00Z","firstTerminalAt":"2026-11-13T19:00:00Z"}`,
		"jobs/" + jobID + "/timeline.jsonl": `{"at":"2026-11-13T18:00:00Z","state":"running","detail":"started","text":"working"}` + "\n" + `{"at":"2026-11-13T19:00:00Z","state":"blocked","detail":"waiting for review","text":"opened a PR"}` + "\n",
		"daemon.lock":                       `{"pid":4242,"version":"2.1.325","jsonPath":"/tmp/d.json","logPath":"/tmp/d.log","startedAt":1794597025793,"origin":"cli","spawnedBy":{},"procStart":"330472961","processWrapper":"none"}`,
		"daemon-auth-status.json":           `{"since":1794597025793,"status":"ok"}`,
		"teams/review/config.json":          `{"name":"review","createdAt":1794597025793,"leadAgentId":"lead","leadSessionId":"22222222-0000-4000-8000-000000000002","leadProcess":{"pid":4242,"pidSpace":"host","claimedAt":1794597025793},"members":[{"agentId":"a1","agentType":"reviewer","backendType":"tmux","cwd":"/work/proj4","joinedAt":1794597025793,"name":"reviewer","subscriptions":[],"tmuxPaneId":"%1"}]}`,
		"tasks/review/1.json":               `{"subject":"read the diff"}`,
	}
}

// writeFiles writes files under dir.
func writeFiles(dir string, files map[string]string) error {
	for rel, body := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), dirMode); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(body), fileMode); err != nil {
			return err
		}
	}
	return nil
}

// run prints the live-session views of the fixture home against an
// in-memory process table, then the background-job files it writes under
// tmp (the fixture has none).
func run(ctx context.Context, w io.Writer, fixture, tmp string) error {
	abs, err := filepath.Abs(fixture)
	if err != nil {
		return err
	}
	if err := writeFiles(tmp, backgroundFiles()); err != nil {
		return err
	}
	host := agentio.Host{UserHome: abs, Env: []string{"HOME=" + abs, "PATH="}, Now: fixtureNow, Processes: processTable()}
	a := claude.New(host)
	if err := liveSessions(w, a, a.DefaultHome()); err != nil {
		return err
	}
	if err := processes(ctx, w, a); err != nil {
		return err
	}
	bg := a.HomeAt(tmp)
	if err := background(w, a, bg); err != nil {
		return err
	}
	return teams(w, a, bg)
}

// snippet:start live
// liveSessions reads the sessions/<pid>.json registry and marks each entry
// Alive only when its pid is running and started at the recorded time: a
// pid reused by another program reads as not alive.
func liveSessions(w io.Writer, a *claude.Agent, h claude.Home) error {
	all, err := a.LiveSessions(h)
	if err != nil {
		return err
	}
	for _, s := range all {
		fmt.Fprintf(w, "session pid %d: %s, status %s (neutral %s), alive %t, cwd %s, v%s\n",
			s.PID, s.Name, s.Status, s.Status.Neutral(), s.Alive, s.Cwd, s.Version)
	}
	in, err := a.LiveSessionsIn(h, busySessionCwd)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  sessions in %s: %d\n", busySessionCwd, len(in))
	live, err := a.IsLive(h, busySessionID)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "  conversation %s live: %t\n", busySessionID[:8], live)
	if len(in) > 0 {
		n := in[0].Neutral(h)
		fmt.Fprintf(w, "  neutral: home %s, kind %s, status %s since %s\n", n.Home.Name(), n.Kind, n.Status, n.StatusSince.UTC().Format(time.RFC3339))
	}
	return nil
}

// snippet:end live

// snippet:start processes
// processes classifies the process table: `claude mcp serve` is an MCP
// server, any other claude process is a CLI session; other programs and
// the desktop app are not Claude Code.
func processes(ctx context.Context, w io.Writer, a *claude.Agent) error {
	for _, p := range processTable().list {
		info, ok := claude.ClassifyProcess(p)
		if !ok {
			fmt.Fprintf(w, "pid %d %s: not Claude Code\n", p.PID, strings.Join(p.Argv, " "))
			continue
		}
		fmt.Fprintf(w, "pid %d %s: %s, session %t\n", p.PID, strings.Join(p.Argv, " "), info.Kind, info.Session)
	}
	list, err := a.Processes(ctx)
	if err != nil {
		return err
	}
	for _, lp := range list {
		cwd := lp.Cwd
		if cwd == "" {
			cwd = "(unknown)"
		}
		fmt.Fprintf(w, "  claude process %d: %s, cwd %s\n", lp.PID, lp.Kind, cwd)
	}
	return nil
}

// snippet:end processes

// snippet:start jobs
// background reads background jobs, a job's timeline and the supervisor
// daemon's files, and builds (never runs) the attach, logs and stop
// commands for a job.
func background(w io.Writer, a *claude.Agent, h claude.Home) error {
	jobs, err := a.Jobs(h)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		fmt.Fprintf(w, "job %s %q: state %s, needs %q, %d tokens, %d children, unknown keys %d\n",
			j.ID, j.Name, j.State, j.Needs, j.Tokens, len(j.Children), len(j.Unknown))
		events, err := a.JobTimeline(h, j.ID)
		if err != nil {
			return err
		}
		for _, ev := range events {
			fmt.Fprintf(w, "  %s %s: %s\n", ev.At.UTC().Format(time.RFC3339), ev.State, ev.Text)
		}
		for _, c := range []agentio.Command{a.AttachCommand(j.ID), a.LogsCommand(j.ID), a.StopCommand(j.ID)} {
			fmt.Fprintf(w, "  command: %s\n", c)
		}
	}
	d, err := a.Daemon(h)
	if err != nil {
		return err
	}
	if d.Lock != nil {
		fmt.Fprintf(w, "daemon: pid %d v%s, running %t", d.Lock.PID, d.Lock.Version, d.Running)
		if d.Auth != nil {
			fmt.Fprintf(w, ", auth %s", d.Auth.Status)
		}
		fmt.Fprintln(w)
	}
	return nil
}

// snippet:end jobs

// snippet:start teams
// teams reads agent teams and task lists. Task items have no recorded
// shape yet, so their bodies stay raw.
func teams(w io.Writer, a *claude.Agent, h claude.Home) error {
	ts, err := a.Teams(h)
	if err != nil {
		return err
	}
	for _, t := range ts {
		fmt.Fprintf(w, "team %s: lead %s, %d members", t.Name, t.LeadAgentID, len(t.Members))
		if t.LeadProcess != nil {
			fmt.Fprintf(w, ", lead pid %d", t.LeadProcess.PID)
		}
		fmt.Fprintln(w)
		for _, m := range t.Members {
			fmt.Fprintf(w, "  member %s (%s, %s) in %s\n", m.Name, m.AgentType, m.BackendType, m.Cwd)
		}
	}
	lists, err := a.TaskLists(h)
	if err != nil {
		return err
	}
	for _, l := range lists {
		for _, it := range l.Items {
			fmt.Fprintf(w, "task list %s: item %s, %d raw bytes\n", l.Name, it.Name, len(it.Raw))
		}
	}
	return nil
}

// snippet:end teams
