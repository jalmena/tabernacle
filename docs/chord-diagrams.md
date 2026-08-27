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

Shapes are sorted by a playability cost: what it takes to lay a hand on the neck
and hold the shape, plus two preferences about what a chord reference prints
first. How that is worked out is *How hard a shape is* below; what matters here
is that the model is approximate and three properties are not:

1. A shape somebody wrote down for this chord comes first, whatever the model
   makes of it. The model is a model of a hand; a curated entry is a decision.
   Sorting on the model alone let a tuning change knock Bb off the first-fret A
   shape everybody plays and onto a barre at the sixth.
2. An open shape always beats a barre.
3. The order is deterministic, because the interface says "2 de 4" and a deep
   link names a shape.

`TestPrimaryShapes` pins the shape offered first for about forty chords against
what a chord reference prints. A change there means the cost function moved,
which may be right but should never be accidental.

### How hard a shape is

Two numbers, not one, and keeping them apart is the whole design.

**Strain** is what the shape asks of the hand. **Cost** is strain plus two
editorial preferences, added in `cost` and nowhere else, and it is the sort key
for a list. `Level` reads the strain, so the word a reader sees is about the
shape and not about where it happens to sit in a list.

#### Strain

Strain is not a sum of penalties. It is the cost of the easiest way a hand can
actually be laid on the neck to hold the shape, which is a different kind of
question and the reason this model replaced the one before it.

**The neck is not evenly spaced**, and that one fact carries most of the model.
Frets fall where the string divides, so they crowd together as they climb: the
first is 36.4 mm wide, the sixth 27.2, the twelfth 19.3. Knuckles do not move —
they sit the same distance apart whatever fret the hand is over, about 17 mm for
the hand these numbers describe. The two meet around the fifteenth fret. Below
it, one finger per fret is a stretch, and the further down the neck the bigger
one; above it, it is the shape the hand is already in. Everything a guitarist
says about the nut being harder starts there, and none of it has to be written
down as a rule.

**The hand is a line of knuckles and four fingers.** Three numbers place it: where
the palm sits, and how far the knuckle line is turned from the neck. Turned along
the neck, the fingers land one to a fret. Turned across the strings, they land in
a row on one fret. Those are the two postures a player rests in, and they are not
two cases — they are one angle with two values, and real chords sit all along it.

The turn is not free, and it has to not be. A hand holding two notes has three
ways to move and only two of them spoken for, so a free angle could always be
turned until any two notes fitted perfectly and one fret apart came out the same
as four. So the wrist has a resting turn and leaving it costs; and how far apart
a pair sits **measured along the knuckle line** is charged separately from
everything else, because it is the one thing moving the palm cannot change.

Each finger runs from its own knuckle to its note. Reaching sideways lengthens it,
exactly as a finger does. What a shape costs is what it asks of each one:

| | |
| --- | --- |
| Reaching | past what the finger reaches, or pulled up short of it |
| Fanning | pointing off straight ahead, free for the first stretch and then charged |
| Spacing | a pair sitting wider apart along the knuckle line than the knuckles do |
| Tendons | two fingers pulled to different lengths, weighted by which pair |
| The finger itself | using the ring or the little finger at all |
| Pressing | holding each note down, and holding a bar flat across several |
| Wrist | turning away from where the forearm rests |

Every one of them is asymmetric, because a hand is. Curling up costs less than
reaching out, and closing a pair costs less than opening one — closing is what
curling does anyway.

**A finger points away from its knuckle**, over the strings, and not back into
the palm. Obvious enough to be left out, and leaving it out was expensive: with
the fan measured as an angle off the knuckle line, a finger aimed a hundred and
eighty degrees the wrong way reads as perfectly straight. A first fret and a
fifth on the string beside it came out easy, held with an index pointing at the
nut and a middle finger pointing at the body.

**Fingers in the order they grow in.** Projected onto the knuckle line, the
fingertips must run index, middle, ring, little. A hand cannot put its middle
finger on the far side of its ring finger — that is not expensive, it is not a
thing. This one rule does the work of four the old model needed a term for each
of: crossing over, filling order, a finger trapped between two others, and a hand
asked to face two ways at once. And because the projection is along the knuckle
line, it reads as one finger per fret when the hand is along the neck and as the
index on the graver string when it is across.

