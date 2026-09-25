// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"reflect"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestSectionAnchor covers the Sphinx convention — ".. _label:" in front
// of a section title, which is how every cross-referenced section in a
// Sphinx document is labelled.
//
// docutils handles it in a transform (references.PropagateTargets): an
// internal target hands its ids and names to the next node, so the
// section carries both "introduction" and "my-anchor". This package
// dropped the target instead, which lost the label AND left every
// "my-anchor_" reference — already resolved to the link "#my-anchor" —
// pointing at an id nothing in the converted document carried.
func TestSectionAnchor(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   []richdoc.Block
	}{
		{
			"the label becomes the heading's id and both references reach it",
			".. _my-anchor:\n\nIntroduction\n============\n\nSee my-anchor_ and `Introduction`_.\n",
			[]richdoc.Block{
				richdoc.Heading{Level: 1, ID: "my-anchor", Inlines: []richdoc.Inline{richdoc.Text{Value: "Introduction"}}},
				richdoc.Paragraph{Inlines: []richdoc.Inline{
					richdoc.Text{Value: "See "},
					richdoc.Link{URL: "#my-anchor", Inlines: []richdoc.Inline{richdoc.Text{Value: "my-anchor"}}},
					richdoc.Text{Value: " and "},
					richdoc.Link{URL: "#my-anchor", Inlines: []richdoc.Inline{richdoc.Text{Value: "Introduction"}}},
					richdoc.Text{Value: "."},
				}},
			},
		},
		{
			// richdoc.Heading has ONE ID, so the first label wins and
			// the others become aliases for it. Nothing dangles, but
			// the extra names are not written back.
			"several labels collapse onto the first, and every reference follows",
			".. _a:\n.. _b:\n\nIntroduction\n============\n\nSee a_ and b_.\n",
			[]richdoc.Block{
				richdoc.Heading{Level: 1, ID: "a", Inlines: []richdoc.Inline{richdoc.Text{Value: "Introduction"}}},
				richdoc.Paragraph{Inlines: []richdoc.Inline{
					richdoc.Text{Value: "See "},
					richdoc.Link{URL: "#a", Inlines: []richdoc.Inline{richdoc.Text{Value: "a"}}},
					richdoc.Text{Value: " and "},
					richdoc.Link{URL: "#a", Inlines: []richdoc.Inline{richdoc.Text{Value: "b"}}},
					richdoc.Text{Value: "."},
				}},
			},
		},
		{
			// CONTROL: a target with a refuri is an external link's
			// definition, not an anchor for what follows it —
			// PropagateTargets skips exactly those. The heading keeps
			// its own slug.
			"a target carrying a URI is not an anchor for the section after it",
			".. _elsewhere: https://example.org/\n\nIntroduction\n============\n\nSee elsewhere_.\n",
			[]richdoc.Block{
				richdoc.Heading{Level: 1, ID: "introduction", Inlines: []richdoc.Inline{richdoc.Text{Value: "Introduction"}}},
				richdoc.Paragraph{Inlines: []richdoc.Inline{
					richdoc.Text{Value: "See "},
					richdoc.Link{URL: "https://example.org/", Inlines: []richdoc.Inline{richdoc.Text{Value: "elsewhere"}}},
					richdoc.Text{Value: "."},
				}},
			},
		},
		{
			// CONTROL: a label that is not immediately before a section
			// has no heading to attach to. richdoc has no id on any
			// other block, so this is still dropped — stated here so
			// that the day a Paragraph grows one, the case is already
			// written down.
			"a label in front of a paragraph is still dropped",
			".. _my-anchor:\n\nSome paragraph.\n",
			[]richdoc.Block{
				richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Text{Value: "Some paragraph."}}},
			},
		},
		{
			// CONTROL: a section with no label keeps its title slug.
			"a section with no label is unchanged",
			"Introduction\n============\n\nSee `Introduction`_.\n",
			[]richdoc.Block{
				richdoc.Heading{Level: 1, ID: "introduction", Inlines: []richdoc.Inline{richdoc.Text{Value: "Introduction"}}},
				richdoc.Paragraph{Inlines: []richdoc.Inline{
					richdoc.Text{Value: "See "},
					richdoc.Link{URL: "#introduction", Inlines: []richdoc.Inline{richdoc.Text{Value: "Introduction"}}},
					richdoc.Text{Value: "."},
				}},
			},
		},
		{
			// The label reaches a NESTED section too, which is where
			// most of them live.
			"a label in front of a subsection works the same",
			"Top\n===\n\n.. _sub-label:\n\nSub\n---\n\nSee sub-label_.\n",
			[]richdoc.Block{
				richdoc.Heading{Level: 1, ID: "top", Inlines: []richdoc.Inline{richdoc.Text{Value: "Top"}}},
				richdoc.Heading{Level: 2, ID: "sub-label", Inlines: []richdoc.Inline{richdoc.Text{Value: "Sub"}}},
				richdoc.Paragraph{Inlines: []richdoc.Inline{
					richdoc.Text{Value: "See "},
					richdoc.Link{URL: "#sub-label", Inlines: []richdoc.Inline{richdoc.Text{Value: "sub-label"}}},
					richdoc.Text{Value: "."},
				}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := Parse([]byte(tc.source))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !reflect.DeepEqual(doc.Blocks, tc.want) {
				t.Errorf("Parse(%q) blocks =\n%#v\nwant:\n%#v", tc.source, doc.Blocks, tc.want)
			}
		})
	}
}

