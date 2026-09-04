// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strconv"
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

	meta, children := leadingMeta(doc.Children)

	blocks := c.convertBlocks(children, 1)
	d := &richdoc.Document{Blocks: blocks}
	if len(meta) > 0 {
		d.Meta = meta
	}
	return d, nil
}

// leadingMeta reads [richdoc.Document.Meta] off the document's very first
// blocks and returns the rest — the convention this package uses (matching
// [Write]) to round-trip it: any OTHER field list, not the document's own
// first block, falls back to [richdoc.RawBlock] instead (see
// convertBlockNode). The first block is either a plain field_list, or
// (docutils/rst v0.12.0+) a promoted docinfo — see docinfoToMeta — followed
// by zero or more dedication/abstract <topic> siblings docutils' own
// DocInfo transform produces alongside it, folded in here too rather than
// left for convertBlockNode to mishandle (a bare <topic>, like a bare
// <docinfo> child, has no block-level case of its own and richdoc has no
// dedicated node for either).
func leadingMeta(children []doctree.Node) (map[string]string, []doctree.Node) {
	meta := map[string]string{}
	i := 0
	if i < len(children) {
		if el, ok := children[i].(*doctree.Element); ok {
			switch el.Tag {
			case doctree.TagFieldList:
				meta = fieldsToMeta(el)
				i++
			case doctree.TagDocinfo:
				meta = docinfoToMeta(el)
				i++
			}
		}
	}
	for i < len(children) {
		topic, ok := children[i].(*doctree.Element)
		if !ok || topic.Tag != doctree.TagTopic {
			break
		}
		class := topic.Attr("class")
		if class != "dedication" && class != "abstract" {
			break
		}
		meta[class] = topicText(topic)
		i++
	}
	return meta, children[i:]
}

// fieldsToMeta reads a field list's name/body pairs into a plain map, using
// the body's flattened text (see [package doc] for why field content isn't
// rendered with full inline fidelity here).
func fieldsToMeta(fl *doctree.Element) map[string]string {
	meta := map[string]string{}
	for _, c := range fl.Children {
		if field, ok := c.(*doctree.Element); ok && field.Tag == doctree.TagField {
			if name, body, ok := fieldNameBody(field); ok {
				meta[name] = body
			}
		}
	}
	return meta
}

// docinfoToMeta reads a promoted <docinfo>'s children into the same flat
// map fieldsToMeta builds from an unpromoted field_list, so a caller sees
// identical Meta either way regardless of which shape docutils/rst
// produced (see [package doc]): a typed field's own tag name becomes the
// key (e.g. "date", "version"); "authors" joins its <author> children with
// "; ", the same separator docutils/rst's own docinfo.go tries FIRST when
// splitting a single author-list field body (falling back to "," only if
// that yields no split), chosen here for the same reason — a name itself
// might contain a comma more plausibly than a semicolon. A plain
// (unrecognized-name or compound-body) <field>, still folded into docinfo
// by real docutils rather than left in a separate list, is read the same
// way fieldsToMeta reads one.
func docinfoToMeta(docinfo *doctree.Element) map[string]string {
	meta := map[string]string{}
	for _, c := range docinfo.Children {
		el, ok := c.(*doctree.Element)
		if !ok {
			continue
		}
		switch el.Tag {
		case doctree.TagField:
			if name, body, ok := fieldNameBody(el); ok {
				meta[name] = body
			}
		case doctree.TagAuthors:
			var names []string
			for _, ac := range el.Children {
				if a, ok := ac.(*doctree.Element); ok && a.Tag == doctree.TagAuthor {
					names = append(names, strings.TrimSpace(doctree.AsText(a)))
				}
			}
			meta["authors"] = strings.Join(names, "; ")
		default:
			meta[el.Tag] = strings.TrimSpace(doctree.AsText(el))
		}
	}
	return meta
}

// fieldNameBody reads one <field>'s name/body pair, ok=false if it has no
// name (malformed input this parser never actually produces, but a
// zero-value guard is cheaper than a panic).
func fieldNameBody(field *doctree.Element) (name, body string, ok bool) {
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
	return name, body, name != ""
}

