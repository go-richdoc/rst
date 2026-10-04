package rst

import (
	"reflect"
	"strings"
	"testing"
)

// TestANestedTableSurvives pins the fourth construct to vanish through
// rawChildSource's fallback, after the figure, the image and the directive: a
// <table>. It fell to AsText, which is the cells' words with no table around them.
//
// PEP 249 puts all three of its tables inside DEFINITIONS and lost every one -- 34
// entries, 18 rows and 8 colspecs in that file alone, 64 nodes across the corpus.
func TestANestedTableSurvives(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		{
			"a simple table in a definition",
			"term\n   Body.\n\n   ==== ====\n   a    b\n   ==== ====\n   1    2\n   ==== ====\n",
			[]string{"+===+===+", "| a | b |", "| 1 | 2 |"},
		},
		{
			"a grid table in a container",
			".. container:: c\n\n   +---+---+\n   | a | b |\n   +---+---+\n",
			[]string{"| a | b |"},
		},
		{
			"a table in an admonition",
			".. note::\n\n   ==== ====\n   a    b\n   ==== ====\n",
			[]string{"| a | b |"},
		},
	}
	for _, c := range cases {
		out, msgs := reconstruct(t, c.src)
		if before := countSystemMessages(t, []byte(c.src)); msgs > before {
			t.Errorf("%s: the reconstruction gained %d diagnostic(s):\n%s", c.name, msgs-before, out)
		}
		for _, want := range c.want {
			if !strings.Contains(out, want) {
				t.Errorf("%s: want %q in:\n%s", c.name, want, out)
			}
		}
	}
}

// TestANestedTableKeepsACellsBlocks is the two features meeting: a table nested in a
// definition, whose own cell holds a bullet list. Before this round the table was
// gone; before richdoc v0.5.0 the list inside it would have been flattened.
func TestANestedTableKeepsACellsBlocks(t *testing.T) {
	const src = "term\n   +---+--------+\n   | a | - one  |\n   |   | - two  |\n   +---+--------+\n"
	out, d1, d2 := roundTrip(t, src)
	if !reflect.DeepEqual(d1.Blocks, d2.Blocks) {
		t.Errorf("not a fixed point:\n%s", out)
	}
	for _, want := range []string{"- one", "- two"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}

// TestATopLevelTableIsUnchanged is the CONTROL: a table at the top of a document
// never went through this path and must keep coming out as it did. Passes either
// way.
func TestATopLevelTableIsUnchanged(t *testing.T) {
	out, _, _ := roundTrip(t, "==== ====\na    b\n==== ====\n1    2\n==== ====\n")
	for _, want := range []string{"| a | b |", "| 1 | 2 |"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}
