// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"regexp"
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
	// The ARGUMENT's continuation lines get the same treatment, and for the same
	// reason one line further up: a directive's argument may run to several lines
	// -- ".. rubric:: This is\n   a multiline rubric" is the shape in the corpus
	// -- and an unindented second line ENDS the directive. docutils then says
	// "Explicit markup ends without a blank line; unexpected unindent." and the
	// rest of the argument becomes a paragraph of its own, which is how the
	// rubric lost half its text.
	header = continuationIndent(header, 3)
	// Every option's continuation lines are indented under its own marker, in
	// ONE place rather than at each of the two dozen call sites that build one.
	// An option VALUE can run to several lines -- an image's ":alt:" is the
	// common case, and PEP 495 has a two-line one -- and a field body's
	// continuation has to be indented or the field ends there: the second line
	// became a NEW option, so the alt text was truncated at the line break and
	// the rest read as an unknown option. The same defect was fixed twice before
	// this, in the line block and in the docinfo field list; fixing it here
	// covers the class instead of the next instance.
	if len(options) > 0 {
		indented := make([]string, len(options))
		for i, o := range options {
			indented[i] = continuationIndent(o, 3)
		}
		options = indented
	}
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

// rawTable rebuilds a <table> as a reST grid table, through the converter and
// writer this package already has for one.
func rawTable(el *doctree.Element) string {
	c := &converter{
		footnoteDefs:  map[string]*doctree.Element{},
		substDefs:     map[string]*doctree.Element{},
		referenced:    map[string]bool{},
		consumed:      map[string]bool{},
		headingAnchor: map[string]string{},
		anchorAlias:   map[string]string{},
	}
	return (&writer{}).writeTable(c.convertTable(el))
}

func rawDirective(el *doctree.Element) string {
	header := ".. " + el.Attr("name") + "::"
	if args := el.Attr("arguments"); args != "" {
		// No continuationIndent here, deliberately: this parser puts only the
		// MARKER LINE's remainder in "arguments" and everything under it in the
		// body, so the argument is never more than one line. Checked rather than
		// assumed -- ".. py:function:: f(a,\n                 b)" arrives as
		// arguments="f(a," with "b)" as body text.
		header += " " + args
	}
	body := doctree.AsText(el)
	if body == "" {
		return header
	}
	// NO blank line between the marker and the body, and that is not a style
	// choice. A <directive> this parser did not implement keeps its whole block
	// as text, OPTION LINES INCLUDED -- an unknown directive has no option_spec
	// to split them off with -- and an option block has to follow the marker
	// immediately. Asked about both spellings, the reference answers
	//
	//	.. image:: a.png      -> <image alt="x" uri="a.png">
	//	   :alt: x
	//
	//	.. image:: a.png      -> nothing at all
	//
	//	   :alt: x
	//
	// so a blank line turns every option into content. For a directive with only
	// content the two spellings are identical (".. note::" with and without one
	// gives the same <note>), so dropping the blank line costs nothing and keeps
	// the options.
	return header + "\n" + indentBlock(body)
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
	if src, ok := rawContents(el); ok {
		return src
	}
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
	bodyLines := imageOptionLines(img, target)
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
		// inlineSourceOf, not AsText: a caption is a paragraph and carries
		// inline markup like any other. AsText returns a node's TEXT, so every
		// link, literal, emphasis and role in a figure's caption was lost -- and
		// a footnote reference came out as its bare label, "Cites 1." for
		// "Cites [1]_.", which also stopped the note's definition from being
		// emitted at all. The same fix the other raw* content paths already got.
		if t := strings.TrimSpace(inlineSourceOf(caption)); t != "" {
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
				term = escapeClassifierDelimiter(inlineSourceOf(ie))
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

// reClassifierDelimiter is the reference's own `classifier_delimiter`,
// `re.compile(' +: +')` (states.Text), which splits a definition-list term into a
// term and its classifiers.
var reClassifierDelimiter = regexp.MustCompile(` +: +`)

// escapeClassifierDelimiter escapes a colon inside a TERM's own text, so a term that
// contains " : " does not come back split into a term and a classifier.
//
// PEP 362 writes "* return_annotation \: object" for exactly this reason: escaped,
// the colon stays part of the term. The escape works because docutils parses the
// term's inline content FIRST, which turns "\:" into NUL+":" -- and the delimiter
// pattern " +: +" cannot match across the NUL. Writing the term back with a bare
// colon therefore produced <term>return_annotation</term> plus
// <classifier>object</classifier> where the source had one term.
//
// Only the term needs it. A classifier cannot contain the delimiter: if it did, the
// parse that produced it would have split there.
func escapeClassifierDelimiter(term string) string {
	return reClassifierDelimiter.ReplaceAllStringFunc(term, func(m string) string {
		i := strings.IndexByte(m, ':')
		return m[:i] + `\:` + m[i+1:]
	})
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
			prefix := "| " + strings.Repeat("  ", depth)
			lines = append(lines, prefix+continuationIndent(strings.TrimSpace(inlineSourceOf(ce)), len(prefix)))
		case doctree.TagLineBlock:
			lines = append(lines, rawLineBlockLines(ce, depth+1)...)
		}
	}
	return lines
}

