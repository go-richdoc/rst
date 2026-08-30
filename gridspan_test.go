// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// gridSpanSource is docutils' own GridTableParser docstring example: a
// header, a plain row, a column-spanning row, and a row-spanning pair —
// the same corpus used to verify grid-table PARSING earlier in
// docutils/rst itself, reused here to verify this package's own Parse ->
// Write -> Parse round-trip for ColSpan/RowSpan (richdoc v0.3.0+).
const gridSpanSource = "+------------------------+------------+----------+----------+\n" +
	"| Header row, column 1   | Header 2   | Header 3 | Header 4 |\n" +
	"+========================+============+==========+==========+\n" +
	"| body row 1, column 1   | column 2   | column 3 | column 4 |\n" +
	"+------------------------+------------+----------+----------+\n" +
	"| body row 2             | Cells may span columns.          |\n" +
	"+------------------------+------------+---------------------+\n" +
	"| body row 3             | Cells may  | - Table cells       |\n" +
	"+------------------------+ span rows. | - contain           |\n" +
	"| body row 4             |            | - body elements.    |\n" +
	"+------------------------+------------+---------------------+\n"

func TestParseColSpanRowSpan(t *testing.T) {
	doc, err := Parse([]byte(gridSpanSource))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tbl := doc.Blocks[0].(richdoc.Table)
	colSpanCell := tbl.Rows[1][1] // "Cells may span columns."
	if colSpanCell.ColSpan != 3 {
		t.Errorf("colspan cell: ColSpan = %d, want 3", colSpanCell.ColSpan)
	}
	rowSpanCell := tbl.Rows[2][1] // "Cells may span rows."
	if rowSpanCell.RowSpan != 2 {
		t.Errorf("rowspan cell: RowSpan = %d, want 2", rowSpanCell.RowSpan)
	}
	bothCell := tbl.Rows[2][2] // "Table cells contain body elements."
	if bothCell.ColSpan != 2 || bothCell.RowSpan != 2 {
		t.Errorf("colspan+rowspan cell: ColSpan=%d RowSpan=%d, want 2 and 2", bothCell.ColSpan, bothCell.RowSpan)
	}
}

// TestWriteColSpanRoundTrips guards the actual bug this feature shipped
// with: an empty PADDING cell (one of richdoc.Table's own short-row/
// spanned-over slots — see spanCellTexts) came out one character narrower
// than its column, because gridRow's padding math used displayWidth
// (which floors at 1, a heading-underline-specific rule) instead of a
// plain rune count for a cell that is genuinely zero characters wide. That
// misaligned every column after the first empty one in that row badly
// enough that re-parsing read it as a line_block, not a table row at all.
func TestWriteColSpanRoundTrips(t *testing.T) {
	d1, err := Parse([]byte(gridSpanSource))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Write(d1)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	d2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	if len(d2.Blocks) != 1 {
		t.Fatalf("re-parsed output is not a single table block (got %d blocks) — rewritten source:\n%s", len(d2.Blocks), out)
	}
	if _, ok := d2.Blocks[0].(richdoc.Table); !ok {
		t.Fatalf("re-parsed block is a %T, not a Table — rewritten source:\n%s", d2.Blocks[0], out)
	}
	tbl2 := d2.Blocks[0].(richdoc.Table)
	if tbl2.Rows[1][1].ColSpan != 3 {
		t.Errorf("colspan lost on round-trip: got %d, want 3", tbl2.Rows[1][1].ColSpan)
	}
}

// TestWriteEmptyCellPadding is the minimal, direct reproduction of the same
// bug TestWriteColSpanRoundTrips catches end-to-end: every output line of a
// grid table must be exactly as wide as its border line, even when a cell
// is genuinely empty (richdoc.Table's own short-row padding, or a
// row-span's continuation slot).
func TestWriteEmptyCellPadding(t *testing.T) {
	doc := richdoc.New().Table(
		nil,
		nil,
		[][]richdoc.Cell{
			{richdoc.Td(richdoc.Txt("a")), {}, richdoc.Td(richdoc.Txt("c"))},
		},
	).Doc()
	out, err := Write(doc)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	want := len(lines[0])
	for i, l := range lines {
		if len(l) != want {
			t.Fatalf("line %d has length %d, want %d (matching the border line) — full output:\n%s", i, len(l), want, out)
		}
	}
}

