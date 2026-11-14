package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/khanakia/agentcli"
)

// Tables of the four small stores (upstream goals_migrations,
// queue_migrations, memory_migrations, logs_migrations).
const (
	tableGoals    = "thread_goals"
	tableQueue    = "queued_items"
	tableStage1   = "stage1_outputs"
	tableJobs     = "jobs"
	tableLogs     = "logs"
	colLogMessage = "feedback_log_body" // logs_2 0002 renamed message → feedback_log_body
	colLogOldMsg  = "message"
)

// GoalStatus is thread_goals.status (CHECK constraint in goals 0001).
type GoalStatus string

// GoalStatus values.
const (
	GoalActive        GoalStatus = "active"
	GoalPaused        GoalStatus = "paused"
	GoalBlocked       GoalStatus = "blocked"
	GoalUsageLimited  GoalStatus = "usage_limited"
	GoalBudgetLimited GoalStatus = "budget_limited"
	GoalComplete      GoalStatus = "complete"
)

// IsKnown reports whether s is a known goal status. Pure.
func (s GoalStatus) IsKnown() bool {
	switch s {
	case GoalActive, GoalPaused, GoalBlocked, GoalUsageLimited, GoalBudgetLimited, GoalComplete:
		return true
	}
	return false
}

// Goal is a thread's long-running objective (`/goal`).
type Goal struct {
	ThreadID    string
	GoalID      string
	Objective   string
	Status      GoalStatus
	TokenBudget *int64 // nil = no budget
	TokensUsed  int64
	TimeUsed    time.Duration
	Created     time.Time
	Updated     time.Time
}

// Goal returns the goal of threadID; false when the thread has none.
// Disk · Read (goals_1.sqlite).
func (d *DB) Goal(ctx context.Context, threadID string) (Goal, bool, error) {
	c, err := d.ready(ctx, FileGoals)
	if err != nil {
		return Goal{}, false, err
	}
	var g Goal
	var status string
	var budget, created, updated sql.NullInt64
	var secs int64
	err = c.QueryRowContext(ctx, "SELECT thread_id, goal_id, objective, status, token_budget, tokens_used, time_used_seconds, created_at_ms, updated_at_ms FROM "+
		tableGoals+" WHERE thread_id = ?", threadID).
		Scan(&g.ThreadID, &g.GoalID, &g.Objective, &status, &budget, &g.TokensUsed, &secs, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return Goal{}, false, nil
	}
	if err != nil {
		return Goal{}, false, fmt.Errorf("codex/state: goal: %w", err)
	}
	g.Status = GoalStatus(status)
	g.TokenBudget = int64Ptr(budget)
	g.TimeUsed = time.Duration(secs) * time.Second
	g.Created, g.Updated = fromMillis(created), fromMillis(updated)
	return g, true, nil
}

// QueuedItem is a message queued for a thread while it is busy.
type QueuedItem struct {
	ID       string
	ThreadID string
	Order    int64
	Payload  agentcli.Raw // payload_json, vendor-defined
	Created  time.Time
	Updated  time.Time
}

