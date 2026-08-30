// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"reflect"
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