// topicText flattens a dedication/abstract <topic>'s content (its own
// <title> child skipped — "Dedication"/"Abstract" restates what the Meta
// key already says) into a single Meta value. Multiple paragraphs join
// with a single space rather than a blank line: writeMeta (write.go)
// emits every Meta value on its own ":key: value" line with no
// continuation-line support at all, so a literal blank line here would
// write out a field a reparse couldn't read back as one value.
func topicText(topic *doctree.Element) string {
	var parts []string
	for _, c := range topic.Children {
		el, ok := c.(*doctree.Element)
		if !ok || el.Tag == doctree.TagTitle {
			continue
		}
		if t := strings.TrimSpace(doctree.AsText(el)); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
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
		// docutils/rst v0.29.0+ -- "name" is the substitution's own
		// identifying attribute (matching real docutils; an earlier
		// version there used a made-up "substitution" attribute
		// instead, since fixed).
		if name := el.Attr("name"); name != "" {
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
	case doctree.TagAttribution:
		// docutils/rst v0.19.0+ — a block quote's trailing "-- text"
		// attribution. Its children are bare INLINE nodes (parseInline's
		// own output), not block-level Paragraph wrappers, so the generic
		// convertBlocks fallback (which only recurses into *doctree.Element
		// children) silently dropped this entirely — the exact same
		// bare-inline-content gap TagRaw hit before it got its own case.
		// richdoc has no dedicated attribution concept, so this maps to a
		// plain Paragraph, the same non-lossy generic-node choice already
		// used for problematic/system_message elsewhere in this file.
		return []richdoc.Block{richdoc.Paragraph{Inlines: c.convertInlines(el.Children)}}
	case doctree.TagTransition:
		return []richdoc.Block{richdoc.ThematicBreak{}}
	case doctree.TagLiteralBlock, doctree.TagDoctestBlock:
		return []richdoc.Block{richdoc.CodeBlock{Text: doctree.AsText(el)}}
	case doctree.TagMathBlock:
		// docutils/rst v0.52.0+ (".. math::") — richdoc has a REAL
		// block-math type of its own, so this maps straight onto it
		// rather than taking the RawBlock fallback admonitions/topics/
		// figure/container/compound/rubric all need: inventing a
		// RawBlock for something richdoc can already represent properly
		// would be a real regression in fidelity, not a neutral
		// simplification (the same call the v0.29.0 <image> case made,
		// where richdoc's own Image type was likewise already waiting).
		// Without any case at all this fell through to the generic
		// convertBlocks default, which has no notion of a bare Text
		// child at block level and DROPPED the math source entirely —
		// caught by checking parse.go's own switch for the new tag name
		// BEFORE trusting the suite staying green, which it did, since
		// no existing fixture exercises a math directive.
		return []richdoc.Block{richdoc.MathBlock{TeX: doctree.AsText(el)}}
	case doctree.TagComment:
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawComment(el)}}
	case doctree.TagRaw:
		// docutils/rst v0.15.0+ (Options.RawEnabled, on by default) — the
		// node's own "format" attribute is the real target format
		// (html, latex, possibly several space-separated), not "rst":
		// unlike this package's OWN RawBlock fallbacks below, this is
		// genuinely raw target-format content, not resynthesized reST
		// only this package knows how to read back. Without a case here
		// it fell through to the generic block walker, which has no
		// notion of a bare Text child at block level and silently
		// dropped the whole node — caught by testing this exact
		// construct after implementing it on the docutils/rst side.
		return []richdoc.Block{richdoc.RawBlock{Format: el.Attr("format"), Text: doctree.AsText(el)}}
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
	case doctree.TagAttention, doctree.TagCaution, doctree.TagDanger,
		doctree.TagErrorAdmonition, doctree.TagHint, doctree.TagImportant,
		doctree.TagNote, doctree.TagTip, doctree.TagWarningAdmonition,
		doctree.TagAdmonition:
		// docutils/rst v0.27.0+ -- the nine generic admonitions plus
		// ".. admonition::" itself. richdoc has no admonition/callout
		// block type at all (its own Block interface is a documented
		// closed set), so -- like field/definition lists above -- this
		// falls back to a RawBlock rather than silently unwrapping to
		// the bare content and losing which admonition it was.
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawAdmonition(el)}}
	case doctree.TagCompound:
		// docutils/rst v0.42.0+ -- structurally identical to the generic
		// admonitions above, just a different tag/directive name; richdoc
		// has no compound-paragraph block type either.
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawCompound(el)}}
	case doctree.TagDecoration:
		// docutils/rst v0.48.0+ -- a document-level singleton wrapping up
		// to one <header> and one <footer>; richdoc has no document-
		// header/footer concept at all. Without an explicit case here
		// this fell through to the generic convertBlocks default below,
		// which recurses into <header>/<footer>'s own children directly
		// -- silently unwrapping the whole thing into ordinary paragraphs
		// indistinguishable from body content, the same "new TOP-LEVEL
		// tag, no matching case" shape as v0.42.0's container/compound
		// and v0.45.0's rubric -- caught by checking parse.go's own
		// switch statement for the new tag names BEFORE trusting the
		// suite staying green (no existing fixture exercises header/
		// footer at all, so a green suite here proves nothing).
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawDecoration(el)}}
	case doctree.TagContainer:
		// docutils/rst v0.42.0+ -- richdoc has no container block type
		// either, and unlike TagCompound this needed its own rawContainer
		// (the classes come from the directive's own ARGUMENT, not a
		// :class: option -- see that function's own doc comment). Without
		// an explicit case here this fell through to the generic
		// convertBlocks default below, which recurses into the
		// container's own children directly -- silently unwrapping it
		// and losing both the fact that it WAS a container and its own
		// class/name attributes, the same shape as every other
		// already-handled directive in this switch, just for a whole
		// top-level tag instead of a child nested under one.
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawContainer(el)}}
	case doctree.TagRubric:
		// docutils/rst v0.45.0+ -- richdoc has no rubric block type
		// either. Without an explicit case here this fell through to the
		// generic convertBlocks default below, which recurses into the
		// rubric's own children directly -- but those are INLINE nodes
		// (Text, possibly emphasis/...), not block-level Elements, so
		// convertBlocks finds nothing it recognizes and the rubric's own
		// text is silently DROPPED ENTIRELY, not just unwrapped -- a
		// worse loss than container's own (which at least kept the bare
		// content). Caught by testing directly, not just the test suite
		// staying green (nothing in the existing suite exercised rubric
		// at all).
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawRubric(el)}}
	case doctree.TagTopic, doctree.TagSidebar:
		// docutils/rst v0.28.0+ -- richdoc has no topic/sidebar block
		// type either, so like the admonitions above this falls back to
		// a RawBlock rather than silently unwrapping to the bare title
		// and content (the leading dedication/abstract <topic> case is
		// already handled earlier, in leadingMeta, before this switch
		// is ever reached for those two).
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawTopic(el)}}
	case doctree.TagImage:
		// docutils/rst v0.29.0+ -- a standalone ".. image::" (as
		// opposed to one embedded in a substitution definition, which
		// reaches convertInlineElement's own TagImage case instead --
		// this one is never reached for that shape, since a
		// substitution_definition's children are walked as INLINES,
		// not blocks). richdoc has no bare block-level image concept,
		// so this wraps the same richdoc.Image in a single-inline
		// Paragraph -- a real, non-lossy placement (the nearest
		// analogue to how CommonMark itself treats a standalone image),
		// not a RawBlock fallback: unlike an admonition or a topic,
		// nothing about "this was a directive" needs preserving here.
		return []richdoc.Block{richdoc.Paragraph{Inlines: c.convertInlines([]doctree.Node{el})}}
	case doctree.TagFigure:
		// docutils/rst v0.29.0+ -- richdoc has no figure/caption/
		// legend concept at all, so -- unlike a bare image above --
		// this falls back to a RawBlock the same way admonitions/
		// topics do, rather than silently unwrapping to its image and
		// losing the caption/legend/figure-level options entirely.
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawFigure(el)}}
	case doctree.TagMeta:
		// docutils/rst v0.30.0+ -- HTML/head metadata, a different
		// concept from Document.Meta above (which is keyed by field
		// NAME for docinfo-derived values) even though the shape looks
		// similar -- richdoc has no dedicated node for it either, so
		// this falls back to a RawBlock the same way admonitions/
		// topics/figure do, one ".. meta::" per element (each <meta>
		// node reaches here on its own, never grouped by its original
		// source directive -- see hoistMetaNodes on the docutils/rst
		// side, which flattens every meta field to a sibling of the
		// document root individually).
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawMeta(el)}}
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
			// docutils/rst v0.17.0+ registers every section title as an
			// implicit hyperlink target (id = a plain-ASCII slug of the
			// title); carrying it onto Heading.ID is what makes a
			// `Some Title`_-style reference — already a resolved
			// richdoc.Link{URL: "#the-slug"} via convertReference below —
			// point at a real anchor instead of a slug nothing in the
			// richdoc tree actually carries. The system-messages section
			// (see rst's own systemMessagesSection) never gets an id at
			// all, so this is empty for it, same as any other heading
			// nobody referenced.
			out = append(out, richdoc.Heading{Level: clampLevel(level), ID: el.Attr("id"), Inlines: c.convertInlines(title.Children)})
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
// single paragraph. docutils/rst v0.25.0+ gives an enumerated_list a "start"
// attribute whenever it doesn't begin at ordinal 1 (rst/parser.go's own
// enumtype/prefix/suffix/start work, read directly) — read here, defaulting
// to 1 when absent (the common case) or unparseable. The list's own
// enumerator TYPE (arabic/alpha/roman) and format (period/parens/rparen)
// have no richdoc equivalent at all and are not carried through — [Write]
// always re-renders as plain arabic "N.", the same one-way-round-trip
// limitation Start itself used to have before this.
func (c *converter) convertList(el *doctree.Element, ordered bool) richdoc.List {
	start := 1
	if s := el.Attr("start"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			start = n
		}
	}
	l := richdoc.List{Ordered: ordered, Start: start, Tight: true}
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

