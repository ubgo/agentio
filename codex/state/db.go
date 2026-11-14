// Package state reads Codex's SQLite stores (CODEX_API.md §9) read-only:
// threads, turns and items, goals, the message queue, memories and logs.
//
// It is its own module so that callers who only need rollouts, config and
// usage (package codex) never download the SQLite driver. The driver is
// modernc.org/sqlite: pure Go, so every target builds with CGO_ENABLED=0.
//
// Codex writes these files live in WAL mode. Every connection is opened
// read-only with query_only set, and never with immutable=1: immutable
// would skip the WAL and miss the newest rows. Nothing here writes;
// archive, delete and queue changes go through Codex's own commands
// (codex.Agent.Archive and friends) because Codex keeps rollouts, threads
// and thread history consistent itself.
package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/khanakia/agentcli"
	"github.com/khanakia/agentcli/codex"

	// Registers the pure-Go "sqlite" driver.
	_ "modernc.org/sqlite"
)

// driverName is the database/sql name modernc.org/sqlite registers.
const driverName = "sqlite"

// busyTimeout bounds how long a read waits while Codex (or Codex Desktop)
// holds a write lock. One short wait, never a retry loop (CODEX_API §9
// Unknowns: SQLITE_BUSY while a write transaction is open).
const busyTimeout = 2 * time.Second

// File names one of the six SQLite stores. Library-owned closed set.
type File uint8

// File values, in Paths order.
const (
	FileState File = iota
	FileThreadHistory
	FileGoals
	FileQueue
	FileMemories
	FileLogs
	fileCount
)

// fileNames is the String form of each File: the base name upstream uses
// (codex-rs/state/src/sqlite.rs *_DB_FILENAME).
func (f File) String() string {
	switch f {
	case FileState:
		return "state_5.sqlite"
	case FileThreadHistory:
		return "thread_history_1.sqlite"
	case FileGoals:
		return "goals_1.sqlite"
	case FileQueue:
		return "queue_1.sqlite"
	case FileMemories:
		return "memories_1.sqlite"
	case FileLogs:
		return "logs_2.sqlite"
	}
	return fmt.Sprintf("File(%d)", uint8(f))
}

// MarshalText renders the file name. Pure.
func (f File) MarshalText() ([]byte, error) { return []byte(f.String()), nil }

// MaxKnownMigration is the newest sqlx migration of f this library was
// written against (upstream rust-v0.162.1: state/migrations 0059,
// thread_history_migrations 0008, the others 0002). A newer database may
// have renamed or dropped a column, so readers refuse it with
// *SchemaTooNewError rather than guess. Pure.
func (f File) MaxKnownMigration() int64 {
	switch f {
	case FileState:
		return maxMigrationState
	case FileThreadHistory:
		return maxMigrationThreadHistory
	case FileGoals, FileQueue, FileMemories, FileLogs:
		return maxMigrationSmall
	}
	return 0
}

// Newest upstream migration per store at rust-v0.162.1.
const (
	maxMigrationState         = 59
	maxMigrationThreadHistory = 8
	maxMigrationSmall         = 2
)

// ErrSchemaTooNew matches every *SchemaTooNewError.
var ErrSchemaTooNew = errors.New("codex/state: database schema is newer than this library")

// ErrNotFound means a requested thread does not exist. It is the root
// sentinel, so callers branch on one value across modules.
var ErrNotFound = agentcli.ErrNotFound

// SchemaTooNewError reports a store whose _sqlx_migrations version is past
// MaxKnownMigration. It matches errors.Is(err, ErrSchemaTooNew).
type SchemaTooNewError struct {
	File  File
	Found int64
	Max   int64
}

// Error describes the mismatch without any row data. Pure.
func (e *SchemaTooNewError) Error() string {
	return fmt.Sprintf("codex/state: %s is at migration %d, this library knows up to %d", e.File, e.Found, e.Max)
}

// Is matches ErrSchemaTooNew. Pure.
func (e *SchemaTooNewError) Is(target error) bool { return target == ErrSchemaTooNew }

// DB is a read-only view over one home's SQLite stores. Each file is opened
// on first use, so a caller that only lists threads never touches logs_2
// (42 MB on the sample home). Safe for concurrent use.
type DB struct {
	paths [fileCount]string

	mu    sync.Mutex
	conns [fileCount]*sql.DB
	// cols caches PRAGMA table_info per (file, table): column sets do not
	// change while a connection is open in a way this reader depends on.
	cols map[string]map[string]bool
}

// Open prepares a DB over the SQLite files named in p (Paths or PathsWith,
// so CODEX_SQLITE_HOME / sqlite_home are honoured). Nothing is read until a
// query runs. Pure (Disk · Read on first query).
func Open(p codex.Paths) (*DB, error) {
	d := &DB{cols: map[string]map[string]bool{}}
	d.paths[FileState] = p.StateDB
	d.paths[FileThreadHistory] = p.ThreadHistoryDB
	d.paths[FileGoals] = p.GoalsDB
	d.paths[FileQueue] = p.QueueDB
	d.paths[FileMemories] = p.MemoriesDB
	d.paths[FileLogs] = p.LogsDB
	for f, path := range d.paths {
		if path == "" {
			return nil, fmt.Errorf("codex/state: no path for %s", File(f))
		}
	}
	return d, nil
}

