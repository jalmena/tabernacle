# Brand assets

| | |
| --- | --- |
| `icon.svg` | the mark. A chord grid: nut, four strings, two fingers |
| `icon.png` | 512×512 raster fallback, generated from the SVG |
| `thumbnail.png` | 1200×630 store card, generated from `thumbnail.html` |
| `screenshot-1..3.png` | the chord page, a song sheet, the library |

The mark is the wordmark's idea in a picture: `Taber` pale, `nacle` orange, so the
structure of the grid is the pale half and the fingers are the orange one. It
carries no fret lines, because at forty-eight pixels in a dock they turn to mush
and take the dots with them; and the dots sit on the two inner strings so they
stay inside the rectangle rather than hanging off its edge.

`icon.svg` is also the application's favicon, embedded from
`internal/web/static/`. Change one and change both.

## Regenerating

```sh
magick -background none assets/icon.svg -resize 512x512 assets/icon.png
firefox --headless --screenshot assets/thumbnail.png --window-size=1200,630 \
        "file://$PWD/assets/thumbnail.html"
```

The thumbnail is rendered by a browser rather than drawn as an SVG on purpose:
it copies its declarations from `app.css`, so the card carries the wordmark's
real weight and tightening.

It names the family outright rather than saying `system-ui`, and that difference
matters. On the page, `system-ui` is the reader's own desktop font, which is what
makes the interface look native — Adwaita Sans on GNOME, Segoe UI on Windows, SF
Pro on macOS. A store card is one fixed PNG, so it has to pick one, and it must
render the same on whatever machine builds it: headless Firefox has no desktop
to ask and quietly falls back to Noto Sans, which is nobody's system font.
Adwaita Sans is the one named, being close enough to the others that the card
does not look like a different product anywhere.

The screenshots come from the running application in the dark theme, which is
what the icon and the card are built for. They run library, song sheet, chord
page — what you see on opening it, then a song, then the tool.