**Fingers are not equally independent**, and that is a tendon rather than a
weighting. The stiffnesses between index and middle, middle and ring, ring and
little are one, one and a half, and three — the numbers a guitarist quotes.
Fingers further apart in the hand get the links in series, so a pair with an idle
finger between them comes out slacker than either link on its own, which is the
slack a player feels when the ring finger has nothing to do. That falls out of
the arithmetic; nobody wrote it.

**What a finger will not do at any price.** Past a certain length, curl or fan,
there is no placement, and the answer stops being a number. That is the whole of
what inhuman means here: not a large score, a refusal.

**A shape with no fingering at all** — one asking for more fingers than a hand has
— starts above the last band and is then charged 40 for each finger it is short,
so being two short ranks past being one short.

**Where the numbers come from**, and the tally is meant to be checkable:

| | | |
| --- | --- | --- |
| `scale`, `nutSpread`, `twelfthSpread` | measured | a common steel-string, and a ruler settles them |
| `stiff` | quoted | 1 : 1½ : 3, said by the player in those words |
| `knuckleGap`, `reach` | fitted inside what a hand allows | see below |
| every cost, and the seven bands | fitted | see below |
| `weak` | fitted, and not geometry | the little finger is weak, which no length says |
| `nutCost` | fitted, and standing in for something | see below |

It reproduces thirty-six of the forty judgements. The six it does not are
listed as owed in the corpus, and the test complains both when a new failure
appears and when a listed one starts holding, so the list can only shrink and
cannot be forgotten. The model it replaced managed twenty-six, and broke the
invariant that fretting one more string cannot make a shape easier four times
over; this one does not break it at all.

The three still owed are one disagreement each. Two are the little finger
reaching back over the index for a note between it and the middle finger, which
the player calls a band harder than the model does. The third is Fmaj7: the
model has it level with an open E and the player calls it easier than anything
else with three fingers on it.

Fitted means fitted **once**, by `TestFitTheHandToWhatThePlayerSaid` in
`calibrate_test.go`, which walks every constant against the whole of
`internal/voicing/testdata/judgements.json` at a time and keeps what satisfies
most of it. That file is what a guitarist said about shapes on a neck, written
down before the model that has to reproduce it. The test is skipped unless
`FIT=1` because it takes an hour, and it lives in the tree so the constants can
be re-derived rather than believed.

Three of them are honest about being something else.

`knuckleGap` and `reach` look like ruler readings and are not. What the model
needs is where the fingers pivot and how far each one reaches **in the posture a
hand takes on a neck**, and that is not the same as a finger's length: the index
lies flatter, being the one that goes across for a barre, and the middle is more
arched, so the order a ruler gives is not the order the neck sees. Fitting them
was worth ten of the forty judgements — but only inside bounds a hand allows,
and the little finger's range stops below the others' so no amount of fitting can
hand it the longest reach. Let loose entirely, the fit produced a hand with a
middle finger shorter than its index, which is not a hand: it is the fitting
covering for something the model has not got.

`weak` is not a length at all: the little finger is weak and the ring finger is
not much better, and without saying so the model handed three-note diagonals to
the index, ring and little fingers — which lies flatter on a neck whose frets are
wider than the knuckles are, and which nobody plays.

And `nutCost` stands in for everything about the end of the neck that is not
length.

Why `nutCost` has to exist at all is worth writing down. Near the nut more string
is left ringing, so it swings wider for the same pluck and takes a firmer stop
before it rattles on the fret. That is real, it is geometry, and it is about a
twentieth between the first fret and the second — while a player puts the first
fret at half again. The rest is the nut being a hard stop the string cannot give
against and the wrist having to come further round to reach it, neither of which
this model has. So it is one fitted number with a name, rather than a derivation
that would be a fiction.

**Why not a sum of weights.** The model this replaced had twelve terms, each one
added to save a shape the eleven before it got wrong. It could not be fixed,
because everything that kept breaking it — crossings, trapped fingers, filling
order — is a property of the whole hand and no sum over pairs can see one. Its
last week produced a shape that scored *less* than the notes inside it, an
unholdable position rated moderate, and the same two notes costing twelve or
nothing depending on which finger got them.

#### The words

