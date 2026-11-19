package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/ubgo/agentio"
	"github.com/ubgo/agentio/opencode"
)

// Table and column names (schema recorded from opencode 1.18.30).
const (
	tableSession = "session"
	tableMessage = "message"
	tablePart    = "part"
)

// SessionModel is the model a session last used (session.model column,
// 1.18.x): {"id":"kimi-k3","providerID":"moonshotai","variant":"high"}.
type SessionModel struct {
	ID         string `json:"id"`
	ProviderID string `json:"providerID"`
	Variant    string `json:"variant,omitempty"`
}

// Session is one session row. Fields of columns an older database lacks
// stay zero.
type Session struct {
	ID        string
	ProjectID string
	// ParentID is set on a child session (a subagent task).
	ParentID  string
	Slug      string
	Directory string
	// Path is the session directory relative to the project worktree, as
	// recorded (newer releases only).
	Path    string
	Title   string
	Version string
	// ShareURL is the public link of a shared session.
	ShareURL    string
	WorkspaceID string
	// Agent is the agent the session last ran ("build").
	Agent string
	// Model is the model the session last used; nil when not recorded.
	Model *SessionModel
	// Cost and Tokens are opencode's own running totals of the session's
	// assistant messages (newer releases only).
	Cost   float64
	Tokens opencode.Tokens
	// SummaryAdditions, SummaryDeletions and SummaryFiles are the session's
	// diff totals.
	SummaryAdditions, SummaryDeletions, SummaryFiles int64
	Created, Updated                                 time.Time
	// Archived is when the session was archived; zero when it is not.
	Archived time.Time
}

// Conversation projects the session onto agentio.Conversation for home h.
// Path is empty: the transcript is rows in the database, not a file a
// TurnFileReader can read. cost: Pure.
func (s Session) Conversation(h opencode.Home) agentio.Conversation {
	return agentio.Conversation{
		Agent: opencode.ID, Home: h.Neutral(), ID: s.ID, Title: s.Title, Dir: s.Directory,
		Created: s.Created, Updated: s.Updated,
	}
}

// sessionColumns are the columns read, in Scan order. The first seven are
// required (the table as 1.2.0 created it has them); the rest are read
// when present.
func sessionColumns() []string {
	return []string{
		"id", "project_id", "directory", "title", "version", "time_created", "time_updated",
		"parent_id", "slug", "path", "share_url", "workspace_id", "agent", "model", "cost",
		"tokens_input", "tokens_output", "tokens_reasoning", "tokens_cache_read", "tokens_cache_write",
		"summary_additions", "summary_deletions", "summary_files", "time_archived",
	}
}

// requiredSessionColumns is how many leading sessionColumns are required.
const requiredSessionColumns = 7

// SessionQuery filters Sessions.
type SessionQuery struct {
	// Dir limits to sessions whose directory is exactly this one.
	Dir string
	// Since limits to sessions updated at or after it; zero = no limit.
	Since time.Time
	// Limit caps results (newest first); <= 0 = no cap.
	Limit int
	// Roots drops child sessions (subagent tasks, parent_id set).
	Roots bool
	// Active drops archived sessions.
	Active bool
}

// Sessions lists sessions newest time_updated first (ties by id).
// cost: Disk · Read (one query).
func (d *DB) Sessions(ctx context.Context, q SessionQuery) ([]Session, error) {
	have, err := d.columns(ctx, tableSession)
	if err != nil {
		return nil, err
	}
	cols := sessionColumns()
	if err := requireColumns(tableSession, have, cols[:requiredSessionColumns]); err != nil {
		return nil, err
	}
	// Every filter is always bound with a "no filter" sentinel so the query
	// has a fixed parameter list (database/sql's variadic takes the empty
	// interface, which this module never names).
	dir := ""
	if q.Dir != "" {
		dir = filepath.Clean(q.Dir)
	}
	since := int64(0)
	if !q.Since.IsZero() {
		since = q.Since.UnixMilli()
	}
	limit := noLimit
	if q.Limit > 0 {
		limit = int64(q.Limit)
	}
	query := "SELECT " + selectList(have, cols) + " FROM " + tableSession +
		" WHERE (? = '' OR directory = ?) AND (? = 0 OR time_updated >= ?)"
	if q.Roots && have["parent_id"] {
		query += " AND parent_id IS NULL"
	}
	if q.Active && have["time_archived"] {
		query += " AND time_archived IS NULL"
	}
	query += " ORDER BY time_updated DESC, id LIMIT ?"
	c, err := d.db()
	if err != nil {
		return nil, err
	}
	rows, err := c.QueryContext(ctx, query, dir, dir, since, since, limit)
	if err != nil {
		return nil, fmt.Errorf("opencode/store: sessions: %w", err)
	}
	return scanSessions(rows)
}

