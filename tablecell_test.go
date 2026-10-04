package rst

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// roundTrip parses, writes and re-parses, returning the written reST and both
// trees' first table.
func roundTrip(t *testing.T, src string) (string, *richdoc.Document, *richdoc.Document) {
	t.Helper()
	d1, err := Parse([]byte(src))
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
	return string(out), d1, d2
}

// TestACellKeepsItsEscape pins the worst of this round's table defects, because
// its symptom is a DIAGNOSTIC becoming document content. PEP 624 writes "\(2)"
// in a cell precisely to stop reST reading an enumerated list there; the escape
// was not re-emitted, so the reconstruction read "(2)" as a list starting at 2
// and put docutils' own "Enumerated list start value not ordinal-1" INFO into
// the cell where the author had written "(2)".
func TestACellKeepsItsEscape(t *testing.T) {
	const src = "===========================  ===============\n" +
		"API                          Note\n" +
		"===========================  ===============\n" +
		"``PyUnicode_EncodeUTF7()``   \\(2)\n" +
		"``PyUnicode_EncodeUTF8()``   \\* not a bullet\n" +
		"``PyUnicode_EncodeUTF16()``  \\- not a dash\n" +
		"===========================  ===============\n"
	out, _, _ := roundTrip(t, src)
	for _, want := range []string{`\(2)`, `\* not a bullet`, `\- not a dash`} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	// The escape only matters because of what happens without it, so check that
	// too: nothing docutils would say about the reconstruction may appear in it.
	if strings.Contains(out, "ordinal-1") {
		t.Errorf("a diagnostic reached the document:\n%s", out)
	}
}

// TestACellKeepsItsOwnLines pins the biggest class by count: a cell's wrapped
// lines used to be collapsed to one, which corrupts nothing and loses nothing
// readable -- reST folds a wrap back to a space -- but re-wraps the cell, so 45
// of the 1564 real-world files came back with a different tree. Keeping the
// author's lines is what makes the tree a fixed point.
func TestACellKeepsItsOwnLines(t *testing.T) {
	const src = "===========  =============================================\n" +
		"Method       Description\n" +
		"===========  =============================================\n" +
		"S.remove(x)  Remove \"x\" from the set.  If \"x\" is not\n" +
		"             present, this method raises a LookupError\n" +
		"             exception.\n" +
		"S.add(x)     Add \"x\" to the set.\n" +
		"===========  =============================================\n"
	out, d1, d2 := roundTrip(t, src)
	if !reflect.DeepEqual(d1.Blocks, d2.Blocks) {
		t.Errorf("not a fixed point:\n%s", out)
	}
	// Every line of the written table must be the same length, or the grid is
	// malformed -- the property the multi-line row could most easily break.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for i, l := range lines {
		if len(l) != len(lines[0]) {
			t.Fatalf("line %d has length %d, want %d:\n%s", i, len(l), len(lines[0]), out)
		}
	}
}

// TestAMultiParagraphCellIsAFixedPoint covers the separator between two blocks
// in one cell. richdoc.Cell has no Blocks, so the structure is gone either way
// and a consumer sees whitespace either way; a blank line is what reST spells
// the break as, and unlike a space it survives a re-parse.
func TestAMultiParagraphCellIsAFixedPoint(t *testing.T) {
	const src = "+-----+-------------+\n| a   | first para  |\n|     |             |\n|     | second para |\n+-----+-------------+\n"
	out, d1, d2 := roundTrip(t, src)
	if !reflect.DeepEqual(d1.Blocks, d2.Blocks) {
		t.Errorf("not a fixed point:\n%s", out)
	}
	if !strings.Contains(out, "first para") || !strings.Contains(out, "second para") {
		t.Errorf("a paragraph went missing:\n%s", out)
	}
}

