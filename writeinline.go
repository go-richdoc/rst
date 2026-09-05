// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strconv"
	"strings"

	"github.com/go-richdoc/richdoc"
)

func (w *writer) writeInlines(nodes []richdoc.Inline) string {
	var b strings.Builder
	for _, n := range nodes {
		b.WriteString(w.writeInline(n))
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
		return escapeText(v.Value)
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
		return rawRole("math", v.TeX)
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
		w.footnotes = append(w.footnotes, v)
		return "[" + strconv.Itoa(len(w.footnotes)) + "]_"
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
		if t, ok := l.Inlines[0].(richdoc.Text); ok && t.Value == l.URL {
			// A bare URL round-trips through this package's own
			// standalone-URI auto-recognition without any embedded-link
			// markup at all.
			return l.URL
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
