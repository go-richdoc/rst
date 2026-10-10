// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	docrst "github.com/go-docutils/docutils/rst"
	"github.com/go-richdoc/richdoc"
)

// Write renders a [richdoc.Document] to reST text. The error return is
// always nil; it is kept for symmetry with [Parse]. A nil document renders
// to empty output.
//
// Non-nil [richdoc.Document.Meta] is emitted as a leading field list
// (":key: value" per entry, sorted by key for a stable output), the same
// convention [Parse] reads back into Meta — see [leadingFieldList].
//
// Footnotes are collected while the body renders (a [richdoc.Footnote]'s
// body sits inline at its reference point in richdoc, but reST's own
// footnote/citation syntax is a separate block referenced by label) and
// their ".. [n] ..." definitions are emitted, numbered in reference order,
// after the document body — the same pattern
// [github.com/go-richdoc/markdown]'s Write uses for CommonMark's "[^n]: ..."
// definitions.
func Write(d *richdoc.Document) ([]byte, error) {
	if d == nil {
		return []byte{}, nil
	}
	w := &writer{}
	// Before anything is numbered: a definition this writer will emit VERBATIM
	// holds its own label, and the counter must not choose the same one.
	w.reserveRawNoteLabels(d.Blocks)
	var parts []string
	if len(d.Meta) > 0 {
		parts = append(parts, writeMeta(d.Meta))
	}
	if body := w.writeBlocks(d.Blocks); body != "" {
		parts = append(parts, body)
	}
	if defs := w.writeFootnoteDefs(); defs != "" {
		parts = append(parts, defs)
	}
	if len(parts) == 0 {
		return []byte{}, nil
	}
	return []byte(strings.Join(parts, "\n\n") + "\n"), nil
}

func writeMeta(meta map[string]string) string {
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		// A field VALUE can be several lines -- an address, a copyright
		// notice -- and a field body's continuation has to be INDENTED or the
		// field list ends there: "Field list ends without a blank line;
		// unexpected unindent." Three spaces, which is all docutils asks for
		// (a consistent indent greater than zero), rather than aligning under
		// the value the way the source happened to.
		lines = append(lines, ":"+k+": "+continuationIndent(meta[k], 3))
	}
	return strings.Join(lines, "\n")
}

// writer holds the render-wide footnote accumulator; see [Write].
type writer struct {
	footnotes []richdoc.Footnote
	// bodies maps a note's RENDERED body to the number already given it, so the
	// same note cited twice is one definition cited twice.
	bodies map[string]int
	// taken holds the labels verbatim definitions already hold, labels the
	// numbers given to each note, and skipped how far the counter has been
	// pushed past a taken label.
	taken   map[string]bool
	labels  []int
	skipped int
}

// reRawNoteDef matches a note definition inside a RawBlock's reST, which keeps
// the author's OWN label.
var reRawNoteDef = regexp.MustCompile(`(?m)^\.\. \[([^\]]+)\]`)

// reserveRawNoteLabels seeds the numbering with the labels already spoken for by
// definitions this writer will emit VERBATIM.
//
// A note referenced only from inside a construct rebuilt as reST source keeps its
// own label on both sides -- the marker is written verbatim and so is the
// definition. Meanwhile footnoteNumber counts 1..N for the notes it inlines, and
// nothing stopped the two from choosing the same label: PEP 550 came out with
// ".. [9]" and ".. [10]" TWICE, once verbatim and once renumbered, so docutils
// reported a duplicate and the references stopped resolving. Found by set-diff
// after the definitions were saved -- the round trip had gone DOWN by three files
// while the definition count went up.
func (w *writer) reserveRawNoteLabels(blocks []richdoc.Block) {
	if w.taken == nil {
		w.taken = map[string]bool{}
	}
	for _, b := range blocks {
		switch v := b.(type) {
		case richdoc.RawBlock:
			if v.Format != "" && !strings.EqualFold(v.Format, "rst") {
				continue
			}
			for _, m := range reRawNoteDef.FindAllStringSubmatch(v.Text, -1) {
				w.taken[m[1]] = true
			}
		case richdoc.BlockQuote:
			w.reserveRawNoteLabels(v.Blocks)
		case richdoc.List:
			for _, it := range v.Items {
				w.reserveRawNoteLabels(it.Blocks)
			}
		}
	}
}

