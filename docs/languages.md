# Languages

The interface is available in nineteen languages. The songs are not touched: a
`.song` file is the writer's, and nothing here rewrites a lyric.

| | |
| --- | --- |
| Base | Español |
| Spain | English · Català · Galego · Euskara |
| Europe | Português · Français · Italiano · Deutsch · Nederlands · Polski · Русский |
| Further out | Türkçe · Bahasa Indonesia · 中文 · 日本語 · 한국어 · हिन्दी · العربية |

Each person picks their own in **Ajustes → Personalización**; it follows them
between devices, because it is stored with the account rather than in the
browser. `TABERNACLE_LANG` sets the language for anybody who has not chosen one,
including everybody on an unclaimed instance.

## How it is put together

One map per language, each in its own file — `internal/i18n/lang_ca.go` and so
on. Adding a language is adding a map, registering it in `catalogue.go` and
naming it in `All()`; nothing else in the package changes and a translator can
be handed one file.

Spanish is the base and the only one guaranteed complete. A key missing anywhere
else falls back to it, so a half-finished translation shows a Spanish sentence
rather than a blank or the key itself. Four tests keep that fallback a safety net
rather than a habit:

- every language has every key,
- a key takes the same arguments in every language, or `%s` lands in the wrong
  hole,
- no language repeats the Spanish word for word in more than 40% of its keys,
  which is what a copied file looks like — judged as a share, because between
  Spanish and Catalan a great many single words genuinely coincide,
- every language renders every page without a raw key showing through.

## Nothing speaks for the reader but the edge

A message is a key and its arguments until the moment it is rendered. That rule
reaches all the way down: the `.song` parser reports `parse.fieldtwice` and the
line it happened on, the import rejects a file with `why.notsong`, the chord
package warns with `warn.notuning` and the tuning's name. None of those packages
knows who is reading, and none of them should have to.

The exceptions are deliberate and narrow. A parse error is written to the index
and to the log in English, because there it is a diagnostic rather than something
a reader is shown. A refused `Host` is answered in English, because that happens
before there is a reader, a session or a page.

## Right to left

Arabic sets `dir="rtl"` on the page and the layout mirrors: the header, the
cards, the lists.

Two things do not mirror, and both matter.

**The guitar.** A chord grid is a picture of an instrument, not a piece of text.
Mirroring it would show a left-handed neck to a right-handed reader, and it would
disagree with the SVG diagrams beside it, which cannot mirror at all.

**The sheet.** A chord sits above the letter it falls on, and that position is a
count of characters from the left. Turn the line round and every chord lands on
the wrong syllable. What this protects is a Spanish song read by somebody whose
interface is in Arabic; a genuinely right-to-left song sheet is a different
problem and an open one.

## What is not translated

- **Song files.** Titles, lyrics, section labels and chord symbols are the
  writer's words.
- **Chord symbols.** `maj7` is `maj7` in every language a guitarist reads. Only
  the gloss beside it in the chooser is words.
- **Note names.** These are a separate setting — `A B C` or `Do Re Mi` — because
  which one a musician uses does not follow from which language they speak. A
  Spaniard reading the interface in English may well still want `Do Re Mi`.
- **The section labels a `.song` file uses.** `## Estrofa 1` is matched against a
  list of aliases in the parser, which is part of the file format rather than of
  the interface.

## A word on quality

The Romance languages, English, German and Dutch were written with confidence in
the guitar vocabulary — *traste*, *cejilla*, *cuerda al aire* and their cousins.

Japanese, Korean, Arabic, Hindi, Turkish, Indonesian, Polish and Russian were
written carefully but deserve a native musician's eye before anybody leans on
them. The terms most worth checking are the ones a chord diagram depends on:
fret, barre, open string, capo, and the names of the fingers.
