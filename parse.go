// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-docutils/docutils/doctree"
	docrst "github.com/go-docutils/docutils/rst"
	"github.com/go-richdoc/richdoc"
)

// Options controls a conversion choice this package cannot infer from
// the source. The zero value is what [Parse] uses.
type Options struct {
	// LineLengthLimit refuses a document containing a line longer than this,
	// with an ERROR rather than a tree -- which is docutils' own
	// denial-of-service guard, 10000 characters by default since
	// go-docutils/docutils v0.139.0.
	//
	// It is here because the parser's refusal is INVISIBLE to a converter: a
	// refused parse is a document holding one <system_message>, this package
	// drops diagnostics by default, and the caller would receive an EMPTY
	// document with nothing saying why. Converting is not parsing -- an empty
	// result is the worst possible answer -- so the limit is checked here and
	// reported as an error.
	//
	// Zero means the default 10000, because Parse passes Options{} and the zero
	// value has to mean "the normal behaviour". A NEGATIVE value means no limit,
	// for a caller that would rather spend the time than lose the document.
	LineLengthLimit int

	// KeepDiagnostics renders docutils' own <system_message> nodes as
	// ordinary paragraphs, the way every version before v0.117.0 did.
	//
	// It defaults FALSE, because a diagnostic is about the SOURCE and
	// not part of the document: left on, a reader of the converted
	// document finds "Explicit markup ends without a blank line;
	// unexpected unindent." sitting in the prose as though an author
	// had written it. That is the same reason this package already
	// turns off the three Options docutils/rst offers for author-facing
	// diagnostics -- but several warnings are behind no flag at all,
	// and those were arriving here regardless.
	//
	// Set it true to get them back: a linting tool that converts a
	// document in order to REPORT on it wants exactly what a renderer
	// does not.
	KeepDiagnostics bool
}

// Parse converts reStructuredText source into a [richdoc.Document] with
// the default [Options]. It never
// returns an error: docutils/rst.Parse has no failure mode of its own (an
// unrecognized construct degrades to plain text, matching docutils' own
// tolerant parsing philosophy), so the error return exists only for symmetry
// with [Write] and the other go-richdoc converters.
func Parse(src []byte) (*richdoc.Document, error) {
	return ParseWithOptions(src, Options{})
}

