package pi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// writeSession adds a session file for cwd to h with header time ts and
// file mtime mod; returns its path.
func writeProcSession(t *testing.T, h Home, cwd, id string, ts, mod time.Time) string {
	t.Helper()
	name := ts.UTC().Format("2006-01-02T15-04-05-000Z") + fileIDSep + id + sessionExt
	body := fmt.Sprintf(`{"type":"session","version":3,"id":%q,"timestamp":%q,"cwd":%q}`+"\n", id, ts.UTC().Format(time.RFC3339Nano), cwd)
	p := agenttest.WriteFile(t, filepath.Join(h.Dir(), sessionsDir, EncodeSessionDir(cwd)), name, body)
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestPiProcessSession joins a title row by the newest session started
// after it, a --continue row by the newest session, a windows script row
// by --session, and never a row it does not classify.
func TestPiProcessSession(t *testing.T) {
	procs := &agenttest.FakeProcesses{Envs: map[int][]string{}, Cwds: map[int]string{}}
	host := agenttest.NewHost(t, agenttest.WithProcesses(procs))
	agenttest.CopyFixture(t, fixtureUserHome, host.UserHome)
	a := New(host)
	h := a.DefaultHome()
	cwd := filepath.Join(host.UserHome, "proj")
	start := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	const old, fresh, newest = "0190f0e0-0000-7000-8000-00000000000a", "0190f0e0-0000-7000-8000-00000000000b", "0190f0e0-0000-7000-8000-00000000000c"
	writeProcSession(t, h, cwd, old, start.Add(-time.Hour), start.Add(5*time.Hour)) // written last, but started before
	writeProcSession(t, h, cwd, fresh, start.Add(time.Minute), start.Add(2*time.Hour))
	freshPath := writeProcSession(t, h, cwd, newest, start.Add(time.Hour), start.Add(3*time.Hour))
	ctx := context.Background()
	row := func(pid int, start time.Time, exe string, argv ...string) agentio.Process {
		procs.Envs[pid] = []string{"PATH=/bin"}
		procs.Cwds[pid] = cwd
		return agentio.Process{PID: pid, Start: start, Exe: exe, Argv: argv}
	}
	s, ok, err := a.ProcessSession(ctx, row(10, start, "/usr/bin/node", "pi"))
	if err != nil || !ok || s.Confidence != agentio.JoinInferred || s.ConversationID != newest || s.Path != freshPath || !s.Home.IsDefault() ||
		!s.LastActivity.Equal(start.Add(3*time.Hour)) || s.Dir != cwd {
		t.Fatalf("title row = %+v %v %v", s, ok, err)
	}
	node := `C:\Program Files\nodejs\node.exe`
	script := `C:\Users\alice\AppData\Roaming\npm\node_modules\@earendil-works\pi-coding-agent\dist\bundle\cli.js`
	s, _, _ = a.ProcessSession(ctx, row(11, start, node, node, script, "--continue"))
	if s.Confidence != agentio.JoinInferred || s.ConversationID != old {
		t.Fatalf("--continue = %+v", s)
	}
	for _, v := range []string{fresh, filepath.Join(h.Dir(), sessionsDir, EncodeSessionDir(cwd), start.Add(time.Minute).UTC().Format("2006-01-02T15-04-05-000Z")+fileIDSep+fresh+sessionExt)} {
		s, ok, _ = a.ProcessSession(ctx, row(12, start, node, node, script, "--session", v))
		if !ok || s.Confidence != agentio.JoinArgv || s.ConversationID != fresh {
			t.Fatalf("--session %s = %+v", v, s)
		}
	}
	s, _, _ = a.ProcessSession(ctx, row(13, start.Add(10*time.Hour), "/usr/bin/node", "pi"))
	if s.Confidence != agentio.JoinUnknown || s.ConversationID != "" {
		t.Fatalf("nothing after start = %+v", s)
	}
	// PI_CODING_AGENT_SESSION_DIR in the process env moves the store.
	flat := filepath.Join(host.UserHome, "flat")
	writeProcSession(t, h, cwd, "0190f0e0-0000-7000-8000-00000000000d", start.Add(time.Minute), start.Add(time.Hour))
	if err := os.MkdirAll(flat, 0o700); err != nil {
		t.Fatal(err)
	}
	procs.Envs[14] = []string{SessionDirEnv + "=" + flat}
	procs.Cwds[14] = cwd
	if s, _, _ := a.ProcessSession(ctx, agentio.Process{PID: 14, Start: start, Exe: "/usr/bin/node", Argv: []string{"pi"}}); s.ConversationID != "" {
		t.Fatalf("flat store from the process env ignored: %+v", s)
	}
	// CLN's pi is not pi.
	if _, ok, _ := a.ProcessSession(ctx, agentio.Process{PID: 15, Exe: "/usr/bin/pi", Argv: []string{"/usr/bin/pi", "1000"}}); ok {
		t.Fatal("digits program joined")
	}
}

// TestPiStopPlan is the base plan with a resume by session id.
func TestPiStopPlan(t *testing.T) {
	a, host := newAgent(t)
	p := agentio.Process{PID: 7, Exe: "/usr/bin/node", Argv: []string{"pi"}}
	s := agentio.ProcessSession{Agent: ID, Home: a.DefaultHome().Neutral(), ConversationID: "abc", Dir: host.UserHome}
	plan, err := a.StopPlan(p, nil, s)
	if err != nil || !slices.Equal(plan.Targets, []int{7}) || plan.Graceful != agentio.StopSignalTerminate || plan.Resume == nil ||
		!slices.Equal(plan.Resume.Args, []string{flagSession, "abc"}) {
		t.Fatalf("plan = %+v %v", plan, err)
	}
	if _, err := a.StopPlan(agentio.Process{PID: 8, Exe: "/usr/bin/pi", Argv: []string{"/usr/bin/pi", "1000"}}, nil, s); !errors.Is(err, agentio.ErrNotFound) {
		t.Fatalf("digits program: %v", err)
	}
	n := a.Neutral()
	if _, ok := agentio.ProcessSessionsOf(n); !ok {
		t.Fatal("no ProcessSessionReader")
	}
	if _, ok := agentio.StopPlannerOf(n); !ok {
		t.Fatal("no StopPlanner")
	}
}
