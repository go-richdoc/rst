// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strings"

	"github.com/go-docutils/docutils/doctree"
	docrst "github.com/go-docutils/docutils/rst"
	"github.com/go-richdoc/richdoc"
)

// Parse converts reStructuredText source into a [richdoc.Document]. It never
// returns an error: docutils/rst.Parse has no failure mode of its own (an
// unrecognized construct degrades to plain text, matching docutils' own
// tolerant parsing philosophy), so the error return exists only for symmetry
// with [Write] and the other go-richdoc converters.
func Parse(src []byte) (*richdoc.Document, error) {
	doc := docrst.Parse(string(src))
	c := &converter{
		footnoteDefs: map[string]*doctree.Element{},
		substDefs:    map[string]*doctree.Element{},
	}
	c.collect(doc)

	children := doc.Children
	meta := map[string]string{}
	if len(children) > 0 {
		if fl, ok := leadingFieldList(children[0]); ok {
			meta = fieldsToMeta(fl)
			children = children[1:]
		}
	}

	blocks := c.convertBlocks(children, 1)
	d := &richdoc.Document{Blocks: blocks}
	if len(meta) > 0 {
		d.Meta = meta
	}
	return d, nil
}

// leadingFieldList reports whether n is a top-level field list, the
// convention this package uses (matching [Write]) to round-trip
// [richdoc.Document.Meta]: any OTHER field list, not the document's very
// first block, falls back to [richdoc.RawBlock] instead (see convertBlockNode).
func leadingFieldList(n doctree.Node) (*doctree.Element, bool) {
	el, ok := n.(*doctree.Element)
	if !ok || el.Tag != doctree.TagFieldList {
		return nil, false
	}
	return el, true
}

// fieldsToMeta reads a field list's name/body pairs into a plain map, using
// the body's flattened text (see [package doc] for why field content isn't
// rendered with full inline fidelity here).
func fieldsToMeta(fl *doctree.Element) map[string]string {
	meta := map[string]string{}
	for _, c := range fl.Children {
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
				body = strings.TrimSpace(doctree.AsText(fe))
			}
		}
		if name != "" {
			meta[name] = body
		}
	}
	return meta
}

// converter carries parse-wide state: the footnote/citation definitions
// (keyed by their "name" attribute) and substitution definitions collected
// by [converter.collect], and the set of names actually consumed while
// converting, so an orphan definition (referenced by nothing) is still
// preserved via [richdoc.RawBlock] rather than silently dropped.
type converter struct {
	footnoteDefs map[string]*doctree.Element
	substDefs    map[string]*doctree.Element
	consumed     map[string]bool
}

// collect walks the whole tree once, before conversion proper, gathering
// every footnote/citation and substitution definition by name. A second pass
// (the actual conversion) then inlines a definition at each reference site
// that names it, mirroring how [github.com/go-richdoc/markdown]'s Parse
// indexes goldmark's trailing FootnoteList before walking the document body.
func (c *converter) collect(n doctree.Node) {
	el, ok := n.(*doctree.Element)
	if !ok {
		return
	}
	switch el.Tag {
	case doctree.TagFootnote, doctree.TagCitation:
		if name := el.Attr("name"); name != "" {
			c.footnoteDefs[name] = el
		}
	case doctree.TagSubstitutionDef:
		// Substitution names are case-SENSITIVE in docutils, unlike a
		// hyperlink target's or footnote's name (see explicit.go's
		// normalizeWhitespace doc comment), so this key is used as-is.
		if name := el.Attr("substitution"); name != "" {
			c.substDefs[name] = el
		}
	}
	for _, ch := range el.Children {
		c.collect(ch)
	}
}

// convertBlocks converts a sequence of doctree nodes to a flat block list.
func (c *converter) convertBlocks(nodes []doctree.Node, level int) []richdoc.Block {
	var out []richdoc.Block
	for _, n := range nodes {
		out = append(out, c.convertBlockNode(n, level)...)
	}
	return out
}