// ParseWithOptions is [Parse] with explicit control over the choices in
// [Options].
func ParseWithOptions(src []byte, opts Options) (*richdoc.Document, error) {
	// ReportUnknownDirectives OFF. docutils/rst v0.68.0+ defaults it on,
	// because docutils' own PARSER raises "Unknown directive type" -- but
	// this package converts a document for a reader, and a directive it
	// has no semantics for still has CONTENT the author wrote. Left on,
	// a Sphinx ".. toctree::" would become an error message and a
	// literal block in the converted output; off, convertBlockNode
	// reconstructs the directive as a RawBlock and the content survives.
	// Same reasoning as the dangling-reference default (v0.66.0): a
	// diagnostic aimed at an author writing reST becomes fabricated
	// CONTENT once it reaches a converted document.
	// The line-length limit, before anything else: see Options.LineLengthLimit
	// for why a converter reports this rather than letting the parse be refused.
	limit := opts.LineLengthLimit
	switch {
	case limit == 0:
		limit = docrst.DefaultOptions().LineLengthLimit
	case limit < 0:
		limit = 0 // docutils' own "no limit"
	}
	if limit > 0 {
		for i, line := range strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n") {
			if len(line) > limit {
				return nil, fmt.Errorf("rst: line %d exceeds the line-length limit of %d characters", i+1, limit)
			}
		}
	}
	dopts := docrst.DefaultOptions()
	// And the same value to the parser, so a caller that lifted the limit here is
	// not refused one layer down -- where the refusal would arrive as an empty
	// document again.
	dopts.LineLengthLimit = limit
	dopts.ReportUnknownDirectives = false
	// Same reasoning for an unknown ROLE: a Sphinx ":doc:" reference is
	// not an error to a reader, it is text, and convertRole preserves it
	// as a RawInline carrying the role name.
	dopts.ReportUnknownRoles = false
	// And the third report flag, for the same reason once more
	// (docutils/rst v0.107.0+): docutils' PARSER rejects an option a
	// directive does not declare, so a sphinx ":caption:" on a code
	// block or ":label:" on an equation replaces the whole block with an
	// error paragraph. Measured on that project's real-world corpus by
	// parsing every file twice and comparing the TREES, 32 of 1564
	// differ: 32 documents that would arrive here having lost their code,
	// their maths, or -- since docutils/rst v0.133.0 wired the same check
	// to fifteen more directives -- an admonition carrying sphinx's
	// ":collapsible:". Off, the option is ignored and the block converts.
	// TestSphinxOnlyOptionKeepsItsBlock is what holds this.
	dopts.ReportUnknownDirectiveOptions = false
	// The opposite direction, and the only one of the five this package
	// turns ON: Document.Meta IS the promoted docinfo (see leadingMeta and
	// docinfoToMeta). docutils/rst defaults it off because DocInfo is one
	// of docutils' TRANSFORMS, so a bare parse leaves a plain field list.
	dopts.PromoteDocInfo = true
	// Likewise: this package renders footnotes, inlining each definition
	// at its reference, so it needs the numbers and symbols
	// transforms.references.Footnotes assigns. Without it an auto
	// footnote arrives with no label and nothing matches a reference to
	// its definition.
	dopts.NumberAutoFootnotes = true
	// And the same for hyperlink resolution
	// (transforms.references.Hyperlinks, off by default in docutils/rst
	// v0.80.0+): convertReference sends the reader to a reference's
	// refuri, so without this every resolvable link arrives carrying
	// only the refname it points at and becomes plain text. The one
	// test that caught this on the v0.80.0 bump was the section-anchor
	// case -- a single case for a change that silently affects EVERY
	// resolvable link in the package.
	dopts.ResolveReferences = true
	doc := docrst.ParseWithOptions(string(src), dopts)
	c := &converter{
		opts:         opts,
		footnoteDefs: map[string]*doctree.Element{},
		substDefs:    map[string]*doctree.Element{},
	}
	c.collect(doc)
	c.resolveConsumed()
	c.collectSectionAnchors(doc)

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
	// Any leading <meta> nodes are STEPPED OVER, not searched through --
	// docutils/rst v0.136.5 hoists a ".. meta::" directive's own nodes to
	// docutils' own insertion point, which is ahead of a leading field
	// list, and its DocInfo transform finds the list by skipping them
	// (first_child_not_matching_class(nodes.PreBibliographic)). Reading
	// children[0] only, a document with BOTH lost every bibliographic
	// field it had: Meta came back empty and the <docinfo> went on to
	// convertBlockNode, which has no case for one and drops it -- a
	// silent content loss, not a missing convenience. The meta nodes
	// themselves stay in the returned children, where rawMeta renders
	// each one.
	metaRun := 0
	for metaRun < len(children) {
		el, ok := children[metaRun].(*doctree.Element)
		if !ok || el.Tag != doctree.TagMeta {
			break
		}
		metaRun++
	}
	i := metaRun
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
	if metaRun > 0 {
		return meta, append(append([]doctree.Node{}, children[:metaRun]...), children[i:]...)
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
// by [converter.collect], the names some reference points at, and the set
// resolved from the two, so an orphan definition (referenced by nothing)
// is still preserved via [richdoc.RawBlock] rather than silently dropped.
type converter struct {
	opts         Options
	footnoteDefs map[string]*doctree.Element
	substDefs    map[string]*doctree.Element
	referenced   map[string]bool
	consumed     map[string]bool
	// headingAnchor maps a section's OWN id to the explicit ".. _name:"
	// anchor written in front of it, and anchorAlias maps every id that
	// now names the same heading -- the section's slug and any further
	// targets -- to that one. See collectSectionAnchors.
	headingAnchor map[string]string
	anchorAlias   map[string]string
	// anchorTaken holds the internal targets a HEADING has already taken
	// responsibility for, so convertBlockElement can tell those from the ones
	// whose next node is not a section -- which richdoc's model cannot attach
	// an id to (no Block but Heading has one), and which are therefore kept as
	// their own reST source instead of being dropped.
	anchorTaken map[*doctree.Element]bool
	// expandingNote holds the footnote/citation names currently being inlined,
	// so a note that cites itself -- directly or through another note -- is not
	// followed forever. See convertNoteRef.
	expandingNote map[string]bool
	// inRawSource is set while collect descends into a subtree that will be
	// rebuilt as reST source, where a reference is written verbatim and
	// therefore consumes nothing. See reconstructedAsSource.
	inRawSource bool
}

// resolveConsumed decides, before any conversion, which definitions will
// be inlined at a reference and so must NOT also be emitted as blocks.
//
// The rule is convertNoteRef's own, stated once: a reference resolves
// when it names a definition that exists. Deriving it here rather than
// letting each site decide keeps the two from drifting apart -- and
// makes the outcome independent of document ORDER, which is what was
// wrong before.
func (c *converter) resolveConsumed() {
	if c.consumed == nil {
		c.consumed = map[string]bool{}
	}
	for name := range c.referenced {
		def, ok := c.footnoteDefs[name]
		if !ok {
			continue
		}
		// A CITATION is never consumed. Its reference is a richdoc.CrossRef
		// carrying only the key, so the definition is the only place the body
		// can live -- dropping it left "[CIT2002]_" pointing at nothing.
		if def.Tag == doctree.TagCitation {
			continue
		}
		c.consumed[name] = true
	}
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
	case doctree.TagFootnoteReference, doctree.TagCitationReference:
		// Gathered in the SAME pre-pass as the definitions, because
		// whether a definition is referenced must not depend on which
		// of the two the conversion happens to reach first. It did:
		// consumed was written at the reference SITE, so a document
		// that puts its definitions BEFORE the references it serves --
		// legal reST, and what a PEP's "References and Footnotes"
		// section does when it precedes nothing -- emitted the
		// definition as a RawBlock on the way past AND inlined it at
		// the reference, so Parse -> Write printed it twice. reST
		// convention puts definitions last, which is why the common
		// case looked right.
		// ...but only where the conversion will actually reach it: see
		// reconstructedAsSource.
		if name := el.Attr("refname"); name != "" && !c.inRawSource {
			if c.referenced == nil {
				c.referenced = map[string]bool{}
			}
			c.referenced[name] = true
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
	if reconstructedAsSource[el.Tag] && !c.inRawSource {
		c.inRawSource = true
		for _, ch := range el.Children {
			c.collect(ch)
		}
		c.inRawSource = false
		return
	}
	for _, ch := range el.Children {
		c.collect(ch)
	}
}

// reconstructedAsSource lists the block elements convertBlockNode rebuilds as
// reST SOURCE rather than converting. Inside one of these, a footnote or
// citation reference is written back verbatim -- convertNoteRef never sees it --
// so it is not a reference that CONSUMES its definition, and counting it as one
// dropped the definition while leaving the marker behind.
//
// PEP 302 opens with a ".. warning::" citing [10]_ and [11]_. Both definitions
// were dropped and both markers stayed, so the document ended with two footnote
// references pointing at nothing and two of its nine notes gone. 14 corpus files
// lose 42 definitions this way.
//
// This list has to agree with convertBlockNode's own raw-source cases. Keeping
// them in step is guarded by TEST rather than by this comment: there is a case
// for a reference inside each container here, and a new raw-source construct
// that forgets to appear in this list fails it.
var reconstructedAsSource = map[string]bool{
	doctree.TagAdmonition: true, doctree.TagAttention: true, doctree.TagCaution: true,
	doctree.TagDanger: true, doctree.TagErrorAdmonition: true, doctree.TagHint: true,
	doctree.TagImportant: true, doctree.TagNote: true, doctree.TagTip: true,
	doctree.TagWarningAdmonition: true,
	doctree.TagTopic:             true, doctree.TagSidebar: true, doctree.TagContainer: true,
	doctree.TagCompound: true, doctree.TagFigure: true, doctree.TagLineBlock: true,
	doctree.TagFieldList: true, doctree.TagDefinitionList: true,
	doctree.TagOptionList: true, doctree.TagDirective: true, doctree.TagComment: true,
	doctree.TagRubric: true, doctree.TagDecoration: true,
}

// collectSectionAnchors finds the explicit anchors written in front of a
// section -- "..  _label:" then a title, which is how Sphinx documents
// label every section they cross-reference.
//
// docutils handles these in a TRANSFORM (transforms.references.
// PropagateTargets, read directly): an internal target, meaning a
// block-level <target> with no refuri/refid/refname of its own, hands its
// ids and names to the next node, so the section ends up carrying BOTH
// "introduction" and "my-anchor". richdoc's Heading has ONE ID, and its
// own documentation says what that ID is for: "a Markdown heading anchor,
// a LaTeX \section immediately followed by \label" -- the author's label,
// not a slug derived from the title. So the label wins, and every id that
// used to name the section is remapped onto it.
//
// Without this the target was simply dropped (see convertBlockElement's
// TagTarget case, which is right for every OTHER target: one carrying a
// refuri is bookkeeping its references already resolved). The label went
// missing from the document, and a "my-anchor_" reference elsewhere --
// already resolved to the Link "#my-anchor" -- pointed at an id nothing
// in the converted tree carried. A DANGLING link, in every document that
// follows the Sphinx convention.
func (c *converter) collectSectionAnchors(el *doctree.Element) {
	var pending []*doctree.Element
	// DOCUMENT ORDER, not per-parent. The first version walked each parent's
	// own children and paired a pending target with a section SIBLING, which
	// misses the commonest shape there is: docutils' section nesting makes a
	// target written between two same-level titles the LAST CHILD of the
	// EARLIER section, so it never met the section it belongs to and was
	// dropped. Asked about exactly that input, the reference answers
	//
	//	<section ids="plain-section plain" names="plain\ section plain">
	//
	// -- PropagateTargets finds "the next node" in document order and crosses
	// the boundary. 59 of the 1564 real-world files round-tripped to a
	// different heading id because of it, and behind each of those was a
	// "#plain" reference pointing at an id nothing in the tree carried.
	for _, ce := range elementsInDocumentOrder(el) {
		switch {
		case isInternalTarget(ce):
			pending = append(pending, ce)
			continue
		case ce.Tag == doctree.TagSystemMessage:
			// PropagateTargets steps over these explicitly, since a
			// later transform may remove them.
			continue
		case ce.Tag == doctree.TagTarget && len(pending) > 0:
			// A bare target CHAINED onto one that carries a reference is not
			// an anchor in this document at all -- it is another name for that
			// destination. Asked about
			//
			//	.. _pythondoc:
			//	.. _gendoc: http://example.com/gendoc
			//
			// the reference answers
			// `<reference name="pythondoc" refuri="http://example.com/gendoc">`:
			// the name resolves to the URL, so the target is bookkeeping and
			// dropping it loses nothing. Marking the group accounted-for is
			// what keeps it out of the raw-block path -- without this,
			// pep-0256's ".. _pythondoc:" was written out immediately before a
			// section it never preceded in the source, and on the next parse it
			// became that section's anchor. A round trip that is not
			// idempotent, and the one file the set-diff showed getting worse.
			if c.anchorTaken == nil {
				c.anchorTaken = map[*doctree.Element]bool{}
			}
			for _, t := range pending {
				c.anchorTaken[t] = true
			}
		case ce.Tag == doctree.TagSection && len(pending) > 0 && pending[0].Attr("id") != "":
			chosen := pending[0].Attr("id")
			if c.headingAnchor == nil {
				c.headingAnchor = map[string]string{}
				c.anchorAlias = map[string]string{}
			}
			if own := ce.Attr("id"); own != "" {
				c.headingAnchor[own] = chosen
				c.anchorAlias[own] = chosen
			}
			for _, t := range pending[1:] {
				if id := t.Attr("id"); id != "" {
					c.anchorAlias[id] = chosen
				}
			}
			// Every target in this group has now been accounted for by the
			// heading, so nothing should write it again as a raw block.
			if c.anchorTaken == nil {
				c.anchorTaken = map[*doctree.Element]bool{}
			}
			for _, t := range pending {
				c.anchorTaken[t] = true
			}
		}
		pending = nil
	}
}

// elementsInDocumentOrder yields el's descendants pre-order -- a container
// before its own children -- which is the order PropagateTargets reads the
// tree in. A <section> therefore appears immediately after whatever preceded
// it, INCLUDING a target that the section nesting parked at the end of the
// previous section.
func elementsInDocumentOrder(el *doctree.Element) []*doctree.Element {
	var out []*doctree.Element
	var walk func(e *doctree.Element)
	walk = func(e *doctree.Element) {
		for _, ch := range e.Children {
			ce, ok := ch.(*doctree.Element)
			if !ok {
				continue
			}
			out = append(out, ce)
			walk(ce)
		}
	}
	walk(el)
	return out
}

// isInternalTarget reports whether el is the kind of target
// PropagateTargets moves: a block-level ".. _name:" with no reference of
// its own. One carrying a refuri is an external link's definition and
// names nothing in this document; one carrying a refname points at
// another target and is resolved before it gets here.
func isInternalTarget(el *doctree.Element) bool {
	return el.Tag == doctree.TagTarget &&
		el.Attr("name") != "" &&
		el.Attr("refuri") == "" &&
		el.Attr("refname") == ""
}

// rawSource reconstructs el as reST for a RawBlock, with the diagnostics
// removed first unless the caller asked to keep them.
//
// Every raw* reconstruction walks the element's own children, so a
// <system_message> nested inside one reaches the output as ordinary text
// -- the TagSystemMessage case that drops diagnostics is never consulted
// for a subtree that has already become a RawBlock. That was harmless
// while every duplicate-name notice sat OUTSIDE its container and became
// a top-level block; docutils/rst v0.136.2 put it inside the ".. note::"
// it belongs to, where it would have leaked "Duplicate implicit target
// name: ..." into the default output as if an author had written it.
// Caught by a CONTROL asserting the default still drops it -- the change
// itself looked right in the mode that keeps them.
func (c *converter) rawSource(el *doctree.Element, reconstruct func(*doctree.Element) string) string {
	if c.opts.KeepDiagnostics {
		return reconstruct(el)
	}
	return reconstruct(withoutDiagnostics(el))
}

// withoutDiagnostics returns a copy of el with every <system_message>
// descendant removed. A message is a BODY element, so it can sit in any
// container inside the subtree, not only directly under it.
func withoutDiagnostics(el *doctree.Element) *doctree.Element {
	out := doctree.NewElement(el.Tag)
	for k, v := range el.Attrs {
		out.SetAttr(k, v)
	}
	for _, ch := range el.Children {
		ce, ok := ch.(*doctree.Element)
		if !ok {
			out.Append(ch)
			continue
		}
		if ce.Tag == doctree.TagSystemMessage {
			continue
		}
		out.Append(withoutDiagnostics(ce))
	}
	return out
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
		return []richdoc.Block{richdoc.Paragraph{Inlines: c.convertInlines(el.Children), Classes: classesOf(el)}}
	case doctree.TagBulletList:
		return []richdoc.Block{c.convertList(el, false)}
	case doctree.TagEnumeratedList:
		return []richdoc.Block{c.convertList(el, true)}
	case doctree.TagBlockQuote:
		return []richdoc.Block{richdoc.BlockQuote{Blocks: c.convertBlocks(el.Children, level), Classes: classesOf(el)}}
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
		return []richdoc.Block{richdoc.Paragraph{Inlines: c.convertInlines(el.Children), Classes: classesOf(el)}}
	case doctree.TagTransition:
		return []richdoc.Block{richdoc.ThematicBreak{}}
	case doctree.TagLiteralBlock, doctree.TagDoctestBlock:
		// A DOCTEST BLOCK -- reST's third spelling for code, a paragraph opening
		// with ">>>" and needing neither "::" nor indentation -- comes out as a
		// "::" literal block, and that is a DECIDED loss rather than an
		// oversight. It renders the same and costs something real, since a
		// doctest is collected by test runners and a literal block is not; 9 of
		// the 1564 real-world files.
		//
		// Two ways to keep it were tried and both cost more. Keying the writer on
		// the text starting with ">>>" turned 110 literal blocks that SHOW a
		// session into doctest blocks (fidelity 1323 -> 1237): there are twelve
		// times as many of those as of real doctest blocks. Marking the language
		// "pycon" instead collided with the authors' own ".. code-block:: pycon",
		// which arrives here indistinguishable -- a sentinel that means two
		// things. And a RawBlock, this package's usual answer for a construct
		// richdoc cannot represent, would make the code VANISH for every consumer
		// that is not reST. A spelling change is the smallest of the three.
		return []richdoc.Block{richdoc.CodeBlock{Language: codeLanguage(el), Text: codeText(el), Classes: authorClasses(el)}}
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
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: c.rawSource(el, rawDirective)}}
	case doctree.TagFieldList:
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: c.rawSource(el, rawFieldList)}}
	case doctree.TagDefinitionList:
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: c.rawSource(el, rawDefinitionList)}}
	case doctree.TagLineBlock:
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: rawLineBlock(el)}}
	case doctree.TagOptionList:
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: c.rawSource(el, rawOptionList)}}
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
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: c.rawSource(el, rawAdmonition)}}
	case doctree.TagCompound:
		// docutils/rst v0.42.0+ -- structurally identical to the generic
		// admonitions above, just a different tag/directive name; richdoc
		// has no compound-paragraph block type either.
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: c.rawSource(el, rawCompound)}}
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
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: c.rawSource(el, rawContainer)}}
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
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: c.rawSource(el, rawTopic)}}
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
	case doctree.TagReference:
		// docutils/rst v0.127.0+ -- an ".. image::" with a ":target:"
		// arrives as a <reference> wrapping the <image>, which is how
		// every project README writes a badge. Without this case the
		// default branch below unwrapped it to its children and the LINK
		// was silently dropped, leaving the picture pointing nowhere.
		// Wrapped in a Paragraph for exactly the reason the bare image
		// above is: richdoc has no block-level image or link, and the
		// single-inline paragraph is the faithful placement, not a
		// fallback.
		return []richdoc.Block{richdoc.Paragraph{Inlines: c.convertInlines([]doctree.Node{el})}}
	case doctree.TagFigure:
		// docutils/rst v0.29.0+ -- richdoc has no figure/caption/
		// legend concept at all, so -- unlike a bare image above --
		// this falls back to a RawBlock the same way admonitions/
		// topics do, rather than silently unwrapping to its image and
		// losing the caption/legend/figure-level options entirely.
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: c.rawSource(el, rawFigure)}}
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
	case doctree.TagPending:
		// docutils/rst v0.64.0+ emits <pending> for a directive whose real
		// work happens in a TRANSFORM (".. class::", ".. sectnum::",
		// ".. target-notes::"). Its only child is a DEBUG DUMP of the
		// transform's name and options, not content, so it is dropped.
		//
		// The default branch below already drops an unrecognized tag, so
		// this case changes nothing today -- it is here because being
		// dropped by ACCIDENT and dropped ON PURPOSE read identically in
		// the output and not at all identically to the next person
		// touching this switch.
		return nil
	case doctree.TagSystemMessage:
		// See Options.KeepDiagnostics. docutils/rst v0.117.0 gave four
		// more constructs the "ends without a blank line" warning, which
		// turned a leak nobody had noticed into a visible one.
		if c.opts.KeepDiagnostics {
			return c.convertBlocks(el.Children, level)
		}
		// Dropping the message must not drop the AUTHOR'S text with it.
		// docutils quotes the offending source inside the message as a
		// <literal_block> -- a malformed table carries its whole source
		// that way (docutils/rst v0.121.0+) -- and that is content, not
		// commentary. Without this the table vanished from the
		// converted document entirely, which is worse than the
		// diagnostic paragraph this default exists to remove.
		//
		// But only where the construct was REFUSED. A warning quotes the
		// source of something docutils built anyway -- "Title underline too
		// short." keeps its section AND quotes the two lines -- so keeping
		// the quote there put the same text in the document twice, once as
		// the heading and once as a literal block nobody wrote. The level
		// is what separates the two: docutils refuses at ERROR and above
		// (malformed table, unknown directive, invalid marker, missing or
		// mismatched underline, incomplete title, a directive with no
		// content) and keeps the construct at WARNING and below (both
		// "too short" adornment cases). Checked against every
		// literal_block-carrying message in the reference rather than
		// inferred from the two that showed it.
		if lvl := el.Attr("level"); lvl != "" && lvl < "3" {
			return nil
		}
		var kept []doctree.Node
		for _, ch := range el.Children {
			if e, ok := ch.(*doctree.Element); ok && e.Tag == doctree.TagLiteralBlock {
				kept = append(kept, e)
			}
		}
		if len(kept) == 0 {
			return nil
		}
		return c.convertBlocks(kept, level)
	case doctree.TagTarget, doctree.TagSubstitutionDef:
		// A target carrying a refuri or a refname IS invisible bookkeeping:
		// its references already arrived with the URI resolved into them (see
		// the rst package's own resolveTargets), so dropping it loses nothing
		// -- checked on all three shapes, including the anonymous ".. __: uri"
		// whose reference comes out as "`x <uri>`__".
		//
		// An INTERNAL target is a different thing wearing the same tag: it
		// names a place in THIS document, and nothing can resolve it away.
		// docutils hands its ids and names to the next node (PropagateTargets),
		// which for a section this package does through collectSectionAnchors
		// -- but richdoc gives no other Block an ID, so a target whose next
		// node is a paragraph, a list or a table has nowhere to go. Dropping it
		// left a "#standalone" reference pointing at nothing. Kept here as its
		// own reST source, which is what RawBlock is documented for
		// ("round-trip fidelity for constructs the model does not represent
		// natively") and what every other unrepresentable construct in this
		// package already does.
		if el.Tag == doctree.TagTarget && isInternalTarget(el) && !c.anchorTaken[el] {
			return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: targetSource(el)}}
		}
		return nil
	case doctree.TagFootnote, doctree.TagCitation:
		// Emitted inline at each resolvable reference (convertInlineNode);
		// reaching this case means the definition was never referenced, so
		// it is preserved here rather than silently dropped.
		if name := el.Attr("name"); name != "" && c.consumed[name] {
			return nil
		}
		return []richdoc.Block{richdoc.RawBlock{Format: "rst", Text: c.rawSource(el, rawFootnoteDef)}}
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
			id := el.Attr("id")
			if anchor, ok := c.headingAnchor[id]; ok {
				// An explicit ".. _label:" in front of this section --
				// see collectSectionAnchors.
				id = anchor
			}
			out = append(out, richdoc.Heading{Level: clampLevel(level), ID: id, Inlines: c.convertInlines(title.Children)})
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
	l := richdoc.List{Ordered: ordered, Start: start, Tight: true, Classes: classesOf(el)}
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
	// A <title> child of the table is its CAPTION -- ".. table:: Caption" -- and
	// richdoc v0.5.0 has a field for it. 24 captions in 13 corpus files had
	// nowhere to go before.
	for _, ch := range el.Children {
		if ce, ok := ch.(*doctree.Element); ok && ce.Tag == doctree.TagTitle {
			t.Caption = c.convertInlines(ce.Children)
			break
		}
	}
	// The DERIVED classes are filtered here, on the way IN, not on the way out.
	// Filtering them in the writer left the model holding "colwidths-given" while
	// the reconstruction omitted it, so the next parse had no class and the round
	// trip was not a fixed point -- 13 files, all of them tables. A class docutils
	// worked out for itself is not part of the document.
	t.Classes = authorTableClasses(classesOf(el))
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
			// BOTH, which is richdoc v0.5.0's own contract for a Cell: Blocks
			// is the faithful content and Inlines the flattened view a consumer
			// that does not read Blocks still renders. See cellBlocks for when
			// Blocks is filled at all.
			Inlines: c.cellInlines(entry.Children),
			Blocks:  c.cellBlocks(entry),
			ColSpan: extraSpan(entry, "morecols"),
			RowSpan: extraSpan(entry, "morerows"),
		})
	}
	return cells
}

