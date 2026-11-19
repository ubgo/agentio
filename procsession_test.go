package agentio_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// pids returns the PIDs of rows in order. Pure.
func pids(rows []agentio.Process) []int {
	out := make([]int, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.PID)
	}
	return out
}

// TestAncestors walks the parent chain nearest first and stops at a
// missing pid, at init, at the hop limit and at a cycle.
func TestAncestors(t *testing.T) {
	t.Parallel()
	table := []agentio.Process{
		{PID: 1, PPID: 0}, {PID: 10, PPID: 1}, {PID: 20, PPID: 10}, {PID: 30, PPID: 20}, {PID: 40, PPID: 30},
		{PID: 50, PPID: 999},                     // parent missing from the table
		{PID: 60, PPID: 70}, {PID: 70, PPID: 60}, // a cycle from a racy read
	}
	for _, tc := range []struct {
		name      string
		pid, hops int
		want      []int
	}{
		{"whole chain stops below init", 40, 0, []int{30, 20, 10}},
		{"hop limit", 40, 2, []int{30, 20}},
		{"missing parent", 50, 0, nil},
		{"unknown pid", 12345, 0, nil},
		{"cycle", 60, 0, []int{70}},
	} {
		if got := pids(agentio.Ancestors(table, tc.pid, tc.hops)); !slices.Equal(got, tc.want) {
			t.Errorf("%s: Ancestors(%d, %d) = %v, want %v", tc.name, tc.pid, tc.hops, got, tc.want)
		}
	}
}

// TestCommandString prefers the raw line and falls back to joined argv.
func TestCommandString(t *testing.T) {
	t.Parallel()
	if got := (agentio.Process{CommandLine: "a  b", Argv: []string{"a", "b"}}).CommandString(); got != "a  b" {
		t.Errorf("raw line: %q", got)
	}
	if got := (agentio.Process{Argv: []string{"a", "b"}}).CommandString(); got != "a b" {
		t.Errorf("joined argv: %q", got)
	}
}

// TestProcessMatchScripts covers a script run through an interpreter, the
// raw-line exclusion darwin's space-split argv cannot see, and the rows
// that must not match.
func TestProcessMatchScripts(t *testing.T) {
	t.Parallel()
	m := agentio.ProcessMatch{
		Basenames:       []string{"toy"},
		ScriptBasenames: []string{"toy"},
		Exclude:         []string{"Application Support/Toy"},
	}
	for _, tc := range []struct {
		name string
		p    agentio.Process
		want bool
	}{
		{"node wrapped", agentio.Process{Argv: []string{"/usr/bin/node", "/opt/bin/toy", "--foo"}}, true},
		{"node flags before the script", agentio.Process{Argv: []string{"node", "--max-old-space-size=8192", "/opt/bin/toy"}}, true},
		{"bun", agentio.Process{Argv: []string{"bun", "/x/toy"}}, true},
		{"interpreter from exe", agentio.Process{Exe: "/usr/local/bin/node", Argv: []string{"weird-title", "/x/toy"}}, true},
		{"node runs another script", agentio.Process{Argv: []string{"node", "/srv/server.js", "toy"}}, false},
		{"not an interpreter", agentio.Process{Argv: []string{"python3", "/x/toy"}}, false},
		{"interpreter alone", agentio.Process{Argv: []string{"node"}}, false},
		{"only flags", agentio.Process{Argv: []string{"node", "--inspect"}}, false},
		{"raw-line exclusion", agentio.Process{
			Argv:        []string{"/Users/alice/Library/Application", "Support/Toy/toy"},
			CommandLine: "/Users/alice/Library/Application Support/Toy/toy",
			Exe:         "toy",
		}, false},
	} {
		if got := m.Match(tc.p); got != tc.want {
			t.Errorf("%s: Match = %v, want %v", tc.name, got, tc.want)
		}
	}
	// The interpreter list is the spec's own when set.
	deno := agentio.ProcessMatch{ScriptBasenames: []string{"toy"}, Interpreters: []string{"deno"}}
	if deno.Match(agentio.Process{Argv: []string{"node", "/x/toy"}}) || !deno.Match(agentio.Process{Argv: []string{"deno", "/x/toy"}}) {
		t.Error("Interpreters override DefaultInterpreters")
	}
	if (agentio.ProcessMatch{ScriptBasenames: []string{"toy"}}).IsZero() {
		t.Error("a script-only matcher is not zero")
	}
	// ArgvContains also sees the raw line (a package path with a space).
	pkg := agentio.ProcessMatch{ArgvContains: []string{"my tools/@toy"}}
	if !pkg.Match(agentio.Process{Argv: []string{"node", "/opt/my", "tools/@toy/cli.js"}, CommandLine: "node /opt/my tools/@toy/cli.js"}) {
		t.Error("ArgvContains on the raw line")
	}
}

