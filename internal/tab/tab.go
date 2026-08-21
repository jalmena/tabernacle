// Package tab transposes guitar tablature written as ASCII art.
//
// Shifting a tab means adding semitones to every fret number, which sounds
// trivial and is not: turning a 9 into a 10 makes that line one character longer
// than the five around it, and a tab whose columns no longer line up is worse
// than useless. So the block is parsed into a grid, shifted, and rebuilt with
// every column wide enough for the longest number in it.
//
// It is a separate package because it is a self-contained text problem with
// nothing to do with chords, and because it can then be tested on its own.
package tab

import (
	"strconv"
	"strings"
)

// MaxFret is the highest fret this package will produce. Two octaves is more
// than any guitar in the house has, and a number above it is a sign the shift
// has gone wrong rather than a note somebody can play.
const MaxFret = 24

// Result says what happened to a block.
type Result struct {
	// Shifted is false when the block was left exactly as it was.
	Shifted bool
	// TooLow is true when shifting down would have taken a note past the open
	// string, which is the floor: there is nothing below an open string.
	TooLow bool
	// TooHigh is true when shifting up would have run off the end of the neck.
	TooHigh bool
	// NotTab is true when the block did not look like tablature at all, in
	// which case it is none of this package's business.
	NotTab bool
}

// token is one piece of a tab line: either a fret number or a run of anything
// else.
type token struct {
	col  int // character position it started at in the original
	span int // how many characters it occupied there, before any shift
	// number is the fret when isNumber; text is the literal characters when not.
	number   int
	text     string
	isNumber bool
}

// Looks reports whether a verbatim block is tablature.
//
// The fence tag is believed when it is there, since somebody who wrote ```tab
// meant it. Otherwise the shape of the text decides: several lines, each with a
// vertical bar near the start and mostly dashes and digits after it.
func Looks(info, text string) bool {
	switch strings.ToLower(strings.TrimSpace(info)) {
	case "tab", "tabs", "tablature", "tablatura":
		return true
	case "":
	default:
		// A tag naming something else means hands off.
		return false
	}

	lines := strings.Split(text, "\n")
	tabLike := 0
	for _, ln := range lines {
		if isTabLine(ln) {
			tabLike++
		}
	}
	// Two lines is not much of a guess, but a single line of dashes could be
	// anything, and six is the common case.
	return tabLike >= 2
}

// isTabLine reports whether one line looks like a string of tablature.
func isTabLine(ln string) bool {
	bar := strings.IndexByte(ln, '|')
	if bar < 0 || bar > 4 {
		return false
	}
	body := ln[bar+1:]
	if len(body) < 3 {
		return false
	}
	dashes, other := 0, 0
	for _, r := range body {
		switch {
		case r == '-':
			dashes++
		case r >= '0' && r <= '9', r == '|', r == ' ':
		case strings.ContainsRune("hpbrtsxv/\\~^()*", r):
		default:
			other++
		}
	}
	// Dashes are what make a tab a tab.
	return dashes*3 >= len(body) && other == 0
}

// Shift adds semitones to every fret in a block and rebuilds it aligned.
//
// The block is returned unchanged, with the reason recorded, when the shift
// cannot be done: a fret cannot go below the open string, and there is no point
// pretending the neck is longer than it is. Leaving the tab as it was and saying
// so is more use to a player than a tab full of impossible numbers.
func Shift(info, text string, semitones int) (string, Result) {
	if !Looks(info, text) {
		return text, Result{NotTab: true}
	}
	if semitones == 0 {
		return text, Result{}
	}

	lines := strings.Split(text, "\n")
	parsed := make([][]token, len(lines))
	isTab := make([]bool, len(lines))

	lowest, highest := -1, -1
	for i, ln := range lines {
		if !isTabLine(ln) {
			continue
		}
		isTab[i] = true
		parsed[i] = tokenize(ln)
		for _, t := range parsed[i] {
			if !t.isNumber {
				continue
			}
			if lowest < 0 || t.number < lowest {
				lowest = t.number
			}
			if t.number > highest {
				highest = t.number
			}
		}
	}
	if lowest < 0 {
		// Tab-shaped but with no numbers in it; nothing to move.
		return text, Result{}
	}

	if lowest+semitones < 0 {
		return text, Result{TooLow: true}
	}
	if highest+semitones > MaxFret {
		return text, Result{TooHigh: true}
	}

	for i := range parsed {
		for j := range parsed[i] {
			if parsed[i][j].isNumber {
				parsed[i][j].number += semitones
			}
		}
	}
	return rebuild(lines, parsed, isTab), Result{Shifted: true}
}

