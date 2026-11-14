// Command codexstate reads Codex's SQLite stores with codex/state: thread
// index, sub-agent edges, Desktop projects, turns and items, goals, the
// message queue, memories and logs.
//
// No database is checked in. The example builds the six stores in a temp
// home from the upstream migrations kept in codex/state/testdata (the same
// SQL Codex runs), seeds a few made-up rows, and then reads them only
// through the read-only state.DB.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/agenttest"
	"github.com/ubgo/agentio/codex"
	"github.com/ubgo/agentio/codex/state"
)

// defaultMigrations holds upstream migrations per store, relative to the
// examples module.
const defaultMigrations = "../codex/state/testdata/migrations"

// Fixture facts.
const (
	driverName  = "sqlite" // registered by codex/state (pure-Go modernc.org/sqlite), imported above
	codexDir    = ".codex"
	fixtureUser = "user"
	dirPerm     = os.FileMode(0o700)
	threadA     = "019a0000-0000-7000-8000-00000000000a" // a Desktop thread with turns
	threadB     = "019a0000-0000-7000-8000-00000000000b" // a sub-agent thread A spawned
)

// sqlxMigrations is the table sqlx records applied migrations in; state
// reads its newest version to refuse a schema newer than it knows.
const sqlxMigrations = `CREATE TABLE _sqlx_migrations (version BIGINT PRIMARY KEY, description TEXT NOT NULL,
installed_on TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, success BOOLEAN NOT NULL, checksum BLOB NOT NULL, execution_time BIGINT NOT NULL)`

