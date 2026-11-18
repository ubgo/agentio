package claude

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
)

// sessionsAgent copies the fixture into a temp user home and returns an
// agent over it with the given process table.
func sessionsAgent(t *testing.T, fixture string, procs []agentio.Process) (*Agent, Home) {
	t.Helper()
	host := agenttest.NewHost(t, agenttest.WithProcesses(&agenttest.FakeProcesses{List: procs, Cwds: map[int]string{4242: "/work/proj4"}}))
	agenttest.CopyFixture(t, fixture, host.UserHome)
	a := New(host)
	return a, a.DefaultHome()
}

// fixtureStartedAt is sessions/4242.json startedAt.
func fixtureStartedAt() time.Time { return millis(1794597025793).Time() }

func TestLiveSessionsAliveAndPidReuse(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		procs := []agentio.Process{
			{PID: 4242, Start: fixtureStartedAt().Add(-3 * time.Second), Exe: "/home/user/.local/share/claude/versions/2.1.323", Argv: []string{"claude"}},
			{PID: 4243, Start: fixtureStartedAt().Add(48 * time.Hour), Argv: []string{"claude"}},
		}
		a, h := sessionsAgent(t, fixture, procs)
		ss, err := a.LiveSessions(h)
		if err != nil {
			t.Fatal(err)
		}
		if len(ss) != 2 {
			t.Fatalf("got %d sessions, want 2", len(ss))
		}
		s := ss[0]
		if s.PID != 4242 || !s.Alive || s.Status != StatusBusy || s.Cwd != "/work/proj4" || s.Kind != agentio.SessionInteractive || s.StatusUpdatedAt.IsZero() {
			t.Fatalf("session 4242 = %+v", s)
		}
		if ss[1].Alive {
			t.Fatalf("pid 4243 started 48h after startedAt must not be alive (pid reuse)")
		}
		n := s.Neutral(h)
		if n.Status != agentio.SessionBusy || n.StatusSince != s.StatusUpdatedAt || n.ConversationID != s.SessionID {
			t.Fatalf("neutral = %+v", n)
		}
		live, err := a.IsLive(h, s.SessionID)
		if err != nil || !live {
			t.Fatalf("IsLive = %v, %v", live, err)
		}
		in, err := a.LiveSessionsIn(h, "/work/proj4")
		if err != nil || len(in) != 1 {
			t.Fatalf("LiveSessionsIn = %d, %v", len(in), err)
		}
	})
}

func TestLiveSessionsNoProcessLister(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		host := agenttest.NewHost(t)
		host.Processes = nil
		agenttest.CopyFixture(t, fixture, host.UserHome)
		a := New(host)
		ss, err := a.LiveSessions(a.DefaultHome())
		if err != nil || len(ss) == 0 || ss[0].Alive {
			t.Fatalf("no lister: %v %v", ss, err)
		}
	})
}

func TestSessionStatusNeutral(t *testing.T) {
	cases := map[SessionStatus]agentio.SessionStatus{StatusIdle: agentio.SessionIdle, StatusBusy: agentio.SessionBusy, StatusShell: agentio.SessionBusy, StatusWaiting: agentio.SessionIdle, "new": agentio.SessionStatusUnknown}
	for in, want := range cases {
		if got := in.Neutral(); got != want {
			t.Errorf("%s → %v, want %v", in, got, want)
		}
		if known := in != "new"; in.IsKnown() != known {
			t.Errorf("%s IsKnown = %v, want %v", in, in.IsKnown(), known)
		}
	}
}

// TestLiveSessionWaiting decodes a registry entry recorded from Claude Code
// 2.1.327 while a dialog was open: status "waiting" plus waitingFor. Both
// are modelled, so nothing lands in Unknown (task drift would list it).
func TestLiveSessionWaiting(t *testing.T) {
	body := `{"pid": 4243, "sessionId": "s", "version": "2.1.327", "status": "waiting", "waitingFor": "dialog open"}`
	s, err := parseLiveSession("4243.json", []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if s.Status != StatusWaiting || s.WaitingFor != "dialog open" || len(s.Unknown) != 0 {
		t.Fatalf("session = %+v", s)
	}
}

func TestProcessesAndClassify(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		procs := []agentio.Process{
			{PID: 4242, Argv: []string{"claude", "--resume", "x"}},
			{PID: 7, Argv: []string{"/Applications/Claude.app/Contents/MacOS/Claude"}},
			{PID: 8, Argv: []string{"claude", "mcp", "serve"}},
			{PID: 9, Argv: []string{"bash"}},
		}
		a, _ := sessionsAgent(t, fixture, procs)
		ps, err := a.Processes(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if len(ps) != 2 || ps[0].PID != 4242 || ps[0].Kind != agentio.ProcessCLI || !ps[0].Session || ps[0].Cwd != "/work/proj4" {
			t.Fatalf("processes = %+v", ps)
		}
		if ps[1].Kind != agentio.ProcessMCPServer || ps[1].Session {
			t.Fatalf("mcp serve = %+v", ps[1])
		}
		if _, ok := ClassifyProcess(procs[1]); ok {
			t.Fatal("Claude.app must be excluded")
		}
	})
}

