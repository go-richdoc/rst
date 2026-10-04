package rst

import (
	"strings"
	"testing"
)

// TestARefusedRoleAtTheStartOfALineIsNotAFieldList pins a paragraph that became
// DOCINFO.
//
// A ":pep:" role whose argument is not a number is refused by docutils, and this
// package passes the text through escaped, as it does for every <problematic>. That
// put ":pep:\`PEP 522: Allow BlockingIOError ..." at the start of a line -- and a
// line beginning with a colon is a FIELD MARKER: the colon after "pep" is followed
// by a backslash, so it is a legal name character, and the one after "522" is
// followed by a space, so it closes the name. docutils read the whole line as a
// field, put it in the document's docinfo, and warned "Field list ends without a
// blank line; unexpected unindent." about the next line.
//
// 6 messages in 2 corpus files, and in both the paragraph was gone.
func TestARefusedRoleAtTheStartOfALineIsNotAFieldList(t *testing.T) {
	const src = "Text before.\n\n:pep:`PEP 522: Allow BlockingIOError in security sensitive APIs on Linux\n<522>`.\n"

	out, msgs := reconstruct(t, src)
	if before := countSystemMessages(t, []byte(src)); msgs > before {
		t.Errorf("the reconstruction gained %d diagnostic(s) (%d -> %d):\n%s", msgs-before, before, msgs, out)
	}
	if !strings.HasPrefix(strings.TrimSpace(strings.Split(out, "\n\n")[1]), `\:pep:`) {
		t.Errorf("the line still starts with an unescaped colon:\n%s", out)
	}
	if strings.Contains(out, "\n\n<522>") {
		t.Errorf("the second line was cut loose from the first:\n%s", out)
	}
}

// TestADirectiveArgumentKeepsItsContinuationIndent pins the other half of the same
// diagnostic. A directive's argument may run to several lines, and an unindented
// second line ENDS the directive: docutils says "Explicit markup ends without a
// blank line; unexpected unindent." and the rest of the argument becomes a
// paragraph. sphinx's own rubric test is where it showed.
func TestADirectiveArgumentKeepsItsContinuationIndent(t *testing.T) {
	const src = ".. rubric:: This is\n   a multiline rubric\n"

	out, msgs := reconstruct(t, src)
	if before := countSystemMessages(t, []byte(src)); msgs > before {
		t.Errorf("the reconstruction gained %d diagnostic(s) (%d -> %d):\n%s", msgs-before, before, msgs, out)
	}
	if !strings.Contains(out, ".. rubric:: This is\n   a multiline rubric") {
		t.Errorf("the argument's second line lost its indent:\n%q", out)
	}
}

// TestARealFieldListIsStillAFieldList is the CONTROL: the escape is positional and
// applies to a PARAGRAPH's first line, so a field list an author actually wrote has
// to come back as one. It passes either way.
func TestARealFieldListIsStillAFieldList(t *testing.T) {
	out, msgs := reconstruct(t, "Intro.\n\n:Author: Someone\n:Version: 2\n")
	if before := countSystemMessages(t, []byte("Intro.\n\n:Author: Someone\n:Version: 2\n")); msgs > before {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs-before, out)
	}
	for _, want := range []string{":Author: Someone", ":Version: 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, `\:Author:`) {
		t.Errorf("a real field list was escaped into text:\n%s", out)
	}
}
