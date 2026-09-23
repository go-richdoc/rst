// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"reflect"
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