// convertBlockNode converts a single doctree node to zero, one, or (for a
// section, whose title and body flatten into the surrounding sequence) many
// richdoc blocks.
func (c *converter) convertBlockNode(n doctree.Node, level int) []richdoc.Block {
	el, ok := n.(*doctree.Element)
	if !ok {
		return nil
	}
	switch el.Tag {
	case doctree.TagSection:
		return c.convertSection(el, level)
	case doctree.TagParagraph:
		return []richdoc.Block{richdoc.Paragraph{Inlines: c.convertInlines(el.Children)}}
	case doctree.TagBulletList:
		return []richdoc.Block{c.convertList(el, false)}
	case doctree.TagEnumeratedList:
		return []richdoc.Block{c.convertList(el, true)}
	case doctree.TagBlockQuote:
		return []richdoc.Block{richdoc.BlockQuote{Blocks: c.convertBlocks(el.Children, level)}}
	case doctree.TagTransition:
		return []richdoc.Block{richdoc.ThematicBreak{}}
	case doctree.TagLiteralBlock, doctree.TagDoctestBlock:
		return []richdoc.Block{richdoc.CodeBlock{Text: doctree.AsText(el)}}
	case doctree.TagComment:
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawComment(el)}}
	case doctree.TagDirective:
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawDirective(el)}}
	case doctree.TagFieldList:
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawFieldList(el)}}
	case doctree.TagDefinitionList:
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawDefinitionList(el)}}
	case doctree.TagLineBlock:
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawLineBlock(el)}}
	case doctree.TagOptionList:
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawOptionList(el)}}
	case doctree.TagTable:
		return []richdoc.Block{c.convertTable(el)}
	case doctree.TagTarget, doctree.TagSubstitutionDef:
		// Invisible bookkeeping nodes: a hyperlink target's references
		// already carry a resolved refuri directly (see the rst package's
		// own resolveTargets), and a substitution definition's value is
		// inlined at each reference by convertInlineNode. Neither has
		// visible content of its own once that resolution has happened.
		return nil
	case doctree.TagFootnote, doctree.TagCitation:
		// Emitted inline at each resolvable reference (convertInlineNode);
		// reaching this case means the definition was never referenced, so
		// it is preserved here rather than silently dropped.
		if name := el.Attr("name"); name != "" && c.consumed[name] {
			return nil
		}
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawFootnoteDef(el)}}
	default:
		return c.convertBlocks(el.Children, level)
	}
}

// convertSection flattens a section into its title (a Heading at level,
// clamped to richdoc's 1..6 range) followed by its remaining content one
// level deeper — richdoc has no Section wrapper, matching both
// [github.com/go-richdoc/markdown] and [github.com/go-richdoc/latex].
func (c *converter) convertSection(el *doctree.Element, level int) []richdoc.Block {
	var out []richdoc.Block
	for _, ch := range el.Children {
		if title, ok := ch.(*doctree.Element); ok && title.Tag == doctree.TagTitle {
			out = append(out, richdoc.Heading{Level: clampLevel(level), Inlines: c.convertInlines(title.Children)})
		}
	}
	for _, ch := range el.Children {
		if title, ok := ch.(*doctree.Element); ok && title.Tag == doctree.TagTitle {
			continue
		}
		out = append(out, c.convertBlockNode(ch, level+1)...)
	}
	return out
}

// clampLevel caps a section-nesting depth at richdoc's 1..6 Heading range.
// convertSection only ever calls this with level >= 1 (Parse starts at 1 and
// only increments), so there is no "too low" case to guard against.
func clampLevel(level int) int {
	if level > 6 {
		return 6
	}
	return level
}

// convertList converts a bullet_list or enumerated_list. Tight mirrors the
// CommonMark convention richdoc borrows: true when every item's content is a
// single paragraph. docutils' own doctree carries no start-number attribute
// for enumerated lists (see rst/parser.go), so Start is always 1 on Parse;
// [Write] still honours a non-1 Start it receives from elsewhere (richdoc is a
// hub other converters build documents for, not just this package's own
// round-trip), which is why that direction is documented as one-way.
func (c *converter) convertList(el *doctree.Element, ordered bool) richdoc.List {
	l := richdoc.List{Ordered: ordered, Start: 1, Tight: true}
	for _, ch := range el.Children {
		item, ok := ch.(*doctree.Element)
		if !ok || item.Tag != doctree.TagListItem {
			continue
		}
		blocks := c.convertBlocks(item.Children, 1)
		if len(blocks) != 1 {
			l.Tight = false
		} else if _, ok := blocks[0].(richdoc.Paragraph); !ok {
			l.Tight = false
		}
		l.Items = append(l.Items, richdoc.ListItem{Blocks: blocks})
	}
	return l
}

// convertTable converts a simple or grid table's thead/tbody rows. richdoc's
// [richdoc.Table] has no colspan/rowspan primitive, matching plain Markdown
// tables, so a grid-table cell's span (see the docutils README) collapses to
// its own cell — a real, documented gap, not silent corruption: the spanned
// cell's own text still appears, just no longer merged.
func (c *converter) convertTable(el *doctree.Element) richdoc.Table {
	var t richdoc.Table
	for _, ch := range el.Children {
		group, ok := ch.(*doctree.Element)
		if !ok {
			continue
		}
		switch group.Tag {
		case doctree.TagThead:
			for _, rc := range group.Children {
				if row, ok := rc.(*doctree.Element); ok && row.Tag == doctree.TagRow {
					t.Header = c.convertRow(row)
					if len(t.Align) < len(t.Header) {
						t.Align = make([]richdoc.Alignment, len(t.Header))
					}
				}
			}
		case doctree.TagTbody:
			for _, rc := range group.Children {
				if row, ok := rc.(*doctree.Element); ok && row.Tag == doctree.TagRow {
					t.Rows = append(t.Rows, c.convertRow(row))
				}
			}
		}
	}
	return t
}

