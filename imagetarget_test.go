// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"reflect"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestImageTarget covers the shape docutils/rst v0.127.0 introduced: an
// ".. image::" (or ".. figure::") carrying a ":target:" arrives as a
// <reference> WRAPPING the <image>, which is how every project README
// writes a build badge.
//
// The upgrade left this package's own suite green while breaking two
// things, which is why each case here was checked by running the
// conversion, not by reading the diff: the block switch had no
// <reference> case, so the default branch unwrapped it and the LINK was
// dropped; and rawFigure looked for a direct <image> child, so a figure
// with a target reconstructed as a bare ".. figure::" with NO URI —
// losing the picture itself, not just its link.
func TestImageTarget(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   []richdoc.Block
	}{
		{
			"a badge keeps its link",
			".. image:: badge.svg\n   :target: https://ci.example.org/\n   :alt: Build Status\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Link{URL: "https://ci.example.org/", Inlines: []richdoc.Inline{
					richdoc.Image{URL: "badge.svg", Alt: "Build Status"},
				}},
			}}},
		},
		{
			// CONTROL: without a target the shape is unchanged — a bare
			// image is still a single-inline paragraph, not a link.
			"an image with no target is unchanged",
			".. image:: a.png\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Image{URL: "a.png"},
			}}},
		},
		{
			"a figure with a target keeps its URI and its target",
			".. figure:: pic.png\n   :target: https://e.org/\n\n   A caption.\n",
			[]richdoc.Block{richdoc.RawBlock{Format: "rst",
				Text: ".. figure:: pic.png\n   :target: https://e.org/\n\n   A caption."}},
		},
		{
			// A target naming another target is re-emitted in the form
			// it was written in, since nothing resolved it to a URI.
			"an unresolved named target is re-emitted as a reference",
			".. figure:: pic.png\n   :target: `some name`_\n\n   A caption.\n",
			[]richdoc.Block{richdoc.RawBlock{Format: "rst",
				Text: ".. figure:: pic.png\n   :target: `some name`_\n\n   A caption."}},
		},
		{
			"a substitution image with a target keeps the link inline",
			"Badge |s| here.\n\n.. |s| image:: a.png\n   :target: https://e.org/\n",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Text{Value: "Badge "},
				richdoc.Link{URL: "https://e.org/", Inlines: []richdoc.Inline{
					richdoc.Image{URL: "a.png", Alt: "s"},
				}},
				richdoc.Text{Value: " here."},
			}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := Parse([]byte(tc.source))
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.source, err)
			}
			if !reflect.DeepEqual(doc.Blocks, tc.want) {
				t.Errorf("Parse(%q) blocks =\n%#v\nwant:\n%#v", tc.source, doc.Blocks, tc.want)
			}
		})
	}
}

// TestWriteImageBlock covers the other half: reST has no inline image, so
// writeInline degrades one to its alt text — which, for a paragraph that
// IS an image, silently wrote the picture out of the document, and for
// the linked badge above wrote "` <uri>`__", an empty-label link that
// does not read back as a link at all. In block position the directive
// is available.
func TestWriteImageBlock(t *testing.T) {
	cases := []struct {
		name   string
		blocks []richdoc.Block
		want   string
	}{
		{
			"a lone image becomes its directive again",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Image{URL: "a.png"},
			}}},
			".. image:: a.png\n",
		},
		{
			"a linked image carries the link back as :target:",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Link{URL: "https://ci.example.org/", Inlines: []richdoc.Inline{
					richdoc.Image{URL: "badge.svg", Alt: "Build Status"},
				}},
			}}},
			".. image:: badge.svg\n   :alt: Build Status\n   :target: https://ci.example.org/\n",
		},
		{
			// CONTROL: a directive is a BLOCK. An image with text beside
			// it cannot become one, so it keeps the documented inline
			// degradation instead.
			"an image with text beside it stays inline",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Text{Value: "see "},
				richdoc.Image{URL: "a.png", Alt: "the picture"},
			}}},
			"see the picture\n",
		},
		{
			// CONTROL: a link around something that is not JUST an image
			// is an ordinary link.
			"a link around an image and text stays a link",
			[]richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{
				richdoc.Link{URL: "https://e.org/", Inlines: []richdoc.Inline{
					richdoc.Image{URL: "a.png", Alt: "pic"},
					richdoc.Text{Value: " and more"},
				}},
			}}},
			"`pic and more <https://e.org/>`__\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Write(&richdoc.Document{Blocks: tc.blocks})
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("Write =\n%q\nwant:\n%q", string(got), tc.want)
			}
		})
	}
}

// TestDirectiveSourceRoundTrips is the control the six fixtures this
// round had to CHANGE were missing. Every reconstruction that emits
// option lines used to put a blank line between the directive and its
// options, which ends the directive's own option region: docutils 0.23
// then reads ":class: x" as a FIELD LIST in the content — visible junk
// in an admonition, an "ends without a blank line"-style ERROR for a
// figure (the caption discarded), and for a rubric, which permits no
// content at all, an ERROR that replaces the whole directive.
//
// A test comparing the written TEXT to a fixture cannot see that: the
// fixture simply records whatever the writer does. This one writes the
// document and READS IT BACK, which is the only check that fails when
// the source stops being parseable.
func TestDirectiveSourceRoundTrips(t *testing.T) {
	sources := []string{
		".. note::\n   :class: testnote\n   :name: mynote\n\n   Body text.\n",
		".. sidebar:: Sidebar Title\n   :subtitle: Optional Subtitle\n\n   Body.\n",
		".. container::\n   :name: my container\n\n   Some text.\n",
		".. rubric:: A Rubric\n   :class: foo\n",
		".. figure:: pic.png\n   :alt: a cat\n   :width: 200\n\n   A caption.\n",
		".. figure:: pic.png\n   :target: https://e.org/\n\n   A caption.\n",
		".. image:: badge.svg\n   :alt: Build Status\n   :target: https://ci.example.org/\n",
	}
	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			first, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			written, err := Write(first)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			second, err := Parse(written)
			if err != nil {
				t.Fatalf("reparse: %v", err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Errorf("round trip changed the document\n  source:  %q\n  written: %q\n  before: %#v\n  after:  %#v",
					src, string(written), first.Blocks, second.Blocks)
			}
			// These seven all happen to be written back byte for byte,
			// which is worth pinning: it says the writer chose the SAME
			// spelling the author did, not merely an equivalent one.
			if string(written) != src {
				t.Errorf("written form differs from the source\n  source:  %q\n  written: %q", src, string(written))
			}
		})
	}
}
