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

// TestEmphasisStillComesBackRoman records a defect that is NOT in this
// repository, together with the measurement that found it.
//
// ⛔ go-tex/engine's Options carry BoldFont and ItalicFont, documented as bound
// to \bf so that \textbf really bolds. This package supplies all three faces.
// They are INERT: a paragraph carrying both **strong** and *emphasis* leaves
// exactly one face in the file, the roman. See go-tex/engine#590.
//
// So this asserts what is TRUE TODAY rather than skipping. A test that skips
// asserts nothing and quietly stops being about anything; this one fails the
// day the engine starts honouring the faces, which is the day to delete it and
// assert the opposite.
func TestEmphasisStillComesBackRoman(t *testing.T) {
	faces := embedded(doc(t, "plain **bold** and *italic*.\n"))
	if len(faces) != 1 {
		t.Errorf("the engine now embeds %d faces (%v) — if it honours bold and italic, "+
			"this test has served its purpose: delete it and assert that it does",
			len(faces), faces)
	}
}