`voicing.Voicing.Level()` turns the strain into one of seven words. The bands
are anchored to chords a guitarist already has an opinion about rather than to
quantiles of everything the generator can produce, because most of what it can
produce is nothing anybody plays:

| | strain | |
| --- | --- | --- |
| Trivial | ≤ 5 | Em7 2, Em 4, E7 5, A 5 |
| Easy | ≤ 16 | D 6, Am 7, Fmaj7 8, E 8, Dm 8, G 8, C 9 |
| Moderate | ≤ 35 | Abm7 20, Gm7 20, Bm7 20, C#m 21, Bm 23, F# 25, B 26, F 27, Bb 30 |
| Advanced | ≤ 43 | |
| Complex | ≤ 51 | |
| Extreme | ≤ 59 | |
| Inhuman | above | |

Every barre in one band, near the nut or away from it, which is where a
guitarist puts them and which the model before this one could not manage —
it had them spread across three.

The first four edges are fitted against the chords the corpus names, with a
floor under how narrow a band may be: let loose, the fitting squeezed each one
down to a point and had every shape come out either trivial or inhuman, which
satisfies the arithmetic and describes nothing. The top two are not fitted at
all, because no chord anybody plays anchors them — they follow the width of the
last band that is anchored, which is what keeps them within reach. Left free,
they went where nothing pulled them and stretched one band from forty to four
hundred.

The number is shown beside the word, small: a word alone cannot say that one
shape is at the top of its band and the next barely scraped in, and the
difference between two shapes in the same band is exactly what somebody
comparing them is looking at.

#### The two preferences that are not difficulty

- **Position**, three points a fret. A chord reference prints the shape nearest
  the nut first. Frets narrow as they climb, so the same barre is easier at the
  ninth fret than at the first, not harder — levelling by cost called one shape
  Moderate and the identical shape nine frets up Advanced.
- **Silent strings**, six points each. A fuller voicing is the better thing to
  print, being the bigger sound and the shape people expect to be shown. It is
  not the harder one: a three-string grip is usually the easiest thing on the
  page.

Which is why the list does not say "easiest first". It never was: a curated
shape leads it, then the fuller and lower ones, and a three-string grip that is
the easiest thing on the page can sit near the end. It says the usual ones come
first, which is true.

### The shape the collection prints comes first

On the chord page the generated shapes are ordered easiest first, but the shape
a lookup would return leads them — the curated one, or when nothing is curated,
the first shape the dataset produces. Without that the page led with whatever
the generator found cheapest, which for B was a four-string grip with the top
string muted, and the same chord then carried a different diagram on a song
sheet. Only the first, and only when nothing is curated: adopting every slid
template put a barre at the eighth fret above the open C.

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
search throws those away rather than ranking them low. No hand-written shape
does it either, which is the check that the rule is a real
one and not a preference. It is also what cuts the search's output roughly in
half: Am7 goes from 370 grips to 168, and every one that is left is something a
hand can simply hold.

### Fingering

Whether to lay a barre is decided by trying it: both fingerings are worked out
and the cheaper is kept, which is what a hand does.

The rule it replaced counted fretted strings and laid a barre only past four,
on the grounds that a barre is how a hand reaches six strings with four fingers.
That is one reason to lay one. The other is to free a finger needed somewhere
else, and counting cannot see it. On `1 5 4 1 0 0` — two strings at the first
fret, and the fourth and fifth frets above them — the count said four, refused
the barre, and spent the middle finger on the first fret while the ring and
little fingers were three and four frets away. It is a fingering no player would
use, and it scored the shape harder than the same shape with a sixth note added,
because the extra note pushed the count past four and let it barre.

E major and A major still come out with three separate fingers, F and Bm with
the full barre, and F/C with the small one across the top two strings — not
because they are named anywhere, but because that is what wins.

Which fingers are used is chosen, not taken in turn from the index. Every choice
of which of the four to use is tried and the one the hand settles on most easily
is kept — never more than a handful, six ways for two fingers and four for three.
Fingers still go on in order of fret and then of string, because a hand cannot
cross them over; what is chosen is which of them are on the neck at all.

