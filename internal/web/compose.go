package web

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/jalmena/tabernacle/internal/chord"
	"github.com/jalmena/tabernacle/internal/i18n"
	"github.com/jalmena/tabernacle/internal/index"
	"github.com/jalmena/tabernacle/internal/song"
	"github.com/jalmena/tabernacle/internal/voicing"
)

// The editor that shows a song as fields and sections rather than as the text of
// a file.
//
// Somebody adding a song should not have to know the format by heart. The fields
// are the metadata block and each section is its own box with its own label, so
// the parts of a song are the parts of the form.
//
// It composes text and does not write files. Whatever the form is doing, what
// leaves here is the text of a .song file and the ordinary create and save paths
// take it from there — one way in, one set of rules, one place a mistake can be
// made. The raw editor is still there behind a button, and pressing it hands
// over exactly the text these fields would have saved.

// composed is the editable song a request is carrying, and what the form is
// asking to do with it.
type composed struct {
	Parts song.Parts
	Raw   string // the text box, when the reader is editing the file directly
	Plain bool   // true when the reader asked for the file rather than the fields

	// Focus is the section the caret goes back into when the page is redrawn,
	// counted from one so that nothing asks for none: the one just added,
	// moved, cut or joined, so the reader carries on where they were.
	Focus int
}

// text is the .song file this form stands for.
func (c composed) text() string {
	if c.Plain {
		return c.Raw
	}
	return string(song.Put(c.Parts))
}

// readCompose pulls the editor out of a posted form.
//
// Values are tidied here and nowhere deeper: a trailing space somebody typed is
// slack rather than content, but a title that really ends in one is content, and
// song.Put has no way to tell them apart. This does, because only what came out
// of a form arrives here.
//
// It reports false for a form it cannot make a song of: a section sent as words
// and places that do not read, with no text beside them to stand for it. Saving
// that would save the section empty, and what was in it would be gone.
func readCompose(r *http.Request) (composed, bool) {
	// The file, when the form says so — and also when it posts text and none of
	// the fields, which is what an upload does and what anything written against
	// the editor before it had fields does. The field form always posts a title,
	// even an empty one, so its absence is the tell.
	_, hasFields := r.PostForm["title"]
	_, hasText := r.PostForm["text"]
	// The tuning is chosen from a list, with a box beside it for one the list
	// does not hold. The box is what counts when the list says so, and when
	// there is no list in what was sent.
	tuning := r.PostFormValue("tuning")
	if pick, ok := r.PostForm["tuningpick"]; ok && first(pick) != tuningCustom {
		tuning = first(pick)
	}
	c := composed{
		Plain: r.PostFormValue("plain") == "1" || (hasText && !hasFields),
		Raw:   r.PostFormValue("text"),
		Parts: song.Parts{
			Title:  strings.TrimSpace(r.PostFormValue("title")),
			Artist: strings.TrimSpace(r.PostFormValue("artist")),
			Key:    strings.TrimSpace(r.PostFormValue("key")),
			Capo:   strings.TrimSpace(r.PostFormValue("capo")),
			Tuning: strings.TrimSpace(tuning),
			Genre:  strings.TrimSpace(r.PostFormValue("genre")),
			Notes:  strings.Trim(newlines(r.PostFormValue("notes")), "\n"),
		},
	}
	for i := 0; ; i++ {
		n := strconv.Itoa(i)
		label, named := r.PostForm["label"+n]
		body, written := r.PostForm["body"+n]
		rows, laid := r.PostForm["rows"+n]
		if !named && !written && !laid {
			break
		}
		text := first(body)
		// A section edited with its chords above its words comes back as the
		// words and where each chord stands, and the brackets are written here.
		// One that nobody touched comes back as the text it was, untouched.
		if laid {
			got, ok := readRows(first(rows))
			switch {
			case ok:
				text = song.FromRows(got)
			case !written:
				return c, false
			}
		}
		c.Parts.Sections = append(c.Parts.Sections, song.Part{
			Label: strings.TrimSpace(first(label)),
			Body:  strings.Trim(newlines(text), "\n"),
		})
	}
	return c, true
}

// readRows reads a section the way the script sends one it has edited.
func readRows(posted string) ([]song.Row, bool) {
	var rows []song.Row
	if err := json.Unmarshal([]byte(posted), &rows); err != nil {
		return nil, false
	}
	return rows, true
}

