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

// TestACellWithAVerbatimNewlineIsFlattened is the guard, and the reason it
// exists. A newline inside a literal is CONTENT, not a wrap: written as a real
// line break, its indentation reads as a block quote and the closing delimiter
// is lost. PEP 307's cell -- a literal BLOCK ("... fails, ::" and an indented
// block, which cellBlockInlines turns into a richdoc.Code whose Value carries
// the newline) -- came back with "Inline literal start-string without
// end-string" INSIDE the cell, and PEP 720's six-line literal gained 26
// diagnostics.
//
// Neither was visible to the round-trip probe, which is BINARY: both files
// already did not round-trip, so they stayed "lossy" while getting much worse.
// What saw it was counting the diagnostics the reconstruction introduces -- 30
// messages before the multi-line change, 54 after, 27 with this guard.
//
// Against the code as it was BEFORE this round it passes trivially -- every cell
// was flattened then, so there was nothing to guard. Its baseline is multi-line
// cells WITHOUT the guard, and the evidence for it is that corpus count, not
// this case.
func TestACellWithAVerbatimNewlineIsFlattened(t *testing.T) {
	const src = "" +
		"+-----+------------------------------------+\n" +
		"| a   | or, if the update() call fails, :: |\n" +
		"|     |                                    |\n" +
		"|     |    for k, v in state.items():      |\n" +
		"|     |        setattr(obj, k, v)          |\n" +
		"+-----+------------------------------------+\n"
	out, _, _ := roundTrip(t, src)
	if strings.Contains(out, "start-string without end-string") {
		t.Errorf("a diagnostic reached the document:\n%s", out)
	}
	// The literal has to come out on ONE line, which is what keeps it a literal.
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "for k, v") && !strings.Contains(l, "setattr") {
			t.Errorf("the literal was split across lines:\n%s", out)
		}
	}
	// And the cell's own paragraph break is flattened with it -- one cell
	// cannot be half multi-line. That is the cost of the guard, stated here so
	// it is a decision and not a surprise.
	content := 0
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "|") {
			content++
		}
	}
	if content != 1 {
		t.Errorf("expected exactly one content line, got %d:\n%s", content, out)
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