Taken in turn from the index, two notes two frets apart came out as the index and
the middle finger, which no hand does. Nothing anywhere says which finger ought to
go where. The knuckles are 21 mm apart and the frets are wider than that near the
nut, so skipping a finger to skip a fret is simply what the hand settles on, and
what a player calls one finger per fret is that settling written down.

Deterministic and without the thumb.

A barre is laid only when four fingers are not enough, which is the only reason
anyone lays one. Asking the question that way needs no special cases: E major and
A major keep their three separate fingers, F and Bm get the full barre, and F/C
gets the small one across the top two strings. The barre spans from the first
sounding string at the lowest fret to the last, and is refused if an open string
falls inside it, because a finger lying across cannot let one ring.

Where a generated fret vector matches a dataset shape, the dataset's shape is
used instead, with its fingering and its CAGED name.

`TestGenerationCoversTheCuratedDataset` requires the generator to rediscover
every hand-typed shape on its own, and asserts the list of exceptions is
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

### The hand on the neck

The board shows the hand the difficulty was worked out with, translucent, over
the shape: a line for the knuckles behind the sixth string and one for each
finger reaching from its knuckle to the note it holds. The finger asked for more
than it reaches is drawn in the warning colour, because that is the one doing the
work and saying which one is most of what the picture is for.

It is drawn foreshortened. A palm sits the length of a finger off the fretboard —
eight or nine string widths, wider than the neck itself — so to scale the
knuckles would be off the side of the page and a reader would see four lines
entering from the left. The knuckles are brought to a line just behind the sixth
string, which keeps the thing worth looking at: how far apart they sit **along**
the neck, which is the whole of how the hand is turned. Bunched at one fret is a
hand lying across the strings; spread down three is a hand lying along the neck
with a finger to each. Where every tip lands is exactly what the model used; the
angle of each finger is not, and cannot survive the squashing.

The overlay is an SVG laid over the grid with a viewBox counting strings across
and frets down, so it stretches with the board and no pixel is measured anywhere.
It takes no clicks — the cells underneath are radio buttons and reaching them is
the point of the board — and it is hidden from a screen reader, which is already
told which finger goes where by every cell's own label. It does not print.

Why show it at all: every mistake found in the model so far was found by somebody
looking at a shape and saying *that is not how you hold it*. A model nobody can
see is a model nobody can argue with.

### The fingers on the neck

Each dot on the editable neck carries the number of the finger that holds it,
and says the same in words for a reader who cannot see it — "third fret, ring
finger". It is the fingering the collection would use, worked out in Go by the
same rules as everything else: a neck that shows dots and says nothing about how
to hold them is a puzzle rather than a lesson.

The number is drawn in the paper colour so it reads against the dot in either
theme, dark on light and light on dark.

### Hearing it

The neck has a button that plays what is drawn on it.

The notes travel from the server on the button as MIDI numbers, because which
pitch a fret on a string makes is music, and no music is decided in `app.js`.
`voicing.Voicing.Pitches()` already produced them for the dataset validator;
the board carries them in `data-sound` and the browser only turns numbers into
air. Editing the neck re-renders the button, so what it plays is always what is
drawn.

The sound is Karplus-Strong: a ring buffer one wavelength long is filled with
noise, then read round and round with each sample averaged against the next.
The averaging is a low-pass filter, so the high partials die before the low
ones and a plucked string comes out rather than a beep — which matters, because
the point is to recognise the chord you drew.

Three details are not obvious:

- The ring is read once per **period**, not once per sample, so a fixed loop
  gain would silence a low string in a breath and let a high one ring for a
  minute. The gain is derived from the frequency so the loss per second is the
  same at any pitch.
- The loop filter throws away energy of its own, and that is what mostly decides
  how long a note lasts.
- The last 80 ms are faded. Whatever is left when the buffer runs out would end
  on a step, and a step is a click.

Strings start 35 ms apart, low to high, so it is a strum and not a piano chord.

### Two guitars and an amplifier

A string on its own still sounds like a synthesiser, because half of a guitar is
the box. What follows the string is a handful of resonances — the air inside the
body, the top plate — and they are most of what makes one guitar recognisably
not another.

| | pluck | body | top | ring |
| --- | --- | --- | --- | --- |
| Nylon | 900 Hz | 105, 215, 400 Hz | 2400 Hz | 2.0 s |
| Steel | 4200 Hz | 100, 250, 2600 Hz | 6500 Hz | 2.4 s |

