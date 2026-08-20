package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// SongState is a reader's saved preferences for one song.
//
// All of it is a convenience rather than data: losing it costs the reader a
// couple of clicks, which is why it may live in the database at all. Notation
// and CapoView are empty when the song follows the global default.
type SongState struct {
	Transpose  int
	Notation   string
	CapoView   string
	Simplify   bool
	ViewCount  int
	LastViewed time.Time
}

// Setting returns a stored setting, or "" when it has never been set.
func (x *Index) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := x.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("index: read setting %q: %w", key, err)
	}
	return v, nil
}

// SetSetting stores a setting.
func (x *Index) SetSetting(ctx context.Context, key, value string) error {
	_, err := x.db.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("index: write setting %q: %w", key, err)
	}
	return nil
}

func (x *Index) settingInt(ctx context.Context, key string) (int, error) {
	v, err := x.Setting(ctx, key)
	if err != nil || v == "" {
		return 0, err
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		// A corrupt value should trigger a rebuild rather than an outage.
		return 0, nil
	}
	return n, nil
}

// State returns a song's saved reading state. A song that has never been opened
// has a zero state, which is the correct default rather than an error.
func (x *Index) State(ctx context.Context, slug string) (SongState, error) {
	var (
		s          SongState
		lastViewed sql.NullInt64
	)
	err := x.db.QueryRowContext(ctx,
		`SELECT transpose, notation, capo_view, simplify, view_count, last_viewed_at
		 FROM song_state WHERE slug = ?`, slug,
	).Scan(&s.Transpose, &s.Notation, &s.CapoView, &s.Simplify, &s.ViewCount, &lastViewed)
	if errors.Is(err, sql.ErrNoRows) {
		return SongState{}, nil
	}
	if err != nil {
		return SongState{}, fmt.Errorf("index: read state for %q: %w", slug, err)
	}
	if lastViewed.Valid {
		s.LastViewed = time.Unix(0, lastViewed.Int64)
	}
	return s, nil
}

// SetState saves a song's reading preferences, leaving the view counters alone.
func (x *Index) SetState(ctx context.Context, slug string, s SongState) error {
	_, err := x.db.ExecContext(ctx,
		`INSERT INTO song_state(slug, transpose, notation, capo_view, simplify)
		 VALUES(?, ?, ?, ?, ?)
		 ON CONFLICT(slug) DO UPDATE SET
		     transpose = excluded.transpose,
		     notation  = excluded.notation,
		     capo_view = excluded.capo_view,
		     simplify  = excluded.simplify`,
		slug, s.Transpose, s.Notation, s.CapoView, s.Simplify)
	if err != nil {
		return fmt.Errorf("index: write state for %q: %w", slug, err)
	}
	return nil
}

// RecordView notes that a song was opened, which is what drives the
// recently-played list.
func (x *Index) RecordView(ctx context.Context, slug string, at time.Time) error {
	_, err := x.db.ExecContext(ctx,
		`INSERT INTO song_state(slug, view_count, last_viewed_at)
		 VALUES(?, 1, ?)
		 ON CONFLICT(slug) DO UPDATE SET
		     view_count     = view_count + 1,
		     last_viewed_at = excluded.last_viewed_at`,
		slug, at.UnixNano())
	if err != nil {
		return fmt.Errorf("index: record view of %q: %w", slug, err)
	}
	return nil
}

// MoveState carries a song's saved state to a new slug after a rename.
//
// Renames are detected by content hash during a scan, so moving a song into a
// different artist folder keeps its transposition and its play count instead of
// silently starting over.
func (x *Index) MoveState(ctx context.Context, oldSlug, newSlug string) error {
	if oldSlug == newSlug {
		return nil
	}
	return x.inTx(ctx, func(tx *sql.Tx) error {
		// The destination may already exist if the reader opened the song at its
		// new path before the scan noticed; in that case the newer state wins
		// and the old row is simply dropped.
		if _, err := tx.ExecContext(ctx,
			`UPDATE OR IGNORE song_state SET slug = ? WHERE slug = ?`, newSlug, oldSlug); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM song_state WHERE slug = ?`, oldSlug); err != nil {
			return err
		}
		// Leave a redirect so existing links keep working.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO slug_history(old_slug, new_slug, moved_at) VALUES(?, ?, ?)
			 ON CONFLICT(old_slug) DO UPDATE SET new_slug = excluded.new_slug, moved_at = excluded.moved_at`,
			oldSlug, newSlug, time.Now().UnixNano()); err != nil {
			return err
		}
		// Anything that used to point at the old slug should now point onward,
		// so a song renamed twice does not need two hops.
		_, err := tx.ExecContext(ctx,
			`UPDATE slug_history SET new_slug = ? WHERE new_slug = ? AND old_slug <> ?`,
			newSlug, oldSlug, newSlug)
		return err
	})
}

// Redirect resolves a slug that no longer exists to the one that replaced it.
func (x *Index) Redirect(ctx context.Context, oldSlug string) (string, bool, error) {
	var to string
	err := x.db.QueryRowContext(ctx,
		`SELECT new_slug FROM slug_history WHERE old_slug = ?`, oldSlug).Scan(&to)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("index: look up redirect for %q: %w", oldSlug, err)
	}
	return to, true, nil
}

// forgetRedirect drops a redirect, used when a slug comes back into existence.
func (x *Index) forgetRedirect(ctx context.Context, tx *sql.Tx, slug string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM slug_history WHERE old_slug = ?`, slug)
	return err
}
