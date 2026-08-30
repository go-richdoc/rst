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
| `section` | flattened: title becomes a `Heading` at the nesting depth (clamped to 1–6), the rest of the section's content follows one level deeper — richdoc has no section wrapper |
| `paragraph` | `Paragraph` |
| `bullet_list` / `enumerated_list` | `List` (`Tight` true when every item is exactly one `Paragraph`) |
| `list_item` | `ListItem` |
| `block_quote` | `BlockQuote` |
| `transition` | `ThematicBreak` |
| `literal_block`, `doctest_block` | `CodeBlock` (no language; the literal/doctest distinction isn't preserved) |
| `table` (simple or grid) | `Table` — a grid cell's row/column span collapses to its own cell, richdoc has no span primitive (same as a plain Markdown table) |
| `emphasis` / `strong` / `literal` | `Emph` / `Strong` / `Code` |
| `title_reference` | `Emph` (the nearest common styling; richdoc has no dedicated node) |
| `reference` with a resolved `refuri` | `Link`; unresolved falls back to plain inline content |
| `substitution_reference` | resolved against its `substitution_definition`'s value and inlined directly — a real resolution this package's sibling docutils/html and docutils/latex writers deliberately don't perform; an orphan reference falls back to its bare name |
| `footnote_reference` / `citation_reference` | resolved against its definition and inlined as a `Footnote` at the reference site — both reST forms are self-contained label+body constructs, unlike LaTeX's external-bibliography `\cite`; an unresolvable reference (most often reST's own auto-numbered `[#]_`/symbol `[*]_` forms, which `docutils/rst` never assigns a name) falls back to a verbatim `RawInline` |
| `:strike:`/`:math:` role | `Strikethrough` / `Math` — this package's own convention (reST has neither natively); `Write` emits the same role names back |
| leading field list (the document's very first block) | `Document.Meta` |
| a hyperlink `target`, a `substitution_definition` | dropped — invisible bookkeeping whose consuming references are already resolved by the time this package sees the tree |

**Falls back to `RawBlock`/`RawInline` with Format `"rst"`** (so nothing is
silently lost, resynthesized from parsed structure rather than a verbatim
source slice — semantically equivalent, not necessarily byte-identical, see
the doc comment on `rawsource.go`): directives, comments, a non-leading field
list, definition lists, line blocks, subscript/superscript, any other
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
| `RawBlock` / `RawInline` | Format `""` or `"rst"` passes through verbatim; any other format is dropped (a converter, not a filter — foreign raw content isn't this package's to interpret) |
| `Emph` / `Strong` / `Code` / `Strikethrough` / `Math` | `*x*` / `**x**` / `` ``x`` `` / `:strike:`x`` / `:math:`x`` |
| `Link` | a bare URL round-trips through standalone-URI auto-recognition with no markup at all; otherwise `` `text <url>`_ `` |
| `Footnote` | an inline `[n]_` reference; its body is collected and emitted as a trailing `.. [n] ...` definition, numbered in reference order — the same accumulate-then-emit pattern `markdown`'s Write uses for `[^n]: ...` |
| `CrossRef` (label) | `` `text <target_>`_ ``, this package's own embedded-alias convention; (citation) a bare `[target]_` — reST citations are self-contained (unlike LaTeX's external-bibliography `\cite`), so without a matching `.. [target] ...` definition elsewhere in the same document this degrades gracefully to plain text on reparse, same as any other unresolved reference |
| `Document.Meta` | a leading field list (`:key: value`, sorted by key), the same convention `Parse` reads back |

**Known one-way gaps**, each because reST's core syntax has no construct for
it at all (not a bug, a real format-capability mismatch — the same category
as `latex`'s undepended-on `multirow` package for real LaTeX rowspan, or
`markdown`'s dropped `Anchor` id): an inline `Image` degrades to its alt
text (reST's only image construct, `.. image::`, is block-level, and can't
legally appear inside a paragraph); a hard `LineBreak` emits a literal
newline, which reads back as an ordinary wrapped line, not a break; an
`Anchor`'s `ID` is dropped, only its marked text renders (`docutils/rst` has
no reader yet for reST's own inline-internal-target syntax — see its
README's "Not yet ported" list — so emitting it couldn't round-trip through
this package's own `Parse` anyway).

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
