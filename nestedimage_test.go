package rst

import (
	"strings"
	"testing"
)

// TestANestedFigureKeepsItsImage pins a picture that vanished. rawChildSource had
// no case for a figure OR an image, so either one nested in a container, an
// admonition, a list item or a definition fell to its AsText fallback -- and an
// <image> has no text, so the picture, every option and the ":target:" link all
// disappeared, leaving at most a caption.
//
// sphinx's own index page puts three logos in a ".. container::", each a figure
// with a ":target:", and all three links went missing. 6 figures (3 with a target)
// and 3 standalone images across 4 corpus files. Four of the five cases below
// fail without the fix; the list-item one is labelled as passing either way.
func TestANestedFigureKeepsItsImage(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		{
			"a figure with a target, in a container",
			".. container:: logos\n\n   .. figure:: logo.png\n      :alt: A Logo\n      :height: 100px\n      :target: https://docs.python.org\n\n      Python\n",
			[]string{".. figure:: logo.png", ":alt: A Logo", ":height: 100px", ":target: https://docs.python.org", "Python"},
		},
		{
			"a standalone image in a container",
			".. container:: c\n\n   .. image:: plain.png\n      :width: 50%\n",
			[]string{".. image:: plain.png", ":width: 50%"},
		},
		{
			// A standalone image with a ":target:" is parsed as a <reference>
			// WRAPPING the <image>, so it reaches rawChildSource under a
			// different tag than the image cases above.
			"a targeted image in a container",
			".. container:: c\n\n   .. image:: t.png\n      :alt: Clickable\n      :target: https://example.org/\n",
			[]string{".. image:: t.png", ":alt: Clickable", ":target: https://example.org/"},
		},
		{
			// ":align:" lands on the <image> for a lone image and on the
			// <figure> for a figure, so the shared option list has to carry it
			// for the image case or it is dropped.
			"an aligned image in a container",
			".. container:: c\n\n   .. image:: r.png\n      :align: right\n",
			[]string{".. image:: r.png", ":align: right"},
		},
		{
			"a figure in an admonition",
			".. note::\n\n   .. figure:: n.png\n\n      Caption.\n",
			[]string{".. figure:: n.png", "Caption."},
		},
		{
			// This one passes either way: a list item's children are written
			// through writeBlock, not rawChildSource. It is here as a second
			// control, naming the boundary of the defect.
			"an image in a list item (passes either way)",
			"- item\n\n  .. image:: li.png\n     :alt: In a list\n",
			[]string{".. image:: li.png", ":alt: In a list"},
		},
		{
			"an image in a definition",
			"term\n   .. image:: d.png\n      :alt: In a definition\n",
			[]string{".. image:: d.png", ":alt: In a definition"},
		},
	}
	for _, c := range cases {
		out, msgs := reconstruct(t, c.src)
		if msgs != 0 {
			t.Errorf("%s: the reconstruction gained %d diagnostic(s):\n%s", c.name, msgs, out)
		}
		for _, want := range c.want {
			if !strings.Contains(out, want) {
				t.Errorf("%s: want %q in:\n%s", c.name, want, out)
			}
		}
	}
}

// TestATopLevelFigureIsUnchanged is the CONTROL: rawFigure already handled a
// figure at the top of a document, and this change must not have touched it. It
// passes either way, and it is here so the cases above cannot be read as "figures
// did not work at all".
func TestATopLevelFigureIsUnchanged(t *testing.T) {
	out, msgs := reconstruct(t, ".. figure:: top.png\n   :target: https://example.com/\n\n   Caption.\n")
	if msgs != 0 {
		t.Errorf("the reconstruction gained %d diagnostic(s):\n%s", msgs, out)
	}
	for _, want := range []string{".. figure:: top.png", ":target: https://example.com/", "Caption."} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in:\n%s", want, out)
		}
	}
}
