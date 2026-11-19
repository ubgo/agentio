package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/opencode"
)

// schemaFile is the recorded 1.18.30 schema of the four tables read here.
const schemaFile = "testdata/schema-1.18.30.sql"

// Fixture rows: every value is fake. The data JSON follows the shapes of
// a real 1.18.30 database (message and part data without their id
// columns, tokens.total, finish, an aborted assistant message, a
// compaction summary, synthetic text).
const seed = `
INSERT INTO project (id, worktree, time_created, time_updated, sandboxes) VALUES ('global', '/', 1, 2, '[]');
INSERT INTO session (id, project_id, parent_id, slug, directory, title, version, share_url, time_created, time_updated, time_archived,
  workspace_id, path, agent, model, cost, tokens_input, tokens_output, tokens_reasoning, tokens_cache_read, tokens_cache_write, summary_additions)
VALUES
 ('ses_A', 'global', NULL, 'brave-otter', '/Users/alice/proj', 'Fix the login form', '1.18.16', NULL, 1789000100000, 1789000190000, NULL,
  NULL, 'Users/alice/proj', 'build', '{"id":"kimi-k3","providerID":"moonshotai","variant":"high"}', 0.11, 29884, 409, 599, 25344, 0, 3),
 ('ses_child', 'global', 'ses_A', 'quiet-fox', '/Users/alice/proj', 'Subtask', '1.18.16', NULL, 1789000120000, 1789000130000, NULL,
  NULL, NULL, NULL, NULL, 0, 0, 0, 0, 0, 0, NULL),
 ('ses_old', 'global', NULL, 'old-owl', '/Users/alice/other', 'Archived one', '1.3.13', 'https://opencode.ai/s/FIXTURE', 1775000100000, 1775000900000, 1775001000000,
  NULL, NULL, NULL, NULL, 0, 0, 0, 0, 0, 0, NULL);
INSERT INTO message (id, session_id, time_created, time_updated, data) VALUES
 ('msg_01', 'ses_A', 100, 100, '{"role":"user","time":{"created":100},"agent":"build","model":{"providerID":"moonshotai","modelID":"kimi-k3","variant":"high"},"summary":{"diffs":[]}}'),
 ('msg_02', 'ses_A', 101, 120, '{"role":"assistant","time":{"created":101,"completed":120},"parentID":"msg_01","modelID":"kimi-k3","providerID":"moonshotai","mode":"build","agent":"build","path":{"cwd":"/Users/alice/proj","root":"/Users/alice/proj"},"cost":0.05,"tokens":{"total":1300,"input":1000,"output":200,"reasoning":100,"cache":{"read":800,"write":0}},"finish":"tool-calls"}'),
 ('msg_03', 'ses_A', 121, 140, '{"role":"assistant","time":{"created":121,"completed":140},"parentID":"msg_01","modelID":"kimi-k3","providerID":"moonshotai","mode":"build","agent":"build","path":{"cwd":"/Users/alice/proj","root":"/Users/alice/proj"},"cost":0.06,"tokens":{"total":1500,"input":1200,"output":250,"reasoning":50,"cache":{"read":900,"write":5}},"finish":"stop"}'),
 ('msg_04', 'ses_A', 150, 150, '{"role":"user","time":{"created":150},"agent":"build","model":{"providerID":"moonshotai","modelID":"kimi-k3"}}'),
 ('msg_05', 'ses_A', 151, 151, '{"role":"assistant","time":{"created":151},"parentID":"msg_04","modelID":"kimi-k3","providerID":"moonshotai","mode":"build","agent":"build","path":{"cwd":"/Users/alice/proj","root":"/Users/alice/proj"},"cost":0,"tokens":{"input":0,"output":0,"reasoning":0,"cache":{"read":0,"write":0}}}'),
 ('msg_11', 'ses_old', 10, 10, '{"role":"user","time":{"created":10},"agent":"build","model":{"providerID":"ollama","modelID":"gemma4"}}'),
 ('msg_12', 'ses_old', 11, 30, '{"role":"assistant","time":{"created":11,"completed":30},"modelID":"gemma4","providerID":"ollama","mode":"build","agent":"build","path":{"cwd":"/Users/alice/other","root":"/"},"cost":0,"tokens":{"input":5,"output":1,"reasoning":0,"cache":{"read":0,"write":0}},"error":{"name":"MessageAbortedError","data":{"message":"Aborted"}}}'),
 ('msg_13', 'ses_old', 31, 40, '{"role":"assistant","time":{"created":31,"completed":40},"modelID":"gemma4","providerID":"ollama","mode":"compaction","agent":"compaction","summary":true,"path":{"cwd":"/Users/alice/other","root":"/"},"cost":0,"tokens":{"input":9,"output":3,"reasoning":0,"cache":{"read":0,"write":0}},"finish":"stop"}');
INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES
 ('prt_01', 'msg_01', 'ses_A', 100, 100, '{"type":"text","text":"fix the login form"}'),
 ('prt_02', 'msg_01', 'ses_A', 100, 100, '{"type":"text","text":"Plan mode reminder.","synthetic":true,"time":{"start":100,"end":100}}'),
 ('prt_03', 'msg_02', 'ses_A', 102, 102, '{"type":"step-start"}'),
 ('prt_04', 'msg_02', 'ses_A', 103, 103, '{"type":"reasoning","text":"Looking at the form.","time":{"start":103,"end":104}}'),
 ('prt_05', 'msg_02', 'ses_A', 105, 105, '{"type":"text","text":"Let me read the file.","time":{"start":105,"end":106}}'),
 ('prt_06', 'msg_02', 'ses_A', 107, 107, '{"type":"tool","callID":"call_1","tool":"read","state":{"status":"completed","input":{},"output":"x","title":"login.ts","metadata":{},"time":{"start":107,"end":108}},"metadata":{}}'),
 ('prt_07', 'msg_02', 'ses_A', 119, 119, '{"type":"step-finish","reason":"tool-calls","tokens":{"total":1300,"input":1000,"output":200,"reasoning":100,"cache":{"write":0,"read":800}},"cost":0.05}'),
 ('prt_08', 'msg_03', 'ses_A', 122, 122, '{"type":"text","text":"I fixed the login form.","time":{"start":122,"end":139}}'),
 ('prt_09', 'msg_04', 'ses_A', 150, 150, '{"type":"text","text":"thanks"}'),
 ('prt_11', 'msg_11', 'ses_old', 10, 10, '{"type":"text","text":"hello"}'),
 ('prt_12', 'msg_13', 'ses_old', 32, 32, '{"type":"text","text":"Summary of the conversation."}'),
 ('prt_13', 'msg_13', 'ses_old', 33, 33, '{"type":"compaction","auto":true,"overflow":true}');
`

