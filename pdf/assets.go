// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package pdf

import (
	"path/filepath"
	"regexp"
	"strings"
)

// docutils v0.137.0 made the latex writer emit \includegraphics for an image
// instead of dropping it, which turned a silent content loss into a question
// this package had no way to answer: WHERE do the document's images live?
//
// The engine reads an image with os.ReadFile and has no resolver seam for one
// (Options.Resolve serves classes, packages and \input files, not figures), so
// a relative path resolves against the PROCESS's working directory -- which for
// a document that arrived from somewhere else is almost never the right answer.
// Measured over the 1564-file corpus, that is 43 documents that typeset before
// and fail now, every one of them "includegraphics …: no such file or
// directory".
//
// BaseDir closes it without touching the engine: each relative reference is
// rewritten to sit under the directory the caller names, before the source
// reaches the engine at all.

var reIncludegraphics = regexp.MustCompile(`(\\includegraphics(?:\[[^]]*\])?\{)([^}]*)(\})`)

// rebaseImages rewrites every RELATIVE \includegraphics path in src to sit
// under base. Four kinds are left exactly as they are, because rewriting them
// would be wrong rather than merely unhelpful:
//
//	/…        already absolute: the caller said where
//	https://  a remote reference; this package does not fetch
//	http://
//	data:     an inline data URI, which \includegraphics has no syntax for
//
// The last three cannot be satisfied by any local file, so they fail with or
// without a base — naming them here keeps a caller from reading the failure as
// a wrong base.
func rebaseImages(src []byte, base string) []byte {
	if base == "" {
		return src
	}
	return reIncludegraphics.ReplaceAllFunc(src, func(m []byte) []byte {
		g := reIncludegraphics.FindSubmatch(m)
		uri := string(g[2])
		switch {
		case uri == "",
			strings.HasPrefix(uri, "/"),
			strings.HasPrefix(uri, "http://"),
			strings.HasPrefix(uri, "https://"),
			strings.HasPrefix(uri, "data:"):
			return m
		}
		return append(append(append([]byte{}, g[1]...),
			[]byte(filepath.Join(base, uri))...), g[3]...)
	})
}