// footnoteNumber gives a note its label, reusing the one an identical note
// already has.
//
// richdoc carries a note BY VALUE: the body is inlined at every reference, so
// three references to one note arrive as three Footnote values with the same
// blocks. Appending each of them produced three definitions with three
// different numbers -- a document that gained two footnotes it never had, in
// 117 of the 1564 real-world files.
//
// The key is the rendered body, which is the only thing the model offers: the
// label is not carried, so two notes cannot be told apart by name. That is also
// this fix's BLIND SPOT, stated rather than hidden -- two genuinely distinct
// notes whose bodies render identically become one. They render identically too,
// and reST has no way to say "these two same-looking notes are different", so
// the cost is a label count rather than content.
func (w *writer) footnoteNumber(fn richdoc.Footnote) int {
	body := w.writeBlocks(fn.Blocks)
	if w.bodies == nil {
		w.bodies = map[string]int{}
	}
	// An EMPTY body carries no identity, so it cannot be a dedup key. PEP 653
	// writes nine notes whose content is unindented, which docutils reads as nine
	// notes holding nothing but a diagnostic -- and this package strips
	// diagnostics, so all nine arrived here with empty blocks and merged into
	// ONE. Nine definitions became one, which is a worse loss than the
	// duplication the dedup exists to prevent.
	if body != "" {
		if n, ok := w.bodies[body]; ok {
			return n
		}
	}
	w.footnotes = append(w.footnotes, fn)
	// Skip any label a verbatim definition already holds; see
	// reserveRawNoteLabels.
	n := len(w.footnotes) + w.skipped
	for w.taken[strconv.Itoa(n)] {
		w.skipped++
		n++
	}
	w.labels = append(w.labels, n)
	if body != "" {
		w.bodies[body] = n
	}
	return n
}

func (w *writer) writeFootnoteDefs() string {
	if len(w.footnotes) == 0 {
		return ""
	}
	// BY INDEX, re-reading the length each time: writing a note's body can
	// append MORE notes, because a note may cite one. "for i, fn := range" fixes
	// the length at entry, so those were numbered at the reference site and then
	// never defined -- PEP 302 came out with references [10]_ and [11]_ and no
	// definitions for them, and two of its nine notes simply gone.
	var parts []string
	for i := 0; i < len(w.footnotes); i++ {
		body := indentBlock(w.writeBlocks(w.footnotes[i].Blocks))
		parts = append(parts, ".. ["+strconv.Itoa(w.labels[i])+"]\n\n"+body)
	}
	return strings.Join(parts, "\n\n")
}

func (w *writer) writeBlocks(blocks []richdoc.Block) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, w.writeBlock(b, 1))
	}
	return strings.Join(parts, "\n\n")
}

// writeBlock renders one block. level is the current section-nesting depth,
// used to pick the title-underline character for a Heading (docutils allows
// any order of underline characters; this package fixes one so nested
// headings survive a round-trip through docutils/rst's own first-seen-style
// ordering, matching how [github.com/go-richdoc/markdown]'s Write always
// emits ATX '#' rather than setext underlines).
// writeBlock renders one block, wrapping it in a ".. class::" directive when it
// carries classes richdoc v0.4.0 can now hold.
//
// The CONTENT form (".. class:: names" and the block indented under it), not the
// bare form: docutils applies a bare ".. class::" to the NEXT element through a
// transform (misc.ClassAttribute), and this project's parser does not run that
// transform -- asked for the bare form it leaves a <pending> and the paragraph
// gets no class at all, where the content form gives
// `<paragraph class="foo bar">` in both parsers. Checked against the reference on
// both spellings before choosing.
//
// A CodeBlock is excluded because ".. code::" carries its own ":class:" option,
// and an Image because ".. image::" does; wrapping either would write the class
// twice.
func (w *writer) writeBlock(b richdoc.Block, level int) string {
	body := w.writeBlockBody(b, level)
	if cs := blockClasses(b); len(cs) > 0 {
		return ".. class:: " + strings.Join(cs, " ") + "\n\n" + indentBlock(body)
	}
	return body
}

