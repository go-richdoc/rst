// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package pdf

import (
	"bytes"
	"fmt"
	"io"

	"github.com/go-docutils/docutils/latex"
	docrst "github.com/go-docutils/docutils/rst"
	"github.com/go-opentype/fonts/lora"
	"github.com/go-richdoc/richdoc"
	"github.com/go-richdoc/rst"
	"github.com/go-tex/engine"
)

// Options say how to typeset.
type Options struct {
	// Strict aborts on the first thing the engine cannot do, as TeX does.
	//
	// The default is lenient, which is what a document arriving from another
	// format wants: a gap in the engine should cost the reader a paragraph
	// rather than the whole document. A caller writing its own LaTeX and
	// wanting to be told when it is wrong asks for strict.
	Strict bool

	// BaseDir says where the document's images are.
	//
	// It exists because docutils v0.137.0 stopped dropping images: a relative
	// \includegraphics resolves against the PROCESS's working directory, and
	// the engine has no resolver seam for a figure (its Options.Resolve serves
	// classes, packages and \input files), so without this a document that
	// arrived from anywhere else cannot find its own pictures. Over the
	// 1564-file corpus that is 43 documents.
	//
	// Empty (the default) keeps the process's working directory, which is the
	// previous behaviour and is right for a caller already sitting in the
	// document's own tree. An absolute reference, a http(s) URL and a data:
	// URI are left alone (see rebaseImages): the last two cannot be satisfied
	// by any local file, base or no base.
	BaseDir string
}

// Write typesets a document and returns the PDF.
func Write(doc *richdoc.Document, opt Options) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := WriteTo(&buf, doc, opt); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteTo typesets a document into w and says how many pages it came to.
//
// The page count is worth having. A document that typesets to nothing is not
// an error anywhere in the chain — LaTeX with no body compiles — so a caller
// about to hand somebody an empty file would otherwise have no way to know.
func WriteTo(w io.Writer, doc *richdoc.Document, opt Options) (pages int, err error) {
	if doc == nil {
		return 0, fmt.Errorf("pdf: there is no document to typeset")
	}
	src, err := toLaTeX(doc)
	if err != nil {
		return 0, fmt.Errorf("pdf: writing the document as reST: %w", err)
	}
	// ⛔ The faces are NOT optional. Without them the engine falls back to its
	// maths default — STIXTwoMath — and the whole document, prose included, is
	// typeset in it: a measurement of the output showed /BaseFont
	// STIXTwoMath-Regular and nothing else. Worse, \bf and \it are bound only
	// when a bold and an italic face are supplied, so emphasis silently came
	// back roman. latex/pdf carried the same defect until v0.7.0.
	pages, err = compile(rebaseImages(src, opt.BaseDir), engine.Options{
		Lenient:    !opt.Strict,
		Font:       lora.TTF,
		BoldFont:   lora.BoldTTF,
		ItalicFont: lora.ItalicTTF,
	}, w)
	if err != nil {
		return 0, fmt.Errorf("pdf: typesetting it: %w", err)
	}
	return pages, nil
}

// The two seams. Each is a variable so a test can watch what happens when the
// step behind it refuses — which is the difference between a caller being
// handed half a file and being told.
//
// toLaTeX folds three steps (Write to reST, Parse that reST, Render the
// doctree as LaTeX) into one seam rather than three: only the first can fail
// today — docutils/rst.Parse and docutils/latex.Render both work on whatever
// tree they are given and return no error — so splitting the other two out
// would add seams nothing can walk through, the same restraint latex/pdf
// applies to its own single writeLaTeX step.
var (
	toLaTeX = func(doc *richdoc.Document) ([]byte, error) {
		src, err := rst.Write(doc)
		if err != nil {
			return nil, err
		}
		return []byte(latex.Render(docrst.Parse(string(src)))), nil
	}
	compile = engine.CompileToPDF
)
