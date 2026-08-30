// Copyright (c) the go-richdoc authors.
// SPDX-License-Identifier: BSD-3-Clause

package rst

import (
	"reflect"
	"strings"
	"testing"

	"github.com/go-richdoc/richdoc"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name   string
		source string
		want   *richdoc.Document
	}{
		{
			"paragraph with inline markup",
			"A *em*, **strong**, and ``code``.\n",
			richdoc.New().P(
				richdoc.Txt("A "), richdoc.Italic(richdoc.Txt("em")), richdoc.Txt(", "),
				richdoc.Bold(richdoc.Txt("strong")), richdoc.Txt(", and "),
				richdoc.Mono("code"), richdoc.Txt("."),
			).Doc(),
		},
		{
			"nested sections",
			"Top\n===\n\nIntro.\n\nSub\n---\n\nNested.\n",
			richdoc.New().
				H(1, richdoc.Txt("Top")).
				P(richdoc.Txt("Intro.")).
				H(2, richdoc.Txt("Sub")).
				P(richdoc.Txt("Nested.")).
				Doc(),
		},
		{
			"tight bullet list",
			"- a\n- b\n",
			richdoc.New().UList(true,
				richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("a")}}),
				richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("b")}}),
			).Doc(),
		},
		{
			"enumerated list always starts at 1 (docutils/rst tracks no start attribute)",
			"3. third\n4. fourth\n",
			richdoc.New().OList(1, true,
				richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("third")}}),
				richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("fourth")}}),
			).Doc(),
		},
		{
			"block quote and transition",
			"Para.\n\n    Quoted.\n\n----\n",
			richdoc.New().
				P(richdoc.Txt("Para.")).
				Quote(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("Quoted.")}}).
				HR().
				Doc(),
		},
		{
			"literal block",
			"Sample::\n\n    code here\n",
			richdoc.New().P(richdoc.Txt("Sample:")).CodeBlock("", "code here").Doc(),
		},
		{
			"reference with embedded URI resolves to a Link",
			"See `Python <https://python.org>`_ now.\n",
			richdoc.New().P(
				richdoc.Txt("See "),
				richdoc.Href("https://python.org", "", richdoc.Txt("Python")),
				richdoc.Txt(" now."),
			).Doc(),
		},
		{
			// docutils/rst v0.13.0+ rewrites a dangling reference to
			// <problematic> and appends a trailing "Docutils System
			// Messages" section (real docutils' own DanglingReferences +
			// Messages transforms, simplified) instead of leaving it a
			// bare, unresolved <reference>; neither <problematic> nor
			// <system_message> has a dedicated conversion case (see
			// parse.go's convertInlineElement/convertBlockNode), so this
			// package's own generic fallbacks handle them: problematic's
			// text passes through as plain richdoc.Text, and the
			// section becomes an ordinary richdoc.Heading + Paragraph
			// like any other section would.
			"unresolved reference becomes problematic text plus a trailing system-messages section",
			"See `nowhere`_ now.\n",
			richdoc.New().
				P(richdoc.Txt("See nowhere now.")).
				H(1, richdoc.Txt("Docutils System Messages")).
				P(richdoc.Txt(`Unknown target name: "nowhere".`)).
				Doc(),
		},
		{
			"standalone URI needs no markup at all",
			"See https://example.com now.\n",
			richdoc.New().P(
				richdoc.Txt("See "),
				richdoc.Href("https://example.com", "", richdoc.Txt("https://example.com")),
				richdoc.Txt(" now."),
			).Doc(),
		},
		{
			"footnote reference inlines its definition's body as a Footnote",
			"See [1]_ here.\n\n.. [1] The note.\n",
			richdoc.New().P(
				richdoc.Txt("See "),
				richdoc.Note(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("The note.")}}),
				richdoc.Txt(" here."),
			).Doc(),
		},
		{
			"citation reference inlines the same way as a footnote",
			"See [CIT2002]_ here.\n\n.. [CIT2002] The citation.\n",
			richdoc.New().P(
				richdoc.Txt("See "),
				richdoc.Note(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("The citation.")}}),
				richdoc.Txt(" here."),
			).Doc(),
		},
		{
			"an auto footnote reference now resolves via docutils/rst v0.7.0's own auto-numbering (synthetic name + matching refname)",
			"An auto footnote [#]_.\n\n.. [#] Text.\n",
			richdoc.New().P(
				richdoc.Txt("An auto footnote "),
				richdoc.Note(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("Text.")}}),
				richdoc.Txt("."),
			).Doc(),
		},
		{
			"an auto footnote reference with no matching definition anywhere stays unresolved",
			"An orphan auto reference [#]_.\n",
			richdoc.New().P(
				richdoc.Txt("An orphan auto reference "), richdoc.RawI("rst", "[#]_"), richdoc.Txt("."),
			).Doc(),
		},
		{
			"an orphan UNNAMED auto footnote definition reconstructs as anonymous, not with docutils' own synthetic name",
			".. [#] Never referenced.\n",
			richdoc.New().RawBlock("rst", ".. [#] Never referenced.").Doc(),
		},
		{
			"an orphan auto footnote whose REAL name merely starts with 'footnote-' is not mistaken for a synthetic one",
			".. [#footnote-abc] Never referenced.\n",
			richdoc.New().RawBlock("rst", ".. [#footnote-abc] Never referenced.").Doc(),
		},
		{
			"substitution reference resolves to its definition's content",
			"A |sub| here.\n\n.. |sub| replace:: value\n",
			richdoc.New().P(richdoc.Txt("A value here.")).Doc(),
		},
		{
			"unresolved substitution reference falls back to its bare name, without the pipe delimiters the source used",
			"A |nowhere| here.\n",
			richdoc.New().P(richdoc.Txt("A nowhere here.")).Doc(),
		},
		{
			"strike role round-trips to Strikethrough",
			"A :strike:`struck` word.\n",
			richdoc.New().P(
				richdoc.Txt("A "), richdoc.Strike(richdoc.Txt("struck")), richdoc.Txt(" word."),
			).Doc(),
		},
		{
			"math role round-trips to Math",
			"A :math:`a^2` term.\n",
			richdoc.New().P(
				richdoc.Txt("A "), richdoc.InlineMath("a^2"), richdoc.Txt(" term."),
			).Doc(),
		},
		{
			"code role maps to Code, same as a backtick literal (docutils/rst v0.3.0+ gives it a dedicated node)",
			"A :code:`x = 1` snippet.\n",
			richdoc.New().P(
				richdoc.Txt("A "), richdoc.Mono("x = 1"), richdoc.Txt(" snippet."),
			).Doc(),
		},
		{
			"unknown role falls back to RawInline preserving the role name",
			"See :custom:`text` here.\n",
			richdoc.New().P(
				richdoc.Txt("See "), richdoc.RawI("rst", ":custom:`text`"), richdoc.Txt(" here."),
			).Doc(),
		},
		{
			"title reference maps to Emph, the nearest common styling",
			"A `title` reference.\n",
			richdoc.New().P(
				richdoc.Txt("A "), richdoc.Italic(richdoc.Txt("title")), richdoc.Txt(" reference."),
			).Doc(),
		},
		{
			"leading field list becomes Document.Meta",
			":title: Hello\n:author: Ann\n\nBody.\n",
			&richdoc.Document{
				Meta:   map[string]string{"title": "Hello", "author": "Ann"},
				Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("Body.")}}},
			},
		},
		{
			// Guards a real bug this docutils/rst release surfaced:
			// docutils promotes a leading field list with a RECOGNIZED
			// bibliographic name (author, date, version, ...) to a
			// <docinfo> element instead of leaving it a plain
			// <field_list> — a shape leadingMeta didn't originally know
			// about, silently losing the whole thing (docinfo has no
			// block-level case of its own, so convertBlockNode's default
			// fallback walked into its typed children as if they were
			// blocks, where they were dropped).
			"a leading field list with recognized bibliographic names (docinfo-promoted) still becomes Document.Meta",
			":Authors: Jane Doe; John Smith\n:Version: 1.0\n\n:Dedication: To my cat.\n\nBody.\n",
			&richdoc.Document{
				Meta: map[string]string{
					"authors":    "Jane Doe; John Smith",
					"version":    "1.0",
					"dedication": "To my cat.",
				},
				Blocks: []richdoc.Block{richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("Body.")}}},
			},
		},
		{
			"simple table",
			"=====  =====\na      b\n=====  =====\n1      2\n=====  =====\n",
			richdoc.New().Table(
				[]richdoc.Alignment{richdoc.AlignDefault, richdoc.AlignDefault},
				[]richdoc.Cell{richdoc.Td(richdoc.Txt("a")), richdoc.Td(richdoc.Txt("b"))},
				[][]richdoc.Cell{{richdoc.Td(richdoc.Txt("1")), richdoc.Td(richdoc.Txt("2"))}},
			).Doc(),
		},
		{
			"comment becomes a RawBlock",
			".. a comment\n   continued\n",
			richdoc.New().RawBlock("rst", ".. a comment\n   continued").Doc(),
		},
		{
			"directive becomes a RawBlock",
			".. note::\n\n   content line\n",
			richdoc.New().RawBlock("rst", ".. note::\n\n   content line").Doc(),
		},
		{
			// Guards a real bug: <raw> (docutils/rst v0.15.0+,
			// Options.RawEnabled — see its README) has a bare Text
			// child, not an Element one, so before this had its own
			// case it fell through to the generic block walker, which
			// has no notion of a block-level bare Text node and
			// silently dropped it — RawBlock with Format "html" here
			// (not "rst": this is genuine target-format content
			// docutils itself already tagged, not this package's own
			// reST resynthesis) is the fix.
			"a raw directive becomes a RawBlock tagged with its real target format, not rst",
			".. raw:: html\n\n   <b>bold</b>\n",
			richdoc.New().RawBlock("html", "<b>bold</b>").Doc(),
		},
		{
			// Guards a real bug: <raw> (docutils/rst v0.16.0+'s inline
			// role form) had no convertInlineElement case, so it fell
			// through to the generic inline-text walk and flattened
			// "<b>x</b>" into what looked like ordinary prose the author
			// typed — losing the "this is raw, not text" distinction
			// entirely, not just formatting. The leading empty RawBlock
			// is the ".. role::" registration itself — invisible
			// bookkeeping, but still a real (if content-free) comment
			// node, same as any other comment this converter preserves.
			"an inline raw role becomes a RawInline, its markup not flattened into plain text",
			".. role:: myraw(raw)\n   :format: html\n\nSee :myraw:`<b>x</b>` here.\n",
			richdoc.New().RawBlock("rst", "..").P(richdoc.Txt("See "), richdoc.RawI("html", "<b>x</b>"), richdoc.Txt(" here.")).Doc(),
		},
		{
			"a non-leading field list becomes a RawBlock, unlike the leading one that becomes Meta",
			"Para.\n\n:key: value\n",
			richdoc.New().P(richdoc.Txt("Para.")).RawBlock("rst", ":key: value").Doc(),
		},
		{
			"definition list becomes a RawBlock",
			"Term\n    Definition body.\n",
			richdoc.New().RawBlock("rst", "Term\n    Definition body.").Doc(),
		},
		{
			"line block becomes a RawBlock",
			"| line one\n| line two\n",
			richdoc.New().RawBlock("rst", "| line one\n| line two").Doc(),
		},
		{
			// Guards a real bug: rawLineBlock used to only walk <line>
			// children, so once docutils/rst v0.11.0 started nesting an
			// indented sub-line into its own <line_block> (see
			// go-docutils-org.md), that nested content was silently
			// dropped rather than reconstructed.
			"line block with an indented sub-line preserves the nested content, not just the top-level lines",
			"| top\n|   nested\n| top again\n",
			richdoc.New().RawBlock("rst", "| top\n|   nested\n| top again").Doc(),
		},
		{
			"option list becomes a RawBlock, its flags preserved rather than silently dropped",
			"-f, --file=FILE  Grouped short+long.\n-ovalue       Embedded.\n",
			richdoc.New().RawBlock("rst", "-f, --file=FILE  Grouped short+long.\n-ovalue  Embedded.").Doc(),
		},
		{
			"subscript and superscript fall back to RawInline (richdoc has no such node)",
			":sub:`x` and :sup:`y`.\n",
			richdoc.New().P(
				richdoc.RawI("rst", ":sub:`x`"), richdoc.Txt(" and "),
				richdoc.RawI("rst", ":sup:`y`"), richdoc.Txt("."),
			).Doc(),
		},
		{
			"abbreviation flattens to plain text, the marking dropped",
			":ab:`WHO`\n",
			richdoc.New().P(richdoc.Txt("WHO")).Doc(),
		},
		{
			"a substitution definition's multi-line directive body is read from its children, not the arguments attribute",
			"A |sub| here.\n\n.. |sub| replace::\n\n   multi body\n",
			richdoc.New().P(richdoc.Txt("A multi body here.")).Doc(),
		},
		{
			"orphan symbol-auto footnote definition preserved as a RawBlock",
			".. [*] Symbol orphan.\n",
			richdoc.New().RawBlock("rst", ".. [*] Symbol orphan.").Doc(),
		},
		{
			"orphan explicit-numbered footnote definition preserved as a RawBlock",
			".. [1] Digit orphan.\n",
			richdoc.New().RawBlock("rst", ".. [1] Digit orphan.").Doc(),
		},
		{
			"orphan citation definition preserved as a RawBlock",
			".. [CIT2002] Citation orphan.\n",
			richdoc.New().RawBlock("rst", ".. [CIT2002] Citation orphan.").Doc(),
		},
		{
			"orphan named-auto footnote definition preserved as a RawBlock",
			".. [#named] Named auto orphan.\n",
			richdoc.New().RawBlock("rst", ".. [#named] Named auto orphan.").Doc(),
		},
		{
			"an empty comment becomes a bare RawBlock",
			".. \n",
			richdoc.New().RawBlock("rst", "..").Doc(),
		},
		{
			"an argument-less, body-less directive becomes a bare header RawBlock",
			".. note::\n",
			richdoc.New().RawBlock("rst", ".. note::").Doc(),
		},
		{
			"an empty field body still emits its marker",
			"Para.\n\n:flag:\n",
			richdoc.New().P(richdoc.Txt("Para.")).RawBlock("rst", ":flag:").Doc(),
		},
		{
			"an explicit numbered reference this package can't resolve preserves its marker as RawInline",
			"See [5]_ nowhere.\n",
			richdoc.New().P(
				richdoc.Txt("See "), richdoc.RawI("rst", "[5]_"), richdoc.Txt(" nowhere."),
			).Doc(),
		},
		{
			"an unresolvable symbol auto footnote reference preserves its marker as RawInline",
			"See [*]_ nowhere.\n",
			richdoc.New().P(
				richdoc.Txt("See "), richdoc.RawI("rst", "[*]_"), richdoc.Txt(" nowhere."),
			).Doc(),
		},
		{
			"an unresolvable named auto footnote reference preserves its marker as RawInline",
			"See [#x]_ nowhere.\n",
			richdoc.New().P(
				richdoc.Txt("See "), richdoc.RawI("rst", "[#x]_"), richdoc.Txt(" nowhere."),
			).Doc(),
		},
		{
			"a directive with arguments but no body reconstructs a one-line RawBlock",
			".. image:: pic.png\n",
			richdoc.New().RawBlock("rst", ".. image:: pic.png").Doc(),
		},
		{
			"an orphan footnote with no body text still emits its bracket alone",
			".. [*]\n",
			richdoc.New().RawBlock("rst", ".. [*]").Doc(),
		},
		{
			"deeply nested sections clamp their heading level at 6",
			"1\n=\n\n2\n-\n\n3\n~\n\n4\n\"\"\"\n\n5\n^^^\n\n6\n___\n\n7\n:::\n",
			richdoc.New().
				H(1, richdoc.Txt("1")).H(2, richdoc.Txt("2")).H(3, richdoc.Txt("3")).
				H(4, richdoc.Txt("4")).H(5, richdoc.Txt("5")).H(6, richdoc.Txt("6")).
				H(6, richdoc.Txt("7")).
				Doc(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse([]byte(tc.source))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Parse(%q) =\n%#v\nwant:\n%#v", tc.source, got, tc.want)
			}
		})
	}
}

