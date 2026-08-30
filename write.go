// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"sort"
	"strconv"
	"strings"

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
		lines = append(lines, ":"+k+": "+meta[k])
	}
	return strings.Join(lines, "\n")
}

// writer holds the render-wide footnote accumulator; see [Write].
type writer struct {
	footnotes []richdoc.Footnote
}

func (w *writer) writeFootnoteDefs() string {
	if len(w.footnotes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(w.footnotes))
	for i, fn := range w.footnotes {
		body := indentBlock(w.writeBlocks(fn.Blocks))
		parts = append(parts, ".. ["+strconv.Itoa(i+1)+"]\n\n"+body)
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
func (w *writer) writeBlock(b richdoc.Block, level int) string {
	switch n := b.(type) {
	case richdoc.Heading:
		return writeHeading(n)
	case richdoc.Paragraph:
		return w.writeInlines(n.Inlines)
	case richdoc.List:
		return w.writeList(n)
	case richdoc.CodeBlock:
		return writeCodeBlock(n)
	case richdoc.BlockQuote:
		return indentBlock(w.writeBlocks(n.Blocks))
	case richdoc.Table:
		return w.writeTable(n)
	case richdoc.MathBlock:
		return ".. math::\n\n" + indentBlock(n.TeX)
	case richdoc.RawBlock:
		if n.Format == "" || strings.EqualFold(n.Format, "rst") {
			return n.Text
		}
		return ""
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

func writeHeading(h richdoc.Heading) string {
	level := h.Level
	if level < 1 {
		level = 1
	}
	idx := level - 1
	if idx >= len(titleChars) {
		idx = len(titleChars) - 1
	}
	text := writeInlinesPlain(h.Inlines)
	under := strings.Repeat(string(titleChars[idx]), displayWidth(text))
	s := text + "\n" + under
	if h.ID != "" {
		s = ".. _" + h.ID + ":\n\n" + s
	}
	return s
}

// displayWidth returns a heading's own underline length: at least 1 (a
// zero-width underline isn't valid reST even for an empty title), never
// less than the rune count (len(s) in bytes would under-count a multi-byte
// title). Table-cell padding math needs the PLAIN rune count instead — an
// empty cell has to pad as 0 characters wide, not 1 — see runeLen; using
// this function there was a real bug (a genuinely empty padding cell came
// out one character narrower than its column, throwing off every column
// after it in that row).
func displayWidth(s string) int {
	if n := runeLen(s); n > 0 {
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
	return "::\n\n" + indentBlock(c.Text)
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
// A cell's ColSpan merges the interior "|" between the columns it covers
// into the cell's own padded content area (the border row above/below stays
// a full "+---+---+", unaffected — only the CONTENT line's interior
// separator disappears; verified against a real docutils grid-table
// example). RowSpan is preserved on the richdoc.Cell itself (so nothing is
// lost from the tree Parse produced), but this writer does not merge the
// horizontal border between spanned rows: reconstructing that would need
// tracking which columns have a row-span still "open" at each border and
// blanking just that segment, real complexity for a rarer construct than
// column-spanning, deferred rather than half-done. The cell's own content
// still renders in full, just as its own bordered row.
func (w *writer) writeTable(t richdoc.Table) string {
	cols := spannedCols(t.Header)
	for _, row := range t.Rows {
		if n := spannedCols(row); n > cols {
			cols = n
		}
	}
	if cols == 0 {
		return ""
	}
	widths := make([]int, cols)
	headerCells := spanCellTexts(w, t.Header, cols)
	widenColumns(widths, headerCells)
	rowCells := make([][]spanCell, len(t.Rows))
	for r, row := range t.Rows {
		rowCells[r] = spanCellTexts(w, row, cols)
		widenColumns(widths, rowCells[r])
	}
	for i, wd := range widths {
		if wd < 1 {
			widths[i] = 1
		}
	}
	widenSpannedColumns(widths, headerCells)
	for _, rc := range rowCells {
		widenSpannedColumns(widths, rc)
	}

	var b strings.Builder
	border := gridBorder(widths, '-')
	b.WriteString(border + "\n")
	if len(t.Header) > 0 {
		b.WriteString(gridRow(headerCells, widths) + "\n")
		b.WriteString(gridBorder(widths, '=') + "\n")
	}
	for _, rc := range rowCells {
		b.WriteString(gridRow(rc, widths) + "\n")
		b.WriteString(border + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// spanCell is one cell's rendered text alongside the column span it covers
// (at least 1).
type spanCell struct {
	text string
	span int
}

func cellSpan(c richdoc.Cell) int {
	if c.ColSpan < 1 {
		return 1
	}
	return c.ColSpan
}

// spannedCols totals a row's logical column count, cells' spans included —
// necessarily >= len(cells), and equal to it only when nothing spans.
func spannedCols(cells []richdoc.Cell) int {
	n := 0
	for _, c := range cells {
		n += cellSpan(c)
	}
	return n
}

// flattenCellText collapses any newline in a cell's rendered text to a
// space — a grid table row is exactly one source line in this writer's
// output, so a literal "\n" (a multi-line cell's own wrapped content,
// preserved verbatim inside its richdoc.Text by Parse; or a richdoc.LineBreak,
// which writeInline renders as a literal newline for ordinary paragraph
// text) would otherwise split a table row across lines and corrupt the
// whole grid, not just that one cell. Same fix markdown's own
// renderTableCell already applies for the identical reason.
func flattenCellText(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

// spanCellTexts renders a row's cells to text+span pairs, padding the
// logical column count out to cols with empty unspanned cells (a short row,
// same as [richdoc.Table]'s own doc comment allows for a headerless or
// ragged table) so [gridRow]/[widenColumns] never need to special-case a
// row narrower than the table.
func spanCellTexts(w *writer, cells []richdoc.Cell, cols int) []spanCell {
	out := make([]spanCell, 0, len(cells))
	used := 0
	for _, c := range cells {
		if used >= cols {
			break
		}
		span := cellSpan(c)
		if used+span > cols {
			span = cols - used
		}
		out = append(out, spanCell{text: flattenCellText(w.writeInlines(c.Inlines)), span: span})
		used += span
	}
	for used < cols {
		out = append(out, spanCell{span: 1})
		used++
	}
	return out
}

// widenColumns widens each UNSPANNED cell's own column to fit its content —
// the same "establish widths from ordinary cells first" pass every table
// here has always done, just column-index-aware now that a row's cells
// don't map 1:1 to columns once something spans.
func widenColumns(widths []int, cells []spanCell) {
	col := 0
	for _, c := range cells {
		if c.span == 1 {
			if wd := runeLen(c.text); wd > widths[col] {
				widths[col] = wd
			}
		}
		col += c.span
	}
}

// widenSpannedColumns runs once unspanned widths are settled: if a spanning
// cell's own content is wider than the columns it covers already provide —
// spanTextWidth, the same quantity [gridRow] computes to pad against —
// widens the LAST column in its span to absorb the whole difference. Simple
// over an even split: correctness (the merged content still fits) matters
// here, not perfectly balanced column widths for a cell nothing else in the
// table constrains.
func widenSpannedColumns(widths []int, cells []spanCell) {
	col := 0
	for _, c := range cells {
		if c.span > 1 {
			need := runeLen(c.text) - spanTextWidth(widths, col, c.span)
			if need > 0 {
				widths[col+c.span-1] += need
			}
		}
		col += c.span
	}
}

// spanTextWidth is the padded-text-area width available to a cell spanning
// `span` columns starting at `col` — derived from gridBorder's own
// per-column "+2" convention: merging N columns removes N-1 interior "|"
// characters but each removal effectively donates 3 characters (the
// interior "|" plus the two single padding spaces that flanked it) to the
// merged content area, verified by matching total line length against
// gridBorder's unchanged output for a real docutils grid-table example.
func spanTextWidth(widths []int, col, span int) int {
	total := 3 * (span - 1)
	for i := col; i < col+span; i++ {
		total += widths[i]
	}
	return total
}

func gridBorder(widths []int, ch byte) string {
	var b strings.Builder
	b.WriteByte('+')
	for _, wd := range widths {
		b.WriteString(strings.Repeat(string(ch), wd+2))
		b.WriteByte('+')
	}
	return b.String()
}

func gridRow(cells []spanCell, widths []int) string {
	var b strings.Builder
	b.WriteByte('|')
	col := 0
	for _, c := range cells {
		// pad is never negative: spanTextWidth is computed from the same
		// widths widenColumns/widenSpannedColumns already grew to fit this
		// very cell (see writeTable).
		wd := spanTextWidth(widths, col, c.span)
		pad := wd - runeLen(c.text)
		b.WriteString(" " + c.text + strings.Repeat(" ", pad) + " |")
		col += c.span
	}
	return b.String()
}
