# Chord diagrams

Diagrams are generated from structured data. There are no images of chords in
this repository and there will not be.

## The three packages

| Package | Question it answers |
| --- | --- |
| `internal/harmony` | What kind of chord is this? |
| `internal/voicing` | Which shapes play it? |
| `internal/diagram` | What does a shape look like? |

They are separate because those are genuinely different questions.
`internal/diagram` imports nothing from the rest of the project, not even the
chord types — a diagram is a picture of fret numbers, and keeping it ignorant of
how chords are modelled is what stops the two becoming entangled.

`internal/harmony` exists because `internal/chord` deliberately treats a quality
suffix as opaque, and that opacity is load-bearing: it is why `Cmaj7#11` survives
a transposition instead of being mangled. Interpreting the suffix is a separate
concern, so it lives separately.

## The dataset

`internal/voicing/data/chords.json`, embedded in the binary.

Shapes are curated rather than generated. Generating every fingering that
satisfies a chord's notes produces mostly things no hand would attempt, and the
point of a diagram is to show something playable.

There are two kinds of entry.

**Curated** shapes are written out in full, with absolute fret numbers. This is
how open-position fingerings get in, because those are idiomatic rather than
derived — `x 3 2 0 1 0` is C because that is how people play C, not because it
falls out of a rule.

**Movable** shapes are templates with frets relative to their own lowest line,
which slide up the neck. A handful of them covers all twelve roots. They are
never emitted at the nut: a template's relative zero means "the barre line", and
at the nut that silently becomes "open string", which changes both the sound and
the fingering.

### Notation

Fret and finger specifications are read **lowest string first** — E A D G B e —
which is the order chord grids are written in.

```
"frets":   "x 0 2 0 1 0"     x = muted, 0 = open (or the barre line in a template)
"fingers": "- - 2 - 1 -"     - = no finger, 1-4 = fingers, T = thumb
```

Separators rather than `x02010`, for one reason: the compact form breaks the
moment a fret reaches double figures, and a hand-curated dataset lives or dies on
being reviewable. This is the same trade the `.song` format itself makes.

### Adding a shape

```json
{ "chord": "Am7", "shapes": [
  { "id": "open", "frets": "x 0 2 0 1 0", "fingers": "- - 2 - 1 -" }
]}
```

For a movable template, add `root` (which string carries the root),
`minFret`/`maxFret`, and a `barre` if there is one:

```json
{ "quality": "min7", "shapes": [
  { "id": "a-shape", "root": 1, "frets": "x 0 2 0 1 0", "fingers": "- 1 3 1 2 1",
    "barre": { "fret": 0, "from": 1, "to": 5, "finger": 1 },
    "minFret": 1, "maxFret": 11 }
]}
```

Barres are recorded, **never inferred**. E major is `0 2 2 1 0 0`: two strings at
the second fret, played with two separate fingers. Inferring a barre from that
would draw a shape nobody plays.

### The validator

`TestCuratedShapesAreCorrect` and `TestMovableShapesAreCorrectAtEveryRoot` are
what make a hand-typed table trustworthy. Using the interval tables from
`internal/harmony` and the tuning's MIDI numbers, they prove that every compiled
shape:

- sounds only notes belonging to the chord,
- contains its root and the interval that defines its family,
- contains the seventh when it claims to be a seventh chord,
- has a finger on every fretted string, or a barre covering it,
- satisfies the barre invariant — every string in the span at or above the barre
  fret, both ends exactly on it, which is subtler than it looks because
  `Bm7 = x 2 4 2 3 2` stacks two fingers on top of its own barre,
- spans no more than four frets and sits no higher than the twelfth.

So a mistyped fret number fails the build rather than teaching somebody a chord
that is not the chord.

## Ordering

Shapes are sorted by a playability cost: fretted notes, stretch, barre width,
position, muted strings inside the shape, thumbed notes. The weights matter less
than two properties, both pinned by tests:

1. An open shape always beats a barre.
2. The order is deterministic, because the interface says "2 de 4" and a deep
   link names a shape.

`TestPrimaryShapes` pins the shape offered first for about forty chords against
what a chord reference prints. A change there means the cost function moved,
which may be right but should never be accidental.

## When there is no shape

A lookup answers with shapes that voice the chord as written, or with nothing.
There is no substitution: a card headed `Cmaj7#11` never carries `Cmaj7`'s dots,
because the reader has no way to tell the difference between a picture of their
chord and a picture of a nearby one. Decoration the model does not cover counts
as a difference and not as a detail to round off — the b9 is the whole point of
`C7b9`.

So a chord the collection cannot play gets no diagram and says so, and the card
points at the control that can do something about it. No diagram is better than a
wrong one.

Simplifying is worth doing, but it belongs to the whole sheet at once, where it
can be announced and turned off again — see [Simplifying a sheet](#simplifying-a-sheet).

## Simplifying a sheet

`Simplificar` rewrites the chords of one sheet into ones the collection can play,
for as long as it is switched on. It appears only when it would change something,
it says on the page and on the printed sheet that the chords are not the file's,
and it never touches the file.

Each chord is tried in order, most faithful first: the chord itself, then without
its slash bass, then each simpler family with and without the bass. The first
form with a real shape wins, so `Cmaj7#11` becomes `Cmaj7` rather than a bare
`C` — enough to be playable, and still the chord that was written. The bass goes
before the quality because on a guitar sheet an inversion is usually somebody
else's note, while the quality is what the chord is. A chord that reaches the end
of that list without finding a shape is left exactly as written.

## Symmetry

A diminished seventh sounds the same set of notes every three frets, and an
augmented triad every four. Templates step by the family's own period rather than
by twelve, so `Cdim7` is offered at the first fret and not only at the tenth.

## Tunings

Standard tuning is the dataset's tuning. A song in another tuning gets the
standard shapes that remain valid — a shape survives only if every string it
*sounds* has the same open pitch in both tunings.

That makes Drop D nearly free: it differs on the lowest string only, so every
shape that mutes or omits that string is correct as-is, which is most of the
useful ones. A tuning where nothing survives, such as DADGAD, gets an honest note
and no diagrams rather than shapes that are quietly wrong.

`song.Tuning` stays free text, as the format promises, so `Drop D (ish)` is not a
parse error — it simply is not a tuning we have shapes for.

## Capo

Diagrams count frets from the nut, and the sheet says so when a capo is set.
`voicing.Options.Capo` exists so that a capo-relative mode is later a flag rather
than a rewrite.

## Deliberately not done

- Chord shapes in the `.song` format. A song file describes music, not
  fingerings, and per-file chord definitions would mean every reader needing this
  code to render a sheet.
- Generated fingerings, for the reason at the top.
- Left-handed diagrams, alternative instruments, and audio. All reasonable; none
  needed yet.