func TestJobsDaemonTeamsTasks(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, h := sessionsAgent(t, fixture, []agentio.Process{{PID: 99, Start: millis(1794000000000).Time()}})
		d := h.Dir()
		agenttest.WriteFile(t, d, "jobs/ab12/state.json", `{"state":"blocked","tokens":230135,"respawnFlags":["--model","opus"],"children":[{"id":"1","href":"<url>","kind":"pr"}],"createdAt":"2026-11-01T10:00:00Z","futureKey":1}`)
		agenttest.WriteFile(t, d, "jobs/ab12/timeline.jsonl", `{"at":"2026-11-01T10:00:00Z","state":"running","detail":"d","text":"t"}`+"\n"+`{"at":"2026-11-01T10:01:00Z","state":"blocked"}`+"\n")
		agenttest.WriteFile(t, d, "daemon.lock", `{"pid":99,"version":"2.1.323","startedAt":1794000000000,"procStart":"1"}`)
		agenttest.WriteFile(t, d, "daemon-auth-status.json", `{"since":1794000000000,"status":"ok"}`)
		agenttest.WriteFile(t, d, "teams/t1/config.json", `{"name":"t1","createdAt":1794000000000,"leadAgentId":"a","members":[{"agentId":"m","name":"n","joinedAt":1794000000000}]}`)
		agenttest.WriteFile(t, d, "tasks/list1/1.json", `{"x":1}`)

		jobs, err := a.Jobs(h)
		if err != nil || len(jobs) != 1 || jobs[0].Tokens != 230135 || jobs[0].Unknown["futureKey"] == nil || len(jobs[0].Children) != 1 {
			t.Fatalf("jobs = %+v, %v", jobs, err)
		}
		tl, err := a.JobTimeline(h, "ab12")
		if err != nil || len(tl) != 2 || tl[1].State != "blocked" {
			t.Fatalf("timeline = %+v, %v", tl, err)
		}
		if _, err := a.JobTimeline(h, "../x"); !errors.Is(err, agentio.ErrNotFound) {
			t.Fatalf("traversal id: %v", err)
		}
		dm, err := a.Daemon(h)
		if err != nil || dm.Lock == nil || !dm.Running || dm.Auth == nil || dm.Auth.Status != "ok" {
			t.Fatalf("daemon = %+v, %v", dm, err)
		}
		teams, err := a.Teams(h)
		if err != nil || len(teams) != 1 || len(teams[0].Members) != 1 {
			t.Fatalf("teams = %+v, %v", teams, err)
		}
		tasks, err := a.TaskLists(h)
		if err != nil || len(tasks) != 1 || len(tasks[0].Items) != 1 || len(tasks[0].Items[0].Raw) == 0 {
			t.Fatalf("tasks = %+v, %v", tasks, err)
		}
	})
}

func TestStatsFixtureAndUnknownVersion(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, h := sessionsAgent(t, fixture, nil)
		s, err := a.Stats(h)
		if err != nil {
			t.Fatal(err)
		}
		if s.Version != 5 || s.TotalSessions == 0 || len(s.Daily) != 3 || len(s.ModelUsage) == 0 || s.Longest == nil {
			t.Fatalf("stats = %+v", s)
		}
		bad, err := ParseStats([]byte(`{"version":6,"x":1}`))
		if !errors.Is(err, agentio.ErrUnknownVersion) || bad.Version != 6 || len(bad.Raw) == 0 {
			t.Fatalf("v6: %+v %v", bad, err)
		}
	})
}

