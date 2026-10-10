// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestWriteRowSpanRoundTrips is the case the grid writer used to fail by
// DESIGN: it kept RowSpan on the cell (so nothing was lost from the tree) and
// then drew the cell as an ordinary one, which both re-wrapped its text and
// shifted every cell of the row below one column to the left. Eight of the
// 1564 real-world files held a table that came back with different spans
// because of it.
//
// The corpus is docutils' own GridTableParser docstring example, already used
// by TestParseColSpanRowSpan for the parse side.
func TestWriteRowSpanRoundTrips(t *testing.T) {
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
	tbl, ok := d2.Blocks[0].(richdoc.Table)
	if !ok {
		t.Fatalf("re-parsed block is a %T, not a Table — rewritten source:\n%s", d2.Blocks[0], out)
	}
	if len(tbl.Rows) != 4 {
		t.Fatalf("re-parsed table has %d rows, want 4 — rewritten source:\n%s", len(tbl.Rows), out)
	}
	if got := tbl.Rows[2][1].RowSpan; got != 2 {
		t.Errorf("row span lost: Rows[2][1].RowSpan = %d, want 2 — rewritten source:\n%s", got, out)
	}
	if got := tbl.Rows[2][2]; got.RowSpan != 2 || got.ColSpan != 2 {
		t.Errorf("both spans: Rows[2][2] ColSpan=%d RowSpan=%d, want 2 and 2 — rewritten source:\n%s", got.ColSpan, got.RowSpan, out)
	}
	// The row UNDER a row span holds fewer cells than the table has columns,
	// and that is the half of the bug a span check alone does not see: the
	// cells that remain have to land in the columns the span left free.
	if n := len(tbl.Rows[3]); n != 1 {
		t.Errorf("the row under the spans has %d cells, want 1 — rewritten source:\n%s", n, out)
	}
}

// TestWriteRowSpanDrawsDocutilsShape pins the drawn frame, because two
// characters of it are easy to get wrong and no round-trip check can tell
// which of the two mistakes was made:
//
//   - a spanning cell's own text keeps flowing across the rule that interrupts
//     it ("size" sits ON a border line), and
//   - the "+" that opens that rule belongs to the cells to its RIGHT, where
//     the span's own edge is a "|".
//
// The shape is docutils' own, taken from its GridTableParser docstring.
func TestWriteRowSpanDrawsDocutilsShape(t *testing.T) {
	src := "+-------+-------------------+\n" +
		"|string | Python 3.2        |\n" +
		"|size   +--------+----------+\n" +
		"|       | 16-bit | 32-bit   |\n" +
		"+-------+--------+----------+\n" +
		"|1      | 32     | 64       |\n" +
		"+-------+--------+----------+\n"
	want := "+--------+--------+--------+\n" +
		"| string | Python 3.2      |\n" +
		"| size   +--------+--------+\n" +
		"|        | 16-bit | 32-bit |\n" +
		"+--------+--------+--------+\n" +
		"| 1      | 32     | 64     |\n" +
		"+--------+--------+--------+\n"
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Write(d)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if string(out) != want {
		t.Errorf("written table:\n%s\nwant:\n%s", out, want)
	}
}

// TestWritePhantomRowDoesNotSurvive covers the one real-world table that is
// NOT a fixed point for a reason the writer cannot remove: PEP 669's table has
// a typo ("+ Two or more |" for "| Two or more |"), docutils answers by
// reading the final rule's row as covered by a span from above, and its own
// doctree carries that empty <row> too — so the parser is right and reST
// simply has no spelling for a row that holds no cells of its own.
//
// What must hold is that the writer does not try: drawing the span over an
// empty row produces a cell three lines tall whose own reconstruction is one
// line tall, so the OUTPUT would not be a fixed point either. Dropping the
// phantom row costs nothing that was ever drawn.
func TestWritePhantomRowDoesNotSurvive(t *testing.T) {
	doc := richdoc.New().Table(
		nil,
		nil,
		[][]richdoc.Cell{
			{{Inlines: []richdoc.Inline{richdoc.Txt("a")}, RowSpan: 2}, {Inlines: []richdoc.Inline{richdoc.Txt("b")}, RowSpan: 2}},
			{},
		},
	).Doc()
	out, err := Write(doc)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n := strings.Count(string(out), "\n"); n != 3 {
		t.Errorf("written table has %d lines, want 3 (the phantom row drew itself):\n%s", n, out)
	}
	d2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	again, err := Write(d2)
	if err != nil {
		t.Fatalf("re-Write: %v", err)
	}
	if string(again) != string(out) {
		t.Errorf("not a fixed point:\n%s\nbecame:\n%s", out, again)
	}
}

// TestWriteRaggedRowKeepsItsColumns guards what placeGrid replaced: rows used
// to be padded out to the table's width BEFORE anything was placed, which is
// only right while no span has moved the cells along. A short row's holes have
// to be filled where they actually fall.
func TestWriteRaggedRowKeepsItsColumns(t *testing.T) {
	doc := richdoc.New().Table(
		nil,
		nil,
		[][]richdoc.Cell{
			{{Inlines: []richdoc.Inline{richdoc.Txt("tall")}, RowSpan: 2}, {Inlines: []richdoc.Inline{richdoc.Txt("b")}}, {Inlines: []richdoc.Inline{richdoc.Txt("c")}}},
			{{Inlines: []richdoc.Inline{richdoc.Txt("e")}}},
		},
	).Doc()
	out, err := Write(doc)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	d2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	tbl, ok := d2.Blocks[0].(richdoc.Table)
	if !ok {
		t.Fatalf("re-parsed block is a %T, not a Table:\n%s", d2.Blocks[0], out)
	}
	if len(tbl.Rows) != 2 || len(tbl.Rows[1]) != 2 {
		t.Fatalf("re-parsed shape is %d rows, second row %d cells, want 2 and 2:\n%s", len(tbl.Rows), len(tbl.Rows[1]), out)
	}
	// "e" went into the column the span left free, and the hole after it is
	// its own empty cell rather than a missing one.
	if got := plainTextOf(tbl.Rows[1][0].Inlines); got != "e" {
		t.Errorf("Rows[1][0] = %q, want %q — the short row moved:\n%s", got, "e", out)
	}
	// Every line is as wide as the frame, which is what a hole left unfilled
	// would break.
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	for i, l := range lines {
		if len(l) != len(lines[0]) {
			t.Fatalf("line %d is %d characters, want %d:\n%s", i, len(l), len(lines[0]), out)
		}
	}
}
