# The `.song` format, version 1

A song lives in one file. The file is the source of truth: the database is an
index that can be thrown away and rebuilt from the files at any time, and a
backup of a collection is a copy of a directory.

```
data/
  songs/
    Traditional/
      Amazing Grace.song
    Bill Withers/
      Ain't No Sunshine.song
```

A file has two parts: a metadata block delimited by `---`, and a body.

```
---
format: 1
title: Amazing Grace
artist: Traditional
key: G
capo: 2
tuning: EADGBE
tempo: 90
meter: 3/4
tags: [hymn, public-domain]
notes: |-
  Public domain: words by John Newton, 1779.
  Play it slowly and let the open strings ring.
---

## Verse 1
A[G]mazing grace how [C]sweet the [G]sound
That saved a [Em]wretch like [D]me [x2]

## Solo
```tab
e|-----------------|
B|-----------------|
G|-----0---2---0---|
D|-0---------------|
A|-----------------|
E|-----------------|
```
```

## Why this shape

The metadata block is structured because metadata is what gets searched,
sorted, filtered and validated. The body is plain text because the body is what
gets pasted, hand-edited and read at arm's length with a guitar in the way.

Putting the body inside the metadata block instead — a single YAML document with
the lyrics in block scalars — was the other candidate. It buys per-section
metadata and structural validation of the sections, and it costs an indentation
tax on every pasted line plus one failure mode where a stray tab anywhere makes
the whole song unreadable. The body is 95% of the file and the part that gets
edited most, so it wins the argument.

Chords are written inline, in brackets, at the syllable they land on. The other
common convention — a chord line positioned above a lyric line with spaces — is
unusable here: transposing `C` to `C#m` changes the width of the symbol and
breaks every alignment on the page, and the whole thing cannot be reflowed onto
a phone screen. A two-line layout can always be *rendered* from inline chords;
the reverse is guesswork.

## Metadata

The metadata block is a deliberately small subset of YAML: flat fields, scalar
values, one-level lists and literal block scalars. Everything the reader accepts
is also valid YAML with the same meaning, so a future switch to a real YAML
parser is not a format change.

| Field    | Type            | Notes                                                    |
| -------- | --------------- | -------------------------------------------------------- |
| `format` | integer         | Format version. Always written; assumed `1` when missing. |
| `title`  | string          | Required.                                                |
| `artist` | string          | Optional.                                                |
| `key`    | chord symbol    | `G`, `Am`, `F#m`, `Bb`. Drives transposition spelling.   |
| `capo`   | integer 0–12    | Fret the capo sits on. `0` means none and is omitted.    |
| `tuning` | string          | Free text: `EADGBE`, `Drop D`, `DADGAD`.                 |
| `tempo`  | integer 1–400   | Beats per minute.                                        |
| `meter`  | `N/N`           | Time signature: `4/4`, `6/8`.                            |
| `tags`   | list of strings | `[hymn, public-domain]` or a `-` list on following lines. |
| `notes`  | string          | Free text. Use `|-` for several lines.                   |

Unknown fields are an error rather than being ignored, so that `titel:` is
caught instead of silently dropping a title. Forward compatibility is handled by
bumping `format`.

Rules worth knowing:

- Fields are not nested and are indented with spaces, never tabs.
- A `#` at the start of a line is a comment. A `#` preceded by a space starts a
  trailing comment, exactly as in YAML, which is why `title: Song # 2` loses the
  `# 2`. Quote the value if you need it.
- `F#m` is safe unquoted: only a `#` after whitespace begins a comment.
- Values that a YAML reader would take for something other than a string are
  quoted automatically when the file is written: a band called `Yes`, an album
  called `1984`.
- Folded block scalars (`>`) are rejected. Use `|` or `|-`; both are read the
  same way, and trailing blank lines in a block are not significant.

## Body

Four constructs, and nothing else. This is not Markdown; `**bold**` is just
asterisks.

**Section headings** are `## Label`. The label is kept exactly as written and
also mapped onto a kind — verse, pre-chorus, chorus, bridge, solo, interlude,
outro, intro, tab — which drives styling, navigation and print layout. The
mapping tolerates numbering and Spanish: `Verse 2`, `VERSE II`, `Estrofa 2`,
`Chorus (x2)` and `Estribillo` all land where you would expect. An unrecognised
label keeps its text and gets a neutral kind.