// TestSectionAnchorRoundTrips writes the label back out, which is what
// makes the anchor survive a conversion rather than merely survive the
// parse. Three passes, for the reason TestHeadingRoundTripIsStable gives.
func TestSectionAnchorRoundTrips(t *testing.T) {
	sources := []string{
		".. _my-anchor:\n\nIntroduction\n============\n\nBody.\n",
		"Top\n===\n\n.. _sub-label:\n\nSub\n---\n\nBody.\n",
	}
	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			doc, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			for pass := 1; pass <= 3; pass++ {
				written, err := Write(doc)
				if err != nil {
					t.Fatalf("pass %d: Write: %v", pass, err)
				}
				if string(written) != src {
					t.Fatalf("pass %d wrote %q, want %q", pass, string(written), src)
				}
				next, err := Parse(written)
				if err != nil {
					t.Fatalf("pass %d: reparse: %v", pass, err)
				}
				if !reflect.DeepEqual(doc, next) {
					t.Fatalf("pass %d changed the document\n  before: %#v\n  after:  %#v", pass, doc.Blocks, next.Blocks)
				}
				doc = next
			}
		})
	}
}

// TestKeptDiagnosticQuotesTheNameAsWritten pins the one place a
// docutils message TEXT reaches a converted document: Options{
// KeepDiagnostics: true}, for a tool that converts a document in order
// to report on it. docutils/rst v0.129.0 stopped escaping the name it
// quotes (the Go %q verb is strconv.Quote, not Python's plain
// interpolation), so a name carrying quotes of its own now reads the way
// the author wrote it.
func TestKeptDiagnosticQuotesTheNameAsWritten(t *testing.T) {
	src := "A `Say \"hi\" <http://e.org/>`_ and `Say \"hi\" <http://e.org/>`_.\n"
	doc, err := ParseWithOptions([]byte(src), Options{KeepDiagnostics: true})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := `Duplicate name "say "hi"" for external target "http://e.org/".`
	first, ok := doc.Blocks[0].(richdoc.Paragraph)
	if !ok {
		t.Fatalf("first block is %T, want the diagnostic Paragraph", doc.Blocks[0])
	}
	got, ok := first.Inlines[0].(richdoc.Text)
	if !ok || got.Value != want {
		t.Errorf("diagnostic text = %#v, want %q", first.Inlines[0], want)
	}
}