func TestPolicyLimitsFixture(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, h := sessionsAgent(t, fixture, nil)
		agenttest.WriteFile(t, h.Dir(), "policy-limits.json.stamp.json", `{"v":1,"kind":"k","identity":"acct-1","sha":"s","confirmed_at":"2026-11-01T10:00:00Z","hipaa_seen":false}`)
		p, err := a.PolicyLimits(h)
		if err != nil || p == nil {
			t.Fatalf("policy: %v", err)
		}
		if _, ok := p.Restrictions["allow_remote_control"]; !ok || p.Stamp == nil || p.Stamp.Identity.Reveal() != "acct-1" {
			t.Fatalf("policy = %+v", p)
		}
		if strings.Contains(p.Stamp.Identity.String(), "acct-1") {
			t.Fatal("stamp identity must print redacted")
		}
	})
}

func TestLocalReaders(t *testing.T) {
	forEachFixture(t, func(t *testing.T, fixture string) {
		a, h := sessionsAgent(t, fixture, nil)
		d := h.Dir()
		agenttest.WriteFile(t, d, "file-history/s1/abc123@v2", "x")
		agenttest.WriteFile(t, d, "shell-snapshots/snapshot-zsh-1794000000000-abcd.sh", "#")
		agenttest.WriteFile(t, d, "ide/41234.lock", `{"pid":5,"workspaceFolders":["/w"],"ideName":"VS Code","transport":"ws","authToken":"x"}`)
		agenttest.WriteFile(t, d, "debug/s1.txt", "log")
		agenttest.WriteFile(t, d, "cache/changelog.md", "# Changelog")
		agenttest.WriteFile(t, d, "cache/model-catalog/m.json", "{}")
		agenttest.WriteFile(t, d, "agentmsg-x.sh", "#")

		cps, err := a.FileHistory(h, "s1")
		if err != nil || len(cps) != 1 || cps[0].FileHash != "abc123" || cps[0].Version != 2 {
			t.Fatalf("file history = %+v, %v", cps, err)
		}
		snaps, err := a.ShellSnapshots(h)
		if err != nil || len(snaps) != 1 || snaps[0].Shell != "zsh" || snaps[0].Created.IsZero() {
			t.Fatalf("snapshots = %+v, %v", snaps, err)
		}
		lc, err := a.LastCleanup(h)
		if err != nil || lc == nil || lc.Year() != 2026 {
			t.Fatalf("last cleanup = %v, %v", lc, err)
		}
		locks, err := a.IDELocks(h)
		if err != nil || len(locks) != 1 || locks[0].Port != 41234 || locks[0].IDEName != "VS Code" || locks[0].Unknown["authToken"] == nil {
			t.Fatalf("ide = %+v, %v", locks, err)
		}
		logs, err := a.DebugLogs(h)
		if err != nil || len(logs) != 1 || logs[0].Session != "s1" {
			t.Fatalf("debug = %+v, %v", logs, err)
		}
		cl, err := a.Changelog(h)
		if err != nil || cl != "# Changelog" {
			t.Fatalf("changelog = %q, %v", cl, err)
		}
		mc, err := a.ModelCatalogFiles(h)
		if err != nil || len(mc) != 1 || filepath.Base(mc[0]) != "m.json" {
			t.Fatalf("catalog = %v, %v", mc, err)
		}
		du, err := a.DataUsage(h)
		if err != nil {
			t.Fatal(err)
		}
		var foreign, projects bool
		for _, e := range du {
			if e.Name == "agentmsg-x.sh" && e.Foreign {
				foreign = true
			}
			if e.Name == projectsDir && !e.Foreign && e.Bytes > 0 {
				projects = true
			}
		}
		if !foreign || !projects {
			t.Fatalf("data usage = %+v", du)
		}
	})
}

// docsStream is a docs-shaped stream-json run covering the lines the
// recorded 2.1.325 run (testdata/stream) did not produce: user, api_retry,
// task_notification.
const docsStream = `{"type":"system","subtype":"init","session_id":"s1","model":"claude-opus-5-5","cwd":"/w"}

{"type":"assistant","session_id":"s1","uuid":"u1","parent_tool_use_id":null,"message":{"role":"assistant","content":[{"type":"text","text":"hello"},{"type":"tool_use","id":"t1","name":"Bash","input":{}}]}}
{"type":"user","session_id":"s1","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}
{"type":"system","subtype":"api_retry","attempt":1,"max_retries":10,"retry_delay_ms":500,"error_status":529,"error":"overloaded","session_id":"s1"}
{"type":"system","subtype":"task_notification","task_id":"k","status":"completed","usage":{"total_tokens":12,"tool_uses":1,"duration_ms":3}}
{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"s1","usage":{"input_tokens":3,"output_tokens":4,"cache_read_input_tokens":5,"cache_creation_input_tokens":6}}
`