// tableGroupChildren returns the elements holding thead/tbody: a <table>'s
// <tgroup> child's own children (docutils/rst v0.10.0+ always wraps rows
// in one, alongside <colspec> column-width metadata this package has no
// use for — a colspec has no children of its own and no case in the
// switch below, so it is silently skipped either way), or the table's own
// children directly if there is no tgroup wrapper.
func tableGroupChildren(table *doctree.Element) []doctree.Node {
	for _, c := range table.Children {
		if ce, ok := c.(*doctree.Element); ok && ce.Tag == doctree.TagTgroup {
			return ce.Children
		}
	}
	return table.Children
}

// convertTable converts a simple or grid table's thead/tbody rows. A
// grid-table cell's column/row span (see the docutils README) carries
// through to richdoc.Cell.ColSpan/RowSpan (richdoc v0.3.0+), rather than
// collapsing to its own unspanned cell the way it used to.
func (c *converter) convertTable(el *doctree.Element) richdoc.Table {
	var t richdoc.Table
	for _, ch := range tableGroupChildren(el) {
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
		cells = append(cells, richdoc.Cell{
			Inlines: c.cellInlines(entry.Children),
			ColSpan: extraSpan(entry, "morecols"),
			RowSpan: extraSpan(entry, "morerows"),
		})
	}
	return cells
}