// noLimit is SQLite's LIMIT value for "every row".
const noLimit int64 = -1

// Session returns one session; agentio.ErrNotFound when absent.
// cost: Disk · Read.
func (d *DB) Session(ctx context.Context, id string) (Session, error) {
	have, err := d.columns(ctx, tableSession)
	if err != nil {
		return Session{}, err
	}
	cols := sessionColumns()
	if err := requireColumns(tableSession, have, cols[:requiredSessionColumns]); err != nil {
		return Session{}, err
	}
	c, err := d.db()
	if err != nil {
		return Session{}, err
	}
	rows, err := c.QueryContext(ctx, "SELECT "+selectList(have, cols)+" FROM "+tableSession+" WHERE id = ?", id)
	if err != nil {
		return Session{}, fmt.Errorf("opencode/store: session: %w", err)
	}
	ss, err := scanSessions(rows)
	if err != nil {
		return Session{}, err
	}
	if len(ss) == 0 {
		return Session{}, fmt.Errorf("%w: opencode session %s", agentio.ErrNotFound, id)
	}
	return ss[0], nil
}

// scanSessions reads and closes rows of a session SELECT built from
// sessionColumns. Disk · Read.
func scanSessions(rows *sql.Rows) ([]Session, error) {
	// Read-only rows: a close error loses nothing.
	defer func() { _ = rows.Close() }()
	var out []Session
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("opencode/store: sessions: %w", err)
	}
	return out, nil
}

// scanSession reads one row in sessionColumns order. Pure (on the row).
func scanSession(rows *sql.Rows) (Session, error) {
	var (
		s                                           Session
		created, updated                            int64
		parent, slug, path, share, workspace, agent sql.NullString
		model                                       sql.NullString
		cost                                        sql.NullFloat64
		in, out, reasoning, cacheRead, cacheWrite   sql.NullInt64
		additions, deletions, files, archived       sql.NullInt64
	)
	err := rows.Scan(&s.ID, &s.ProjectID, &s.Directory, &s.Title, &s.Version, &created, &updated,
		&parent, &slug, &path, &share, &workspace, &agent, &model, &cost,
		&in, &out, &reasoning, &cacheRead, &cacheWrite, &additions, &deletions, &files, &archived)
	if err != nil {
		return Session{}, fmt.Errorf("opencode/store: session row: %w", err)
	}
	s.Created, s.Updated = opencode.Millis(created).Time(), opencode.Millis(updated).Time()
	s.ParentID, s.Slug, s.Path, s.ShareURL = parent.String, slug.String, path.String, share.String
	s.WorkspaceID, s.Agent, s.Cost = workspace.String, agent.String, cost.Float64
	s.Tokens = opencode.Tokens{Input: in.Int64, Output: out.Int64, Reasoning: reasoning.Int64,
		Cache: opencode.CacheTokens{Read: cacheRead.Int64, Write: cacheWrite.Int64}}
	s.SummaryAdditions, s.SummaryDeletions, s.SummaryFiles = additions.Int64, deletions.Int64, files.Int64
	s.Archived = opencode.Millis(archived.Int64).Time()
	if model.Valid && model.String != "" {
		var m SessionModel
		if err := json.Unmarshal([]byte(model.String), &m); err != nil {
			return Session{}, fmt.Errorf("opencode/store: session %s model: %w", s.ID, err)
		}
		s.Model = &m
	}
	return s, nil
}

// Conversations lists the resumable sessions (no child sessions, no
// archived ones) as neutral conversations, newest first, filtered by q.
// cost: Disk · Read.
func (d *DB) Conversations(ctx context.Context, q agentio.ConversationQuery) ([]agentio.Conversation, error) {
	ss, err := d.Sessions(ctx, SessionQuery{Dir: q.Dir, Since: q.Since, Limit: q.Limit, Roots: true, Active: true})
	if err != nil {
		return nil, err
	}
	out := make([]agentio.Conversation, 0, len(ss))
	for _, s := range ss {
		out = append(out, s.Conversation(d.home))
	}
	return out, nil
}