func TestWriteContains(t *testing.T) {
	cases := []struct {
		name string
		doc  *richdoc.Document
		want []string
	}{
		{
			"inline markup",
			richdoc.New().P(
				richdoc.Italic(richdoc.Txt("em")), richdoc.Txt(" "),
				richdoc.Bold(richdoc.Txt("strong")), richdoc.Txt(" "),
				richdoc.Mono("code"),
			).Doc(),
			[]string{"*em*", "**strong**", "``code``"},
		},
		{
			"heading underline matches title width",
			richdoc.New().H(1, richdoc.Txt("Hello")).Doc(),
			[]string{"Hello\n====="},
		},
		{
			"bullet and enumerated lists",
			richdoc.New().
				UList(true,
					richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("a")}}),
				).
				OList(3, true,
					richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("c")}}),
				).
				Doc(),
			[]string{"- a", "3. c"},
		},
		{
			"code block as literal block",
			richdoc.New().CodeBlock("", "x := 1").Doc(),
			[]string{"::", "x := 1"},
		},
		{
			"link with distinct text",
			richdoc.New().P(richdoc.Href("https://x.io", "", richdoc.Txt("site"))).Doc(),
			[]string{"`site <https://x.io>`_"},
		},
		{
			"bare-URL link emits no embedded-link markup",
			richdoc.New().P(richdoc.Href("https://x.io", "", richdoc.Txt("https://x.io"))).Doc(),
			[]string{"https://x.io"},
		},
		{
			"footnote emits an inline reference and a trailing definition",
			richdoc.New().P(
				richdoc.Txt("See it"),
				richdoc.Note(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("Body.")}}),
				richdoc.Txt("."),
			).Doc(),
			[]string{"[1]_", ".. [1]", "Body."},
		},
		{
			"document meta emits a leading field list",
			&richdoc.Document{Meta: map[string]string{"title": "Hi"}},
			[]string{":title: Hi"},
		},
		{
			"strikethrough emits the strike role",
			richdoc.New().P(richdoc.Strike(richdoc.Txt("gone"))).Doc(),
			[]string{":strike:`gone`"},
		},
		{
			"special characters are escaped",
			richdoc.New().P(richdoc.Txt("a*b `c` d|e f_")).Doc(),
			[]string{"a\\*b \\`c\\` d\\|e f\\_"},
		},
		{
			"inline math emits the math role",
			richdoc.New().P(richdoc.InlineMath("a^2")).Doc(),
			[]string{":math:`a^2`"},
		},
		{
			"a raw block/inline for this package's own format passes through verbatim; a foreign one is dropped",
			richdoc.New().
				RawBlock("rst", ".. raw block").
				RawBlock("html", "<p>dropped</p>").
				P(richdoc.RawI("rst", "*raw inline*"), richdoc.RawI("html", "<b>dropped</b>")).
				Doc(),
			[]string{".. raw block", "*raw inline*"},
		},
		{
			"an inline image degrades to its alt text",
			richdoc.New().P(richdoc.Img("pic.png", "a cat", "")).Doc(),
			[]string{"a cat"},
		},
		{
			"a hard line break renders as a literal newline",
			richdoc.New().P(richdoc.Txt("one"), richdoc.Br(), richdoc.Txt("two")).Doc(),
			[]string{"one\ntwo"},
		},
		{
			"an anchor with visible text emits reST's inline internal target, which docutils/rst v0.4.0+ reads back",
			richdoc.New().P(richdoc.Txt("see "), richdoc.Mark("pt", richdoc.Txt("here"))).Doc(),
			[]string{"see _`here`"},
		},
		{
			"a point anchor (no visible text) has no reST equivalent and renders to nothing",
			richdoc.New().P(richdoc.Txt("see "), richdoc.Mark("pt"), richdoc.Txt("here")).Doc(),
			[]string{"see here"},
		},
		{
			"a label cross-reference emits this package's own embedded-alias convention",
			richdoc.New().P(richdoc.Txt("see "), richdoc.Ref("sec-intro", richdoc.Txt("the intro"))).Doc(),
			[]string{"`the intro <sec-intro_>`_"},
		},
		{
			"a citation cross-reference emits a bare citation-reference marker",
			richdoc.New().P(richdoc.Txt("see "), richdoc.Cite("knuth1984")).Doc(),
			[]string{"[knuth1984]_"},
		},
		{
			"an empty document writes to nothing",
			richdoc.New().Doc(),
			nil,
		},
		{
			"a nil document writes to nothing",
			nil,
			nil,
		},
		{
			"an empty table writes to nothing",
			richdoc.New().Table(nil, nil, nil).Doc(),
			nil,
		},
		{
			"a heading with no inline content still gets a one-character underline",
			&richdoc.Document{Blocks: []richdoc.Block{richdoc.Heading{Level: 1}}},
			[]string{"\n="},
		},
		{
			"a heading id emits a leading hyperlink target",
			&richdoc.Document{Blocks: []richdoc.Block{
				richdoc.Heading{Level: 1, ID: "sec-intro", Inlines: []richdoc.Inline{richdoc.Txt("Intro")}},
			}},
			[]string{".. _sec-intro:", "Intro"},
		},
		{
			"a label cross-reference with no visible text falls back to its target",
			richdoc.New().P(richdoc.Ref("sec-intro")).Doc(),
			[]string{"`sec-intro <sec-intro_>`_"},
		},
		{
			"a display math block emits the math directive",
			richdoc.New().MathBlock("a^2").Doc(),
			[]string{".. math::", "a^2"},
		},
		{
			"a heading level below 1 clamps to the top level",
			&richdoc.Document{Blocks: []richdoc.Block{richdoc.Heading{Level: 0, Inlines: []richdoc.Inline{richdoc.Txt("T")}}}},
			[]string{"T\n="},
		},
		{
			"a heading level past 6 reuses the deepest underline character",
			&richdoc.Document{Blocks: []richdoc.Block{richdoc.Heading{Level: 9, Inlines: []richdoc.Inline{richdoc.Txt("T")}}}},
			[]string{"T\n^"},
		},
		{
			"an ordered list with a start below 1 clamps to 1",
			richdoc.New().OList(0, true,
				richdoc.Item(richdoc.Paragraph{Inlines: []richdoc.Inline{richdoc.Txt("a")}}),
			).Doc(),
			[]string{"1. a"},
		},
		{
			"a table row wider than its header widens the grid to fit",
			richdoc.New().Table(nil,
				[]richdoc.Cell{richdoc.Td(richdoc.Txt("a"))},
				[][]richdoc.Cell{{richdoc.Td(richdoc.Txt("1")), richdoc.Td(richdoc.Txt("2"))}},
			).Doc(),
			[]string{"1", "2"},
		},
		{
			"a link whose text is richly marked up exercises every plainText branch",
			richdoc.New().P(richdoc.Href("https://x.io", "",
				richdoc.Bold(richdoc.Txt("b")), richdoc.Italic(richdoc.Txt("i")),
				richdoc.Strike(richdoc.Txt("s")), richdoc.Mono("c"), richdoc.InlineMath("m"),
				richdoc.RawI("rst", "r"), richdoc.Mark("id", richdoc.Txt("a")),
				richdoc.Ref("t", richdoc.Txt("cr")),
				richdoc.Href("https://y.io", "", richdoc.Txt("nested")),
			)).Doc(),
			[]string{"<https://x.io>`_"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Write(tc.doc)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			for _, want := range tc.want {
				if !strings.Contains(string(got), want) {
					t.Errorf("Write output missing %q\ngot:\n%s", want, got)
				}
			}
		})
	}
}
