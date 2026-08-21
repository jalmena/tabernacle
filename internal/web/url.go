package web

import (
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/jalmena/tabernacle/internal/config"
	"github.com/jalmena/tabernacle/internal/render"
)

// encodeQuery renders a query string, including the leading "?" when there is
// anything to render.
//
// Keys are sorted so a given set of parameters always produces the same URL,
// which keeps the client-side sheet cache from storing the same page twice.
func encodeQuery(values map[string][]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteByte('?')
	first := true
	for _, k := range keys {
		for _, v := range values[k] {
			if !first {
				b.WriteByte('&')
			}
			first = false
			b.WriteString(url.QueryEscape(k))
			b.WriteByte('=')
			b.WriteString(url.QueryEscape(v))
		}
	}
	return b.String()
}

// songView describes which rendering of a song a URL should point at.
type songView struct {
	Slug      string
	Transpose int
	Notation  string
	Simplify  bool
	Print     bool
	Theme     string // print paper; only ever appears on a print URL

	// DefaultNotation is what a URL with no notation parameter would show, so
	// that the parameter can be left out when it would change nothing.
	DefaultNotation string
}

// URL builds the address of this view.
//
// Defaults are left out of the query string on purpose: the plain song URL stays
// clean and shareable, and two links that mean the same thing are the same
// string.
func (v songView) URL() string {
	q := map[string][]string{}
	if t := render.NormalizeTranspose(v.Transpose); t != 0 {
		q["t"] = []string{strconv.Itoa(t)}
	}
	// Only name the notation when it differs from what omitting it would give.
	// Otherwise every link on the page carries "?n=english", which makes a
	// shared URL noisy and stores the same sheet twice in the client's cache.
	if v.Notation != "" && v.Notation != v.DefaultNotation {
		q["n"] = []string{v.Notation}
	}
	if v.Simplify {
		q["s"] = []string{"1"}
	}
	base := "/songs/" + v.Slug
	if v.Print {
		base += "/print"
		// The paper only means anything on the print view, so it is left out
		// everywhere else rather than trailing around the whole application.
		if v.Theme != "" && v.Theme != "white" {
			q["theme"] = []string{v.Theme}
		}
	}
	return base + encodeQuery(q)
}

// with returns a copy of the view with one field changed.
func (v songView) withTranspose(n int) songView {
	v.Transpose = render.NormalizeTranspose(n)
	return v
}

func (v songView) withNotation(n string) songView {
	v.Notation = n
	return v
}

func (v songView) withSimplify(on bool) songView {
	v.Simplify = on
	return v
}

func (v songView) printing() songView {
	v.Print = true
	return v
}

// otherNotation is the one the toggle switches to.
func otherNotation(current string) string {
	if current == config.NotationSolfege {
		return config.NotationEnglish
	}
	return config.NotationSolfege
}
