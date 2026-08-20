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

## Known limitations

- Column layout is not offered. Two columns fit more on a page and are harder to
  follow while playing, and the second is what matters.
- Page numbers and repeated headers on later pages are not printed. Worth adding
  if long songs turn out to be common in practice.
