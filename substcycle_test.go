// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestSubstitutionCycleDoesNotCrash covers a DENIAL OF SERVICE, and twenty-one
// bytes are enough for it:
//
//	.. |a| replace:: |a|
//
//	see |a| here
//
// A substitution's definition may reference another substitution, so resolving
// one is recursive; a definition that reaches ITSELF recursed forever and Go
// answered "fatal error: stack overflow". That is not a panic — no recover()
// catches it — so a service converting a document it did not write could be
// killed by it and could not defend itself.
//
// ⚠ With the guard removed this case does not FAIL, it kills the test binary:
// `go test` reports the package as failed with "fatal error: stack overflow"
// rather than printing a failed assertion. That is still an observable
// control, and it is worth knowing which shape it takes before running it.
//
// The reference detects the circularity and leaves the marker as text. The
// identical hazard one function below — a footnote citing itself — was found
// and guarded in an earlier round; this one was not looked at then.
func TestSubstitutionCycleDoesNotCrash(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"a substitution referencing itself", ".. |a| replace:: |a|\n\nsee |a| here\n"},
		{"a mutual pair", ".. |a| replace:: |b|\n.. |b| replace:: |a|\n\nsee |a| here\n"},
		{"a three-step ring", ".. |a| replace:: |b|\n.. |b| replace:: |c|\n.. |c| replace:: |a|\n\nsee |a| here\n"},
		{"a cycle reached through a second reference", ".. |a| replace:: x |b|\n.. |b| replace:: |a| y\n\nsee |a| here\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			doc, err := Parse([]byte(c.src))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			text := richdoc.PlainText(doc)
			if !strings.Contains(text, "see") || !strings.Contains(text, "here") {
				t.Errorf("the surrounding text did not survive: %q", text)
			}
			// And the output is BOUNDED: the point of the guard is that a cycle
			// stops, so a few dozen bytes in must not become thousands out.
			if len(text) > 200 {
				t.Errorf("a cycle produced %d bytes of text from %d of source", len(text), len(c.src))
			}
			// It must still be writable, which is what a consumer does next.
			if _, err := Write(doc); err != nil {
				t.Errorf("Write: %v", err)
			}
		})
	}
}

// TestNestedSubstitutionStillExpands is the control: the guard must stop a
// CYCLE and nothing else. A definition that references another one, with no
// ring, expands exactly as the reference expands it — asked for
// ".. |b| replace:: |a| and |a|" over ".. |a| replace:: x", docutils answers
// "see x and x here".
func TestNestedSubstitutionStillExpands(t *testing.T) {
	const src = ".. |a| replace:: x\n.. |b| replace:: |a| and |a|\n\nsee |b| here\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := richdoc.PlainText(doc), "see x and x here"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

// TestSubstitutionUsedTwiceIsNotACycle is the other half of the control: the
// guard is keyed on a name being expanded ON THE CURRENT PATH, not on having
// been expanded before, so the same substitution used twice in one paragraph
// must expand twice.
func TestSubstitutionUsedTwiceIsNotACycle(t *testing.T) {
	const src = ".. |a| replace:: x\n\nsee |a| and |a| here\n"
	doc, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := richdoc.PlainText(doc), "see x and x here"; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}