func (c *converter) convertRow(row *doctree.Element) []richdoc.Cell {
	var cells []richdoc.Cell
	for _, ch := range row.Children {
		entry, ok := ch.(*doctree.Element)
		if !ok || entry.Tag != doctree.TagEntry {
			continue
		}
		cells = append(cells, richdoc.Cell{Inlines: c.convertInlines(entry.Children)})
	}
	return cells
}

// convertInlines converts a sequence of doctree nodes to richdoc inlines,
// coalescing adjacent literal text the same way
// [github.com/go-richdoc/markdown]'s Parse does, so this package's own
// tokenisation choices don't leak into the model.
func (c *converter) convertInlines(nodes []doctree.Node) []richdoc.Inline {
	var out []richdoc.Inline
	for _, n := range nodes {
		for _, in := range c.convertInlineNode(n) {
			if t, ok := in.(richdoc.Text); ok && len(out) > 0 {
				if prev, ok := out[len(out)-1].(richdoc.Text); ok {
					out[len(out)-1] = richdoc.Text{Value: prev.Value + t.Value}
					continue
				}
			}
			out = append(out, in)
		}
	}
	return out
}

func (c *converter) convertInlineNode(n doctree.Node) []richdoc.Inline {
	switch v := n.(type) {
	case *doctree.Text:
		if v.Data == "" {
			return nil
		}
		return []richdoc.Inline{richdoc.Text{Value: v.Data}}
	case *doctree.Element:
		return c.convertInlineElement(v)
	}
	return nil
}

func (c *converter) convertInlineElement(el *doctree.Element) []richdoc.Inline {
	switch el.Tag {
	case doctree.TagEmphasis:
		return []richdoc.Inline{richdoc.Emph{Inlines: c.convertInlines(el.Children)}}
	case doctree.TagStrong:
		return []richdoc.Inline{richdoc.Strong{Inlines: c.convertInlines(el.Children)}}
	case doctree.TagLiteral:
		return []richdoc.Inline{richdoc.Code{Value: doctree.AsText(el)}}
	case doctree.TagMath:
		// docutils/rst v0.3.0+ gives :math: its own dedicated node (see
		// its README) rather than routing it through TagInline like every
		// other role, so it's handled here, not in convertRole.
		return []richdoc.Inline{richdoc.Math{TeX: doctree.AsText(el)}}
	case doctree.TagTarget:
		// Reached only for an INLINE internal target ("_`text`",
		// docutils/rst v0.4.0+) — a block-level hyperlink target never
		// appears as a paragraph's child, so it never reaches inline
		// conversion at all (see convertBlockNode's TagTarget case, which
		// drops it: its consuming references already carry a resolved
		// refuri directly). An inline target's "name" attr is exactly
		// richdoc.Anchor's ID.
		return []richdoc.Inline{richdoc.Anchor{ID: el.Attr("name"), Inlines: c.convertInlines(el.Children)}}
	case doctree.TagTitleReference:
		// The nearest common rendering (italics) for a construct richdoc has
		// no dedicated node for; see the package doc comment.
		return []richdoc.Inline{richdoc.Emph{Inlines: c.convertInlines(el.Children)}}
	case doctree.TagSubscript, doctree.TagSuperscript:
		return []richdoc.Inline{richdoc.RawInline{Format: "rst", Text: rawRole(subSupRole(el.Tag), doctree.AsText(el))}}
	case doctree.TagAbbreviation, doctree.TagAcronym:
		// Flattened to plain text: the visible content stays readable, only
		// the "this was marked as an abbreviation" fact is lost.
		return c.convertInlines(el.Children)
	case doctree.TagInline:
		return c.convertRole(el)
	case doctree.TagReference:
		return c.convertReference(el)
	case doctree.TagSubstitutionRef:
		return c.convertSubstitutionRef(el)
	case doctree.TagFootnoteReference, doctree.TagCitationReference:
		return c.convertNoteRef(el)
	default:
		return c.convertInlines(el.Children)
	}
}

