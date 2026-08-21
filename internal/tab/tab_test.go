package tab

import (
	"strings"
	"testing"
)

// A six-string block, the shape almost every tab has.
const solo = `e|-----------------|
B|-----------------|
G|-----0---2---0---|
D|-0---------------|
A|-----------------|
E|-----------------|`

// lineWidths returns the length of every line, which is how alignment is
// checked: if the strings no longer agree, the tab is ruined.
func lineWidths(text string) []int {
	var out []int
	for _, ln := range strings.Split(text, "\n") {
		out = append(out, len(ln))
	}
	return out
}

func allEqual(ns []int) bool {
	for _, n := range ns[1:] {
		if n != ns[0] {
			return false
		}
	}
	return true
}

func TestLooks(t *testing.T) {
	tests := []struct {
		name, info, text string
		want             bool
	}{
		{"tagged tab", "tab", solo, true},
		{"tagged tablatura", "tablatura", solo, true},
		{"untagged but tab-shaped", "", solo, true},
		{"a tag naming something else", "chords", solo, false},
		{"prose", "", "This is just a note about the song.\nSecond line.", false},
		{"one line of dashes", "", "----------", false},
		{"chord grid, not tab", "", "C  G  Am  F\nC  G  Am  F", false},
		{"empty", "", "", false},
	}
	for _, tt := range tests {
		if got := Looks(tt.info, tt.text); got != tt.want {
			t.Errorf("%s: Looks = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestShiftUp(t *testing.T) {
	got, res := Shift("tab", solo, 2)
	if !res.Shifted {
		t.Fatalf("not shifted: %+v", res)
	}
	// Every fret up two.
	if !strings.Contains(got, "G|-----2---4---2---|") {
		t.Errorf("G string = wrong:\n%s", got)
	}
	if !strings.Contains(got, "D|-2---------------|") {
		t.Errorf("D string = wrong:\n%s", got)
	}
	if w := lineWidths(got); !allEqual(w) {
		t.Errorf("the strings no longer line up: %v\n%s", w, got)
	}
}

func TestShiftDown(t *testing.T) {
	// The lowest fret in the block is 0, so down is refused.
	got, res := Shift("tab", solo, -1)
	if res.Shifted {
		t.Error("shifted below the open string")
	}
	if !res.TooLow {
		t.Errorf("expected TooLow, got %+v", res)
	}
	if got != solo {
		t.Error("the block was modified even though the shift was refused")
	}

	// With room underneath, it moves.
	room := "e|---5---7---|\nB|---5---7---|"
	got, res = Shift("tab", room, -3)
	if !res.Shifted {
		t.Fatalf("not shifted: %+v", res)
	}
	if !strings.Contains(got, "e|---2---4---|") {
		t.Errorf("got:\n%s", got)
	}
}

// The interesting case: single digits becoming double must not skew the grid.
func TestAlignmentSurvivesWideningNumbers(t *testing.T) {
	src := "e|---7---9---7---|\nB|---0---2---0---|\nG|---9---7---9---|"
	got, res := Shift("tab", src, 3)
	if !res.Shifted {
		t.Fatalf("not shifted: %+v", res)
	}
	// 7->10, 9->12, 0->3, 2->5.
	for _, want := range []string{"10", "12", "-3-", "-5-"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
	if w := lineWidths(got); !allEqual(w) {
		t.Errorf("lines have different lengths %v, so the tab is skewed:\n%s", w, got)
	}
	// The bar lines have to stay in the same column on every string, which is
	// the thing a reader actually notices.
	for _, ln := range strings.Split(got, "\n") {
		if strings.Count(ln, "|") != 2 {
			t.Errorf("line %q lost a bar line", ln)
		}
	}
	cols := map[int]bool{}
	for _, ln := range strings.Split(got, "\n") {
		cols[strings.LastIndexByte(ln, '|')] = true
	}
	if len(cols) != 1 {
		t.Errorf("the closing bar is at different columns on different strings:\n%s", got)
	}
}

// Going back down again must return the original, which is the property that
// makes transposing safe to play with.
func TestShiftRoundTrips(t *testing.T) {
	for _, src := range []string{
		"e|---5---7---|\nB|---5---7---|",
		"e|---9---11---|\nB|---0----2---|",
		"G|-----5---7---5---|\nD|-5---------------|",
	} {
		up, res := Shift("tab", src, 4)
		if !res.Shifted {
			t.Fatalf("could not shift up: %+v", res)
		}
		back, res := Shift("tab", up, -4)
		if !res.Shifted {
			t.Fatalf("could not shift back: %+v", res)
		}
		// The numbers must match; the padding may differ, so compare the frets.
		if fretsOf(back) != fretsOf(src) {
			t.Errorf("round trip changed the notes\nfrom: %s\nto:   %s", fretsOf(src), fretsOf(back))
		}
	}
}

// fretsOf extracts the numbers in order, ignoring layout.
func fretsOf(text string) string {
	var out []string
	for _, ln := range strings.Split(text, "\n") {
		bar := strings.IndexByte(ln, '|')
		if bar < 0 {
			continue
		}
		cur := ""
		for _, r := range ln[bar+1:] {
			if r >= '0' && r <= '9' {
				cur += string(r)
				continue
			}
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		}
		if cur != "" {
			out = append(out, cur)
		}
		out = append(out, "/")
	}
	return strings.Join(out, " ")
}

func TestShiftRefusesToRunOffTheNeck(t *testing.T) {
	src := "e|---22---|\nB|---20---|"
	got, res := Shift("tab", src, 5)
	if res.Shifted {
		t.Error("shifted past the end of the neck")
	}
	if !res.TooHigh {
		t.Errorf("expected TooHigh, got %+v", res)
	}
	if got != src {
		t.Error("the block was modified even though the shift was refused")
	}
}

func TestTechniqueMarkingsSurvive(t *testing.T) {
	// Hammer-ons, slides, bends and mutes all have to come through untouched,
	// with the frets around them moved.
	src := "e|--5h7--9/11--7b9--x--|\nB|--5----0-----0----x--|"
	got, res := Shift("tab", src, 2)
	if !res.Shifted {
		t.Fatalf("not shifted: %+v", res)
	}
	for _, want := range []string{"h", "/", "b", "x"} {
		if !strings.Contains(got, want) {
			t.Errorf("the %q marking was lost:\n%s", want, got)
		}
	}
	// 5h7 becomes 7h9, 9/11 becomes 11/13, 7b9 becomes 9b11.
	for _, want := range []string{"7h9", "11/13", "9b11"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in:\n%s", want, got)
		}
	}
	if w := lineWidths(got); !allEqual(w) {
		t.Errorf("lines skewed %v:\n%s", w, got)
	}
}

func TestZeroShiftChangesNothing(t *testing.T) {
	got, res := Shift("tab", solo, 0)
	if res.Shifted {
		t.Error("a shift of zero reported a change")
	}
	if got != solo {
		t.Error("a shift of zero altered the block")
	}
}

func TestNonTabIsLeftAlone(t *testing.T) {
	prose := "Play this bit twice, then stop."
	got, res := Shift("", prose, 3)
	if !res.NotTab {
		t.Errorf("prose was treated as tablature: %+v", res)
	}
	if got != prose {
		t.Error("prose was modified")
	}
	// A tagged block naming something else is also off limits.
	if _, res := Shift("chords", solo, 3); !res.NotTab {
		t.Errorf("a block tagged 'chords' was shifted: %+v", res)
	}
}

func TestTabWithNoNumbers(t *testing.T) {
	src := "e|-------|\nB|-------|"
	got, res := Shift("tab", src, 5)
	if res.Shifted {
		t.Error("reported a change to a block with no frets in it")
	}
	if got != src {
		t.Error("a block with no frets was altered")
	}
}

// Lines that are not tablature inside an otherwise tab block — a comment, a
// blank line — must pass through untouched.
func TestMixedBlockKeepsItsProse(t *testing.T) {
	src := "Intro riff:\ne|---5---7---|\nB|---5---7---|\n(twice)"
	got, res := Shift("tab", src, 2)
	if !res.Shifted {
		t.Fatalf("not shifted: %+v", res)
	}
	if !strings.HasPrefix(got, "Intro riff:\n") {
		t.Errorf("the comment line was mangled:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n(twice)") {
		t.Errorf("the trailing note was mangled:\n%s", got)
	}
	if !strings.Contains(got, "---7---9---") {
		t.Errorf("the frets did not move:\n%s", got)
	}
}

func FuzzShift(f *testing.F) {
	f.Add(solo, 2)
	f.Add("e|---5h7---|\nB|---0-----|", -3)
	f.Add("nonsense", 1)
	f.Fuzz(func(t *testing.T, text string, semitones int) {
		if semitones < -36 || semitones > 36 {
			return
		}
		got, res := Shift("tab", text, semitones)
		if !res.Shifted {
			if got != text {
				t.Fatal("a refused shift still altered the block")
			}
			return
		}
		// Whatever came out must have the same number of lines, or content has
		// been lost.
		if strings.Count(got, "\n") != strings.Count(text, "\n") {
			t.Fatalf("line count changed\nfrom: %q\nto:   %q", text, got)
		}
		// No fret this package produced may run past the neck. Only the lines it
		// actually treated as tablature are checked: a line it declined to
		// touch keeps whatever was in it, which is the correct behaviour and not
		// something to hold against the shift.
		in, out := strings.Split(text, "\n"), strings.Split(got, "\n")
		for i := range out {
			if i >= len(in) || !isTabLine(in[i]) {
				continue
			}
			for _, f := range strings.Fields(fretsOf(out[i])) {
				if f == "/" {
					continue
				}
				n := 0
				for _, r := range f {
					n = n*10 + int(r-'0')
				}
				if n > MaxFret {
					t.Fatalf("produced fret %d on a shifted line, past the neck:\n%s", n, got)
				}
			}
		}
	})
}

// A line the package declines to treat as tablature keeps exactly what was in
// it, even if that is a number no guitar could play. Found by the fuzzer, which
// is what it is for.
func TestLinesThatAreNotTabAreNeverTouched(t *testing.T) {
	src := "|70\n|--0---0---|"
	got, res := Shift("tab", src, 12)
	if !res.Shifted {
		t.Fatalf("the real tab line was not shifted: %+v", res)
	}
	lines := strings.Split(got, "\n")
	if lines[0] != "|70" {
		t.Errorf("a non-tab line was rewritten: %q", lines[0])
	}
	if !strings.Contains(lines[1], "12") {
		t.Errorf("the tab line did not move: %q", lines[1])
	}
}
