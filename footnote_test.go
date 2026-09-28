package rst

import (
	"fmt"
	"strings"
	"testing"
)

// TestAFootnoteThatCitesItselfDoesNotRecurseForever pins a CRASH -- a stack
// overflow, on reST docutils reads without complaint. richdoc carries a note BY
// VALUE (the body is inlined at every reference), so a note citing itself is a
// cycle, and following it exhausted the goroutine stack.
//
// Nothing in the 1564-file corpus does this, which is why no sweep had found it.
// The shape turned up while asking a different question about footnote
// numbering, and the reference settles what it means: one <footnote> with two
// backrefs.
func TestAFootnoteThatCitesItselfDoesNotRecurseForever(t *testing.T) {
	for _, src := range []string{
		"Body [1]_.\n\n.. [1] A note citing itself [1]_.\n",
		"B [1]_.\n\n.. [1] One cites [2]_.\n\n.. [2] Two cites [1]_.\n",
		"B [CIT]_.\n\n.. [CIT] A citation citing itself [CIT]_.\n",
	} {
		// A run that does not return is the failure this is about, so the work
		// happens in a goroutine the test can outlive.
		done := make(chan string, 1)
		go func() {
			d, err := Parse([]byte(src))
			if err != nil {
				done <- "parse: " + err.Error()
				return
			}
			out, err := Write(d)
			if err != nil {
				done <- "write: " + err.Error()
				return
			}
			done <- string(out)
		}()
		select {
		case got := <-done:
			if strings.HasPrefix(got, "parse: ") || strings.HasPrefix(got, "write: ") {
				t.Errorf("%q: %s", src, got)
			}
		case <-t.Context().Done():
			t.Fatalf("%q: did not finish", src)
		}
	}
}

// TestANoteCitedTwiceIsOneDefinition pins the count. Three references to one
// note arrive as three Footnote values with the same blocks, and appending each
// produced three definitions with three different numbers -- a document that
// gained two footnotes it never had, in 117 of the 1564 corpus files.
func TestANoteCitedTwiceIsOneDefinition(t *testing.T) {
	out, msgs := reconstruct(t, "One [1]_ and again [1]_ and a third [1]_.\n\n.. [1] The note.\n")
	if msgs != 0 {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs, out)
	}
	if n := strings.Count(out, ".. ["); n != 1 {
		t.Errorf("want one definition, got %d:\n%s", n, out)
	}
	if n := strings.Count(out, "[1]_"); n != 3 {
		t.Errorf("want three references to [1], got %d:\n%s", n, out)
	}
}

// TestANoteCitedInsideANoteIsStillDefined pins the defect that made PEP 302 come
// out with references [10]_ and [11]_ and no definitions for them, two of its
// nine notes simply gone: writing a note's body can append MORE notes, and
// "for i, fn := range w.footnotes" fixes the length at entry, so those were
// numbered at the reference site and never defined.
func TestANoteCitedInsideANoteIsStillDefined(t *testing.T) {
	out, msgs := reconstruct(t, "Body with a note [1]_.\n\n.. [1] The outer note, which cites [2]_.\n\n.. [2] The inner note.\n")
	if msgs != 0 {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs, out)
	}
	if n := strings.Count(out, ".. ["); n != 2 {
		t.Errorf("want two definitions, got %d:\n%s", n, out)
	}
	// Every reference in the output must have a definition. The labels are
	// renumbered by design (richdoc carries no label), so the check is on the
	// PAIRING, not on the numbers.
	for _, ref := range []string{"[1]_", "[2]_"} {
		label := ".. [" + strings.TrimSuffix(strings.TrimPrefix(ref, "["), "]_") + "]"
		if strings.Contains(out, ref) && !strings.Contains(out, label) {
			t.Errorf("%s is referenced with no definition:\n%s", ref, out)
		}
	}
}

