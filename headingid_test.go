// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestHeadingAnchorsFromExactMakeID pins the anchors docutils/rst
// v0.128.0's exact make_id produces, since this package's writer asks
// upstream for that rule rather than keeping its own copy. An apostrophe
// is DELETED, not turned into a hyphen; ß is "sz"; a title with no ASCII
// at all yields no identifier and the section gets a positional one.
func TestHeadingAnchorsFromExactMakeID(t *testing.T) {
	cases := []struct {
		source string
		wantID string
	}{
		{"What’s New In Python\n====================\n\nBody.\n", "whats-new-in-python"},
		{"Straße und Ärger\n================\n\nBody.\n", "strasze-und-arger"},
		{"中文标题\n========\n\nBody.\n", "section-1"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			doc, err := Parse([]byte(tc.source))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			h, ok := doc.Blocks[0].(richdoc.Heading)
			if !ok {
				t.Fatalf("first block is %T, want a Heading", doc.Blocks[0])
			}
			if h.ID != tc.wantID {
				t.Errorf("heading id = %q, want %q", h.ID, tc.wantID)
			}
		})
	}
}

// TestHeadingRoundTripIsStable runs THREE passes, not one and not two.
//
// The writer emits an explicit ".. _id:" before a heading whose title
// would not already slug to that id. For an id the PARSER made up — the
// positional "section-1" of a title with no ASCII, or the "intro-1"
// claimID gives the second section titled "Intro" — that target claims
// the name, which pushes the section's own id one suffix further, and
// the next pass writes the pushed id and lands back on the first. The
// document oscillates between two forms forever.
//
// One pass looks like plain degradation. TWO looks like it converged.
// Only three tell them apart, which is why every pass is compared here.
func TestHeadingRoundTripIsStable(t *testing.T) {
	sources := []string{
		"中文标题\n========\n\nBody.\n",
		"中文标题\n========\n\nBody.\n\n日本語\n======\n\nMore.\n",
		"Intro\n=====\n\na\n\nIntro\n=====\n\nb\n",
		"What’s New\n==========\n\nBody.\n",
	}
	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			doc, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			text := src
			for pass := 1; pass <= 3; pass++ {
				written, err := Write(doc)
				if err != nil {
					t.Fatalf("pass %d: Write: %v", pass, err)
				}
				if string(written) != text {
					t.Fatalf("pass %d wrote %q, want %q", pass, string(written), text)
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

// TestCustomHeadingAnchorIsStillWritten is the CONTROL for the rule
// above: an id that is NOT derivable from the title plus a numeric
// suffix belongs to whoever built the document — a LaTeX \label, say —
// and must still be written out, or converting into reST loses every
// anchor another format carried.
func TestCustomHeadingAnchorIsStillWritten(t *testing.T) {
	cases := []struct {
		name string
		doc  []richdoc.Block
		want string
	}{
		{
			"a label unrelated to the title is written",
			[]richdoc.Block{richdoc.Heading{Level: 1, ID: "sec-intro",
				Inlines: []richdoc.Inline{richdoc.Text{Value: "Introduction"}}}},
			".. _sec-intro:\n\nIntroduction\n============\n",
		},
		{
			// A NON-numeric suffix is not what claimID appends, so this
			// is an author's id, not a generated one.
			"a title-derived id with a non-numeric suffix is written",
			[]richdoc.Block{richdoc.Heading{Level: 1, ID: "intro-old",
				Inlines: []richdoc.Inline{richdoc.Text{Value: "Intro"}}}},
			".. _intro-old:\n\nIntro\n=====\n",
		},
		{
			"an id equal to the title's own slug is not written",
			[]richdoc.Block{richdoc.Heading{Level: 1, ID: "intro",
				Inlines: []richdoc.Inline{richdoc.Text{Value: "Intro"}}}},
			"Intro\n=====\n",
		},
		{
			"a generated disambiguating suffix is not written",
			[]richdoc.Block{richdoc.Heading{Level: 1, ID: "intro-1",
				Inlines: []richdoc.Inline{richdoc.Text{Value: "Intro"}}}},
			"Intro\n=====\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Write(&richdoc.Document{Blocks: tc.doc})
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("Write =\n%q\nwant:\n%q", string(got), tc.want)
			}
		})
	}
}

// TestAdornmentTriplesDoNotBecomeHeadings pins three shapes that
// docutils/rst v0.136.7 changed underneath this package. A punctuation
// line after an overline reaches docutils' own Line.underline, never the
// three-line Line.text path, so three IDENTICAL adornment lines are not a
// title at all.
//
// Each case was probed, not reasoned about, and each was wrong here before
// the bump: "====" three times produced a Heading whose text was "====";
// "..." three times produced a Heading and DROPPED its third line; and the
// docutils testsuite's own section_headers[32] came out with heading levels
// 1, 2, 3 where the third section returns to the document level, because
// its underline style is the one already established there.
//
// The last assertion is the CONTROL: an ordinary two-level document still
// nests, so "levels come from the adornment styles in order of first
// appearance" is not quietly replaced by "every heading is level 1".
func TestAdornmentTriplesDoNotBecomeHeadings(t *testing.T) {
	t.Run("three identical long adornments are a refused block, not a heading", func(t *testing.T) {
		d, err := Parse([]byte("====\n====\n====\n"))
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range d.Blocks {
			if h, ok := b.(richdoc.Heading); ok {
				t.Errorf("built a heading %q from an invalid marker", plainTextOf(h.Inlines))
			}
		}
	})
	t.Run("a demoted short adornment keeps the line after its section", func(t *testing.T) {
		d, err := Parse([]byte("...\n...\n...\n"))
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Blocks) != 2 {
			t.Fatalf("want a heading and a paragraph, got %d blocks: %+v", len(d.Blocks), d.Blocks)
		}
		if _, ok := d.Blocks[1].(richdoc.Paragraph); !ok {
			t.Errorf("the third line was lost: %T", d.Blocks[1])
		}
	})
	t.Run("the testsuite's section_headers[32] nests 1, 2, 1", func(t *testing.T) {
		d, err := Parse([]byte("...\n...\n\n...\n---\n\n...\n...\n...\n"))
		if err != nil {
			t.Fatal(err)
		}
		var levels []int
		for _, b := range d.Blocks {
			if h, ok := b.(richdoc.Heading); ok {
				levels = append(levels, h.Level)
			}
		}
		if len(levels) != 3 || levels[0] != 1 || levels[1] != 2 || levels[2] != 1 {
			t.Errorf("heading levels = %v, want [1 2 1]", levels)
		}
	})
	t.Run("CONTROL: an ordinary two-level document still nests", func(t *testing.T) {
		d, err := Parse([]byte("Top\n===\n\nSub\n---\n\nbody\n"))
		if err != nil {
			t.Fatal(err)
		}
		var levels []int
		for _, b := range d.Blocks {
			if h, ok := b.(richdoc.Heading); ok {
				levels = append(levels, h.Level)
			}
		}
		if len(levels) != 2 || levels[0] != 1 || levels[1] != 2 {
			t.Errorf("heading levels = %v, want [1 2]", levels)
		}
	})
}

