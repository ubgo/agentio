package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/codex"
)

// Table and column names of state_5.sqlite (upstream state/migrations,
// rust-v0.162.1). Kept as constants so a schema rename is one edit.
const (
	tableThreads        = "threads"
	tableSpawnEdges     = "thread_spawn_edges"
	tableProjects       = "projects"
	tableProjectRoots   = "project_roots"
	pinnedSectionID     = "01984de2-8f74-7c91-a3b2-5c5e937cf318" // lib.rs PINNED_THREAD_SECTION_ID
	colThreadID         = "id"
	colArchived         = "archived"
	colCwd              = "cwd"
	colProjectID        = "project_id"
	colIsPinned         = "is_pinned"
	colThreadSectionID  = "thread_section_id"
	colUpdatedAtMs      = "updated_at_ms"
	colUpdatedAtSeconds = "updated_at"
)

// threadColumns is the SELECT layout scanned by scanThread, in order.
// Columns added after migration 0001 may be absent on older files and are
// selected as NULL there (presence-tracked). A function, not a package
// var, per the no-package-state rule.
func threadColumns() []string {
	return []string{
		"id", "rollout_path", "created_at", "created_at_ms", "updated_at", "updated_at_ms", "recency_at_ms",
		"source", "model_provider", "model", "reasoning_effort", "cwd", "title", "name", "preview",
		"first_user_message", "sandbox_policy", "approval_mode", "tokens_used", "has_user_event",
		"archived", "archived_at", "git_sha", "git_branch", "git_origin_url", "cli_version",
		"agent_nickname", "agent_role", "agent_path", "memory_mode", "history_mode", "thread_source",
		"is_pinned", "thread_section_id", "project_id", "originator", "creator_user_id", "creator_account_id",
	}
}

// HistoryMode is threads.history_mode: whether turns and items live in
// thread_history_1 (paginated) or only in the rollout (legacy).
type HistoryMode string

// HistoryMode values.
const (
	HistoryLegacy    HistoryMode = "legacy"
	HistoryPaginated HistoryMode = "paginated"
)

// IsKnown reports whether m is a known history mode. Pure.
func (m HistoryMode) IsKnown() bool { return m == HistoryLegacy || m == HistoryPaginated }

// SubAgentInfo is set on a thread that a parent thread spawned.
type SubAgentInfo struct {
	Nickname, Role, Path string
}

// Thread is one row of state_5.sqlite threads: Codex's authoritative
// conversation index (richer than rollout files: title, preview, tokens,
// archive flag, git info).
type Thread struct {
	ID               string
	RolloutPath      string // absolute; the reason Codex conversations cannot move (invariant 6)
	Cwd              string
	Title            string
	Name             *string // user-set name (0041); nil when unset
	Preview          string
	FirstUserMessage string
	Created          time.Time
	Updated          time.Time
	Recency          time.Time
	ArchivedAt       *time.Time
	Archived         bool
	Pinned           bool // is_pinned or the built-in Pinned section
	HasUserEvent     bool
	Source           codex.SessionSource
	ThreadSource     *string // user | subagent | guardian_review …
	Originator       *string // "codex-tui", "Codex Desktop", …
	ModelProvider    string
	Model            *string
	ReasoningEffort  codex.ReasoningEffort
	Sandbox          codex.SandboxPolicy
	Approval         codex.ApprovalPolicy
	TokensUsed       int64
	CLIVersion       agentio.Version // zero when the column is empty or absent
	Git              *codex.GitInfo
	Agent            *SubAgentInfo
	MemoryMode       *string
	HistoryMode      HistoryMode
	ProjectID        *string
	SectionID        *string
	CreatorUserID    agentio.Personal
	CreatorAccountID agentio.Personal
	// Warnings lists columns whose value did not decode (e.g. a
	// sandbox_policy shape this library has not seen); the rest of the row
	// still reads (invariant 17).
	Warnings agentio.Warnings
}

// ThreadStatusFilter narrows Threads. Zero value = every thread, newest
// update first, capped at the default limit.
type ThreadStatusFilter struct {
	Archived  *bool  // nil = both
	Dir       string // exact cwd match
	ProjectID string
	Pinned    *bool
	Limit     int // 0 = default (200); capped at 10000
}