// cellInlines converts a table entry's content to inline text for
// richdoc.Cell, which can only hold inline content — unlike a document's
// top-level blocks, reST's grid tables allow full block content in a cell
// (nested lists, multiple paragraphs; see docutils/rst's own README). A
// cell holding just one paragraph (the overwhelming common case) is
// unaffected by any of this; a cell with more than one top-level block has
// them joined by a single space instead of running together with no
// separator at all — lossy (which words belonged to which list item or
// paragraph is gone), but not GARBLED. Same underlying constraint as
// go-richdoc/latex's own documented "cell content flattened to plain
// text": richdoc.Cell has no Blocks field to hold real block structure in.
func (c *converter) cellInlines(children []doctree.Node) []richdoc.Inline {
	var parts [][]richdoc.Inline
	for _, ch := range children {
		if in := c.cellBlockInlines(ch); len(in) > 0 {
			parts = append(parts, in)
		}
	}
	var out []richdoc.Inline
	for i, p := range parts {
		if i > 0 {
			out = append(out, richdoc.Text{Value: " "})
		}
		out = append(out, p...)
	}
	return out
}

// cellBlockInlines flattens one child of a table entry. A paragraph or a
// genuinely inline node (emphasis, a reference, ...) converts the normal
// way; a further block container (a list, a list item, a block quote, a
// field/definition list and its parts) recurses through cellInlines
// itself, so ITS OWN children get the same space-joining treatment,
// keeping (for example) separate list items from running together.
func (c *converter) cellBlockInlines(n doctree.Node) []richdoc.Inline {
	el, ok := n.(*doctree.Element)
	if !ok {
		return c.convertInlineNode(n)
	}
	switch el.Tag {
	case doctree.TagParagraph:
		return c.convertInlines(el.Children)
	case doctree.TagBulletList, doctree.TagEnumeratedList, doctree.TagListItem,
		doctree.TagBlockQuote, doctree.TagDefinitionList, doctree.TagDefinitionListItem,
		doctree.TagFieldList, doctree.TagField, doctree.TagTerm, doctree.TagDefinition,
		doctree.TagFieldName, doctree.TagFieldBody:
		return c.cellInlines(el.Children)
	case doctree.TagLiteralBlock, doctree.TagDoctestBlock, doctree.TagLineBlock:
		return []richdoc.Inline{richdoc.Code{Value: doctree.AsText(el)}}
	default:
		return c.convertInlineElement(el)
	}
}

