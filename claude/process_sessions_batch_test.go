package claude

import (
	"context"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// TestProcessSessionsBatch joins a whole table with one environment read,
// no cwd lookup for a row whose Process.Cwd is set, and the same result
// per row as ProcessSession; the row-by-row form reads one environment
// per row.
func TestProcessSessionsBatch(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, home := procAgent(t, fixture, &agenttest.FakeProcesses{})
		procs := a.host.Processes.(*agenttest.FakeProcesses)
		procs.Envs = map[int][]string{
			fixturePID4243: {"PATH=/bin"},
			fixtureWorkPID: {ConfigDirEnv + "=" + filepath.Join(home, ".claude-work")},
		}
		procs.Cwds = map[int]string{5000: "/work/looked-up"}
		table := []agentio.Process{
			{PID: fixturePID4243, Start: millis(1794563211895).Time(), Argv: []string{"claude"}},
			{PID: fixtureWorkPID, Start: millis(1794544889729).Time(), Argv: []string{"claude"}},
			// env unreadable, argv join, cwd already known to the caller
			{PID: 5000, Argv: []string{"claude", "--resume", fixtureArgvTranscript}, Cwd: "/work/proj99"},
			{PID: 5003, Argv: []string{"claude", "mcp", "serve"}},
			{PID: 5004, Argv: []string{"claude", "mcp", "list"}},
			{PID: 5005, Argv: []string{"vim"}},
		}
		ctx := context.Background()
		got, err := a.ProcessSessions(ctx, table)
		if err != nil {
			t.Fatal(err)
		}
		if keys := slices.Sorted(maps.Keys(got)); !slices.Equal(keys, []int{fixturePID4243, fixtureWorkPID, 5000}) {
			t.Fatalf("found pids %v", keys)
		}
		if procs.EnvBatchCalls != 1 || procs.EnvCalls != 0 || procs.CwdCalls != 0 || procs.TableReads != 0 {
			t.Errorf("host calls: %d env batch, %d env, %d cwd, %d table; want 1 env batch only",
				procs.EnvBatchCalls, procs.EnvCalls, procs.CwdCalls, procs.TableReads)
		}
		if s := got[fixtureWorkPID]; s.Confidence != agentio.JoinExact || s.ConversationID != fixtureWorkSession || s.Home.Name() != "work" {
			t.Errorf("work row = %+v", s)
		}
		if s := got[5000]; s.Confidence != agentio.JoinArgv || s.Dir != "/work/proj99" || s.Home.Name() != "work" {
			t.Errorf("argv row = %+v", s)
		}
		// Row by row: the same sessions, one environment read per session row.
		procs.EnvBatchCalls, procs.EnvCalls = 0, 0
		for _, p := range table {
			s, ok, err := a.ProcessSession(ctx, p)
			want, inBatch := got[p.PID]
			if err != nil || ok != inBatch || (ok && !reflect.DeepEqual(s, want)) {
				t.Errorf("pid %d: single %+v %v %v, batch %+v", p.PID, s, ok, err, want)
			}
		}
		if procs.EnvCalls != 3 || procs.EnvBatchCalls != 0 || procs.CwdCalls != 0 {
			t.Errorf("row-by-row host calls: %d env, %d env batch, %d cwd; want 3 env", procs.EnvCalls, procs.EnvBatchCalls, procs.CwdCalls)
		}
		// Without Process.Cwd the argv row looks the cwd up once.
		bare := table[2]
		bare.Cwd = ""
		if s, _, _ := a.ProcessSession(ctx, bare); s.Dir != "/work/looked-up" || procs.CwdCalls != 1 {
			t.Errorf("cwd lookup: %q after %d lookups", s.Dir, procs.CwdCalls)
		}
		// Through the root: FindProcessSessions takes the batch path.
		procs.EnvBatchCalls = 0
		all, err := agentio.FindProcessSessions(ctx, []agentio.Agent{a.Neutral()}, table)
		if err != nil || len(all) != 3 || procs.EnvBatchCalls != 1 {
			t.Errorf("FindProcessSessions: %d sessions, %v, %d env batches", len(all), err, procs.EnvBatchCalls)
		}
		if _, ok := agentio.ProcessSessionBatchOf(a.Neutral()); !ok {
			t.Error("claude has the batch reader")
		}
		// No session rows: nothing is read.
		procs.EnvBatchCalls = 0
		if none, err := a.ProcessSessions(ctx, table[3:]); err != nil || len(none) != 0 || procs.EnvBatchCalls != 0 {
			t.Errorf("non-session rows: %v %v, %d env batches", none, err, procs.EnvBatchCalls)
		}
	})
}

// TestStopPlanRefusesMCPCommand: a `claude mcp list` row is refused like
// `claude mcp serve`.
func TestStopPlanRefusesMCPCommand(t *testing.T) {
	a, _ := procAgent(t, filepath.Join("testdata", "homes", "2.1.327", "userhome"), &agenttest.FakeProcesses{})
	plan, err := a.StopPlan(agentio.Process{PID: 9, Argv: []string{"claude", "mcp", "list"}}, nil, agentio.ProcessSession{})
	if err != nil || plan.Refuse == "" || len(plan.Targets) != 0 {
		t.Fatalf("mcp list plan = %+v %v", plan, err)
	}
}

// TestClaudeResumeLine prints CLAUDE_CONFIG_DIR for a non-default home's
// resume and leaves it out for the default home.
func TestClaudeResumeLine(t *testing.T) {
	a, home := procAgent(t, filepath.Join("testdata", "homes", "2.1.327", "userhome"), &agenttest.FakeProcesses{})
	p := agentio.Process{PID: 9, Argv: []string{"claude"}}
	work := a.HomeAt(filepath.Join(home, ".claude-work")).Neutral()
	plan, err := a.StopPlan(p, nil, agentio.ProcessSession{Agent: ID, PID: 9, ConversationID: fixtureWorkSession, Home: work})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := plan.ResumeLine(), ConfigDirEnv+"="+work.Dir()+" claude --resume "+fixtureWorkSession; got != want {
		t.Errorf("work resume = %q, want %q", got, want)
	}
	plan, err = a.StopPlan(p, nil, agentio.ProcessSession{Agent: ID, PID: 9, ConversationID: fixtureSession4243, Home: a.DefaultHome().Neutral()})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := plan.ResumeLine(), "claude --resume "+fixtureSession4243; got != want {
		t.Errorf("default resume = %q, want %q", got, want)
	}
}
