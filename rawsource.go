// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"sort"
	"strconv"
	"strings"

	"github.com/go-docutils/docutils/doctree"
)

// This file reconstructs reST source text for constructs richdoc has no
// native node for (directives, comments, field lists, definition lists, line
// blocks, an unresolved role or note reference, an orphan footnote/citation
// definition). docutils/rst's doctree keeps no byte-offset back-reference
// into the original source for these, so the text below is a resynthesis
// from parsed structure, not a verbatim slice — semantically equivalent
// reST, not necessarily byte-identical to what was typed. A footnote,
// field, definition or option-list body goes through [rawBlockBody] and
// is hung under its own marker by [hangUnder], so a multi-paragraph body
// keeps its paragraphs and a list inside one keeps its items. That
// replaced a helper joining each child block's text with a single SPACE,
// which turned two paragraphs into one sentence -- the same flattening
// removed from five other places in v0.99.0 and left in these four.
// Inline STYLING inside such a body is still lost: rendering it would
// need a second full inline-to-reST emitter just for this fallback path
// (see [Write]'s emitter for the one that matters: ordinary body text).

func rawComment(el *doctree.Element) string {
	text := doctree.AsText(el)
	if text == "" {
		return ".."
	}
	return ".. " + indentContinuation(text)
}

// rawDirectiveSource assembles a directive's reST source from its header
// line, its OPTION lines and its content — the one place the three are
// spaced correctly, because getting that spacing wrong produces source
// that no parser reads back.
//
// An option block must start on the line DIRECTLY under the directive.
// Five of these reconstructions put a blank line there, which ends the
// directive's own argument/option region: real docutils then reads
// ":alt: x" as a FIELD LIST in the content. For a figure that is an
// ERROR ("Figure caption must be a paragraph or empty comment") that
// discards the caption; for an admonition the field list is simply
// rendered, so ":class: x" appears as visible text in the document.
// Both were checked directly against docutils 0.23, not reasoned about.
//
// Content, in contrast, is separated from whatever precedes it by one
// blank line, which is why this cannot be a single join.
// inlineSourceOf renders a node's INLINE children back to reST source, by
// converting them to richdoc inlines and writing them with this package's own
// inline writer. One spelling of the inline grammar, used in both directions.
//
// The reading side used doctree.AsText, which returns a node's TEXT: every
// emphasis, inline literal, role and reference inside a construct that gets
// reconstructed was dropped, so a definition term "**Read the Docs**" came
// back as plain "Read the Docs" and the document quietly changed. Measured by
// comparing the doctree of a source with the doctree of its own round trip
// (/Users/Shared/rstcorpus/fidprobe), which is the only judge that can see it:
// the flattened text is already in the first tree, so a round-trip comparison
// reproduces it and calls it stable.
//
// The converter and writer here are THROWAWAY, and for this purpose that is
// not a shortcut but the right thing: a substitution reference inside a term
// must come back out as "|name|", the source form, not as the expansion the
// real converter resolves it to. What it does cost is a footnote inside one of
// these constructs, which keeps its marker and loses its body -- the real
// writer is what emits footnote definitions. No corpus document has one.
func inlineSourceOf(el *doctree.Element) string {
	c := &converter{
		footnoteDefs:  map[string]*doctree.Element{},
		substDefs:     map[string]*doctree.Element{},
		referenced:    map[string]bool{},
		consumed:      map[string]bool{},
		headingAnchor: map[string]string{},
		anchorAlias:   map[string]string{},
	}
	return (&writer{}).writeInlines(c.convertInlines(el.Children))
}

func rawDirectiveSource(header string, options []string, content string) string {
	switch {
	case len(options) == 0 && content == "":
		return header
	case len(options) == 0:
		return header + "\n\n" + indentBlock(content)
	case content == "":
		return header + "\n" + indentBlock(strings.Join(options, "\n"))
	default:
		return header + "\n" + indentBlock(strings.Join(options, "\n")+"\n\n"+content)
	}
}

func rawDirective(el *doctree.Element) string {
	header := ".. " + el.Attr("name") + "::"
	if args := el.Attr("arguments"); args != "" {
		header += " " + args
	}
	body := doctree.AsText(el)
	if body == "" {
		return header
	}
	return header + "\n\n" + indentBlock(body)
}

