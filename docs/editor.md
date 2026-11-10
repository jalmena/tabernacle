# The editor

A song is written one of two ways, and saved one way. Whichever is on screen,
what is saved is a `.song` file, checked by the same reader as any other — see
[the format](song-format.md).

- **By fields.** The header is a form and each section is a box of its own, with
  a name. Sections are added, removed, moved, joined and split with buttons.
- **The file.** The whole text in one box, for anything the fields do not reach
  and for pasting a song in. **Editar el archivo** and **Editar por campos**
  turn one into the other without saving.

## Chords above the words

In the fields, a section shows its words as plain text and every chord floating
over the syllable it falls on, the way the sheet shows them. Nothing is typed in
brackets.

| To | |
| --- | --- |
| Put a chord in | Press above its syllable, where a `+` shows where it would go, or type `[` in the words |
| Move it | Drag it along the line, or to another line of the same box |
| Change it | Press it |
| Copy it | Drag it with `Ctrl` or `Alt` held |
| Take it out | `Delete` with the chord chosen, or the button in the chooser |

Pressing a chord opens a chooser on it: a box for its name, the chords the song
already has, the twelve notes, the usual kinds of chord, and the note in the
bass. A note moves the whole chord, so `C/E` on D is `D/F#`; a kind or a bass
changes only that. `Enter` keeps what the box says and `Esc` leaves the chord as
it was.

Typing the name is always enough, and it is read generously, because where a
chord goes nothing else could be meant: `am` is `Am`, `lam` and `LAm` are `Am`,
`sol7` is `G7`. What is still not a chord is an annotation, `x2` or `let ring`,
and floats among the chords in grey.

Note names are letters here whatever the reader has chosen for sheets: the
editor shows what the file holds.

### What the chords do when the words change

A chord belongs to the letter it stands over, and goes where that letter goes.

- Typing in front of a chord pushes it along with its syllable.
- Rubbing out the word under a chord keeps the chord. Type another word and it
  is over that one.
- Deleting whole lines takes their chords with them.
- Breaking a line takes the chords after the break down to the new line.
- A chord put in with `[` waits for what is typed next, which is the syllable it
  is for: `[` `Am` `Enter` and then the word, as in a file.
- Words copied or cut and pasted inside the editor bring their chords. Anything
  else they are pasted into gets the words alone. Dragging selected words about
  inside a box is turned off: cut and paste moves them, chords and all.
- `Ctrl Z` and `Ctrl Y` undo and redo words and chords alike, one box at a time.

Two chords closer together than their names are wide cannot both stand over
their syllable. The second moves along and carries a small arrowhead pointing
back; the line under a chord that is pointed at or chosen shows the letter it
really belongs to.

### With a finger

- The chooser is a sheet at the foot of the screen, and the page moves up if
  the chord would be behind it. A tap anywhere else closes it and does nothing
  more: it puts no chord in and presses no button that happened to be there.
- A finger that starts on a chord and goes sideways moves the chord. One that
  goes up or down is scrolling the page. Resting on the chord first picks it
  up, and then it can be carried to another line.

### Tablature

A fenced block is a fixed-width box of its own, without its fences. They are
written when it is saved, as long as what the block holds needs. Emptying the
box removes the block.

## The keys

| | |
| --- | --- |
| `Ctrl` `↵` | Save |
| `[` | A chord where the caret is |
| `Tab` | From the words to their chords |
| `←` `→` | On a chord: to the one beside it |
| `Alt` `←` `→` | On a chord: move it a letter; with `Shift`, a word. In the words: the same, for the chord standing at the caret |
| `Alt` `↑` `↓` | On a chord: move it to the line above or below |
| `Delete` | On a chord: take it out |
| A letter | On a chord: start its new name |
| `Esc` | On a chord: back to the words |
| `Ctrl` `Z`, `Ctrl` `Y` | Undo, redo |
| `Alt` `↵` | Split the section at the caret |
| `Alt` `X`, `Alt` `M` | Tick the section; join the ticked ones, or this and the next |
| `Alt` `↑` `↓` | In the words: move the section |
| `Alt` `N` | Add a section |

## Leaving

Closing the tab, going back or following a link asks first when the page holds
something that is in no file yet, with the browser's own question: a field typed
in, a chord moved, a section added and not saved. **Guardar**, **Cancelar** and
the editor's own buttons are not leaving, and do not ask.

## Without a script

Every section is a box of text with its chords in brackets, and every button is
a form that goes to the server and comes back with the change made. Nothing in
the editor is out of reach.

That box is also what a section stays as when it cannot be shown any other way:
a bracket left open, a fence with no end. The mistake is in the text, and the
text is where it is mended.

Nothing asks before the page is left, since only a script can.

## How it is saved

The browser never writes a bracket. The page carries each section twice: as the
text it is, and taken apart into its words and the place in them each bracket
stood. Moving a chord changes a number.

- A section nobody touched goes back as its text, and is saved byte for byte.
- A section that was changed goes back as words and places, and the server
  writes each line the way it writes every line of a song file: the escapes,
  what counts as a chord, how long a fence has to be.

So there is one set of rules for the format, in the one place that tests them.
What a typed name means is asked of the server for the same reason. A song saved
before the answer has come loses nothing: the name goes marked as unread, and is
read as the line is written.

## Left out

- Dragging a chord from one section to another.
- Pasting a sheet that has its chords on the line above the words, or text with
  chords in brackets: pasted into the words, brackets are words. Both are read
  in the file view, or when a file is loaded — see
  [the format](song-format.md).
- Words in a section that is only a fenced block. There is no box for them
  until the file view has put a line there.
- Choosing how a chord is held. A song file says which chords are played, not
  how.