// TestNestedTitleAttemptKeepsItsSource pins what docutils/rst v0.136.9
// changed underneath this package. A section title cannot appear inside a
// block quote or a list item, and the reference reports one there
// ("Unexpected section title.") with the offending two lines quoted --
// including when the underline is under four characters, which upstream
// used to read as ordinary text.
//
// So "dup" over "===" inside a block quote arrived here as a PARAGRAPH
// whose text was "dup ===", the line break gone and the adornment glued
// on. It now takes the path every refused construct takes: a CodeBlock
// holding the source as written, which a writer can put back.
//
// The control is the top level, where the same two lines are a real
// heading -- the distinction this whole rule is about.
func TestNestedTitleAttemptKeepsItsSource(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"in a block quote", "intro\n\n   dup\n   ===\n\nlast\n"},
		{"a demoted adornment in a block quote", "intro\n\n   ...\n   ===\n\nlast\n"},
		{"in a bullet item", "intro\n\n- dup\n  ===\n\nlast\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse([]byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			found := ""
			var walk func([]richdoc.Block)
			walk = func(bs []richdoc.Block) {
				for _, b := range bs {
					switch v := b.(type) {
					case richdoc.CodeBlock:
						found = v.Text
					case richdoc.BlockQuote:
						walk(v.Blocks)
					case richdoc.List:
						for _, it := range v.Items {
							walk(it.Blocks)
						}
					case richdoc.Heading:
						t.Errorf("built a heading where a title is not allowed: %q", plainTextOf(v.Inlines))
					}
				}
			}
			walk(d.Blocks)
			if !strings.Contains(found, "===") {
				t.Errorf("the refused source did not survive as a code block: %q\n%+v", found, d.Blocks)
			}
			if strings.Contains(found, "dup ===") {
				t.Errorf("the line break was lost, so the source cannot be written back: %q", found)
			}
		})
	}
	t.Run("CONTROL: at the top level the same two lines are a heading", func(t *testing.T) {
		d, err := Parse([]byte("intro\n\ndup\n===\n\nlast\n"))
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range d.Blocks {
			if h, ok := b.(richdoc.Heading); ok && plainTextOf(h.Inlines) == "dup" {
				return
			}
		}
		t.Errorf("no heading at the top level: %+v", d.Blocks)
	})
}
