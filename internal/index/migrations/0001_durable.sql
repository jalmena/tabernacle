-- Durable tables: everything here holds state that is NOT recoverable from the
-- .song files, so it is migrated forward and never dropped.
--
-- Note what is absent. There is no favourites table: a favourite is the
-- reserved tag "favorite" written into the song file itself, so it survives a
-- database wipe, travels with a backup and shows up in a git diff as one line.
-- That is what makes "delete the index and lose nothing" literally true rather
-- than nearly true.

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
) WITHOUT ROWID;

-- Per-song reading state, keyed by slug rather than by a row id.
--
-- The slug is derived deterministically from the file's path, so it survives
-- the derived tables being dropped and rebuilt. A foreign key to songs(id)
-- would instead cascade this away on every rebuild, which is exactly the data
-- loss the two-tier design exists to prevent.
CREATE TABLE song_state (
    slug           TEXT PRIMARY KEY,
    transpose      INTEGER NOT NULL DEFAULT 0,
    notation       TEXT    NOT NULL DEFAULT '',  -- '' follows the global setting
    capo_view      TEXT    NOT NULL DEFAULT '',  -- '' means chords as written
    simplify       INTEGER NOT NULL DEFAULT 0,
    view_count     INTEGER NOT NULL DEFAULT 0,
    last_viewed_at INTEGER
) WITHOUT ROWID;

CREATE INDEX song_state_recent ON song_state(last_viewed_at DESC)
    WHERE last_viewed_at IS NOT NULL;

-- Where a slug used to point, so renaming a file redirects instead of breaking
-- a bookmark. Both sides are slugs rather than row ids, for the same reason as
-- above.
CREATE TABLE slug_history (
    old_slug TEXT PRIMARY KEY,
    new_slug TEXT NOT NULL,
    moved_at INTEGER NOT NULL
) WITHOUT ROWID;
