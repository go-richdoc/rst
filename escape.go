// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import "strings"

// escapeText backslash-escapes the characters that start or end reST inline
// markup (*emphasis*, **strong**, single backtick interpreted text or double
// backtick literal, |substitution|, a trailing word_ reference), unconditionally rather than
// only where they'd actually be recognized — docutils' own adjacency rules
// for when a marker character truly triggers markup are intricate (see
// docutils/rst/inline.go's own SCOPE comment); over-escaping a character
// that wasn't actually dangerous produces a harmless "\x" in the source
// instead of silently corrupting a document where it was. The same
// conservative policy [github.com/go-richdoc/markdown]'s escapeText uses for
// CommonMark's own marker set.
func escapeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\', '*', '`', '|', '_':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
