-- Derived tables: every column here is recomputed from the .song files, so
-- these are never migrated. When their shape changes they are dropped and
-- rebuilt by a full scan, which means there is no migration to get wrong.

DROP TABLE IF EXISTS song_fts;
DROP TABLE IF EXISTS song_chords;
DROP TABLE IF EXISTS song_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS songs;

CREATE TABLE songs (
    id               INTEGER PRIMARY KEY,
    rel_path         TEXT    NOT NULL UNIQUE,  -- slash separated, always inside the collection
    slug             TEXT    NOT NULL UNIQUE,
    title            TEXT    NOT NULL,
    artist           TEXT    NOT NULL DEFAULT '',
    song_key         TEXT    NOT NULL DEFAULT '',
    capo             INTEGER NOT NULL DEFAULT 0,
    tuning           TEXT    NOT NULL DEFAULT '',
    genre            TEXT    NOT NULL DEFAULT '',
    genre_fold       TEXT    NOT NULL DEFAULT '',
    notes            TEXT    NOT NULL DEFAULT '',
    sort_title       TEXT    NOT NULL,
    sort_artist      TEXT    NOT NULL,
    section_count    INTEGER NOT NULL DEFAULT 0,
    line_count       INTEGER NOT NULL DEFAULT 0,
    size_bytes       INTEGER NOT NULL DEFAULT 0,
    mtime_unix_ns    INTEGER NOT NULL DEFAULT 0,
    content_hash     TEXT    NOT NULL,
    -- A file that will not parse keeps its row and its last good content, so
    -- search still finds it and the editor can open it to be fixed.
    parse_error      TEXT    NOT NULL DEFAULT '',
    parse_error_line INTEGER NOT NULL DEFAULT 0,
    first_seen_at    INTEGER NOT NULL,
    indexed_at       INTEGER NOT NULL
);

CREATE INDEX songs_by_title  ON songs(sort_title);
CREATE INDEX songs_by_artist ON songs(sort_artist, sort_title);
CREATE INDEX songs_by_hash   ON songs(content_hash);   -- rename detection
CREATE INDEX songs_broken    ON songs(id) WHERE parse_error <> '';
CREATE INDEX songs_by_genre  ON songs(genre_fold) WHERE genre_fold <> '';

-- The chords a song actually uses, derived from its content. The user never
-- maintains this list, which is the same rule the .song format applies to the
-- "chords used" section.
CREATE TABLE song_chords (
    song_id INTEGER NOT NULL REFERENCES songs(id) ON DELETE CASCADE,
    symbol  TEXT    NOT NULL,  -- canonical English spelling, e.g. "F#m7"
    ord     INTEGER NOT NULL,  -- order of first appearance
    PRIMARY KEY (song_id, symbol)
) WITHOUT ROWID;
CREATE INDEX song_chords_by_symbol ON song_chords(symbol, song_id);

-- rowid is kept equal to songs.id and maintained in the same transaction as the
-- songs write. No triggers: Go-side maintenance is visible in one place and can
-- be tested.
--
-- remove_diacritics 2 is what makes "corazon" find "corazon" with an accent,
-- which plain LIKE cannot do without a second folded copy of every lyric.
-- The table is NOT contentless, because snippet() needs the text to build the
-- excerpt shown in search results.
CREATE VIRTUAL TABLE song_fts USING fts5(
    title,
    artist,
    genre,
    lyrics,
    notes,
    chords,
    tokenize = "unicode61 remove_diacritics 2",
    prefix   = '2 3'
);