// seeds are made-up rows per store (ids are uuid v7 shapes, paths /work).
// A function, not a var: the repo allows no package-level vars.
func seeds() map[state.File]string {
	return map[state.File]string{
		state.FileState: `
INSERT INTO threads (id, rollout_path, created_at, updated_at, created_at_ms, updated_at_ms, recency_at_ms, source, model_provider, model,
  reasoning_effort, cwd, title, preview, first_user_message, sandbox_policy, approval_mode, tokens_used, has_user_event, archived,
  git_sha, git_branch, git_origin_url, cli_version, memory_mode, history_mode, thread_source, is_pinned, originator)
VALUES
 ('` + threadA + `', '/home/user/.codex/sessions/2026/11/01/rollout-2026-11-01T10-00-00-` + threadA + `.jsonl', 1793000000, 1793000600,
  1793000000000, 1793000600000, 1793000600000, 'vscode', 'openai', 'gpt-5.5', 'medium', '/work/app', 'Fix the build', 'fix the build',
  'fix the build', '{"type":"workspace-write","writable_roots":[],"network_access":false}', 'on-request', 1234, 1, 0,
  'abc123', 'main', 'https://example.test/repo.git', '0.162.1', 'enabled', 'paginated', 'user', 1, 'Codex Desktop'),
 ('` + threadB + `', '/home/user/.codex/sessions/2026/11/02/rollout-2026-11-02T10-00-00-` + threadB + `.jsonl', 1793100000, 1793100600,
  1793100000000, 1793100600000, 1793100600000, '{"subagent":{"thread_spawn":{"parent_thread_id":"` + threadA + `","depth":1}}}',
  'openai', NULL, NULL, '/work/app', 'Sub task', '', '', '"danger-full-access"', 'never', 10, 0, 1,
  NULL, NULL, NULL, '0.162.1', 'enabled', 'legacy', 'subagent', 0, 'codex-tui');
UPDATE threads SET agent_nickname = 'helper', agent_role = 'worker', agent_path = '/root/helper', archived_at = 1793100700 WHERE id = '` + threadB + `';
INSERT INTO thread_spawn_edges (parent_thread_id, child_thread_id, status) VALUES ('` + threadA + `', '` + threadB + `', 'closed');
INSERT INTO projects (id, name, metadata, position, created_at_ms, updated_at_ms) VALUES ('p1', 'App', '{}', 0, 1793000000000, 1793000000000);
INSERT INTO project_roots (project_id, position, path) VALUES ('p1', 0, '/work/app'), ('p1', 1, '/work/app-docs');
UPDATE threads SET project_id = 'p1' WHERE id = '` + threadA + `';`,
		state.FileThreadHistory: `
INSERT INTO thread_turns (thread_id, turn_id, rollout_ordinal, status, error_json, started_at, completed_at, duration_ms, first_user_item_id, final_agent_item_id, rollout_byte_offset)
VALUES ('` + threadA + `', 't1', 3, 'completed', NULL, 1793000000, 1793000060, 60000, 'i1', 'i2', 2048),
       ('` + threadA + `', 't2', 9, 'failed', '{"message":"boom"}', 1793000100, 1793000110, 10000, 'i3', NULL, 4096);
INSERT INTO thread_items (thread_id, turn_id, item_id, rollout_ordinal, created_at_ms, item_json, item_type, updated_at_ordinal, started_at_ms, completed_at_ms)
VALUES ('` + threadA + `', 't1', 'i1', 4, 1793000000000, '{"type":"userMessage","id":"i1","content":[]}', 'userMessage', 4, NULL, NULL),
       ('` + threadA + `', 't1', 'i2', 5, 1793000050000, '{"type":"agentMessage","id":"i2","text":"done"}', 'agentMessage', 5, 1793000040000, 1793000050000),
       ('` + threadA + `', 't2', 'i3', 10, 1793000100000, '{"type":"userMessage","id":"i3","content":[]}', 'userMessage', 10, NULL, NULL);`,
		state.FileGoals: `INSERT INTO thread_goals (thread_id, goal_id, objective, status, token_budget, tokens_used, time_used_seconds, created_at_ms, updated_at_ms)
VALUES ('` + threadA + `', 'g1', 'ship it', 'budget_limited', 1000, 1200, 90, 1793000000000, 1793000100000);`,
		state.FileQueue: `INSERT INTO queued_items (id, thread_id, payload_json, queue_order, created_at_ms, updated_at_ms)
VALUES ('q2', '` + threadA + `', '{"text":"second"}', 2, 1793000000000, 1793000000000),
       ('q1', '` + threadA + `', '{"text":"first"}', 1, 1793000000000, 1793000000000);`,
		state.FileMemories: `INSERT INTO stage1_outputs (thread_id, source_updated_at, raw_memory, rollout_summary, rollout_slug, generated_at, usage_count, last_usage, selected_for_phase2)
VALUES ('` + threadA + `', 1793000600, 'the build uses make', 'fixed the build', 'fix-build', 1793000700, 2, 1793000800, 1);
INSERT INTO jobs (kind, job_key, status, worker_id, ownership_token, started_at, finished_at, retry_remaining, last_error)
VALUES ('stage1', '` + threadA + `', 'done', 'w', 'not-read', 1793000600, 1793000700, 3, NULL);`,
		state.FileLogs: `INSERT INTO logs (ts, ts_nanos, level, target, feedback_log_body, module_path, file, line, thread_id, process_uuid)
VALUES (1793000000, 5, 'INFO', 'codex_core', 'started', 'codex_core::x', 'x.rs', 10, '` + threadA + `', 'proc'),
       (1793000010, 0, 'WARN', 'codex_core', 'slow tool call', NULL, NULL, NULL, '` + threadA + `', 'proc'),
       (1793000020, 0, 'INFO', 'codex_tui', 'other thread', NULL, NULL, NULL, NULL, 'proc');`,
	}
}

func main() {
	migrations := flag.String("migrations", defaultMigrations, "directory of upstream migrations per store")
	flag.Parse()
	if err := run(os.Stdout, *migrations); err != nil {
		fmt.Fprintln(os.Stderr, "codexstate:", err)
		os.Exit(1)
	}
}

