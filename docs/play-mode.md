# Play mode

Everything else in Tabernacle assumes you are reading. Play mode assumes both
your hands are on the guitar and the screen is a metre away on a stand.

Press **Play** on a song sheet. The page becomes the words and nothing else, at
a size you can read from the stand, and it scrolls by itself.

## What it does

- **Scrolls the sheet**, at one of eight speeds, from a crawl that keeps a long
  verse on screen to a pace for a song you already know.
- **Grows the words.** The size comes from `--song-size`, the same token the
  sheet already reads, so the chords, the lyrics and the tablature grow
  together instead of drifting out of alignment.
- **Hides everything that is not the song**: the header, the sections row, the
  transposition controls, the footer and the appendix of chord shapes. The
  scroll ends at the end of the song rather than at the twentieth diagram.
- **Keeps the screen on**, through the Wake Lock API, for as long as the mode is
  on. A browser without it, or one that refuses, loses nothing else.

Touching the page — a wheel, a finger — pauses the scroll, so looking ahead is
not a fight with it. The bar starts it again.

## The keys

| | |
| --- | --- |
| Space | Start or pause |
| ↑ / ↓ | Faster, slower |
| Esc | Leave |

## Why the button hides itself

Every other control on a sheet is a link or a form that works with scripting
turned off; transposition, notation and printing all round-trip to the server. A
page cannot scroll itself without a script, so the **Play** button is marked
`.needsjs` and stays hidden until `app.js` has run and put `scripted` on the
body. A button that does nothing is worse than no button, which is why it is
absent rather than dead.

It is the only control on a song sheet that needs scripting. The chord page has
the other one: hearing a shape needs a script and a sound card, and it is marked
the same way — see [chord diagrams](chord-diagrams.md).

## What is not remembered

Nothing. The speed and the text size last as long as the mode does.

Play mode is what you are doing right now, not a property of the song. A speed
that belonged to a song would belong in its `.song` file, next to `capo` and
`tuning`, because the files are the source of truth and the database is an
index that can be deleted. If per-song speed is ever wanted, that is where it
goes — not in a browser's local storage, where it would be invisible to a
backup and lost on the next device.
