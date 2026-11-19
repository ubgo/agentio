// Package store reads opencode's SQLite database (opencode.db, every
// session since opencode 1.2.0) read-only: sessions with their token and
// cost totals, and the messages and parts of a session as neutral turns.
//
// It is its own module so that callers who only need the opencode agent
// (package opencode: install, homes, launch, legacy storage) never
// download the SQLite driver. The driver is modernc.org/sqlite: pure Go,
// so every target builds with CGO_ENABLED=0.
//
// opencode writes the database live in WAL mode. The connection is opened
// read-only with query_only set, and never with immutable=1: immutable
// would skip the WAL and miss the newest rows. Nothing here writes.
//
// The schema is read by column presence, not by a migration number:
// opencode's migrations are named, not numbered, so a column a newer
// release adds is simply not read, and a column an older database lacks
// reads as zero (columns added after 1.2.0, such as the session token
// totals, are optional; the original ones are required).
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ubgo/agentio/opencode"

	// Registers the pure-Go "sqlite" driver.
	_ "modernc.org/sqlite"
)

// driverName is the database/sql name modernc.org/sqlite registers.
const driverName = "sqlite"

// busyTimeout bounds how long a read waits while opencode holds a write
// lock: one short wait, never a retry loop.
const busyTimeout = 2 * time.Second

// ErrNoDatabase means the home keeps its database in memory
// (OPENCODE_DB=":memory:"), so there is no file to read.
var ErrNoDatabase = errors.New("opencode/store: the database is in memory (OPENCODE_DB=:memory:)")

// DB is a read-only view of one opencode.db. The file is opened on first
// use. Safe for concurrent use.
type DB struct {
	path string
	home opencode.Home

	mu   sync.Mutex
	conn *sql.DB
	// cols caches the column set per table: columns do not change while a
	// connection is open in a way this reader depends on.
	cols map[string]map[string]bool
}

// Open prepares a DB over the database file at path for home h (the home
// the conversations it returns belong to). Nothing is read until a query
// runs. Pure (Disk · Read on first query).
func Open(h opencode.Home, path string) *DB {
	return &DB{path: path, home: h, cols: map[string]map[string]bool{}}
}

// OpenHome opens the database a release build of opencode uses for h
// (opencode.Agent.DatabasePath: OPENCODE_DB from the Host env, else
// <data dir>/opencode.db). ErrNoDatabase when OPENCODE_DB keeps it in
// memory. Pure (Disk · Read on first query).
func OpenHome(a *opencode.Agent, h opencode.Home) (*DB, error) {
	path, ok := a.DatabasePath(h)
	if !ok {
		return nil, ErrNoDatabase
	}
	return Open(h, path), nil
}

// Path returns the database file. Pure.
func (d *DB) Path() string { return d.path }

// Home returns the home the database belongs to. Pure.
func (d *DB) Home() opencode.Home { return d.home }

// Close closes the connection if one was opened. Pure (releases handles).
func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil {
		return nil
	}
	err := d.conn.Close()
	d.conn = nil
	return err
}

// dsn builds a read-only URI for path. mode=ro plus query_only(1) means a
// bug here cannot write; busy_timeout gives one bounded wait on a lock.
// The path is turned into a URI path first (fileURIPath), so a Windows
// path becomes file:///C:/... instead of an invalid authority. Pure.
func dsn(path string) string {
	q := url.Values{}
	q.Set("mode", "ro")
	q.Add("_pragma", "query_only(1)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", busyTimeout.Milliseconds()))
	u := url.URL{Scheme: "file", Path: fileURIPath(filepath.ToSlash(path)), RawQuery: q.Encode()}
	return u.String()
}

// fileURIPath turns a slash-separated file path into the path part of a
// file: URI as SQLite reads it (sqlite.org/uri.html): a drive path
// "C:/Users/x" gets a leading slash so url.URL renders "file:///C:/Users/x"
// (empty authority). POSIX, UNC ("//host/share") and relative paths are
// returned unchanged. Pure.
func fileURIPath(slashed string) string {
	if isDrivePath(slashed) {
		return "/" + slashed
	}
	return slashed
}

// isDrivePath reports a Windows drive path such as "C:/x" or "c:". Pure.
func isDrivePath(p string) bool {
	if len(p) < len("C:") || p[1] != ':' {
		return false
	}
	c := p[0] | ('a' - 'A') // fold to lower case
	return c >= 'a' && c <= 'z'
}

// db opens the file once. A missing file is reported as os.ErrNotExist
// rather than letting SQLite fail with a generic "unable to open".
// Disk · Read.
func (d *DB) db() (*sql.DB, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn != nil {
		return d.conn, nil
	}
	if _, err := os.Stat(d.path); err != nil {
		return nil, fmt.Errorf("opencode/store: %w", err)
	}
	c, err := sql.Open(driverName, dsn(d.path))
	if err != nil {
		return nil, fmt.Errorf("opencode/store: open %s: %w", d.path, err)
	}
	d.conn = c
	return c, nil
}

// columns returns the column set of table, cached per DB. Disk · Read.
func (d *DB) columns(ctx context.Context, table string) (map[string]bool, error) {
	d.mu.Lock()
	cached, ok := d.cols[table]
	d.mu.Unlock()
	if ok {
		return cached, nil
	}
	c, err := d.db()
	if err != nil {
		return nil, err
	}
	rows, err := c.QueryContext(ctx, "SELECT name FROM pragma_table_info(?)", table)
	if err != nil {
		return nil, fmt.Errorf("opencode/store: columns of %s: %w", table, err)
	}
	// Read-only rows: a close error loses nothing.
	defer func() { _ = rows.Close() }()
	set := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("opencode/store: columns of %s: %w", table, err)
		}
		set[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("opencode/store: columns of %s: %w", table, err)
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("opencode/store: %s has no %s table", d.path, table)
	}
	d.mu.Lock()
	d.cols[table] = set
	d.mu.Unlock()
	return set, nil
}

// selectList renders want as a SELECT list, substituting NULL for a column
// an older schema lacks, so one Scan layout works on every schema. Column
// names come from this package's constants, never from input. Pure.
func selectList(have map[string]bool, want []string) string {
	parts := make([]string, len(want))
	for i, c := range want {
		if have[c] {
			parts[i] = c
		} else {
			parts[i] = "NULL AS " + c
		}
	}
	return strings.Join(parts, ", ")
}

// requireColumns fails when the table lacks a column this reader cannot
// do without. Pure.
func requireColumns(table string, have map[string]bool, need []string) error {
	for _, c := range need {
		if !have[c] {
			return fmt.Errorf("opencode/store: %s has no column %s", table, c)
		}
	}
	return nil
}
