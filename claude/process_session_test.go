package claude

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// Fixture facts (sessions/*.json in every fixture version).
const (
	fixturePID4243        = 4243
	fixtureSession4243    = "3e03d92d-0f60-fb93-80b9-9524ba57cf17"
	fixtureWorkPID        = 4342
	fixtureWorkSession    = "78d3d87c-4440-06f3-ceba-625a49fad9ec"
	fixtureArgvTranscript = "0b5e3a8e-0000-4000-8000-000000000099" // .claude-work/projects/-work-proj99 in 2.1.327
)

// procAgent builds an agent over a copy of fixture with procs as the
// process table; envs nil makes every start env unreadable (windows).
func procAgent(t *testing.T, fixture string, procs *agenttest.FakeProcesses, opts ...agenttest.HostOption) (*Agent, string) {
	t.Helper()
	base := []agenttest.HostOption{agenttest.WithProcesses(procs)}
	host := agenttest.NewHost(t, append(base, opts...)...)
	agenttest.CopyFixture(t, fixture, host.UserHome)
	return New(host), host.UserHome
}

// TestProcessSessionExact joins a row to sessions/<pid>.json in the home
// its env names, with every recorded field, and skips a file left by an
// earlier process that had the same pid.
func TestProcessSessionExact(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		started := millis(1794563211895).Time()
		row := agentio.Process{PID: fixturePID4243, Start: started.Add(3 * time.Second), Argv: []string{"claude"}}
		procs := &agenttest.FakeProcesses{List: []agentio.Process{row}, Envs: map[int][]string{fixturePID4243: {"PATH=/bin"}}}
		a, _ := procAgent(t, fixture, procs)
		s, ok, err := a.ProcessSession(context.Background(), row)
		if err != nil || !ok {
			t.Fatalf("exact: %v %v", ok, err)
		}
		l, _ := a.LiveSessions(a.DefaultHome())
		want := l[slices.IndexFunc(l, func(x LiveSession) bool { return x.PID == fixturePID4243 })]
		if s.Confidence != agentio.JoinExact || s.ConversationID != fixtureSession4243 || !s.Home.IsDefault() || s.Dir != "/work/proj6" ||
			s.Version != want.Version || s.Kind != agentio.SessionInteractive || s.Entrypoint != "cli" || s.Status != want.Status.Neutral() ||
			!s.StatusSince.Equal(want.StatusUpdatedAt) || !s.LastActivity.Equal(want.UpdatedAt) || s.PID != fixturePID4243 || s.Agent != ID {
			t.Fatalf("exact session = %+v", s)
		}
		// The same pid, started two days later: the file is a leftover.
		reused := row
		reused.Start = started.Add(48 * time.Hour)
		s, ok, err = a.ProcessSession(context.Background(), reused)
		if err != nil || !ok || s.Confidence != agentio.JoinUnknown || s.ConversationID != "" || !s.Home.IsDefault() {
			t.Fatalf("pid reuse: %+v %v %v", s, ok, err)
		}
		// No start time on the row: the file is read but is no kill evidence.
		s, _, _ = a.ProcessSession(context.Background(), agentio.Process{PID: fixturePID4243, Argv: []string{"claude"}})
		if s.Confidence != agentio.JoinInferred || s.ConversationID != fixtureSession4243 {
			t.Fatalf("no start time: %+v", s)
		}
	})
}

// TestProcessSessionWaiting maps a waiting status onto SessionWaiting.
func TestProcessSessionWaiting(t *testing.T) {
	fixture := filepath.Join("testdata", "homes", "2.1.327", "userhome")
	row := agentio.Process{PID: fixturePID4243, Start: millis(1794563211895).Time(), Argv: []string{"claude"}}
	a, _ := procAgent(t, fixture, &agenttest.FakeProcesses{Envs: map[int][]string{fixturePID4243: {"PATH=/bin"}}})
	s, ok, err := a.ProcessSession(context.Background(), row)
	if err != nil || !ok || s.Status != agentio.SessionWaiting {
		t.Fatalf("waiting = %+v %v %v", s, ok, err)
	}
}

