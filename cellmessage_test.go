package rst

import (
	"strings"
	"testing"
)

// TestACellDoesNotPrintTheParsersComplaint pins a sentence no author wrote.
//
// A table cell is flattened to INLINES (richdoc.Cell has no Blocks), so a
// <system_message> in one reached cellBlockInlines' default and came back as its
// own TEXT: "Unexpected possible title overline or transition. Treating it as
// ordinary text because it's so short." printed into the table, where every reader
// sees it. convertBlockNode's TagSystemMessage case -- the one that drops
// diagnostics -- is never consulted on the cell path.
//
// Measured over the 1564-file corpus: 9 files, 29 occurrences. diagprobe, which
// counts <system_message> elements in the reparse, read 0 the whole time: a
// message that became a PARAGRAPH is no longer a diagnostic.
func TestACellDoesNotPrintTheParsersComplaint(t *testing.T) {
	// "??" alone in a simple-table cell is the shape behind 28 of the 29: the
	// parser offers an INFO about a possible overline or transition.
	const src = "=====  =====\nA      B\n=====  =====\nyes    ??\n=====  =====\n"

	// Not "the output has no diagnostic": this INPUT has one -- "??" alone in a
	// cell raises the same INFO whichever table spelling holds it -- so the
	// question is whether the reconstruction ADDS any, counted the same way on
	// both sides. An absolute count here asserts something about the corpus
	// shape rather than about this converter.
	out, msgs := reconstruct(t, src)
	if before := countSystemMessages(t, []byte(src)); msgs > before {
		t.Errorf("the reconstruction gained %d diagnostic(s) (%d -> %d):\n%s", msgs-before, before, msgs, out)
	}
	if strings.Contains(out, "Unexpected possible title overline") {
		t.Errorf("a diagnostic reached the cell as text:\n%s", out)
	}
	// The author's own cell has to survive the removal.
	if !strings.Contains(out, "??") {
		t.Errorf("the cell's own content went with the diagnostic:\n%s", out)
	}

	// CONTROL: KeepDiagnostics must still keep it. Without this, dropping the
	// cell's content wholesale would pass the assertions above.
	kept, err := ParseWithOptions([]byte(src), Options{KeepDiagnostics: true})
	if err != nil {
		t.Fatalf("ParseWithOptions: %v", err)
	}
	keptOut, err := Write(kept)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(string(keptOut), "Unexpected possible title overline") {
		t.Errorf("KeepDiagnostics dropped a message it exists to keep:\n%s", keptOut)
	}
}

// TestACellKeepsTheSourceOfAConstructDocutilsRefused is the other half of the
// same policy, inside a cell: at ERROR level and above docutils REFUSED to build
// the construct and quotes the author's source in a <literal_block>. That quote is
// content, so it stays -- dropping the message must not drop the table with it.
func TestACellKeepsTheSourceOfAConstructDocutilsRefused(t *testing.T) {
	const src = ".. list-table::\n\n   * - a\n     - +---+\n       | x\n       +---+\n"

	out, msgs := reconstruct(t, src)
	if before := countSystemMessages(t, []byte(src)); msgs > before {
		t.Errorf("the reconstruction gained %d diagnostic(s) (%d -> %d):\n%s", msgs-before, before, msgs, out)
	}
	if strings.Contains(out, "Malformed table") {
		t.Errorf("the complaint reached the cell as text:\n%s", out)
	}
	if !strings.Contains(out, "| x") {
		t.Errorf("the refused construct's own source is gone:\n%s", out)
	}
}

// TestAnOrdinaryCellIsUnchanged is the CONTROL: it passes either way, and it is
// here so the two tests above cannot be read as "table cells did not work".
func TestAnOrdinaryCellIsUnchanged(t *testing.T) {
	out, msgs := reconstruct(t, "=====  =====\nA      B\n=====  =====\nyes    no\n=====  =====\n")
	if msgs != 0 {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs, out)
	}
	for _, want := range []string{"A", "B", "yes", "no"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}