// TestALiteralBlockInACellIsALiteralBlock is what richdoc v0.5.0 made possible,
// and it replaces a guard rather than adding to it.
//
// The guard it replaces (TestACellWithAVerbatimNewlineIsFlattened, removed here)
// forced a cell's whole content onto ONE line, because a newline inside a flattened
// literal is CONTENT and not a wrap: written as a real line break its indentation
// read as a block quote and the closing delimiter was lost, which gave PEP 307 an
// "Inline literal start-string without end-string" INSIDE the cell and PEP 720's
// six-line literal 26 diagnostics. The cost was stated in that test: one cell
// cannot be half multi-line, so the paragraph break went too.
//
// With Cell.Blocks the literal block is a literal block again, written as one, and
// neither of the two treatments the flattened path needs applies -- see cellText.
// The guard survives for the path that still flattens, immediately below.
func TestALiteralBlockInACellIsALiteralBlock(t *testing.T) {
	const src = "" +
		"+-----+------------------------------------+\n" +
		"| a   | or, if the update() call fails, :: |\n" +
		"|     |                                    |\n" +
		"|     |    for k, v in state.items():      |\n" +
		"|     |        setattr(obj, k, v)          |\n" +
		"+-----+------------------------------------+\n"
	out, d1, d2 := roundTrip(t, src)
	if strings.Contains(out, "start-string without end-string") {
		t.Errorf("a diagnostic reached the document:\n%s", out)
	}
	if !reflect.DeepEqual(d1.Blocks, d2.Blocks) {
		t.Errorf("not a fixed point:\n%s", out)
	}
	// The literal block's own lines, kept as lines and indented under a "::".
	for _, want := range []string{"::", "for k, v in state.items():", "setattr(obj, k, v)"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	// And the cell really does hold a CodeBlock now, not a Code inline.
	tbl, ok := d1.Blocks[0].(richdoc.Table)
	if !ok {
		t.Fatalf("first block is %T, want a Table", d1.Blocks[0])
	}
	cell := tbl.Rows[0][1]
	if len(cell.Blocks) == 0 {
		t.Fatalf("the cell carries no Blocks: %#v", cell)
	}
	if len(cell.Inlines) == 0 {
		t.Errorf("the cell carries Blocks but no Inlines, which breaks richdoc's own contract: %#v", cell)
	}
	var sawCode bool
	for _, b := range cell.Blocks {
		if _, ok := b.(richdoc.CodeBlock); ok {
			sawCode = true
		}
	}
	if !sawCode {
		t.Errorf("no CodeBlock among the cell's blocks: %#v", cell.Blocks)
	}
}

// TestACellWhoseInlinesHoldAVerbatimNewlineIsStillFlattened keeps the old guard
// alive for the path that still needs it: a Cell built with a multi-line Code
// inline and NO Blocks -- by hand, or by any producer that has not moved to
// richdoc v0.5.0. Written as a real line break that newline would end the literal.
func TestACellWhoseInlinesHoldAVerbatimNewlineIsStillFlattened(t *testing.T) {
	doc := richdoc.New().Add(richdoc.Table{
		Rows: [][]richdoc.Cell{{
			richdoc.Td(richdoc.Txt("a")),
			richdoc.Td(richdoc.Mono("for k, v in state.items():\n    setattr(obj, k, v)")),
		}},
	}).Doc()
	out, err := Write(doc)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, "for k, v") && !strings.Contains(l, "setattr") {
			t.Errorf("the literal was split across lines:\n%s", out)
		}
	}
	if n := countSystemMessages(t, out); n != 0 {
		t.Errorf("the output has %d diagnostic(s):\n%s", n, out)
	}
}

// TestEscapeBlockStartReachesEachParagraph pins the generalisation the blank
// line inside a cell made necessary: a line following a blank one is at a block
// start again. For a paragraph, which holds no blank line, this is exactly the
// previous line-0-only behaviour -- so the witness has to be a cell.
func TestEscapeBlockStartReachesEachParagraph(t *testing.T) {
	const src = "+-----+-----------------------+\n" +
		"| a   | first paragraph       |\n" +
		"|     |                       |\n" +
		"|     | \\- not a bullet       |\n" +
		"+-----+-----------------------+\n"
	out, _, _ := roundTrip(t, src)
	if !strings.Contains(out, `\- not a bullet`) {
		t.Errorf("the escape was lost, so the second paragraph becomes a bullet list:\n%s", out)
	}
}

