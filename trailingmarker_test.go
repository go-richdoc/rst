// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestTrailingDoubleColonSurvives is PEP 473's shape. The source writes THREE
// colons:
//
//	allowing to display the following:::
//
// docutils takes the last pair as the literal-block marker and keeps one of
// them, so the paragraph's text ends "following::" — two colons the author
// wrote. Writing those back bare made the next parse reduce them again, to
// one: a character of the document lost once per round trip, until it
// stabilised a colon short.
func TestTrailingDoubleColonSurvives(t *testing.T) {
	const src = "allowing to display the following:::\n\n  a = {}\n  b = 1\n"
	d1, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p1, ok := d1.Blocks[0].(richdoc.Paragraph)
	if !ok {
		t.Fatalf("first block is a %T, not a Paragraph", d1.Blocks[0])
	}
	want := plainTextOf(p1.Inlines)
	if !strings.HasSuffix(want, "::") {
		t.Fatalf("the parse did not keep two colons: %q", want)
	}
	out, err := Write(d1)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	d2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	p2, ok := d2.Blocks[0].(richdoc.Paragraph)
	if !ok {
		t.Fatalf("re-parsed first block is a %T — written as:\n%s", d2.Blocks[0], out)
	}
	if got := plainTextOf(p2.Inlines); got != want {
		t.Errorf("text %q -> %q — written as:\n%s", want, got, out)
	}
	// And the code block is still a code block: the escape must not take the
	// marker the writer emits for it.
	if len(d2.Blocks) != len(d1.Blocks) {
		t.Fatalf("%d blocks -> %d — written as:\n%s", len(d1.Blocks), len(d2.Blocks), out)
	}
	if _, ok := d2.Blocks[1].(richdoc.CodeBlock); !ok {
		t.Errorf("the code block came back as a %T — written as:\n%s", d2.Blocks[1], out)
	}
}

// TestTrailingColonInACellSurvives covers the other place a flattened text
// block ends up next to an indented one: reST reads a cell's content as its
// own block fragment, so a cell ending in "::" marks a literal block inside
// the cell.
func TestTrailingColonInACellSurvives(t *testing.T) {
	doc := richdoc.New().Table(
		nil, nil,
		[][]richdoc.Cell{{richdoc.Td(richdoc.Txt("see this::")), richdoc.Td(richdoc.Txt("b"))}},
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
		t.Fatalf("re-parsed a %T — written as:\n%s", d2.Blocks[0], out)
	}
	if got := plainTextOf(tbl.Rows[0][0].Inlines); got != "see this::" {
		t.Errorf("cell text = %q, want %q — written as:\n%s", got, "see this::", out)
	}
}

// TestThreeColonsSurviveToo is a second witness, not a control: the reduction
// takes the last pair whatever precedes them, so ":::" comes back "::" without
// the escape. It lived in the control below until the control was run with the
// fix removed and failed — a control that fails without the fix is a witness
// wearing the wrong name, and the next reader would have trusted it.
func TestThreeColonsSurviveToo(t *testing.T) {
	const text = "three colons:::"
	out, err := Write(richdoc.New().P(richdoc.Txt(text)).Doc())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	d2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	p, ok := d2.Blocks[0].(richdoc.Paragraph)
	if !ok {
		t.Fatalf("came back as a %T — written as:\n%s", d2.Blocks[0], out)
	}
	if got := plainTextOf(p.Inlines); got != text {
		t.Errorf("%q -> %q — written as:\n%s", text, got, out)
	}
}

// TestASingleTrailingColonIsNotEscaped is the control, and it passes with the
// fix removed, which is what makes it one. One colon is not a marker, and an
// escape there would put a backslash in the document for nothing — the
// commonest paragraph ending in reST is "like this:".
func TestASingleTrailingColonIsNotEscaped(t *testing.T) {
	for _, text := range []string{"a sentence:", "a sentence."} {
		out, err := Write(richdoc.New().P(richdoc.Txt(text)).Doc())
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		d2, err := Parse(out)
		if err != nil {
			t.Fatalf("re-Parse: %v", err)
		}
		p, ok := d2.Blocks[0].(richdoc.Paragraph)
		if !ok {
			t.Fatalf("%q came back as a %T — written as:\n%s", text, d2.Blocks[0], out)
		}
		if got := plainTextOf(p.Inlines); got != text {
			t.Errorf("%q -> %q — written as:\n%s", text, got, out)
		}
		if strings.Contains(string(out), `\`) {
			t.Errorf("%q was escaped, and needs no escape:\n%s", text, out)
		}
	}
}
