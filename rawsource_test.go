package rst

import (
	"strings"
	"testing"
)

// TestPendingIsNotRawSourceContent guards a leak the docutils/rst
// v0.99.0 bump surfaced: ".. contents::" now parses to a <topic> whose
// only child is a <pending>, an INTERNAL node whose text is the parser's
// own ".. internal attributes:" block. parse.go already drops <pending>
// on the block path, but the raw-source reconstruction walked it as
// content and put ".transform: docutils.transforms.parts.Contents" into
// the rendered document.
func TestPendingIsNotRawSourceContent(t *testing.T) {
	d, err := Parse([]byte("x\n\n.. contents::\n\nT\n=\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := Write(d)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, leak := range []string{"internal attributes", ".transform:", "docutils.transforms"} {
		if strings.Contains(string(out), leak) {
			t.Errorf("parser internals reached the output (%q):\n%s", leak, out)
		}
	}
	if !strings.Contains(string(out), ".. topic:: Contents") {
		t.Errorf("the topic itself was lost:\n%s", out)
	}
}

// TestNestedStructureSurvivesRawSource covers the five raw* helpers that
// share the contentParts loop (admonition, topic, container, compound,
// figure). All of them rendered content with doctree.AsText, which
// returns text with NO structure -- so a bullet list inside a note came
// out as "onetwo", its items concatenated with neither bullets nor
// separators. A reader of the converted document could not tell there
// had been a list.
//
// Round-trip STABILITY is the property asserted, not just the text:
// parsing the output again and re-writing it must give the same bytes.
// A reconstruction that merely looks plausible can still fail to
// re-parse, and that is the difference that matters for a format meant
// to survive an edit.
func TestNestedStructureSurvivesRawSource(t *testing.T) {
	cases := []struct{ name, source string }{
		{"a bullet list in a note", ".. note::\n\n   - one\n   - two\n"},
		{"an enumerated list in a note", ".. note::\n\n   1. first\n   2. second\n"},
		{"a paragraph and then a list", ".. note::\n\n   text\n\n   - a\n   - b\n"},
		{"a literal block in a note", ".. note::\n\n   ::\n\n      code\n"},
		{"a list in a topic", ".. topic:: T\n\n   - x\n"},
		{"a list in a container", ".. container:: c\n\n   - y\n"},
		{"a legend list in a figure", ".. figure:: a.png\n\n   caption\n\n   - legend item\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse([]byte(tc.source))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			first, err := Write(d)
			if err != nil {
				t.Fatalf("write: %v", err)
			}
			// The output must be the SOURCE back. An earlier version of
			// this looked for "- " or "::" anywhere in the output, which
			// could not fail: ".. note::" contains "::". It passed on the
			// old flattening path too, so it tested nothing.
			if string(first) != tc.source {
				t.Errorf("round trip changed the document:\n--- want\n%s--- got\n%s", tc.source, first)
			}
			d2, err := Parse(first)
			if err != nil {
				t.Fatalf("re-parse: %v", err)
			}
			second, err := Write(d2)
			if err != nil {
				t.Fatalf("re-write: %v", err)
			}
			if string(first) != string(second) {
				t.Errorf("round trip is not stable:\n--- pass 1\n%s--- pass 2\n%s", first, second)
			}
		})
	}
}

// TestFlattenedListWasTheDefect is the narrow regression: the exact
// output the old AsText path produced must not come back.
func TestFlattenedListWasTheDefect(t *testing.T) {
	d, _ := Parse([]byte(".. note::\n\n   - one\n   - two\n"))
	out, _ := Write(d)
	if strings.Contains(string(out), "onetwo") {
		t.Errorf("the list was flattened again:\n%s", out)
	}
}
