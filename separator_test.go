package rst

import (
	"strings"
	"testing"
)

// TestARoleAdjacentToAWordKeepsItsNullSeparator pins a CONTENT loss, not a
// formatting one. PEP 410 writes
//
//	10\ :sup:`-9`
//
// where "\ " is reST's null separator -- an escaped space attaching the
// superscript to the digit with no visible gap. Without it the writer produced
// "10:sup:`-9`", which docutils does not read as a role at all: it is plain text,
// and the SUPERSCRIPT DISAPPEARS.
//
// The separator logic knew about "*`[|_" and not about ":", which is how a role
// begins.
func TestARoleAdjacentToAWordKeepsItsNullSeparator(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"superscript after a digit", "Values of 10\\ :sup:`-9` here.\n", `10\ :sup:`},
		{"subscript after a word", "The x\\ :sub:`i` term.\n", `x\ :sub:`},
		{"a role before punctuation", "See :sup:`2`\\ , then.\n", `:sup:`},
	}
	for _, c := range cases {
		out, msgs := reconstruct(t, c.src)
		if msgs != 0 {
			t.Errorf("%s: the reconstruction gained %d diagnostic(s):\n%s", c.name, msgs, out)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: want %q in:\n%s", c.name, c.want, out)
		}
	}
	// The loss this is really about: the role has to survive as a ROLE, so the
	// re-parse must still contain a superscript and not the literal text.
	out, _ := reconstruct(t, "Values of 10\\ :sup:`-9` here.\n")
	if strings.Contains(out, "10:sup:") {
		t.Errorf("the separator was dropped, so the role is plain text now:\n%s", out)
	}
}

// TestAnOptionValueKeepsItsContinuationIndent pins the same defect as the line
// block and the docinfo field list, in its third home: a directive OPTION value
// can run to several lines -- an image's ":alt:" is the common case, PEP 495 has
// a two-line one -- and a field body's continuation has to be indented or the
// field ends there. The second line became a NEW option, so the alt text was
// truncated at the break and the rest read as an unknown option.
//
// Fixed in rawDirectiveSource, the one place all two dozen option-building sites
// funnel through, rather than at whichever one the next corpus file happens to
// reach.
func TestAnOptionValueKeepsItsContinuationIndent(t *testing.T) {
	cases := []struct{ name, src string }{
		{"a figure's alt", ".. figure:: p.png\n   :alt: First line of the alt text.\n         Second line of it.\n\n   Caption.\n"},
		{"a standalone image's alt", ".. image:: p.png\n   :alt: First line of the alt text.\n         Second line of it.\n\nBody.\n"},
	}
	for _, c := range cases {
		out, msgs := reconstruct(t, c.src)
		if msgs != 0 {
			t.Errorf("%s: the reconstruction gained %d diagnostic(s):\n%s", c.name, msgs, out)
		}
		if !strings.Contains(out, "Second line of it.") {
			t.Errorf("%s: the second line of the value was lost:\n%s", c.name, out)
		}
		// It must be a CONTINUATION, deeper than the option marker, not a
		// sibling option at the same indent.
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "Second line of it.") {
				indent := len(l) - len(strings.TrimLeft(l, " "))
				if indent <= 3 {
					t.Errorf("%s: the continuation sits at indent %d, no deeper than the marker:\n%s", c.name, indent, out)
				}
			}
		}
	}
}
