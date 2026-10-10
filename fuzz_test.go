// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"strings"
	"testing"

	docrst "github.com/go-docutils/docutils/rst"
	"github.com/go-richdoc/richdoc"
)

// FuzzParseWrite drives the pair the way a caller does -- reST in, tree,
// reST out, tree again -- and asserts the three properties that make that
// pipeline safe to point at a document nobody vetted:
//
//   - Write does not panic on anything Parse accepted. The writer computes a
//     table's geometry from the spans the parser found, so an input that makes
//     those disagree with the cells reaches it as an index, not as data.
//   - what Write emits, Parse reads. A writer that emits reST its own parser
//     rejects has turned a document into an error, which is worse than losing
//     a construct.
//   - a grid table's lines all have the same width. A table that re-parses
//     into something else is a fidelity defect and the corpus measures those;
//     a table whose frame is RAGGED is a corrupt document, and no round-trip
//     check can see it, because the reconstruction of a broken frame is a
//     line block that parses perfectly well.
//
// That last one is checked by writing each table the tree holds on its OWN,
// never by looking for frames in the finished document. The first version did
// look, and the fuzzer broke it in one second with "+++\n++-+" -- a
// PARAGRAPH whose two lines each read as a rule. Nothing in the text says
// which lines are a table; the tree does.
//
// Seeded with docutils' own GridTableParser docstring example and with the
// two real-world tables that drove the row-span work: PEP 393's (a row span
// beside a column span) and PEP 669's (malformed, "+" where a "|" belongs).
func FuzzParseWrite(f *testing.F) {
	f.Add(gridSpanSource)
	f.Add("+-------+-------------------+\n|string | Python 3.2        |\n|size   +--------+----------+\n|       | 16-bit | 32-bit   |\n+-------+--------+----------+\n|1      | 32     | 64       |\n+-------+--------+----------+\n")
	f.Add("+----+----+\n|         |\n+----+----+\n| a  | b  |\n+====+====+\n| c  | d  |\n+----+----+\n+ e  | f  |\n+----+----+\n")
	f.Add("=====  =====\na      b\n=====  =====\n1      2\n=====  =====\n")
	f.Add("a title\n=======\n\n- a list\n\n  | a line block\n  | another\n\n.. note:: a directive\n")
	f.Fuzz(func(t *testing.T, src string) {
		doc, err := Parse([]byte(src))
		if err != nil || doc == nil {
			return
		}
		out, err := Write(doc)
		if err != nil {
			t.Fatalf("Write: %v\ninput:\n%s", err, src)
		}
		if _, err := Parse(out); err != nil {
			t.Fatalf("re-Parse of our own output: %v\ninput:\n%s\noutput:\n%s", err, src, out)
		}
		for _, tbl := range allTables(doc.Blocks) {
			checkGridFrame(t, tbl)
		}
	})
}

// checkGridFrame writes one table as a whole document, where every framed
// line is known to BE the frame, and requires them all to be the same display
// width. A caption indents the grid under a ".. table::" directive, so the
// comparison is of left-trimmed lines.
func checkGridFrame(t *testing.T, tbl richdoc.Table) {
	t.Helper()
	out, err := Write(&richdoc.Document{Blocks: []richdoc.Block{tbl}})
	if err != nil {
		t.Fatalf("Write of one table: %v", err)
	}
	width, first := -1, ""
	for _, l := range strings.Split(string(out), "\n") {
		l = strings.TrimLeft(l, " ")
		if !strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "|") {
			continue
		}
		w := docrst.TableColumnWidth(l)
		if width < 0 {
			width, first = w, l
			continue
		}
		if w != width {
			t.Fatalf("frame line %q is %d columns wide, %q is %d:\n%s", l, w, first, width, out)
		}
	}
}

// allTables collects every table in the tree, a cell's own blocks included: a
// grid table can hold one.
func allTables(bs []richdoc.Block) []richdoc.Table {
	var out []richdoc.Table
	for _, b := range bs {
		switch v := b.(type) {
		case richdoc.Table:
			out = append(out, v)
			for _, row := range append([][]richdoc.Cell{v.Header}, v.Rows...) {
				for _, c := range row {
					out = append(out, allTables(c.Blocks)...)
				}
			}
		case richdoc.BlockQuote:
			out = append(out, allTables(v.Blocks)...)
		case richdoc.List:
			for _, it := range v.Items {
				out = append(out, allTables(it.Blocks)...)
			}
		}
	}
	return out
}
