package index

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"sort"
	"strings"
	"unicode"
)

// SlugForPath derives a song's URL identifier from its path in the collection.
//
// The slug comes from the path rather than from the title and artist because
// the path is unique by construction. Deriving it from metadata would need a
// tie-break for two songs with the same title, and the obvious tie-break — a
// counter — depends on the order files are walked in. That would make slugs
// unstable across a database rebuild, so a bookmark or a QR code taped to a
// music stand could silently come to point at a different song. "Delete the
// database and lose nothing" has to include the user's links.
//
//	"Traditional/Amazing Grace.song" -> "traditional-amazing-grace"
//	"Amazing Grace.song"             -> "amazing-grace"
func SlugForPath(rel string) string {
	rel = strings.TrimSuffix(rel, path.Ext(rel))
	parts := strings.Split(rel, "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := Slugify(p); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return "song"
	}
	return strings.Join(out, "-")
}

// Slugify reduces text to the restricted alphabet [a-z0-9-].
//
// Accented letters are folded to their base letter where that is a single rune,
// so "Joaquín Sabina" becomes "joaquin-sabina" and a URL stays typable. Anything
// else non-alphanumeric becomes a separator.
func Slugify(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastDash := true // suppresses a leading dash
	for _, r := range strings.ToLower(s) {
		if f, ok := fold[r]; ok {
			r = f
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			// A letter with no ASCII equivalent, such as Cyrillic or CJK. Keep
			// it: a slug the owner of the collection can read beats a row of
			// hyphens, and percent-encoding handles the URL.
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// fold maps the accented letters that actually turn up in artist and song names
// onto ASCII. It is deliberately a small table rather than full Unicode
// normalisation, which would mean a dependency for a cosmetic gain in URLs.
var fold = map[rune]rune{
	'á': 'a', 'à': 'a', 'ä': 'a', 'â': 'a', 'ã': 'a', 'å': 'a', 'ā': 'a',
	'é': 'e', 'è': 'e', 'ë': 'e', 'ê': 'e', 'ē': 'e',
	'í': 'i', 'ì': 'i', 'ï': 'i', 'î': 'i', 'ī': 'i',
	'ó': 'o', 'ò': 'o', 'ö': 'o', 'ô': 'o', 'õ': 'o', 'ø': 'o', 'ō': 'o',
	'ú': 'u', 'ù': 'u', 'ü': 'u', 'û': 'u', 'ū': 'u',
	'ñ': 'n', 'ç': 'c', 'ß': 's', 'ý': 'y', 'ÿ': 'y', 'ł': 'l',
	'æ': 'a', 'œ': 'o', 'ð': 'd', 'þ': 't', 'š': 's', 'ž': 'z', 'č': 'c',
}

// ResolveSlugs assigns a unique slug to every path in the collection.
//
// Slugifying is lossy, so two different paths can reduce to the same slug
// ("A/B-C.song" and "A-B/C.song"). The rule is: among the paths that collide,
// the lexicographically first keeps the plain slug and the others get a short
// hash of their own path appended. That is deterministic and independent of the
// order the files were walked in, so rebuilding the index from scratch produces
// exactly the same slugs, and adding a new colliding song does not change the
// slug of the one that was already there.
func ResolveSlugs(paths []string) map[string]string {
	groups := make(map[string][]string, len(paths))
	for _, p := range paths {
		s := SlugForPath(p)
		groups[s] = append(groups[s], p)
	}
	out := make(map[string]string, len(paths))
	for base, members := range groups {
		if len(members) == 1 {
			out[members[0]] = base
			continue
		}
		sort.Strings(members)
		for i, p := range members {
			if i == 0 {
				out[p] = base
			} else {
				out[p] = base + "-" + pathHash(p)
			}
		}
	}
	return out
}

// pathHash is a short, stable fingerprint of a path, used only to break slug
// collisions.
func pathHash(rel string) string {
	sum := sha256.Sum256([]byte(rel))
	return hex.EncodeToString(sum[:])[:6]
}

// sortKey prepares a title or artist for alphabetical ordering: folded to the
// slug alphabet, with a leading English or Spanish article moved out of the way
// so "The Beatles" files under B and "Los Lobos" under L.
func sortKey(s string) string {
	k := Slugify(s)
	for _, article := range []string{"the-", "a-", "an-", "el-", "la-", "los-", "las-", "un-", "una-"} {
		if strings.HasPrefix(k, article) {
			return strings.TrimPrefix(k, article)
		}
	}
	return k
}
