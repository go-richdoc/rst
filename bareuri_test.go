// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestBareURIKeepsItsTail covers the writer's choice between the two spellings
// of a link whose text IS its URL. Written bare, reST ends a standalone URI
// before most trailing punctuation — so that a sentence's full stop is not
// swallowed into the link — and a URL that genuinely ends in one of those
// cannot be written bare at all.
//
// PEP 615 cites a Microsoft URL ending in "-". Written bare it came back as a
// link to the URL WITHOUT the hyphen, plus a separate text node holding it.
//
// The set was measured against the reference, every ASCII punctuation
// character in turn: only "*+/=~" survive as a final character. Both halves
// are asserted here, because a predicate that answered "no" to everything
// would fix the hyphen and lose every bare URL in the corpus.
func TestBareURIKeepsItsTail(t *testing.T) {
	bare := []string{
		"https://example.com/a",
		"https://example.com/a1",
		"https://example.com/a*",
		"https://example.com/a+",
		"https://example.com/a/",
		"https://example.com/a=",
		"https://example.com/a~",
		"mailto:a@b.example",
	}
	embedded := []string{
		"https://example.com/a-",
		"https://example.com/a.",
		"https://example.com/a)",
		"https://example.com/a_",
		"https://example.com/a?",
		"https://example.com/a#",
		"https://example.com/a%",
		// Non-ASCII refuses for a worse reason than the others: the reference
		// drops the whole PATH, not just the character.
		"https://example.com/aé",
	}
	for _, u := range bare {
		got := writeLink(richdoc.Link{URL: u, Inlines: []richdoc.Inline{richdoc.Txt(u)}})
		if got != u {
			t.Errorf("%q: written as %q, want the bare URL", u, got)
		}
	}
	for _, u := range embedded {
		got := writeLink(richdoc.Link{URL: u, Inlines: []richdoc.Inline{richdoc.Txt(u)}})
		if !strings.HasPrefix(got, "`") {
			t.Errorf("%q: written as %q, want the embedded form", u, got)
		}
	}
}

// TestTrailingHyphenURLRoundTrips is the same case end to end, which is what
// the writer exists to get right: the link has to come back as ONE link
// carrying the whole URL.
func TestTrailingHyphenURLRoundTrips(t *testing.T) {
	const url = "https://docs.example.com/intl/international-components-for-unicode--icu-"
	d1 := richdoc.New().P(richdoc.Link{URL: url, Inlines: []richdoc.Inline{richdoc.Txt(url)}}).Doc()
	out, err := Write(d1)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	d2, err := Parse(out)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	p, ok := d2.Blocks[0].(richdoc.Paragraph)
	if !ok {
		t.Fatalf("re-parsed block is a %T:\n%s", d2.Blocks[0], out)
	}
	if len(p.Inlines) != 1 {
		t.Fatalf("paragraph has %d inlines, want 1 (the hyphen broke off):\n%s\n%+v", len(p.Inlines), out, p.Inlines)
	}
	l, ok := p.Inlines[0].(richdoc.Link)
	if !ok {
		t.Fatalf("inline is a %T, not a Link:\n%s", p.Inlines[0], out)
	}
	if l.URL != url {
		t.Errorf("URL = %q, want %q:\n%s", l.URL, url, out)
	}
}