// TestProcessSessionHomes reads only the env-named home when the env is
// readable, and every discovered home when it is not.
func TestProcessSessionHomes(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		row := agentio.Process{PID: fixtureWorkPID, Start: millis(1794544889729).Time(), Argv: []string{"claude"}}
		ctx := context.Background()
		// Env readable, no CLAUDE_CONFIG_DIR: the default home has no 4342.json.
		a, _ := procAgent(t, fixture, &agenttest.FakeProcesses{Envs: map[int][]string{fixtureWorkPID: {"PATH=/bin"}}})
		if s, _, _ := a.ProcessSession(ctx, row); s.Confidence == agentio.JoinExact {
			t.Fatalf("default-home process joined another home's file: %+v", s)
		}
		// Env names the work home.
		a, home := procAgent(t, fixture, &agenttest.FakeProcesses{})
		procs := a.host.Processes.(*agenttest.FakeProcesses)
		procs.Envs = map[int][]string{fixtureWorkPID: {ConfigDirEnv + "=" + filepath.Join(home, ".claude-work")}}
		s, ok, err := a.ProcessSession(ctx, row)
		if err != nil || !ok || s.Confidence != agentio.JoinExact || s.ConversationID != fixtureWorkSession || s.Home.Name() != "work" {
			t.Fatalf("work home = %+v %v %v", s, ok, err)
		}
		// Env unreadable: every discovered home is tried.
		a, _ = procAgent(t, fixture, &agenttest.FakeProcesses{})
		s, ok, err = a.ProcessSession(ctx, row)
		if err != nil || !ok || s.Confidence != agentio.JoinExact || s.ConversationID != fixtureWorkSession {
			t.Fatalf("unreadable env = %+v %v %v", s, ok, err)
		}
	})
}

// TestProcessSessionArgv falls back to a UUID after --resume, finds the
// home holding its transcript when the env is unreadable, and ignores a
// search term.
func TestProcessSessionArgv(t *testing.T) {
	fixture := filepath.Join("testdata", "homes", "2.1.327", "userhome")
	ctx := context.Background()
	a, _ := procAgent(t, fixture, &agenttest.FakeProcesses{Cwds: map[int]string{5000: "/work/proj99"}})
	for _, argv := range [][]string{
		{"claude", "--resume", fixtureArgvTranscript, "--dangerously-skip-permissions"},
		{"claude", "-r", fixtureArgvTranscript},
		{"claude", "--resume=" + fixtureArgvTranscript},
	} {
		s, ok, err := a.ProcessSession(ctx, agentio.Process{PID: 5000, Argv: argv})
		if err != nil || !ok || s.Confidence != agentio.JoinArgv || s.ConversationID != fixtureArgvTranscript || s.Home.Name() != "work" ||
			!strings.HasSuffix(s.Path, fixtureArgvTranscript+transcriptExt) || s.LastActivity.IsZero() || s.Dir != "/work/proj99" {
			t.Fatalf("%q = %+v %v %v", argv, s, ok, err)
		}
	}
	// A transcript no home holds: the default home.
	other := "11111111-2222-4333-8444-555555555555"
	s, ok, _ := a.ProcessSession(ctx, agentio.Process{PID: 5001, Argv: []string{"claude", "--resume", other}})
	if !ok || !s.Home.IsDefault() || s.Path != "" || s.ConversationID != other {
		t.Fatalf("unknown transcript = %+v", s)
	}
	// A search term is not an id; with an unreadable env nothing ties the row.
	if s, ok, _ := a.ProcessSession(ctx, agentio.Process{PID: 5002, Argv: []string{"claude", "--resume", "fix the bug"}}); ok {
		t.Fatalf("search term joined: %+v", s)
	}
	// Not a session process.
	if _, ok, _ := a.ProcessSession(ctx, agentio.Process{PID: 5003, Argv: []string{"claude", "mcp", "serve"}}); ok {
		t.Fatal("mcp serve has no session")
	}
}