// Threads lists threads, most recently updated first. Disk · Read
// (state_5.sqlite).
func (d *DB) Threads(ctx context.Context, f ThreadStatusFilter) ([]Thread, error) {
	c, have, err := d.threadsTable(ctx)
	if err != nil {
		return nil, err
	}
	// Every filter is always bound with a "no filter" sentinel so the
	// query has a fixed parameter list (database/sql's variadic takes the
	// empty interface, which this module never names).
	archived := int64(noFilter)
	if f.Archived != nil {
		archived = boolInt(*f.Archived)
	}
	pinned := int64(noFilter)
	if f.Pinned != nil {
		pinned = boolInt(*f.Pinned)
	}
	project := "NULL"
	if have[colProjectID] {
		project = colProjectID
	}
	order := colUpdatedAtSeconds
	if have[colUpdatedAtMs] {
		order = "COALESCE(" + colUpdatedAtMs + ", " + colUpdatedAtSeconds + " * 1000)"
	}
	q := "SELECT " + selectList(have, "", threadColumns()) + " FROM " + tableThreads +
		" WHERE (? < 0 OR " + colArchived + " = ?)" +
		" AND (? = '' OR " + colCwd + " = ?)" +
		" AND (? = '' OR " + project + " = ?)" +
		" AND (? < 0 OR " + pinnedExpr(have) + " = ?)" +
		" ORDER BY " + order + " DESC, " + colThreadID + " DESC LIMIT ?"
	rows, err := c.QueryContext(ctx, q, archived, archived, f.Dir, f.Dir, f.ProjectID, f.ProjectID, pinned, pinned, clampLimit(f.Limit))
	if err != nil {
		return nil, fmt.Errorf("codex/state: threads: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Thread
	for rows.Next() {
		t, err := scanThread(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("codex/state: threads: %w", err)
	}
	return out, nil
}

// Thread returns one thread by id; ErrNotFound when absent. Disk · Read
// (state_5.sqlite).
func (d *DB) Thread(ctx context.Context, id string) (Thread, error) {
	c, have, err := d.threadsTable(ctx)
	if err != nil {
		return Thread{}, err
	}
	q := "SELECT " + selectList(have, "", threadColumns()) + " FROM " + tableThreads + " WHERE " + colThreadID + " = ?"
	rows, err := c.QueryContext(ctx, q, id)
	if err != nil {
		return Thread{}, fmt.Errorf("codex/state: thread: %w", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Thread{}, fmt.Errorf("codex/state: thread: %w", err)
		}
		return Thread{}, fmt.Errorf("thread %q: %w", id, ErrNotFound)
	}
	return scanThread(rows)
}

func (d *DB) threadsTable(ctx context.Context) (*sql.DB, map[string]bool, error) {
	c, err := d.ready(ctx, FileState)
	if err != nil {
		return nil, nil, err
	}
	have, err := d.columns(ctx, FileState, tableThreads)
	if err != nil {
		return nil, nil, err
	}
	return c, have, nil
}

// pinnedExpr is the SQL truth of "pinned" for the columns present.
func pinnedExpr(have map[string]bool) string {
	var parts []string
	if have[colIsPinned] {
		parts = append(parts, colIsPinned+" = 1")
	}
	if have[colThreadSectionID] {
		parts = append(parts, "COALESCE("+colThreadSectionID+", '') = '"+pinnedSectionID+"'")
	}
	if len(parts) == 0 {
		return "0"
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

// noFilter is the bound sentinel meaning "do not filter on this flag".
const noFilter = -1

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// scanThread reads one row in threadColumns order.
func scanThread(rows *sql.Rows) (Thread, error) {
	var (
		id, rollout, source, provider, cwd, title, sandbox, approval      sql.NullString
		model, effort, name, preview, firstMsg, gitSHA, gitBranch, gitURL sql.NullString
		cliVersion, nick, role, path, memMode, histMode, threadSource     sql.NullString
		section, project, originator, creatorUser, creatorAccount         sql.NullString
		created, createdMs, updated, updatedMs, recencyMs, tokens         sql.NullInt64
		hasUser, archived, archivedAt, pinned                             sql.NullInt64
	)
	err := rows.Scan(&id, &rollout, &created, &createdMs, &updated, &updatedMs, &recencyMs,
		&source, &provider, &model, &effort, &cwd, &title, &name, &preview,
		&firstMsg, &sandbox, &approval, &tokens, &hasUser,
		&archived, &archivedAt, &gitSHA, &gitBranch, &gitURL, &cliVersion,
		&nick, &role, &path, &memMode, &histMode, &threadSource,
		&pinned, &section, &project, &originator, &creatorUser, &creatorAccount)
	if err != nil {
		return Thread{}, fmt.Errorf("codex/state: scan thread: %w", err)
	}
	t := Thread{
		ID: id.String, RolloutPath: rollout.String, Cwd: cwd.String, Title: title.String,
		Name: strPtr(name), Preview: preview.String, FirstUserMessage: firstMsg.String,
		Created:      pickTime(createdMs, created),
		Updated:      pickTime(updatedMs, updated),
		Recency:      fromMillis(recencyMs),
		ArchivedAt:   timePtr(fromSeconds(archivedAt)),
		Archived:     archived.Int64 != 0,
		HasUserEvent: hasUser.Int64 != 0,
		Pinned:       pinned.Int64 != 0 || section.String == pinnedSectionID,
		Source:       codex.ParseSessionSource(source.String),
		ThreadSource: strPtr(threadSource), Originator: strPtr(originator),
		ModelProvider: provider.String, Model: strPtr(model),
		ReasoningEffort: codex.ReasoningEffort(effort.String),
		Approval:        codex.ApprovalPolicy(approval.String),
		TokensUsed:      tokens.Int64,
		MemoryMode:      strPtr(memMode),
		HistoryMode:     HistoryMode(histMode.String),
		ProjectID:       strPtr(project), SectionID: strPtr(section),
		CreatorUserID:    agentio.NewPersonal(creatorUser.String),
		CreatorAccountID: agentio.NewPersonal(creatorAccount.String),
	}
	if !histMode.Valid {
		// Before migration 0040 every thread was legacy (column default).
		t.HistoryMode = HistoryLegacy
	}
	if sandbox.String != "" {
		if err := json.Unmarshal([]byte(sandbox.String), &t.Sandbox); err != nil {
			t.Warnings = append(t.Warnings, agentio.Warning{Kind: agentio.WarnTypeChanged, File: FileState.String(), Key: "threads.sandbox_policy", Detail: "unrecognised sandbox_policy shape"})
		}
	}
	if cliVersion.String != "" {
		v, err := codex.ParseVersion(cliVersion.String)
		if err == nil {
			t.CLIVersion = v
		} else {
			t.Warnings = append(t.Warnings, agentio.Warning{Kind: agentio.WarnTypeChanged, File: FileState.String(), Key: "threads.cli_version", Detail: "unparseable version"})
		}
	}
	if gitSHA.Valid || gitBranch.Valid || gitURL.Valid {
		t.Git = &codex.GitInfo{CommitHash: gitSHA.String, Branch: gitBranch.String, RepositoryURL: gitURL.String}
	}
	if nick.Valid || role.Valid || path.Valid {
		t.Agent = &SubAgentInfo{Nickname: nick.String, Role: role.String, Path: path.String}
	}
	return t, nil
}

// pickTime prefers the millisecond column (0025+) and falls back to the
// older seconds column. Migration 0025 also notes some old rows already
// stored milliseconds in the seconds column; values past 2020-01-01 in ms
// are treated as milliseconds, matching that migration.
func pickTime(ms, s sql.NullInt64) time.Time {
	if ms.Valid && ms.Int64 != 0 {
		return fromMillis(ms)
	}
	if s.Valid && s.Int64 >= msThreshold {
		return fromMillis(s)
	}
	return fromSeconds(s)
}

// msThreshold is 2020-01-01T00:00:00Z in milliseconds: upstream migration
// 0025 uses it to tell a millisecond value from a seconds value.
const msThreshold = 1577836800000

// SpawnEdge links a parent thread to a sub-agent thread it spawned.
type SpawnEdge struct {
	ParentID, ChildID string
	Status            EdgeStatus
}

// EdgeStatus is thread_spawn_edges.status.
type EdgeStatus string

// EdgeStatus values (graph.rs DirectionalThreadSpawnEdgeStatus).
const (
	EdgeOpen   EdgeStatus = "open"
	EdgeClosed EdgeStatus = "closed"
)

// IsKnown reports whether s is a known edge status. Pure.
func (s EdgeStatus) IsKnown() bool { return s == EdgeOpen || s == EdgeClosed }

// SubAgents lists the threads parentID spawned. Disk · Read (state_5.sqlite).
func (d *DB) SubAgents(ctx context.Context, parentID string) ([]SpawnEdge, error) {
	c, err := d.ready(ctx, FileState)
	if err != nil {
		return nil, err
	}
	have, err := d.columns(ctx, FileState, tableSpawnEdges)
	if err != nil {
		return nil, err
	}
	if len(have) == 0 {
		return nil, nil // table predates migration 0021
	}
	rows, err := c.QueryContext(ctx, "SELECT parent_thread_id, child_thread_id, status FROM "+tableSpawnEdges+
		" WHERE parent_thread_id = ? ORDER BY child_thread_id", parentID)
	if err != nil {
		return nil, fmt.Errorf("codex/state: spawn edges: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []SpawnEdge
	for rows.Next() {
		var e SpawnEdge
		var status string
		if err := rows.Scan(&e.ParentID, &e.ChildID, &status); err != nil {
			return nil, fmt.Errorf("codex/state: spawn edges: %w", err)
		}
		e.Status = EdgeStatus(status)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Project is a Codex Desktop project: a named group of folders.
type Project struct {
	ID       string
	Name     string
	Position int64
	Roots    []string // in position order
	Metadata agentio.Raw
	Created  time.Time
	Updated  time.Time
}

// Projects lists Desktop projects in their display order. Disk · Read
// (state_5.sqlite).
func (d *DB) Projects(ctx context.Context) ([]Project, error) {
	c, err := d.ready(ctx, FileState)
	if err != nil {
		return nil, err
	}
	have, err := d.columns(ctx, FileState, tableProjects)
	if err != nil {
		return nil, err
	}
	if len(have) == 0 {
		return nil, nil // table predates migration 0049
	}
	rows, err := c.QueryContext(ctx, "SELECT id, name, metadata, position, created_at_ms, updated_at_ms FROM "+
		tableProjects+" ORDER BY position, id")
	if err != nil {
		return nil, fmt.Errorf("codex/state: projects: %w", err)
	}
	var out []Project
	idx := map[string]int{}
	for rows.Next() {
		var p Project
		var meta sql.NullString
		var created, updated sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Name, &meta, &p.Position, &created, &updated); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("codex/state: projects: %w", err)
		}
		if meta.String != "" {
			p.Metadata = agentio.Raw(meta.String)
		}
		p.Created, p.Updated = fromMillis(created), fromMillis(updated)
		idx[p.ID] = len(out)
		out = append(out, p)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return nil, fmt.Errorf("codex/state: projects: %w", err)
	}
	roots, err := c.QueryContext(ctx, "SELECT project_id, path FROM "+tableProjectRoots+" ORDER BY project_id, position")
	if err != nil {
		return nil, fmt.Errorf("codex/state: project roots: %w", err)
	}
	defer func() { _ = roots.Close() }()
	for roots.Next() {
		var pid, path string
		if err := roots.Scan(&pid, &path); err != nil {
			return nil, fmt.Errorf("codex/state: project roots: %w", err)
		}
		if i, ok := idx[pid]; ok {
			out[i].Roots = append(out[i].Roots, path)
		}
	}
	return out, roots.Err()
}

// Conversation projects t onto codex.Conversation for home h, so callers
// that read the SQLite index get the same type the rollout lister returns.
// The title prefers the user-set name, then the stored title, then the
// first user message, mirroring Codex's own thread list. Pure.
func (t Thread) Conversation(h codex.Home) codex.Conversation {
	title := t.Title
	if t.Name != nil && *t.Name != "" {
		title = *t.Name
	}
	if title == "" {
		title = t.FirstUserMessage
	}
	c := codex.Conversation{
		ID: t.ID, Home: h, Path: t.RolloutPath, Dir: t.Cwd, Title: title,
		Created: t.Created, Updated: t.Updated, CLIVersion: t.CLIVersion, Source: t.Source,
		ModelProvider: t.ModelProvider, Git: t.Git, Archived: t.Archived,
	}
	if t.Originator != nil {
		c.Originator = *t.Originator
	}
	return c
}
