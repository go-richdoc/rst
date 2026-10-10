// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-richdoc/richdoc"
)

// reST recognizes inline markup only at a BOUNDARY: a start-string must be
// preceded by whitespace or one of these, and an end-string followed by
// whitespace or one of the suffix set (Inliner.start_string_prefix and
// end_string_suffix, docutils/rst's own port of them).
const (
	// ":" is here because a ROLE starts with one. PEP 410 writes
	// "10\ :sup:`-9`" -- an escaped space, reST's null separator, attaching the
	// superscript to the digit with no visible gap -- and without ":" in this
	// set the separator was not written back, so "10:sup:`-9`" came out. That is
	// not a role at all: docutils reads it as plain text and the superscript
	// DISAPPEARS. A colon at an inline boundary that was not markup gets a
	// harmless null separator, which is the same conservative trade escapeText
	// already makes.
	inlineStarters = "*`[|_:"
	validBeforeSet = "-:/'\"<([{‘“«¡¿"
	validAfterSet  = "-.,:;!?\\/'\")]}>’”»"
)

func lastRune(s string) rune {
	r := []rune(s)
	if len(r) == 0 {
		return 0
	}
	return r[len(r)-1]
}

func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

func okBefore(r rune) bool {
	return r == 0 || unicode.IsSpace(r) || strings.ContainsRune(validBeforeSet, r)
}

func okAfter(r rune) bool {
	return r == 0 || unicode.IsSpace(r) || strings.ContainsRune(validAfterSet, r)
}

// writeInlines joins the rendered inlines, inserting reST's NULL SEPARATOR
// ("\ ", a backslash-escaped space, which renders nothing) wherever the join
// would put markup where reST cannot see it.
//
// Without it, four of the five inline kinds silently stopped being markup when
// they followed a word directly: richdoc's own
// [Text("See it"), Emph("em"), Text(".")] was written "See it*em*." and read
// back as the literal text "See it*em*." -- emphasis, strong, inline literal
// and footnote reference all degraded, and only a phrase reference survived.
// A converter's output that no longer says what its input said is the worst
// kind of loss, because nothing reports it.
func (w *writer) writeInlines(nodes []richdoc.Inline) string {
	var b strings.Builder
	prev := ""
	for _, n := range nodes {
		s := w.writeInline(n)
		if s == "" {
			continue
		}
		if prev != "" {
			startsMarkup := strings.ContainsRune(inlineStarters, firstRune(s))
			endsMarkup := strings.ContainsRune(inlineStarters, lastRune(prev))
			if (startsMarkup && !okBefore(lastRune(prev))) ||
				(endsMarkup && !okAfter(firstRune(s))) {
				b.WriteString("\\ ")
			}
		}
		b.WriteString(s)
		prev = s
	}
	return b.String()
}

// writeInline renders one inline node. Several richdoc inlines have no
// native reST construct at all (an inline [richdoc.Image], a hard
// [richdoc.LineBreak] inside an ordinary paragraph, a point [richdoc.Anchor]
// with no visible text, an unresolved [richdoc.CrossRef] target with no
// matching hyperlink target elsewhere in the same document) — each is
// documented at its case below with what this writer does instead and why.
func (w *writer) writeInline(n richdoc.Inline) string {
	switch v := n.(type) {
	case richdoc.Text:
		// A newline inside a text node is reST's SOFT WRAP -- docutils keeps the
		// source's line breaks in the node, and this reader passes them through
		// -- so writing one back verbatim carries whatever indentation followed
		// it. PEP 262 wraps a paragraph with an escaped continuation
		// ("...around 28\n\   Dec 1999)"), and re-emitting that newline plus its
		// four spaces turned the paragraph into a DEFINITION LIST: the first
		// line became a term and the rest its definition.
		//
		// Collapsing the break to a single space is what reST means by it. A
		// HARD break is richdoc.LineBreak, a different node, so this cannot eat
		// one.
		return escapeText(collapseSoftWrap(v.Value))
	case richdoc.Emph:
		return "*" + w.writeInlines(v.Inlines) + "*"
	case richdoc.Strong:
		return "**" + w.writeInlines(v.Inlines) + "**"
	case richdoc.Strikethrough:
		// reST has no native strikethrough; this package's own convention
		// (see convertRole in parse.go, which reads it back) is the
		// generic-role syntax an unknown interpreted-text role would use.
		return rawRole("strike", plainText(v.Inlines))
	case richdoc.Code:
		return "``" + v.Value + "``"
	case richdoc.Link:
		return writeLink(v)
	case richdoc.Image:
		// reST's core syntax has no INLINE image (only the block-level
		// ".. image::" directive); degrading to the alt text keeps the
		// document readable instead of emitting a directive fragment that
		// can't legally appear inside a paragraph.
		return escapeText(v.Alt)
	case richdoc.Math:
		// Not core docutils either, but a role, like "strike" above, this
		// package's own Parse resolves back to richdoc.Math specifically.
		// rawMathRole, not rawRole: math is one of the three roles that KEEP
		// their backslashes, so escaping them doubled every TeX command.
		return rawMathRole(v.TeX)
	case richdoc.LineBreak:
		// reST paragraphs have no hard-break syntax (a literal newline
		// inside one is just a wrapped line, folded back to a space on
		// reparse); a literal newline is the closest visual approximation,
		// though it does not round-trip as a LineBreak.
		return "\n"
	case richdoc.RawInline:
		if v.Format == "" || strings.EqualFold(v.Format, "rst") {
			return v.Text
		}
		return ""
	case richdoc.Footnote:
		// EXPLICITLY numbered ("[1]_"), not auto ("[#]_"), and that choice was
		// MEASURED rather than reasoned. richdoc's Footnote carries no label,
		// so an auto-numbered source ("[#]_", 53 corpus files) cannot be told
		// from a manually numbered one and one spelling has to serve both.
		// Writing the auto form fixes those 53 and costs more than it buys: a
		// document whose labels are 1..N in order -- much the commoner shape --
		// round-trips EXACTLY under the explicit form and becomes an auto
		// reference under the other. Source-vs-output equivalence said so
		// plainly: 935 with this, 865 with "[#]_".
		return "[" + strconv.Itoa(w.footnoteNumber(v)) + "]_"
	case richdoc.Anchor:
		return writeAnchor(v)
	}
	// richdoc.Inline is closed; the only remaining variant is CrossRef.
	return writeCrossRef(n.(richdoc.CrossRef))
}

