// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package pdf turns a richdoc document into a typeset PDF, by way of real
// reStructuredText.
//
// [github.com/go-richdoc/latex]'s own pdf package goes straight from richdoc
// to LaTeX text and compiles that. This one goes one step further: the
// document goes out through the parent [github.com/go-richdoc/rst]'s Write as
// actual reST source, that source is parsed by
// [github.com/go-docutils/docutils/rst] — the reference reST engine the
// parent package already builds on for Parse — and the resulting doctree is
// rendered to LaTeX by [github.com/go-docutils/docutils/latex] before
// [github.com/go-tex/engine] compiles it. Four steps, not two, because the
// point of living here rather than in latex/pdf is proving something about
// this package's own reST specifically: that Write's output is not merely
// reST that reparses into the same tree (the parent's own round-trip check),
// but reST a real LaTeX toolchain accepts and typesets. That composition
// — reST source through docutils/rst.Parse through docutils/latex.Render
// through a real compile — was proven by hand earlier the same day this
// package was written, checking that go-tex/engine could typeset go-tex's
// own documentation once it went through docutils first; this package is
// that same chain, kept.
//
// # Why a package rather than the parent
//
// The same reason latex/pdf gives: the engine is a six-megabyte TeX
// implementation, and the parent package's own tests already name it only to
// check that Write's output typesets — importing rst on its own must not
// link it. A separate module would solve that too and cost another
// repository and another release to keep in step, for a composition this
// short.
package pdf