// rawAdmonition reconstructs one of the nine generic admonition
// directives (docutils/rst v0.27.0+, el.Tag itself IS the directive
// name), or ".. admonition:: TITLE" specifically, as literal reST
// source — richdoc has no admonition/callout block type at all (its own
// Block interface is a documented closed set), so like field/definition
// lists above this falls back to a RawBlock. Content is flattened the
// same lossy way rawDirective already does for a genuinely unimplemented
// directive (doctree.AsText per child block, joined by blank lines — no
// attempt at preserving deeper nested-list structure within one block):
// richdoc's own fully-structured converters exist for ordinary body
// content, this path only exists for a construct richdoc has nowhere
// else to put it. :class:/:name: are always reconstructed when present,
// even for ".. admonition::"'s own auto-generated default class —
// redundant but harmless on reparse, not worth the fragility of trying
// to detect "was this explicit".
func rawAdmonition(el *doctree.Element) string {
	header := ".. " + el.Tag + "::"
	var bodyLines []string
	for _, c := range el.Children {
		if ce, ok := c.(*doctree.Element); ok && ce.Tag == doctree.TagTitle {
			header += " " + inlineSourceOf(ce)
			break
		}
	}
	if class := el.Attr("class"); class != "" {
		bodyLines = append(bodyLines, ":class: "+class)
	}
	if name := el.Attr("name"); name != "" {
		bodyLines = append(bodyLines, ":name: "+name)
	}
	var contentParts []string
	for _, c := range el.Children {
		if ce, ok := c.(*doctree.Element); ok && ce.Tag == doctree.TagTitle {
			continue
		}
		if t := rawChildSource(c); t != "" {
			contentParts = append(contentParts, t)
		}
	}
	return rawDirectiveSource(header, bodyLines, strings.Join(contentParts, "\n\n"))
}

// rawTopic reconstructs ".. topic::" or ".. sidebar::" (docutils/rst
// v0.28.0+, el.Tag itself IS the directive name, same convention as
// rawAdmonition) as literal reST source — richdoc has no topic/sidebar
// block type either. Unlike an admonition's title, a topic's title is
// REQUIRED and a sidebar's is optional but may carry its own
// ":subtitle:" (only ever present alongside a title, per
// runTopicOrSidebar's own validation, so no empty-title case to guard
// here). Its title and subtitle keep their inline markup (inlineSourceOf);
// the CONTENT is still flattened the way rawAdmonition's is.
func rawTopic(el *doctree.Element) string {
	header := ".. " + el.Tag + "::"
	var subtitle string
	for _, c := range el.Children {
		ce, ok := c.(*doctree.Element)
		if !ok {
			continue
		}
		switch ce.Tag {
		case doctree.TagTitle:
			header += " " + inlineSourceOf(ce)
		case doctree.TagSubtitle:
			subtitle = inlineSourceOf(ce)
		}
	}
	var bodyLines []string
	if subtitle != "" {
		bodyLines = append(bodyLines, ":subtitle: "+subtitle)
	}
	if class := el.Attr("class"); class != "" {
		bodyLines = append(bodyLines, ":class: "+class)
	}
	if name := el.Attr("name"); name != "" {
		bodyLines = append(bodyLines, ":name: "+name)
	}
	var contentParts []string
	for _, c := range el.Children {
		if ce, ok := c.(*doctree.Element); ok {
			switch ce.Tag {
			case doctree.TagTitle, doctree.TagSubtitle:
				continue
			case doctree.TagPending:
				// An INTERNAL node: docutils/rst v0.99.0+ gives
				// ".. contents::" a <pending> child whose text is the
				// parser's own ".. internal attributes:" block. parse.go
				// already drops <pending> on the block path; this
				// reconstruction walked it as content and leaked
				// ".transform: docutils.transforms.parts.Contents" into
				// the rendered document.
				continue
			}
		}
		if t := rawChildSource(c); t != "" {
			contentParts = append(contentParts, t)
		}
	}
	return rawDirectiveSource(header, bodyLines, strings.Join(contentParts, "\n\n"))
}

