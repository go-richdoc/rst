// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"reflect"
	"testing"
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
