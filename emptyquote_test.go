// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// pep12Quote is PEP 12's own shape: a FOOTNOTE DEFINITION indented under a
// sentence, which docutils reads as a block quote containing a footnote.
const pep12Quote = "A paragraph with a reference [#TeXbook]_.\n\nwhich renders as\n\n" +
	"    .. [#TeXbook] Donald Knuth's *The TeXbook*, pages 195 and 196.\n\n" +
	"Footnotes will be numbered automatically.\n"

// TestEmptyBlockQuoteIsNotABlock covers the last STRUCTURAL difference in the
// round trip over the real-world corpus — the only class where a block came
// back as a different TYPE rather than with different content.
//
// A footnote definition becomes a richdoc.Footnote at its reference point, not
// a block, so a quote that held nothing else converts to a BlockQuote with no
// blocks. reST spells a quote by INDENTING its content, so an empty one has no
// spelling: the writer emits nothing, the block vanishes, and every block after
// it shifts. The probe reported that as "BlockQuote -> Paragraph" at index 94
// of 150, which was the misalignment and not a conversion — the kind of report
// an index-aligned comparison gives when an item is MISSING.
func TestEmptyBlockQuoteIsNotABlock(t *testing.T) {
	d1, err := Parse([]byte(pep12Quote))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for i, b := range d1.Blocks {
		q, ok := b.(richdoc.BlockQuote)
		if ok && len(q.Blocks) == 0 {
			t.Fatalf("block %d is a BlockQuote with no blocks; nothing can write one", i)
		}
	}
	// And the footnote's text is still in the document, at the end, where this
	// writer puts every definition.
	out, err := Write(d1)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(string(out), "TeXbook") {
		t.Errorf("the footnote's text left the document:\n%s", out)
	}
	// The round trip is then a fixed point in COUNT, which is what the
	// vanishing block broke.
	d2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	if len(d2.Blocks) != len(d1.Blocks) {
		t.Errorf("block count %d -> %d:\n%s", len(d1.Blocks), len(d2.Blocks), out)
	}
}

// TestNonEmptyBlockQuoteSurvives is the control the case above needs: dropping
// an EMPTY quote must not drop a real one, and a check that cannot tell the
// two apart would pass either way.
func TestNonEmptyBlockQuoteSurvives(t *testing.T) {
	d1, err := Parse([]byte("intro\n\n    a real quote\n\nafter\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	found := false
	for _, b := range d1.Blocks {
		if q, ok := b.(richdoc.BlockQuote); ok && len(q.Blocks) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("a quote with content did not survive the parse: %+v", d1.Blocks)
	}
	out, err := Write(d1)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	d2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	for _, b := range d2.Blocks {
		if q, ok := b.(richdoc.BlockQuote); ok && len(q.Blocks) > 0 {
			return
		}
	}
	t.Errorf("the quote is gone from the reconstruction:\n%s", out)
}