// Queue lists a thread's queued messages in send order. Disk · Read
// (queue_1.sqlite).
func (d *DB) Queue(ctx context.Context, threadID string) ([]QueuedItem, error) {
	c, err := d.ready(ctx, FileQueue)
	if err != nil {
		return nil, err
	}
	rows, err := c.QueryContext(ctx, "SELECT id, thread_id, queue_order, payload_json, created_at_ms, updated_at_ms FROM "+
		tableQueue+" WHERE thread_id = ? ORDER BY queue_order LIMIT ?", threadID, maxLimit)
	if err != nil {
		return nil, fmt.Errorf("codex/state: queue: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []QueuedItem
	for rows.Next() {
		var q QueuedItem
		var payload string
		var created, updated sql.NullInt64
		if err := rows.Scan(&q.ID, &q.ThreadID, &q.Order, &payload, &created, &updated); err != nil {
			return nil, fmt.Errorf("codex/state: queue: %w", err)
		}
		q.Payload = agentcli.Raw(payload)
		q.Created, q.Updated = fromMillis(created), fromMillis(updated)
		out = append(out, q)
	}
	return out, rows.Err()
}

// Memory is one stage-1 memory extracted from a thread's rollout.
type Memory struct {
	ThreadID       string
	SourceUpdated  time.Time
	RawMemory      string
	RolloutSummary string
	RolloutSlug    *string
	Generated      time.Time
	UsageCount     *int64
	LastUsage      *time.Time
	SelectedPhase2 bool
}

// Memories lists stage-1 memories, newest source first, at most limit
// (0 = 200, capped at 10000). Disk · Read (memories_1.sqlite).
func (d *DB) Memories(ctx context.Context, limit int) ([]Memory, error) {
	c, err := d.ready(ctx, FileMemories)
	if err != nil {
		return nil, err
	}
	rows, err := c.QueryContext(ctx, "SELECT thread_id, source_updated_at, raw_memory, rollout_summary, rollout_slug, generated_at, usage_count, last_usage, selected_for_phase2 FROM "+
		tableStage1+" ORDER BY source_updated_at DESC, thread_id DESC LIMIT ?", clampLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("codex/state: memories: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Memory
	for rows.Next() {
		var m Memory
		var src, gen, usage, last sql.NullInt64
		var slug sql.NullString
		var sel int64
		if err := rows.Scan(&m.ThreadID, &src, &m.RawMemory, &m.RolloutSummary, &slug, &gen, &usage, &last, &sel); err != nil {
			return nil, fmt.Errorf("codex/state: memories: %w", err)
		}
		m.SourceUpdated, m.Generated = fromSeconds(src), fromSeconds(gen)
		m.RolloutSlug, m.UsageCount = strPtr(slug), int64Ptr(usage)
		m.LastUsage = timePtr(fromSeconds(last))
		m.SelectedPhase2 = sel != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

// JobStatus is jobs.status; "running" is the only value upstream names in
// a constraint-free column, so the type stays open.
type JobStatus string

// JobStatus values seen in upstream runtime/memories.rs.
const (
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
)

// IsKnown reports whether s is a known job status. Pure.
func (s JobStatus) IsKnown() bool { return s == JobRunning || s == JobDone }

// MemoryJob is one background memory job (extraction or consolidation).
// The worker ownership token is never read.
type MemoryJob struct {
	Kind           string
	Key            string
	Status         JobStatus
	Started        *time.Time
	Finished       *time.Time
	LeaseUntil     *time.Time
	RetryAt        *time.Time
	RetryRemaining int64
	LastError      *string
}

// MemoryJobs lists memory jobs by kind and key. Disk · Read
// (memories_1.sqlite).
func (d *DB) MemoryJobs(ctx context.Context) ([]MemoryJob, error) {
	c, err := d.ready(ctx, FileMemories)
	if err != nil {
		return nil, err
	}
	rows, err := c.QueryContext(ctx, "SELECT kind, job_key, status, started_at, finished_at, lease_until, retry_at, retry_remaining, last_error FROM "+
		tableJobs+" ORDER BY kind, job_key LIMIT ?", maxLimit)
	if err != nil {
		return nil, fmt.Errorf("codex/state: memory jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []MemoryJob
	for rows.Next() {
		var j MemoryJob
		var status string
		var started, finished, lease, retry sql.NullInt64
		var lastErr sql.NullString
		if err := rows.Scan(&j.Kind, &j.Key, &status, &started, &finished, &lease, &retry, &j.RetryRemaining, &lastErr); err != nil {
			return nil, fmt.Errorf("codex/state: memory jobs: %w", err)
		}
		j.Status = JobStatus(status)
		j.Started, j.Finished = timePtr(fromSeconds(started)), timePtr(fromSeconds(finished))
		j.LeaseUntil, j.RetryAt = timePtr(fromSeconds(lease)), timePtr(fromSeconds(retry))
		j.LastError = strPtr(lastErr)
		out = append(out, j)
	}
	return out, rows.Err()
}

// LogLevel is logs.level (tracing level names, upper case upstream).
type LogLevel string

// LogLevel values.
const (
	LevelTrace LogLevel = "TRACE"
	LevelDebug LogLevel = "DEBUG"
	LevelInfo  LogLevel = "INFO"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
)

// IsKnown reports whether l is a known level. Pure.
func (l LogLevel) IsKnown() bool {
	switch l {
	case LevelTrace, LevelDebug, LevelInfo, LevelWarn, LevelError:
		return true
	}
	return false
}

// LogQuery narrows Logs. Zero value = newest 200 lines.
type LogQuery struct {
	ThreadID string
	Level    LogLevel  // exact level; "" = all
	Since    time.Time // zero = no lower bound
	Limit    int       // 0 = 200, capped at 10000
}

// LogLine is one row of logs_2.
type LogLine struct {
	ID          int64
	Time        time.Time
	Level       LogLevel
	Target      string
	Body        string // feedback_log_body (message before logs 0002)
	ModulePath  *string
	File        *string
	Line        *int64
	ThreadID    *string
	ProcessUUID *string
}

// Logs returns log lines newest first, bounded by q.Limit. Disk · Read
// (logs_2.sqlite; the file can be tens of MB, the query uses its ts index).
func (d *DB) Logs(ctx context.Context, q LogQuery) ([]LogLine, error) {
	c, err := d.ready(ctx, FileLogs)
	if err != nil {
		return nil, err
	}
	have, err := d.columns(ctx, FileLogs, tableLogs)
	if err != nil {
		return nil, err
	}
	body := colLogMessage
	if !have[colLogMessage] {
		body = colLogOldMsg
	}
	var since int64
	if !q.Since.IsZero() {
		since = q.Since.Unix()
	}
	sqlText := "SELECT id, ts, ts_nanos, level, target, " + body + ", module_path, file, line, thread_id, process_uuid FROM " + tableLogs +
		" WHERE (? = '' OR thread_id = ?) AND (? = '' OR level = ?) AND ts >= ?" +
		" ORDER BY ts DESC, ts_nanos DESC, id DESC LIMIT ?"
	rows, err := c.QueryContext(ctx, sqlText, q.ThreadID, q.ThreadID, string(q.Level), string(q.Level), since, clampLimit(q.Limit))
	if err != nil {
		return nil, fmt.Errorf("codex/state: logs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []LogLine
	for rows.Next() {
		var l LogLine
		var ts, nanos int64
		var level string
		var msg, module, file, thread, proc sql.NullString
		var line sql.NullInt64
		if err := rows.Scan(&l.ID, &ts, &nanos, &level, &l.Target, &msg, &module, &file, &line, &thread, &proc); err != nil {
			return nil, fmt.Errorf("codex/state: logs: %w", err)
		}
		l.Time = time.Unix(ts, nanos).UTC()
		l.Level = LogLevel(level)
		l.Body = msg.String
		l.ModulePath, l.File, l.Line = strPtr(module), strPtr(file), int64Ptr(line)
		l.ThreadID, l.ProcessUUID = strPtr(thread), strPtr(proc)
		out = append(out, l)
	}
	return out, rows.Err()
}
