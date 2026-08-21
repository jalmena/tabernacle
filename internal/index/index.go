// Package index is the only part of Tabernacle that touches SQL.
//
// It holds a search index over the collection, derived metadata, and the small
// amount of state that genuinely is not in the song files. Song content never
// lives here: deleting the database and starting again must lose nothing but
// time, and the tests assert exactly that.
//
// The schema comes in two tiers. Derived tables are recomputed from the .song
// files, so when their shape changes they are dropped and rebuilt by a full
// scan and there is no migration to get wrong. Durable tables — settings,
// per-song reading state and slug redirects — hold what cannot be recovered
// from the files, so they are migrated forward and never dropped.
package index

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

//go:embed derived.sql
var derivedSchema string

// derivedSchemaVersion is bumped whenever derived.sql changes. A mismatch
// against what the database records causes the derived tables to be rebuilt and
// NeedsFullScan to report true.
const derivedSchemaVersion = 3

const settingDerivedVersion = "derived_schema_version"

// Index is an open connection to the index database.
type Index struct {
	db *sql.DB
	// Read by request handlers and by the rescan ticker, written once at open
	// and once when the first scan finishes, so it is atomic rather than plain.
	needsFullScan atomic.Bool
}

// Open connects to the index, applies migrations and reports whether a full
// rescan of the collection is required.
func Open(ctx context.Context, path string) (*Index, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("index: open %s: %w", path, err)
	}

	// One connection. WAL plus concurrent writers is how a SQLite application
	// discovers SQLITE_BUSY in production and never in its tests; serialising
	// access removes that whole class of bug, and at the scale of a personal
	// collection it costs nothing measurable. If it ever does, the upgrade is a
	// read pool alongside a single writer.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("index: connect to %s: %w", path, err)
	}

	x := &Index{db: db}
	if err := x.checkFTS5(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := x.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return x, nil
}

// dsn builds the connection string.
//
// The pragmas are set here rather than issued afterwards so that they apply to
// the connection from the first statement, including the ones a migration runs.
func dsn(path string) string {
	u := url.URL{Scheme: "file", Path: path}
	u.RawQuery = strings.Join([]string{
		"_pragma=journal_mode(WAL)",   // readers never block the writer
		"_pragma=busy_timeout(5000)",  // wait rather than fail on a lock
		"_pragma=foreign_keys(1)",     // the cascades in derived.sql must work
		"_pragma=synchronous(NORMAL)", // WAL makes this durable enough here
	}, "&")
	return u.String()
}

// Close releases the connection.
func (x *Index) Close() error { return x.db.Close() }

// NeedsFullScan reports whether the derived tables were just rebuilt and so
// hold nothing yet.
func (x *Index) NeedsFullScan() bool { return x.needsFullScan.Load() }

// FullScanDone records that the collection has been read into the rebuilt
// tables, so the next caller is not told to scan all over again.
func (x *Index) FullScanDone() { x.needsFullScan.Store(false) }

// checkFTS5 fails loudly at startup rather than at the first search.
//
// Full-text search is not optional: it is how the library is browsed, and a
// build of the driver without FTS5 would otherwise look fine until somebody
// typed in the search box.
func (x *Index) checkFTS5(ctx context.Context) error {
	_, err := x.db.ExecContext(ctx,
		`CREATE VIRTUAL TABLE temp.fts5_probe USING fts5(x, tokenize="unicode61 remove_diacritics 2")`)
	if err != nil {
		return fmt.Errorf("index: this build of SQLite has no usable FTS5 support: %w", err)
	}
	_, err = x.db.ExecContext(ctx, `DROP TABLE temp.fts5_probe`)
	return err
}

// migrate brings the schema up to date: durable migrations forward, derived
// tables rebuilt when their shape has changed.
func (x *Index) migrate(ctx context.Context) error {
	names, err := migrationNames()
	if err != nil {
		return err
	}

	var userVersion int
	if err := x.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&userVersion); err != nil {
		return fmt.Errorf("index: read schema version: %w", err)
	}

	for i, name := range names {
		version := i + 1
		if version <= userVersion {
			continue
		}
		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("index: read migration %s: %w", name, err)
		}
		if err := x.inTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("index: apply migration %s: %w", name, err)
			}
			// PRAGMA does not accept a parameter, and version is an int we
			// derived from the file list, never from input.
			_, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", version))
			return err
		}); err != nil {
			return err
		}
	}

	// The derived tier.
	have, err := x.settingInt(ctx, settingDerivedVersion)
	if err != nil {
		return err
	}
	if have != derivedSchemaVersion {
		if err := x.inTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, derivedSchema); err != nil {
				return fmt.Errorf("index: rebuild derived schema: %w", err)
			}
			_, err := tx.ExecContext(ctx,
				`INSERT INTO settings(key, value) VALUES(?, ?)
				 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
				settingDerivedVersion, fmt.Sprint(derivedSchemaVersion))
			return err
		}); err != nil {
			return err
		}
		x.needsFullScan.Store(true)
	}
	return nil
}

// migrationNames lists the durable migrations in the order they must run.
func migrationNames() ([]string, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("index: list migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	// Filenames are zero-padded and numbered, so lexicographic order is
	// numeric order.
	sort.Strings(names)
	if len(names) == 0 {
		return nil, errors.New("index: no migrations were embedded")
	}
	return names, nil
}

// inTx runs fn in a transaction, rolling back on any error.
func (x *Index) inTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := x.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
