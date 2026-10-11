// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestEscapedContinuationRoundTrips is PEP 277's shape, and PEP 262's: a
// paragraph wrapped with an ESCAPED continuation, which is how reST spells a
// hanging indent inside running text.
//
//	[1] Microsoft Windows APIs
//	\   https://msdn.example.com/
//
// The writer used to drop the indentation, for a reason that was sound as far
// as it went — re-emitted as a plain indented line, the continuation reads as
// a definition list. The backslash is what makes the line legal, and it is
// what the source has.
func TestEscapedContinuationRoundTrips(t *testing.T) {
	const src = "[1] Microsoft Windows APIs\n\\   https://msdn.example.com/\n"
	d1, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p1, ok := d1.Blocks[0].(richdoc.Paragraph)
	if !ok {
		t.Fatalf("source block is a %T, not a Paragraph", d1.Blocks[0])
	}
	first, ok := p1.Inlines[0].(richdoc.Text)
	if !ok || !strings.HasSuffix(first.Value, "\n  ") {
		t.Fatalf("the parse did not keep the escaped indentation: %#v", p1.Inlines)
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
		t.Fatalf("re-parsed block is a %T, not a Paragraph — written as:\n%s", d2.Blocks[0], out)
	}
	if len(p2.Inlines) != len(p1.Inlines) {
		t.Fatalf("inline count %d -> %d — written as:\n%s", len(p1.Inlines), len(p2.Inlines), out)
	}
	got, ok := p2.Inlines[0].(richdoc.Text)
	if !ok {
		t.Fatalf("first inline is a %T — written as:\n%s", p2.Inlines[0], out)
	}
	if got.Value != first.Value {
		t.Errorf("text %q -> %q — written as:\n%s", first.Value, got.Value, out)
	}
}

// TestEscapedContinuationArithmetic pins the rule the reference answered for,
// one space at a time: a backslash escapes the ONE whitespace character after
// it and is itself removed, so writing N spaces of indentation takes N+1.
//
// Both directions are asserted. A writer that emitted the indentation with no
// backslash would make a definition list of it, and one that emitted the
// backslash with no extra space would lose a space per wrap.
func TestEscapedContinuationArithmetic(t *testing.T) {
	for _, spaces := range []int{0, 1, 2, 3, 7} {
		want := "one\n" + strings.Repeat(" ", spaces) + "two"
		d := richdoc.New().P(richdoc.Txt(want)).Doc()
		out, err := Write(d)
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		if spaces > 0 {
			line := strings.Split(strings.TrimRight(string(out), "\n"), "\n")[1]
			if !strings.HasPrefix(line, "\\"+strings.Repeat(" ", spaces+1)) {
				t.Errorf("%d spaces: continuation line is %q, want a backslash and %d spaces", spaces, line, spaces+1)
			}
		}
		back, err := Parse(out)
		if err != nil {
			t.Fatalf("re-Parse: %v", err)
		}
		p, ok := back.Blocks[0].(richdoc.Paragraph)
		if !ok {
			t.Fatalf("%d spaces: re-parsed a %T — written as:\n%s", spaces, back.Blocks[0], out)
		}
		txt, ok := p.Inlines[0].(richdoc.Text)
		if !ok || txt.Value != want {
			t.Errorf("%d spaces: came back as %#v, want %q — written as:\n%s", spaces, p.Inlines, want, out)
		}
	}
}

// TestNoDanglingWrapEscape covers the half of this that is NOT about keeping
// spaces: an escape with nothing after it on its line rstrips to a lone
// backslash, and the reference reads that as a title-underline candidate,
// putting "Possible title underline, too short for the title." into the
// document. Trading a dropped space for a diagnostic is worse than the bug.
func TestNoDanglingWrapEscape(t *testing.T) {
	// A trailing run cannot reach here from a parse any more (a paragraph's
	// text is rstripped since docutils/rst v0.141.0), so the tree is built
	// directly: this asserts the guard, not the parser.
	d := richdoc.New().P(richdoc.Txt("one\n  ")).Doc()
	out, err := Write(d)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	for i, l := range strings.Split(string(out), "\n") {
		if strings.TrimRight(l, " ") == "\\" {
			t.Fatalf("line %d is a lone backslash, which reads as a title underline:\n%q", i, out)
		}
	}
	d2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	for _, b := range d2.Blocks {
		if p, ok := b.(richdoc.Paragraph); ok {
			if txt := plainTextOf(p.Inlines); strings.Contains(txt, "title underline") {
				t.Errorf("the reconstruction gained a diagnostic: %q\nwritten as:\n%s", txt, out)
			}
		}
	}
}