// TestBacktrackedURIReachesTheDocument pins the two links docutils/rst
// v0.130.0 recovers: a standalone URI or email address followed by a
// character that cannot end one is no longer abandoned, so the prefix
// becomes a real Link here instead of staying inside a text run. Both
// round-trip.
func TestBacktrackedURIReachesTheDocument(t *testing.T) {
	cases := []struct {
		source string
		want   []richdoc.Inline
	}{
		{
			"See https://e.org/issues/{{ x }} end\n",
			[]richdoc.Inline{
				richdoc.Text{Value: "See "},
				richdoc.Link{URL: "https://e.org/issues", Inlines: []richdoc.Inline{richdoc.Text{Value: "https://e.org/issues"}}},
				richdoc.Text{Value: "/{{ x }} end"},
			},
		},
		{
			"See user@e.org/{ end\n",
			[]richdoc.Inline{
				richdoc.Text{Value: "See "},
				richdoc.Link{URL: "mailto:user@e.org", Inlines: []richdoc.Inline{richdoc.Text{Value: "user@e.org"}}},
				richdoc.Text{Value: "/{ end"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			doc, err := Parse([]byte(tc.source))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			p, ok := doc.Blocks[0].(richdoc.Paragraph)
			if !ok {
				t.Fatalf("first block is %T, want a Paragraph", doc.Blocks[0])
			}
			if !reflect.DeepEqual(p.Inlines, tc.want) {
				t.Errorf("inlines =\n%#v\nwant:\n%#v", p.Inlines, tc.want)
			}
			written, err := Write(doc)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			again, err := Parse(written)
			if err != nil {
				t.Fatalf("reparse: %v", err)
			}
			if !reflect.DeepEqual(doc, again) {
				t.Errorf("round trip changed the document: %q -> %#v", string(written), again.Blocks)
			}
		})
	}
}

// TestSphinxOnlyOptionKeepsItsBlock holds the one Option this package
// turns OFF for the reader's sake: docutils' parser rejects an option a
// directive does not declare, which replaces the whole block with an
// error. Thirty-two of the 1564 real-world corpus files carry such an
// option, and docutils/rst v0.133.0 extended the check from two
// directives to seventeen — so this is the difference between converting
// those documents and converting an apology for them.
//
// It asserts the block's TYPE, not its text. A first version asserted
// that the content was still present and could not fail: dropping a
// system_message keeps the literal_block inside it (that is deliberate,
// see convertBlockElement), so the author's text survives the strict
// parse too — as a CodeBlock holding the whole directive source. What
// the flag actually decides is whether a code block arrives as a
// CodeBlock with its LANGUAGE, an equation as a MathBlock, and an
// admonition as its own reconstruction, or whether all three arrive as
// one quoted blob.
func TestSphinxOnlyOptionKeepsItsBlock(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   richdoc.Block
	}{
		{
			"sphinx :collapsible: on a note",
			".. note::\n   :collapsible:\n\n   Body text.\n",
			richdoc.RawBlock{Format: "rst", Text: ".. note::\n\n   Body text."},
		},
		{
			"sphinx :caption: on a code block",
			".. code:: go\n   :caption: hi\n\n   x := 1\n",
			richdoc.CodeBlock{Language: "go", Text: "x := 1"},
		},
		{
			"sphinx :label: on an equation",
			".. math::\n   :label: eq\n\n   a^2\n",
			richdoc.MathBlock{TeX: "a^2"},
		},
		{
			"an undeclared option on a rubric",
			".. rubric:: R\n   :bogus: x\n",
			richdoc.RawBlock{Format: "rst", Text: ".. rubric:: R"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := Parse([]byte(tc.source))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(doc.Blocks) != 1 || !reflect.DeepEqual(doc.Blocks[0], tc.want) {
				t.Errorf("Parse(%q) blocks =\n%#v\nwant one block:\n%#v", tc.source, doc.Blocks, tc.want)
			}
		})
	}
}
