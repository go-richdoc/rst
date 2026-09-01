// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strings"

	"github.com/go-docutils/docutils/doctree"
)

// This file reconstructs reST source text for constructs richdoc has no
// native node for (directives, comments, field lists, definition lists, line
// blocks, an unresolved role or note reference, an orphan footnote/citation
// definition). docutils/rst's doctree keeps no byte-offset back-reference
// into the original source for these, so the text below is a resynthesis
// from parsed structure, not a verbatim slice — semantically equivalent
// reST, not necessarily byte-identical to what was typed. Field/definition
// list content is flattened with [flattenBody], which joins each child
// block's text with a single space: multi-paragraph or richly-marked-up
// field content loses its internal structure and inline styling here, an
// accepted cost of routing an unmodeled construct through [flattenText]
// rather than a second full inline-to-reST emitter just for this fallback
// path (see [Write]'s emitter for the one that matters: ordinary body text).

func rawComment(el *doctree.Element) string {
	text := doctree.AsText(el)
	if text == "" {
		return ".."
	}
	return ".. " + indentContinuation(text)
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
			header += " " + doctree.AsText(ce)
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
		if t := strings.TrimSpace(doctree.AsText(c)); t != "" {
			contentParts = append(contentParts, t)
		}
	}
	if len(contentParts) > 0 {
		if len(bodyLines) > 0 {
			bodyLines = append(bodyLines, "")
		}
		bodyLines = append(bodyLines, strings.Join(contentParts, "\n\n"))
	}
	if len(bodyLines) == 0 {
		return header
	}
	return header + "\n\n" + indentBlock(strings.Join(bodyLines, "\n"))
}

// rawTopic reconstructs ".. topic::" or ".. sidebar::" (docutils/rst
// v0.28.0+, el.Tag itself IS the directive name, same convention as
// rawAdmonition) as literal reST source — richdoc has no topic/sidebar
// block type either. Unlike an admonition's title, a topic's title is
// REQUIRED and a sidebar's is optional but may carry its own
// ":subtitle:" (only ever present alongside a title, per
// runTopicOrSidebar's own validation, so no empty-title case to guard
// here). Content is flattened the same lossy way rawAdmonition does.
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
			header += " " + doctree.AsText(ce)
		case doctree.TagSubtitle:
			subtitle = doctree.AsText(ce)
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
		if ce, ok := c.(*doctree.Element); ok && (ce.Tag == doctree.TagTitle || ce.Tag == doctree.TagSubtitle) {
			continue
		}
		if t := strings.TrimSpace(doctree.AsText(c)); t != "" {
			contentParts = append(contentParts, t)
		}
	}
	if len(contentParts) > 0 {
		if len(bodyLines) > 0 {
			bodyLines = append(bodyLines, "")
		}
		bodyLines = append(bodyLines, strings.Join(contentParts, "\n\n"))
	}
	if len(bodyLines) == 0 {
		return header
	}
	return header + "\n\n" + indentBlock(strings.Join(bodyLines, "\n"))
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
	for _, c := range el.Children {
		ce, ok := c.(*doctree.Element)
		if !ok {
			continue
		}
		switch ce.Tag {
		case doctree.TagImage:
			img = ce
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
		if t := strings.TrimSpace(doctree.AsText(legend)); t != "" {
			contentParts = append(contentParts, t)
		}
	}
	if len(contentParts) > 0 {
		if len(bodyLines) > 0 {
			bodyLines = append(bodyLines, "")
		}
		bodyLines = append(bodyLines, strings.Join(contentParts, "\n\n"))
	}
	if len(bodyLines) == 0 {
		return header
	}
	return header + "\n\n" + indentBlock(strings.Join(bodyLines, "\n"))
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
				name = doctree.AsText(fe)
			case doctree.TagFieldBody:
				body = flattenBody(fe)
			}
		}
		line := ":" + name + ":"
		if body != "" {
			line += " " + body
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
		for _, ic := range item.Children {
			ie, ok := ic.(*doctree.Element)
			if !ok {
				continue
			}
			switch ie.Tag {
			case doctree.TagTerm:
				term = doctree.AsText(ie)
			case doctree.TagDefinition:
				def = flattenBody(ie)
			}
		}
		parts = append(parts, term+"\n    "+def)
	}
	return strings.Join(parts, "\n\n")
}

// rawOptionList reconstructs a man-page-style option list ("-f, --file=ARG
// Description."). richdoc has no node for it at all (it's rarer even than
// field/definition lists, which is why docutils/rst itself deferred it
// initially — see that repo's rst/fieldlist.go), so like those two it falls
// back to a RawBlock; the description is flattened the same lossy way (see
// flattenBody) as a field body.
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
				desc = flattenBody(ie)
			}
		}
		line := marker
		if desc != "" {
			line += "  " + desc
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
			lines = append(lines, "| "+strings.Repeat("  ", depth)+strings.TrimSpace(doctree.AsText(ce)))
		case doctree.TagLineBlock:
			lines = append(lines, rawLineBlockLines(ce, depth+1)...)
		}
	}
	return lines
}

func rawRole(role, text string) string {
	return ":" + role + ":`" + text + "`"
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
	if body := flattenBody(el); body != "" {
		return header + " " + body
	}
	return header
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

// flattenBody joins a container's child blocks' text with a single space,
// skipping a leading [doctree.TagLabel] (a footnote/citation's own rendered
// marker, redundant with the ".. [label]" this file already emits).
func flattenBody(el *doctree.Element) string {
	var parts []string
	for _, c := range el.Children {
		switch v := c.(type) {
		case *doctree.Element:
			if v.Tag == doctree.TagLabel {
				continue
			}
			if t := strings.TrimSpace(doctree.AsText(v)); t != "" {
				parts = append(parts, t)
			}
		case *doctree.Text:
			if t := strings.TrimSpace(v.Data); t != "" {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, " ")
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
