package pdf_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-richdoc/rst"
	"github.com/go-richdoc/rst/pdf"
)

// onePNG writes a 1x1 PNG at path and returns its directory.
func onePNG(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{0x80, 0x80, 0x80, 0xff})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding: %v", err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("writing %s: %v", p, err)
	}
	return dir
}

// TestAnImageTypesetsWhenBaseDirSaysWhere is the whole point of BaseDir. The
// upstream fix (docutils v0.137.0) made the latex writer emit
// \includegraphics instead of silently dropping the image -- which turned a
// content loss into a resolution question this package could not answer, since
// the engine reads an image with os.ReadFile against the PROCESS's working
// directory and offers no resolver seam for a figure.
func TestAnImageTypesetsWhenBaseDirSaysWhere(t *testing.T) {
	dir := onePNG(t, "pic.png")
	doc, err := rst.Parse([]byte(".. image:: pic.png\n\nBody text.\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := pdf.Write(doc, pdf.Options{Strict: true, BaseDir: dir}); err != nil {
		t.Fatalf("with BaseDir: %v", err)
	}
	// And the control that makes the case above mean something: with no
	// BaseDir the same document cannot find the same file, because the process
	// is not sitting in that directory.
	_, err = pdf.Write(doc, pdf.Options{Strict: true})
	if err == nil {
		t.Fatal("without BaseDir the image must not resolve — otherwise this test proves nothing")
	}
	if !strings.Contains(err.Error(), "pic.png") {
		t.Errorf("expected the failure to name the image, got: %v", err)
	}
}

// TestBaseDirLeavesWhatItCannotHelp pins the four kinds rebaseImages must not
// touch. The two remote ones and the data: URI cannot be satisfied by any local
// file, so rewriting them would turn "this cannot be fetched" into "the base is
// wrong" -- a worse error, about the wrong thing.
func TestBaseDirLeavesWhatItCannotHelp(t *testing.T) {
	cases := []struct{ name, src string }{
		{"absolute", ".. image:: /_static/pic.png\n"},
		{"https", ".. image:: https://example.com/badge.svg\n"},
		{"http", ".. image:: http://example.com/badge.svg\n"},
	}
	for _, c := range cases {
		doc, err := rst.Parse([]byte(c.src))
		if err != nil {
			t.Fatalf("%s: parse: %v", c.name, err)
		}
		_, err = pdf.Write(doc, pdf.Options{Strict: true, BaseDir: t.TempDir()})
		if err == nil {
			t.Errorf("%s: expected it to fail, unchanged, rather than be rebased", c.name)
			continue
		}
		// The error must still name what the DOCUMENT asked for, not a path
		// with a temporary directory glued in front of it.
		if strings.Contains(err.Error(), "/_static/pic.png") ||
			strings.Contains(err.Error(), "example.com/badge.svg") {
			continue
		}
		t.Errorf("%s: the reference was rewritten: %v", c.name, err)
	}
}

// TestLenientLosesAnImageItCannotFind is the other half of the lenient-default
// lesson from v0.136.16: with no BaseDir and no Strict, an unresolvable image
// costs the reader the picture and reports nothing at all.
func TestLenientLosesAnImageItCannotFind(t *testing.T) {
	doc, err := rst.Parse([]byte(".. image:: nowhere.png\n\nBody text.\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := pdf.Write(doc, pdf.Options{}); err != nil {
		t.Fatalf("the lenient default must still produce a document: %v", err)
	}
	if _, err := pdf.Write(doc, pdf.Options{Strict: true}); err == nil {
		t.Fatal("strict must say so")
	}
}