// writeAnchor renders a richdoc.Anchor as reST's inline internal target,
// "_`text`" (docutils/rst v0.4.0+ reads this back, see convertInlineElement
// in parse.go), when it has visible content — that syntax requires
// non-empty backtick-quoted text, so a point anchor (Inlines empty, per
// richdoc's own doc comment) has no reST equivalent at all and degrades to
// nothing, same as before. NOTE this is a lossy round-trip when Anchor.ID
// doesn't already match the normalized form of its own visible text (the
// common case for one this package's own Parse produced, since it always
// sets ID from that same text) — reST resolves an inline target by its
// VISIBLE TEXT, not by an externally supplied id, so a cross-converter
// Anchor whose id is a separate stable slug re-resolves under a different
// name on reparse. Still strictly better than the old behavior (dropping
// the anchor construct entirely, which left nothing any reference could
// ever resolve to).
func writeAnchor(a richdoc.Anchor) string {
	if len(a.Inlines) == 0 {
		return ""
	}
	return "_`" + plainText(a.Inlines) + "`"
}

func writeLink(l richdoc.Link) string {
	if len(l.Inlines) == 1 {
		if t, ok := l.Inlines[0].(richdoc.Text); ok {
			// A bare URL round-trips through this package's own
			// standalone-URI auto-recognition without any embedded-link
			// markup at all -- but ONLY if it is one. docutils recognises a
			// standalone URI as an absolute URI (a scheme, then ":") or an
			// email address, and nothing else (Inliner.patterns.uri, read
			// directly): "py-code.org" has neither, so collapsing
			// "`py-code.org <py-code.org>`__" to "py-code.org" turned a link
			// into plain text. PEP 770 writes exactly that.
			if t.Value == l.URL && isStandaloneURI(l.URL) && bareURIKeepsItsTail(l.URL) {
				return l.URL
			}
			// And so does a bare EMAIL address, which is the same
			// construct one layer down: docutils recognizes it standalone
			// and gives the reference a "mailto:" URI the source never
			// wrote. Only the URL form differed, so this fell through to
			// the explicit spelling and every plain address in a document
			// came back as "`a@b <mailto:a@b>`__" -- the biggest single
			// difference between the doctree of a source and the doctree of
			// its own round trip, measured over the 1564-file corpus.
			if l.URL == "mailto:"+t.Value {
				return t.Value
			}
		}
	}
	// ANONYMOUS ("`__"), not named ("`_"). docutils' Inliner.phrase_ref
	// appends an implicit <target> alongside the reference for the named
	// form only — and that target claims the link TEXT as a reference
	// name, which a link has no business doing. When the text matched an
	// anchor already in the document ("_`important term`" plus a link
	// reading "important term"), the stray claim collided with it, and
	// docutils/rst v0.57.0+ reports `Duplicate implicit target name` for
	// exactly that. Which is how this surfaced: the round-trip test caught
	// the writer emitting reST that no longer read back as what it was
	// written from. The anonymous form produces the same
	// <reference refuri="..."> with NO target at all — verified against
	// the reference implementation — which is what a link means here.
	return "`" + plainText(l.Inlines) + " <" + l.URL + ">`__"
}

// writeCrossRef renders a cross-reference. A citation (reST citations are
// self-contained label+body constructs, unlike LaTeX's external-bibliography
// \cite — see convertNoteRef in parse.go) emits a bare "[target]_" citation
// reference; without a matching ".. [target] ..." definition elsewhere in
// the document (richdoc.CrossRef carries no body to emit one from), it
// degrades gracefully to plain text on reparse, same as any other unresolved
// reference. A label reference emits this package's own embedded-alias
// convention ("`text <target_>`_"), which is subject to the same caveat: it
// only truly resolves if some other block in the document independently
// defines that target.
func writeCrossRef(c richdoc.CrossRef) string {
	if c.Kind == richdoc.RefCite {
		return "[" + c.Target + "]_"
	}
	text := plainText(c.Inlines)
	if text == "" {
		text = c.Target
	}
	return "`" + escapeText(text) + " <" + c.Target + "_>`_"
}

