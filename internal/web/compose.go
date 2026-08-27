package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jalmena/tabernacle/internal/i18n"
	"github.com/jalmena/tabernacle/internal/song"
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
func readCompose(r *http.Request) composed {
	// The file, when the form says so — and also when it posts text and none of
	// the fields, which is what an upload does and what anything written against
	// the editor before it had fields does. The field form always posts a title,
	// even an empty one, so its absence is the tell.
	_, hasFields := r.PostForm["title"]
	_, hasText := r.PostForm["text"]
	c := composed{
		Plain: r.PostFormValue("plain") == "1" || (hasText && !hasFields),
		Raw:   r.PostFormValue("text"),
		Parts: song.Parts{
			Title:  strings.TrimSpace(r.PostFormValue("title")),
			Artist: strings.TrimSpace(r.PostFormValue("artist")),
			Key:    strings.TrimSpace(r.PostFormValue("key")),
			Capo:   strings.TrimSpace(r.PostFormValue("capo")),
			Tuning: strings.TrimSpace(r.PostFormValue("tuning")),
			Genre:  strings.TrimSpace(r.PostFormValue("genre")),
			Notes:  strings.Trim(newlines(r.PostFormValue("notes")), "\n"),
		},
	}
	for i := 0; ; i++ {
		label, ok := r.PostForm["label"+strconv.Itoa(i)]
		body, ok2 := r.PostForm["body"+strconv.Itoa(i)]
		if !ok && !ok2 {
			break
		}
		c.Parts.Sections = append(c.Parts.Sections, song.Part{
			Label: strings.TrimSpace(first(label)),
			Body:  strings.Trim(newlines(first(body)), "\n"),
		})
	}
	return c
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
	switch {
	case r.PostFormValue("add") != "":
		kind := r.PostFormValue("add")
		c.Parts.Sections = append(c.Parts.Sections, song.Part{
			Label: sectionName(say, kind),
			Body:  scaffold(kind),
		})
		return true
	case r.PostFormValue("plainer") != "":
		c.Plain, c.Raw = true, c.text()
		return true
	case r.PostFormValue("fielder") != "":
		// Back to the fields, from whatever the text now says. A file that will
		// not parse cannot be shown as fields, so the reader stays where the
		// mistake is.
		p, err := song.Take([]byte(c.Raw))
		if err != nil {
			return false
		}
		c.Plain, c.Parts = false, p
		return true
	}
	if n, ok := indexed(r, "del"); ok {
		c.Parts.Sections = drop(c.Parts.Sections, n)
		return true
	}
	if n, ok := indexed(r, "up"); ok && n > 0 {
		c.Parts.Sections = swap(c.Parts.Sections, n, n-1)
		return true
	}
	if n, ok := indexed(r, "down"); ok && n+1 < len(c.Parts.Sections) {
		c.Parts.Sections = swap(c.Parts.Sections, n, n+1)
		return true
	}
	return false
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