// readerAgent is a config agent with a canned ProcessSessionReader.
type readerAgent struct {
	*agentio.Core
	got     *[]int
	session agentio.ProcessSession
}

// ProcessSession records the pid and returns the canned session. Pure.
func (r readerAgent) ProcessSession(_ context.Context, p agentio.Process) (agentio.ProcessSession, bool, error) {
	*r.got = append(*r.got, p.PID)
	s := r.session
	s.PID = p.PID
	return s, true, nil
}

// TestFindProcessSession asks only the agent that classifies the row, and
// only for a session kind.
func TestFindProcessSession(t *testing.T) {
	t.Parallel()
	host := agenttest.NewHost(t)
	plain, err := agentio.FromSpec(sampleSpec(), host)
	if err != nil {
		t.Fatal(err)
	}
	other := sampleSpec()
	other.ID, other.Binary, other.Process = "other", "other", agentio.ProcessMatch{Basenames: []string{"other"}}
	core, err := agentio.NewCore(other, host)
	if err != nil {
		t.Fatal(err)
	}
	var asked []int
	withReader := readerAgent{Core: core, got: &asked, session: agentio.ProcessSession{Agent: "other", ConversationID: "c1", Confidence: agentio.JoinArgv}}
	agents := []agentio.Agent{plain, withReader}
	ctx := context.Background()

	s, ok, err := agentio.FindProcessSession(ctx, agents, agentio.Process{PID: 7, Argv: []string{"other"}})
	if err != nil || !ok || s.ConversationID != "c1" || s.PID != 7 || s.Confidence != agentio.JoinArgv {
		t.Fatalf("reader agent: %+v %v %v", s, ok, err)
	}
	if _, ok, _ := agentio.FindProcessSession(ctx, agents, agentio.Process{PID: 8, Argv: []string{"toy"}}); ok {
		t.Error("an agent without a reader finds nothing")
	}
	if _, ok, _ := agentio.FindProcessSession(ctx, agents, agentio.Process{PID: 9, Argv: []string{"vim"}}); ok {
		t.Error("an unclaimed row finds nothing")
	}
	if !slices.Equal(asked, []int{7}) {
		t.Errorf("reader asked for %v, want only [7]", asked)
	}
}

// TestArgValue reads both flag spellings and refuses a dangling flag.
func TestArgValue(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		argv []string
		want string
		ok   bool
	}{
		{[]string{"claude", "--resume", "abc"}, "abc", true},
		{[]string{"claude", "-r", "abc"}, "abc", true},
		{[]string{"claude", "--resume=abc"}, "abc", true},
		{[]string{"claude", "--resume"}, "", false},
		{[]string{"--resume", "abc"}, "", false}, // argv[0] is the program
		{[]string{"claude", "--resumex", "abc"}, "", false},
	} {
		got, ok := agentio.ArgValue(tc.argv, "--resume", "-r")
		if got != tc.want || ok != tc.ok {
			t.Errorf("ArgValue(%q) = %q %v, want %q %v", tc.argv, got, ok, tc.want, tc.ok)
		}
	}
}

