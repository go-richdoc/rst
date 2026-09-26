// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// corpus holds representative reST documents exercising every block and
// inline construct this converter maps to a native richdoc node (the
// RawBlock/RawInline fallback paths — directives, field lists, an
// unresolvable auto footnote, and so on — are covered by TestParse instead,
// since by design they do NOT survive a Parse -> Write -> Parse round-trip
// byte-for-byte the way a natively-mapped construct does). Each entry here
// must reproduce the same richdoc tree after Parse -> Write -> Parse, up to
// the normalisation Write performs (fixed title-underline characters,
// enumerated lists always restarting at 1, "-"/"N." markers).
var corpus = map[string]string{
	"headings": "Top\n===\n\nIntro.\n\nSub\n----\n\nNested.\n",

	"inline styles": "Text with *emph*, **strong**, and ``code`` spans.\n",

	"tight bullet list": "- one\n- two\n- three\n",

	"loose list with nested paragraph": "- first paragraph\n\n  second paragraph in the same item\n\n- next item\n",

	"ordered list": "1. first\n2. second\n3. third\n",

	"blockquote": "Before.\n\n    Quoted paragraph.\n\nAfter.\n",

	"literal block": "Sample::\n\n    code line one\n    code line two\n",

	"reference with embedded uri": "See `Python <https://python.org>`_ site.\n",

	"substitution reference used as a hyperlink": ".. |sub| replace:: replacement text\n\n.. _sub: https://example.org/sub\n\nSee |sub|_ for more.\n",

	"raw directive": ".. raw:: html\n\n   <b>bold</b>\n",

	"inline internal target and a same-document reference to it": "See the _`important term` and later refer to `important term`_.\n",

	"reference to a section title, resolved via the section's own implicit target": "See `My Section`_.\n\nMy Section\n==========\n\nContent.\n",

	"standalone uri": "Visit https://example.com today.\n",

	"thematic break": "before\n\n----\n\nafter\n",

	"footnote": "A claim.[1]_\n\n.. [1] The footnote body.\n",

	"citation": "See CIT2002.[CIT2002]_\n\n.. [CIT2002] The citation body.\n",

	"strikethrough": "Some *emph* and a :strike:`struck` word.\n",

	"inline math": "A term :math:`a^2 + b^2` here.\n",

	"table": "=====  =====\na      b\n=====  =====\n1      2\n3      4\n=====  =====\n",

	"document meta": ":title: Hello\n:author: Ann\n\nBody text.\n",

	"empty": "",
}

func TestRoundTrip(t *testing.T) {
	for name, src := range corpus {
		t.Run(name, func(t *testing.T) {
			d1, err := Parse([]byte(src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			out, err := Write(d1)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			d2, err := Parse(out)
			if err != nil {
				t.Fatalf("re-Parse: %v", err)
			}
			if !reflect.DeepEqual(d1, d2) {
				t.Errorf("round-trip changed the tree\n--- rewritten source ---\n%s\n--- d1 ---\n%#v\n--- d2 ---\n%#v", out, d1, d2)
			}
		})
	}
}

// TestRoundTripStableOutput checks that a second Write of the re-parsed tree
// produces byte-identical output, i.e. the writer reaches a fixed point.
func TestRoundTripStableOutput(t *testing.T) {
	for name, src := range corpus {
		t.Run(name, func(t *testing.T) {
			d1, _ := Parse([]byte(src))
			out1, _ := Write(d1)
			d2, _ := Parse(out1)
			out2, _ := Write(d2)
			if string(out1) != string(out2) {
				t.Errorf("writer not idempotent\n--- out1 ---\n%s\n--- out2 ---\n%s", out1, out2)
			}
		})
	}
}

// TestHeadingKeepsItsInlineMarkup pins what a heading is written as. The
// writer used the PLAIN text of a heading's inlines, on the reasoning that
// a title underline must match the title's visible width -- so every
// emphasis, inline literal, role and reference inside a heading was silently
// dropped on the way out, and the round trip was STABLE afterwards, which
// is why no output-comparing check ever saw it.
//
// docutils only warns when an underline is SHORTER than its title line
// ("column_width(title) > len(underline)", states.py); a longer one is
// legal. So underlining the rendered line costs nothing.
//
// Measured by comparing round-tripped TREES over the 1564-file real-world
// corpus (/Users/Shared/rstcorpus/rtprobe): 398 files did not come back as
// themselves, and this was 188 of them.
func TestHeadingKeepsItsInlineMarkup(t *testing.T) {
	cases := []struct {
		name   string
		source string
		markup string // must appear in the WRITTEN source, not just reparse
	}{
		{"an inline literal in a heading", "``build.py`` tools\n==================\n\nbody\n", "``build.py``"},
		{"emphasis in a heading", "Why *not* a keyword\n===================\n\nbody\n", "*not*"},
		{"strong plus text", "About **PEP 8** here\n====================\n\nbody\n", "**PEP 8**"},
		{"a role in a heading", "The :emphasis:`x` role\n======================\n\nbody\n", "*x*"},
		{"an embedded-URI reference in a heading", "see `the docs <http://example.com/>`__ now\n=========================================\n\nbody\n", "<http://example.com/>"},
		{"markup in a NESTED heading", "Top\n===\n\nbody\n\nSub with ``code``\n-----------------\n\nmore\n", "``code``"},
		{"CONTROL: a plain heading is untouched", "Plain Title\n===========\n\nbody\n", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d1, err := Parse([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			out, err := Write(d1)
			if err != nil {
				t.Fatal(err)
			}
			d2, err := Parse(out)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(d1, d2) {
				t.Errorf("the tree did not come back as itself.\nwritten: %q\nbefore: %+v\nafter:  %+v", out, d1.Blocks, d2.Blocks)
			}
			// And the loss this is about: the markup must be IN the
			// output. A tree comparison alone would pass on a writer that
			// flattened BOTH ways consistently, which is precisely the
			// failure mode here -- write-twice was stable all along.
			if tc.markup != "" && !strings.Contains(string(out), tc.markup) {
				t.Errorf("the heading lost %q on the way out: %q", tc.markup, out)
			}
			// The underline must still be long enough, or the reparse
			// reports "Title underline too short." instead of a heading.
			if _, ok := d2.Blocks[0].(richdoc.Heading); !ok {
				t.Errorf("what came back is not a heading: %T", d2.Blocks[0])
			}
		})
	}
}
