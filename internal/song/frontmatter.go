package song

import (
	"fmt"
	"strings"

	"github.com/jalmena/tabernacle/internal/i18n"
)

// frontMatterDelim opens and closes the metadata block of a song file.
const frontMatterDelim = "---"

// SyntaxError reports a problem at a specific line of a song file. Line
// numbers are one-based so that they match what an editor shows.
//
// The complaint is carried as a catalogue key and its arguments rather than as a
// finished sentence, because the person who has to act on it is reading the
// interface in their own language. Error() renders it in English, which is what
// a log wants and what the error contract expects.
type SyntaxError struct {
	Line int
	Key  string
	Args []any
}

// Msg is the complaint in English, kept as a method so that the error reads the
// same in a log wherever it is printed.
func (e *SyntaxError) Msg() string { return i18n.For(i18n.English).T(e.Key, e.Args...) }

func (e *SyntaxError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg()) }

// lineErr builds a SyntaxError from a zero-based line index.
func lineErr(i int, key string, args ...any) error {
	return &SyntaxError{Line: i + 1, Key: key, Args: args}
}

// entry is one front-matter field as it was read from the file.
type entry struct {
	key    string
	line   int // zero-based
	scalar string
	list   []string
	isList bool
}

// parseFrontMatter reads the metadata block at the top of a song file and
// returns the fields plus the index of the first body line.
//
// The syntax is a deliberately small subset of YAML rather than YAML itself:
// flat fields, scalars, one-level lists and literal block scalars. Tabernacle
// has no third-party dependencies, and a restricted subset that refuses what
// it does not understand is safer than a hand-rolled parser that claims to be
// YAML and quietly misreads it. Every file accepted here is also valid YAML
// with the same meaning, so adopting a real YAML parser later would not be a
// format break.
func parseFrontMatter(lines []string) ([]entry, int, error) {
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != frontMatterDelim {
		return nil, 0, &SyntaxError{Line: 1, Key: "parse.start"}
	}

	var entries []entry
	seen := make(map[string]int)

	i := 1
	for i < len(lines) {
		raw := lines[i]
		if strings.TrimRight(raw, " \t") == frontMatterDelim {
			return entries, i + 1, nil
		}
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}
		if strings.HasPrefix(raw, "\t") {
			return nil, 0, lineErr(i, "parse.tabsfront")
		}
		if raw[0] == ' ' {
			return nil, 0, lineErr(i, "parse.indent")
		}
		if raw[0] == '-' {
			return nil, 0, lineErr(i, "parse.orphanitem")
		}

		colon := strings.IndexByte(raw, ':')
		if colon < 0 {
			return nil, 0, lineErr(i, "parse.fieldform")
		}
		key := raw[:colon]
		if !validFieldName(key) {
			return nil, 0, lineErr(i, "parse.fieldname", key)
		}
		if prev, dup := seen[key]; dup {
			return nil, 0, lineErr(i, "parse.fieldtwice", key, prev+1)
		}
		seen[key] = i

		e := entry{key: key, line: i}
		rest := strings.TrimSpace(raw[colon+1:])
		next := i + 1

		switch {
		case rest == "|" || rest == "|-":
			text, after, err := readBlockScalar(lines, i+1)
			if err != nil {
				return nil, 0, err
			}
			e.scalar, next = text, after
		case strings.HasPrefix(rest, ">"):
			return nil, 0, lineErr(i, "parse.folded")
		case strings.HasPrefix(rest, "["):
			items, err := parseFlowList(rest, i)
			if err != nil {
				return nil, 0, err
			}
			e.list, e.isList = items, true
		case rest == "":
			items, after, err := readBlockList(lines, i+1)
			if err != nil {
				return nil, 0, err
			}
			if len(items) > 0 {
				e.list, e.isList = items, true
				next = after
			}
		default:
			v, err := parseScalar(rest, i)
			if err != nil {
				return nil, 0, err
			}
			e.scalar = v
		}

		entries = append(entries, e)
		i = next
	}
	// Point at the opening delimiter rather than at the end of the file: the
	// question the user needs answered is which block was never closed.
	return nil, 0, &SyntaxError{Line: 1, Key: "parse.frontopen"}
}

func validFieldName(s string) bool {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}