// blockClasses returns the classes a block carries, or nil for one whose own
// reconstruction already writes them.
func blockClasses(b richdoc.Block) []string {
	switch n := b.(type) {
	case richdoc.Paragraph:
		return n.Classes
	case richdoc.List:
		return n.Classes
	case richdoc.BlockQuote:
		// NOT wrapped. A block quote is written by INDENTING its content, and
		// indenting that again under a directive loses the quote: ".. class::
		// epigraph" with a doubly-indented body reparses as a PARAGRAPH with the
		// class, the quote gone. Its own directive carries the class instead --
		// see quoteDirective -- and every block-quote class in the 1564-document
		// corpus is one of those three (6 epigraph, 1 pull-quote).
		return nil
	case richdoc.Table:
		// Already filtered on the way in; see convertTable.
		return n.Classes
	}
	return nil
}

// quoteDirective is the directive that produced a classed block quote, or "" for
// one this package cannot name. docutils' epigraph, highlights and pull-quote
// directives all yield a <block_quote> carrying their own name as its class, so
// writing the directive back is both faithful and the only spelling that keeps
// the quote.
func quoteDirective(classes []string) string {
	if len(classes) != 1 {
		return ""
	}
	switch classes[0] {
	case "epigraph", "highlights", "pull-quote":
		return classes[0]
	}
	return ""
}

