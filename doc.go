// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package rst converts between reStructuredText and the neutral
// [github.com/go-richdoc/richdoc] document model.
//
// [Parse] reads reST source into a [richdoc.Document]; [Write] emits reST
// text from a [richdoc.Document]. The two are designed as a faithful
// round-trip: Parse(Write(Parse(src))) reproduces Parse(src)'s tree for the
// supported subset.
//
// Unlike [github.com/go-richdoc/latex], which ships its own LaTeX-subset
// parser because go-tex/engine exposes no reusable parse tree, this package
// parses by delegating to [github.com/go-docutils/docutils/rst] — a
// full, independently maintained reST engine — and walks its doctree. That
// engine is also used, indirectly, as this package's own correctness check:
// [Write]'s output is proven valid reST by feeding it back through
// docutils/rst.Parse and confirming it survives another round-trip, no
// separate reference tool needed.
//
// Constructs richdoc has no native node for (reST directives, field lists,
// definition lists, line blocks, sub/superscript roles, and any comment) are
// preserved verbatim through [richdoc.RawBlock] / [richdoc.RawInline] with
// Format "rst", so nothing in the source is silently lost.
//
// The package is pure Go and builds with CGO disabled.
package rst
