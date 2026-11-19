package codex

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// writeRollout adds a rollout with only its session_meta line to h. Pure
// apart from the file write.
func writeRollout(t *testing.T, h Home, id, cwd string, created time.Time) string {
	t.Helper()
	rel := filepath.Join("sessions", created.Format(rolloutDayLayout), "rollout-"+created.Format("2006-01-02T15-04-05")+"-"+id+".jsonl")
	body := fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"id":%q,"timestamp":%q,"cwd":%q,"originator":"codex-tui","cli_version":"0.162.1","source":"cli"}}`+"\n",
		created.Format(time.RFC3339Nano), id, created.Format(time.RFC3339Nano), cwd)
	return agenttest.WriteFile(t, h.Dir(), rel, body)
}

// TestCodexProcessSessionArgv joins `codex resume <id>` and
// `codex exec resume <id>` to the rollout in the env-named home.
func TestCodexProcessSessionArgv(t *testing.T) {
	procs := &agenttest.FakeProcesses{Envs: map[int][]string{}, Cwds: map[int]string{}}
	a, h := setup(t, agenttest.WithProcesses(procs))
	ctx := context.Background()
	for pid, argv := range map[int][]string{
		10: {"codex", "resume", convNew},
		11: {"codex", "exec", "--json", "resume", convNew},
		12: {"codex", "resume", "--last", convNew},
	} {
		procs.Envs[pid] = []string{"PATH=/bin"}
		s, ok, err := a.ProcessSession(ctx, agentio.Process{PID: pid, Argv: argv})
		if err != nil || !ok || s.Confidence != agentio.JoinArgv || s.ConversationID != convNew || !s.Home.IsDefault() ||
			s.Dir != "/work/proj" || s.Version != "0.162.1" || s.Entrypoint != "codex-tui" || s.LastActivity.IsZero() || s.Path == "" {
			t.Fatalf("%q = %+v %v %v", argv, s, ok, err)
		}
	}
	// A thread name or `--last` alone is not an id: no argv join.
	procs.Envs[13] = []string{"PATH=/bin"}
	s, ok, _ := a.ProcessSession(ctx, agentio.Process{PID: 13, Argv: []string{"codex", "resume", "--last"}})
	if !ok || s.Confidence != agentio.JoinUnknown || !s.Home.IsDefault() {
		t.Fatalf("resume --last = %+v", s)
	}
	// CODEX_HOME names another home without the rollout: the id is kept,
	// the home is the named one.
	procs.Envs[14] = []string{EnvHome + "=" + filepath.Join(filepath.Dir(h.Dir()), "codex-work")}
	s, _, _ = a.ProcessSession(ctx, agentio.Process{PID: 14, Argv: []string{"codex", "resume", convNew}})
	if s.ConversationID != convNew || s.Home.IsDefault() || s.Path != "" {
		t.Fatalf("other home = %+v", s)
	}
	// The launcher, app-server and mcp-server are not session processes.
	for _, argv := range [][]string{
		{"node", "/lib/node_modules/@openai/codex/bin/codex.js", "resume", convNew},
		{"codex", "app-server"},
		{"codex", "mcp-server"},
	} {
		if _, ok, _ := a.ProcessSession(ctx, agentio.Process{PID: 20, Argv: argv}); ok {
			t.Fatalf("%q has a session", argv)
		}
	}
}

// TestCodexProcessSessionInferred picks the rollout in the process cwd
// created closest after its start, ignores one created before it and one
// in another folder, honours -C, and stays a JoinInferred guess.
func TestCodexProcessSessionInferred(t *testing.T) {
	start := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	procs := &agenttest.FakeProcesses{Envs: map[int][]string{30: {"PATH=/bin"}, 31: {"PATH=/bin"}, 32: {"PATH=/bin"}}, Cwds: map[int]string{30: "/work/app", 31: "/work", 32: "/work/empty"}}
	a, h := setup(t, agenttest.WithProcesses(procs))
	const before, first, later, elsewhere, tooLate = "0199e1a0-0000-7000-8000-00000000000a", "0199e1a0-0000-7000-8000-00000000000b", "0199e1a0-0000-7000-8000-00000000000c", "0199e1a0-0000-7000-8000-00000000000d", "0199e1a0-0000-7000-8000-00000000000e"
	writeRollout(t, h, before, "/work/app", start.Add(-time.Hour))
	writeRollout(t, h, first, "/work/app", start.Add(26*time.Hour)) // next day's folder
	writeRollout(t, h, later, "/work/app", start.Add(30*time.Hour))
	writeRollout(t, h, elsewhere, "/work/other", start.Add(time.Minute))
	writeRollout(t, h, tooLate, "/work/empty", start.Add(inferAfter+2*day))
	ctx := context.Background()
	s, ok, err := a.ProcessSession(ctx, agentio.Process{PID: 30, Start: start, Argv: []string{"codex"}})
	if err != nil || !ok || s.Confidence != agentio.JoinInferred || s.ConversationID != first || s.Dir != "/work/app" || s.Path == "" {
		t.Fatalf("inferred = %+v %v %v", s, ok, err)
	}
	s, _, _ = a.ProcessSession(ctx, agentio.Process{PID: 31, Start: start, Argv: []string{"codex", "-C", "app"}})
	if s.ConversationID != first {
		t.Fatalf("-C relative = %+v", s)
	}
	s, _, _ = a.ProcessSession(ctx, agentio.Process{PID: 32, Start: start, Argv: []string{"codex"}})
	if s.ConversationID != "" || s.Confidence != agentio.JoinUnknown {
		t.Fatalf("rollout outside the scanned folders joined: %+v", s)
	}
	// No start time: no guess.
	s, _, _ = a.ProcessSession(ctx, agentio.Process{PID: 30, Argv: []string{"codex"}})
	if s.ConversationID != "" {
		t.Fatalf("guess without a start time: %+v", s)
	}
	// Env unreadable and nothing in argv or cwd: nothing ties the row.
	procs.Envs = nil
	if _, ok, _ := a.ProcessSession(ctx, agentio.Process{PID: 40, Argv: []string{"codex"}}); ok {
		t.Fatal("unreadable env, no join")
	}
	// Env unreadable: discovered homes are searched.
	s, ok, _ = a.ProcessSession(ctx, agentio.Process{PID: 30, Start: start, Argv: []string{"codex"}})
	if !ok || s.ConversationID != first || !s.Home.IsDefault() {
		t.Fatalf("discovered homes = %+v", s)
	}
}

// TestCodexStopPlan refuses the daemons, targets the launcher's native
// child then the launcher, refuses a launcher without its child, and
// resumes a known conversation.
func TestCodexStopPlan(t *testing.T) {
	a, h := setup(t)
	launcher := agentio.Process{PID: 50, Exe: "/usr/bin/node", Argv: []string{"node", "/lib/node_modules/@openai/codex/bin/codex.js"}}
	child := agentio.Process{PID: 51, PPID: 50, Exe: "/lib/node_modules/@openai/codex/vendor/x/codex/codex", Argv: []string{"/lib/node_modules/@openai/codex/vendor/x/codex/codex"}}
	unrelated := agentio.Process{PID: 52, PPID: 50, Argv: []string{"git", "status"}}
	table := []agentio.Process{launcher, unrelated, child}
	for _, tc := range []struct {
		p      agentio.Process
		refuse string
	}{
		{agentio.Process{PID: 1, Argv: []string{"codex", "app-server"}}, refuseAppServer},
		{agentio.Process{PID: 2, Argv: []string{"codex", "mcp-server"}}, refuseMCPServer},
	} {
		plan, err := a.StopPlan(tc.p, table, agentio.ProcessSession{})
		if err != nil || plan.Refuse != tc.refuse || len(plan.Targets) != 0 {
			t.Fatalf("%q = %+v %v", tc.p.Argv, plan, err)
		}
	}
	s := agentio.ProcessSession{Agent: ID, Home: h.Neutral(), ConversationID: convNew, Dir: "/work/proj"}
	plan, err := a.StopPlan(launcher, table, s)
	if err != nil || !slices.Equal(plan.Targets, []int{51, 50}) || plan.Graceful != agentio.StopSignalTerminate || plan.Resume == nil {
		t.Fatalf("launcher plan = %+v %v", plan, err)
	}
	if !slices.Equal(plan.Resume.Args, []string{subResume, convNew}) {
		t.Fatalf("resume = %q", plan.Resume.Args)
	}
	plan, _ = a.StopPlan(launcher, []agentio.Process{launcher, unrelated}, s)
	if plan.Refuse != refuseLauncherNoChild || len(plan.Targets) != 0 {
		t.Fatalf("launcher without child = %+v", plan)
	}
	plan, _ = a.StopPlan(child, table, agentio.ProcessSession{})
	if !slices.Equal(plan.Targets, []int{51}) || plan.Resume != nil {
		t.Fatalf("cli plan = %+v", plan)
	}
	if _, err := a.StopPlan(unrelated, table, s); !errors.Is(err, agentio.ErrNotFound) {
		t.Fatalf("foreign row: %v", err)
	}
	n := a.Neutral()
	if _, ok := agentio.ProcessSessionsOf(n); !ok {
		t.Fatal("no ProcessSessionReader")
	}
	if _, ok := agentio.StopPlannerOf(n); !ok {
		t.Fatal("no StopPlanner")
	}
}
