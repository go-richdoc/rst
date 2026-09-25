// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-docutils/docutils/doctree"
)

// TestFieldsToMetaFallback exercises leadingMeta's plain-field_list branch
// directly rather than through Parse: docutils/rst v0.12.0+ always
// promotes a non-empty leading field list to <docinfo> (see docinfo.go
// there), so a bare <field_list> can no longer reach here through real
// parsing — this is a defensive fallback, not dead code, the same
// category as tableGroupChildren's own pre-tgroup fallback (parse.go).
func TestFieldsToMetaFallback(t *testing.T) {
	fl := doctree.NewElement(doctree.TagFieldList,
		doctree.NewElement(doctree.TagField,
			doctree.NewElement(doctree.TagFieldName, &doctree.Text{Data: "title"}),
			doctree.NewElement(doctree.TagFieldBody, doctree.NewElement(doctree.TagParagraph, &doctree.Text{Data: "Hello"})),
		),
	)
	meta, rest := leadingMeta([]doctree.Node{fl, doctree.NewElement(doctree.TagParagraph, &doctree.Text{Data: "Body."})})
	want := map[string]string{"title": "Hello"}
	if !reflect.DeepEqual(meta, want) {
		t.Errorf("leadingMeta meta = %#v, want %#v", meta, want)
	}
	if len(rest) != 1 {
		t.Fatalf("leadingMeta rest = %#v, want the trailing paragraph alone", rest)
	}
}

// TestALeadingMetaDoesNotHideTheDocInfo pins what docutils/rst v0.136.5
// changed underneath this package: a ".. meta::" directive's own nodes are
// hoisted to docutils' own insertion point, which is AHEAD of a leading
// field list, and the DocInfo transform still finds that list by stepping
// over them. leadingMeta read children[0] and nothing else, so a document
// with BOTH came back with an empty Meta -- and the promoted <docinfo>,
// no longer claimed here, reached convertBlockNode, which has no case for
// one and drops it. Every bibliographic field in the document was lost
// silently, in both Meta and the blocks; nothing errored.
//
// The two CONTROLS are what make this test able to fail in the other
// direction, since "step over the meta nodes" must not become "search the
// whole document for a field list": a field list that is NOT leading still
// falls back to a RawBlock (the convention Write round-trips), and a meta
// with no field list at all still yields an empty Meta rather than
// picking up something further down.
func TestALeadingMetaDoesNotHideTheDocInfo(t *testing.T) {
	t.Run("both a leading field list and a meta", func(t *testing.T) {
		src := ":date: 2026-01-01\n:author: A\n\n.. meta::\n   :name: content\n\npara\n"
		d, err := Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		if d.Meta["date"] != "2026-01-01" || d.Meta["author"] != "A" {
			t.Errorf("Meta = %v, want date and author", d.Meta)
		}
		// The meta's own raw block must SURVIVE the step-over, not be
		// consumed with the field list.
		out, err := Write(d)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{":date: 2026-01-01", ":author: A", ".. meta::", ":name: content"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("round trip lost %q:\n%s", want, out)
			}
		}
	})
	t.Run("CONTROL: a leading field list with no meta at all", func(t *testing.T) {
		d, err := Parse([]byte(":date: 2026-01-01\n\npara\n"))
		if err != nil {
			t.Fatal(err)
		}
		if d.Meta["date"] != "2026-01-01" {
			t.Errorf("Meta = %v, want date", d.Meta)
		}
	})
	t.Run("CONTROL: a meta with no field list picks nothing up", func(t *testing.T) {
		d, err := Parse([]byte(".. meta::\n   :name: content\n\npara\n\n:date: 2026-01-01\n"))
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Meta) != 0 {
			t.Errorf("Meta = %v, want empty: the field list is not leading", d.Meta)
		}
		out, err := Write(d)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), ":date: 2026-01-01") {
			t.Errorf("round trip lost the non-leading field list:\n%s", out)
		}
	})
}