// run builds the demo stores in a temp ~/.codex and reads them.
func run(w io.Writer, migrations string) error {
	userHome, err := os.MkdirTemp("", "codexstate-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(userHome) }()
	home := filepath.Join(userHome, codexDir)
	if err := os.MkdirAll(home, dirPerm); err != nil {
		return err
	}
	for f, seed := range seeds() {
		if err := buildStore(filepath.Join(home, f.String()), filepath.Join(migrations, strings.TrimSuffix(f.String(), ".sqlite")), seed); err != nil {
			return fmt.Errorf("build %s: %w", f, err)
		}
	}
	host := agentio.Host{
		UserHome:  userHome,
		User:      fixtureUser,
		Env:       []string{"HOME=" + userHome},
		Now:       agenttest.FixedNow,
		Secrets:   agenttest.NewFakeSecrets(),
		Processes: &agenttest.FakeProcesses{},
	}
	db, err := openState(w, codex.New(host))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	if err := listThreads(ctx, w, db, codex.New(host).DefaultHome()); err != nil {
		return err
	}
	if err := readTurnsItems(ctx, w, db); err != nil {
		return err
	}
	return readSmallStores(ctx, w, db)
}

// buildStore applies every migration in dir to a new SQLite file at path
// and records them the way sqlx does, then inserts seed. Demo-only
// writer: state itself opens every file read-only.
func buildStore(path, dir, seed string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	db, err := sql.Open(driverName, path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(sqlxMigrations); err != nil {
		return err
	}
	for _, n := range names {
		v, err := strconv.ParseInt(strings.SplitN(n, "_", 2)[0], 10, 64)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return err
		}
		if _, err := db.Exec(string(body)); err != nil {
			return fmt.Errorf("%s: %w", n, err)
		}
		if _, err := db.Exec(`INSERT INTO _sqlx_migrations (version, description, success, checksum, execution_time) VALUES (?, ?, 1, x'00', 0)`, v, n); err != nil {
			return err
		}
	}
	_, err = db.Exec(seed)
	return err
}

// snippet:start open
// openState opens the stores of the default home. Open reads nothing:
// each file is opened read-only on its first query. PathsWith honours
// sqlite_home and CODEX_SQLITE_HOME, which move the files out of the home.
func openState(w io.Writer, a *codex.Agent) (*state.DB, error) {
	h := a.DefaultHome()
	cfg, err := a.ConfigFile(h)
	if err != nil {
		return nil, err
	}
	db, err := state.Open(a.PathsWith(h, cfg))
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	for _, f := range []state.File{state.FileState, state.FileThreadHistory, state.FileLogs} {
		v, err := db.SchemaVersion(ctx, f)
		var tooNew *state.SchemaTooNewError
		switch {
		case errors.As(err, &tooNew):
			// A newer Codex may have renamed columns: refuse, don't guess.
			fmt.Fprintf(w, "%s: schema %d is newer than %d\n", f, tooNew.Found, tooNew.Max)
		case err != nil:
			return nil, err
		default:
			fmt.Fprintf(w, "%s at %s schema=%d max-known=%d\n", f, filepath.Base(db.Path(f)), v, f.MaxKnownMigration())
		}
	}
	return db, nil
}

// snippet:end open

// snippet:start threads
// listThreads reads the thread index: richer than rollout files (title,
// preview, tokens, archive flag, git info). Thread.Conversation projects
// a row onto the same codex.Conversation the rollout lister returns.
func listThreads(ctx context.Context, w io.Writer, db *state.DB, h codex.Home) error {
	ts, err := db.Threads(ctx, state.ThreadStatusFilter{Limit: 50})
	if err != nil {
		return err
	}
	for _, t := range ts {
		fmt.Fprintf(w, "thread %s %q source=%s archived=%v pinned=%v tokens=%d history=%s sandbox=%s\n",
			t.ID, t.Title, t.Source, t.Archived, t.Pinned, t.TokensUsed, t.HistoryMode, t.Sandbox.Type)
	}
	notArchived := false
	active, err := db.Threads(ctx, state.ThreadStatusFilter{Archived: &notArchived, Dir: "/work/app"})
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "active threads in /work/app: %d\n", len(active))

	a, err := db.Thread(ctx, threadA)
	if err != nil {
		return err
	}
	c := a.Conversation(h)
	fmt.Fprintf(w, "as conversation: %q dir=%s originator=%s branch=%s\n", c.Title, c.Dir, c.Originator, c.Git.Branch)
	if _, err := db.Thread(ctx, "019a0000-0000-7000-8000-0000000000ff"); errors.Is(err, state.ErrNotFound) {
		fmt.Fprintln(w, "unknown thread: ErrNotFound")
	}

	edges, err := db.SubAgents(ctx, threadA)
	if err != nil {
		return err
	}
	for _, e := range edges {
		child, err := db.Thread(ctx, e.ChildID)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "spawned %s status=%s nickname=%s role=%s\n", e.ChildID, e.Status, child.Agent.Nickname, child.Agent.Role)
	}

	projects, err := db.Projects(ctx)
	if err != nil {
		return err
	}
	for _, p := range projects {
		fmt.Fprintf(w, "project %s %q roots=%v\n", p.ID, p.Name, p.Roots)
	}
	return nil
}

