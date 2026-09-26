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

// TestReconstructedInlineMarkupSurvives covers every construct whose reST
// source this package rebuilds from a tree and whose content docutils parses
// for INLINE markup. All of them used doctree.AsText, which returns a node's
// TEXT, so a definition term "**Read the Docs**" came back as plain "Read the
// Docs": the document changed, once, silently.
//
// inlineSourceOf replaces that by converting the node's children to richdoc
// inlines and writing them with this package's own inline writer -- one
// spelling of the inline grammar, used in both directions.
//
// Found by comparing the doctree of a SOURCE with the doctree of its own
// round trip (/Users/Shared/rstcorpus/fidprobe); 78 of the 1564 real-world
// corpus files change. A round-trip comparison cannot see this class at all,
// since the flattened text is already in the first tree.
//
// Each case was checked against the reference first, because "parses inline
// markup" is not true of every container: an option-list flag and a comment
// are verbatim there, and they are the CONTROLS.
func TestReconstructedInlineMarkupSurvives(t *testing.T) {
	cases := []struct {
		name, source, want string
	}{
		{"a definition term", "**Read the Docs**\n   is a service.\n", "**Read the Docs**"},
		{"a term's classifier", "term with ``code`` : *classifier*\n   definition\n", "``code`` : *classifier*"},
		{"an admonition title", ".. admonition:: a *emph* b\n\n   body\n", ".. admonition:: a *emph* b"},
		{"a topic title", ".. topic:: a ``lit`` b\n\n   body\n", ".. topic:: a ``lit`` b"},
		{"a sidebar subtitle", ".. sidebar:: T\n   :subtitle: a *em* b\n\n   body\n", ":subtitle: a *em* b"},
		{"a rubric argument", ".. rubric:: a *emph* b\n", ".. rubric:: a *emph* b"},
		{"a line block's line", "| line with *emph*\n| second\n", "| line with *emph*"},
		{"a field name, not leading", "intro\n\n:field with ``code``: value\n", ":field with ``code``:"},
		// CONTROLS: content docutils does NOT parse for inline markup, which
		// must come back exactly as written rather than through the inline
		// writer (which would escape or re-spell it).
		{"CONTROL: an option-list flag is verbatim", "-o *notmarkup*  description\n", "-o *notmarkup*"},
		{"CONTROL: a comment is verbatim", ".. this is *not* parsed\n", ".. this is *not* parsed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse([]byte(tc.source))
			if err != nil {
				t.Fatal(err)
			}
			out, err := Write(d)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(out), tc.want) {
				t.Errorf("want %q in the output:\n%s", tc.want, out)
			}
		})
	}
	// And the boundary this does NOT cross, asserted so it is a decision
	// rather than an oversight: a LEADING field list becomes Document.Meta,
	// a map[string]string, which has nowhere to put markup.
	t.Run("BOUNDARY: a leading field list flattens into Meta", func(t *testing.T) {
		d, err := Parse([]byte(":leading with ``code``: value\n\nbody\n"))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := d.Meta["leading with code"]; !ok {
			t.Errorf("Meta = %v, want the flattened key: this is the model's limit, not a defect", d.Meta)
		}
	})
}
