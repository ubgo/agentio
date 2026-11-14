package state

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/codex"
)

// migrationDirs maps each store to its upstream migrations, copied
// verbatim from codex-rs/state at rust-v0.162.1 into testdata.
func migrationDir(f File) string {
	return filepath.Join("testdata", "migrations", strings.TrimSuffix(f.String(), ".sqlite"))
}

// buildDB creates f under dir by applying the upstream migrations up to
// upTo (0 = all) and recording them in _sqlx_migrations as sqlx does.
// Test-only writer: the package itself never opens a file writable.
func buildDB(t *testing.T, dir string, f File, upTo int64, seed string) {
	t.Helper()
	entries, err := os.ReadDir(migrationDir(f))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	db, err := sql.Open(driverName, filepath.Join(dir, f.String()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE _sqlx_migrations (version BIGINT PRIMARY KEY, description TEXT NOT NULL,
		installed_on TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, success BOOLEAN NOT NULL, checksum BLOB NOT NULL, execution_time BIGINT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		v, err := strconv.ParseInt(strings.SplitN(n, "_", 2)[0], 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		if upTo > 0 && v > upTo {
			break
		}
		body, err := os.ReadFile(filepath.Join(migrationDir(f), n))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(body)); err != nil {
			t.Fatalf("%s %s: %v", f, n, err)
		}
		if _, err := db.Exec(`INSERT INTO _sqlx_migrations (version, description, success, checksum, execution_time) VALUES (?, ?, 1, x'00', 0)`, v, n); err != nil {
			t.Fatal(err)
		}
	}
	if seed != "" {
		if _, err := db.Exec(seed); err != nil {
			t.Fatalf("seed %s: %v", f, err)
		}
	}
}

// Redacted fixture rows: ids are made-up uuid v7 shapes, paths are /work.
const (
	threadA = "019a0000-0000-7000-8000-00000000000a"
	threadB = "019a0000-0000-7000-8000-00000000000b"
	threadC = "019a0000-0000-7000-8000-00000000000c"
)

const seedState = `
INSERT INTO threads (id, rollout_path, created_at, updated_at, created_at_ms, updated_at_ms, recency_at_ms, source, model_provider, model,
  reasoning_effort, cwd, title, name, preview, first_user_message, sandbox_policy, approval_mode, tokens_used, has_user_event, archived,
  git_sha, git_branch, git_origin_url, cli_version, memory_mode, history_mode, thread_source, is_pinned, originator, creator_user_id)
VALUES
 ('` + threadA + `', '/home/u/.codex/sessions/2026/11/01/rollout-2026-11-01T10-00-00-` + threadA + `.jsonl', 1793000000, 1793000600, 1793000000000, 1793000600000, 1793000600000,
  'vscode', 'openai', 'gpt-5.5', 'medium', '/work/app', 'Fix the build', NULL, 'fix the build', 'fix the build',
  '{"type":"workspace-write","writable_roots":[],"network_access":false}', 'on-request', 1234, 1, 0,
  'abc123', 'main', 'https://example.com/repo.git', '0.162.1', 'enabled', 'paginated', 'user', 1, 'Codex Desktop', '<user>'),
 ('` + threadB + `', '/home/u/.codex/sessions/2026/11/02/rollout-b.jsonl', 1793100000, 1793100600, 1793100000000, 1793100600000, 1793100600000,
  '{"subagent":{"thread_spawn":{"parent_thread_id":"` + threadA + `","depth":1}}}', 'openai', NULL,
  NULL, '/work/app', 'Sub task', 'renamed', '', '', '"danger-full-access"', 'never', 10, 0, 1,
  NULL, NULL, NULL, '', 'enabled', 'legacy', 'subagent', 0, 'codex-tui', NULL),
 ('` + threadC + `', '/home/u/.codex/sessions/2026/11/03/rollout-c.jsonl', 1793200000, 1793200600, 1793200000000, 1793200600000, 1793200600000,
  'cli', 'openai', 'gpt-5.5', 'xhigh', '/work/other', 'Other', NULL, 'p', 'p', '{"type":"brand-new-sandbox","weird":[1]}', 'on-request', 5, 1, 0,
  NULL, NULL, NULL, 'garbage', 'enabled', 'paginated', 'user', 0, 'codex-tui', NULL);
UPDATE threads SET agent_nickname = 'helper', agent_role = 'worker', agent_path = '/root/helper' WHERE id = '` + threadB + `';
UPDATE threads SET archived_at = 1793100700 WHERE id = '` + threadB + `';
INSERT INTO thread_spawn_edges (parent_thread_id, child_thread_id, status) VALUES ('` + threadA + `', '` + threadB + `', 'closed');
INSERT INTO projects (id, name, metadata, position, created_at_ms, updated_at_ms) VALUES ('p1', 'App', '{}', 0, 1793000000000, 1793000000000);
INSERT INTO project_roots (project_id, position, path) VALUES ('p1', 1, '/work/app2'), ('p1', 0, '/work/app');
UPDATE threads SET project_id = 'p1' WHERE id = '` + threadA + `';
`

const seedHistory = `
INSERT INTO thread_turns (thread_id, turn_id, rollout_ordinal, status, error_json, started_at, completed_at, duration_ms, first_user_item_id, final_agent_item_id, rollout_byte_offset)
VALUES ('` + threadA + `', 't1', 3, 'completed', NULL, 1793000000, 1793000060, 60000, 'i1', 'i2', 2048),
       ('` + threadA + `', 't2', 9, 'failed', '{"message":"boom"}', 1793000100, 1793000110, 10000, 'i3', NULL, 4096);
INSERT INTO thread_items (thread_id, turn_id, item_id, rollout_ordinal, created_at_ms, item_json, item_type, updated_at_ordinal, started_at_ms, completed_at_ms)
VALUES ('` + threadA + `', 't1', 'i1', 4, 1793000000000, '{"type":"userMessage","id":"i1","content":[]}', 'userMessage', 4, NULL, NULL),
       ('` + threadA + `', 't1', 'i2', 5, 1793000050000, '{"type":"agentMessage","id":"i2","text":"done"}', 'agentMessage', 5, 1793000040000, 1793000050000),
       ('` + threadA + `', 't2', 'i3', 10, 1793000100000, '{"type":"userMessage","id":"i3","content":[]}', 'userMessage', 10, NULL, NULL);
`

const seedGoals = `INSERT INTO thread_goals (thread_id, goal_id, objective, status, token_budget, tokens_used, time_used_seconds, created_at_ms, updated_at_ms)
VALUES ('` + threadA + `', 'g1', 'ship it', 'budget_limited', 1000, 1200, 90, 1793000000000, 1793000100000);`

const seedQueue = `INSERT INTO queued_items (id, thread_id, payload_json, queue_order, created_at_ms, updated_at_ms)
VALUES ('q2', '` + threadA + `', '{"text":"second"}', 2, 1793000000000, 1793000000000),
       ('q1', '` + threadA + `', '{"text":"first"}', 1, 1793000000000, 1793000000000);`

const seedMemories = `INSERT INTO stage1_outputs (thread_id, source_updated_at, raw_memory, rollout_summary, rollout_slug, generated_at, usage_count, last_usage, selected_for_phase2)
VALUES ('` + threadA + `', 1793000600, 'mem', 'summary', 'fix-build', 1793000700, 2, 1793000800, 1),
       ('` + threadC + `', 1793200600, 'mem2', 'summary2', NULL, 1793200700, NULL, NULL, 0);
INSERT INTO jobs (kind, job_key, status, worker_id, ownership_token, started_at, finished_at, retry_remaining, last_error)
VALUES ('stage1', '` + threadA + `', 'done', 'w', 'secret-token', 1793000600, 1793000700, 3, NULL);`

const seedLogs = `INSERT INTO logs (ts, ts_nanos, level, target, feedback_log_body, module_path, file, line, thread_id, process_uuid)
VALUES (1793000000, 5, 'INFO', 'codex_core', 'started', 'codex_core::x', 'x.rs', 10, '` + threadA + `', 'proc'),
       (1793000010, 0, 'WARN', 'codex_core', 'slow', NULL, NULL, NULL, '` + threadA + `', 'proc'),
       (1793000020, 0, 'INFO', 'codex_tui', 'other', NULL, NULL, NULL, NULL, 'proc');`

// fixtureDB builds every store in a temp home and opens it read-only.
func fixtureDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	buildDB(t, dir, FileState, 0, seedState)
	buildDB(t, dir, FileThreadHistory, 0, seedHistory)
	buildDB(t, dir, FileGoals, 0, seedGoals)
	buildDB(t, dir, FileQueue, 0, seedQueue)
	buildDB(t, dir, FileMemories, 0, seedMemories)
	buildDB(t, dir, FileLogs, 0, seedLogs)
	return openDir(t, dir)
}

func openDir(t *testing.T, dir string) *DB {
	t.Helper()
	// Paths built directly: Open needs only the six DB paths, and this keeps
	// the test independent of home discovery in package codex.
	j := func(f File) string { return filepath.Join(dir, f.String()) }
	db := Open(codex.Paths{
		Home: dir, SQLiteHome: dir,
		StateDB: j(FileState), ThreadHistoryDB: j(FileThreadHistory), GoalsDB: j(FileGoals),
		QueueDB: j(FileQueue), MemoriesDB: j(FileMemories), LogsDB: j(FileLogs),
	})
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSchemaVersion(t *testing.T) {
	db := fixtureDB(t)
	ctx := context.Background()
	for f := FileState; f < fileCount; f++ {
		v, err := db.SchemaVersion(ctx, f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if v != f.MaxKnownMigration() {
			t.Errorf("%s: version %d, want %d", f, v, f.MaxKnownMigration())
		}
	}
}

func TestThreads(t *testing.T) {
	db := fixtureDB(t)
	ctx := context.Background()
	all, err := db.Threads(ctx, ThreadStatusFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].ID != threadC || all[2].ID != threadA {
		t.Fatalf("order: %+v", ids(all))
	}
	a := all[2]
	if a.Source != codex.SourceVSCode || a.Model == nil || *a.Model != "gpt-5.5" || a.ReasoningEffort != codex.EffortMedium {
		t.Errorf("thread A fields: %+v", a)
	}
	if a.Sandbox.Type != codex.ModeWorkspaceWrite || a.Approval != codex.ApprovalOnRequest || a.TokensUsed != 1234 {
		t.Errorf("thread A policy: %+v %v", a.Sandbox, a.Approval)
	}
	if !a.Pinned || a.Archived || !a.HasUserEvent || a.HistoryMode != HistoryPaginated {
		t.Errorf("thread A flags: %+v", a)
	}
	if a.Git == nil || a.Git.Branch != "main" || a.CLIVersion.Minor != 162 {
		t.Errorf("thread A git/version: %+v %+v", a.Git, a.CLIVersion)
	}
	if !a.Created.Equal(time.UnixMilli(1793000000000).UTC()) || a.ProjectID == nil || *a.ProjectID != "p1" {
		t.Errorf("thread A created/project: %v %v", a.Created, a.ProjectID)
	}
	if strings.Contains(a.CreatorUserID.String(), "<user>") {
		t.Error("creator id not redacted")
	}
	b := all[1]
	if b.Source != codex.SourceSubagent || b.Sandbox.Type != codex.ModeDangerFullAccess || !b.Archived || b.ArchivedAt == nil {
		t.Errorf("thread B: %+v", b)
	}
	if b.Agent == nil || b.Agent.Nickname != "helper" || b.Name == nil || *b.Name != "renamed" {
		t.Errorf("thread B agent/name: %+v %v", b.Agent, b.Name)
	}
	c := all[0]
	if len(c.Warnings) != 1 || c.Sandbox.Type != "brand-new-sandbox" {
		// cli_version "garbage" warns; the unknown sandbox type decodes as an open value.
		t.Errorf("thread C warnings: %+v sandbox %+v", c.Warnings, c.Sandbox)
	}

	no := false
	live, err := db.Threads(ctx, ThreadStatusFilter{Archived: &no, Dir: "/work/app"})
	if err != nil || len(live) != 1 || live[0].ID != threadA {
		t.Fatalf("filter: %v %v", ids(live), err)
	}
	yes := true
	pinned, err := db.Threads(ctx, ThreadStatusFilter{Pinned: &yes})
	if err != nil || len(pinned) != 1 {
		t.Fatalf("pinned: %v %v", ids(pinned), err)
	}
	byProject, err := db.Threads(ctx, ThreadStatusFilter{ProjectID: "p1", Limit: 1})
	if err != nil || len(byProject) != 1 {
		t.Fatalf("project: %v %v", ids(byProject), err)
	}

	one, err := db.Thread(ctx, threadB)
	if err != nil || one.ID != threadB {
		t.Fatalf("thread: %v", err)
	}
	if _, err := db.Thread(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func ids(ts []Thread) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.ID)
	}
	return out
}

func TestSubAgentsAndProjects(t *testing.T) {
	db := fixtureDB(t)
	ctx := context.Background()
	edges, err := db.SubAgents(ctx, threadA)
	if err != nil || len(edges) != 1 || edges[0].ChildID != threadB || edges[0].Status != EdgeClosed {
		t.Fatalf("edges: %+v %v", edges, err)
	}
	ps, err := db.Projects(ctx)
	if err != nil || len(ps) != 1 || len(ps[0].Roots) != 2 || ps[0].Roots[0] != "/work/app" {
		t.Fatalf("projects: %+v %v", ps, err)
	}
}

func TestTurnsAndItems(t *testing.T) {
	db := fixtureDB(t)
	ctx := context.Background()
	turns, err := db.Turns(ctx, threadA)
	if err != nil || len(turns) != 2 {
		t.Fatalf("turns: %+v %v", turns, err)
	}
	if turns[0].Status != TurnCompleted || turns[0].Duration != time.Minute || *turns[0].RolloutByteOffset != 2048 || turns[0].RootTurnID != nil {
		t.Errorf("turn 1: %+v", turns[0])
	}
	if turns[1].Status != TurnFailed || string(turns[1].Error) != `{"message":"boom"}` {
		t.Errorf("turn 2: %+v", turns[1])
	}
	items, err := db.Items(ctx, threadA, 4, 1)
	if err != nil || len(items) != 1 || items[0].ItemID != "i2" || items[0].Type != ItemAgentMessage || items[0].Completed == nil {
		t.Fatalf("items: %+v %v", items, err)
	}
	all, err := db.Items(ctx, threadA, 0, 0)
	if err != nil || len(all) != 3 {
		t.Fatalf("all items: %d %v", len(all), err)
	}
}

func TestSmallStores(t *testing.T) {
	db := fixtureDB(t)
	ctx := context.Background()
	g, ok, err := db.Goal(ctx, threadA)
	if err != nil || !ok || g.Status != GoalBudgetLimited || g.TokenBudget == nil || *g.TokenBudget != 1000 || g.TimeUsed != 90*time.Second {
		t.Fatalf("goal: %+v %v %v", g, ok, err)
	}
	if _, ok, err := db.Goal(ctx, threadC); ok || err != nil {
		t.Fatalf("absent goal: %v %v", ok, err)
	}
	q, err := db.Queue(ctx, threadA)
	if err != nil || len(q) != 2 || q[0].ID != "q1" {
		t.Fatalf("queue: %+v %v", q, err)
	}
	mems, err := db.Memories(ctx, 0)
	if err != nil || len(mems) != 2 || mems[0].ThreadID != threadC || mems[1].RolloutSlug == nil || !mems[1].SelectedPhase2 {
		t.Fatalf("memories: %+v %v", mems, err)
	}
	jobs, err := db.MemoryJobs(ctx)
	if err != nil || len(jobs) != 1 || jobs[0].Status != JobDone || jobs[0].Finished == nil {
		t.Fatalf("jobs: %+v %v", jobs, err)
	}
	logs, err := db.Logs(ctx, LogQuery{ThreadID: threadA})
	if err != nil || len(logs) != 2 || logs[0].Body != "slow" || logs[1].Line == nil {
		t.Fatalf("logs: %+v %v", logs, err)
	}
	warn, err := db.Logs(ctx, LogQuery{Level: LevelWarn, Limit: 5})
	if err != nil || len(warn) != 1 {
		t.Fatalf("warn logs: %+v %v", warn, err)
	}
	recent, err := db.Logs(ctx, LogQuery{Since: time.Unix(1793000015, 0)})
	if err != nil || len(recent) != 1 || recent[0].Target != "codex_tui" {
		t.Fatalf("since logs: %+v %v", recent, err)
	}
}

func TestOlderSchemaReads(t *testing.T) {
	dir := t.TempDir()
	// Migration 0001 only: no ms columns, no model, no history_mode.
	buildDB(t, dir, FileState, 1, `INSERT INTO threads (id, rollout_path, created_at, updated_at, source, model_provider, cwd, title, sandbox_policy, approval_mode)
		VALUES ('`+threadA+`', '/r.jsonl', 1793000000, 1793000600, 'cli', 'openai', '/work', 't', '{"type":"read-only"}', 'untrusted')`)
	db := openDir(t, dir)
	ts, err := db.Threads(context.Background(), ThreadStatusFilter{ProjectID: ""})
	if err != nil || len(ts) != 1 {
		t.Fatalf("old threads: %v %v", ts, err)
	}
	got := ts[0]
	if got.Model != nil || got.HistoryMode != HistoryLegacy || !got.Updated.Equal(time.Unix(1793000600, 0).UTC()) || got.Approval != codex.ApprovalUntrusted {
		t.Errorf("old thread: %+v", got)
	}
	if ps, err := db.Projects(context.Background()); err != nil || ps != nil {
		t.Errorf("old projects: %v %v", ps, err)
	}
	if es, err := db.SubAgents(context.Background(), threadA); err != nil || es != nil {
		t.Errorf("old edges: %v %v", es, err)
	}
}

func TestSchemaTooNew(t *testing.T) {
	dir := t.TempDir()
	buildDB(t, dir, FileGoals, 0, `INSERT INTO _sqlx_migrations (version, description, success, checksum, execution_time) VALUES (3, 'future', 1, x'00', 0)`)
	buildDB(t, dir, FileState, 0, "")
	db := openDir(t, dir)
	_, _, err := db.Goal(context.Background(), threadA)
	var tooNew *SchemaTooNewError
	if !errors.As(err, &tooNew) || !errors.Is(err, ErrSchemaTooNew) || tooNew.Found != 3 || tooNew.File != FileGoals {
		t.Fatalf("want SchemaTooNewError, got %v", err)
	}
}

func TestMissingFileAndReadOnly(t *testing.T) {
	dir := t.TempDir()
	buildDB(t, dir, FileState, 0, seedState)
	db := openDir(t, dir)
	if _, err := db.Logs(context.Background(), LogQuery{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing logs: %v", err)
	}
	c, err := db.conn(FileState)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Exec("DELETE FROM threads"); err == nil {
		t.Fatal("read-only connection accepted a write")
	}
}

func TestFileText(t *testing.T) {
	b, err := FileLogs.MarshalText()
	if err != nil || string(b) != "logs_2.sqlite" {
		t.Fatalf("%s %v", b, err)
	}
	if File(99).MaxKnownMigration() != 0 || !strings.HasPrefix(File(99).String(), "File(") {
		t.Fatal("unknown file")
	}
}

func TestThreadConversation(t *testing.T) {
	db := fixtureDB(t)
	th, err := db.Thread(context.Background(), threadB)
	if err != nil {
		t.Fatal(err)
	}
	c := th.Conversation(codex.Home{})
	if c.ID != threadB || c.Title != "renamed" || !c.Archived || c.Originator != "codex-tui" || c.Dir != "/work/app" {
		t.Fatalf("conversation: %+v", c)
	}
}

// TestOpenWithSomePaths: Open needs only the paths of the stores a caller
// uses. A store with no path fails with *NoPathError (matching ErrNoPath)
// only when it is queried; the others work.
func TestOpenWithSomePaths(t *testing.T) {
	dir := t.TempDir()
	buildDB(t, dir, FileState, maxMigrationState, seedState)
	db := Open(codex.Paths{StateDB: filepath.Join(dir, FileState.String())})
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	if _, err := db.SchemaVersion(ctx, FileState); err != nil {
		t.Fatalf("state store with its path: %v", err)
	}
	_, _, err := db.Goal(ctx, "t1")
	var np *NoPathError
	if !errors.As(err, &np) || !errors.Is(err, ErrNoPath) || np.File != FileGoals {
		t.Fatalf("goals without a path: %v, want *NoPathError for %s", err, FileGoals)
	}
}

// TestTurnErrorOmittedWhenNil: a turn without error_json encodes without
// an Error key instead of "Error":null; a real error stays verbatim.
func TestTurnErrorOmittedWhenNil(t *testing.T) {
	b, err := json.Marshal(Turn{TurnID: "t1"})
	if err != nil || bytes.Contains(b, []byte(`"Error"`)) {
		t.Fatalf("nil Error encoded: %s %v", b, err)
	}
	b, err = json.Marshal(Turn{TurnID: "t2", Error: agentio.Raw(`{"message":"boom"}`)})
	if err != nil || !bytes.Contains(b, []byte(`"Error":{"message":"boom"}`)) {
		t.Fatalf("error not kept: %s %v", b, err)
	}
}