// olderSchema is the session table as 1.2.0 created it: the columns before
// the ones later releases added with ALTER TABLE (workspace_id, path,
// agent, model, cost, tokens_*, metadata), as the recorded DDL shows.
const olderSchema = `
CREATE TABLE session (id text PRIMARY KEY, project_id text NOT NULL, parent_id text, slug text NOT NULL, directory text NOT NULL,
  title text NOT NULL, version text NOT NULL, share_url text, summary_additions integer, summary_deletions integer, summary_files integer,
  summary_diffs text, revert text, permission text, time_created integer NOT NULL, time_updated integer NOT NULL, time_compacting integer, time_archived integer);
CREATE TABLE message (id text PRIMARY KEY, session_id text NOT NULL, time_created integer NOT NULL, time_updated integer NOT NULL, data text NOT NULL);
CREATE TABLE part (id text PRIMARY KEY, message_id text NOT NULL, session_id text NOT NULL, time_created integer NOT NULL, time_updated integer NOT NULL, data text NOT NULL);
INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated) VALUES ('ses_B', 'global', 's', '/d', 'Old', '1.2.0', 1, 2);
`

// buildDB writes a database at dir/opencode.db from statements, in WAL
// mode like opencode. Test-only writer: the package never opens a file
// writable. It returns the writer, still open, so a test can add rows
// that sit in the WAL.
func buildDB(t *testing.T, path string, stmts ...string) *sql.DB {
	t.Helper()
	w, err := sql.Open(driverName, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	if _, err := w.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	for _, s := range stmts {
		if _, err := w.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

// fixture builds the 1.18.30 database in a temp data dir and opens it.
func fixture(t *testing.T) (*DB, *sql.DB) {
	t.Helper()
	schema, err := os.ReadFile(schemaFile)
	if err != nil {
		t.Fatal(err)
	}
	host := agenttest.NewHost(t)
	a := opencode.New(host)
	h := a.DefaultHome()
	if err := os.MkdirAll(h.Dir(), 0o700); err != nil {
		t.Fatal(err)
	}
	path, _ := a.DatabasePath(h)
	w := buildDB(t, path, string(schema), seed)
	db, err := OpenHome(a, h)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, w
}

// collect reads every turn from cursor from.
func collect(t *testing.T, db *DB, id string, from agentio.Offset) ([]agentio.Turn, agentio.Offset) {
	t.Helper()
	var out []agentio.Turn
	off, err := db.Turns(context.Background(), id, from, func(tu agentio.Turn) error {
		out = append(out, tu)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out, off
}

func TestSessions(t *testing.T) {
	t.Parallel()
	db, _ := fixture(t)
	ctx := context.Background()
	ss, err := db.Sessions(ctx, SessionQuery{})
	if err != nil || len(ss) != 3 || ss[0].ID != "ses_A" || ss[1].ID != "ses_child" || ss[2].ID != "ses_old" {
		t.Fatalf("sessions = %+v %v", ss, err)
	}
	s := ss[0]
	want := opencode.Tokens{Input: 29884, Output: 409, Reasoning: 599, Cache: opencode.CacheTokens{Read: 25344}}
	if s.Version != "1.18.16" || s.Directory != "/Users/alice/proj" || s.Title != "Fix the login form" || s.Tokens != want || s.Cost != 0.11 ||
		s.Model == nil || s.Model.ID != "kimi-k3" || s.Model.Variant != "high" || s.Agent != "build" || s.Slug != "brave-otter" || s.Path != "Users/alice/proj" ||
		s.SummaryAdditions != 3 || !s.Archived.IsZero() || s.Created.IsZero() || !s.Updated.After(s.Created) {
		t.Fatalf("session A = %+v", s)
	}
	if ss[1].ParentID != "ses_A" || ss[2].ShareURL == "" || ss[2].Archived.IsZero() {
		t.Fatalf("child/old = %+v %+v", ss[1], ss[2])
	}
	if roots, _ := db.Sessions(ctx, SessionQuery{Roots: true, Active: true}); len(roots) != 1 || roots[0].ID != "ses_A" {
		t.Fatalf("roots = %+v", roots)
	}
	if byDir, _ := db.Sessions(ctx, SessionQuery{Dir: "/Users/alice/other/"}); len(byDir) != 1 || byDir[0].ID != "ses_old" {
		t.Fatalf("dir = %+v", byDir)
	}
	if lim, _ := db.Sessions(ctx, SessionQuery{Limit: 2}); len(lim) != 2 {
		t.Fatalf("limit = %d", len(lim))
	}
	if since, _ := db.Sessions(ctx, SessionQuery{Since: s.Updated}); len(since) != 1 {
		t.Fatalf("since = %d", len(since))
	}
	cs, err := db.Conversations(ctx, agentio.ConversationQuery{})
	if err != nil || len(cs) != 1 || cs[0].Agent != opencode.ID || cs[0].Home != db.Home().Neutral() || cs[0].Path != "" || cs[0].Dir != s.Directory {
		t.Fatalf("conversations = %+v %v", cs, err)
	}
	one, err := db.Session(ctx, "ses_old")
	if err != nil || one.Title != "Archived one" || one.Model != nil {
		t.Fatalf("session = %+v %v", one, err)
	}
	if _, err := db.Session(ctx, "ses_missing"); !errors.Is(err, agentio.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestTurns(t *testing.T) {
	t.Parallel()
	db, w := fixture(t)
	ts, off := collect(t, db, "ses_A", 0)
	if len(ts) != 4 || off != 4 {
		t.Fatalf("turns %d off %d", len(ts), off)
	}
	u, step, answer := ts[0], ts[1], ts[2]
	if u.Role != agentio.RoleUser || u.ID != "msg_01" || u.Text != "fix the login form" || u.Model != "moonshotai/kimi-k3" || u.ConversationID != "ses_A" || u.Dir != "/Users/alice/proj" || u.AgentVersion != "1.18.16" {
		t.Fatalf("user %+v", u)
	}
	if !step.Interim || step.Text != "Let me read the file." || step.StopReason != "tool-calls" || step.Tokens.Reasoning != 100 || step.Tokens.CachedInput != 800 {
		t.Fatalf("step %+v", step)
	}
	want := agentio.TokenCounts{Input: 1200, CachedInput: 900, CacheWrite: 5, Output: 250, Reasoning: 50}
	if answer.Interim || answer.Text != "I fixed the login form." || answer.Tokens == nil || *answer.Tokens != want || len(answer.Raw) == 0 {
		t.Fatalf("answer %+v", answer)
	}
	// The newest assistant message (msg_05) is still streaming: the cursor
	// stops before it until opencode completes it, then it is emitted once.
	if more, again := collect(t, db, "ses_A", off); len(more) != 0 || again != 4 {
		t.Fatalf("re-read %d %d", len(more), again)
	}
	if _, err := w.Exec(`UPDATE message SET data = json_set(data, '$.time.completed', 160, '$.finish', 'stop', '$.tokens.input', 7) WHERE id = 'msg_05'`); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data) VALUES ('prt_10', 'msg_05', 'ses_A', 152, 152, '{"type":"text","text":"You are welcome."}')`); err != nil {
		t.Fatal(err)
	}
	more, again := collect(t, db, "ses_A", off)
	if len(more) != 1 || again != 5 || more[0].ID != "msg_05" || more[0].Text != "You are welcome." || more[0].Tokens.Input != 7 {
		t.Fatalf("after completion %+v %d", more, again)
	}
	// An aborted message (error) and a compaction summary.
	old, offOld := collect(t, db, "ses_old", 0)
	if len(old) != 3 || offOld != 3 || old[1].Text != "" || !old[2].Interim || old[2].Text != "Summary of the conversation." {
		t.Fatalf("old %+v", old)
	}
	if past, back := collect(t, db, "ses_A", 99); len(past) != 0 || back != 5 {
		t.Fatalf("past end %d %d", len(past), back)
	}
	if neg, back := collect(t, db, "ses_old", -3); len(neg) != 3 || back != 3 {
		t.Fatalf("negative cursor %d %d", len(neg), back)
	}
	stop := errors.New("stop")
	if off, err := db.Turns(context.Background(), "ses_A", 0, func(agentio.Turn) error { return stop }); !errors.Is(err, stop) || off != 1 {
		t.Fatalf("fn error %v %d", err, off)
	}
	if _, err := db.Turns(context.Background(), "ses_missing", 0, func(agentio.Turn) error { return nil }); !errors.Is(err, agentio.ErrNotFound) {
		t.Fatalf("missing %v", err)
	}
	all, err := db.Messages(context.Background(), "ses_A")
	if err != nil || len(all) != 5 || len(all[1].Parts) != 5 || all[1].Parts[3].Type != "tool" || all[1].Parts[3].Unknown["state"] == nil || all[1].Parts[0].ID != "prt_03" || all[1].Parts[0].MessageID != "msg_02" {
		t.Fatalf("messages %v", err)
	}
	if all[0].Info.Summary == nil || all[0].Info.Model.Variant != "high" || all[1].Info.Tokens.Total != 1300 {
		t.Fatalf("message info %+v", all[0].Info)
	}
	if none, err := db.Messages(context.Background(), "ses_child"); err != nil || len(none) != 0 {
		t.Fatalf("empty session %v %v", none, err)
	}
}

func TestOlderSchema(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "opencode.db")
	buildDB(t, path, olderSchema)
	db := Open(opencode.New(agenttest.NewHost(t)).DefaultHome(), path)
	defer func() { _ = db.Close() }()
	ss, err := db.Sessions(context.Background(), SessionQuery{Roots: true, Active: true})
	if err != nil || len(ss) != 1 || ss[0].Agent != "" || ss[0].Tokens != (opencode.Tokens{}) || ss[0].Model != nil {
		t.Fatalf("older schema %+v %v", ss, err)
	}
	if ts, off := collect(t, db, "ses_B", 0); len(ts) != 0 || off != 0 {
		t.Fatalf("older turns %d %d", len(ts), off)
	}
}

func TestErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	h := opencode.New(agenttest.NewHost(t)).DefaultHome()
	missing := Open(h, filepath.Join(t.TempDir(), "opencode.db"))
	if _, err := missing.Sessions(ctx, SessionQuery{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := missing.Turns(ctx, "x", 0, func(agentio.Turn) error { return nil }); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file turns: %v", err)
	}
	if err := missing.Close(); err != nil {
		t.Fatal(err)
	}
	// A database without the tables, and one missing a required column.
	empty := filepath.Join(t.TempDir(), "opencode.db")
	buildDB(t, empty, "CREATE TABLE other (x integer)")
	if _, err := Open(h, empty).Sessions(ctx, SessionQuery{}); err == nil || !strings.Contains(err.Error(), "no session table") {
		t.Fatalf("no table: %v", err)
	}
	if _, err := Open(h, empty).Messages(ctx, "x"); err == nil {
		t.Fatal("messages without tables")
	}
	thin := filepath.Join(t.TempDir(), "opencode.db")
	buildDB(t, thin, "CREATE TABLE session (id text)")
	if _, err := Open(h, thin).Session(ctx, "x"); err == nil || !strings.Contains(err.Error(), "no column") {
		t.Fatalf("thin: %v", err)
	}
	// Broken JSON in a model column and in a message row.
	db, w := fixture(t)
	if _, err := w.Exec(`UPDATE session SET model = '{' WHERE id = 'ses_old'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Session(ctx, "ses_old"); err == nil {
		t.Fatal("broken model decoded")
	}
	if _, err := w.Exec(`UPDATE message SET data = '[' WHERE id = 'msg_04'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Turns(ctx, "ses_A", 0, func(agentio.Turn) error { return nil }); err == nil {
		t.Fatal("broken message decoded")
	}
	if _, err := w.Exec(`UPDATE part SET data = '[' WHERE id = 'prt_11'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Messages(ctx, "ses_old"); err == nil {
		t.Fatal("broken part decoded")
	}
	m := opencode.New(agenttest.NewHost(t, agenttest.WithEnv("HOME=/h", opencode.DBEnv+"=:memory:")))
	if _, err := OpenHome(m, m.DefaultHome()); !errors.Is(err, ErrNoDatabase) {
		t.Fatalf("memory: %v", err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := db.Sessions(cctx, SessionQuery{}); err == nil {
		t.Fatal("canceled query ran")
	}
}

// TestReadOnly proves the connection refuses writes and still sees rows a
// writer left in the WAL (no immutable=1).
func TestReadOnly(t *testing.T) {
	t.Parallel()
	db, w := fixture(t)
	if db.Path() == "" || filepath.Base(db.Path()) != "opencode.db" {
		t.Fatalf("path %s", db.Path())
	}
	if _, err := db.Sessions(context.Background(), SessionQuery{}); err != nil {
		t.Fatal(err)
	}
	c, err := db.db()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Exec("DELETE FROM session"); err == nil {
		t.Fatal("read-only connection accepted a write")
	}
	if _, err := w.Exec(`INSERT INTO session (id, project_id, slug, directory, title, version, time_created, time_updated) VALUES ('ses_new', 'global', 'n', '/n', 'New', '1.18.30', 1, 9999999999999)`); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(db.Path() + "-wal"); err != nil {
		t.Fatalf("no WAL file: %v", err)
	}
	ss, err := db.Sessions(context.Background(), SessionQuery{Limit: 1})
	if err != nil || len(ss) != 1 || ss[0].ID != "ses_new" || ss[0].Updated.Before(time.Unix(0, 0)) {
		t.Fatalf("WAL row not seen: %+v %v", ss, err)
	}
}

func TestDSN(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"/home/u/.local/share/opencode/opencode.db":    "/home/u/.local/share/opencode/opencode.db",
		"C:/Users/u/.local/share/opencode/opencode.db": "/C:/Users/u/.local/share/opencode/opencode.db",
		"d:/x.db":             "/d:/x.db",
		"//server/share/x.db": "//server/share/x.db",
		"x.db":                "x.db",
		"":                    "",
	} {
		if got := fileURIPath(in); got != want {
			t.Errorf("fileURIPath(%q) = %q, want %q", in, got, want)
		}
	}
	got := dsn("C:/Users/u/opencode.db")
	if !strings.HasPrefix(got, "file:///C:/Users/u/opencode.db?") {
		t.Fatalf("dsn = %q", got)
	}
	u, err := url.Parse(got)
	if err != nil || u.Host != "" || u.Query().Get("mode") != "ro" || len(u.Query()["_pragma"]) != 2 {
		t.Fatalf("dsn query %v %v", u, err)
	}
}