// TestEscapeBlockStartLeavesALaterLineAlone is the boundary, and the reason the
// rule is per-PARAGRAPH rather than per-line: once a paragraph has started, a
// line shaped like a bullet is ordinary continuation text, and escaping it would
// put a visible backslash into the document.
func TestEscapeBlockStartLeavesALaterLineAlone(t *testing.T) {
	got := escapeBlockStart("first line here\n- looks like a bullet")
	if got != "first line here\n- looks like a bullet" {
		t.Errorf("a continuation line was escaped: %q", got)
	}
}

// TestCellKeepsItsLinesPerInlineKind exercises the guard on every inline kind
// that can carry a newline of its own, rather than only on the literal block the
// corpus happened to have. Each of these is CONTENT with a line break in it, so
// writing the cell across lines would put that break at a block start; a
// richdoc.Text's newline is a soft wrap and must not trip the guard.
func TestCellKeepsItsLinesPerInlineKind(t *testing.T) {
	cases := []struct {
		name string
		in   []richdoc.Inline
		keep bool
	}{
		{"a text's newline is a soft wrap", []richdoc.Inline{richdoc.Text{Value: "a\nb"}}, true},
		{"code", []richdoc.Inline{richdoc.Code{Value: "a\nb"}}, false},
		{"code on one line", []richdoc.Inline{richdoc.Code{Value: "ab"}}, true},
		{"math", []richdoc.Inline{richdoc.Math{TeX: "a\nb"}}, false},
		{"raw inline", []richdoc.Inline{richdoc.RawInline{Format: "rst", Text: "a\nb"}}, false},
		{"inside a link", []richdoc.Inline{richdoc.Link{URL: "u", Inlines: []richdoc.Inline{richdoc.Code{Value: "a\nb"}}}}, false},
		{"inside emphasis", []richdoc.Inline{richdoc.Emph{Inlines: []richdoc.Inline{richdoc.Code{Value: "a\nb"}}}}, false},
		{"inside strong", []richdoc.Inline{richdoc.Strong{Inlines: []richdoc.Inline{richdoc.Code{Value: "a\nb"}}}}, false},
		{"inside an anchor", []richdoc.Inline{richdoc.Anchor{ID: "x", Inlines: []richdoc.Inline{richdoc.Code{Value: "a\nb"}}}}, false},
		{"a link with nothing verbatim", []richdoc.Inline{richdoc.Link{URL: "u", Inlines: []richdoc.Inline{richdoc.Text{Value: "a\nb"}}}}, true},
	}
	for _, c := range cases {
		if got := cellKeepsItsLines(c.in); got != c.keep {
			t.Errorf("%s: cellKeepsItsLines = %v, want %v", c.name, got, c.keep)
		}
	}
}

// TestEscapeBlockStartEscapesAnAdornmentAfterABlankLine covers the adornment
// branch at the second block start, which a cell can now reach: four or more of
// one punctuation character is a transition or a section underline, and left bare
// it would end the cell's paragraph and start a section.
func TestEscapeBlockStartEscapesAnAdornmentAfterABlankLine(t *testing.T) {
	got := escapeBlockStart("first paragraph\n\n++++")
	if got != "first paragraph\n\n\\++++" {
		t.Errorf("escapeBlockStart = %q, want the adornment escaped", got)
	}
	// A blank line with spaces in it still ends the block, and the line after it
	// is still a block start.
	if got := escapeBlockStart("first\n   \n- bullet"); got != "first\n   \n\\- bullet" {
		t.Errorf("a whitespace-only line did not end the block: %q", got)
	}
}
