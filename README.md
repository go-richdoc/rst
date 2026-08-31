# go-richdoc/rst

Convert between reStructuredText and the format-agnostic
[`richdoc`](https://github.com/go-richdoc/richdoc) document model.

- **`Parse`** reads reST source into a `*richdoc.Document`. Parsing delegates
  to [`github.com/go-docutils/docutils/rst`](https://github.com/go-docutils/docutils)
  — a full, independently maintained pure-Go reST engine — and walks its
  doctree; this package never hand-rolls a reST parser, the same principle
  [`markdown`](https://github.com/go-richdoc/markdown) follows for
  CommonMark via goldmark.
- **`Write`** renders a `*richdoc.Document` back to reST text.

```go
import "github.com/go-richdoc/rst"

doc, err := rst.Parse([]byte("Title\n=====\n\nHello **world**.\n"))
// ... inspect or edit doc ...
out, err := rst.Write(doc)
```

## Why delegate parsing instead of shipping a subset parser

[`latex`](https://github.com/go-richdoc/latex) ships its own LaTeX-subset
parser because [go-tex/engine](https://github.com/go-tex/engine) is a
typesetting engine whose tokenizer is internal — it exposes a compile API,
not a reusable parse tree. reST has no such gap: `docutils/rst` already is a
full, tested, pure-Go parse tree producer. Writing a second one here would
duplicate real work for no fidelity gain, so this package leans on it
directly. That dependency also becomes this package's own correctness proof:
`Write`'s output is verified valid reST by feeding it back through
`docutils/rst.Parse` and confirming it survives another round-trip — no
separate reference tool (no tectonic-style external compiler, no Python
`docutils` install) needed, unlike `latex`'s engine-compile check.

## Node mapping

### Parse (docutils doctree → richdoc)

| doctree tag | richdoc node |
| --- | --- |
| `section` | flattened: title becomes a `Heading` at the nesting depth (clamped to 1–6), the rest of the section's content follows one level deeper — richdoc has no section wrapper. `Heading.ID` carries the section's own implicit-target slug (docutils/rst v0.17.0+), so a resolved `` `Some Title`_ `` reference — already a `Link` to `"#the-slug"` — points at a real anchor |
| `paragraph` | `Paragraph` |
| `bullet_list` / `enumerated_list` | `List` (`Tight` true when every item is exactly one `Paragraph`) |
| `list_item` | `ListItem` |
| `block_quote` | `BlockQuote`, nested (docutils/rst v0.19.0+) when the source's own indentation varies within the run |
| `attribution` (docutils/rst v0.19.0+ — a block quote's trailing "-- text" line) | a plain trailing `Paragraph` inside the enclosing `BlockQuote` — richdoc has no dedicated attribution concept, and the generic block fallback can't reach it at all (its children are bare inline nodes, not block-level `Paragraph` wrappers, the same shape `<raw>` needed its own case for), so this has a dedicated case too, preserving the text rather than dropping it |
| `transition` | `ThematicBreak` |
| `literal_block`, `doctest_block` | `CodeBlock` (no language; the literal/doctest distinction isn't preserved) |
| `table` (simple or grid) | `Table` — a grid cell's row/column span carries through to `Cell.ColSpan`/`RowSpan` (richdoc v0.3.0+); a cell's own content, when it's more than one top-level block (a nested list, several paragraphs — grid tables allow full block content in a cell, `Cell` cannot), is flattened with each top-level block joined by a single space rather than the words running together |
| `emphasis` / `strong` / `literal` | `Emph` / `Strong` / `Code` |
| `title_reference` | `Emph` (the nearest common styling; richdoc has no dedicated node) |
| `math` (docutils' dedicated `:math:` node, not routed through `inline` at all) | `Math` |
| an INLINE `raw` (docutils/rst v0.16.0+ — a `.. role:: name(raw)`-registered role invoked as `` :name:`text` ``) | `RawInline`, Format the role's own real target format — the inline counterpart of the block-level `raw` case below |
| an INLINE `target` (`` _`text` ``, docutils/rst v0.4.0+ — a target inside a paragraph, as opposed to a block-level hyperlink target, see below) | `Anchor` — its "name" attribute (derived from its own visible text) becomes `Anchor.ID` |
| `reference` with a resolved `refuri` | `Link`; an unresolved ANONYMOUS one (docutils/rst never rewrites those — see its own README) falls back to plain inline content; a resolved same-document anchor (`refuri` starting with `#`, from an inline internal target above) is still just a `Link` — richdoc has no distinct "internal cross-reference by name" inline type of its own besides `CrossRef`, which this package reserves for its own `Write` output (see below) |
| `problematic` (docutils/rst v0.13.0+ — a dangling NAMED reference rewritten in place; v0.18.0+ also an unclosed inline-markup start-string, e.g. an emphasis `*` with no closing `*`) / a trailing `<section class="system-messages">` | no dedicated case for either: `problematic`'s own text passes through as plain inline text via this package's generic inline fallback, and the section becomes an ordinary `Heading` + `Paragraph` (its `system_message` children have no case either, so their own `paragraph` child converts normally) the same way any other section would |
| `substitution_reference` | resolved against its `substitution_definition`'s value and inlined directly — a real resolution this package's sibling docutils/html and docutils/latex writers deliberately don't perform; an orphan reference falls back to its bare name |
| `footnote_reference` / `citation_reference` | resolved against its definition and inlined as a `Footnote` at the reference site — both reST forms are self-contained label+body constructs, unlike LaTeX's external-bibliography `\cite`; this includes auto-numbered `[#]_`/symbol `[*]_` forms as of `docutils/rst` v0.7.0, which assigns each an internal synthetic name so it resolves the same way as any other; a reference with genuinely no matching definition anywhere falls back to a verbatim `RawInline` |
| `:strike:` role | `Strikethrough` — this package's own convention (reST has no native strikethrough at all); `Write` emits the same role name back |
| leading field list (the document's very first block) — plain, or (`docutils/rst` v0.12.0+) promoted to `docinfo` when it has a registered bibliographic name | `Document.Meta`, keyed by field name or, for a typed docinfo child, its own tag (`author`, `date`, `version`, ...); `authors` joins its names with `"; "`; a trailing `dedication`/`abstract` `topic` sibling (docutils' own DocInfo transform emits it right after docinfo, not inside it) is folded in as one more Meta entry, its own title dropped |
| a BLOCK-level hyperlink `target`, a `substitution_definition` | dropped — invisible bookkeeping whose consuming references are already resolved by the time this package sees the tree |
| `raw` (`docutils/rst` v0.15.0+, `Options.RawEnabled` — on by default there) | `RawBlock`, Format its real target format (`"html"`, `"latex"`, possibly several space-separated) — genuine target-format content docutils itself already tagged, not this package's own reST resynthesis, so `Write` reconstructs it as a real `.. raw:: FORMAT` directive rather than dropping it the way any OTHER non-`"rst"` `RawBlock` still is (see below) |

**Falls back to `RawBlock`/`RawInline` with Format `"rst"`** (so nothing is
silently lost, resynthesized from parsed structure rather than a verbatim
source slice — semantically equivalent, not necessarily byte-identical, see
the doc comment on `rawsource.go`): directives, comments, a non-leading field
list, definition lists, line blocks, option lists (man-page-style
`-f, --file=ARG` items), subscript/superscript, any other
interpreted-text role, an unresolvable footnote/citation reference, and an
orphan footnote/citation definition (one no reference in the document ever
resolved to — preserved rather than dropped, in case a converter or a human
reader still wants it). An abbreviation/acronym instead flattens straight to
its plain text: the visible content stays readable, only the "this was
marked" fact is lost.

### Write (richdoc → reST)

| richdoc node | reST output |
| --- | --- |
| `Heading` | underlined title (`=`, `-`, `~`, `"`, `^` by depth, clamped); a non-empty `ID` emits a leading `.. _id:` hyperlink target |
| `Paragraph` | inline text |
| `List` | `-` / `N.` items; a non-1 `Start` (richdoc is a hub other converters build documents for, not just this package's own round-trip — `docutils/rst`'s own doctree carries no start-number attribute, so `Parse` can't recover one) is honoured on write, one-way |
| `CodeBlock` | `::` literal block |
| `BlockQuote` | indented block |
| `Table` | a GRID table (`+---+`), column widths computed from actual cell content |
| `MathBlock` | `.. math::` directive |
| `RawBlock` (block) | Format `""` or `"rst"` passes through verbatim; any OTHER format reconstructs as a real `.. raw:: FORMAT` directive — a general reST construct any reader can interpret, not something specific to this package |
| `RawInline` | Format `""` or `"rst"` passes through verbatim; any other format is dropped — unlike the block case above, reST HAS an inline raw construct (`docutils/rst` v0.16.0+'s `` :name:`text` `` where `name` is a `.. role:: name(raw)`-registered role), but using it means emitting that registration as its own block BEFORE the paragraph currently being written, which this package's inline-rendering functions (building one paragraph's text at a time) have no way to reach back and do — a real gap, not a "nothing to reconstruct from" one like the parallel block case's old excuse used to be |
| `Emph` / `Strong` / `Code` / `Strikethrough` / `Math` | `*x*` / `**x**` / `` ``x`` `` / `:strike:`x`` / `:math:`x`` |
| `Link` | a bare URL round-trips through standalone-URI auto-recognition with no markup at all; otherwise `` `text <url>`_ `` |
| `Anchor` with visible text | `` _`text` ``, reST's inline internal target — `Parse` reads this back (see above); a point anchor (no visible text) has no reST equivalent and renders to nothing |
| `Footnote` | an inline `[n]_` reference; its body is collected and emitted as a trailing `.. [n] ...` definition, numbered in reference order — the same accumulate-then-emit pattern `markdown`'s Write uses for `[^n]: ...` |
| `CrossRef` (label) | `` `text <target_>`_ ``, this package's own embedded-alias convention; (citation) a bare `[target]_` — reST citations are self-contained (unlike LaTeX's external-bibliography `\cite`), so without a matching `.. [target] ...` definition elsewhere in the same document this degrades gracefully to plain text on reparse, same as any other unresolved reference |
| `Document.Meta` | a leading field list (`:key: value`, sorted by key), the same convention `Parse` reads back |

**Known one-way gaps**, each because reST's core syntax has no construct for
it at all (not a bug, a real format-capability mismatch — the same category
as `latex`'s undepended-on `multirow` package for real LaTeX rowspan, or
`markdown`'s dropped `Anchor` id): an inline `Image` degrades to its alt
text (reST's only image construct, `.. image::`, is block-level, and can't
legally appear inside a paragraph); a hard `LineBreak` emits a literal
newline, which reads back as an ordinary wrapped line, not a break; a POINT
`Anchor` (no visible text) has nothing to attach reST's inline-target syntax
to (that syntax requires non-empty backtick-quoted content) and renders to
nothing — an `Anchor` WITH visible text now round-trips faithfully as of
`docutils/rst` v0.4.0 (see above), UNLESS its `ID` was supplied separately
from its own text (for example by a converter other than this package's own
`Parse` — LaTeX's `\label{sec-intro}` next to unrelated visible content):
reST resolves an inline target by its visible text, not by an externally
attached id, so the written document's target re-resolves under a different
name than the original `ID`.

## To a PDF

`rst/pdf` typesets a document rather than writing reST source for one:

```go
data, err := pdf.Write(doc, pdf.Options{})   // *richdoc.Document -> PDF bytes
```

It goes one step further than [`latex/pdf`](https://github.com/go-richdoc/latex),
which writes LaTeX directly. Here the document goes out through this
package's own `Write` as real reST source, that source is parsed by
[`docutils/rst`](https://github.com/go-docutils/docutils) — the same engine
`Parse` above builds on — and the resulting doctree is rendered to LaTeX by
`docutils/latex` before [go-tex/engine](https://github.com/go-tex/engine)
compiles it. The extra hop is the point: it proves `Write`'s reST is not
merely reST that reparses into the same tree (this package's own round-trip
check, above), but reST a real LaTeX toolchain accepts and typesets — the
same chain proved by hand, earlier, compiling go-tex's own documentation
after it went through docutils first.

**It is a package rather than a module, and not part of this one**, for the
same reason as `latex/pdf`: the engine is a six-megabyte TeX implementation
this module already names only from a test, so importing `rst` on its own
must not link it.

What survives the whole way, read back out of the finished PDF with
`pdftotext` rather than trusted: headings, emphasis, bold, bulleted lists,
block code, and accented text.

## Round-trip

`Parse(Write(Parse(src)))` reproduces `Parse(src)`'s tree for the natively
mapped constructs above (verified in `roundtrip_test.go` against a corpus
covering each one, using `docutils/rst.Parse` itself as the check — see
"Why delegate parsing" above). The `RawBlock`/`RawInline` fallback
constructs are covered separately in `unit_test.go` instead, since by design
they resynthesize reST rather than preserve a byte-exact slice.

## Testing

`go test ./...`. `go vet ./...` and `gofmt -l .` are clean; CI enforces a
95% coverage floor rather than 100% — see the comment in
`.github/workflows/ci.yml` for why this package's coverage bar differs from
its sibling converters'.

## License

BSD-3-Clause. See [LICENSE](LICENSE).