// authorTableClasses drops the classes docutils DERIVES for a table from its own
// column widths. "colwidths-given" comes from ".. table::"'s ":widths:" option
// and "colwidths-auto" from its absence (checked: a plain grid table carries
// neither) -- 70 of the corpus's 83 table classes are one of the two, and writing
// one back as an explicit ":class:" would state as the author's what docutils
// worked out for itself.
func authorTableClasses(classes []string) []string {
	var out []string
	for _, c := range classes {
		if c == "colwidths-given" || c == "colwidths-auto" {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (w *writer) writeBlockBody(b richdoc.Block, level int) string {
	switch n := b.(type) {
	case richdoc.Heading:
		return w.writeHeading(n)
	case richdoc.Paragraph:
		// A paragraph holding nothing but an image — or a link around
		// nothing but an image — is what an ".. image::" directive
		// becomes when it is read (see convertBlockElement), and reST
		// has no INLINE image to write it back as: writeInline degrades
		// one to its alt text, so a picture disappeared from every
		// document this writer produced, and a LINKED one came out as
		// the empty-label "` <uri>`__", which does not even read back as
		// a link. In block position the directive is available and
		// nothing has to be lost.
		if src, ok := writeImageBlock(n.Inlines); ok {
			return src
		}
		// escapeBlockStart, not just escapeText: a paragraph's own first
		// characters decide whether reST reads it as a paragraph at all.
		// "B. Smith wrote this" came back as an enumerated list and
		// ".. not a directive" as a comment. Applied to the rendered text
		// rather than at the marker sites, so a paragraph that is a list
		// item's own first block is covered too -- "- " prepended to text
		// that itself starts with "-" is a NESTED bullet list.
		return escapeBlockStart(w.writeInlines(n.Inlines))
	case richdoc.List:
		return w.writeList(n)
	case richdoc.CodeBlock:
		return writeCodeBlock(n)
	case richdoc.BlockQuote:
		if d := quoteDirective(n.Classes); d != "" {
			// NOT indentBlock: rawDirectiveSource indents the content itself,
			// and the directive's body IS the quote -- indenting twice put the
			// text at six spaces, which reparses as one paragraph with the class
			// and no quote, the very loss this branch exists to avoid.
			return rawDirectiveSource(".. "+d+"::", nil, w.writeBlocks(n.Blocks))
		}
		return indentBlock(w.writeBlocks(n.Blocks))
	case richdoc.Table:
		return w.writeTable(n)
	case richdoc.MathBlock:
		return ".. math::\n\n" + indentBlock(n.TeX)
	case richdoc.RawBlock:
		switch {
		case n.Format == "" || strings.EqualFold(n.Format, "rst"):
			return n.Text
		default:
			// Any other format reconstructs as a real ".. raw:: FORMAT"
			// directive rather than being dropped — unlike this
			// package's own RawBlock fallbacks (always Format "rst",
			// package-specific resynthesized reST this package alone
			// knows how to read back), a non-rst Format here almost
			// certainly came from docutils/rst's own <raw> node (see its
			// README's node-mapping table), and ".. raw:: FORMAT" is a
			// general, well-defined reST construct, not something only
			// this package's own Parse can interpret.
			return ".. raw:: " + n.Format + "\n\n" + indentBlock(n.Text)
		}
	}
	// richdoc.Block is closed; the only remaining variant is ThematicBreak,
	// which carries no data.
	return strings.Repeat("-", 4)
}

// titleChars are the underline characters docutils' own first-seen-style
// ordering would assign to increasing depths in a document this package
// writes on its own (see rst/parser.go's titleStyle): '=' for the first
// level ever seen, then '-', '~', '"', '^', deeper levels repeating the last.
var titleChars = []byte{'=', '-', '~', '"', '^'}

func (w *writer) writeHeading(h richdoc.Heading) string {
	level := h.Level
	if level < 1 {
		level = 1
	}
	idx := level - 1
	if idx >= len(titleChars) {
		idx = len(titleChars) - 1
	}
	// The title line carries its MARKUP, and the underline is measured
	// against that line. Writing the plain text instead lost every emphasis,
	// literal and role inside a heading -- 242 of the 1564 real-world corpus
	// files, found by comparing the round-tripped TREE rather than the
	// output (the flattened text is stable on a second write, so an
	// output-to-output check calls it fine).
	//
	// The reasoning it replaces was that an underline must match the title's
	// VISIBLE width. docutils only warns when an underline is SHORTER than
	// its title line ("column_width(title) > len(underline)", states.py); a
	// longer one is perfectly legal. So underlining the rendered line costs
	// nothing and keeps the markup.
	// escapeBlockStart, for the same reason a paragraph gets it and with a
	// sharper consequence. A title line is still the FIRST LINE OF A BLOCK, and
	// reST tries explicit markup before it tries a title: a heading whose text
	// begins ".. include:: /etc/passwd" was written out verbatim and read back as
	// a DIRECTIVE, with the underline becoming a transition. Converting an
	// untrusted Markdown document ("# <b>.. include:: /etc/passwd</b>", whose
	// inline HTML this package drops while keeping its text) therefore produced
	// reST that makes a docutils parse read an arbitrary file -- demonstrated
	// against the reference, which inlined that file's contents.
	//
	// A probe over every position a document can hold text (injectprobe, beside
	// the corpus) says this was the ONLY one: a paragraph, a list item, a quote,
	// a cell, a caption, a footnote body, a link's text and an image's alt were
	// all escaped already.
	//
	// The escape is applied BEFORE the underline is measured, so the underline
	// still cannot be shorter than its title line.
	text := escapeBlockStart(w.writeInlines(h.Inlines))
	plain := escapeBlockStart(writeInlinesPlain(h.Inlines))
	if strings.ContainsAny(text, "\n") {
		// Nothing inline should render a newline, but a title that did
		// would break its own underline: fall back rather than emit
		// something that cannot read back.
		text = plain
	}
	under := strings.Repeat(string(titleChars[idx]), displayWidth(text))
	s := text + "\n" + under
	// Emit an explicit target ONLY when the heading's own title would not
	// already produce this id. reStructuredText gives every section an
	// implicit target named after its title, so ".. _top:" before a
	// heading called "Top" does not add an anchor -- it adds a SECOND
	// claim on the same name, which docutils reports as
	// `Duplicate implicit target name: "top".` (docutils/rst v0.57.0+
	// ports that diagnostic, which is how this surfaced: the round-trip
	// test caught the writer emitting reST that no longer read back as
	// what it was written from). MakeID is the upstream rule itself
	// rather than a local reimplementation, so the two cannot drift.
	// The implicit target a section gets is named after its TEXT, not its
	// markup, so the id comparison stays on the plain form.
	if h.ID != "" && h.ID != docrst.MakeID(plain) && !generatedHeadingID(h.ID, plain) {
		s = ".. _" + h.ID + ":\n\n" + s
	}
	return s
}

// generatedHeadingID reports whether id is one the PARSER made up rather
// than one an author wrote — either shape:
//
//	the title's own make_id plus a numeric suffix  ("intro-1" for the
//	   SECOND section titled "Intro", claimID's disambiguation)
//	"section-" plus a counter  ("section-1", for a title that yields no
//	   identifier at all: a CJK or Cyrillic one, since make_id keeps
//	   only ASCII)
//
// Writing either back as an explicit target is worse than useless. The
// parser assigns the section its id from its own title and position
// whatever targets precede it — a real ".. _my-anchor:" before a CJK
// title is dropped the same way — so the target adds no anchor to the
// model; it only CLAIMS that name, which pushes the section's own id one
// suffix further. The next round trip writes the pushed id, and the id
// goes back to the first one: the document OSCILLATES between two forms
// forever.
//
// Three passes is what shows that. One pass looks like plain degradation
// and two looks like it converged — which is why the test below runs
// three and compares all of them.
func generatedHeadingID(id, title string) bool {
	base := docrst.MakeID(title)
	if base == "" {
		base = "section"
	}
	rest, ok := strings.CutPrefix(id, base+"-")
	if !ok {
		return false
	}
	for _, part := range strings.Split(rest, "-") {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

// displayWidth returns a heading's own underline length: at least 1 (a
// zero-width underline isn't valid reST even for an empty title), never
// less than the rune count (len(s) in bytes would under-count a multi-byte
// title).
//
// A title underline is NOT the rule a TABLE column follows, but it is
// not the rune count either, which is what stood here until v0.116.0.
// docutils compares column_width(title) against len(underline)
// (states.py:2888), so a title of five code points and seven COLUMNS
// needs seven underline characters. Writing five produced a heading
// that came back as a warning and a literal block.
//
// That correction is worth its own line: the earlier claim here said
// the test "compares lengths in code points, verified against real
// docutils". It had been verified against a document too SHORT to form
// a section, so the check never ran and both answers looked right.
// [docrst.ColumnWidth] is the rule; [docrst.TableColumnWidth] is the
// table one, and docutils genuinely keeps both. Table-cell padding needs
// [docrst.TableColumnWidth] instead, which counts an East Asian Wide or
// Fullwidth character as TWO columns because that is what the grid
// itself is measured in (docutils/rst v0.110.0+). The minimum of 1
// belongs here and not there: an empty cell has to pad as 0 characters
// wide, and using this function for one was a real bug (a genuinely
// empty padding cell came out one character narrower than its column,
// throwing off every column after it in that row).
func displayWidth(s string) int {
	if n := docrst.ColumnWidth(s); n > 0 {
		return n
	}
	return 1
}

// runeLen is the plain rune count of s, no minimum.
func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func writeCodeBlock(c richdoc.CodeBlock) string {
	// The other half of the language loss codeLanguage fixes (v0.108.0):
	// this wrote "::" for every code block, so a language that HAD
	// survived the read -- from a markdown fence, say, converted through
	// richdoc -- was discarded on the way out. ".. code::" is the
	// directive that carries one; "::" stays the shape when there is
	// nothing to carry, since the directive form is noisier and a
	// languageless block gains nothing from it.
	if len(c.Classes) > 0 {
		// ".. code::" carries its own ":class:", and it is the only way to write
		// a class onto a literal block: wrapping it in ".. class::" would put
		// the class on the block AND leave "::" inside, which reparses with the
		// class twice. A languageless block still needs the directive here,
		// since "::" has no options at all.
		opts := []string{":class: " + strings.Join(c.Classes, " ")}
		return rawDirectiveSource(".. code:: "+c.Language, opts, indentBlock(c.Text))
	}
	if c.Language == "" {
		return "::\n\n" + indentBlock(c.Text)
	}
	return ".. code:: " + c.Language + "\n\n" + indentBlock(c.Text)
}

func (w *writer) writeList(l richdoc.List) string {
	items := make([]string, 0, len(l.Items))
	start := l.Start
	if start < 1 {
		start = 1
	}
	for i, it := range l.Items {
		marker := "- "
		if l.Ordered {
			marker = strconv.Itoa(start+i) + ". "
		}
		items = append(items, indentItem(w.writeItemBlocks(it.Blocks), marker))
	}
	return strings.Join(items, "\n\n")
}

// writeItemBlocks renders a list item's blocks, blank-line separated (reST
// requires a blank line between a list item's own paragraph and any further
// block, unlike CommonMark's tight-list shortcut).
func (w *writer) writeItemBlocks(blocks []richdoc.Block) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, w.writeBlock(b, 1))
	}
	return strings.Join(parts, "\n\n")
}

// indentItem prefixes the first line with marker and every following line
// with matching spaces, so nested blocks stay inside the item.
func indentItem(content, marker string) string {
	pad := strings.Repeat(" ", len(marker))
	lines := strings.Split(content, "\n")
	var sb strings.Builder
	for i, ln := range lines {
		if i > 0 {
			sb.WriteByte('\n')
			if ln != "" {
				sb.WriteString(pad)
			}
		} else {
			sb.WriteString(marker)
		}
		sb.WriteString(ln)
	}
	return sb.String()
}

// writeTable emits a GRID table (`+---+`-bordered): unlike a simple table's
// fixed-width columns, a grid table's column widths are computed from actual
// cell content, which keeps this writer from having to invent a padding
// scheme independent of what's in the cells.
//
// Both spans are drawn, which is a placement problem before it is a drawing
// one -- see grid.go. A ColSpan merges the interior "|" between the columns
// it covers into the cell's own padded content area; a RowSpan takes the
// horizontal rule out from under itself and shifts the row below, since that
// row's cells no longer start at column 0.
func (w *writer) writeTable(t richdoc.Table) string {
	rows := make([][]spanCell, 0, len(t.Rows)+1)
	headerRule := 0
	if len(t.Header) > 0 {
		// A header cell may not span DOWN: the "=" rule under the header has
		// to cross the whole table, or what comes back is not a header.
		header := spanCellTexts(w, t.Header)
		for i := range header {
			header[i].rows = 1
		}
		rows = append(rows, header)
		headerRule = 1
	}
	for _, row := range t.Rows {
		rows = append(rows, spanCellTexts(w, row))
	}
	if len(rows) == 0 {
		return ""
	}
	cells, owner, nrows, ncols := placeGrid(rows)
	if ncols == 0 {
		return ""
	}
	lines := make([][]string, len(cells))
	for i, c := range cells {
		// A row is as many source lines as its TALLEST cell. It used to be
		// exactly one, with every newline collapsed to a space -- which
		// corrupted nothing and lost nothing readable, but re-wrapped the
		// cell: "...is not\npresent, this method raises a" came back as one
		// long line, so 45 of the 1564 real-world files did not round-trip.
		// reST folds a wrap back to a space, so the two are the same
		// DOCUMENT; they are not the same tree, and keeping the author's own
		// lines is what makes the tree a fixed point.
		//
		// A richdoc.LineBreak inside a cell is still lossy either way: it
		// renders as a newline, and reST reads a newline inside a cell as a
		// soft wrap, so it comes back as text rather than as a break. Named
		// rather than hidden -- a grid cell has no reST spelling for a hard
		// break outside a line block.
		lines[i] = strings.Split(c.text, "\n")
	}
	widths := gridWidths(cells, ncols)
	heights := gridHeights(cells, lines, nrows)
	grid := renderGrid(cells, owner, widths, heights, lines, headerRule)
	// A CAPTION is the ".. table::" directive's argument, with the grid as its
	// content: that is the only spelling reST has for one, and docutils puts it
	// back as the <title> child this converter reads it from.
	if len(t.Caption) > 0 {
		return ".. table:: " + continuationIndent(w.writeInlines(t.Caption), 3) + "\n\n" + indentBlock(grid)
	}
	return grid
}

// spanCell is one cell's rendered text alongside the column span it covers
// (at least 1).
type spanCell struct {
	text string
	span int
	rows int
}

func cellSpan(c richdoc.Cell) int {
	if c.ColSpan < 1 {
		return 1
	}
	return c.ColSpan
}

func cellRows(c richdoc.Cell) int {
	if c.RowSpan < 1 {
		return 1
	}
	return c.RowSpan
}

// cellKeepsItsLines reports whether a cell's own line breaks can be written as
// line breaks. They can when every newline is a SOFT WRAP inside a
// richdoc.Text: reST folds one back to a space, so the cell comes out of the
// next parse with the same text.
//
// They cannot when a newline sits inside something verbatim -- an inline
// literal, inline maths, raw inline. Those are written with their own
// delimiters and their indentation is content, so putting the second line on
// its own line makes docutils read the indentation as a block quote and lose the
// closing delimiter: PEP 307's "`for k, v in state.items():\n    setattr(obj, k,
// v)`" came back with "Inline literal start-string without end-string" INSIDE the
// cell, and PEP 720's six-line literal gained 26 diagnostics. Such a cell is
// flattened, as every cell used to be.
//
// The binary round-trip probe could not see either: both files already did not
// round-trip, so they stayed "lossy" while getting much worse. What saw it was
// counting the diagnostics the RECONSTRUCTION introduces (30 messages before
// this change, 54 after, back to 28 with this guard).
func cellKeepsItsLines(inlines []richdoc.Inline) bool {
	for _, in := range inlines {
		switch v := in.(type) {
		case richdoc.Text:
			// A soft wrap, which is what this whole feature is for.
		case richdoc.Code:
			if strings.Contains(v.Value, "\n") {
				return false
			}
		case richdoc.Math:
			if strings.Contains(v.TeX, "\n") {
				return false
			}
		case richdoc.RawInline:
			if strings.Contains(v.Text, "\n") {
				return false
			}
		case richdoc.Link:
			if !cellKeepsItsLines(v.Inlines) {
				return false
			}
		case richdoc.Emph:
			if !cellKeepsItsLines(v.Inlines) {
				return false
			}
		case richdoc.Strong:
			if !cellKeepsItsLines(v.Inlines) {
				return false
			}
		case richdoc.Anchor:
			if !cellKeepsItsLines(v.Inlines) {
				return false
			}
		}
	}
	return true
}

// cellWidth is a cell's width as a grid column sees it: the widest of its
// LINES, not the length of the whole text. A cell that keeps the author's own
// wrapping has several, and measuring the joined string instead made every such
// column as wide as the cell's entire content.
func cellWidth(text string) int {
	w := 0
	for _, l := range strings.Split(text, "\n") {
		if n := docrst.TableColumnWidth(l); n > w {
			w = n
		}
	}
	return w
}

// spanCellTexts renders a row's cells to text+span triples. It no longer pads
// the row out to the table's width: a short row (which [richdoc.Table]'s own
// doc comment allows for a headerless or ragged table) is a row with HOLES in
// it, and only [placeGrid] can see where they fall once a span from an earlier
// row has moved the cells along.
func spanCellTexts(w *writer, cells []richdoc.Cell) []spanCell {
	out := make([]spanCell, 0, len(cells))
	for _, c := range cells {
		// escapeBlockStart, and BEFORE flattening: a cell's content is parsed
		// as its own block fragment, so a cell beginning with something reST
		// reads as a block marker starts one. PEP 624 writes "\(2)" in a cell
		// precisely to stop that, and the escape was not re-emitted -- so the
		// reconstruction read it as an enumerated list starting at 2 and put
		// docutils' own "Enumerated list start value not ordinal-1" INFO into
		// the cell, where the author had written "(2)". A diagnostic became
		// document content.
		//
		// escapeText already covered "*" and "|", being inline markers; "(", a
		// bullet "-" and the other seven block shapes are positional and need
		// this. Flattening afterwards is what makes escaping the FIRST line
		// enough: every later line ends up mid-line, where no block marker is
		// recognised.
		text := cellText(w, c)
		out = append(out, spanCell{text: text, span: cellSpan(c), rows: cellRows(c)})
	}
	return out
}

// cellText renders one cell, from its BLOCKS when richdoc v0.5.0's Cell carries
// them and from its flattened Inlines when it does not.
//
// Neither of the two treatments the inline path needs applies to blocks, and
// applying either would be wrong:
//
//   - escapeBlockStart escapes a first line that LOOKS like a block marker, because
//     flattened text that begins "(2)" or "- " would start a list the author did
//     not write. A cell's blocks are real markup, so escaping them would break the
//     very list this version exists to keep.
//   - the newline-to-space collapse exists because a soft wrap inside flattened
//     text is not meaningful. In block content every line break is.
//
// The grid writer already lays a multi-line cell out: that is how a cell holding
// two paragraphs has worked since the blank line between them became a fixed
// point.
func cellText(w *writer, c richdoc.Cell) string {
	if len(c.Blocks) > 0 {
		return w.writeBlocks(c.Blocks)
	}
	text := escapeBlockStart(w.writeInlines(c.Inlines))
	if !cellKeepsItsLines(c.Inlines) {
		text = strings.ReplaceAll(text, "\n", " ")
	}
	return text
}

// spanTextWidth is the padded-text-area width available to a cell spanning
// `span` columns starting at `col` — derived from the frame's own
// per-column "+2" convention: merging N columns removes N-1 interior "|"
// characters but each removal effectively donates 3 characters (the
// interior "|" plus the two single padding spaces that flanked it) to the
// merged content area, verified by matching total line length against
// a real docutils grid-table example's own line lengths.
func spanTextWidth(widths []int, col, span int) int {
	total := 3 * (span - 1)
	for i := col; i < col+span; i++ {
		total += widths[i]
	}
	return total
}

// writeImageBlock renders a paragraph that is exactly one image, or
// exactly one link around exactly one image, as the ".. image::"
// directive it was read from — ":target:" carrying the link back.
// Anything else in the paragraph, even an empty text node beside the
// image, makes it a real paragraph again and this declines: the
// directive is a BLOCK, so it cannot be spliced into running text.
func writeImageBlock(inlines []richdoc.Inline) (string, bool) {
	if len(inlines) != 1 {
		return "", false
	}
	target := ""
	img, ok := inlines[0].(richdoc.Image)
	if !ok {
		link, isLink := inlines[0].(richdoc.Link)
		if !isLink || len(link.Inlines) != 1 {
			return "", false
		}
		if img, ok = link.Inlines[0].(richdoc.Image); !ok {
			return "", false
		}
		target = link.URL
	}
	if img.URL == "" {
		return "", false
	}
	var options []string
	if img.Alt != "" {
		options = append(options, ":alt: "+img.Alt)
	}
	// The options richdoc gained in v0.4.0. Their ORDER follows docutils' own
	// option_spec listing rather than the struct's field order, so a
	// reconstruction reads like the sources this corpus is made of.
	if img.Height != "" {
		options = append(options, ":height: "+img.Height)
	}
	if img.Width != "" {
		options = append(options, ":width: "+img.Width)
	}
	if img.Scale != 0 {
		options = append(options, ":scale: "+strconv.Itoa(img.Scale))
	}
	if a := alignName(img.Align); a != "" {
		options = append(options, ":align: "+a)
	}
	if len(img.Classes) > 0 {
		options = append(options, ":class: "+strings.Join(img.Classes, " "))
	}
	if target != "" {
		options = append(options, ":target: "+target)
	}
	return rawDirectiveSource(".. image:: "+img.URL, options, ""), true
}

// alignName is the reST spelling of a richdoc.Alignment, or "" for
// AlignDefault -- which means "not given" and must not be written as an option
// at all, since ":align:" has no "default" value to name.
func alignName(a richdoc.Alignment) string {
	switch a {
	case richdoc.AlignLeft:
		return "left"
	case richdoc.AlignCenter:
		return "center"
	case richdoc.AlignRight:
		return "right"
	}
	return ""
}
