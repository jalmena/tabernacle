# Tabernacle

**Your sacred repository of guitar tabs.**

A self-hosted personal guitar songbook. Your songs are plain text files in a directory you own, so
the collection is yours to back up, edit by hand, put in git and keep for twenty years. Tabernacle
adds the things a songbook needs — fast search, transposition, chord diagrams with alternative
voicings, note names in either C-D-E or Do-Re-Mi, and a printed sheet that is actually worth
putting in a ring binder.

It runs as a single binary or a single container, works offline, sends nothing anywhere, and has no
accounts.

> **Status: under construction.** The `.song` format and the chord domain are complete and tested.
> The web interface is being built. See [Roadmap](#roadmap) for what works today.

---

## Screenshots

<!-- TODO: library view, song sheet on desktop, song sheet on a phone, a chord popover
     with voicing navigation, and a printed A4 sheet. -->

*Coming once the interface lands.*

---

## Features

**The songbook**

- Songs are `.song` text files in `data/songs/Artist/Title.song`. That is the whole storage design.
- Full-text search across titles, artists, lyrics, tags and chords, accent-insensitive so
  `corazon` finds `corazón`.
- Browse by artist, by tag, by chord, by favourites, by recently opened.
- Paste a song in and save it, or edit the raw `.song` text with syntax errors reported by line.

**Playing**

- Transpose up and down a semitone at a time. Sharps or flats are chosen once for the whole sheet
  from the destination key, so a page never mixes `A#` and `Bb`.
- Every chord on the page is tappable and opens a diagram with fingering, without navigating away
  or losing your place.
- Several practical voicings per chord — open position, barre shapes, higher positions — with
  left/right navigation. Real playable shapes only.
- A "chords used" panel at the top so you can learn the shapes before you start.
- Note names in English (`C D E F G A B`) or solfège (`Do Re Mi Fa Sol La Si`), switchable at any
  time. One file serves both; switching never rewrites anything.
- Capo and tuning are first-class metadata, and chords can be shown as written or as fingered with
  the capo on.

**Practical matters**

- A print stylesheet that produces a real A4 song sheet: no navigation, no buttons, no stanzas
  split across a page break.
- Mobile-first, because the usual reading device is a phone propped against a guitar case.
- Import and export `.song` files individually or as a zip.
- Works with JavaScript disabled. Everything essential is a plain link or form.

**What it deliberately does not do**

No accounts, no telemetry, no analytics, no cloud, no external APIs, no scraping of chord websites.
It needs the internet exactly once: to be installed.

---

## Quick start

### Docker Compose

```sh
git clone https://github.com/jalmena/tabernacle.git
cd tabernacle
docker compose up -d
```

Open <http://localhost:8080>. Songs live in `./data/songs`.

Set `PUID` and `PGID` in `compose.yaml` to whoever owns that directory — `id -u` and `id -g` will
tell you. On a NAS this is usually the difference between a working install and a permissions error.

### CasaOS, Zima, Portainer, Unraid

Tabernacle ships a compose file with the metadata these platforms want, so there is nothing to
adapt.

- **CasaOS / ZimaOS** — App Store → **Custom Install** → *Import* → paste
  [`compose.yaml`](compose.yaml). Adjust the volume to `/DATA/AppData/tabernacle/data:/data` and
  set `PUID`/`PGID`, then install.
- **Portainer** — Stacks → **Add stack** → Web editor → paste `compose.yaml` → Deploy.
- **Unraid / Synology** — use the compose file, or create a container from
  `ghcr.io/jalmena/tabernacle:latest` with `/data` mapped to a share and `PUID`/`PGID` set.

Images are published for `linux/amd64` and `linux/arm64`, so a Raspberry Pi or an ARM NAS works the
same as an x86 box.

### From source

Requires Go 1.25 or newer.

```sh
go build ./cmd/tabernacle
TABERNACLE_DATA_DIR=./data ./tabernacle
```

---

## Docker deployment

```yaml
services:
  tabernacle:
    image: ghcr.io/jalmena/tabernacle:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    environment:
      PUID: "1000"
      PGID: "1000"
```

The container listens on plain HTTP and runs as a non-root user. **TLS is handled outside**, which
is the normal shape for a homelab:

```
VPN / trusted LAN
       |
       v
     Caddy  (TLS terminates here)
       |
       v
  Tabernacle
     /    \
 index.db  data/songs/*.song
```

A minimal `Caddyfile`:

```caddy
songs.example.com {
	reverse_proxy tabernacle:8080
}
```

When you put a real domain in front of it, set `TABERNACLE_BASE_URL` and
`TABERNACLE_ALLOWED_HOSTS` to that hostname.

### A word on access control

Tabernacle has **no authentication**. Anyone who can reach it can read, edit and delete songs. That
is a deliberate choice for a personal tool on a trusted network, and it means you should not expose
it directly to the internet. Put it behind a VPN such as WireGuard or Tailscale, or behind an
authenticating proxy.

Out of the box Tabernacle accepts requests addressed to LAN addresses, `localhost` and names like
`nas.local`, and refuses public hostnames. That blocks DNS rebinding — a website you visit
resolving its own name to your NAS and then talking to Tabernacle from your browser — without
needing any configuration. `TABERNACLE_READ_ONLY=true` refuses every change, which is a reasonable
way to share the library with guests.

---

## Configuration

Everything is an environment variable, and every one has a sensible default.

| Variable | Default | What it does |
| --- | --- | --- |
| `TABERNACLE_ADDR` | `:8080` | Address to listen on. |
| `TABERNACLE_DATA_DIR` | `/data` | Base directory for songs, trash and the index. |
| `TABERNACLE_SONGS_DIR` | `$DATA_DIR/songs` | The collection. Override to point at an existing library. |
| `TABERNACLE_TRASH_DIR` | `$DATA_DIR/trash` | Deleted songs land here instead of being erased. |
| `TABERNACLE_DB_PATH` | `$DATA_DIR/index.db` | The search index. Safe to delete; it rebuilds. |
| `TABERNACLE_BASE_URL` | `http://localhost:8080` | Canonical URL. Seeds the host allowlist. |
| `TABERNACLE_ALLOWED_HOSTS` | *(LAN policy)* | Comma-separated exact hostnames to accept. |
| `TABERNACLE_SCAN_INTERVAL` | `60s` | How often to notice files changed outside the app. `0` disables it. |
| `TABERNACLE_NOTATION` | `english` | Default note names: `english` or `solfege`. |
| `TABERNACLE_READ_ONLY` | `false` | Refuse every modification. |
| `TABERNACLE_MAX_UPLOAD_BYTES` | `33554432` | Cap on one import request. |
| `TABERNACLE_MAX_SONG_BYTES` | `1048576` | Cap on one `.song` file. |
| `TABERNACLE_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. |
| `PUID` / `PGID` | `1000` | Container only: user and group that should own the data directory. |
| `TZ` | *(UTC)* | Container only: time zone. |

Invalid configuration fails at startup and reports **every** problem at once, naming the variable:

```
tabernacle: invalid configuration:
TABERNACLE_NOTATION must be "english" or "solfege", got "spanish"
TABERNACLE_LOG_LEVEL "chatty": use debug, info, warn or error
```

---

## Data directory

```
data/
  songs/                        your collection — the only thing that matters
    Traditional/
      Amazing Grace.song
    Bill Withers/
      Ain't No Sunshine.song
  trash/                        deleted songs, kept rather than erased
  index.db                      search index — derived, disposable
```

**The files are the source of truth.** The database holds only the search index, derived metadata
and small conveniences such as which songs you opened recently. Delete `index.db` and Tabernacle
rebuilds it from the files on the next start, with the same URLs as before.

Song content never lives only in the database. Favourites are stored as a `favorite` tag inside the
song file itself, so they survive a database wipe and travel with your backup.

You can edit songs with any text editor while Tabernacle is running; it notices within a minute, or
immediately if you press Reindex.

---

## The `.song` format

A song is one file: a small metadata block, then the words with chords in brackets where they fall.

````
---
format: 1
title: Amazing Grace
artist: Traditional
key: G
capo: 2
tuning: EADGBE
tags: [hymn, public-domain]
---

## Verse 1
A[G]mazing grace how [C]sweet the [G]sound
That saved a [Em]wretch like [D]me [x2]

## Solo
```tab
e|-----------------|
G|-----0---2---0---|
```
````

It is designed to be pleasant to type, readable in a text editor, and friendly to `git diff`.
Chords go inline rather than on a line above the lyrics, because transposing changes the width of a
chord symbol and destroys any alignment — and a two-line layout can always be *rendered* from
inline chords, while the reverse is guesswork.

The full specification, including the reasoning behind each decision, is in
**[docs/song-format.md](docs/song-format.md)**.

You never maintain a list of the chords a song uses. That is derived from the song.

---

## Backup and restore

**Backup** is copying a directory:

```sh
tar czf tabernacle-backup-$(date +%F).tar.gz data/songs
```

Or use the web interface: **Export** downloads the whole collection as a zip.

**Restore** is putting the files back:

```sh
tar xzf tabernacle-backup-2026-08-20.tar.gz
```

You do not need to back up `index.db`. There is no proprietary export step, no database dump, and
nothing to migrate. If Tabernacle disappeared tomorrow your songs would still be readable text.

---

## Import and export

- **Import** one or many `.song` files, or a `.zip` of them. Every file is validated before
  anything is written, and you get a report naming what was added, skipped or rejected and why. A
  single bad file never aborts the rest of the import.
- **Export** the whole collection or a filtered subset as a zip.
- **Download** any single song as its original `.song` file.

Imported files are treated as untrusted input.

---

## The chord system

Chords are parsed rather than pattern-matched, which is what makes transposition trustworthy.

A chord is a root note, a quality suffix and an optional slash bass. The suffix is validated but
kept verbatim, so transposition moves the root and the bass and leaves the quality alone:

| | |
| --- | --- |
| `C` → `D` | `Cm` → `Dm` |
| `C7` → `D7` | `Cmaj7` → `Dmaj7` |
| `C/E` → `D/F#` | `Cm7b5` → `Dm7b5` |

Practical guitar notation is understood: `Am`, `F#m7`, `Bb`, `Cadd9`, `Csus4`, `C7sus4`, `Cdim7`,
`C°`, `Caug`, `C6/9`, `Cm(maj7)`, `C7(b9)`, `Dno3`, `Fmaj7/A`.

Sharps or flats are decided **once per sheet** from the destination key, because a page mixing `A#`
and `Bb` is slower to read than either. A song in E moved up a semitone gives F, Bb and C — not F,
A# and C. A song you wrote in Gb stays in flats.

Bracketed text that is not a chord is left alone: `[x2]`, `[N.C.]` and `[let ring]` are annotations
and are never transposed. This is also what stops `[Chorus]` from being read as a C chord.

Capo is never touched by transposition. It describes what you do with a piece of metal, not what
the sheet says.

### Chord diagrams

Diagrams are generated from structured data — string, fret, finger, barre — not stored as images.
Each chord offers several genuinely playable voicings, ordered easiest first, and you can page
through them without closing the panel. Theoretically valid but unplayable fingerings are not
included to inflate the count.

If a chart uses chords with no practical shape, nothing is silently substituted. Instead the sheet
offers one control that simplifies *all* of them at once and says so — the campfire version of a
jazz chart, with every chord relabelled honestly.

---

## English and Spanish notation

Both note-name systems are fully supported, everywhere: in the lyrics, in the chords-used panel, in
chord diagrams, in transposition results and in search.

| Internal | English | Solfège |
| --- | --- | --- |
| C | `C` | `Do` |
| B | `B` | `Si` |
| C♯ | `C#` | `Do#` |
| D♭ | `Db` | `Reb` |
| B♭ | `Bb` | `Sib` |
| F♯ | `F#` | `Fa#` |

Notation is **presentation only**. Switching it never modifies a file. Files always store English
note names so that a collection stays diffable and unambiguous, and so that one file serves a
Spanish reader and an English one without keeping two copies of the same song.

Solfège chord symbols are understood on the way *in* too, so you can paste a sheet written
`Do Sol Lam Fa` and it will be stored and displayed correctly.

---

## Development

Requires Go 1.25 or newer. No other tooling, no code generation, no build step for assets.

```sh
go build ./cmd/tabernacle     # build
go test ./...                 # tests
go test -race ./...           # tests, with the race detector
gofmt -l .                    # formatting (should print nothing)
go vet ./...                  # vet
```

Fuzz targets exist for the parts that read untrusted input:

```sh
go test ./internal/song -run FuzzParse -fuzz FuzzParse -fuzztime 30s
```

Build the container:

```sh
docker build --build-arg VERSION=dev -t tabernacle:dev .
```

### Dependencies

The musical domain — chord parsing, transposition, the `.song` format — has **no dependencies at
all** and is intended to stay that way.

The application has exactly one direct dependency: **`modernc.org/sqlite`**, a pure-Go SQLite. It
is chosen over the faster cgo `mattn/go-sqlite3` because `CGO_ENABLED=0` yields a static binary,
means no C toolchain is needed to build the project anywhere, and makes the arm64 image free to
cross-compile. At personal-library scale the performance difference is irrelevant and the
build simplicity is not.

For the record, the actual cost is one direct dependency and nine indirect ones, all from the
same well-maintained family:

```
modernc.org/sqlite  modernc.org/libc  modernc.org/mathutil  modernc.org/memory
golang.org/x/sys  github.com/dustin/go-humanize  github.com/google/uuid
github.com/mattn/go-isatty  github.com/ncruces/go-strftime
github.com/remyoudompheng/bigfft
```

Anything else needs a concrete reason that the standard library cannot address.

---

## Architecture

Layers depend strictly downwards, so the musical domain never learns about HTTP, HTML or SQL.

```
cmd/tabernacle        configuration, wiring, startup sync, graceful shutdown
  internal/web        handlers, middleware, embedded templates and assets
    internal/render   Song -> view model            (no HTTP, no SQL, no filesystem)
      internal/diagram  a shape -> SVG              (standard library only)
      internal/voicing  chord identity -> playable guitar shapes
        internal/harmony  chord symbol -> quality, intervals, simplification
    internal/library  use cases; keeps files and index consistent
      internal/songfs   the songs directory: confinement, scan, atomic writes
      internal/index    SQLite: migrations, search, application state
      internal/importer pasted text -> a song
        internal/song   the .song format: parse and serialise
          internal/chord chords, note names, transposition
  internal/config     environment -> configuration
```

Consequences worth knowing:

- `internal/chord` and `internal/song` are pure and independently testable. Reading a `.song` file
  and writing it back is guaranteed to produce identical bytes, so saving a song never churns the
  file.
- `internal/diagram` cannot see the chord types at all, so diagram rendering and chord
  representation cannot become entangled.
- Every write goes to the file first and the index second. A file written with a failed index
  update is fixed by the next scan; the other order would lose data.
- The filesystem is reached only through a confined root, so path traversal is prevented at the
  syscall level rather than filtered out with string checks.

---

## Roadmap

Complete and tested:

- [x] The `.song` format: parsing, serialisation, versioning, byte-identical round trip, fuzzing
- [x] Chords: parsing, note names in both systems, transposition, enharmonic spelling
- [x] Configuration, the binary, health endpoint, container image, compose packaging
- [x] Song storage: a confined collection directory, atomic writes, rename detection
- [x] The search index, rebuildable from the files at any time
- [x] Library browsing, full-text search, tags and artists
- [x] The song sheet: transposition, both note-name systems, the capo view, printing

- [x] Chord diagrams, alternative voicings, and the chord panel

- [x] Authoring: paste a song, edit the raw file, favourites, delete to trash

In progress, in this order:

- [ ] Import and export

Later: setlists, and whatever turns out to be genuinely useful rather than merely possible.

---

## Copyright and musical content

**Tabernacle is software for managing musical content that you provide.** Please read this section;
the distinction matters.

- Tabernacle **does not provide, supply, license or distribute any musical work**. It ships with no
  song library. The few example files included are original or public-domain content, marked as
  such.
- The rights in any song, lyric, tab or chord sheet you store, import, export, display, print or
  share are **entirely separate** from the licence covering this software. A licence to use
  Tabernacle grants you no rights whatsoever in anyone's music.
- **You are responsible** for ensuring that the content you put into Tabernacle, and anything you
  do with it, complies with the law that applies to you. Rules on private copying, personal use and
  sharing differ considerably between countries.
- Tabernacle does not and will not scrape Ultimate Guitar, LaCuerda or any other site, download
  copyrighted content, bypass paywalls or circumvent access controls. There is no import-from-URL
  feature, and that is on purpose.
- Please do not open pull requests adding copyrighted lyrics, tabs or chord sheets. They will be
  declined.

This section describes the project's position. It is not legal advice. If you need certainty about
your situation, ask a lawyer.

---

## Licence

Tabernacle is free software, licensed under the **GNU General Public License version 3**. The full
text is in [LICENSE](LICENSE).

In plain terms: you may use, study, modify and share it, including commercially — GPLv3 permits
commercial use. If you distribute a modified version, you must release your changes under the same
licence.

The GPLv3 applies to **the software only**. It grants no rights in any musical work, and it imposes
no conditions on the songs in your collection — your songs are yours, and Tabernacle claims nothing
over them.

No additional restrictions are placed on the GPLv3.

---

## Contributing

Contributions are welcome. The project values boring, durable technology and being genuinely
pleasant for musicians, in that combination.

Before opening a pull request:

- Run `gofmt -l .`, `go vet ./...` and `go test -race ./...`.
- Add tests for anything touching the `.song` format, chord logic or a security boundary.
- Explain *why* in commit messages and comments. The *what* is visible in the diff.
- New dependencies need a justification that the standard library genuinely cannot cover.
- No copyrighted musical content, ever.

If you play guitar and something about the reading, transposing or printing experience is annoying
in practice, that is a valuable bug report. Please open an issue.
