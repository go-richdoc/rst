package rst

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestAClassSurvivesOnEachBlockThatCanHoldOne pins what richdoc v0.4.0 made
// possible. Before those fields, a ":class:" option, a ".. class::" directive and
// sphinx's ".. rst-class::" reached this converter with nowhere to go: 187 author
// class attributes in 57 of the 1564 corpus files.
//
// The writer uses the CONTENT form of ".. class::" (the directive with the block
// indented under it), not the bare form. docutils applies a bare one to the NEXT
// element through a transform this project's parser does not run, so the bare
// form leaves a <pending> and the paragraph gets no class at all -- checked
// against the reference on both spellings.
func TestAClassSurvivesOnEachBlockThatCanHoldOne(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"paragraph", ".. class:: foo bar\n\n   A paragraph.\n", "foo bar"},
		{"bullet list", ".. class:: quux\n\n   - one\n   - two\n", "quux"},
		{"code block", ".. code:: python\n   :class: highlighted\n\n   x = 1\n", "highlighted"},
	}
	for _, c := range cases {
		out, msgs := reconstruct(t, c.src)
		if msgs != 0 {
			t.Errorf("%s: the reconstruction gained %d diagnostic(s):\n%s", c.name, msgs, out)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: the class was lost:\n%s", c.name, out)
		}
	}
}

// TestAClassedBlockQuoteKeepsItsQuote is the case that makes the rule above
// conditional. A block quote is written by INDENTING its content, and indenting
// that again under ".. class::" loses the quote: the body lands at six spaces and
// reparses as one PARAGRAPH with the class. So its own directive carries the class
// instead -- and every block-quote class in the corpus is one of docutils' three
// (6 epigraph, 1 pull-quote).
func TestAClassedBlockQuoteKeepsItsQuote(t *testing.T) {
	for _, d := range []string{"epigraph", "highlights", "pull-quote"} {
		src := ".. " + d + "::\n\n   Quoted text.\n"
		out, msgs := reconstruct(t, src)
		if msgs != 0 {
			t.Errorf("%s: the reconstruction gained %d diagnostic(s):\n%s", d, msgs, out)
		}
		if !strings.Contains(out, ".. "+d+"::") {
			t.Errorf("%s: want the directive back, got:\n%s", d, out)
		}
		if strings.Contains(out, ".. class::") {
			t.Errorf("%s: a class wrapper loses the quote:\n%s", d, out)
		}
		// The quote itself has to survive, which is the whole point.
		d2, err := Parse([]byte(out))
		if err != nil {
			t.Fatalf("%s: re-Parse: %v", d, err)
		}
		if _, ok := d2.Blocks[0].(richdoc.BlockQuote); !ok {
			t.Errorf("%s: the quote became %T:\n%s", d, d2.Blocks[0], out)
		}
	}
}

// TestADerivedTableClassIsNotWrittenBack pins the other filter. "colwidths-given"
// comes from ".. table::"'s ":widths:" option and "colwidths-auto" from its
// absence -- a plain grid table carries neither, checked -- so 70 of the corpus's
// 83 table classes are docutils' own working-out. Writing one back as an explicit
// ":class:" would state as the author's what the parser derived.
func TestADerivedTableClassIsNotWrittenBack(t *testing.T) {
	const src = ".. table::\n   :widths: 10 20\n\n   +---+---+\n   | a | b |\n   +---+---+\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Write(doc)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if strings.Contains(string(out), "colwidths") {
		t.Errorf("a derived class was written back as the author's:\n%s", out)
	}
	// An author's own table class still is.
	doc2, err := Parse([]byte(".. class:: longtable\n\n   +---+---+\n   | a | b |\n   +---+---+\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out2, err := Write(doc2)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(string(out2), "longtable") {
		t.Errorf("an author's table class was lost:\n%s", out2)
	}
}

// TestAnImageKeepsEverySizeItWasGiven pins the other half of richdoc v0.4.0. 43
// of the 79 standalone images in the corpus carry at least one of these, in 18
// files, and every one used to be dropped.
func TestAnImageKeepsEverySizeItWasGiven(t *testing.T) {
	const src = ".. image:: pic.png\n" +
		"   :alt: Some alt text.\n" +
		"   :height: 3em\n" +
		"   :width: 30%\n" +
		"   :scale: 50\n" +
		"   :align: center\n" +
		"   :class: invert-in-dark-mode\n\nBody.\n"
	out, msgs := reconstruct(t, src)
	if msgs != 0 {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs, out)
	}
	for _, want := range []string{":height: 3em", ":width: 30%", ":scale: 50", ":align: center", ":class: invert-in-dark-mode"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	// And the values reach the MODEL as written, units and all, so a converter
	// for another format has what it needs.
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var img richdoc.Image
	for _, in := range doc.Blocks[0].(richdoc.Paragraph).Inlines {
		if v, ok := in.(richdoc.Image); ok {
			img = v
		}
	}
	want := richdoc.Image{
		URL: "pic.png", Alt: "Some alt text.", Height: "3em", Width: "30%",
		Scale: 50, Align: richdoc.AlignCenter, Classes: []string{"invert-in-dark-mode"},
	}
	if !reflect.DeepEqual(img, want) {
		t.Errorf("Image =\n%#v\nwant:\n%#v", img, want)
	}
}

// TestANestedCodeBlockKeepsItsLanguage pins a loss the class work uncovered: the
// raw-source path wrote a bare "::" for every literal block, so a
// ".. code:: python" nested inside a list item, an admonition or a definition came
// back unlabelled. 16 corpus files, and invisible while the class was being
// subtracted from the comparison.
func TestANestedCodeBlockKeepsItsLanguage(t *testing.T) {
	cases := []struct{ name, src string }{
		{"in a list item", "- item\n\n  .. code:: python\n\n     x = 1\n"},
		{"in an admonition", ".. note::\n\n   .. code:: python\n\n      x = 1\n"},
		{"in a definition", "term\n   .. code:: python\n\n      x = 1\n"},
	}
	for _, c := range cases {
		out, msgs := reconstruct(t, c.src)
		if msgs != 0 {
			t.Errorf("%s: the reconstruction gained %d diagnostic(s):\n%s", c.name, msgs, out)
		}
		if !strings.Contains(out, "code:: python") {
			t.Errorf("%s: the language was lost:\n%s", c.name, out)
		}
	}
}
