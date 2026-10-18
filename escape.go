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
// startsWithFieldMarker reports whether a line begins with a FIELD MARKER, and it
// is a transcription of docutils' own pattern rather than an approximation of it:
//
//	field_marker=r':(?![: ])([^:\\]|\\.|:(?!([ `]|$)))*(?<! ):( +|$)'
//
// Body.patterns, read for this. Go's regexp is RE2 and has neither lookahead nor
// lookbehind, so the three assertions are code: the opening colon may not be
// followed by a colon or a space, a colon INSIDE the name may not be followed by a
// space, a backtick or the end of the line, and the name may not END in a space.
//
// The approximation this replaces -- "^:[^:]+:(\s|$)", a name with no colon in it
// -- missed the shape that found this: a ":pep:" role docutils refused, written
// back as escaped text, starts a line with ":pep:\`PEP 522: Allow ...". The colon
// after "pep" is followed by a BACKSLASH, so it is a legal name character; the one
// after "522" is followed by a space, so it closes the marker. docutils read the
// whole first line as a field named "pep:\`PEP 522", put it in the DOCINFO, and
// warned "Field list ends without a blank line; unexpected unindent." about the
// second. 6 messages in 2 corpus files, and in both the paragraph was gone.
func startsWithFieldMarker(s string) bool {
	if len(s) < 3 || s[0] != ':' || s[1] == ':' || s[1] == ' ' {
		return false
	}
	for i := 1; i < len(s); {
		if s[i] == ':' {
			// A closing colon: not preceded by a space, and followed by a
			// space or the end of the line.
			if s[i-1] != ' ' && (i+1 == len(s) || s[i+1] == ' ') {
				return true
			}
			// Otherwise it may still be part of the NAME, unless what
			// follows forbids it.
			if i+1 == len(s) || s[i+1] == ' ' || s[i+1] == '`' {
				return false
			}
			i++
			continue
		}
		if s[i] == '\\' {
			i += 2
			continue
		}
		i++
	}
	return false
}

// isUnderline reports a line of TWO or more of one punctuation character,
// which is what docutils will read as a section underline when it sits
// directly under the first line of a block. Two is the threshold rather than
// four because the shorter run still changes the document: the reference keeps
// the text and adds a "Possible title underline, too short for the title."
// INFO to it, and a diagnostic nobody wrote is the thing this file exists to
// prevent. Four or more is worse -- the paragraph becomes a <title>.
func isUnderline(s string) bool {
	s = strings.TrimRight(s, " ")
	r := []rune(s)
	if len(r) < 2 {
		return false
	}
	if !strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", r[0]) {
		return false
	}
	for _, c := range r[1:] {
		if c != r[0] {
			return false
		}
	}
	return true
}

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