// rawCompound reconstructs ".. compound::" (docutils/rst v0.42.0+) as
// literal reST source — richdoc has no compound-paragraph block type
// either. Structurally identical to one of the nine generic admonitions
// rawAdmonition already handles (no title, :class:/:name: options, free-
// form content), just a different directive name, so this is a thin
// wrapper rather than a duplicate implementation.
func rawCompound(el *doctree.Element) string {
	return rawAdmonition(el)
}

// rawContainer reconstructs ".. container::" (docutils/rst v0.42.0+) as
// literal reST source — richdoc has no container block type either.
// Unlike rawAdmonition/rawCompound, a container's classes come from its
// own directive ARGUMENT, not a :class: option (real docutils' own
// Container directive has no :class: in its option_spec at all, only
// :name:) — so the "class" attribute is written into the header's own
// argument position instead of a body option line.
func rawContainer(el *doctree.Element) string {
	header := ".. container::"
	if class := el.Attr("class"); class != "" {
		header += " " + class
	}
	var bodyLines []string
	if name := el.Attr("name"); name != "" {
		bodyLines = append(bodyLines, ":name: "+name)
	}
	var contentParts []string
	for _, c := range el.Children {
		if t := rawChildSource(c); t != "" {
			contentParts = append(contentParts, t)
		}
	}
	return rawDirectiveSource(header, bodyLines, strings.Join(contentParts, "\n\n"))
}

// rawRubric reconstructs ".. rubric:: TEXT" (docutils/rst v0.45.0+) as
// literal reST source — richdoc has no rubric block type either.
// Unlike rawAdmonition/rawTopic, a rubric's own text ISN'T a block-level
// <title> child, it's the rubric element's OWN inline content directly
// (no wrapping title node at all — see runRubricDirective's own doc
// comment), so this reads it via doctree.AsText on the element itself,
// the same lossy-but-content-preserving flattening literal_block's own
// TagLiteralBlock case already relies on for a parsed-literal's inline
// children (v0.45.0 also added those) — inline styling within the
// rubric's own text is lost, its actual text content isn't.
func rawRubric(el *doctree.Element) string {
	header := ".. rubric:: " + inlineSourceOf(el)
	var bodyLines []string
	if class := el.Attr("class"); class != "" {
		bodyLines = append(bodyLines, ":class: "+class)
	}
	if name := el.Attr("name"); name != "" {
		bodyLines = append(bodyLines, ":name: "+name)
	}
	return rawDirectiveSource(header, bodyLines, "")
}

// rawDecoration reconstructs docutils/rst v0.48.0's <decoration> — a
// document-level SINGLETON wrapping up to one <header> and one <footer>
// (in that fixed order, regardless of which was declared first in the
// source: docutils/rst always normalizes it that way, see that
// package's own doc comment) — as literal reST source, one ".. header::"/
// ".. footer::" block per child present, joined by a blank line when
// both are. richdoc has no document-header/footer concept at all.
// Neither directive has any options (real docutils' own Header/Footer
// declare no option_spec), so this is simpler than rawAdmonition/
// rawTopic: just the directive name and its content, no :class:/:name:
// line ever needed.
func rawDecoration(el *doctree.Element) string {
	var parts []string
	for _, c := range el.Children {
		ce, ok := c.(*doctree.Element)
		if !ok {
			continue
		}
		var contentParts []string
		for _, cc := range ce.Children {
			if t := strings.TrimSpace(doctree.AsText(cc)); t != "" {
				contentParts = append(contentParts, t)
			}
		}
		header := ".. " + ce.Tag + "::"
		if len(contentParts) > 0 {
			header += "\n\n" + indentBlock(strings.Join(contentParts, "\n\n"))
		}
		parts = append(parts, header)
	}
	return strings.Join(parts, "\n\n")
}

// rawImageTarget reconstructs the ":target:" option value of an image or
// figure from the <reference> docutils/rst v0.127.0+ wraps the <image>
// in. A resolved link gives its refuri back directly; a target written as
// another target's NAME is re-emitted in the form it was written in —
// backquoted when the name is not a single simplename word, bare
// otherwise — so the directive still says what the author said.
func rawImageTarget(ref *doctree.Element) string {
	if uri := ref.Attr("refuri"); uri != "" {
		return uri
	}
	name := ref.Attr("name")
	if name == "" {
		name = ref.Attr("refname")
	}
	if name == "" {
		return ""
	}
	if strings.ContainsAny(name, " \t`") {
		return "`" + name + "`_"
	}
	return name + "_"
}