// TestProcessSessionTranscriptPath fills Path when the transcript exists.
func TestProcessSessionTranscriptPath(t *testing.T) {
	fixture := filepath.Join("testdata", "homes", "2.1.327", "userhome")
	row := agentio.Process{PID: fixturePID4243, Start: millis(1794563211895).Time(), Argv: []string{"claude"}}
	a, _ := procAgent(t, fixture, &agenttest.FakeProcesses{Envs: map[int][]string{fixturePID4243: {"PATH=/bin"}}})
	path := filepath.Join(a.HistoryDir(a.DefaultHome(), "/work/proj6"), fixtureSession4243+transcriptExt)
	agenttest.WriteFile(t, filepath.Dir(path), filepath.Base(path), "{}\n")
	s, _, _ := a.ProcessSession(context.Background(), row)
	if s.Path != path {
		t.Fatalf("path = %q, want %q", s.Path, path)
	}
}

// TestClaudeStopPlan refuses mcp serve, plans SIGTERM with a resume for a
// session, `claude stop` for a background session, and no graceful
// signal on windows.
func TestClaudeStopPlan(t *testing.T) {
	fixture := filepath.Join("testdata", "homes", "2.1.327", "userhome")
	a, home := procAgent(t, fixture, &agenttest.FakeProcesses{})
	row := agentio.Process{PID: 42, Argv: []string{"claude"}}
	plan, err := a.StopPlan(agentio.Process{PID: 8, Argv: []string{"claude", "mcp", "serve"}}, nil, agentio.ProcessSession{})
	if err != nil || plan.Refuse != refuseMCPServe || len(plan.Targets) != 0 {
		t.Fatalf("mcp serve plan = %+v %v", plan, err)
	}
	s := agentio.ProcessSession{Agent: ID, PID: 42, Home: a.DefaultHome().Neutral(), ConversationID: fixtureSession4243, Dir: "/work/proj6", Kind: agentio.SessionInteractive}
	plan, err = a.StopPlan(row, nil, s)
	if err != nil || !slices.Equal(plan.Targets, []int{42}) || plan.Graceful != agentio.StopSignalTerminate || plan.Grace != stopGrace || !plan.Force || plan.Command != nil {
		t.Fatalf("session plan = %+v %v", plan, err)
	}
	if plan.Resume == nil || !slices.Equal(plan.Resume.Args, []string{flagResume, fixtureSession4243}) || plan.Resume.Dir != "/work/proj6" {
		t.Fatalf("resume = %+v", plan.Resume)
	}
	work := a.HomeAt(filepath.Join(home, ".claude-work"))
	s.Kind, s.Home = agentio.SessionBackground, work.Neutral()
	plan, _ = a.StopPlan(row, nil, s)
	if plan.Command == nil || !slices.Equal(plan.Command.Args, []string{subStop, fixtureSession4243}) || plan.Force {
		t.Fatalf("background plan = %+v", plan)
	}
	if v, ok := agentio.EnvLookup(plan.Command.Env, ConfigDirEnv); !ok || v != work.Dir() {
		t.Fatalf("stop command env = %q", plan.Command.Env)
	}
	if _, err := a.StopPlan(agentio.Process{PID: 1, Argv: []string{"vim"}}, nil, s); !errors.Is(err, agentio.ErrNotFound) {
		t.Fatalf("foreign row: %v", err)
	}
	win, _ := procAgent(t, fixture, &agenttest.FakeProcesses{}, agenttest.WithSystem(agentio.OSWindows, ""))
	plan, _ = win.StopPlan(row, nil, agentio.ProcessSession{})
	if plan.Graceful != agentio.StopSignalNone || !plan.Force {
		t.Fatalf("windows plan = %+v", plan)
	}
	// The neutral view exposes both capabilities.
	n := a.Neutral()
	if _, ok := agentio.ProcessSessionsOf(n); !ok {
		t.Fatal("no ProcessSessionReader")
	}
	if _, ok := agentio.StopPlannerOf(n); !ok {
		t.Fatal("no StopPlanner")
	}
}
