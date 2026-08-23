# Chord diagrams

Diagrams are generated from structured data. There are no images of chords in
this repository and there will not be.

## The three packages

| Package | Question it answers |
| --- | --- |
| `internal/harmony` | What kind of chord is this? |
| `internal/voicing` | Which shapes play it, and what is this shape? |
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

A song sheet is drawn from this dataset and never from the generator. A sheet
shows one shape per chord and it should be the shape people actually play, which
is a question about custom rather than about arithmetic — `x 3 2 0 1 0` is C
because that is how people play C, not because it falls out of a rule. The chord
page is the other way round and is described under
[Generating shapes](#generating-shapes).

There are two kinds of entry.

**Curated** shapes are written out in full, with absolute fret numbers. This is
how open-position fingerings get in, because those are idiomatic rather than
derived.

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

## Naming a shape

A caption says where on the neck a shape sits, which is what a player wants to
know before looking at the dots. There are four:

| Caption | When |
| --- | --- |
| `Cuerdas al aire` | nothing is fretted |
| `Cejilla en el traste N` | a finger lies across several strings, at fret N |
| `Posición abierta` | some string rings open and the hand stays inside the first four frets |
| `Traste N` | anything else, named by the fret the hand sits at |

Two of those need saying out loud.

**Open position is about open strings, not about the first fret.** `G` is
`3 2 0 0 0 3` and owns no first-fret note; it is as open a chord as `C`. So the
test is whether a string rings open with the hand near the nut, not where the
lowest dot happens to land. A shape with no open strings is closed wherever it
sits, including at the first fret, and a barre chord is never open — `F` sits at
the first fret and no chord book calls it an open chord.

**The fret a caption names is not always the fret a picture starts at.** An
A-shape barre is `x 3 5 5 5 5`: index at the third fret, barre at the fifth. A
player calls that a barre at the fifth, and the picture still has to begin at the
third or it loses a note. So the window opens at the lowest fretted note and the
caption names the barre.

The caption and the window come out of one function, `windowFor`, for exactly that
reason: computing them separately is how a picture drawn from the nut ends up
captioned "Traste 2". `TestCaptionAndWindowAgree` walks every shape the
collection holds and checks that any fret the words name is a fret the picture
contains, reading the caption's own text rather than recomputing the rule.

Five frets are drawn. A shape spans at most four, which `internal/voicing`
enforces, so five always holds the whole shape and leaves the grid room to
breathe. How many frets a picture shows is a matter of taste; what counts as open
position is not, and the two numbers are deliberately separate.

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

## Generating shapes

The chord page answers a different question from a song sheet. A sheet asks "how
do I play this chord", once; the page asks "show me every way", and the dataset
has one shape per chord and no template at all for seven of the twenty families.
So the page generates, on demand, per request, with nothing precomputed.

### What counts as playing the chord

One rule, in `Requirement.Satisfies`, used by the generator, the identifier and
the dataset's own validator, so the three cannot drift apart.

- No note sounds that the chord does not contain.
- Every note of the chord sounds, **except a perfect fifth when the chord has
  four notes or more**. That one sentence gives the right answer for all twenty
  families without a list of exceptions: the third and the seventh never drop, a
  seventh chord may lose its fifth, and an *altered* fifth never does, because a
  diminished fifth is six semitones and an augmented one is eight, so the clause
  never reaches them. The four-note clause is also what stops `{C, E}` being
  called C.
- The root sounds. A rootless voicing is real practice, but it cannot honestly be
  captioned with the chord's name on a page for learning chords.
- A ninth sounds *above* the root. A D below the E is a sus2 colour, not an added
  ninth.
- A slash chord's named bass is the lowest note.

### The search

Per window of four frets from the first to the twelfth, each string may be
muted, open if its note belongs, or fretted at one of the four frets whose note
belongs. That is a few thousand combinations per chord and a few milliseconds.

Four frets and not the five a diagram draws: a shape spans at most four, so
walking windows of four is already exhaustive and a wider one would only
rediscover the same shapes.

**Nothing silent in the middle.** A muted string between two ringing ones has to
be damped deliberately, with a fingertip leaning off a note already being held.
It is a real technique and not a shape to hand somebody learning a chord, so the
search throws those away rather than ranking them low. None of the fifty
hand-written shapes does it either, which is the check that the rule is a real
one and not a preference. It is also what cuts the search's output roughly in
half: Am7 goes from 370 grips to 168, and every one that is left is something a
hand can simply hold.

### Fingering

Deterministic and without the thumb. Fingers go on in order of fret and then of
string, which is what a player does — it is why the ordinary D and Dm come out
with the fingering they are taught with.

A barre is laid only when four fingers are not enough, which is the only reason
anyone lays one. Asking the question that way needs no special cases: E major and
A major keep their three separate fingers, F and Bm get the full barre, and F/C
gets the small one across the top two strings. The barre spans from the first
sounding string at the lowest fret to the last, and is refused if an open string
falls inside it, because a finger lying across cannot let one ring.

Where a generated fret vector matches a dataset shape, the dataset's shape is
used instead, with its fingering and its CAGED name.

`TestGenerationCoversTheCuratedDataset` requires the generator to rediscover all
fifty hand-typed shapes on its own, and asserts the list of exceptions is
**exact** so it cannot grow in silence. It is currently empty.

### The order shapes are shown in

Not the order a song sheet uses, and not divided up. Somebody learning a chord
wants the one they can play today and then the next one, which is an order rather
than a set of categories.

1. **A shape written down for this exact chord**, easiest of those first. A
   curated entry names a chord and a grip together — somebody decided that this
   is how this chord is held — and there is nothing to weigh that against.
2. **Everything else**: fullest, then easiest.

The distinction in the first rule is between the dataset's two kinds of entry,
and it matters. A movable template slid until its root lands on the right note is
arithmetic — useful, often the only answer high up the neck, but no reason to put
a barre at the eighth fret ahead of an open shape a beginner could hold today.
`Csus4` has no curated entry, and treating its slid `sus4` template as authority
led with a barre when everyone plays `x 3 3 0 1 1`.

Fullness comes before cost, and stops counting once the chord is whole. Cost
measures how hard a grip is to hold, and by that measure the thinnest shape
always wins: for `Csus4` it puts two notes on two strings ahead of the voicing
everyone is taught. Whole means every note plus one doubling, which depends on
the chord — a triad is whole on four strings, a power chord on three. Counting
further would rank a six-string barre above an open shape; counting less would
rank two notes above the whole chord.

The list is trimmed to twelve, with the real total printed beside it, and asking
for more raises the limit rather than removing it.

Inversions are never mixed in. A diagram captioned `C` whose lowest note is E is
a lie by omission, and the chord already has a name for that, so `C/E` is offered
as its own page.

## Identifying a shape

`voicing.Identify` names what a reader has drawn: twelve roots against twenty
families, accepted by the same `Satisfies`.

**The answer is a list, not a name.** The ambiguity is real, and on a page for
learning chords it is the interesting part rather than noise to hide:

| Notes | Readings |
| --- | --- |
| C E G A | **C6** if C is lowest, **Am7** if A is |
| C E♭ G♭ A | Cdim7, E♭dim7, G♭dim7, Adim7 — all four exact |
| C F G | Csus4 / Fsus2 |
| C E A | Am/C — three notes, and C is not its root |

Ties are broken by fewest notes omitted, then root position before inversion,
then an explicit order of preference between families. Explicit, rather than the
order the `Quality` constants happen to be declared in: that order belongs to
another package and could be rearranged by somebody with no idea it was deciding
what a fingering is called. A test fails if a family is missing from it, so
adding a twenty-first is a decision rather than an accident.

If nothing fits, the page says which notes are sounding and that they are not a
chord we can name. It never invents one.

### Near misses

`voicing.Suggest` names the chords a shape *nearly* plays. Somebody learning does
not reliably hear the difference between a string they have stopped and one they
have left open, so a D drawn on an untouched neck comes out as three right notes
over two wrong ones and gets no name at all. This turns that into "D/A, without
the sixth" and "D, without the sixth and the fifth" — which is both the answer
and the lesson. Following one silences those strings as well as naming the chord,
because seeing the neck turn into the real shape is the part that teaches.

Three rules keep it a hint rather than a guessing game:

- **Only open strings may be stopped.** A fretted note was put there on purpose
  and is not a mistake to correct.
- **At most two**, because two is the number a beginner leaves ringing by
  accident: the pair of thick strings above a D or a Dm. Three would start naming
  chords with little to do with what is on the neck.
- **It has to be an improvement.** Stopping a string that belongs to the chord
  only thins it out — the plain `x x 0 2 3 2` would otherwise be told it could be
  D/A. So a suggestion either takes away a note the chord does not have, or it
  puts the root underneath where it was not.

Nothing is suggested for a shape that already plays a chord exactly, or for an
untouched neck: six open strings are not a near miss, they are a starting point.

## The editable neck

The neck on the chord page is a form of radio buttons, one group per string, and
not a drawing. The browser already knows how to move between radios with the
arrow keys, label them for a screen reader and submit them without any script;
thirty-six clickable SVG circles would be worse at every one of those and useless
with JavaScript off.

It is held upright, strings running down the page with the sixth on the left,
narrower than it is tall, which is how a chord chart is drawn and therefore how
every diagram beside it is drawn — a shape can be copied from a picture onto the
board fret for fret. The window's first fret is named beside its own bar, which
is where a chord chart puts it and is why the other four need no number. The
outside columns draw half a fret line each, so the grid ends on the sixth and the
first string rather than half a column past them.

The modifier classes on those cells are deliberately specific. The stylesheet is
one namespace, and `.top` already belongs to the page header: a cell marked with
that name quietly inherits a sticky position, padding and a bottom border, which
is a stray line across the fretboard that no test can see.

**What the two halves do to each other.** Naming a chord puts its easiest shape on
the neck, because a reader who asks for F wants to see F and not an empty grid
beside a list of pictures of it. Drawing on the neck names what the notes are. An
empty page starts with the six strings ringing open, which is what a guitar does
when nobody is holding it — not silent, which is what a row of crosses would say.

**Taking a finger off.** Pressing the dot that is already there leaves the string
open. Only the × silences it: a string you have stopped holding is not a string
you have muted.

**The script does not navigate.** It fetches the same region the server renders
and swaps it in, so trying chords is a continuous thing rather than a series of
pages, and it leaves the address bar alone — the URL a reader arrived on is still
the one they can share, and it does not turn into a log of every dot they tried.
Every control underneath is still a link or a GET form that works on its own, and
a test asserts the fragment and the page produce identical markup.

Three tests guard the join, because the join is what breaks and nothing else
looks at it. `TestTheScriptLooksForThingsThatExist` reads the selectors out of
`app.js` and checks each one against the rendered pages.
`TestTheClassesTheScriptAddsAreStyled` does the reverse for the classes it puts
on things — a class nothing styles leaves the controls it was meant to hide
sitting on the page. `TestEveryLinkOnTheChordPageIsHandled` requires every link
inside the editor to carry the mark that says the script takes it over: a link
that navigates reloads the page, throws away what was drawn, and writes the
reader's fumbling into the address bar. Each of those failures looks like the
page working slightly worse rather than like a bug. What breaks in a progressive
enhancement is the join between the markup and the script, and nothing else looks
at it: splitting the neck into one radio group per string left the script
listening for `input[name="s"]`, and renaming the region it swaps left it looking
for `#chordresult`. Both silently did nothing while every other test passed.

The result diagrams stay SVG. The editor is a control and the variants are
pictures; they answer different questions.

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
- Generated fingerings on a song sheet. Generation belongs to the chord page,
  where a reader is asking for choices; a sheet wants one shape and it should be
  the one a person chose.
- Left-handed diagrams, alternative instruments, and audio. All reasonable; none
  needed yet.
