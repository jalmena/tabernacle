# Printing

A printed song sheet is not a screenshot of a web page. It has to survive being
put in a ring binder, opened on a music stand and read from a metre away, and it
has to do that on whatever printer is in the house.

## What the print stylesheet does

- A4 portrait, with margins a consumer printer can actually reach.
- Navigation, buttons, the transposition controls and the chord popover are all
  removed. Only the sheet is left.
- Black on white, whatever theme the screen was using.
- A stanza carries `break-inside: avoid`, so a verse is never split across two
  pages. This is the reason the renderer groups lines into blocks at all.
- A section heading carries `break-after: avoid`, so no page ends with a heading
  and nothing under it.
- Tablature keeps its own box and its fixed-width font, because tab that reflows
  is worthless.
- The chord positions appendix prints too, at a smaller size, showing the primary
  shape for each chord. Alternative positions do not print: a page of options is
  not what goes in a binder.
- Link URLs are not appended after link text.

## Printing a transposed sheet

`Imprimir` keeps whatever you are looking at. A sheet transposed on screen prints
transposed, and says so under the title, so a printout can never quietly be in
the wrong key. The `.song` file is untouched either way.

There is also a dedicated print view at `/songs/<slug>/print` which carries the
same query parameters, so `?t=2&n=solfege` prints a sheet two semitones up with
solfège note names.

## Manual checklist

Some of this cannot be automated, and a stylesheet that looks right in a browser
can still be wrong on paper. Before trusting a change to the sheet layout, print
these and look at them:

- [ ] A short song, one page. Does it use the width, or is it a narrow column
      down the middle?
- [ ] A four-page song. Does any verse straddle a page break? Does any page end
      with a heading?
- [ ] A song with no chords at all. Is the spacing tight, with no empty chord
      row above every line?
- [ ] A tab-heavy song. Do the tab blocks fit the width without being cut off?
- [ ] A song with many distinct chords. Does the positions appendix fit sensibly,
      or does it push onto a page of its own?
- [ ] A transposed sheet. Does it say so?
- [ ] Read one at arm's length, standing, on a music stand. Are the chords
      findable at a glance?

## Two columns, in a songbook

A single sheet is one column: two are harder to follow while playing, and a sheet
on a music stand has room.

A songbook is a different object. A song too long for one side is set in two
columns, which hold about twice as much — the difference between seeing a song
whole and turning the page over in the middle of a verse. The length is estimated
from the rendered lines, counting a chord line and the words under it as two.

A stanza may run over a page or a column. What may not is a heading separated
from the lines it introduces: the heading and the first two lines are one
unbreakable piece, said in the markup rather than with `break-after: avoid`, which
print engines honour unevenly and hardly at all inside columns.

## Headers and footers

The document prints none: no page numbers, no running titles, nothing but the
songs.

The line of small print some browsers add at the top and bottom of every sheet —
the URL, the date, the page number — is not ours and no stylesheet can remove it.
It is a checkbox in the print dialog, usually under *More settings* → *Headers
and footers*.

## Known limitations

- Page numbers and repeated headers on later pages are not printed, in a songbook
  either. Worth adding if a book of sixty songs turns out to want an index that
  points somewhere.
