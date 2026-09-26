// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"regexp"
	"strings"
)

// escapeText backslash-escapes the characters that start or end reST inline
// markup (*emphasis*, **strong**, single backtick interpreted text or double
// backtick literal, |substitution|, a trailing word_ reference), unconditionally rather than
// only where they'd actually be recognized — docutils' own adjacency rules
// for when a marker character truly triggers markup are intricate (see
// docutils/rst/inline.go's own SCOPE comment); over-escaping a character
// that wasn't actually dangerous produces a harmless "\x" in the source
// instead of silently corrupting a document where it was. The same
// conservative policy [github.com/go-richdoc/markdown]'s escapeText uses for
// CommonMark's own marker set.
func escapeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\', '*', '`', '|', '_':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// blockStarters are the line shapes reST reads as the start of a BLOCK rather
// than as paragraph text, transcribed from docutils' own Body patterns: a
// bullet, an enumerator in any of its six spellings, explicit markup, a field
// marker, an option list, a doctest block, and an adornment line (transition
// or section underline).
var blockStarters = []*regexp.Regexp{
	regexp.MustCompile(`^[-+*\x{2022}\x{2023}\x{2043}](\s|$)`),                // bullet
	regexp.MustCompile(`^\(?([0-9]+|[a-zA-Z]|[ivxlcdmIVXLCDM]+|#)[.)](\s|$)`), // enumerator
	regexp.MustCompile(`^\.\.(\s|$)`),                                         // explicit markup
	regexp.MustCompile(`^:[^:]+:(\s|$)`),                                      // field marker
	regexp.MustCompile(`^(-{1,2}[a-zA-Z]|/[a-zA-Z])`),                         // option list
	regexp.MustCompile(`^>>>\s`),                                              // doctest
	regexp.MustCompile(`^\+[-=]{2,}`),                                         // grid table top
	regexp.MustCompile(`^=+(\s+=+)+\s*$`),                                     // simple table top
}

// escapeBlockStart puts a backslash before the first character of any line
// that reST would otherwise read as a BLOCK construct. The backslash makes
// that character literal, so the line is a paragraph again, and it renders as
// nothing.
//
// escapeText handles the INLINE markers wherever they appear; this is the
// positional half, and it had exactly one case ("|", via escapeText) out of
// nine. A paragraph reading "B. Smith wrote this" came back as an enumerated
// list, "- Not a bullet either" as a bullet list, ":not: a field" as a field
// list, and ".. not a directive" as a COMMENT -- invisible in every rendering.
// The document parsed, so nothing reported any of it.
// isAdornment reports a line of four or more of ONE punctuation character,
// which reST reads as a transition or a section underline. Go's regexp is RE2
// and has no backreference, so this is code rather than a pattern -- the
// backreference version compiled to a panic at init, which the first test run
// caught.
func isAdornment(s string) bool {
	s = strings.TrimRight(s, " ")
	r := []rune(s)
	if len(r) < 4 {
		return false
	}
	first := r[0]
	if !strings.ContainsRune("!-/:-@[-`{-~", first) && !strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", first) {
		return false
	}
	for _, c := range r[1:] {
		if c != first {
			return false
		}
	}
	return true
}

// Only the FIRST line is escaped, and that is the rule rather than a
// shortcut: docutils reads a whole text block before deciding what it is, so a
// line shaped like an option list or a bullet is plain content once a paragraph
// has started. Escaping every line corrupted the ones INSIDE an inline literal
// -- "“python\n-m zipapp“" became "python \-m zipapp", a backslash in
// verbatim content -- which four corpus files caught immediately.
func escapeBlockStart(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if i > 0 {
			break
		}
		trimmed := strings.TrimLeft(l, " ")
		if trimmed == "" {
			continue
		}
		indent := l[:len(l)-len(trimmed)]
		if isAdornment(trimmed) {
			lines[i] = indent + `\` + trimmed
			continue
		}
		for _, re := range blockStarters {
			if re.MatchString(trimmed) {
				lines[i] = indent + `\` + trimmed
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}
