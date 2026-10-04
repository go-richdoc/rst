package rst

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestACellKeepsItsBlocks pins what richdoc v0.5.0's Cell.Blocks is for. reST's
// grid tables allow full block content in a cell, and before the model could hold
// it every cell was flattened to a run of inlines: a bullet list in a cell came back
// as two paragraphs, a literal block as an inline literal, a line block as prose.
//
// Measured over the 1564-file corpus: 64 list items, 61 line-block lines, 35
// literal blocks, 27 bullet lists, 24 enumerated-list and colspec nodes and 10
// images, in 24 files.
func TestACellKeepsItsBlocks(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		{
			"a bullet list in a cell",
			"+---+------------+\n| a | - Greg     |\n|   | - Barry    |\n+---+------------+\n",
			[]string{"- Greg", "- Barry"},
		},
		{
			"a line block in a cell",
			"+---+-----------+\n| a | | one     |\n|   | | two     |\n+---+-----------+\n",
			[]string{"| one", "| two"},
		},
		{
			"an enumerated list in a cell",
			"+---+-----------+\n| a | 1. first  |\n|   | 2. second |\n+---+-----------+\n",
			[]string{"1. first", "2. second"},
		},
	}
	for _, c := range cases {
		out, d1, d2 := roundTrip(t, c.src)
		if !reflect.DeepEqual(d1.Blocks, d2.Blocks) {
			t.Errorf("%s: not a fixed point:\n%s", c.name, out)
		}
		for _, want := range c.want {
			if !strings.Contains(out, want) {
				t.Errorf("%s: want %q in:\n%s", c.name, want, out)
			}
		}
	}
}

// TestACellWithOneParagraphCarriesNoBlocks is the other half of the contract, and a
// CONTROL on the one above: the common case must stay exactly as it was, Inlines
// only. Filling Blocks for every cell would make every consumer choose between two
// spellings of the same content.
func TestACellWithOneParagraphCarriesNoBlocks(t *testing.T) {
	d, err := Parse([]byte("+---+---+\n| a | b |\n+---+---+\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tbl := d.Blocks[0].(richdoc.Table)
	for _, row := range tbl.Rows {
		for _, cell := range row {
			if len(cell.Blocks) != 0 {
				t.Errorf("a one-paragraph cell carries Blocks: %#v", cell)
			}
			if len(cell.Inlines) == 0 {
				t.Errorf("a one-paragraph cell carries no Inlines: %#v", cell)
			}
		}
	}
}

// TestACellThatCarriesBlocksAlsoCarriesInlines pins richdoc v0.5.0's own contract
// from the producer's side: a consumer that does not read Blocks must still find
// the cell's words in Inlines, or this change would make every such converter
// render an empty cell.
func TestACellThatCarriesBlocksAlsoCarriesInlines(t *testing.T) {
	d, err := Parse([]byte("+---+--------+\n| a | - one  |\n|   | - two  |\n+---+--------+\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cell := d.Blocks[0].(richdoc.Table).Rows[0][1]
	if len(cell.Blocks) == 0 {
		t.Fatalf("the cell carries no Blocks: %#v", cell)
	}
	if len(cell.Inlines) == 0 {
		t.Fatalf("the cell carries Blocks but no Inlines: %#v", cell)
	}
	if !strings.Contains(richdoc.PlainText(d), "one") {
		t.Errorf("the cell's words are not reachable as text: %q", richdoc.PlainText(d))
	}
}

// TestATableKeepsItsCaption pins the other field richdoc v0.5.0 added. A caption is
// the ".. table::" directive's argument -- the only spelling reST has for one -- and
// docutils puts it back as the <title> child this converter reads it from. 24
// captions in 13 corpus files had nowhere to go before.
func TestATableKeepsItsCaption(t *testing.T) {
	const src = ".. table:: Should be Table 1\n\n   +---+---+\n   | a | b |\n   +---+---+\n"
	out, d1, d2 := roundTrip(t, src)
	if !reflect.DeepEqual(d1.Blocks, d2.Blocks) {
		t.Errorf("not a fixed point:\n%s", out)
	}
	if !strings.Contains(out, ".. table:: Should be Table 1") {
		t.Errorf("the caption is gone:\n%s", out)
	}
	if got := d1.Blocks[0].(richdoc.Table).Caption; len(got) == 0 {
		t.Errorf("the model carries no caption: %#v", d1.Blocks[0])
	}
}

// TestATableWithoutACaptionGetsNoDirective is the control for the caption: a plain
// grid table must not grow a ".. table::" wrapper. It passes either way.
func TestATableWithoutACaptionGetsNoDirective(t *testing.T) {
	out, _, _ := roundTrip(t, "+---+---+\n| a | b |\n+---+---+\n")
	if strings.Contains(out, ".. table::") {
		t.Errorf("a plain table gained a directive:\n%s", out)
	}
}