// laidOut is a section's body taken apart for the script: its words, and where
// each bracket stood in them. It is empty for a body that cannot be shown that
// way, a bracket left open or a fence, and that section stays a box of text.
func laidOut(body string) string {
	rows, err := song.Rows(body)
	if err != nil {
		return ""
	}
	out, err := json.Marshal(rows)
	if err != nil {
		return ""
	}
	return string(out)
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

// newlines makes every line ending the same one, the way a browser does not.
//
// A textarea posts CRLF whatever the reader typed, so without this every song
// saved from the form would differ from the same song saved from a file.
func newlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

// act carries out whatever the form's buttons asked for, and reports whether
// anything was done — in which case the page is redrawn rather than saved.
//
// Adding and removing sections without saving means a round trip to the server,
// which is how the fret window and transposing already work. Nothing in the
// editor needs a script to reach it.
func act(r *http.Request, c *composed, say *i18n.Printer) bool {
	n := len(c.Parts.Sections)
	switch {
	case r.PostFormValue("stay") != "":
		// Enter in one of the fields, which is not a reason to leave.
		return true
	case r.PostFormValue("addnow") != "" && r.PostFormValue("add") != "":
		kind := r.PostFormValue("add")
		// After the section the caret is in, when the script has said which,
		// since that is where somebody working down a song wants it; at the end
		// otherwise.
		at := n
		if a, err := strconv.Atoi(r.PostFormValue("after")); err == nil && a >= 0 && a < n {
			at = a + 1
		}
		c.Parts.Sections = slices.Insert(c.Parts.Sections, at, song.Part{
			Label: sectionName(say, kind),
			Body:  scaffold(kind),
		})
		c.Focus = at + 1
		return true
	case r.PostFormValue("merge") != "":
		if picks := picked(r, n); len(picks) >= 2 {
			c.Parts.Sections = fuse(c.Parts.Sections, picks)
			c.Focus = picks[0] + 1
		}
		return true
	case r.PostFormValue("plainer") != "":
		c.Plain, c.Raw = true, c.text()
		return true
	case r.PostFormValue("fielder") != "":
		// Back to the fields, from whatever the text now says. Text with no
		// metadata block is a chord sheet somebody pasted, and is read as one. A
		// file that will not parse cannot be shown as fields, so the reader stays
		// where the mistake is.
		if !song.IsFile([]byte(c.Raw)) {
			c.Plain, c.Parts = false, song.FromSheet(c.Raw, say.T("section.verse"))
			return true
		}
		p, err := song.Take([]byte(c.Raw))
		if err != nil {
			return false
		}
		c.Plain, c.Parts = false, p
		return true
	}
	// The buttons below belong to one section each. Pressing one at the edge
	// of the song, the first section's up or the last one's down, changes
	// nothing and is still not a save.
	if i, ok := indexed(r, "split"); ok {
		at, _ := strconv.Atoi(r.PostFormValue("at"))
		var done bool
		c.Parts.Sections, done = cut(c.Parts.Sections, i, at)
		c.Focus = i + 1
		if done {
			c.Focus = i + 2
		}
		return true
	}
	if i, ok := indexed(r, "del"); ok {
		c.Parts.Sections = drop(c.Parts.Sections, i)
		return true
	}
	if i, ok := indexed(r, "up"); ok {
		c.Focus = i + 1
		if i > 0 && i < n {
			c.Parts.Sections, c.Focus = swap(c.Parts.Sections, i, i-1), i
		}
		return true
	}
	if i, ok := indexed(r, "down"); ok {
		c.Focus = i + 1
		if i+1 < n {
			c.Parts.Sections, c.Focus = swap(c.Parts.Sections, i, i+1), i+2
		}
		return true
	}
	return false
}

// picked lists the sections ticked for joining, in the order they stand.
func picked(r *http.Request, n int) []int {
	var out []int
	for i := 0; i < n; i++ {
		if r.PostFormValue("pick"+strconv.Itoa(i)) != "" {
			out = append(out, i)
		}
	}
	return out
}

// fuse joins the picked sections into one, standing where the first of them
// stood, with a blank line between what each held.
//
// It takes the name most of them share, and the first one's when there is no
// majority, so three "Estrofa" and a "Coro" make an "Estrofa". A section with no
// name has no say, since joining it to a named one should not unname them both.
func fuse(ss []song.Part, picks []int) []song.Part {
	count := map[string]int{}
	for _, i := range picks {
		if ss[i].Label != "" {
			count[ss[i].Label]++
		}
	}
	merged, best := song.Part{}, 0
	var bodies []string
	for _, i := range picks {
		if l := ss[i].Label; count[l] > best {
			merged.Label, best = l, count[l]
		}
		if b := strings.Trim(ss[i].Body, "\n"); b != "" {
			bodies = append(bodies, b)
		}
	}
	merged.Body = strings.Join(bodies, "\n\n")

	gone := map[int]bool{}
	for _, i := range picks[1:] {
		gone[i] = true
	}
	out := make([]song.Part, 0, len(ss)-len(gone))
	for i, s := range ss {
		switch {
		case i == picks[0]:
			out = append(out, merged)
		case !gone[i]:
			out = append(out, s)
		}
	}
	return out
}

// cut splits a section in two before one of its lines, the second half going
// after it under the same name, which is the name it most likely still has. It
// reports false, and leaves the song alone, when either half would be empty.
func cut(ss []song.Part, i, at int) ([]song.Part, bool) {
	if i < 0 || i >= len(ss) {
		return ss, false
	}
	lines := strings.Split(strings.TrimLeft(ss[i].Body, "\n"), "\n")
	if at <= 0 || at >= len(lines) {
		return ss, false
	}
	first := strings.Trim(strings.Join(lines[:at], "\n"), "\n")
	second := strings.Trim(strings.Join(lines[at:], "\n"), "\n")
	if first == "" || second == "" {
		return ss, false
	}
	out := append(ss[:i:i], song.Part{Label: ss[i].Label, Body: first}, song.Part{Label: ss[i].Label, Body: second})
	return append(out, ss[i+1:]...), true
}

// indexed finds a button named like "del2" and says which section it meant.
func indexed(r *http.Request, prefix string) (int, bool) {
	for key := range r.PostForm {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if n, err := strconv.Atoi(key[len(prefix):]); err == nil && n >= 0 {
			return n, true
		}
	}
	return 0, false
}

func drop(ss []song.Part, n int) []song.Part {
	if n < 0 || n >= len(ss) {
		return ss
	}
	return append(ss[:n:n], ss[n+1:]...)
}

func swap(ss []song.Part, a, b int) []song.Part {
	if a < 0 || b < 0 || a >= len(ss) || b >= len(ss) {
		return ss
	}
	out := append(ss[:0:0], ss...)
	out[a], out[b] = out[b], out[a]
	return out
}

// sectionKinds are the sections a song is usually made of, in the order they
// tend to come in one.
//
// The list is short on purpose. It is a way of not having to remember what a
// heading looks like, not a vocabulary a reader has to fit their song into: the
// label is an ordinary text box and anything can be typed over what the button
// put there.
var sectionKinds = []song.Kind{
	song.KindIntro, song.KindVerse, song.KindPreChorus, song.KindChorus,
	song.KindBridge, song.KindSolo, song.KindInterlude, song.KindOutro,
}

// sectionName is what a kind is called in the reader's language, which is what
// goes in the heading. The heading is the reader's text and stays whatever they
// make of it; this only decides what it starts as.
func sectionName(say *i18n.Printer, kind string) string {
	for _, k := range sectionKinds {
		if string(k) == kind {
			return say.T("section." + kind)
		}
	}
	return ""
}

// scaffold is what a new section starts with.
//
// Only a solo gets anything: six strings of an empty grid, in a fenced block,
// because a tab is the one section whose shape nobody can be expected to type
// from memory and the one where getting the alignment right by hand is fiddly.
// Everything else starts empty, since a verse is just words.
func scaffold(kind string) string {
	if song.Kind(kind) != song.KindSolo && song.Kind(kind) != song.KindTab {
		return ""
	}
	var b strings.Builder
	b.WriteString("```tab\n")
	for _, s := range []string{"e", "B", "G", "D", "A", "E"} {
		b.WriteString(s + "|" + strings.Repeat("-", tabWidth) + "|\n")
	}
	b.WriteString("```")
	return b.String()
}

// tabWidth is how many dashes an empty grid starts with: two bars of four in
// eighths, which is the length of phrase somebody writes down.
const tabWidth = 32

// The header of a song as lists to choose from.
//
// The key and the tuning each have a short list of the values nearly every song
// has, so they are chosen and not typed. The artist and the genre are anybody's
// words and stay boxes to type in, with what the collection already holds on
// offer, so that the same artist is not spelled two ways.
//
// None of it changes what a file may say. A value a list does not hold is kept
// as it is written and shown as itself, and a song that is only opened and
// saved again is saved with the header it had.

// keyOptions is every key a song is usually in.
type keyOptions struct {
	// Written is the song's key when it is none of them, Gb or A7, to be offered
	// as it stands.
	Written string
	Major   []option
	Minor   []option
}

// Keys lists the keys, spelled the way each is usually written: Eb and not D#,
// F#m and not Gbm.
func (b editorBody) Keys() keyOptions {
	key := b.Compose.Parts.Key
	out := keyOptions{Major: make([]option, 0, 12), Minor: make([]option, 0, 12)}
	found := key == ""
	for pc := range 12 {
		major := chord.Spell(pc, chord.SpellingFor(pc, false)).Format(chord.English)
		minor := chord.Spell(pc, chord.SpellingFor(pc, true)).Format(chord.English) + "m"
		out.Major = append(out.Major, option{Value: major, Label: major, On: key == major})
		out.Minor = append(out.Minor, option{Value: minor, Label: minor, On: key == minor})
		found = found || key == major || key == minor
	}
	if !found {
		out.Written = key
	}
	return out
}

// tuningCustom is what the list of tunings sends when the tuning is one it does
// not hold, typed in the box beside it.
const tuningCustom = "custom"

// tuningOptions is the tunings that have diagrams, and the one that is typed.
type tuningOptions struct {
	Known  []option
	Custom bool   // the song's tuning is not one of them
	Other  string // and is this
}

// Tunings lists the tunings the application knows, the usual one first.
//
// The usual one goes without saying: chosen for a song that names no tuning, it
// writes none. A song that does name its tuning, "EADGBE" or "drop d", keeps its
// own words for it, so that opening a song and saving it changes nothing.
func (b editorBody) Tunings(say *i18n.Printer) tuningOptions {
	written := b.Compose.Parts.Tuning
	current, known := voicing.ParseTuning(written)
	out := tuningOptions{Custom: !known}
	if !known {
		out.Other = written
	}
	for i, t := range voicing.Tunings() {
		o := option{Value: t.Name, Label: tuningLabel(t)}
		if i == 0 {
			o.Value, o.Label = "", say.T("editor.tuning.standard")
		}
		if known && current.Equal(t) {
			o.On, o.Value = true, written
		}
		out.Known = append(out.Known, o)
	}
	return out
}

// tuningLabel is a tuning's name and, when the name does not say them, the
// notes its strings are tuned to.
func tuningLabel(t voicing.Tuning) string {
	spelling := chord.Sharps
	if len(t.Name) > 1 && t.Name[1] == 'b' {
		spelling = chord.Flats
	}
	names := make([]string, len(t.Open))
	plain := true
	for i, midi := range t.Open {
		names[i] = chord.Spell(midi, spelling).Format(chord.English)
		plain = plain && len(names[i]) == 1
	}
	sep := " "
	if plain {
		sep = ""
	}
	notes := strings.Join(names, sep)
	if strings.EqualFold(notes, t.Name) {
		return t.Name
	}
	return t.Name + " (" + notes + ")"
}

// offer is what the artist and the genre can be chosen from.
type offer struct {
	Artists []string
	Genres  []string
}

// offerFor gathers the artists the collection has, and its genres followed by
// the usual ones.
//
// The collection's own come first, the most used first, because the genre
// somebody wants is nearly always one they have used. The rest are there so
// that the list is worth opening on a collection that has none yet. A failure
// to read the collection leaves the lists short and the boxes as they were.
func (s *Server) offerFor(r *http.Request, say *i18n.Printer) offer {
	var out offer
	if artists, err := s.lib.Artists(r.Context()); err == nil {
		for _, a := range artists {
			out.Artists = append(out.Artists, a.Name)
		}
	}

	seen := map[string]bool{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		key := index.Slugify(name)
		if key == "" {
			key = strings.ToLower(name) // a script the slug alphabet has no letters for
		}
		if name == "" || seen[key] {
			return
		}
		seen[key] = true
		out.Genres = append(out.Genres, name)
	}
	if genres, err := s.lib.Genres(r.Context()); err == nil {
		for _, g := range genres {
			add(g.Name)
		}
	}
	usual := append(strings.Split(say.T("editor.genres"), ","), commonGenres...)
	for i := range usual {
		usual[i] = strings.TrimSpace(usual[i])
	}
	slices.SortFunc(usual, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	for _, g := range usual {
		add(g)
	}
	return out
}

// commonGenres are the kinds of music that are called the same in most
// languages, which is why they are not in the catalogue of messages: a rumba is
// a rumba. The few that are words and not names, "ballad", "children's", are
// translated there, under editor.genres.
var commonGenres = []string{
	"bachata", "bluegrass", "blues", "bolero", "bossa nova", "chacarera", "copla",
	"corrido", "country", "cumbia", "disco", "fado", "flamenco", "folk", "funk",
	"gospel", "grunge", "hard rock", "heavy metal", "hip hop", "indie", "jazz",
	"mariachi", "merengue", "metal", "milonga", "pasodoble", "pop", "pop rock",
	"punk", "R&B", "ranchera", "reggae", "rock", "rock and roll", "rockabilly",
	"rumba", "salsa", "samba", "sevillanas", "ska", "soul", "swing", "tango",
	"trova", "vallenato", "zamba",
}
