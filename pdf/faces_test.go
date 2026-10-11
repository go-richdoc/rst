// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build !js

package pdf_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/go-richdoc/rst"
	"github.com/go-richdoc/rst/pdf"
)

// embedded is the set of font names a PDF carries, read off its own /BaseFont
// entries with the six-letter subset tag removed.
//
// ⛔ Read off the FILE. Asserting that this package passed engine.Options with
// three faces in it proves the call, not the page. What a reader gets is what
// came back, and the only way to see that is to open it.
func embedded(b []byte) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`/BaseFont\s*/([A-Za-z0-9+_-]+)`).FindAllSubmatch(b, -1) {
		name := string(m[1])
		if i := strings.IndexByte(name, '+'); i == 6 {
			name = name[i+1:]
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func doc(t *testing.T, src string) []byte {
	t.Helper()
	d, err := rst.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parsing reST: %v", err)
	}
	out, err := pdf.Write(d, pdf.Options{})
	if err != nil {
		t.Fatalf("typesetting: %v", err)
	}
	return out
}

// TestProseIsSetInATextFaceAndNotAMathsOne is the control over the faces this
// package hands the engine.
//
// ⛔ Before it, every reST document converted here came back set in
// STIXTwoMath-Regular — a MATHS face, setting running prose, because the
// engine's default is whatever go-tex/math bundles and this package named no
// font at all. Nothing said so: the page count was right, the text was
// readable, and the file opened. latex/pdf carried the identical defect and
// fixed it in v0.7.0; this package was not looked at then, which is why a fix
// in one sibling has to be CHECKED in the others rather than assumed.
func TestProseIsSetInATextFaceAndNotAMathsOne(t *testing.T) {
	faces := embedded(doc(t, "the quick brown fox jumps over the lazy dog\n"))
	if len(faces) == 0 {
		t.Fatal("the PDF names no font at all, so this test can see nothing")
	}
	for _, f := range faces {
		if strings.Contains(strings.ToLower(f), "math") {
			t.Errorf("prose is set in %q, which is a maths face", f)
		}
	}
	if !strings.Contains(strings.ToLower(faces[0]), "lora") {
		t.Errorf("the text face is %q, and this package asks for Lora", faces[0])
	}
}

// TestItalicComesBackInTheItalicFace is the HALF of go-tex/engine#590 that
// closed, and the assertion its tripwire asked for.
//
// Until go-tex/engine v0.252.0 this package supplied all three faces and the
// engine ignored two of them: a paragraph carrying both **strong** and
// *emphasis* left exactly ONE face in the file, the roman.
// TestEmphasisStillComesBackRoman asserted that, deliberately, so that it
// would fail the day the engine started honouring them -- "the day to delete
// it and assert the opposite". That day was 2026-10-11, and the measurement
// says the opposite is only half true:
//
//	"the quick brown fox"           -> [Lora-Regular]
//	"plain **bold** and *italic*."  -> [Lora-Regular Lora-Italic]
//	"plain **bold** here."          -> [Lora-Regular]
//	"plain *italic* here."          -> [Lora-Regular Lora-Italic]
//
// So the tripwire is SPLIT rather than removed: italic is asserted here, bold
// keeps its own below. Reading /BaseFont off the finished PDF rather than
// checking that the right Options were passed -- asserting the call proves the
// call, not the page.
func TestItalicComesBackInTheItalicFace(t *testing.T) {
	faces := embedded(doc(t, "plain *italic* here.\n"))
	italic := false
	for _, f := range faces {
		if strings.Contains(strings.ToLower(f), "italic") {
			italic = true
		}
	}
	if !italic {
		t.Errorf("emphasis is set in %v, with no italic face among them", faces)
	}
}

// TestBoldStillComesBackRoman is the half that is still open, and it asserts
// what is TRUE TODAY for the same reason its predecessor did: a test that
// skips asserts nothing and quietly stops being about anything, where this one
// fails the day go-tex/engine honours BoldFont too -- which is the day to
// delete it and assert that it does.
//
// ⛔ go-tex/engine's Options carry BoldFont and ItalicFont, documented as bound
// to \bf so that \textbf really bolds. This package supplies both. As of
// engine v0.252.0 the italic one works and the bold one does not: a paragraph
// whose only markup is **bold** leaves the roman face alone in the file.
func TestBoldStillComesBackRoman(t *testing.T) {
	faces := embedded(doc(t, "plain **bold** here.\n"))
	for _, f := range faces {
		if strings.Contains(strings.ToLower(f), "bold") {
			t.Errorf("the engine now embeds a bold face (%v) \u2014 if it honours BoldFont, "+
				"this test has served its purpose: delete it and assert that it does", faces)
		}
	}
}
