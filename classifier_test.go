package rst

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-docutils/docutils/doctree"
	docrst "github.com/go-docutils/docutils/rst"
)

// TestATermKeepsItsOwnColon pins a term that came back SPLIT.
//
// reST reads "term : classifier" as a term plus a classifier, and an author who wants
// the colon inside the term escapes it -- PEP 362 writes
// "* return_annotation \: object" for exactly that. The escape works because docutils
// parses a term's inline content FIRST, which turns "\:" into NUL+":", and the
// delimiter pattern ' +: +' (states.Text.classifier_delimiter, read for this) cannot
// match across the NUL.
//
// This package rebuilds a definition list as reST source and wrote the term's colon
// BARE, so the next parse split it: <term>return_annotation</term> plus
// <classifier>object</classifier> where the source had one term. 8 terms in 1 corpus
// file, out of 1030 terms.
func TestATermKeepsItsOwnColon(t *testing.T) {
	const src = "- return_annotation \\: object\n      The annotation.\n"
	out, d1, d2 := roundTrip(t, src)
	if !reflect.DeepEqual(d1.Blocks, d2.Blocks) {
		t.Errorf("not a fixed point:\n%s", out)
	}
	if !strings.Contains(out, `\:`) {
		t.Errorf("the term's colon was written bare, so the next parse splits it:\n%s", out)
	}
	// The assertion that matters is on the DOCTREE, and neither the round trip nor
	// PlainText can make it: the list is a RawBlock, so PlainText contributes
	// nothing for it, and a split term rejoined with " : " gives back the SAME raw
	// text -- which is exactly why this defect was invisible to rtprobe and showed
	// only in fidprobe, the probe that compares the two doctrees.
	if n := countElements(t, out, doctree.TagClassifier); n != 0 {
		t.Errorf("the escaped colon produced %d classifier(s), so the term was split:\n%s", n, out)
	}
	if n := countElements(t, out, doctree.TagTerm); n != 1 {
		t.Errorf("want exactly one term, got %d:\n%s", n, out)
	}
}

// countElements reparses src with docutils and counts one tag -- the question a
// round trip through richdoc cannot answer for a construct that comes back as a
// RawBlock.
func countElements(t *testing.T, src string, tag string) int {
	t.Helper()
	var walk func(n doctree.Node) int
	walk = func(n doctree.Node) int {
		el, ok := n.(*doctree.Element)
		if !ok {
			return 0
		}
		c := 0
		if el.Tag == tag {
			c++
		}
		for _, ch := range el.Children {
			c += walk(ch)
		}
		return c
	}
	return walk(docrst.Parse(src))
}

// TestARealClassifierIsStillAClassifier is the CONTROL that keeps the escape from
// swallowing the feature it protects: "term : classifier" in the SOURCE is two things,
// and must come back as two.
func TestARealClassifierIsStillAClassifier(t *testing.T) {
	const src = "term : classifier\n    The definition.\n"
	out, d1, d2 := roundTrip(t, src)
	if !reflect.DeepEqual(d1.Blocks, d2.Blocks) {
		t.Errorf("not a fixed point:\n%s", out)
	}
	if strings.Contains(out, `\:`) {
		t.Errorf("a real classifier delimiter was escaped away:\n%s", out)
	}
	if n := countElements(t, out, doctree.TagClassifier); n != 1 {
		t.Errorf("want the classifier back, got %d:\n%s", n, out)
	}
}

// TestAnOrdinaryTermIsUnchanged is the second control: a term with no colon must gain
// nothing. It passes either way.
func TestAnOrdinaryTermIsUnchanged(t *testing.T) {
	out, _, _ := roundTrip(t, "plain term\n    The definition.\n")
	if strings.Contains(out, `\`) {
		t.Errorf("an ordinary term gained an escape:\n%s", out)
	}
}

// TestEscapeClassifierDelimiterSpacing pins the pattern's own shape: the reference's
// delimiter is ' +: +', so the escape has to survive several spaces on either side,
// and a colon with no space around it is NOT a delimiter and must be left alone.
func TestEscapeClassifierDelimiterSpacing(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a : b", `a \: b`},
		{"a   :   b", `a   \:   b`},
		{"a:b", "a:b"},
		{"a: b", "a: b"},
		{"a :b", "a :b"},
		{"a : b : c", `a \: b \: c`},
		{"no colon here", "no colon here"},
	}
	for _, c := range cases {
		if got := escapeClassifierDelimiter(c.in); got != c.want {
			t.Errorf("escapeClassifierDelimiter(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
