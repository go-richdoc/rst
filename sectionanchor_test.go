// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"reflect"
	"strings"
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

// TestEmbeddedAliasLinkText pins the link LABEL for a phrase reference
// whose text is omitted — "`<target>`_", where the target supplies the
// text. docutils/rst v0.134.0 made that label the alias AS COMPUTED, so a
// name alias arrives normalized and an email alias arrives as the whole
// "mailto:" URI it links to, which is what docutils' own HTML writer
// renders.
//
// It reaches converted documents: over the 1564-file corpus this changed
// six lines, all of them in PEP 447, whose message-ID links are written
// this way.
func TestEmbeddedAliasLinkText(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   richdoc.Inline
	}{
		{
			"a name alias is normalized",
			"`<Feature Negotiation_>`__\n\n.. _feature negotiation: https://e.org/\n",
			richdoc.Link{URL: "https://e.org/", Inlines: []richdoc.Inline{richdoc.Text{Value: "feature negotiation"}}},
		},
		{
			"an email alias keeps the mailto: it links to",
			"`<user@e.org>`__\n",
			richdoc.Link{URL: "mailto:user@e.org", Inlines: []richdoc.Inline{richdoc.Text{Value: "mailto:user@e.org"}}},
		},
		{
			// CONTROL: a reference WITH text of its own is untouched.
			"an alias with text keeps the text",
			"`shown <Feature Negotiation_>`__\n\n.. _feature negotiation: https://e.org/\n",
			richdoc.Link{URL: "https://e.org/", Inlines: []richdoc.Inline{richdoc.Text{Value: "shown"}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := Parse([]byte(tc.source))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			p, ok := doc.Blocks[0].(richdoc.Paragraph)
			if !ok || len(p.Inlines) != 1 {
				t.Fatalf("want one inline in a Paragraph, got %#v", doc.Blocks[0])
			}
			if !reflect.DeepEqual(p.Inlines[0], tc.want) {
				t.Errorf("inline =\n%#v\nwant:\n%#v", p.Inlines[0], tc.want)
			}
		})
	}
}

// TestLiteralBlockIndentedWithNoBreakSpaces pins the one thing
// docutils/rst v0.135.0 changes in a converted document: a literal block
// whose indentation mixes plain spaces with NO-BREAK SPACES is dedented by
// its whole width, not by the plain spaces alone.
//
// pytest's own documentation writes one that way, and a CodeBlock
// preserves what it is given — so the phantom indentation was visible in
// every rendering. Two lines of the 1564-file corpus, and this is them.
func TestLiteralBlockIndentedWithNoBreakSpaces(t *testing.T) {
	const src = "Run it::\n\n     pytest one\n     pytest two\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := richdoc.CodeBlock{Text: "pytest one\npytest two"}
	if len(doc.Blocks) != 2 || !reflect.DeepEqual(doc.Blocks[1], want) {
		t.Errorf("Parse(%q) blocks =\n%#v\nwant the second to be:\n%#v", src, doc.Blocks, want)
	}
}

// TestCSVCellSpanningLines pins what docutils/rst v0.136.0 changes in a
// converted document: a quoted csv-table cell spanning several source
// lines is parsed as a BLOCK, so a cell whose opening quote sits alone on
// its line no longer begins with an empty line.
//
// PEP 578 writes several, and its table is the only thing in the
// 1564-file corpus this moved — 104 lines of it, since one cell's text
// changes every column width around it.
func TestCSVCellSpanningLines(t *testing.T) {
	const src = ".. csv-table::\n\n   ``a``, \"\n   Detect dynamic code compilation.\n   \"\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	table, ok := doc.Blocks[0].(richdoc.Table)
	if !ok {
		t.Fatalf("first block is %T, want a Table", doc.Blocks[0])
	}
	want := []richdoc.Inline{richdoc.Text{Value: "Detect dynamic code compilation."}}
	got := table.Rows[0][1].Inlines
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the second cell =\n%#v\nwant:\n%#v", got, want)
	}
}

