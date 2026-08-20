package index

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var slugAlphabet = regexp.MustCompile(`^[a-z0-9-]+$`)

func TestSlugForPath(t *testing.T) {
	tests := []struct{ rel, want string }{
		{"Traditional/Amazing Grace.song", "traditional-amazing-grace"},
		{"Amazing Grace.song", "amazing-grace"},
		{"Bill Withers/Ain't No Sunshine.song", "bill-withers-ain-t-no-sunshine"},
		{"Joaquín Sabina/Y nos dieron las diez.song", "joaquin-sabina-y-nos-dieron-las-diez"},
		{"AC-DC/T.N.T.song", "ac-dc-t-n-t"},
		{"Sigur Rós/Hoppípolla.song", "sigur-ros-hoppipolla"},
		{"Genre/Artist/Song.song", "genre-artist-song"},
		{"---.song", "song"}, // nothing usable survives; must still be a valid slug
	}
	for _, tt := range tests {
		got := SlugForPath(tt.rel)
		if got != tt.want {
			t.Errorf("SlugForPath(%q) = %q, want %q", tt.rel, got, tt.want)
		}
		if !slugAlphabet.MatchString(got) {
			t.Errorf("SlugForPath(%q) = %q, which leaves the [a-z0-9-] alphabet", tt.rel, got)
		}
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Amazing Grace", "amazing-grace"},
		{"  spaced  out  ", "spaced-out"},
		{"Joaquín", "joaquin"},
		{"Björk", "bjork"},
		{"Sigur Rós", "sigur-ros"},
		{"Mötley Crüe", "motley-crue"},
		{"Niño", "nino"},
		{"Beyoncé", "beyonce"},
		{"AC/DC", "ac-dc"},
		{"!!!", ""},
		{"", ""},
		{"F#m", "f-m"},
		{"a---b", "a-b"},
		{"-lead-and-trail-", "lead-and-trail"},
		{"Song 2", "song-2"},
	}
	for _, tt := range tests {
		if got := Slugify(tt.in); got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// The property the whole URL scheme rests on: rebuilding the index from the
// same files must produce the same slugs, whatever order the files are seen in.
func TestResolveSlugsIsDeterministicAndOrderIndependent(t *testing.T) {
	paths := []string{
		"Traditional/Amazing Grace.song",
		"Bill Withers/Ain't No Sunshine.song",
		"A/B-C.song", // these two collide once slugified
		"A-B/C.song",
		"Loose.song",
	}
	first := ResolveSlugs(paths)

	// Same set, reversed: a different walk order must change nothing.
	reversed := make([]string, len(paths))
	for i, p := range paths {
		reversed[len(paths)-1-i] = p
	}
	second := ResolveSlugs(reversed)

	if len(first) != len(second) {
		t.Fatalf("different sizes: %d and %d", len(first), len(second))
	}
	for p, s := range first {
		if second[p] != s {
			t.Errorf("%q got slug %q in one order and %q in another", p, s, second[p])
		}
	}

	// Slugs must be unique, or two songs share a URL.
	seen := make(map[string]string, len(first))
	for p, s := range first {
		if prev, dup := seen[s]; dup {
			t.Errorf("slug %q assigned to both %q and %q", s, prev, p)
		}
		seen[s] = p
		if !slugAlphabet.MatchString(s) {
			t.Errorf("slug %q for %q leaves the allowed alphabet", s, p)
		}
	}

	// The colliding pair: the lexicographically first path keeps the plain slug.
	if first["A-B/C.song"] != "a-b-c" {
		t.Errorf(`"A-B/C.song" got %q, want the plain slug "a-b-c"`, first["A-B/C.song"])
	}
	if s := first["A/B-C.song"]; s == "a-b-c" || !strings.HasPrefix(s, "a-b-c-") {
		t.Errorf(`"A/B-C.song" got %q, want "a-b-c-" plus a hash`, s)
	}
}

// Adding a song must not silently change the URL of one already there.
func TestResolveSlugsIsStableWhenSongsAreAdded(t *testing.T) {
	before := ResolveSlugs([]string{"A-B/C.song", "Other.song"})
	after := ResolveSlugs([]string{"A-B/C.song", "Other.song", "A/B-C.song"})

	for _, p := range []string{"A-B/C.song", "Other.song"} {
		if before[p] != after[p] {
			t.Errorf("%q changed slug from %q to %q when another song was added",
				p, before[p], after[p])
		}
	}
}

func TestResolveSlugsHandlesManyCollisions(t *testing.T) {
	var paths []string
	for i := range 50 {
		// Every one of these slugifies to "a-b".
		paths = append(paths, fmt.Sprintf("a/b%s.song", strings.Repeat("!", i)))
	}
	got := ResolveSlugs(paths)
	seen := make(map[string]bool, len(got))
	for p, s := range got {
		if seen[s] {
			t.Fatalf("duplicate slug %q (from %q)", s, p)
		}
		seen[s] = true
	}
	if len(seen) != len(paths) {
		t.Errorf("got %d distinct slugs for %d paths", len(seen), len(paths))
	}
}

func TestSortKeyMovesLeadingArticles(t *testing.T) {
	tests := []struct{ in, want string }{
		{"The Beatles", "beatles"},
		{"the beatles", "beatles"},
		{"Los Lobos", "lobos"},
		{"La Bamba", "bamba"},
		{"El Barrio", "barrio"},
		{"A Day in the Life", "day-in-the-life"},
		{"An Ending", "ending"},
		{"Theme from Shaft", "theme-from-shaft"}, // "The" is part of the word
		{"Radiohead", "radiohead"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := sortKey(tt.in); got != tt.want {
			t.Errorf("sortKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSortKeyOrdersSensibly(t *testing.T) {
	names := []string{"The Zombies", "Adele", "Los Lobos", "Beatles, The"}
	sort.Slice(names, func(i, j int) bool { return sortKey(names[i]) < sortKey(names[j]) })
	want := []string{"Adele", "Beatles, The", "Los Lobos", "The Zombies"}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("sorted = %v, want %v", names, want)
			break
		}
	}
}