// TestEveryFootnoteReferenceHasADefinition is the invariant behind all three,
// checked on a shape with gaps in the author's own numbering -- which is what
// PEP 302 has (1,2,3,4,5,7,9,10,11) and what made the missing definitions hard
// to see by eye.
func TestEveryFootnoteReferenceHasADefinition(t *testing.T) {
	const src = "A [1]_ B [2]_ C [7]_ D [9]_.\n\n" +
		".. [1] one\n\n.. [2] two cites [9]_\n\n.. [7] seven\n\n.. [9] nine\n"
	out, msgs := reconstruct(t, src)
	if msgs != 0 {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs, out)
	}
	refs := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		for _, tok := range strings.Fields(l) {
			tok = strings.Trim(tok, ".,;:()")
			if strings.HasPrefix(tok, "[") && strings.HasSuffix(tok, "]_") {
				refs[strings.TrimSuffix(strings.TrimPrefix(tok, "["), "]_")] = true
			}
		}
	}
	for label := range refs {
		if !strings.Contains(out, ".. ["+label+"]") {
			t.Errorf("[%s]_ is referenced with no definition:\n%s", label, out)
		}
	}
	if len(refs) == 0 {
		t.Fatalf("the check found no references at all, so it proves nothing:\n%s", out)
	}
}

// TestAReferenceInsideARebuiltBlockDoesNotConsumeItsNote pins the defect that
// cost PEP 302 two of its nine notes. convertBlockNode rebuilds a whole family
// of constructs as reST SOURCE, and inside one of those a note reference is
// written back VERBATIM -- convertNoteRef never sees it. Counting it as a
// reference that consumes its definition dropped the definition and left the
// marker behind: a footnote reference pointing at nothing. 14 corpus files lost
// 42 definitions this way.
//
// There is a case for each container that can hold one, and that is the DRIFT
// GUARD: reconstructedAsSource has to agree with convertBlockNode's own
// raw-source cases, and a new construct that forgets to appear in the list fails
// here rather than being caught by reading two lists side by side.
func TestAReferenceInsideARebuiltBlockDoesNotConsumeItsNote(t *testing.T) {
	cases := []struct{ name, body string }{
		{"warning", ".. warning::\n   Cites [1]_.\n"},
		{"note", ".. note::\n   Cites [1]_.\n"},
		{"admonition", ".. admonition:: T\n   Cites [1]_.\n"},
		{"topic", ".. topic:: T\n\n   Cites [1]_.\n"},
		{"sidebar", ".. sidebar:: T\n\n   Cites [1]_.\n"},
		{"container", ".. container:: c\n\n   Cites [1]_.\n"},
		{"compound", ".. compound::\n\n   Cites [1]_.\n"},
		{"figure", ".. figure:: f.png\n\n   Cites [1]_.\n"},
		{"line block", "| Cites [1]_.\n"},
		// NOT at the top of the document: a LEADING field list is docinfo,
		// which richdoc carries as a Meta map of plain strings, so its inline
		// markup is gone by design and this case would be testing that boundary
		// instead of this one.
		{"field list", "Body first.\n\n:Field: Cites [1]_.\n"},
		{"definition list", "Term\n   Cites [1]_.\n"},
		{"rubric", ".. rubric:: Cites [1]_\n"},
	}
	for _, c := range cases {
		src := c.body + "\nBody text.\n\n.. [1] The note.\n"
		out, msgs := reconstruct(t, src)
		if msgs != 0 {
			t.Errorf("%s: the reconstruction gained %d diagnostic(s):\n%s", c.name, msgs, out)
		}
		if !strings.Contains(out, "[1]_") {
			t.Errorf("%s: the reference was lost:\n%s", c.name, out)
			continue
		}
		if !strings.Contains(out, ".. [1]") {
			t.Errorf("%s: the reference survived with no definition:\n%s", c.name, out)
		}
	}
}