// cellBlocks converts a table entry's children as BLOCKS, for the cells that need
// it, and returns nil for the cells that do not.
//
// reST's grid tables allow full block content in a cell (see docutils/rst's own
// README) and richdoc v0.5.0 can hold it. Before that every cell was flattened to
// a run of inlines, which cost 64 list items, 61 line-block lines, 35 literal
// blocks and 23 bullet lists over the 1564-file corpus -- a list in a cell came
// back as two paragraphs, a literal block as an inline literal.
//
// nil for the common case ON PURPOSE: a cell holding exactly one paragraph says
// everything it has to say in Inlines, and filling Blocks as well would make every
// consumer choose between two spellings of the same thing. The rule is therefore
// "anything a paragraph cannot hold": more than one block, or a single block that
// is not a paragraph.
func (c *converter) cellBlocks(entry *doctree.Element) []richdoc.Block {
	blocks := c.convertBlocks(entry.Children, 1)
	if len(blocks) == 0 {
		return nil
	}
	if len(blocks) == 1 {
		if _, ok := blocks[0].(richdoc.Paragraph); ok {
			return nil
		}
	}
	return blocks
}

// cellInlines converts a table entry's content to inline text for
// richdoc.Cell, which can only hold inline content — unlike a document's
// top-level blocks, reST's grid tables allow full block content in a cell
// (nested lists, multiple paragraphs; see docutils/rst's own README). A
// cell holding just one paragraph (the overwhelming common case) is
// unaffected by any of this; a cell with more than one top-level block has
// them joined by a blank line instead of running together with no
// separator at all — lossy (which words belonged to which list item or
// paragraph is gone), but not GARBLED.
//
// Since richdoc v0.5.0 this is the DEGRADED view, not the only one:
// [cellBlocks] fills Cell.Blocks with the real structure, and this function
// keeps producing Cell.Inlines beside it for every consumer that does not read
// Blocks. richdoc's own Cell doc comment is the contract.
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
			// A BLANK LINE, not a space. Either way the block structure is
			// gone from THIS view of the cell -- Cell.Blocks is where it lives
			// since richdoc v0.5.0 -- and either way a consumer
			// rendering to HTML sees whitespace, so nothing is lost by the
			// change. What it buys is a FIXED POINT: a space between two texts
			// re-parses as one text, so the cell came back with a different
			// inline list every time and 19 of the 1564 real-world files could
			// not round-trip for that reason alone. A blank line re-parses as
			// two blocks again, and they are joined here the same way.
			//
			// It also puts the break back into the reconstructed reST, where a
			// grid cell really can hold two paragraphs.
			out = append(out, richdoc.Text{Value: "\n\n"})
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
		return []richdoc.Inline{richdoc.Code{Value: codeText(el)}}
	case doctree.TagSystemMessage:
		// A cell is flattened to INLINES, so a <system_message> in one reached
		// the default below and came back as its own TEXT: the parser's
		// complaint printed into the table, in a sentence no author wrote and
		// every reader sees. convertBlockNode's TagSystemMessage case -- the one
		// that drops diagnostics -- is never consulted on this path, which is
		// the same blind spot rawSource had (see withoutDiagnostics) in a third
		// place.
		//
		// The reference removes a message below its report level with a
		// TRANSFORM, universal.FilterMessages, not at parse time: asked for the
		// doctree of a simple table whose cell holds "??", docutils' PARSER
		// attaches the INFO "Unexpected possible title overline or transition."
		// and the published document does not have it. So the message really is
		// in the tree this converter reads, and dropping it is what the
		// reference does too.
		return c.cellMessageInlines(el)
	default:
		return c.convertInlineElement(el)
	}
}

