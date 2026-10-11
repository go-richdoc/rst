// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

// A blank line does not end every reST construct, so two adjacent blocks can
// come back as one. Each pair below was asked of the reference, and the
// separator reST answers with is an EMPTY COMMENT.
//
// The witness is pytest's own skipping guide, a document nobody wrote by hand:
// its "1." items carry continuations indented two spaces where the enumerator
// is three wide, so reST reads the code blocks as quotes OUTSIDE the list.
// Written back at the ordinary quote indent they became the items' own
// content, and a 1-item list came back as a 3-item one.
func TestAdjacentBlocksDoNotMerge(t *testing.T) {
	quote := func(text string) richdoc.BlockQuote {
		return richdoc.BlockQuote{Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt(text)}}}}
	}
	ulist := func(text string) richdoc.List {
		return richdoc.List{Tight: true, Start: 1, Items: []richdoc.ListItem{{Blocks: []richdoc.Block{
			richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt(text)}},
		}}}}
	}
	olist := func(start int, text string) richdoc.List {
		l := ulist(text)
		l.Ordered = true
		l.Start = start
		return l
	}
	cases := []struct {
		name   string
		blocks []richdoc.Block
	}{
		{"a quote after a list is not the last item's content", []richdoc.Block{ulist("item"), quote("quoted")}},
		{"two quotes are not one quote with two paragraphs", []richdoc.Block{quote("one"), quote("two")}},
		{"two bullet lists are not one list with two items", []richdoc.Block{ulist("a"), ulist("b")}},
		{"an ordered list continuing the previous one", []richdoc.Block{olist(1, "a"), olist(2, "b")}},
		{"a quote after an ordered list", []richdoc.Block{olist(1, "a"), quote("quoted")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d1 := &richdoc.Document{Blocks: c.blocks}
			out, err := Write(d1)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			d2, err := Parse(out)
			if err != nil {
				t.Fatalf("re-Parse: %v", err)
			}
			if len(d2.Blocks) != len(c.blocks) {
				t.Fatalf("%d blocks came back as %d (%s) — written as:\n%s",
					len(c.blocks), len(d2.Blocks), kindsOf(d2.Blocks), out)
			}
			for i := range c.blocks {
				if fmtType(c.blocks[i]) != fmtType(d2.Blocks[i]) {
					t.Errorf("block %d: %s -> %s — written as:\n%s", i, fmtType(c.blocks[i]), fmtType(d2.Blocks[i]), out)
				}
			}
		})
	}
}

// TestNoSeparatorWhereNoneIsNeeded is the control, and it is what keeps the
// rule from becoming "punctuate everything": an empty comment in the output is
// a node in the OUTPUT's doctree that the source did not have, so it is only
// worth emitting where two blocks would actually merge.
//
// An ordered list whose successor does NOT continue its numbering is the case
// the reference draws the line at: asked for "1. a" then "1. b" it answers two
// <enumerated_list>s, because an enumerator that does not continue starts a
// new list.
func TestNoSeparatorWhereNoneIsNeeded(t *testing.T) {
	para := func(text string) richdoc.Paragraph {
		return richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt(text)}}
	}
	olist := func(start int, text string) richdoc.List {
		return richdoc.List{Ordered: true, Tight: true, Start: start, Items: []richdoc.ListItem{{Blocks: []richdoc.Block{para(text)}}}}
	}
	ulist := richdoc.List{Tight: true, Start: 1, Items: []richdoc.ListItem{{Blocks: []richdoc.Block{para("b")}}}}
	cases := []struct {
		name   string
		blocks []richdoc.Block
	}{
		{"two ordered lists that do not continue", []richdoc.Block{olist(1, "a"), olist(1, "b")}},
		{"a bullet list after an ordered one", []richdoc.Block{olist(1, "a"), ulist}},
		{"a list after a quote", []richdoc.Block{richdoc.BlockQuote{Blocks: []richdoc.Block{para("quoted")}}, ulist}},
		{"two paragraphs", []richdoc.Block{para("one"), para("two")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := Write(&richdoc.Document{Blocks: c.blocks})
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			for _, l := range strings.Split(string(out), "\n") {
				if strings.TrimSpace(l) == ".." {
					t.Errorf("separator emitted where reST needs none:\n%s", out)
				}
			}
			d2, err := Parse(out)
			if err != nil {
				t.Fatalf("re-Parse: %v", err)
			}
			if len(d2.Blocks) != len(c.blocks) {
				t.Errorf("%d blocks came back as %d (%s) — written as:\n%s",
					len(c.blocks), len(d2.Blocks), kindsOf(d2.Blocks), out)
			}
		})
	}
}
