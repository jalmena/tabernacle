# Browser tests

What the Go tests cannot reach: typing in the words, dragging a chord, the
chooser, a finger on a phone. These drive a real browser against a real
Tabernacle. They are run by hand, before a release that touches the editor, and
are not part of CI.

```sh
go build -o /tmp/tabernacle ./cmd/tabernacle
export TABERNACLE_BIN=/tmp/tabernacle
export PLAYWRIGHT_CORE=/path/to/node_modules/playwright-core
node tests/browser/all.js
```

| Script | What it tries |
| --- | --- |
| `run.js [word]` | Every case in [cases/](cases/), in Chromium. A word narrows it to the cases whose name holds it, or to the file of that name. |
| `geometry.js` | Each chord over its letter, at the sizes of a desktop, a phone and a tablet and at four zooms. |
| `roundtrip.js` | Every song opened and saved, untouched and with every block touched, comes back byte for byte. `SONGS=dir` tries a copy of another collection. |
| `random.js [sessions] [steps] [seed]` | Random editing, with the rules the editor keeps checked after every step. A failure names its seed. |
| `firefox.js` | The layout and the main gestures in Firefox, over WebDriver BiDi. Skipped when there is no Firefox. |
| `all.js` | All of the above. |

## What they need

- Node 22 or later.
- A Tabernacle built from the tree being tried, named in `TABERNACLE_BIN`.
- [playwright-core](https://www.npmjs.com/package/playwright-core), named in
  `PLAYWRIGHT_CORE` unless Node finds it by itself, and a Chromium: the one
  Playwright downloaded, or the one named in `CHROMIUM`.
- For `firefox.js`, a `firefox` on the path or named in `FIREFOX`.

Nothing is installed by this repository and nothing in it is written to. Each
script starts its own instance on a free port, in Spanish, on a temporary
directory holding [examples/songs](../../examples/songs) and
[songs/](songs/), and removes it when it ends. Screenshots go to the directory
named in `SHOTS`, or to `tabernacle-browser-shots` in the system's temporary
directory.

## Adding a case

A case is a `test(name, async (page) => { … })` in a file under `cases/`. It
gets a page on an instance of its own, so it can change whatever it likes, and
fails if it throws or if the page logged an error. [harness.js](harness.js) has
the hands the cases share: `edit` opens a song, `caret` puts the caret in a
block, `marks` says where its chords stand, `dragChip` carries one, `sent` says
what a section would send, `file` reads what was saved.

The cases read the editor's own state from `element.block` on each `.lyr`, so
they say where a chord stands in the words, not where it was drawn. Whether it
is drawn there is what `geometry.js` is for.