// Path returns the file path behind f. Pure.
func (d *DB) Path(f File) string {
	if f >= fileCount {
		return ""
	}
	return d.paths[f]
}

// Close closes every opened connection. Pure (releases handles).
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	var errs []error
	for i, c := range d.conns {
		if c != nil {
			errs = append(errs, c.Close())
			d.conns[i] = nil
		}
	}
	return errors.Join(errs...)
}

// dsn builds a read-only URI for path. mode=ro plus query_only(1) means a
// bug here cannot write; busy_timeout gives one bounded wait on a lock.
func dsn(path string) string {
	q := url.Values{}
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeout.Milliseconds()))
	u := url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}
	return u.String()
}

// conn opens f once. A missing file is reported as os.ErrNotExist rather
// than letting SQLite fail with a generic "unable to open".
func (d *DB) conn(f File) (*sql.DB, error) {
	if f >= fileCount {
		return nil, fmt.Errorf("codex/state: unknown file %d", uint8(f))
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if c := d.conns[f]; c != nil {
		return c, nil
	}
	if _, err := os.Stat(d.paths[f]); err != nil {
		return nil, fmt.Errorf("codex/state: %s: %w", f, err)
	}
	c, err := sql.Open(driverName, dsn(d.paths[f]))
	if err != nil {
		return nil, fmt.Errorf("codex/state: open %s: %w", f, err)
	}
	d.conns[f] = c
	return c, nil
}

// migrationsTable is the sqlx bookkeeping table in every store.
const migrationsTable = "_sqlx_migrations"

// SchemaVersion returns the newest applied migration of f (0 when the
// table is empty). Disk · Read.
func (d *DB) SchemaVersion(ctx context.Context, f File) (int64, error) {
	c, err := d.conn(f)
	if err != nil {
		return 0, err
	}
	var v sql.NullInt64
	q := "SELECT MAX(version) FROM " + migrationsTable + " WHERE success = 1"
	if err := c.QueryRowContext(ctx, q).Scan(&v); err != nil {
		return 0, fmt.Errorf("codex/state: %s schema version: %w", f, err)
	}
	return v.Int64, nil
}

// ready opens f and checks its schema is not newer than this library.
func (d *DB) ready(ctx context.Context, f File) (*sql.DB, error) {
	v, err := d.SchemaVersion(ctx, f)
	if err != nil {
		return nil, err
	}
	if v > f.MaxKnownMigration() {
		return nil, &SchemaTooNewError{File: f, Found: v, Max: f.MaxKnownMigration()}
	}
	return d.conn(f)
}

// columns returns the column set of table in f, cached per DB.
func (d *DB) columns(ctx context.Context, f File, table string) (map[string]bool, error) {
	key := f.String() + "/" + table
	d.mu.Lock()
	cached, ok := d.cols[key]
	d.mu.Unlock()
	if ok {
		return cached, nil
	}
	c, err := d.conn(f)
	if err != nil {
		return nil, err
	}
	rows, err := c.QueryContext(ctx, "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return nil, fmt.Errorf("codex/state: %s columns of %s: %w", f, table, err)
	}
	defer func() { _ = rows.Close() }()
	set := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("codex/state: %s columns of %s: %w", f, table, err)
		}
		set[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("codex/state: %s columns of %s: %w", f, table, err)
	}
	d.mu.Lock()
	d.cols[key] = set
	d.mu.Unlock()
	return set, nil
}

// selectList renders want as a SELECT list, substituting NULL for a column
// an older schema lacks, so one Scan layout works on every schema version.
// Column names come from this package's constants, never from input.
func selectList(have map[string]bool, prefix string, want []string) string {
	parts := make([]string, len(want))
	for i, c := range want {
		if have[c] {
			parts[i] = prefix + c
		} else {
			parts[i] = "NULL AS " + c
		}
	}
	return strings.Join(parts, ", ")
}

// Bounds for paged readers: Codex DBs can be large (logs_2 was 42 MB on
// the sample home), so every list is capped even when the caller asks for
// "all".
const (
	defaultLimit = 200
	maxLimit     = 10000
)

// clampLimit maps a caller limit onto [1, maxLimit], 0 meaning default.
func clampLimit(n int) int {
	switch {
	case n <= 0:
		return defaultLimit
	case n > maxLimit:
		return maxLimit
	}
	return n
}

// Epoch conversions. Codex stores seconds in older columns and
// milliseconds in *_ms columns; both become UTC time.Time at this edge.
func fromSeconds(v sql.NullInt64) time.Time {
	if !v.Valid || v.Int64 == 0 {
		return time.Time{}
	}
	return time.Unix(v.Int64, 0).UTC()
}

func fromMillis(v sql.NullInt64) time.Time {
	if !v.Valid || v.Int64 == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v.Int64).UTC()
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func strPtr(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func int64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}
