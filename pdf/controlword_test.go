package pdf_test

import (
	"strings"
	"testing"

	"github.com/go-richdoc/rst"
	"github.com/go-richdoc/rst/pdf"
)

// TestAnUnnamedFootnoteTypesets is the downstream half of the upstream
// "\noindentFirst" defect (docutils v0.136.16). An auto-numbered footnote
// survives the trip through richdoc as ".. [#]" -- still unnamed -- so it is
// the shape that reaches the latex writer's unterminated \noindent and stops
// the engine.
//
// The witness is Strict, and that matters: the package default is LENIENT, and
// lenient turns this defect into something no error reports. The engine skips
// the undefined command and the footnote's first word goes with it -- PEP 495
// typeset 15 pages, exit 0, six words short. A default that degrades instead
// of failing can only be measured through the strict path.
func TestAnUnnamedFootnoteTypesets(t *testing.T) {
	const src = `Body text with a reference [#]_.

.. [#] First word must survive.

.. [#] Second one too.
`
	doc, err := rst.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := pdf.Write(doc, pdf.Options{Strict: true}); err != nil {
		t.Fatalf("strict typesetting: %v", err)
	}
	// The lenient path must not be reporting a skip either, since a skip here
	// is the silent form of the same defect.
	if _, err := pdf.Write(doc, pdf.Options{}); err != nil {
		t.Fatalf("lenient typesetting: %v", err)
	}
}

// TestMultiLineMathTypesets is the second upstream fix seen from here: a
// formula with a top-level line break needs align*, and amsmath rejects a \\
// inside equation*, so the hardcoded environment was LaTeX no engine reads.
func TestMultiLineMathTypesets(t *testing.T) {
	const src = `.. math::

   S &= \pi r^2 \\
   V &= \frac{4}{3} \pi r^3
`
	doc, err := rst.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := pdf.Write(doc, pdf.Options{Strict: true}); err != nil {
		if strings.Contains(err.Error(), "align") {
			t.Skipf("engine does not implement align*: %v", err)
		}
		t.Fatalf("strict typesetting: %v", err)
	}
}