// TestAnEmptyNoteBodyIsNotADedupKey is the other half of the dedup. An empty
// body carries no identity: PEP 653 writes nine notes whose content is
// unindented, which docutils reads as nine notes holding nothing but a
// diagnostic, and this package strips diagnostics -- so all nine arrived with
// empty blocks. Keyed on the body they merged into ONE, nine definitions becoming
// one, a worse loss than the duplication the dedup exists to prevent.
func TestAnEmptyNoteBodyIsNotADedupKey(t *testing.T) {
	// Three notes whose bodies docutils cannot attach, each referenced once.
	const src = "A [1]_ B [2]_ C [3]_.\n\n.. [1]\n\nnot indented\n\n.. [2]\n\nalso not\n\n.. [3]\n\nnor this\n"
	out, _ := reconstruct(t, src)
	if n := strings.Count(out, ".. ["); n != 3 {
		t.Errorf("want three definitions, got %d:\n%s", n, out)
	}
}

// TestAFigureCaptionKeepsItsInlineMarkup pins the last of this round's losses,
// found by the drift-guard case above rather than looked for: a figure's caption
// was rendered with doctree.AsText, which returns a node's TEXT. Every link,
// literal, emphasis and role in a caption was gone, and a footnote reference came
// out as its bare label -- "Cites 1." for "Cites [1]_." -- which also stopped the
// note's definition from being emitted at all.
func TestAFigureCaptionKeepsItsInlineMarkup(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"a literal", ".. figure:: f.png\n\n   A ``literal`` caption.\n", "``literal``"},
		{"emphasis", ".. figure:: f.png\n\n   An *emphatic* caption.\n", "*emphatic*"},
		{"a link", ".. figure:: f.png\n\n   See `the docs <https://example.com/>`__.\n", "https://example.com/"},
		// "Body text." and not "B.": docutils reads "B." as an upper-alpha
		// ENUMERATOR, so that fixture was an enumerated list, and the case was
		// failing on the list rather than on the caption.
		{"a note reference", ".. figure:: f.png\n\n   Cites [1]_.\n\nBody text.\n\n.. [1] The note.\n", "[1]_"},
	}
	for _, c := range cases {
		out, msgs := reconstruct(t, c.src)
		if msgs != 0 {
			t.Errorf("%s: the reconstruction gained %d diagnostic(s):\n%s", c.name, msgs, out)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: want %q in:\n%s", c.name, c.want, out)
		}
	}
}

// TestARenumberedNoteDoesNotStealAVerbatimLabel pins the collision the previous
// fix created. A note referenced only from inside a construct rebuilt as reST
// source keeps its OWN label on both sides -- marker and definition are written
// verbatim -- while footnoteNumber counts 1..N for the notes it inlines. Nothing
// stopped the two from choosing the same label, and PEP 550 came out with
// ".. [9]" and ".. [10]" TWICE: docutils reported a duplicate and those
// references stopped resolving.
//
// It was found by SET-DIFF, not by the total: the round trip went down by three
// files while the definition count went up, and a total would have shown a net
// gain.
func TestARenumberedNoteDoesNotStealAVerbatimLabel(t *testing.T) {
	// [9] is referenced only from inside a warning, so it keeps its label; the
	// other nine notes are inlined and numbered, and one of them would otherwise
	// be given 9 as well.
	var b strings.Builder
	b.WriteString(".. warning::\n   Cites [9]_.\n\n")
	for i := 1; i <= 9; i++ {
		fmt.Fprintf(&b, "Para %d cites [a%d]_.\n\n", i, i)
	}
	b.WriteString(".. [9] The verbatim note.\n\n")
	for i := 1; i <= 9; i++ {
		fmt.Fprintf(&b, ".. [a%d] Note %d.\n\n", i, i)
	}
	out, msgs := reconstruct(t, b.String())
	if msgs != 0 {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs, out)
	}
	seen := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		if label, ok := strings.CutPrefix(l, ".. ["); ok {
			label = strings.SplitN(label, "]", 2)[0]
			if seen[label] {
				t.Errorf("label [%s] is defined twice:\n%s", label, out)
			}
			seen[label] = true
		}
	}
	if !seen["9"] {
		t.Errorf("the verbatim note lost its own label:\n%s", out)
	}
}
