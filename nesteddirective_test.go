package rst

import (
	"strings"
	"testing"
)

// TestANestedDirectiveSurvives pins a construct that vanished WITHOUT A TRACE.
//
// rawChildSource -- the fallback that rebuilds reST for the child of a block this
// converter cannot map natively -- had no case for a <directive>, the element a
// directive this parser has no implementation for comes back as. It fell through to
// AsText, and a bare ".. versionadded:: 1.8" has NO text at all, so it disappeared
// completely; one with content kept the content and lost its marker line.
//
// 95 directives in 9 files, 77 of them in sphinx's own latex.rst, which nests them
// in definitions throughout. The same shape as the figure and the image in the
// round before, which is why there is now a probe (tagprobe) that counts every tag
// the reconstruction loses rather than finding them one at a time.
func TestANestedDirectiveSurvives(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		{
			"a bare directive in a definition",
			"term\n   Body text.\n\n   .. versionadded:: 0.4\n",
			[]string{".. versionadded:: 0.4", "Body text."},
		},
		{
			"a directive with content in a container",
			".. container:: c\n\n   .. deprecated:: 1.0\n\n      Use the other one.\n",
			[]string{".. deprecated:: 1.0", "Use the other one."},
		},
		{
			"a directive in an admonition",
			".. note::\n\n   .. versionadded:: 2.1\n",
			[]string{".. versionadded:: 2.1"},
		},
		{
			"a directive in a list item",
			"- item\n\n  .. versionchanged:: 3.0\n",
			[]string{".. versionchanged:: 3.0"},
		},
	}
	for _, c := range cases {
		out, msgs := reconstruct(t, c.src)
		if before := countSystemMessages(t, []byte(c.src)); msgs > before {
			t.Errorf("%s: the reconstruction gained %d diagnostic(s):\n%s", c.name, msgs-before, out)
		}
		for _, want := range c.want {
			if !strings.Contains(out, want) {
				t.Errorf("%s: want %q in:\n%s", c.name, want, out)
			}
		}
	}
}

// TestADirectivesOptionsStayOptions pins the spelling of the body, which the
// reference settles. An unimplemented directive keeps its whole block as TEXT,
// option lines included -- it has no option_spec to split them off with -- and an
// option block has to follow the marker immediately:
//
//	.. image:: a.png     ->  <image alt="x" uri="a.png">
//	   :alt: x
//
//	.. image:: a.png     ->  nothing at all
//
//	   :alt: x
//
// So the blank line this writer used to emit turned every option into content. For
// a directive with only content the two spellings are identical, which is what
// makes dropping the blank line free.
func TestADirectivesOptionsStayOptions(t *testing.T) {
	out, _ := reconstruct(t, ".. graphviz:: /x.dot\n   :class: builder\n")
	if !strings.Contains(out, ".. graphviz:: /x.dot\n   :class: builder") {
		t.Errorf("the option was cut loose from its directive:\n%q", out)
	}
}

// TestAnImplementedDirectiveIsUnchanged is the CONTROL: a directive this parser
// DOES implement never becomes a <directive> element, and must keep coming back as
// itself. It passes either way.
func TestAnImplementedDirectiveIsUnchanged(t *testing.T) {
	out, _ := reconstruct(t, ".. note::\n\n   Just a note.\n")
	if !strings.Contains(out, ".. note::") || !strings.Contains(out, "Just a note.") {
		t.Errorf("an implemented directive changed:\n%s", out)
	}
}
