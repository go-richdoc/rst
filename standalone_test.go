package rst

import (
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// TestACollapseNeedsAScheme pins a link turned into plain text. The writer drops
// the embedded-link markup when a link's visible text equals its URL, because a
// bare URL is auto-recognised when read back — but only if reST recognises it.
//
// docutils reads a standalone URI as an ABSOLUTE URI (a scheme, then ":") or an
// EMAIL address, and nothing else (Inliner.patterns.uri, read directly). A bare
// host name is neither, so "`py-code.org <py-code.org>`__" came back as the words
// "py-code.org" with no link at all. PEP 770 writes exactly that.
func TestACollapseNeedsAScheme(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"a bare host name keeps its markup",
			"See `py-code.org <py-code.org>`__ here.\n",
			"`py-code.org <py-code.org>`__",
		},
		{
			"a relative path keeps it too",
			"See `docs/index.html <docs/index.html>`__ here.\n",
			"`docs/index.html <docs/index.html>`__",
		},
		{
			// CONTROL: a real URI still collapses, which is what the rule is
			// for. It passes either way.
			"an absolute URI still collapses",
			"See https://example.com/ here.\n",
			"https://example.com/",
		},
		{
			// CONTROL: and so does an email address, through its own branch.
			"an email address still collapses",
			"Write to a@b.com here.\n",
			"a@b.com",
		},
	}
	for _, c := range cases {
		out, msgs := reconstruct(t, c.src)
		if msgs != 0 {
			t.Errorf("%s: the reconstruction gained %d diagnostic(s):\n%s", c.name, msgs, out)
		}
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: want %q in:\n%s", c.name, c.want, out)
		}
	}
	// The loss this is really about: the schemeless one has to come back as a
	// LINK, not as the words it was made of.
	out, _ := reconstruct(t, "See `py-code.org <py-code.org>`__ here.\n")
	d, err := Parse([]byte(out))
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	if !strings.Contains(plainOf(d.Blocks), "py-code.org") {
		t.Fatalf("the text went missing too:\n%s", out)
	}
	if !hasLink(d.Blocks) {
		t.Errorf("the link became plain text:\n%s", out)
	}
	// CONTROL for that check: it must be able to answer "no". The same document
	// without the markup has no link, and this says so.
	bare, err := Parse([]byte("See py-code.org here.\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if hasLink(bare.Blocks) {
		t.Errorf("hasLink says yes for a document with no link, so it proves nothing")
	}
}

// plainOf and hasLink are the two small readers the cases above need.
func plainOf(blocks []richdoc.Block) string {
	var b strings.Builder
	for _, bl := range blocks {
		if p, ok := bl.(richdoc.Paragraph); ok {
			b.WriteString(plainText(p.Inlines))
		}
	}
	return b.String()
}

func hasLink(blocks []richdoc.Block) bool {
	for _, bl := range blocks {
		p, ok := bl.(richdoc.Paragraph)
		if !ok {
			continue
		}
		for _, in := range p.Inlines {
			if _, ok := in.(richdoc.Link); ok {
				return true
			}
		}
	}
	return false
}
