// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"fmt"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestSecondLineUnderlineIsEscaped is an INJECTION case: a paragraph whose
// second line is a uniform run of punctuation was written out verbatim, and
// docutils' Text state reads exactly one line past the first before it
// commits — so the paragraph came back as a <title> with a "Title underline
// too short." message. Text became document structure.
//
// Found by extending injectprobe, not by reading the writer: every payload it
// had was a single line placed at the START of a text node, so it could never
// reach a paragraph's second line. A payload set inherits the shape of the
// writer it was written against.
func TestSecondLineUnderlineIsEscaped(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"four dashes become a title", "first line\n----"},
		{"four equals", "first line\n===="},
		{"two dashes are only a diagnostic, and still not the author's", "first line\n--"},
		{"three tildes", "first line\n~~~"},
		{"indented, inside the same paragraph", "first line\n  ----"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d1 := richdoc.New().P(richdoc.Txt(c.text)).Doc()
			out, err := Write(d1)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			d2, err := Parse(out)
			if err != nil {
				t.Fatalf("re-Parse: %v", err)
			}
			if len(d2.Blocks) != 1 {
				t.Fatalf("one paragraph came back as %d blocks (%s) — written as:\n%s", len(d2.Blocks), kindsOf(d2.Blocks), out)
			}
			p, ok := d2.Blocks[0].(richdoc.Paragraph)
			if !ok {
				t.Fatalf("came back as a %T, not a Paragraph — written as:\n%s", d2.Blocks[0], out)
			}
			if got := plainTextOf(p.Inlines); got != c.text {
				t.Errorf("text %q -> %q — written as:\n%s", c.text, got, out)
			}
		})
	}
}

// TestThirdLineUnderlineIsNotEscaped is the control, and it is the half that
// keeps the rule narrow. docutils gives that meaning to ONE line only: a
// uniform run on the third line or later is ordinary text, measured against
// the reference ("a\nb\n----" is a plain paragraph there). Escaping it would
// put a backslash in the document for nothing — and escaping every line is
// what corrupted inline literals the first time this was tried.
func TestThirdLineUnderlineIsNotEscaped(t *testing.T) {
	const text = "first line\nsecond line\n----"
	out, err := Write(richdoc.New().P(richdoc.Txt(text)).Doc())
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if strings.Contains(string(out), `\----`) {
		t.Errorf("the third line was escaped, which reST does not need:\n%s", out)
	}
	d2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	p, ok := d2.Blocks[0].(richdoc.Paragraph)
	if !ok {
		t.Fatalf("came back as a %T:\n%s", d2.Blocks[0], out)
	}
	if got := plainTextOf(p.Inlines); got != text {
		t.Errorf("text %q -> %q:\n%s", text, got, out)
	}
}

func kindsOf(bs []richdoc.Block) string {
	s := make([]string, 0, len(bs))
	for _, b := range bs {
		s = append(s, strings.TrimPrefix(fmtType(b), "richdoc."))
	}
	return strings.Join(s, " ")
}

func fmtType(v any) string { return fmt.Sprintf("%T", v) }