// continuationIndent indents every line of text after the first by width
// spaces, so a line block LINE that wrapped in the source stays one line on
// reparse.
//
// One <line> can span several source lines: docutils reads
//
//	| ``__setitem__(integer | slice, integer) ->
//	  None``
//
// as a single line whose literal contains a newline -- confirmed against the
// reference, which gives one <line> with one <literal> -- and it is the INDENT
// that says so. Written at column 0 the line block simply ends there, which
// cost PEP 368 twenty-one diagnostics in its reconstruction: three per wrapped
// line, "Line block ends without a blank line" plus the unterminated literal
// and emphasis the break left behind. None of them was visible to the
// round-trip probe, whose answer for that file was already "no".
func continuationIndent(text string, width int) string {
	if !strings.Contains(text, "\n") {
		return text
	}
	pad := strings.Repeat(" ", width)
	lines := strings.Split(text, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = pad + strings.TrimLeft(lines[i], " ")
	}
	return strings.Join(lines, "\n")
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
// rawMathRole writes a ":math:" role VERBATIM, which is the whole of it: math is
// one of exactly three roles whose content keeps its backslashes.
//
// docutils' inline parser replaces every escaping backslash with a NUL and each
// role decides what to do with them; only "raw", "code" and "math" call
// nodes.unescape(text, True) -- "return a string with nulls ... restored to
// backslashes" (docutils/nodes.py and parsers/rst/roles.py, read for this). Every
// other role, ":literal:" and ":sub:" included, drops them: asked about
// ":literal:`\emptyset`" the reference answers "<literal>emptyset", and about
// ":math:`\emptyset`" it answers "<math>\emptyset".
//
// So writing a formula through rawRole, which escapes a backslash for a role in
// general, doubled every TeX command in it: ":math:`\emptyset`" came back as
// ":math:`\\emptyset`", TeX for a line break followed by a word. 16 formulas in 5
// of the 14 corpus files that hold one.
//
// Escaping the BACKTICK would be wrong for the same reason -- the added backslash
// survives the restore, so "a\`b" came back "a\\`b" -- and leaving it alone is
// also what round-trips: the backslash an author already wrote in front of it is
// what keeps the role from ending there, and it comes back unchanged.
//
// Two shapes cannot be written at all, here or in docutils: a formula holding a
// BARE backtick, and one ENDING in a lone backslash. Either way the role's closing
// backtick is consumed and there is no spelling that avoids it. Neither is valid
// TeX in the first place, and the corpus has none.
func rawMathRole(tex string) string {
	return ":math:`" + tex + "`"
}

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
		//
		// The LANGUAGE has to come with it. This path wrote a bare "::" for
		// every one, so a ".. code:: python" nested inside a list item, an
		// admonition or a definition came back as an unlabelled literal block --
		// 16 corpus files, and invisible while the class was being subtracted
		// from the comparison. writeCodeBlock's own rule is reused rather than
		// restated: directive when there is something to carry, "::" when there
		// is not.
		if lang := codeLanguage(el); lang != "" || len(authorClasses(el)) > 0 {
			var opts []string
			if cs := authorClasses(el); len(cs) > 0 {
				opts = append(opts, ":class: "+strings.Join(cs, " "))
			}
			return rawDirectiveSource(".. code:: "+lang, opts, indentBlock(doctree.AsText(el)))
		}
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
	// There is deliberately NO case for a topic or a sidebar here. docutils
	// refuses one nested in a body element at all -- ".. topic:: X" inside a
	// ".. container::" parses to `The "topic" directive may not be used within
	// topics or body elements.` -- so the case was dead code, which is what the
	// coverage floor caught after the corpus had nothing to say either way.
	case doctree.TagFigure:
		return rawFigure(el)
	case doctree.TagImage:
		return rawImage(el)
	case doctree.TagReference:
		// A block-level <reference> wrapping an image is the ":target:" form of
		// an ".. image::" -- see rawImage. Any other reference at block level is
		// not something this path produces.
		if src := rawImage(el); src != "" {
			return src
		}
		return strings.TrimSpace(inlineSourceOf(el))
	case doctree.TagComment:
		// Without this the fallback returned the comment's TEXT, so a comment
		// nested in a container or an admonition stopped being a comment:
		// sphinx's own index page comments out a whole admonition
		// (".. .. admonition:: 🌐 Integration with Version Control"), and the
		// reconstruction wrote it back as a REAL directive with no body --
		// "Content block expected for the \"admonition\" directive; none
		// found." A commented-out construct came back switched on.
		return rawComment(el)
	case doctree.TagRubric:
		return rawRubric(el)
	case doctree.TagContainer:
		return rawContainer(el)
	case doctree.TagTable:
		// A table nested in a construct rebuilt as source -- a simple table inside
		// a DEFINITION is the shape in the corpus -- fell to AsText, which is a run
		// of the cells' words with no table around them. PEP 249 put all three of
		// its tables in definitions and lost every one: 34 entries, 18 rows and 8
		// colspecs in that file alone.
		//
		// Rebuilt through the same path a top-level table takes, rather than a
		// second grid writer: the throwaway converter/writer pair inlineSourceOf
		// already uses for the same reason, so a cell's own blocks (richdoc
		// v0.5.0) and a caption come with it.
		return rawTable(el)
	case doctree.TagDirective:
		// A directive this parser has no implementation for keeps its name, its
		// argument and its whole block as text, and nothing else here can put
		// those back together. Without this case it fell to AsText: a bare
		// ".. versionadded:: 1.8" nested in a definition has NO text, so it
		// disappeared completely, and one with content lost its marker line and
		// became a paragraph. 95 directives in 9 files -- 77 of them in sphinx's
		// own latex.rst, which nests them in definitions throughout.
		return rawDirective(el)
	case doctree.TagRaw:
		// A raw block's content is markup for its own target format, and this
		// is the directive that says so. The fallback emitted the content bare,
		// which for "raw:: html" put tags into the reST as if the author had
		// typed them.
		return rawDirectiveSource(".. raw:: "+el.Attr("format"), nil, doctree.AsText(el))
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

// rawContents recognises the <topic> docutils builds for ".. contents::" and
// writes that directive back, rather than the ".. topic::" the generic path
// produced.
//
// The generic path skipped the <pending> child (correctly -- its text is
// docutils' own ".. internal attributes:" block and must never reach a reader)
// and so emitted a topic with a TITLE and NO CONTENT, which docutils rejects:
// "Content block expected for the \"topic\" directive; none found." Four of the
// five diagnostics the reconstruction still introduced were this one, in three
// spellings -- the contentless topic, the same with no title at all ("1
// argument(s) required, 0 supplied"), and an invented ":name:" option taken
// from the implicit target the TITLE created, which ".. contents::" does not
// even accept.
//
// The options come back out of the pending's own details, and only the ones
// that can be read unambiguously. docutils' Contents.option_spec (read
// directly) is backlinks/class/depth/local, and the tree distinguishes the two
// spellings that both mean "no backlinks": ":backlinks: none" leaves
// "backlinks: None" and a bare ":backlinks:" leaves "backlinks: ”". The
// "contents" and "local" CLASSES are not written back, because docutils derives
// them from the directive and from :local: itself -- writing them as an
// explicit :class: would duplicate them on the next parse.
func rawContents(el *doctree.Element) (string, bool) {
	var details string
	for _, c := range el.Children {
		ce, ok := c.(*doctree.Element)
		if !ok || ce.Tag != doctree.TagPending {
			continue
		}
		t := doctree.AsText(ce)
		if strings.Contains(t, "docutils.transforms.parts.Contents") {
			details = t
			break
		}
	}
	if details == "" {
		return "", false
	}
	header := ".. contents::"
	for _, c := range el.Children {
		if ce, ok := c.(*doctree.Element); ok && ce.Tag == doctree.TagTitle {
			header += " " + inlineSourceOf(ce)
			break
		}
	}
	var options []string
	if v, ok := pendingDetail(details, "depth"); ok {
		options = append(options, ":depth: "+v)
	}
	if v, ok := pendingDetail(details, "local"); ok && v == "None" {
		options = append(options, ":local:")
	}
	if v, ok := pendingDetail(details, "backlinks"); ok {
		switch v {
		case "None":
			options = append(options, ":backlinks: none")
		case "''":
			options = append(options, ":backlinks:")
		default:
			options = append(options, ":backlinks: "+strings.Trim(v, "'"))
		}
	}
	// A directive with options and no content is legal for ".. contents::",
	// whose content_spec takes none -- which is the whole point of writing this
	// directive rather than a topic.
	return rawDirectiveSource(header, options, ""), true
}

// pendingDetail reads one "  key: value" line out of a <pending>'s own
// ".. internal attributes:" text.
func pendingDetail(details, key string) (string, bool) {
	for _, l := range strings.Split(details, "\n") {
		l = strings.TrimSpace(l)
		if v, ok := strings.CutPrefix(l, key+": "); ok {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

// imageOptionLines builds the ":alt:"/":height:"/… lines an <image> carries, in
// the reference's own order -- images.Image.option_spec, read from docutils, not
// alphabetical -- with ":target:" in its place among them rather than appended
// after. Shared by rawFigure and rawImage so a figure's image and a standalone one
// cannot drift apart.
//
// "align" is in the list for the standalone case: the reference puts a figure's
// align on the <figure> (rawFigure writes it from there) and a lone image's on the
// <image> itself, so leaving it out here dropped it.
func imageOptionLines(img *doctree.Element, target string) []string {
	var out []string
	for _, opt := range []string{"alt", "height", "width", "scale", "align", "target", "loading", "class", "name"} {
		v := ""
		if opt == "target" {
			v = target
		} else if img != nil {
			v = img.Attr(opt)
		}
		if v != "" {
			out = append(out, ":"+opt+": "+v)
		}
	}
	return out
}

// rawImage reconstructs ".. image:: URI" for an image nested inside a construct
// that is itself rebuilt as reST source.
//
// rawChildSource had no case for an image OR a figure, so either one nested in a
// container, an admonition, a list item or a definition fell to its AsText
// fallback -- and an <image> has no text, so the picture, every option and the
// ":target:" link all vanished, leaving at most a caption. sphinx's own index page
// puts three logos in a ".. container::", each a figure with a ":target:", and all
// three links disappeared; 6 figures and 3 standalone images across 4 corpus files.
//
// An image wrapped in a <reference> is the ":target:" form, exactly as in
// rawFigure: the <image> is then a GRANDCHILD, which is why the caller passes the
// image it found rather than this reaching for it.
func rawImage(el *doctree.Element) string {
	img, target := el, ""
	if el.Tag == doctree.TagReference {
		target = rawImageTarget(el)
		for _, c := range el.Children {
			if ce, ok := c.(*doctree.Element); ok && ce.Tag == doctree.TagImage {
				img = ce
			}
		}
	}
	if img.Tag != doctree.TagImage {
		return ""
	}
	return rawDirectiveSource(".. image:: "+img.Attr("uri"), imageOptionLines(img, target), "")
}
