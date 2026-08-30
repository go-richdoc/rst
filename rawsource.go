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

func rawLineBlock(el *doctree.Element) string {
	var lines []string
	for _, c := range el.Children {
		line, ok := c.(*doctree.Element)
		if !ok || line.Tag != doctree.TagLine {
			continue
		}
		lines = append(lines, "| "+strings.TrimSpace(doctree.AsText(line)))
	}
	return strings.Join(lines, "\n")
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

// rawFootnoteDef reconstructs an orphan footnote/citation definition (one
// convertNoteRef never resolved a reference to) using the same label rules
// docutils/rst's own parseFootnoteOrCitation uses in reverse.
func rawFootnoteDef(el *doctree.Element) string {
	var label string
	switch {
	case el.Tag == doctree.TagFootnote && el.Attr("auto") == "*":
		label = "*"
	case el.Tag == doctree.TagFootnote && el.Attr("auto") == "1":
		if n := el.Attr("name"); n != "" {
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
