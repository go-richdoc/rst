package rst

import (
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestACitationKeepsItsKey pins the difference between reST's two note
// constructs. A footnote is a note at the foot of the page; a CITATION names a
// bibliographic entry, and its key is what the reader sees -- "[CIT2002]".
// Converting it as a footnote inlined the body and threw the key away, so the
// reconstruction said "[1]_" and ".. [1]" where the author wrote "[CIT2002]_"
// and ".. [CIT2002]": a number in place of a key, in 15 of the 1564 real-world
// files.
//
// richdoc.CrossRef with RefCite is what models it, and the write side has always
// emitted "[target]_" for that -- only the parse side was missing.
func TestACitationKeepsItsKey(t *testing.T) {
	const src = "A citation [CIT2002]_ here.\n\n.. [CIT2002] The citation.\n"
	out, msgs := reconstruct(t, src)
	if msgs != 0 {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs, out)
	}
	for _, want := range []string{"[CIT2002]_", ".. [CIT2002]"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[1]_") {
		t.Errorf("the key was replaced by a number:\n%s", out)
	}
}

// TestACitationTargetIsTheKeyAsWritten pins the case, which is not decoration:
// docutils NORMALISES a name to lower case for matching, so "[CIT2002]_" carries
// refname="cit2002". Writing the refname still resolves -- reST names are
// case-insensitive -- and shows "cit2002" where the document says "CIT2002".
func TestACitationTargetIsTheKeyAsWritten(t *testing.T) {
	doc, err := Parse([]byte("A citation [CIT2002]_ here.\n\n.. [CIT2002] The citation.\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p, ok := doc.Blocks[0].(richdoc.Paragraph)
	if !ok {
		t.Fatalf("want a paragraph first, got %T", doc.Blocks[0])
	}
	found := false
	for _, in := range p.Inlines {
		if ref, ok := in.(richdoc.CrossRef); ok {
			found = true
			if ref.Kind != richdoc.RefCite {
				t.Errorf("Kind = %v, want RefCite", ref.Kind)
			}
			if ref.Target != "CIT2002" {
				t.Errorf("Target = %q, want %q", ref.Target, "CIT2002")
			}
		}
	}
	if !found {
		t.Errorf("no CrossRef in %#v", p.Inlines)
	}
}

// TestAFootnoteIsStillAFootnote is the CONTROL: the change is about citations,
// and a footnote must keep inlining its body as it did. It passes either way and
// is here so the case above cannot be read as "notes are CrossRefs now".
func TestAFootnoteIsStillAFootnote(t *testing.T) {
	doc, err := Parse([]byte("See [1]_ here.\n\n.. [1] The note.\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	p := doc.Blocks[0].(richdoc.Paragraph)
	for _, in := range p.Inlines {
		if _, ok := in.(richdoc.Footnote); ok {
			return
		}
	}
	t.Errorf("a footnote reference should still inline its body: %#v", p.Inlines)
}