// TestProcessHome reads the home variable from the process env, falls
// back to the default home when the variable is absent, and reports an
// unreadable env as an error, never as the default.
func TestProcessHome(t *testing.T) {
	t.Parallel()
	procs := &agenttest.FakeProcesses{Envs: map[int][]string{
		1: {"PATH=/bin", "TOY_HOME=/work/toy-home"},
		2: {"PATH=/bin"},
		3: {"TOY_HOME="},
	}}
	host := agenttest.NewHost(t, agenttest.WithProcesses(procs))
	c, err := agentio.NewCore(sampleSpec(), host)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if h, err := c.ProcessHome(ctx, 1); err != nil || h.Dir() != filepath.Clean("/work/toy-home") || h.IsDefault() {
		t.Errorf("env home = %v %v", h, err)
	}
	for _, pid := range []int{2, 3} {
		if h, err := c.ProcessHome(ctx, pid); err != nil || !h.IsDefault() {
			t.Errorf("pid %d: default home = %v %v", pid, h, err)
		}
	}
	if h, err := c.ProcessHome(ctx, 4); err == nil || !h.IsZero() {
		t.Errorf("missing env: %v %v", h, err)
	}
	unreadable := agenttest.NewHost(t, agenttest.WithProcesses(&agenttest.FakeProcesses{}))
	c2, _ := agentio.NewCore(sampleSpec(), unreadable)
	if _, err := c2.ProcessHome(ctx, 1); !errors.Is(err, agentio.ErrUnsupported) {
		t.Errorf("unreadable env: %v", err)
	}
	// A spec whose variable names the parent (Gemini) appends the default dir.
	parent := sampleSpec()
	parent.HomeEnvNamesParent = true
	c3, _ := agentio.NewCore(parent, host)
	if h, err := c3.ProcessHome(ctx, 1); err != nil || h.Dir() != filepath.Join(filepath.Clean("/work/toy-home"), ".toy") {
		t.Errorf("parent-naming env home = %v %v", h, err)
	}
}

// TestBaseStopPlan refuses a non-session kind, plans a single SIGTERM
// target with a resume for a known conversation, and on a windows host
// asks to force without a graceful signal.
func TestBaseStopPlan(t *testing.T) {
	t.Parallel()
	toy, host := newToy(t)
	c := toy.(interface {
		BaseStopPlan(agentio.ProcessInfo, agentio.Process, agentio.ProcessSession) agentio.StopPlan
	})
	p := agentio.Process{PID: 42, Argv: []string{"toy"}}
	refused := c.BaseStopPlan(agentio.ProcessInfo{Agent: "toy", Kind: agentio.ProcessMCPServer}, p, agentio.ProcessSession{})
	if refused.Refuse == "" || len(refused.Targets) != 0 {
		t.Errorf("non-session plan = %+v", refused)
	}
	info := agentio.ProcessInfo{Agent: "toy", Kind: agentio.ProcessCLI, Session: true}
	bare := c.BaseStopPlan(info, p, agentio.ProcessSession{})
	if !slices.Equal(bare.Targets, []int{42}) || bare.Graceful != agentio.StopSignalTerminate || bare.Grace != agentio.DefaultStopGrace || !bare.Force || !bare.KeepsConversation || bare.Resume != nil {
		t.Errorf("bare plan = %+v", bare)
	}
	s := agentio.ProcessSession{Agent: "toy", PID: 42, ConversationID: "abc", Home: toy.DefaultHome(), Dir: host.UserHome}
	withResume := c.BaseStopPlan(info, p, s)
	if withResume.Resume == nil || !slices.Contains(withResume.Resume.Args, "abc") || withResume.Resume.Dir != host.UserHome {
		t.Errorf("resume = %+v", withResume.Resume)
	}
	// Another agent's home never builds a resume.
	s.Home = agentio.NewHome("other", host.UserHome, true, "personal")
	if c.BaseStopPlan(info, p, s).Resume != nil {
		t.Error("resume for a foreign home")
	}
	win, _ := newToy(t, agenttest.WithSystem(agentio.OSWindows, ""))
	wp := win.(interface {
		BaseStopPlan(agentio.ProcessInfo, agentio.Process, agentio.ProcessSession) agentio.StopPlan
	}).BaseStopPlan(info, p, agentio.ProcessSession{})
	if wp.Graceful != agentio.StopSignalNone || !wp.Force || wp.Grace != agentio.DefaultStopGrace {
		t.Errorf("windows plan = %+v", wp)
	}
	// StopPlan refuses a row the agent does not classify.
	sp, ok := agentio.StopPlannerOf(toy)
	if !ok {
		t.Fatal("config agent with a process match has a StopPlanner")
	}
	if _, err := sp.StopPlan(agentio.Process{PID: 1, Argv: []string{"vim"}}, nil, agentio.ProcessSession{}); !errors.Is(err, agentio.ErrNotFound) {
		t.Errorf("foreign row: %v", err)
	}
	if plan, err := sp.StopPlan(p, nil, agentio.ProcessSession{}); err != nil || !slices.Equal(plan.Targets, []int{42}) {
		t.Errorf("StopPlan = %+v %v", plan, err)
	}
}