// snippet:end threads

// snippet:start turns
// readTurnsItems reads a paginated thread's turns and pages through its
// items by rollout ordinal. A legacy thread (HistoryMode legacy) has no
// rows here: its history lives only in the rollout file.
func readTurnsItems(ctx context.Context, w io.Writer, db *state.DB) error {
	turns, err := db.Turns(ctx, threadA)
	if err != nil {
		return err
	}
	for _, t := range turns {
		fmt.Fprintf(w, "turn %s status=%s duration=%s", t.TurnID, t.Status, t.Duration)
		if len(t.Error) > 0 { // nil when error_json is NULL
			fmt.Fprintf(w, " error=%s", t.Error)
		}
		fmt.Fprintln(w)
	}
	const pageSize = 2
	var after int64
	for page := 1; ; page++ {
		items, err := db.Items(ctx, threadA, after, pageSize)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			fmt.Fprintf(w, "page %d item %s type=%s known=%v ordinal=%d\n", page, it.ItemID, it.Type, it.Type.IsKnown(), it.RolloutOrdinal)
		}
		after = items[len(items)-1].RolloutOrdinal
	}
	legacy, err := db.Turns(ctx, threadB)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "legacy thread turns: %d\n", len(legacy))
	return nil
}

// snippet:end turns

// snippet:start small
// readSmallStores reads the per-thread goal and message queue, the
// memories and memory jobs, and the newest warning log lines. Each store
// is its own file, opened only when first queried.
func readSmallStores(ctx context.Context, w io.Writer, db *state.DB) error {
	g, found, err := db.Goal(ctx, threadA)
	if err != nil {
		return err
	}
	if found {
		fmt.Fprintf(w, "goal %q status=%s used=%d budget=%d time=%s\n", g.Objective, g.Status, g.TokensUsed, *g.TokenBudget, g.TimeUsed)
	}

	queue, err := db.Queue(ctx, threadA)
	if err != nil {
		return err
	}
	for _, q := range queue {
		fmt.Fprintf(w, "queued %s order=%d payload=%s\n", q.ID, q.Order, q.Payload)
	}

	mems, err := db.Memories(ctx, 10)
	if err != nil {
		return err
	}
	for _, m := range mems {
		fmt.Fprintf(w, "memory thread=%s summary=%q phase2=%v\n", m.ThreadID, m.RolloutSummary, m.SelectedPhase2)
	}
	jobs, err := db.MemoryJobs(ctx)
	if err != nil {
		return err
	}
	for _, j := range jobs {
		fmt.Fprintf(w, "memory job %s status=%s retries-left=%d\n", j.Kind, j.Status, j.RetryRemaining)
	}

	logs, err := db.Logs(ctx, state.LogQuery{ThreadID: threadA, Level: state.LevelWarn, Since: time.Unix(1793000000, 0), Limit: 20})
	if err != nil {
		return err
	}
	for _, l := range logs {
		fmt.Fprintf(w, "log %s %s %s %q\n", l.Time.UTC().Format(time.RFC3339), l.Level, l.Target, l.Body)
	}
	return nil
}

// snippet:end small