// rawFigure reconstructs ".. figure::" (docutils/rst v0.29.0+) as
// literal reST source — richdoc has no figure/caption/legend concept
// either. Unlike rawTopic/rawAdmonition, a figure's own children are a
// FIXED shape (an <image>, then an optional <caption>, then an optional
// <legend>) rather than free-form content, so this reads them
// positionally instead of scanning for a <title>: every image-level
// option (alt/height/width/scale/loading/class/name) is reconstructed
// alongside the figure's own (figwidth/figclass/figname/align) on the
// SAME options block, matching real docutils' own shape (Figure directly
// reuses Image's option_spec on one directive invocation, not two nested
// ones) — round-trips semantically, not byte-identically, the same
// accepted cost as every other function in this file.
func rawFigure(el *doctree.Element) string {
	var img, caption, legend *doctree.Element
	target := ""
	for _, c := range el.Children {
		ce, ok := c.(*doctree.Element)
		if !ok {
			continue
		}
		switch ce.Tag {
		case doctree.TagImage:
			img = ce
		case doctree.TagReference:
			// docutils/rst v0.127.0+ honours the ":target:" option, and
			// Figure.run wraps whatever Image.run returned — so with a
			// target the <image> is a GRANDCHILD and this loop stopped
			// finding it. The header then came out as a bare
			// ".. figure::" with no URI at all: the picture, not just
			// its link, was gone from the converted document.
			target = rawImageTarget(ce)
			for _, gc := range ce.Children {
				if ge, ok := gc.(*doctree.Element); ok && ge.Tag == doctree.TagImage {
					img = ge
				}
			}
		case doctree.TagCaption:
			caption = ce
		case doctree.TagLegend:
			legend = ce
		}
	}
	header := ".. figure::"
	if img != nil {
		header += " " + img.Attr("uri")
	}
	var bodyLines []string
	if img != nil {
		if v := img.Attr("alt"); v != "" {
			bodyLines = append(bodyLines, ":alt: "+v)
		}
		if v := img.Attr("height"); v != "" {
			bodyLines = append(bodyLines, ":height: "+v)
		}
		if v := img.Attr("width"); v != "" {
			bodyLines = append(bodyLines, ":width: "+v)
		}
		if v := img.Attr("scale"); v != "" {
			bodyLines = append(bodyLines, ":scale: "+v)
		}
		if v := img.Attr("loading"); v != "" {
			bodyLines = append(bodyLines, ":loading: "+v)
		}
		if v := img.Attr("class"); v != "" {
			bodyLines = append(bodyLines, ":class: "+v)
		}
		if v := img.Attr("name"); v != "" {
			bodyLines = append(bodyLines, ":name: "+v)
		}
	}
	if target != "" {
		bodyLines = append(bodyLines, ":target: "+target)
	}
	if v := el.Attr("width"); v != "" {
		bodyLines = append(bodyLines, ":figwidth: "+v)
	}
	if v := el.Attr("class"); v != "" {
		bodyLines = append(bodyLines, ":figclass: "+v)
	}
	if v := el.Attr("name"); v != "" {
		bodyLines = append(bodyLines, ":figname: "+v)
	}
	if v := el.Attr("align"); v != "" {
		bodyLines = append(bodyLines, ":align: "+v)
	}
	var contentParts []string
	if caption != nil {
		if t := strings.TrimSpace(doctree.AsText(caption)); t != "" {
			contentParts = append(contentParts, t)
		}
	}
	if legend != nil {
		// The legend is a CONTAINER of block children, unlike the
		// caption, which is a paragraph -- so it needs the block
		// renderer or its own list comes out flattened.
		if t := strings.TrimSpace(rawChildren(legend)); t != "" {
			contentParts = append(contentParts, t)
		}
	}
	return rawDirectiveSource(header, bodyLines, strings.Join(contentParts, "\n\n"))
}