func TestStreamDecoder(t *testing.T) {
	d := NewStreamDecoder(strings.NewReader(docsStream))
	var msgs []StreamMessage
	for {
		m, err := d.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, m)
	}
	if len(msgs) != 6 {
		t.Fatalf("got %d messages", len(msgs))
	}
	want := []agentio.RunEventType{agentio.RunEventStarted, agentio.RunEventMessage, agentio.RunEventTool, agentio.RunEventOther, agentio.RunEventOther, agentio.RunEventDone}
	for i, m := range msgs {
		if got := m.Neutral().Type; got != want[i] {
			t.Errorf("msg %d (%s/%s) → %v, want %v", i, m.Type, m.Subtype, got, want[i])
		}
	}
	if ev := msgs[1].Neutral(); ev.Text != "hello" || ev.ConversationID != "s1" {
		t.Fatalf("assistant event = %+v", ev)
	}
	if ev := msgs[5].Neutral(); ev.Tokens == nil || ev.Tokens.CachedInput != 5 || ev.Tokens.CacheWrite != 6 || ev.Text != "done" {
		t.Fatalf("result event = %+v", ev)
	}
	r, err := msgs[3].AsAPIRetry()
	if err != nil || r.Attempt != 1 || r.ErrorStatus == nil || *r.ErrorStatus != 529 {
		t.Fatalf("retry = %+v, %v", r, err)
	}
	tn, err := msgs[4].AsTaskNotification()
	if err != nil || tn.Usage == nil || tn.Usage.TotalTokens != 12 {
		t.Fatalf("task = %+v, %v", tn, err)
	}
	if _, err := msgs[0].AsAPIRetry(); !errors.Is(err, ErrWrongStreamMessage) {
		t.Fatalf("wrong type: %v", err)
	}
	rd := newRunDecoder(strings.NewReader(docsStream))
	if ev, err := rd.Next(); err != nil || ev.Type != agentio.RunEventStarted {
		t.Fatalf("run decoder: %+v %v", ev, err)
	}
	res, err := DecodeJSONResult(strings.NewReader(` {"type":"result"} `))
	if err != nil || string(res.Raw) != `{"type":"result"}` {
		t.Fatalf("json result = %s, %v", res.Raw, err)
	}
	if _, err := DecodeJSONResult(strings.NewReader(`{"type":"system"}`)); !errors.Is(err, ErrWrongStreamMessage) {
		t.Fatalf("non-result document: %v", err)
	}
}

// TestFixture2_1_327Deltas reads what the 2.1.327 fixture adds over 2.1.325
// (the drift run's observed deltas): a session waiting on a dialog, three
// new global-config keys and gitCommitSha on plugin entries. The modelled
// ones decode into fields; the rest stay in Unknown, never dropped.
func TestFixture2_1_327Deltas(t *testing.T) {
	a, h := sessionsAgent(t, filepath.Join("testdata", "homes", "2.1.327", "userhome"), nil)
	ss, err := a.LiveSessions(h)
	if err != nil || len(ss) != 2 {
		t.Fatalf("sessions %d, %v", len(ss), err)
	}
	w := ss[1]
	if w.Status != StatusWaiting || w.WaitingFor != "dialog open" || w.Version != "2.1.327" {
		t.Fatalf("waiting session = %+v", w)
	}
	if _, ok := w.Unknown["waitingFor"]; ok {
		t.Fatalf("waiting session = %+v", w)
	}
	gc, err := a.GlobalConfig(h)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"hasSeenTasksHint", "lspRecommendationIgnoredCount", "pluginUsageLspGraceAppliedIds"} {
		if _, ok := gc.Unknown[k]; !ok {
			t.Errorf("global config key %s not kept in Unknown", k)
		}
	}
	ps, err := a.Plugins(h, nil)
	if err != nil || len(ps) == 0 {
		t.Fatalf("plugins %d, %v", len(ps), err)
	}
	if _, ok := ps[0].Unknown["gitCommitSha"]; !ok {
		t.Errorf("plugin gitCommitSha not kept in Unknown: %+v", ps[0])
	}
}