// readBlockScalar reads an indented literal block starting at line start and
// returns its text with the common indentation removed.
//
// Trailing blank lines are dropped rather than preserved, so that "|" and "|-"
// mean the same thing here. A trailing newline in a notes field is not worth a
// second syntax to express.
func readBlockScalar(lines []string, start int) (string, int, error) {
	var block []string
	i := start
	for ; i < len(lines); i++ {
		raw := lines[i]
		if strings.TrimSpace(raw) == "" {
			block = append(block, "")
			continue
		}
		if strings.HasPrefix(raw, "\t") {
			return "", 0, lineErr(i, "parse.tabsblock")
		}
		if raw[0] != ' ' {
			break
		}
		block = append(block, raw)
	}
	for len(block) > 0 && strings.TrimSpace(block[len(block)-1]) == "" {
		block = block[:len(block)-1]
	}
	if len(block) == 0 {
		return "", i, nil
	}

	indent := -1
	for _, l := range block {
		if strings.TrimSpace(l) == "" {
			continue
		}
		n := len(l) - len(strings.TrimLeft(l, " "))
		if indent < 0 || n < indent {
			indent = n
		}
	}
	for j, l := range block {
		if len(l) >= indent {
			block[j] = l[indent:]
		} else {
			block[j] = ""
		}
	}
	return strings.Join(block, "\n"), i, nil
}

// readBlockList reads "- item" lines starting at line start.
func readBlockList(lines []string, start int) ([]string, int, error) {
	var items []string
	i := start
	for ; i < len(lines); i++ {
		raw := lines[i]
		trimmed := strings.TrimLeft(raw, " ")
		if !strings.HasPrefix(trimmed, "- ") && trimmed != "-" {
			break
		}
		if trimmed == "-" {
			return nil, 0, lineErr(i, "parse.emptyitem")
		}
		v, err := parseScalar(strings.TrimSpace(trimmed[2:]), i)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, v)
	}
	return items, i, nil
}

// parseFlowList reads an inline list such as [hymn, public-domain].
func parseFlowList(s string, line int) ([]string, error) {
	if !strings.HasSuffix(s, "]") {
		return nil, lineErr(line, "parse.listopen")
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if strings.ContainsAny(inner, "[]{}") {
		return nil, lineErr(line, "parse.nested")
	}
	if inner == "" {
		return nil, nil
	}
	var items []string
	for _, part := range splitFlow(inner) {
		v, err := parseScalar(strings.TrimSpace(part), line)
		if err != nil {
			return nil, err
		}
		if v == "" {
			return nil, lineErr(line, "parse.emptyitem")
		}
		items = append(items, v)
	}
	return items, nil
}

// splitFlow splits on commas that are not inside quotes.
func splitFlow(s string) []string {
	var parts []string
	var cur strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
			cur.WriteByte(c)
		case c == '\'' || c == '"':
			quote = c
			cur.WriteByte(c)
		case c == ',':
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	parts = append(parts, cur.String())
	return parts
}

// parseScalar reads a single value: double quoted, single quoted, or plain.
//
// Plain values are taken literally except for a trailing comment, which
// matches YAML: a "#" preceded by whitespace starts a comment. That is why a
// value containing such a "#" has to be quoted, and why the writer quotes
// those values automatically.
func parseScalar(s string, line int) (string, error) {
	if s == "" {
		return "", nil
	}
	switch s[0] {
	case '"':
		end := findClosing(s, '"')
		if end < 0 {
			return "", lineErr(line, "parse.dquote")
		}
		if err := checkTrailer(s[end+1:], line); err != nil {
			return "", err
		}
		return unescapeDouble(s[1:end]), nil
	case '\'':
		end := findClosing(s, '\'')
		if end < 0 {
			return "", lineErr(line, "parse.squote")
		}
		if err := checkTrailer(s[end+1:], line); err != nil {
			return "", err
		}
		return strings.ReplaceAll(s[1:end], "''", "'"), nil
	}

	if i := strings.Index(s, " #"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if strings.ContainsAny(s[:1], "&*!%@`{}|>,") {
		return "", lineErr(line, "parse.needquote", s[:1])
	}
	if strings.Contains(s, ": ") {
		return "", lineErr(line, "parse.colonquote")
	}
	return s, nil
}

// findClosing returns the index of the closing quote, honouring "\"" inside
// double quotes and "”" inside single quotes.
func findClosing(s string, quote byte) int {
	for i := 1; i < len(s); i++ {
		switch {
		case quote == '"' && s[i] == '\\':
			i++
		case s[i] == quote:
			if quote == '\'' && i+1 < len(s) && s[i+1] == '\'' {
				i++
				continue
			}
			return i
		}
	}
	return -1
}

// checkTrailer allows only whitespace or a comment after a quoted value.
func checkTrailer(s string, line int) error {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "#") {
		return nil
	}
	return lineErr(line, "parse.afterquote")
}

func unescapeDouble(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