// TestDiagnosticInsideAnAdmonition pins what docutils/rst v0.136.2
// changes here: a duplicate-name notice raised inside a ".. note::" is
// now the note's own first child, where it used to precede the note.
//
// It is visible only under KeepDiagnostics — the mode that exists for a
// tool converting a document in order to report on it — and only in PEP
// 813 across the 1564-file corpus, 26 lines of it. The lenient default
// drops the message either way, which the second case is here to say.
func TestDiagnosticInsideAnAdmonition(t *testing.T) {
	const src = "`A <http://e.org/1>`_\n\n.. note::\n\n   `A <http://e.org/2>`_\n"

	kept, err := ParseWithOptions([]byte(src), Options{KeepDiagnostics: true})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := richdoc.RawBlock{Format: "rst", Text: ".. note::\n\n   Duplicate implicit target name: \"a\".\n\n   A"}
	if len(kept.Blocks) != 2 || !reflect.DeepEqual(kept.Blocks[1], want) {
		t.Errorf("with KeepDiagnostics the note =\n%#v\nwant:\n%#v", kept.Blocks, want)
	}

	// CONTROL: the default drops it, so the note keeps only its content.
	lenient, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	wantLenient := richdoc.RawBlock{Format: "rst", Text: ".. note::\n\n   A"}
	if len(lenient.Blocks) != 2 || !reflect.DeepEqual(lenient.Blocks[1], wantLenient) {
		t.Errorf("the default note =\n%#v\nwant:\n%#v", lenient.Blocks, wantLenient)
	}
}

// TestDiagnosticsNeverLeakIntoARawBlock covers the OTHER half of the
// same discovery, and it predates docutils/rst v0.136.2 by a long way:
// every raw* reconstruction walks its element's own children, so a
// <system_message> nested inside a construct that becomes a RawBlock
// reached the output as prose. The TagSystemMessage case that drops
// diagnostics is never consulted for a subtree already turned into reST
// source.
//
// The shape is taken from sphinx's own theming.rst, reduced: a bullet
// list inside a definition body, each item ending in an unknown
// ".. versionadded::". Three real-world corpus files ended with a
// sentence like `Duplicate explicit target name: "versionadded".` sitting
// in the prose as if an author had written it.
func TestDiagnosticsNeverLeakIntoARawBlock(t *testing.T) {
	const src = "term\n" +
		"  - **a** (bool): first.\n" +
		"    Defaults to ``True``.\n\n" +
		"    .. versionadded:: 3.1\n\n" +
		"  - **b** (bool): second.\n" +
		"    Defaults to ``False``.\n\n" +
		"    .. versionadded:: 3.1\n"

	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Write(doc)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if strings.Contains(string(out), "Duplicate explicit target name") {
		t.Errorf("a diagnostic reached the converted document as prose:\n%s", out)
	}

	// CONTROL: with KeepDiagnostics the same message must still be there.
	// Without this, dropping diagnostics EVERYWHERE would pass.
	kept, err := ParseWithOptions([]byte(src), Options{KeepDiagnostics: true})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	keptOut, err := Write(kept)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(string(keptOut), "Duplicate explicit target name") {
		t.Errorf("KeepDiagnostics dropped a message it is meant to keep:\n%s", keptOut)
	}
}

// TestClassArgumentBlockIsNotAFieldList pins what docutils/rst v0.136.3
// changes here: ".. class:: Sphinx" with ":no-index:" on the next line is
// ONE argument block — the two classes — not a class plus a field list.
//
// Four converted documents (sphinx's extdev/appapi and three cpp-domain
// test roots) carried a stray ":no-index:" line as a field list before
// this; the classes go on the content, which for a converter means they
// vanish, and what is left is the content alone.
func TestClassArgumentBlockIsNotAFieldList(t *testing.T) {
	const src = ".. class:: Sphinx\n   :no-index:\n\n   The method does a thing.\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
		richdoc.Text{Value: "The method does a thing."},
	}}}
	if !reflect.DeepEqual(doc.Blocks, want) {
		t.Errorf("Parse(%q) blocks =\n%#v\nwant:\n%#v", src, doc.Blocks, want)
	}
	out, err := Write(doc)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if strings.Contains(string(out), ":no-index:") {
		t.Errorf("the argument block came through as a field:\n%s", out)
	}
}
