package rst

import (
	"strings"
	"testing"
)

// reconstruct parses src, writes it back, and returns the reST plus the number
// of diagnostics docutils has about that reST. The count is the point: a
// reconstruction that makes docutils say something has put text into the
// document that the author never wrote.
func reconstruct(t *testing.T, src string) (string, int) {
	t.Helper()
	d, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out, err := Write(d)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	return string(out), countSystemMessages(t, out)
}

// TestALineBlockLineKeepsItsContinuationIndent pins the largest of the
// diagnostics the reconstruction used to introduce: 21 of 27, all in PEP 368.
//
// One <line> can span several source lines, and it is the INDENT that says so --
// confirmed against the reference, which reads
//
//	| ``__setitem__(integer | slice, integer) ->
//	  None``
//
// as ONE <line> holding ONE <literal>. Written at column 0 the line block simply
// ends there, so each wrapped line cost three messages: "Line block ends without
// a blank line" plus the unterminated literal and emphasis the break left behind.
func TestALineBlockLineKeepsItsContinuationIndent(t *testing.T) {
	const src = "| ``__setitem__(integer | slice, integer) ->\n  None``\n| second line\n"
	out, msgs := reconstruct(t, src)
	if msgs != 0 {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs, out)
	}
	if !strings.Contains(out, "\n  None``") {
		t.Errorf("the continuation lost its indent:\n%s", out)
	}
}

// TestAFieldValueKeepsItsContinuationIndent is the same defect in the docinfo
// field list: an address or a copyright notice runs to several lines, and a
// field body's continuation has to be indented or the list ends there
// ("Field list ends without a blank line; unexpected unindent.").
func TestAFieldValueKeepsItsContinuationIndent(t *testing.T) {
	const src = ":Address: 123 Example Street\n" +
		"          Example, EX  Canada\n" +
		"          A1B 2C3\n" +
		"\nBody text.\n"
	out, msgs := reconstruct(t, src)
	if msgs != 0 {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs, out)
	}
	if !strings.Contains(out, "\n   Example, EX  Canada") {
		t.Errorf("the continuation lost its indent:\n%s", out)
	}
}

// TestAContentsDirectiveComesBackAsItself pins the third cause. docutils gives
// ".. contents::" a <topic> whose only child besides the title is a <pending>;
// skipping the pending (which must never reach a reader) left a topic with a
// title and NO CONTENT, which docutils rejects. The options come back out of the
// pending's own details.
func TestAContentsDirectiveComesBackAsItself(t *testing.T) {
	const src = ".. contents:: Installation methods\n   :depth: 2\n   :local:\n   :backlinks: none\n\nBody\n====\n\ntext\n"
	out, msgs := reconstruct(t, src)
	if msgs != 0 {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs, out)
	}
	for _, want := range []string{
		".. contents:: Installation methods",
		":depth: 2",
		":local:",
		":backlinks: none",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	// The classes docutils derives itself must NOT be written back as an
	// explicit :class:, or the next parse has them twice.
	if strings.Contains(out, ":class: contents") {
		t.Errorf("the derived classes were written back:\n%s", out)
	}
	// And ":name:", which ".. contents::" does not even accept, came from the
	// implicit target the TITLE created.
	if strings.Contains(out, ":name: installation methods") {
		t.Errorf("a :name: option was invented from the title:\n%s", out)
	}
}

// TestABareContentsDirectiveKeepsItsOptions covers the two spellings that both
// mean "no backlinks" and that the tree DOES distinguish: ":backlinks: none"
// leaves "backlinks: None" in the pending's details and a bare ":backlinks:"
// leaves "backlinks: ”".
func TestABareContentsDirectiveKeepsItsOptions(t *testing.T) {
	for src, want := range map[string]string{
		".. contents::\n   :backlinks: none\n\nB\n=\n\nx\n": ":backlinks: none",
		".. contents::\n   :backlinks:\n\nB\n=\n\nx\n":      ":backlinks:",
	} {
		out, msgs := reconstruct(t, src)
		if msgs != 0 {
			t.Errorf("%q: the reconstruction gained %d diagnostic(s):\n%s", src, msgs, out)
		}
		if !strings.Contains(out, want) {
			t.Errorf("%q: want %q in:\n%s", src, want, out)
		}
	}
}

// TestACommentedOutDirectiveStaysCommented is the fourth cause, and the one with
// the worst symptom: sphinx's own index page comments out a whole admonition
// (".. .. admonition:: ..."), and inside a container the reconstruction wrote it
// back as a REAL directive. A construct the author switched OFF came back on.
//
// The cause was a missing case: rawChildSource had no TagComment, so the
// fallback returned the comment's TEXT without the ".." that makes it one. At
// the top level the dedicated rawComment was used and the same document was
// fine, which is why only the nested spelling showed it.
func TestACommentedOutDirectiveStaysCommented(t *testing.T) {
	const src = ".. container:: feat\n\n" +
		"   .. .. admonition:: Title Here\n" +
		"   ..    :class: x\n\n" +
		"   ..    Body text.\n\n" +
		"   .. admonition:: Real One\n" +
		"      :class: y\n\n" +
		"      Real body.\n"
	out, msgs := reconstruct(t, src)
	if msgs != 0 {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs, out)
	}
	// The commented-out admonition must still be inside a comment, and the real
	// one must still be a directive.
	if !strings.Contains(out, ".. .. admonition:: Title Here") {
		t.Errorf("a commented-out directive was switched on:\n%s", out)
	}
	if !strings.Contains(out, ".. admonition:: Real One") {
		t.Errorf("the real admonition was lost:\n%s", out)
	}
}
