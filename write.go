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

// displayWidth returns the rune count of the LAST LINE of s (a heading's
// text is always one line in practice; len(s) in bytes would under-count a
// multi-byte title and produce an underline shorter than docutils requires).
func displayWidth(s string) int {
	n := 0
	for range s {
		n++
	}
	if n < 1 {
		return 1
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
func (w *writer) writeTable(t richdoc.Table) string {
	cols := len(t.Header)
	for _, row := range t.Rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return ""
	}
	widths := make([]int, cols)
	headerText := w.cellTexts(t.Header, cols)
	for i, s := range headerText {
		widths[i] = max(widths[i], displayWidth(s))
	}
	rowTexts := make([][]string, len(t.Rows))
	for r, row := range t.Rows {
		rowTexts[r] = w.cellTexts(row, cols)
		for i, s := range rowTexts[r] {
			widths[i] = max(widths[i], displayWidth(s))
		}
	}
	for i, wd := range widths {
		if wd < 1 {
			widths[i] = 1
		}
	}

	var b strings.Builder
	border := gridBorder(widths, '-')
	b.WriteString(border + "\n")
	if len(t.Header) > 0 {
		b.WriteString(gridRow(headerText, widths) + "\n")
		b.WriteString(gridBorder(widths, '=') + "\n")
	}
	for _, rt := range rowTexts {
		b.WriteString(gridRow(rt, widths) + "\n")
		b.WriteString(border + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (w *writer) cellTexts(cells []richdoc.Cell, cols int) []string {
	out := make([]string, cols)
	for i := 0; i < cols; i++ {
		if i < len(cells) {
			out[i] = w.writeInlines(cells[i].Inlines)
		}
	}
	return out
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

func gridRow(cells []string, widths []int) string {
	var b strings.Builder
	b.WriteByte('|')
	for i, wd := range widths {
		cell := cells[i]
		// pad is never negative: wd is the max displayWidth across the
		// column, including this very cell (see writeTable).
		pad := wd - displayWidth(cell)
		b.WriteString(" " + cell + strings.Repeat(" ", pad) + " |")
	}
	return b.String()
}
