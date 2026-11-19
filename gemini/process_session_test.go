package gemini

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// procFixture is the shared fixture with a process lister and chat mtimes
// set (copying gives every file the same mtime).
func procFixture(t *testing.T, procs *agenttest.FakeProcesses) fixture {
	t.Helper()
	f := setup(t, agenttest.WithProcesses(procs))
	chats := filepath.Join(f.home.ProjectTmpDir("demo"), chatsDir)
	for name, mod := range map[string]time.Time{
		"session-2026-11-01T10-00-11111111.jsonl": time.Date(2026, 11, 1, 11, 0, 0, 0, time.UTC),
		"session-2026-11-02T09-30-22222222.jsonl": time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC),
		"session-2026-11-03T08-00-33333333.jsonl": time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC),
		"session-2026-10-20T12-00-44444444.json":  time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC),
	} {
		if err := os.Chtimes(filepath.Join(chats, name), mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// TestGeminiProcessSession joins by --resume id, by --resume latest, and
// by the newest chat started after the process; only the relaunch child
// is a session.
func TestGeminiProcessSession(t *testing.T) {
	procs := &agenttest.FakeProcesses{Envs: map[int][]string{}, Cwds: map[int]string{}}
	f := procFixture(t, procs)
	ctx := context.Background()
	row := func(pid int, start time.Time, args ...string) agentio.Process {
		procs.Envs[pid] = []string{"PATH=/bin"}
		procs.Cwds[pid] = f.project
		return agentio.Process{PID: pid, Start: start, Argv: append([]string{"node", "/usr/local/bin/gemini"}, args...)}
	}
	// --resume <uuid> / -r <uuid>: exact chat of the project.
	for _, args := range [][]string{{"--resume", sessChangelog}, {"-r", sessChangelog}, {"--resume=" + sessChangelog}} {
		s, ok, err := f.a.ProcessSession(ctx, row(10, time.Time{}, args...))
		if err != nil || !ok || s.Confidence != agentio.JoinArgv || s.ConversationID != sessChangelog || !s.Home.IsDefault() ||
			filepath.Base(s.Path) != "session-2026-11-02T09-30-22222222.jsonl" || !s.LastActivity.Equal(time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)) {
			t.Fatalf("%q = %+v %v %v", args, s, ok, err)
		}
	}
	// --resume latest and a bare --resume: the chat written last, a guess.
	for _, args := range [][]string{{"--resume", "latest"}, {"--resume"}} {
		s, _, _ := f.a.ProcessSession(ctx, row(11, time.Time{}, args...))
		if s.Confidence != agentio.JoinInferred || s.ConversationID != sessEmpty {
			t.Fatalf("%q = %+v", args, s)
		}
	}
	// No resume: the newest chat that started after the process.
	s, _, _ := f.a.ProcessSession(ctx, row(12, time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)))
	if s.Confidence != agentio.JoinInferred || s.ConversationID != sessEmpty {
		t.Fatalf("inferred = %+v", s)
	}
	s, ok, _ := f.a.ProcessSession(ctx, row(13, time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)))
	if !ok || s.Confidence != agentio.JoinUnknown || s.ConversationID != "" || !s.Home.IsDefault() {
		t.Fatalf("no chat after start = %+v", s)
	}
	// An unknown id in another home (GEMINI_CLI_HOME names its parent):
	// the id and that home are kept.
	other := row(14, time.Time{}, "--resume", "99999999-9999-4999-8999-999999999999")
	procs.Envs[14] = []string{HomeEnv + "=" + filepath.Join(f.host.UserHome, "gemini-work")}
	s, _, _ = f.a.ProcessSession(ctx, other)
	if s.ConversationID != "99999999-9999-4999-8999-999999999999" || s.Home.Dir() != filepath.Join(f.host.UserHome, "gemini-work", defaultDirName) || s.Path != "" {
		t.Fatalf("other home = %+v", s)
	}
	// The relaunch pair: the parent is no session, the child is.
	parent := row(20, time.Time{}, "--resume", sessChangelog)
	child := row(21, time.Time{}, "--resume", sessChangelog)
	child.PPID = 20
	child.Argv = []string{"node", "--max-old-space-size=8192", "/usr/local/bin/gemini", "--resume", sessChangelog}
	procs.List = []agentio.Process{parent, child}
	if _, ok, _ := f.a.ProcessSession(ctx, parent); ok {
		t.Fatal("the relaunch parent is not the session")
	}
	if s, ok, _ := f.a.ProcessSession(ctx, child); !ok || s.ConversationID != sessChangelog {
		t.Fatalf("child = %+v %v", s, ok)
	}
	// A management subcommand is no session.
	if _, ok, _ := f.a.ProcessSession(ctx, agentio.Process{PID: 30, Argv: []string{"gemini", "mcp", "list"}}); ok {
		t.Fatal("gemini mcp list has a session")
	}
}

// TestGeminiStopPlan targets the child then the parent from either row,
// refuses a management subcommand, and resumes a known chat.
func TestGeminiStopPlan(t *testing.T) {
	f := procFixture(t, &agenttest.FakeProcesses{})
	parent := agentio.Process{PID: 20, Argv: []string{"node", "/usr/local/bin/gemini"}}
	child := agentio.Process{PID: 21, PPID: 20, Argv: []string{"node", "--max-old-space-size=8192", "/usr/local/bin/gemini"}}
	shell := agentio.Process{PID: 22, PPID: 20, Argv: []string{"bash"}}
	table := []agentio.Process{parent, shell, child}
	s := agentio.ProcessSession{Agent: ID, Home: f.home.Neutral(), ConversationID: sessChangelog, Dir: f.project}
	for _, p := range []agentio.Process{parent, child} {
		plan, err := f.a.StopPlan(p, table, s)
		if err != nil || !slices.Equal(plan.Targets, []int{21, 20}) || plan.Graceful != agentio.StopSignalTerminate || plan.Resume == nil {
			t.Fatalf("pid %d plan = %+v %v", p.PID, plan, err)
		}
		if !slices.Equal(plan.Resume.Args, []string{flagResume, sessChangelog}) || plan.Resume.Dir != f.project {
			t.Fatalf("resume = %+v", plan.Resume)
		}
	}
	if plan, _ := f.a.StopPlan(parent, nil, agentio.ProcessSession{}); !slices.Equal(plan.Targets, []int{20}) {
		t.Fatalf("no relaunch = %+v", plan)
	}
	if plan, _ := f.a.StopPlan(agentio.Process{PID: 30, Argv: []string{"gemini", "mcp", "list"}}, nil, s); plan.Refuse == "" {
		t.Fatalf("management plan = %+v", plan)
	}
	if _, err := f.a.StopPlan(shell, table, s); !errors.Is(err, agentio.ErrNotFound) {
		t.Fatalf("foreign row: %v", err)
	}
	if c, ok := f.a.RelaunchChild(parent, table); !ok || c.PID != 21 {
		t.Fatalf("RelaunchChild = %+v %v", c, ok)
	}
	n := f.a.Neutral()
	if _, ok := agentio.ProcessSessionsOf(n); !ok {
		t.Fatal("no ProcessSessionReader")
	}
	if _, ok := agentio.StopPlannerOf(n); !ok {
		t.Fatal("no StopPlanner")
	}
}