// cellMessageInlines applies convertBlockNode's diagnostic policy inside a table
// cell: keep nothing by default, keep everything when the caller asked for the
// diagnostics, and keep the quoted source of a construct docutils REFUSED (level
// 3 and above), which is the author's text and not commentary.
func (c *converter) cellMessageInlines(el *doctree.Element) []richdoc.Inline {
	if c.opts.KeepDiagnostics {
		return c.cellInlines(el.Children)
	}
	if lvl := el.Attr("level"); lvl != "" && lvl < "3" {
		return nil
	}
	var kept []doctree.Node
	for _, ch := range el.Children {
		if e, ok := ch.(*doctree.Element); ok && e.Tag == doctree.TagLiteralBlock {
			kept = append(kept, e)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return c.cellInlines(kept)
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
		return []richdoc.Inline{imageFrom(el)}
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
		return []richdoc.Inline{richdoc.Anchor{ID: anchorID(el), Inlines: c.convertInlines(el.Children)}}
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
	case doctree.TagCitationReference:
		// A CITATION, not a footnote. reST has both and they are different
		// things: a footnote is a note at the foot of the page, and a citation
		// names a bibliographic entry -- "[CIT2002]_" -- which is what a reader
		// sees and what richdoc.CrossRef with RefCite already models (the write
		// side emits "[target]_" for it and always has).
		//
		// Inlining a citation as a richdoc.Footnote threw the LABEL away:
		// "[CIT2002]_" came back as "[1]_" and ".. [CIT2002]" as ".. [1]", so
		// the document showed a number where the author wrote a key. 15 of the
		// 1564 real-world files.
		if name := el.Attr("refname"); name != "" {
			// The reference's own TEXT, not its refname: docutils NORMALISES a
			// name to lower case for matching, so "[CIT2002]_" carries
			// refname="cit2002" while the text is what the author typed and
			// what the definition's own <label> keeps. Writing the refname
			// still resolves -- reST names are case-insensitive -- but shows
			// "cit2002" where the document says "CIT2002".
			target := strings.TrimSpace(doctree.AsText(el))
			if target == "" {
				target = name
			}
			return []richdoc.Inline{richdoc.CrossRef{Target: target, Kind: richdoc.RefCite}}
		}
		return c.convertNoteRef(el)
	case doctree.TagFootnoteReference:
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
	// A same-document link may point at an id the heading no longer
	// carries, because an explicit anchor in front of a section takes
	// that heading's one ID slot -- so "#introduction" and "#my-anchor"
	// both have to arrive at whichever one was kept.
	if name, ok := strings.CutPrefix(uri, "#"); ok {
		if anchor, found := c.anchorAlias[name]; found {
			uri = "#" + anchor
		}
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
	// A note's body is INLINED here, so a note that cites itself is a CYCLE, and
	// following it was a stack overflow -- a crash, on reST docutils reads
	// without complaint: ".. [1] A note citing itself [1]_." gives one <footnote>
	// with two backrefs. Nothing in the 1564-file corpus does it, which is why
	// no sweep had found it; the shape turned up while asking a different
	// question about footnote numbering.
	//
	// A reference back into a note already being expanded degrades to the
	// verbatim marker, which is exactly what an unresolvable reference above
	// already does: the marker is not lost, and it is the only answer available
	// to a model that carries a note by VALUE.
	if c.expandingNote[name] {
		return []richdoc.Inline{richdoc.RawInline{Format: "rst", Text: rawNoteRef(el)}}
	}
	if c.expandingNote == nil {
		c.expandingNote = map[string]bool{}
	}
	c.expandingNote[name] = true
	blocks := c.convertBlocks(noteBody(def), 1)
	delete(c.expandingNote, name)
	return []richdoc.Inline{richdoc.Footnote{Blocks: blocks}}
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

// anchorID picks the id for an inline internal target's Anchor. "name" is
// the natural choice and stays the default, but it is NOT always there:
// since docutils/rst v0.57.0 a target whose name collides with another's
// is INVALIDATED, its name moved to "dupname" — so two "_`term`" targets
// in one document both arrived here with no "name" at all and produced
// two anchors with an EMPTY id, indistinguishable from each other and
// useless as link destinations. The "id" attribute is always present and
// already disambiguated upstream ("term", "term-1"), so it is the right
// fallback; "dupname" is deliberately not used, since the whole point of
// the invalidation is that the name no longer identifies one place.
func anchorID(el *doctree.Element) string {
	if name := el.Attr("name"); name != "" {
		return name
	}
	return el.Attr("id")
}

// codeLanguage recovers a code block's language from the class list the
// ".. code::" directive leaves on its <literal_block>. Found in
// v0.108.0, while verifying a DIFFERENT change: every code block in
// every converted document had arrived with an empty Language, because
// nothing here had ever read one.
//
// docutils' CodeBlock.run (parsers/rst/directives/body.py, read
// directly) builds the list as ["code"], then the language argument if
// there is one, then whatever the author's own :class: adds. So the
// language is the SECOND class, and only when the first is "code" -- a
// plain "::" literal block carries no classes at all and must stay
// languageless.
//
// The position is the only signal there is, and it is genuinely
// ambiguous: ".. code:: c" and ".. code::" with ":class: c" produce the
// identical class list, and docutils itself cannot tell them apart
// afterwards either. Its own writers dodge the question by highlighting
// at PARSE time with pygments and emitting token spans, so there is no
// reference behaviour to copy here -- only the class list, read the way
// the directive wrote it.
// codeText is a literal block's own text WITHOUT the line numbers
// ":number-lines:" generates. docutils puts those in <inline class="ln">
// children of the block (an "1 " before each line), and taking the node's whole
// text folded them into the CODE: ".. code:: python" with :number-lines: and
// "x = 1" came back as "1 x = 1", which is no longer the program the author
// wrote. richdoc.CodeBlock has no line-numbering flag, so the OPTION cannot
// survive -- but keeping the code correct matters more than keeping a
// presentation option, and baking generated numbers into it is the one outcome
// that is wrong either way.
func codeText(el *doctree.Element) string {
	var b strings.Builder
	for _, ch := range el.Children {
		if ce, ok := ch.(*doctree.Element); ok {
			if ce.Tag == doctree.TagInline && strings.Contains(" "+ce.Attr("class")+" ", " ln ") {
				continue
			}
		}
		b.WriteString(doctree.AsText(ch))
	}
	return b.String()
}

func codeLanguage(el *doctree.Element) string {
	classes := strings.Fields(el.Attrs["class"])
	if len(classes) < 2 || classes[0] != "code" {
		return ""
	}
	return classes[1]
}

// targetSource writes an internal hyperlink target back as reST.
//
// The quoting rule is the inverse of the reference parser's own pattern
// (states.py, Body.explicit.patterns.target, read directly):
//
//	(?!_) (?P<quote>`?) (?![ `]) (?P<name>.+?) (?P=quote) (?<!:) :
//
// An unquoted name is ".+?" stopped by the colon that ends the marker, so a
// name CONTAINING a colon has to be backquoted; so does one starting with an
// underscore (which would read as the anonymous form), a space or a backquote.
// Everything else is written bare, which is how the author almost always wrote
// it.
func targetSource(el *doctree.Element) string {
	name := el.Attr("name")
	if needsTargetQuotes(name) {
		return ".. _`" + name + "`:"
	}
	return ".. _" + name + ":"
}

func needsTargetQuotes(name string) bool {
	if name == "" {
		return false
	}
	if strings.ContainsRune(name, ':') {
		return true
	}
	switch name[0] {
	case '_', ' ', '`':
		return true
	}
	return false
}

// imageFrom reads every attribute richdoc.Image can now hold (v0.4.0). Before
// those fields existed, ":align:", ":width:", ":height:", ":scale:" and
// ":class:" reached this converter with nowhere to go: 43 of the 79 standalone
// images in the 1564-document corpus carry at least one, in 18 files.
//
// The values go across AS WRITTEN, with their units. docutils has already
// validated the spelling (rst.formatMeasure/formatPercentage) and normalised a
// class name through nodes.make_id, so nothing here has to parse or canonicalise
// them -- and a converter for a format that sizes images differently needs the
// unit, not a number this package guessed at.
func imageFrom(el *doctree.Element) richdoc.Image {
	img := richdoc.Image{
		URL:     el.Attr("uri"),
		Alt:     el.Attr("alt"),
		Width:   el.Attr("width"),
		Height:  el.Attr("height"),
		Classes: classesOf(el),
		Align:   alignmentOf(el.Attr("align")),
	}
	// ":scale: 50" arrives as "50": a PERCENTAGE, which is why richdoc.Image
	// holds it as an int and not beside Width as a length.
	if n, err := strconv.Atoi(strings.TrimSuffix(el.Attr("scale"), "%")); err == nil {
		img.Scale = n
	}
	return img
}

// classesOf splits a doctree "class" attribute into richdoc's own Classes slice.
// docutils stores them space-separated, each already put through nodes.make_id
// by class_option (read directly), so they are written back verbatim and reparse
// to the same list.
func classesOf(el *doctree.Element) []string {
	if c := strings.Fields(el.Attr("class")); len(c) > 0 {
		return c
	}
	return nil
}

// alignmentOf maps reST's image alignment onto richdoc's own enum. Only the three
// horizontal values have a counterpart: "top", "middle" and "bottom" place an
// INLINE image against the text line, which richdoc's Alignment -- a column
// alignment borrowed for this -- does not express, so they leave it at
// AlignDefault rather than being bent into one of the three.
func alignmentOf(v string) richdoc.Alignment {
	switch v {
	case "left":
		return richdoc.AlignLeft
	case "center":
		return richdoc.AlignCenter
	case "right":
		return richdoc.AlignRight
	}
	return richdoc.AlignDefault
}

// authorClasses is classesOf minus the classes docutils gave the node ITSELF. A
// literal_block from ".. code:: python" carries class="code python", which the
// writer reproduces by writing that directive back -- carrying it in Classes too
// would write it a second time, as ":class: code python", and the next parse would
// have it twice.
func authorClasses(el *doctree.Element) []string {
	var out []string
	for i, c := range strings.Fields(el.Attr("class")) {
		if i == 0 && c == "code" {
			continue
		}
		// The language follows "code" and is already in CodeBlock.Language.
		if i == 1 && c == codeLanguage(el) {
			continue
		}
		out = append(out, c)
	}
	return out
}