// TestParseNestedListInCellDoesNotConcatenateWords guards the second bug
// this feature surfaced: a cell holding more than one top-level block (a
// bullet list, here — richdoc.Cell can only hold inline content, so this
// is always somewhat lossy, see cellInlines) must still separate those
// blocks with a space. The very first version ran every word together
// with no separator at all, since convertInlines has no concept of a block
// boundary to insert one at.
func TestParseNestedListInCellDoesNotConcatenateWords(t *testing.T) {
	doc, err := Parse([]byte(gridSpanSource))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tbl := doc.Blocks[0].(richdoc.Table)
	cell := tbl.Rows[2][2] // the bulleted "Table cells / contain / body elements." cell
	got := plainTextOf(cell.Inlines)
	want := "Table cells contain body elements."
	if got != want {
		t.Errorf("cell text = %q, want %q (words ran together with no separator)", got, want)
	}
}

// TestParseMultiParagraphCellJoinsWithSpace covers cellInlines' other real
// case besides a bulleted cell: a cell holding more than one PARAGRAPH
// (also valid grid-table content, not just lists).
func TestParseMultiParagraphCellJoinsWithSpace(t *testing.T) {
	src := "+-----+-------------+\n| a   | first para  |\n|     |             |\n|     | second para |\n+-----+-------------+\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tbl := doc.Blocks[0].(richdoc.Table)
	got := plainTextOf(tbl.Rows[0][1].Inlines)
	want := "first para second para"
	if got != want {
		t.Errorf("multi-paragraph cell text = %q, want %q", got, want)
	}
}

func plainTextOf(inlines []richdoc.Inline) string {
	var b strings.Builder
	for _, in := range inlines {
		if t, ok := in.(richdoc.Text); ok {
			b.WriteString(t.Value)
		}
	}
	return b.String()
}

// TestParseLiteralBlockInCellBecomesCode covers cellBlockInlines'
// TagLiteralBlock case: a cell whose second block is a literal block (a
// paragraph ending in "::" followed by an indented block) degrades to an
// inline richdoc.Code span, joined by a space after the leading paragraph
// like any other second top-level block in a cell.
func TestParseLiteralBlockInCellBecomesCode(t *testing.T) {
	src := "+-----+------------+\n| a   | code here::|\n|     |            |\n|     |    x = 1   |\n+-----+------------+\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tbl := doc.Blocks[0].(richdoc.Table)
	cell := tbl.Rows[0][1]
	found := false
	for _, in := range cell.Inlines {
		if c, ok := in.(richdoc.Code); ok && c.Value == "x = 1" {
			found = true
		}
	}
	if !found {
		t.Errorf("cell Inlines has no Code(%q): %#v", "x = 1", cell.Inlines)
	}
}

func TestCellInlinesSingleParagraphUnaffected(t *testing.T) {
	// The overwhelming common case — a cell holding exactly one paragraph —
	// must produce IDENTICAL output to before cellInlines existed: no
	// leading/trailing space, no behavior change at all.
	d1, err := Parse([]byte("=====  =====\na      b\n=====  =====\n1      2\n=====  =====\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tbl := d1.Blocks[0].(richdoc.Table)
	want := richdoc.Table{
		Align:  []richdoc.Alignment{richdoc.AlignDefault, richdoc.AlignDefault},
		Header: []richdoc.Cell{richdoc.Td(richdoc.Txt("a")), richdoc.Td(richdoc.Txt("b"))},
		Rows:   [][]richdoc.Cell{{richdoc.Td(richdoc.Txt("1")), richdoc.Td(richdoc.Txt("2"))}},
	}
	if !reflect.DeepEqual(tbl, want) {
		t.Errorf("simple table = %#v, want %#v", tbl, want)
	}
}
