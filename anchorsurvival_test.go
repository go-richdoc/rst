package rst

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestAnAnchorBelongingToTheNextSectionSurvives pins the commonest shape there
// is, and the one collectSectionAnchors used to miss. docutils' section nesting
// makes a target written between two same-level titles the LAST CHILD of the
// EARLIER section, so pairing a pending target with a section SIBLING never
// found it. Asked about this input the reference answers
// `<section ids="plain-section plain" names="plain\ section plain">`:
// PropagateTargets finds "the next node" in document order and crosses the
// boundary.
func TestAnAnchorBelongingToTheNextSectionSurvives(t *testing.T) {
	const src = `See plain_.

.. _first:

First Section
=============

Body.

.. _plain:

Plain Section
=============

More.
`
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var ids []string
	for _, b := range doc.Blocks {
		if h, ok := b.(richdoc.Heading); ok {
			ids = append(ids, h.ID)
		}
	}
	want := []string{"first", "plain"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("heading ids = %v, want %v", ids, want)
	}
}

// TestAnAnchorWithNoHeadingToTakeItIsKept covers the other half: richdoc gives
// no Block but Heading an ID, so a target whose next node is a paragraph, a list
// or a table has nowhere to attach. It used to be dropped, which left a
// "#standalone" reference pointing at nothing. Kept as its own reST source
// instead -- what RawBlock is documented for.
func TestAnAnchorWithNoHeadingToTakeItIsKept(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"before a paragraph", ".. _standalone:\n\nA paragraph.\n", ".. _standalone:"},
		{"before a list", ".. _thelist:\n\n* one\n* two\n", ".. _thelist:"},
		{"a name needing backquotes", ".. _`has: colon`:\n\nA paragraph.\n", ".. _`has: colon`:"},
	}
	for _, c := range cases {
		doc, err := Parse([]byte(c.src))
		if err != nil {
			t.Fatalf("%s: Parse: %v", c.name, err)
		}
		found := false
		for _, b := range doc.Blocks {
			if rb, ok := b.(richdoc.RawBlock); ok && rb.Format == "rst" && rb.Text == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: want a RawBlock %q in %#v", c.name, c.want, doc.Blocks)
		}
	}
}

// TestAKeptAnchorReparsesToItself is the property that matters more than the
// shape: the reconstruction has to be a FIXED POINT. Writing a target back is
// only an improvement if the next parse agrees, and the first version of this
// was not -- pep-0256's ".. _pythondoc:" is chained onto an external target, so
// writing it out put it immediately before a section it never preceded in the
// source, and the next parse made it that section's anchor. One file, found by
// set-diffing the corpus rather than by the total, which had gone up.
func TestAKeptAnchorReparsesToItself(t *testing.T) {
	for _, src := range []string{
		".. _standalone:\n\nA paragraph.\n",
		"See plain_.\n\n.. _first:\n\nFirst\n=====\n\nBody.\n\n.. _plain:\n\nPlain\n=====\n\nMore.\n",
		"See pythondoc_.\n\n.. _pythondoc:\n.. _gendoc: http://example.com/g\n\nSection\n=======\n",
	} {
		d1, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("Parse(%q): %v", src, err)
		}
		out, err := Write(d1)
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		d2, err := Parse(out)
		if err != nil {
			t.Fatalf("re-Parse: %v", err)
		}
		if !reflect.DeepEqual(d1.Blocks, d2.Blocks) {
			t.Errorf("not a fixed point for %q:\n first: %#v\nsecond: %#v\nwritten:\n%s",
				src, d1.Blocks, d2.Blocks, out)
		}
	}
}

// TestAChainedAnchorIsNotWrittenBack is the reason the fixed point holds: a bare
// target chained onto one that carries a reference is another NAME for that
// destination, not an anchor in this document. docutils resolves the name to the
// URL (v0.137.1 upstream), so the target is bookkeeping and writing it back
// would invent an anchor the source never had.
//
// Checked without the parse.go change it passes trivially -- with no raw-block
// path there is nothing to write back -- so it pins a boundary of the new
// behaviour rather than witnessing it. What it does witness is the UPSTREAM
// half: the two URL assertions fail against docutils v0.137.0.
func TestAChainedAnchorIsNotWrittenBack(t *testing.T) {
	doc, err := Parse([]byte("See pythondoc_.\n\n.. _pythondoc:\n.. _gendoc: http://example.com/g\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, b := range doc.Blocks {
		if rb, ok := b.(richdoc.RawBlock); ok && strings.Contains(rb.Text, "_pythondoc") {
			t.Errorf("a chained target was written back as an anchor: %#v", doc.Blocks)
		}
	}
	// And the name must have resolved to the URL rather than to a fragment.
	out, err := Write(doc)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(string(out), "http://example.com/g") {
		t.Errorf("the reference lost its URL:\n%s", out)
	}
	if strings.Contains(string(out), "#pythondoc") {
		t.Errorf("the reference still points at a fragment nothing carries:\n%s", out)
	}
}