// tokenize splits a tab line into fret numbers and everything else, recording
// where each piece began so the grid can be rebuilt.
func tokenize(ln string) []token {
	var out []token
	i := 0
	// The string label and the leading bar are not part of the grid.
	if bar := strings.IndexByte(ln, '|'); bar >= 0 && bar <= 4 {
		out = append(out, token{col: 0, span: bar + 1, text: ln[:bar+1]})
		i = bar + 1
	}
	for i < len(ln) {
		if ln[i] >= '0' && ln[i] <= '9' {
			j := i
			for j < len(ln) && ln[j] >= '0' && ln[j] <= '9' {
				j++
			}
			n, err := strconv.Atoi(ln[i:j])
			if err != nil {
				// Absurdly long digit run; treat it as text rather than a fret.
				out = append(out, token{col: i, span: j - i, text: ln[i:j]})
			} else {
				out = append(out, token{col: i, span: j - i, number: n, isNumber: true})
			}
			i = j
			continue
		}
		out = append(out, token{col: i, span: 1, text: string(ln[i])})
		i++
	}
	return out
}

// rebuild reassembles the block, widening columns so that every string stays in
// step.
//
// The subtlety is that a fret occupying two characters occupies two columns, so
// the room available to it is the width of both of them together. Reserving only
// its starting column is what skews a tab when a 9 becomes a 12: the other
// strings still fill the second column, and that string does not.
func rebuild(lines []string, parsed [][]token, isTab []bool) string {
	maxCol := 0
	for i := range parsed {
		if !isTab[i] {
			continue
		}
		for _, t := range parsed[i] {
			if e := t.col + t.span - 1; e > maxCol {
				maxCol = e
			}
		}
	}

	// Every original column starts one character wide, and grows only as much as
	// some token genuinely needs.
	width := make([]int, maxCol+1)
	for i := range width {
		width[i] = 1
	}
	room := func(t token) int {
		total := 0
		for c := t.col; c < t.col+t.span && c < len(width); c++ {
			total += width[c]
		}
		return total
	}
	// Widening a column can only ever help another token, so this settles
	// quickly; the bound is there so a pathological block cannot spin.
	for pass := 0; pass < 8; pass++ {
		grew := false
		for i := range parsed {
			if !isTab[i] {
				continue
			}
			for _, t := range parsed[i] {
				need := len(pieceOf(t))
				if short := need - room(t); short > 0 {
					// The deficit goes on the token's last column, which keeps
					// the growth next to the number that caused it.
					last := t.col + t.span - 1
					if last >= len(width) {
						last = len(width) - 1
					}
					width[last] += short
					grew = true
				}
			}
		}
		if !grew {
			break
		}
	}

	out := make([]string, len(lines))
	for i, ln := range lines {
		if !isTab[i] {
			out[i] = ln
			continue
		}
		at := make(map[int]token, len(parsed[i]))
		lastCol := -1
		for _, t := range parsed[i] {
			at[t.col] = t
			if e := t.col + t.span - 1; e > lastCol {
				lastCol = e
			}
		}

		var b strings.Builder
		filler := fillerOf(ln)
		for c := 0; c <= lastCol && c < len(width); c++ {
			if t, here := at[c]; here {
				piece := pieceOf(t)
				// Pad out to the room its whole span provides, so the next
				// column starts where the other strings expect it to.
				if n := room(t) - len(piece); n > 0 {
					piece += strings.Repeat(filler, n)
				}
				b.WriteString(piece)
				continue
			}
			if covered(parsed[i], c) {
				continue // a token starting earlier already accounted for this
			}
			b.WriteString(strings.Repeat(filler, width[c]))
		}
		out[i] = b.String()
	}
	return strings.Join(out, "\n")
}

// pieceOf is the text a token contributes after any shift.
func pieceOf(t token) string {
	if t.isNumber {
		return strconv.Itoa(t.number)
	}
	return t.text
}

// covered reports whether column c falls inside a token that started earlier on
// this line.
//
// The original span is what matters, not the shifted width: the columns describe
// where things sat before anything moved.
func covered(tokens []token, c int) bool {
	for _, t := range tokens {
		if t.col >= c {
			break
		}
		if t.col+t.span > c {
			return true
		}
	}
	return false
}

// fillerOf picks the character to pad with: whatever this line already uses
// between notes, which is a dash in every tab anybody writes.
func fillerOf(ln string) string {
	if strings.Contains(ln, "-") {
		return "-"
	}
	return " "
}