// plainText flattens inline content to its literal text, with no reST
// escaping — used where the result is embedded inside markup that already
// supplies its own delimiters (a link's visible text, a role's argument).
func plainText(nodes []richdoc.Inline) string {
	var b strings.Builder
	for _, n := range nodes {
		switch v := n.(type) {
		case richdoc.Text:
			b.WriteString(v.Value)
		case richdoc.Emph:
			b.WriteString(plainText(v.Inlines))
		case richdoc.Strong:
			b.WriteString(plainText(v.Inlines))
		case richdoc.Strikethrough:
			b.WriteString(plainText(v.Inlines))
		case richdoc.Code:
			b.WriteString(v.Value)
		case richdoc.Image:
			// The same degradation writeInline documents for an inline
			// image — the alt text. Without this case an image inside a
			// link contributed NOTHING, so a linked badge came out as
			// "` <uri>`__", an empty-label link that does not even read
			// back as a link. A paragraph that is only a linked image
			// becomes an ".. image::" directive before it reaches here
			// (see writeImageBlock); this is what is left for one that
			// shares its paragraph with something else.
			b.WriteString(v.Alt)
		case richdoc.Link:
			b.WriteString(plainText(v.Inlines))
		case richdoc.Math:
			b.WriteString(v.TeX)
		case richdoc.RawInline:
			b.WriteString(v.Text)
		case richdoc.Anchor:
			b.WriteString(plainText(v.Inlines))
		case richdoc.CrossRef:
			b.WriteString(plainText(v.Inlines))
		}
	}
	return b.String()
}

// writeInlinesPlain renders inline content as its literal text (see
// [plainText]) for a context, a heading, where a title-underline's length
// must match the title's VISIBLE width, not the byte length of markup
// characters that would otherwise inflate it.
func writeInlinesPlain(nodes []richdoc.Inline) string {
	return plainText(nodes)
}

// reScheme is the scheme of an absolute URI, transcribed from docutils'
// Inliner.patterns.uri: "[a-zA-Z][a-zA-Z0-9.+-]*" followed by ":".
var reScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9.+-]*:`)

// isStandaloneURI reports whether reST would recognise s written on its own as a
// hyperlink, which is what makes it safe to drop the embedded-link markup around
// it.
//
// Two alternatives, and no others (Inliner.patterns.uri, read directly): an
// ABSOLUTE URI -- a scheme and a colon -- or an EMAIL address. A bare host name
// is neither; docutils reads "py-code.org" as ordinary text, which is why the
// collapse has to be conditional. The email form is approximated by "it contains
// an @ and no space", which is enough here: the only question is whether to KEEP
// the markup, and keeping it for an address this test rejects is harmless --
// "`a@b <a@b>`__" is a valid link -- while dropping it for something reST does
// not recognise is not.
// bareURIKeepsItsTail reports whether a standalone URI's LAST CHARACTER would
// still be part of the URI once reST read it back. docutils ends a standalone
// URI before most trailing punctuation, so that a sentence's own full stop or
// a closing bracket is not swallowed into the link -- and a URL that genuinely
// ends in one of those cannot be written bare at all.
//
// One real-world footnote does: PEP 615 cites a Microsoft URL ending in "-",
// and written bare it came back as a link to the URL WITHOUT the hyphen plus a
// separate text node holding it. The embedded form keeps it.
//
// The surviving set was MEASURED against the reference rather than read off its
// regex -- every ASCII punctuation character in turn as the final character of
// "https://example.com/a", asking publish_doctree for the resulting refuri.
// Only "*+/=~" come back whole, and ASCII alphanumerics.
//
// Non-ASCII is in the REFUSING half, and the measurement is what says so: this
// function first returned true for it, on the reasoning that docutils' uri
// pattern would treat an unknown character as ordinary text and simply end the
// URI before it. It does worse than that. A URL ending in "e" with an acute
// accent comes back as "https://example.com" -- the whole PATH is gone, not
// just the last character -- and so do a CJK character, a copyright sign and a
// zero-width space. Only an em dash loses just itself. Written bare, such a URL
// does not lose a character, it loses its path.
func bareURIKeepsItsTail(s string) bool {
	if s == "" {
		return false
	}
	last := s[len(s)-1]
	if last >= 0x80 {
		return false
	}
	if last >= '0' && last <= '9' || last >= 'a' && last <= 'z' || last >= 'A' && last <= 'Z' {
		return true
	}
	return strings.IndexByte("*+/=~", last) >= 0
}

func isStandaloneURI(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t\n") {
		return false
	}
	if reScheme.MatchString(s) {
		return true
	}
	at := strings.IndexByte(s, '@')
	return at > 0 && at < len(s)-1
}
