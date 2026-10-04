package rst

import (
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestAHeadingCannotBecomeADirective pins a vulnerability, not a tidiness problem.
//
// A title line is the FIRST LINE OF A BLOCK, and reST tries explicit markup before
// it tries a title. A heading whose text began ".. include:: /etc/passwd" was written
// verbatim, so the next parse read a DIRECTIVE with the underline as a transition --
// and a docutils parse with a source path then READS THAT FILE. Demonstrated against
// the reference, which inlined /etc/passwd's contents into the document.
//
// The route in is ordinary: this package's own Markdown sibling drops inline HTML and
// keeps its text, so "# <b>.. include:: /etc/passwd</b>" in an untrusted Markdown
// document arrives here as a Heading whose text starts with "..".
//
// A probe over every position a document can hold text (injectprobe, beside the
// corpus) reported this as the only one: 14 (payload, position) pairs, all of them a
// heading, and 0 after the fix.
func TestAHeadingCannotBecomeADirective(t *testing.T) {
	payloads := []struct{ name, text string }{
		{"a directive", ".. include:: /etc/passwd"},
		{"a comment", ".. just a comment"},
		{"a bullet", "- a bullet"},
		{"a field", ":field: a value"},
		{"a doctest block", ">>> doctest()"},
		{"a grid table", "+---+"},
		{"a transition", "----"},
	}
	for _, p := range payloads {
		for _, level := range []int{1, 2} {
			doc := richdoc.New().H(level, richdoc.Txt(p.text)).Doc()
			out, err := Write(doc)
			if err != nil {
				t.Fatalf("%s: Write: %v", p.name, err)
			}
			back, err := Parse(out)
			if err != nil {
				t.Fatalf("%s: Parse: %v", p.name, err)
			}
			if len(back.Blocks) == 0 {
				t.Errorf("%s at level %d: the heading disappeared:\n%q", p.name, level, out)
				continue
			}
			h, ok := back.Blocks[0].(richdoc.Heading)
			if !ok {
				t.Errorf("%s at level %d: came back as %T, not a Heading:\n%q",
					p.name, level, back.Blocks[0], out)
				continue
			}
			if got := richdoc.PlainText(&richdoc.Document{Blocks: []richdoc.Block{h}}); got != p.text {
				t.Errorf("%s at level %d: the heading's text came back as %q, want %q",
					p.name, level, got, p.text)
			}
			if n := countSystemMessages(t, out); n != 0 {
				t.Errorf("%s at level %d: the output has %d diagnostic(s):\n%s", p.name, level, n, out)
			}
		}
	}
}

// TestAnOrdinaryHeadingIsUnchanged is the CONTROL. The escape is positional, so a
// heading that does not begin with a block marker must come out exactly as before --
// no stray backslash, and an underline still at least as long as its title.
func TestAnOrdinaryHeadingIsUnchanged(t *testing.T) {
	doc := richdoc.New().H(1, richdoc.Txt("A Normal Heading")).Doc()
	out, err := Write(doc)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	const want = "A Normal Heading\n================"
	if !strings.Contains(string(out), want) {
		t.Errorf("got:\n%q\nwant it to contain:\n%q", string(out), want)
	}
	if strings.Contains(string(out), `\`) {
		t.Errorf("an ordinary heading gained an escape:\n%q", string(out))
	}
}

// TestAnEscapedHeadingsUnderlineIsLongEnough pins the ordering: the escape adds a
// character to the title line, so the underline has to be measured AFTER it.
// docutils warns when an underline is shorter than its title.
func TestAnEscapedHeadingsUnderlineIsLongEnough(t *testing.T) {
	out, err := Write(richdoc.New().H(1, richdoc.Txt(".. include:: x")).Doc())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("want a title and an underline, got %q", string(out))
	}
	if len(lines[1]) < len(lines[0]) {
		t.Errorf("the underline (%d) is shorter than the title (%d):\n%q",
			len(lines[1]), len(lines[0]), string(out))
	}
	if n := countSystemMessages(t, out); n != 0 {
		t.Errorf("the output has %d diagnostic(s):\n%s", n, out)
	}
}

// TestALongLineIsAnErrorNotAnEmptyDocument pins what a converter owes its caller
// when the parser refuses a document.
//
// docutils refuses a document with a line over 10000 characters -- its own
// denial-of-service guard -- by producing one <system_message> and no tree. This
// package drops diagnostics by default, so that refusal arrived as an EMPTY
// document with nothing saying why: the worst possible answer for a converter.
func TestALongLineIsAnErrorNotAnEmptyDocument(t *testing.T) {
	src := []byte("Para.\n\n" + strings.Repeat("a", 10001) + "\n")
	doc, err := Parse(src)
	if err == nil {
		t.Fatalf("want an error, got a document with %d block(s)", len(doc.Blocks))
	}
	if !strings.Contains(err.Error(), "line 3 exceeds the line-length limit") {
		t.Errorf("the error should say which line and what the limit is, got: %v", err)
	}

	// A NEGATIVE limit is the escape hatch for a caller that would rather spend
	// the time than lose the document.
	doc, err = ParseWithOptions(src, Options{LineLengthLimit: -1})
	if err != nil {
		t.Fatalf("with no limit: %v", err)
	}
	if len(doc.Blocks) < 2 {
		t.Errorf("with no limit the document should be whole, got %d block(s)", len(doc.Blocks))
	}
}

// TestAnOrdinaryDocumentIsUnaffectedByTheLimit is the control: the corpus's longest
// line is 1005 characters, so nothing real comes near the limit.
func TestAnOrdinaryDocumentIsUnaffectedByTheLimit(t *testing.T) {
	doc, err := Parse([]byte("Para with a " + strings.Repeat("long ", 500) + "line.\n"))
	if err != nil {
		t.Fatalf("a 2500-character line should be fine: %v", err)
	}
	if len(doc.Blocks) != 1 {
		t.Errorf("want one paragraph, got %d block(s)", len(doc.Blocks))
	}
}