The **pluck** is a one-pole filter over the initial burst, and it carries most
of the difference between a nylon string and a steel one. The loop filter alone
does not: over the handful of turns that matter it treats the two almost the
same, and the measured brightness came out within seven per cent before the
burst was shaped. With it, nylon reads about half as bright as steel.

The **body** is three peaking filters. A classical guitar's air resonance sits
low and its top plate just above, and both are strong — that is the warmth. A
steel-string puts the same two a little higher and adds a presence peak, which
is why a strummed dreadnought cuts through a room.

The **electric** is the steel guitar with an amplifier after it.

Two things that are not the string matter as much as it does. A ring of whole
samples can only play the notes whose wavelength happens to be a whole number of
them, and everything else came out as much as five cents off — across six
strings, a chord beating against itself; an allpass in the loop supplies the
missing fraction. And a pluck is not only a string: the nail or the pick
crossing it arrives before the note, and a few milliseconds of noise on top is a
large part of hearing a guitar rather than a tone.

The **electric** earns its sustain by barely damping itself, and then goes
through a small amplifier: the deep bass is rolled off first or six strings turn
to mud, the signal is driven hard into a `tanh` soft clip — distortion is not
the strings being loud, it is the amplifier refusing to go any louder and
folding the tops over — and the fizz is taken off the top the way a speaker
cabinet does.

Pressing a guitar plays the chord on it, which is the only way to compare two of
them without hunting for the other button. The row sits **outside** `#chordlab`
on purpose: editing the neck replaces everything inside that div, and a choice
that reset itself every time a string moved would be worse than not offering
one. Out there it needs no storing and no restoring.

Sound needs a script and a sound card. The button is marked `.needsjs` and is
hidden until `app.js` has run; if the browser has no `AudioContext`, the block
removes the button rather than leaving one that does nothing.

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

The chords written in a `.song` file are the shapes the left hand holds. `capo:
2` says the player clamps the second fret, so the sheet sounds two semitones
above what it reads, and transposing a sheet leaves the capo exactly where it
is: it describes what the player does with a piece of metal, not what the sheet
says.

### Where the capo could go

A capo moves the nut. What you hear is what you finger plus where the capo sits,
so with the sound held still, moving the capo from A to B means fingering every
chord `B − A` semitones lower. `voicing.CapoOptions` is that sentence applied to
each fret from none to the seventh, and scored.

Each fret is scored by the primary shape of every distinct chord it produces:

| Shape | Cost |
| --- | --- |
| none in the collection | 100 |
| needs a barre | 10 |
| closed, no barre | 3 |
| open position | 0 |

Plus one point per fret, so a capo has to earn its place and only ever wins a
near tie by being lower on the neck. A chord repeated nine times counts once: a
song with nine G's and one F is an easy song with one hard chord in it.

Seven is as far up as an option goes. Past that there is not much guitar left
above the capo and the tone thins towards a mandolin.

The numbers land where a teacher lands. A song in F — F, B♭, C7, Dm — comes out
capo 1 playing in E, capo 3 playing in D, capo 5 playing in C, in that order,
with one minor chord barred in all three. Removing every barre is not the
promise; being easier than the sheet you were handed is.

### Staying quiet

The rule for showing nothing matters more than the advice. Most songs are
already open chords, and a panel that appears on all of them teaches the reader
to stop looking at that corner of the page. `render.capoAdvice` offers a fret
only when it removes work — a barre, or a chord we hold no shape for — and never
merely because it scores a point lower. G, C, D, Em gets no panel.

### Advice, not a change

The panel says where the capo could go and what to hold there. It does not
rewrite the sheet, and the sound never moves. Applying it would mean a sheet
that displays one key while sounding another, and a key label honest about both
— worth doing, and a separate piece of work from telling the reader the fret.

## Deliberately not done

- Chord shapes in the `.song` format. A song file describes music, not
  fingerings, and per-file chord definitions would mean every reader needing this
  code to render a sheet.
- Generated fingerings on a song sheet. Generation belongs to the chord page,
  where a reader is asking for choices; a sheet wants one shape and it should be
  the one a person chose.
- Left-handed diagrams, alternative instruments, and audio. All reasonable; none
  needed yet.
