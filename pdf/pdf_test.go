// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build !js

package pdf_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
	"github.com/go-richdoc/rst/pdf"
	"github.com/go-tex/engine"
)

// para builds the smallest document there is.
func para(words string) *richdoc.Document {
	return richdoc.New().P(richdoc.Text{Value: words}).Doc()
}

// everything builds one carrying each thing a converter can produce, so what
// the whole chain keeps is asserted rather than assumed.
func everything() *richdoc.Document {
	return richdoc.New().
		H(1, richdoc.Text{Value: "Heading"}).
		P(richdoc.Text{Value: "Some "}, richdoc.Emph{Inlines: []richdoc.Inline{richdoc.Text{Value: "slanted"}}},
			richdoc.Text{Value: " and "}, richdoc.Strong{Inlines: []richdoc.Inline{richdoc.Text{Value: "heavy"}}},
			richdoc.Text{Value: " words, and enough of them that a line has to break somewhere."}).
		UList(true,
			richdoc.ListItem{Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Text{Value: "alpha"}}}}},
			richdoc.ListItem{Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Text{Value: "beta"}}}}}).
		CodeBlock("", "fmt.Println()").
		P(richdoc.Text{Value: "Ete a Noel, ou ca?"}).
		Doc()
}

// says reads the words back out of a PDF with poppler, so what is checked is
// what another implementation finds rather than what this one believes it
// wrote.
func says(t *testing.T, data []byte) string {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext is not installed")
	}
	f, err := os.CreateTemp(t.TempDir(), "*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	f.Close()
	out, err := exec.Command("pdftotext", f.Name(), "-").Output()
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	return string(out)
}

func TestADocumentComesOutAsAPDFThatSaysIt(t *testing.T) {
	data, err := pdf.Write(para("Some words, and a second sentence."), pdf.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		t.Fatalf("what came back is not a PDF")
	}
	if got := says(t, data); !strings.Contains(got, "Some words") {
		t.Errorf("the PDF does not say it:\n%s", got)
	}
}

func TestWhatSurvivesTheWholeWay(t *testing.T) {
	// The point of this package is that a document keeps its shape across
	// two libraries, a reST re-parse, a LaTeX render, and a TeX engine. Each
	// of these is a thing a converter can produce, and each is read back out
	// of the finished PDF by poppler rather than trusted.
	data, err := pdf.Write(everything(), pdf.Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := says(t, data)
	for _, want := range []string{
		"Heading", "slanted", "heavy", "alpha", "beta", "fmt.Println", "Noel",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%q did not survive:\n%s", want, got)
		}
	}
}

func TestHowManyPagesItCameTo(t *testing.T) {
	// A document that typesets to nothing is not an error anywhere in the
	// chain, so a caller about to hand somebody an empty file would
	// otherwise have no way to know.
	var buf bytes.Buffer
	pages, err := pdf.WriteTo(&buf, para("Words."), pdf.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if pages != 1 {
		t.Errorf("it came to %d pages", pages)
	}
}

func TestThereIsNoDocument(t *testing.T) {
	if _, err := pdf.Write(nil, pdf.Options{}); err == nil {
		t.Error("nothing at all was typeset")
	}
}

func TestSomewhereThatWillNotBeWrittenTo(t *testing.T) {
	if _, err := pdf.WriteTo(failingWriter{}, para("Words."), pdf.Options{}); err == nil {
		t.Error("writing nowhere reported success")
	}
}

// failingWriter is a place to write that will not have it.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("no") }

var _ = engine.Options{}
var _ io.Writer = failingWriter{}