Content before the first heading is allowed and goes into an unlabelled section,
because pasting a short song in as one block is a reasonable thing to do.

**Chords** are `[C]` placed inside the lyric at the syllable they fall on.
Practical guitar notation is supported: `Am`, `F#m7`, `Bb`, `Cmaj7`, `CM7`,
`Cadd9`, `Csus4`, `C7sus4`, `Cm7b5`, `Cdim7`, `C°`, `Caug`, `C+`, `C6/9`,
`Cm(maj7)`, `C7(b9)`, `Dno3`, and slash chords like `C/E` or `Fmaj7/A`. Root
names must be capitalised. `Bb5` is a Bb power chord, not a B with a flattened
fifth; write `B(b5)` for that.

Solfege input (`Do`, `Reb`, `Mim7`, `Sol/Si`) is understood by the importer and
can be chosen for display, but files always store English note names. One
canonical spelling on disk keeps diffs readable and transposition simple.

**Annotations** are the same brackets holding something that is not a chord:
`[x2]`, `[N.C.]`, `[let ring]`. They are kept verbatim and never transposed.
This is also the safety net that makes bracket syntax workable at all — a chord
symbol has to be built from known pieces, so `[Chorus]` is an annotation rather
than a C chord with the suffix `horus` that a transposition would turn into
`Dhorus`.

**Verbatim blocks** are fenced with three or more backticks and reproduced
exactly, which is what tablature needs:

````
```tab
e|---0---3---|
B|---1---0---|
```
````

Use more backticks in the fence than appear inside the block. The tag after the
fence (`tab`) is preserved.

**Blank lines** separate stanzas and are kept, because they are how a page gets
grouped for reading and for print. Blank lines at the edges of a section are
not.

**Escaping**: a backslash makes the next character literal. `\[`, `\\`, and a
leading `\#` or ``\` `` when a lyric would otherwise look like a heading or a
fence. A line in the body that starts with a single `#` is reserved and rejected,
so that it can be given a meaning later without breaking existing files.

## Transposition

Transposition moves roots and slash basses and leaves the quality suffix
untouched, so `Cm7b5` becomes `Dm7b5` and an exotic suffix survives intact
rather than being reinterpreted.

Sharps or flats are decided once for the whole sheet, from the destination key,
because a page that mixes `A#` and `Bb` is slower to read than either. Moving a
song in E up a semitone gives F, Bb and C rather than F, A# and C. An explicit
accidental in the declared key wins, so a song written in Gb stays in flats. A
song with no declared key falls back to its first chord.

Capo is left alone. It describes what the player does with a piece of metal, not
what the sheet says.

## Guarantees

- Reading a canonical file and writing it back returns the same bytes. Saving a
  song that was only transposed does not reflow the rest of the file.
- Writing and reading again always yields the same song.
- Anything the reader rejects, it rejects with a line number.

Two things are not preserved across a write: comments in the metadata block, and
a `format` older than the current one. Files are therefore only rewritten when
something actually changed.

## Deliberately not in version 1

- Per-section metadata such as repeat counts. `## Chorus (x2)` keeps the text;
  parsing it would mean inventing a small language in the heading, and that can
  wait until there is a real need.
- Chord diagrams, fingerings and custom chord definitions.
- Multiple arrangements or translations in one file. Use separate files.
- ChordPro import and export. The inline bracket notation is the same, so this
  is a converter rather than a format concern.

## Reserved tags

One tag has a meaning to the application: `favorite` marks a song as a
favourite.

It lives in the file rather than in the database on purpose. A favourite then
survives the index being deleted, travels with a copy of the collection, and
appears in a `git diff` as a single line. The cost is that starring a song writes
to its file, which is fair for something done deliberately.

Any other tag is yours. Nothing else is reserved, and a tag called `favorite`
that you added by hand means exactly what the star means.

## What the application derives

Nothing in this list belongs in the file, and writing it there by hand would only
create something to fall out of step:

- the chords a song uses, and their order of first appearance,
- guitar shapes for those chords,
- the song's URL, which comes from its path,
- how many sections and lines it has,
- the searchable text, which is the words with the chord brackets removed.
