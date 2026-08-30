// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

//go:build !js

package pdf

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
	"github.com/go-tex/engine"
)

// A document going out through two steps can fail at either, and a caller
// has to be told which. Neither failure can be produced from outside — the
// reST/doctree/LaTeX chain inside toLaTeX works on whatever it is given and
// the engine typesets an ordinary paragraph — so they are reached through
// the seams.
func TestWhenAStepRefuses(t *testing.T) {
	doc := richdoc.New().P(richdoc.Text{Value: "words"}).Doc()

	t.Run("writing the reST", func(t *testing.T) {
		was := toLaTeX
		t.Cleanup(func() { toLaTeX = was })
		toLaTeX = func(*richdoc.Document) ([]byte, error) { return nil, errors.New("no") }
		_, err := Write(doc, Options{})
		if err == nil {
			t.Fatal("a refusal came back as a PDF")
		}
		if !strings.Contains(err.Error(), "as reST") {
			t.Errorf("it said %q", err)
		}
	})

	t.Run("typesetting it", func(t *testing.T) {
		was := compile
		t.Cleanup(func() { compile = was })
		compile = func([]byte, engine.Options, io.Writer) (int, error) { return 0, errors.New("no") }
		_, err := Write(doc, Options{})
		if err == nil {
			t.Fatal("a refusal came back as a PDF")
		}
		if !strings.Contains(err.Error(), "typesetting it") {
			t.Errorf("it said %q", err)
		}
	})
}

func TestStrictAndLenient(t *testing.T) {
	// The default is lenient, which is what a document arriving from
	// another format wants: a gap in the engine should cost a paragraph
	// rather than the document. A caller writing its own reST asks for
	// strict.
	doc := richdoc.New().P(richdoc.Text{Value: "words"}).Doc()
	var got engine.Options
	was := compile
	t.Cleanup(func() { compile = was })
	compile = func(src []byte, opt engine.Options, w io.Writer) (int, error) {
		got = opt
		return was(src, opt, w)
	}
	if _, err := Write(doc, Options{}); err != nil {
		t.Fatal(err)
	}
	if !got.Lenient {
		t.Error("the default was not lenient")
	}
	if _, err := Write(doc, Options{Strict: true}); err != nil {
		t.Fatal(err)
	}
	if got.Lenient {
		t.Error("Strict did not turn leniency off")
	}
}