// rawMeta reconstructs ".. meta::" (docutils/rst v0.30.0+) as literal
// reST source — richdoc has no HTML/head-metadata concept at all
// (Document.Meta is keyed by field NAME for docinfo-derived values, a
// different concept even though the shape looks similar). A <meta>
// element's own attributes carry no ordering or "which one was the
// marker's own bare first token vs. an extra key=value token" signal —
// this project's doctree.Element.Attrs is a flat, unordered map — so
// this reconstructs a semantically equivalent, deterministic marker
// (sorted keys) rather than a byte-identical one, the same accepted
// cost as every other function in this file: prefer "name", then
// "http-equiv", then any other single attribute (alphabetically first)
// as the marker's OWN primary token; every remaining attribute becomes
// a trailing "key=value" token on the same marker line.
func rawMeta(el *doctree.Element) string {
	content := el.Attr("content")
	var keys []string
	for k := range el.Attrs {
		if k != "content" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	primary := ""
	for _, candidate := range []string{"name", "http-equiv"} {
		for _, k := range keys {
			if k == candidate {
				primary = k
				break
			}
		}
		if primary != "" {
			break
		}
	}
	if primary == "" && len(keys) > 0 {
		primary = keys[0]
	}
	var tokens []string
	if primary != "" {
		if primary == "name" {
			tokens = append(tokens, el.Attrs[primary])
		} else {
			tokens = append(tokens, primary+"="+el.Attrs[primary])
		}
	}
	for _, k := range keys {
		if k == primary {
			continue
		}
		tokens = append(tokens, k+"="+el.Attrs[k])
	}
	marker := ":" + strings.Join(tokens, " ") + ":"
	line := marker
	if content != "" {
		line += " " + content
	}
	return ".. meta::\n\n" + indentBlock(line)
}

func rawFieldList(el *doctree.Element) string {
	var lines []string
	for _, c := range el.Children {
		field, ok := c.(*doctree.Element)
		if !ok || field.Tag != doctree.TagField {
			continue
		}
		var name, body string
		for _, fc := range field.Children {
			fe, ok := fc.(*doctree.Element)
			if !ok {
				continue
			}
			switch fe.Tag {
			case doctree.TagFieldName:
				name = inlineSourceOf(fe)
			case doctree.TagFieldBody:
				body = rawBlockBody(fe)
			}
		}
		line := ":" + name + ":"
		if body != "" {
			line = hangUnder(line+" ", body, "   ")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func rawDefinitionList(el *doctree.Element) string {
	var parts []string
	for _, c := range el.Children {
		item, ok := c.(*doctree.Element)
		if !ok || item.Tag != doctree.TagDefinitionListItem {
			continue
		}
		var term, def string
		// docutils/rst v0.38.0+ -- a term may be followed by one or more
		// <classifier> siblings ("term : classifier"); each one is
		// rejoined onto the term line with the same " : " delimiter
		// docutils/rst's own splitTermClassifiers split on, or its own
		// content would silently vanish from the reconstructed source.
		for _, ic := range item.Children {
			ie, ok := ic.(*doctree.Element)
			if !ok {
				continue
			}
			switch ie.Tag {
			case doctree.TagTerm:
				term = inlineSourceOf(ie)
			case doctree.TagClassifier:
				term += " : " + inlineSourceOf(ie)
			case doctree.TagDefinition:
				def = rawBlockBody(ie)
			}
		}
		parts = append(parts, hangUnder(term+"\n    ", def, "    "))
	}
	return strings.Join(parts, "\n\n")
}

// rawOptionList reconstructs a man-page-style option list ("-f, --file=ARG
// Description."). richdoc has no node for it at all (it's rarer even than
// field/definition lists, which is why docutils/rst itself deferred it
// initially — see that repo's rst/fieldlist.go), so like those two it falls
// back to a RawBlock; the description goes through rawBlockBody, so a
// multi-paragraph one keeps its paragraphs.
func rawOptionList(el *doctree.Element) string {
	var lines []string
	for _, c := range el.Children {
		item, ok := c.(*doctree.Element)
		if !ok || item.Tag != doctree.TagOptionListItem {
			continue
		}
		var marker, desc string
		for _, ic := range item.Children {
			ie, ok := ic.(*doctree.Element)
			if !ok {
				continue
			}
			switch ie.Tag {
			case doctree.TagOptionGroup:
				marker = rawOptionGroup(ie)
			case doctree.TagDescription:
				desc = rawBlockBody(ie)
			}
		}
		line := marker
		if desc != "" {
			// Continuation lines align under the description's own
			// column, which is where the marker and its two separating
			// spaces end.
			line = hangUnder(marker+"  ", desc, strings.Repeat(" ", len([]rune(marker))+2))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func rawOptionGroup(group *doctree.Element) string {
	var opts []string
	for _, c := range group.Children {
		opt, ok := c.(*doctree.Element)
		if !ok || opt.Tag != doctree.TagOption {
			continue
		}
		opts = append(opts, rawOption(opt))
	}
	return strings.Join(opts, ", ")
}

// rawOption reconstructs one "-f", "-f ARG", "--file=ARG" flag/argument
// pair, its delimiter (" ", "=", or "" for the embedded "-ovalue" form)
// read directly off the option_argument element, the same attribute
// docutils/rst's own optionNode sets.
func rawOption(opt *doctree.Element) string {
	var flag, arg, delim string
	for _, c := range opt.Children {
		ce, ok := c.(*doctree.Element)
		if !ok {
			continue
		}
		switch ce.Tag {
		case doctree.TagOptionString:
			flag = doctree.AsText(ce)
		case doctree.TagOptionArgument:
			arg = doctree.AsText(ce)
			delim = ce.Attr("delimiter")
		}
	}
	if arg == "" {
		return flag
	}
	return flag + delim + arg
}

func rawLineBlock(el *doctree.Element) string {
	return strings.Join(rawLineBlockLines(el, 0), "\n")
}

// rawLineBlockLines walks a (possibly nested, docutils/rst v0.11.0+)
// line_block, reconstructing each line with enough extra leading space
// after "| " to preserve its nesting depth relative to its siblings on
// reparse — nestLineBlockSegment (docutils/rst's lineblock.go) only cares
// about the RELATIVE indent between sibling lines, not the exact column
// the original source used, so this doesn't need to match byte-for-byte.
func rawLineBlockLines(el *doctree.Element, depth int) []string {
	var lines []string
	for _, c := range el.Children {
		ce, ok := c.(*doctree.Element)
		if !ok {
			continue
		}
		switch ce.Tag {
		case doctree.TagLine:
			lines = append(lines, "| "+strings.Repeat("  ", depth)+strings.TrimSpace(inlineSourceOf(ce)))
		case doctree.TagLineBlock:
			lines = append(lines, rawLineBlockLines(ce, depth+1)...)
		}
	}
	return lines
}

// rawRole rebuilds ":role:`text`" from a role's parsed CONTENT, so the
// text has to be re-escaped on the way back out: it is content here and
// source there.
//
// Two characters change meaning inside the backquotes. A backslash is
// reST's escape, so content "PC\python" written literally re-parses as
// "PCpython" -- and writing THAT again loses nothing more, which is why
// the damage compounds silently across round trips rather than showing
// up as an error. A backquote CLOSES the role, so content "a`b" written
// literally ends the construct early and the rest becomes ordinary
// text.
//
// Nothing else needs it: "*", "|" and "_" are inert inside a role's
// backquotes, and were checked rather than assumed.
func rawRole(role, text string) string {
	var b strings.Builder
	b.Grow(len(role) + len(text) + 4)
	b.WriteString(":" + role + ":`")
	for _, r := range text {
		if r == '\\' || r == '`' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteString("`")
	return b.String()
}

// rawNoteRef reconstructs a footnote/citation reference marker for one this
// package could not resolve to a definition — most often reST's own
// auto-numbered `[#]_`/symbol `[*]_` forms, which docutils/rst's README
// documents as never assigned a refname by that engine.
func rawNoteRef(el *doctree.Element) string {
	switch {
	case el.Attr("auto") == "*":
		return "[*]_"
	case el.Attr("auto") == "1":
		if name := el.Attr("refname"); name != "" {
			return "[#" + name + "]_"
		}
		return "[#]_"
	default:
		return "[" + el.Attr("refname") + "]_"
	}
}

// isSyntheticFootnoteName reports whether name looks like one of
// docutils/rst v0.7.0+'s own synthetic "footnote-N" names — assigned to a
// footnote that was originally UNNAMED ("[#]_"), purely so its reference
// can resolve through the same refname-based mechanism a genuinely named
// one uses (see docutils/rst's resolveFootnoteNumbers). An orphan
// definition (this file's whole reason to exist: one no reference ever
// resolved to) carries that synthetic name unconditionally, so
// rawFootnoteDef can't tell "originally named" from "originally anonymous"
// by checking for a non-empty name attribute alone, the way every other
// case here does — checking the shape of the name itself is the only
// signal available. A user-authored footnote whose REAL name happens to
// collide with this exact synthetic shape is vanishingly unlikely, and the
// failure mode if it ever happens is cosmetic (a named orphan
// reconstructed as anonymous), not data loss — the body text is untouched
// either way.
func isSyntheticFootnoteName(name string) bool {
	n, ok := strings.CutPrefix(name, "footnote-")
	if !ok || n == "" {
		return false
	}
	for _, r := range n {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// rawFootnoteDef reconstructs an orphan footnote/citation definition (one
// convertNoteRef never resolved a reference to) using the same label rules
// docutils/rst's own parseFootnoteOrCitation uses in reverse.
func rawFootnoteDef(el *doctree.Element) string {
	var label string
	switch {
	case el.Tag == doctree.TagFootnote && el.Attr("auto") == "*":
		label = "*"
	case el.Tag == doctree.TagFootnote && el.Attr("auto") == "1":
		if n := el.Attr("name"); n != "" && !isSyntheticFootnoteName(n) {
			label = "#" + n
		} else {
			label = "#"
		}
	default:
		label = labelText(el)
	}
	header := ".. [" + label + "]"
	body := rawBlockBody(el)
	if body == "" {
		return header
	}
	// The marker's own line carries the first line of the body; every
	// line after it is indented under it, which is what makes a second
	// paragraph part of the FOOTNOTE rather than a sibling of it.
	return hangUnder(header+" ", body, "   ")
}

// hangUnder puts the first line of body after marker and indents every
// line after it, which is what makes a second paragraph part of the
// construct rather than a sibling of it. A blank line stays blank:
// trailing spaces on it would be a change in the text.
func hangUnder(marker, body, indent string) string {
	lines := strings.Split(body, "\n")
	out := marker + lines[0]
	for _, l := range lines[1:] {
		if l == "" {
			out += "\n"
			continue
		}
		out += "\n" + indent + l
	}
	return out
}

// rawBlockBody reconstructs a container's block children as reST,
// skipping a <label> (whatever marker introduces the container already
// carries it) and any <system_message> (a diagnostic about the source
// is not part of the source).
//
// It goes through rawChildSource, like every other reconstruction in
// this file since v0.99.0. The one it replaced joined doctree.AsText of
// each child with a SPACE, so a two-paragraph footnote came back as one
// paragraph and a list inside one came back as a run-on sentence --
// the same flattening v0.99.0 removed from five other places and left
// here.
func rawBlockBody(el *doctree.Element) string {
	var parts []string
	for _, c := range el.Children {
		if v, ok := c.(*doctree.Element); ok {
			if v.Tag == doctree.TagLabel || v.Tag == doctree.TagSystemMessage {
				continue
			}
		}
		if t := rawChildSource(c); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n")
}

// labelText reads a footnote/citation's rendered [Label] child (present for
// an explicit numeric or citation label, never for an auto "*"/"#" one — see
// docutils/rst's parseFootnoteOrCitation), falling back to the "name"
// attribute when absent.
func labelText(el *doctree.Element) string {
	for _, c := range el.Children {
		if e, ok := c.(*doctree.Element); ok && e.Tag == doctree.TagLabel {
			return doctree.AsText(e)
		}
	}
	return el.Attr("name")
}

// indentContinuation indents every line after the first by 3 spaces (a
// comment's own convention: the first line shares ".. ", the rest align
// under it), matching how docutils reconstructs a wrapped explicit-markup
// block's continuation lines.
func indentContinuation(text string) string {
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			lines[i] = "   " + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

// indentBlock indents every line by 3 spaces, a directive body's own
// convention (distinct from [indentContinuation]: here every line, including
// the first, sits below the ".. name::" header on its own).
func indentBlock(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "   " + l
		}
	}
	return strings.Join(lines, "\n")
}

// rawChildSource renders ONE block child back to reST for the raw*
// helpers above. They all used doctree.AsText, which returns a node's
// text with no structure at all -- so a bullet list inside an
// admonition came out as "onetwo", its items concatenated with neither
// bullets nor separators. The same loss applied to a list inside a
// topic, container, compound or figure, all five having the identical
// contentParts/AsText loop.
//
// Only the structures that a directive body actually tends to hold are
// reconstructed; anything else keeps the AsText behaviour, which is
// correct for a paragraph and honest for the rest. The list-shaped tags
// delegate to the raw* helpers that already existed for them, so there
// is one renderer per construct rather than two.
func rawChildSource(n doctree.Node) string {
	el, ok := n.(*doctree.Element)
	if !ok {
		return strings.TrimSpace(doctree.AsText(n))
	}
	switch el.Tag {
	case doctree.TagBulletList:
		bullet := el.Attr("bullet")
		if bullet == "" {
			bullet = "-"
		}
		return rawListItems(el, func(int) string { return bullet + " " })
	case doctree.TagEnumeratedList:
		// enumtype/prefix/suffix carry the enumerator's own shape; only
		// the arabic form is reconstructed, the others keeping their
		// numbers as written would need the counter docutils stores in
		// "start" plus a numeral converter.
		suffix := el.Attr("suffix")
		if suffix == "" {
			suffix = "."
		}
		start := 1
		if v := el.Attr("start"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				start = n
			}
		}
		return rawListItems(el, func(i int) string {
			return el.Attr("prefix") + strconv.Itoa(start+i) + suffix + " "
		})
	case doctree.TagParagraph:
		// A paragraph inside a reconstructed construct keeps its INLINE
		// markup. The fallback below returns a node's text, so every link,
		// emphasis, literal and role inside a ".. note::" body was lost --
		// PEP 6's own note reads "documented in `the devguide <...>`__" and
		// came back as "documented in the devguide", the link gone. 270 of
		// the 1564 corpus files reach this case; v0.136.x fixed the TITLE
		// paths and left the content one.
		return strings.TrimSpace(inlineSourceOf(el))
	case doctree.TagLiteralBlock:
		// Verbatim on purpose: docutils does not parse markup inside a
		// literal block, so its text IS its source.
		return "::\n\n" + indentBlock(doctree.AsText(el))
	case doctree.TagBlockQuote:
		return indentBlock(rawChildren(el))
	case doctree.TagFieldList:
		return rawFieldList(el)
	case doctree.TagDefinitionList:
		return rawDefinitionList(el)
	case doctree.TagLineBlock:
		return rawLineBlock(el)
	case doctree.TagOptionList:
		return rawOptionList(el)
	case doctree.TagAttention, doctree.TagCaution, doctree.TagDanger,
		doctree.TagErrorAdmonition, doctree.TagHint, doctree.TagImportant,
		doctree.TagNote, doctree.TagTip, doctree.TagWarningAdmonition,
		doctree.TagAdmonition:
		return rawAdmonition(el)
	}
	return strings.TrimSpace(doctree.AsText(el))
}

// rawListItems renders a list's items, marker() giving each its own
// marker. A multi-block item keeps its blocks, indented under the
// marker's own width.
func rawListItems(list *doctree.Element, marker func(int) string) string {
	var out []string
	i := 0
	for _, c := range list.Children {
		item, ok := c.(*doctree.Element)
		if !ok || item.Tag != doctree.TagListItem {
			continue
		}
		m := marker(i)
		body := rawChildren(item)
		lines := strings.Split(body, "\n")
		for k, l := range lines {
			switch {
			case k == 0:
				lines[k] = m + l
			case l == "":
			default:
				lines[k] = strings.Repeat(" ", len(m)) + l
			}
		}
		out = append(out, strings.Join(lines, "\n"))
		i++
	}
	return strings.Join(out, "\n")
}

// rawChildren renders every block child of el, blank-line separated.
func rawChildren(el *doctree.Element) string {
	var parts []string
	for _, c := range el.Children {
		if t := rawChildSource(c); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n")
}