// Only the first line is escaped, AND THE SECOND WHEN IT IS AN ADORNMENT, and
// that pair is the rule rather than a shortcut.
//
// docutils reads a whole text block before deciding what it is, so a line
// shaped like an option list or a bullet is plain content once a paragraph has
// started, and escaping every line corrupted the ones INSIDE an inline literal
// -- "“python\n-m zipapp“" became "python \-m zipapp", a backslash in
// verbatim content -- which four corpus files caught immediately.
//
// The exception is real and was found by a probe rather than by reading: the
// Text state looks at exactly ONE more line before it commits, and a uniform
// run there is an UNDERLINE. Measured against the reference, by line:
//
//	line 2, 4+ characters   "first line\n----"  -> a <title>, plus "Title
//	                                               underline too short."
//	line 2, 2-3 characters  "first line\n--"    -> the text survives with a
//	                                               "Possible title underline"
//	                                               INFO added to the document
//	line 3 or later         "a\nb\n----"        -> plain paragraph text
//
// So a paragraph carrying "first line\n----" was written out verbatim and came
// back as a SECTION TITLE: text became document structure. A leading backslash
// stops it, and the reference then reads the text node back exactly -- the
// backslash is removed and nothing else changes.
//
// This is narrow on purpose: the second line of a block, nothing else, which
// is the only line docutils gives that meaning to.
func escapeBlockStart(text string) string {
	lines := strings.Split(text, "\n")
	// The first line of each PARAGRAPH, not just of the text: a blank line
	// starts a new block, so the line after one is at a block start again. For
	// a paragraph (which holds no blank line) this is exactly the old
	// line-0-only behaviour; it matters for a table CELL, whose two paragraphs
	// are separated by a blank line since this writer started keeping the break
	// (see cellInlines).
	atBlockStart := true
	secondLine := false
	for i, l := range lines {
		if !atBlockStart {
			if strings.TrimSpace(l) == "" {
				atBlockStart = true
				secondLine = false
				continue
			}
			if secondLine {
				secondLine = false
				trimmed := strings.TrimLeft(l, " ")
				if isUnderline(trimmed) {
					lines[i] = l[:len(l)-len(trimmed)] + `\` + trimmed
				}
			}
			continue
		}
		if strings.TrimSpace(l) == "" {
			continue
		}
		atBlockStart = false
		secondLine = true
		trimmed := strings.TrimLeft(l, " ")
		if trimmed == "" {
			continue
		}
		indent := l[:len(l)-len(trimmed)]
		if isAdornment(trimmed) || startsWithFieldMarker(trimmed) {
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

// escapeSoftWrapIndent writes the INDENTATION that follows a newline inside a
// text node the way reST spells it: a backslash, then one space more than the
// indentation itself.
//
// That arithmetic is the reference's, measured rather than reasoned -- a
// backslash escapes the ONE whitespace character after it and is itself
// removed, so "\" plus three spaces reads back as two:
//
//	\   two spaces   ->  "\n  "
//	\  one space     ->  "\n "
//	\ nothing        ->  "\n"
//
// It used to drop the indentation instead, and the reason was sound as far as
// it went: a continuation deeper than its own first line is a definition or a
// block quote, so re-emitting PEP 262's wrap ("...around 28\n\   Dec 1999)")
// as a plain indented line came back as a DEFINITION LIST. The backslash is
// what the source has and what makes the line legal -- its first character is
// not a space, so nothing is indented -- and dropping it cost 14 of the 1564
// real-world files their own text nodes.
//
// A TAB in that run is left dropped, and cannot arise from a document anyway:
// docutils expands tabs when it READS the source (statemachine.string2lines),
// so "\" followed by a tab comes back as six spaces, not a tab, and no text
// node built from a source can hold one there.
//
// It runs AFTER escapeText, which would otherwise double the backslash this
// adds into a literal one.
func escapeSoftWrapIndent(s string) string {
	if !strings.ContainsAny(s, "\n\r") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		b.WriteByte(c)
		if c != '\n' && c != '\r' {
			continue
		}
		j := i + 1
		for j < len(s) && s[j] == ' ' {
			j++
		}
		if j == i+1 {
			continue
		}
		if j < len(s) && (s[j] == '\t') {
			// A tab in the run: dropped, as before, and the run with it.
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
			i = j - 1
			continue
		}
		b.WriteByte('\\')
		b.WriteString(strings.Repeat(" ", j-i))
		i = j - 1
	}
	return b.String()
}

// dropDanglingWrapEscape removes an escape [escapeSoftWrapIndent] left with
// nothing after it on its line. Such a line rstrips to a lone backslash, and
// the reference reads THAT as a title-underline candidate -- "Possible title
// underline, too short for the title." -- so the escape would trade a dropped
// space for a diagnostic in the document.
//
// It cannot arise from a paragraph any more (docutils/rst v0.141.0 rstrips a
// paragraph's text, so its last text node does not end in whitespace), which
// is exactly why it is guarded here rather than relied upon: this function is
// the proof, not the parser's current behaviour.
func dropDanglingWrapEscape(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		t := strings.TrimRight(l, " ")
		if t == "\\" {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// escapeTrailingLiteralMarker escapes a "::" that ends a block of flattened
// text, where reST reads it as the LITERAL BLOCK MARKER rather than as two
// colons an author wrote.
//
// docutils' rule, ported in consumeParagraph and read from states.py:
// "(?<!\\)(\\\\)*::$" matches, and then the text keeps ONE colon
// ("data[:-1]") when the character before the pair is not a space or a
// newline. So PEP 473's "allowing to display the following:::" arrives as a
// paragraph whose text ends "following::" -- three colons in the source, two
// in the tree -- and writing those two back bare made the NEXT parse reduce
// them again, to one. A character of the document, lost once per round trip
// until it stabilised a colon short.
//
// A single backslash before the pair is what stops it: the lookbehind fails,
// so there is no marker and no reduction, and the escape itself is removed on
// the way in. The package's own literal-block cases already record that
// reading ("a single escaping backslash right before :: means only ONE real
// colon, no literal block, no reduction").
//
// This writer never NEEDS the trailing "::" to act as a marker: a
// richdoc.CodeBlock is written with its own "::" or as a directive, never by
// leaning on the paragraph before it.
//
// An ODD number of backslashes already before the pair means the text itself
// ends with an escaped backslash, where docutils' lookbehind already fails and
// adding another would change the text.
func escapeTrailingLiteralMarker(text string) string {
	if !strings.HasSuffix(text, "::") {
		return text
	}
	body := text[:len(text)-2]
	n := 0
	for n < len(body) && body[len(body)-1-n] == '\\' {
		n++
	}
	if n%2 == 1 {
		return text
	}
	return body + `\::`
}