// extraSpan reads a grid-table entry's morecols/morerows attribute — the
// number of EXTRA columns/rows spanned, docutils' own convention, e.g.
// morecols="1" for a cell spanning 2 columns — into richdoc.Cell's own
// ColSpan/RowSpan (the TOTAL span, so that same cell gets ColSpan 2), a
// deliberately off-by-one difference from the attribute's own name kept
// consistent with the docutils/html and docutils/latex writers' identical
// "+1" convention for the same attribute.
func extraSpan(entry *doctree.Element, attr string) int {
	extra := entry.Attr(attr)
	if extra == "" {
		return 0
	}
	n, err := strconv.Atoi(extra)
	if err != nil {
		return 0
	}
	return n + 1
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
	case doctree.TagImage:
		// docutils/rst v0.29.0+ -- reached both for a substitution
		// definition's own embedded "image::" (whose <image> child is
		// inline-classified in real docutils too, the only reason it
		// survives that filter unflattened) and, via convertBlockNode's
		// own TagImage case below, a standalone block-level image
		// wrapped in a single-inline Paragraph -- richdoc already has a
		// real Image inline type for exactly this.
		return []richdoc.Inline{richdoc.Image{URL: el.Attr("uri"), Alt: el.Attr("alt")}}
	case doctree.TagRaw:
		// docutils/rst v0.16.0+'s inline raw role (".. role:: x(raw)"),
		// the inline counterpart of the block-level TagRaw case below —
		// without this case it fell through to the generic inline-text
		// walk, which flattened genuine target-format markup ("<b>x</b>")
		// into what LOOKS like ordinary prose the author typed, losing
		// the "this is raw, not text" distinction entirely rather than
		// just losing formatting.
		return []richdoc.Inline{richdoc.RawInline{Format: el.Attr("format"), Text: doctree.AsText(el)}}
	case doctree.TagTarget:
		// Reached for an INLINE internal target ("_`text`", docutils/rst
		// v0.4.0+ — real visible content, its own "name" attr exactly
		// richdoc.Anchor's ID) AND, since docutils/rst v0.31.0+, for the
		// IMPLICIT target a named phrase-reference-with-embedded-link
		// (`` `text <uri>`_ ``/`` `text <alias_>`_ ``) also emits as a
		// sibling right after its own <reference> — that one carries no
		// content of its own at all (real docutils constructs it with no
		// text, just refuri/refname for some OTHER reference elsewhere
		// to resolve against), so it's dropped here exactly like a
		// block-level hyperlink target already is (convertBlockNode's
		// own TagTarget case): the reference that produced it already
		// carries its OWN resolved refuri/refname directly (this
		// project's upstream dependency resolves eagerly, before this
		// package ever sees the tree), so nothing is lost by dropping
		// it — keeping it instead would have produced a SECOND, empty
		// anchor with an unrelated id right next to the real link.
		if len(el.Children) == 0 {
			return nil
		}
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
	return c.convertInlines(def.Children)
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