// convertRole maps an interpreted-text role. "strike" is this package's own
// [Write] convention round-tripped specifically (docutils has no native
// strikethrough markup at all), matching how
// [github.com/go-richdoc/markdown]'s Write leans on the pandoc `[@key]`
// convention for a citation CommonMark cannot express. Any other role —
// including docutils' own GENERIC roles this package's parser already
// resolves to a dedicated tag, and "math", which docutils/rst v0.3.0+ gives
// its own dedicated TagMath node rather than routing through TagInline at
// all (see convertInlineElement), so it never reaches here either — falls
// back to a verbatim [richdoc.RawInline].
func (c *converter) convertRole(el *doctree.Element) []richdoc.Inline {
	if strings.EqualFold(el.Attr("role"), "strike") {
		return []richdoc.Inline{richdoc.Strikethrough{Inlines: c.convertInlines(el.Children)}}
	}
	return []richdoc.Inline{richdoc.RawInline{Format: "rst", Text: rawRole(el.Attr("role"), doctree.AsText(el))}}
}

// convertReference maps a resolved hyperlink reference. An unresolved one (no
// refuri — the target it named doesn't exist) degrades to plain inline
// content, matching this package's own html/latex writers' "falls back to
// plain text" treatment of the same case.
func (c *converter) convertReference(el *doctree.Element) []richdoc.Inline {
	uri := el.Attr("refuri")
	if uri == "" {
		return c.convertInlines(el.Children)
	}
	return []richdoc.Inline{richdoc.Link{URL: uri, Inlines: c.convertInlines(el.Children)}}
}

// convertSubstitutionRef resolves a substitution reference against the
// definitions [converter.collect] gathered, inlining the definition's own
// content — a real resolution this package's sibling html/latex writers
// deliberately don't perform (see their SCOPE comments); an orphan reference
// (no matching definition) falls back to its bare name as plain text, the
// same fallback those writers use unconditionally.
func (c *converter) convertSubstitutionRef(el *doctree.Element) []richdoc.Inline {
	def, ok := c.substDefs[el.Attr("refname")]
	if !ok {
		return []richdoc.Inline{richdoc.Text{Value: doctree.AsText(el)}}
	}
	return c.convertInlines(substitutionValue(def))
}

// substitutionValue reads a substitution definition's value. Its content is
// always an embedded directive invocation (see docutils/rst's
// parseSubstitutionDef) — most often "replace::", whose single-line value
// parseDirective stores as the "arguments" attribute rather than a child, so
// that's checked first; a multi-line directive body (rare for a
// substitution, but the same directive machinery allows it) falls back to
// the Text child parseDirective appends in that case.
func substitutionValue(def *doctree.Element) []doctree.Node {
	if args := def.Attr("arguments"); args != "" {
		return []doctree.Node{&doctree.Text{Data: args}}
	}
	return def.Children
}

// convertNoteRef resolves a footnote/citation reference against the
// definitions [converter.collect] gathered, inlining the note's body as a
// [richdoc.Footnote] at the reference site — richdoc's own shape for a note
// (LaTeX \footnote, an ODF footnote), regardless of whether the source used
// reST's footnote or citation syntax: both are self-contained label+body
// constructs in reST, unlike LaTeX's \cite, which points at an external
// bibliography richdoc's [richdoc.CrossRef] models instead. A reference this
// package cannot resolve — most often reST's own auto-numbered [#]_ or
// symbol [*]_ forms, which docutils/rst's README documents as never assigned
// a refname by this engine — degrades to a verbatim [richdoc.RawInline]
// rather than a body-less Footnote, so the source marker isn't lost.
func (c *converter) convertNoteRef(el *doctree.Element) []richdoc.Inline {
	name := el.Attr("refname")
	def, ok := c.footnoteDefs[name]
	if name == "" || !ok {
		return []richdoc.Inline{richdoc.RawInline{Format: "rst", Text: rawNoteRef(el)}}
	}
	if c.consumed == nil {
		c.consumed = map[string]bool{}
	}
	c.consumed[name] = true
	return []richdoc.Inline{richdoc.Footnote{Blocks: c.convertBlocks(noteBody(def), 1)}}
}

// noteBody returns a footnote/citation definition's content children,
// skipping its leading [richdoc.Text]-only Label child (the "[1]"/"[CIT2002]"
// marker docutils renders before the body, which the reference site's own
// marker already conveys).
func noteBody(def *doctree.Element) []doctree.Node {
	var out []doctree.Node
	for _, ch := range def.Children {
		if e, ok := ch.(*doctree.Element); ok && e.Tag == doctree.TagLabel {
			continue
		}
		out = append(out, ch)
	}
	return out
}

func subSupRole(tag string) string {
	if tag == doctree.TagSubscript {
		return "sub"
	}
	return "sup"
}
