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
| `section` | flattened: title becomes a `Heading` at the nesting depth (clamped to 1–6), the rest of the section's content follows one level deeper — richdoc has no section wrapper. `Heading.ID` carries the section's own implicit-target slug (docutils/rst v0.17.0+), so a resolved `` `Some Title`_ `` reference — already a `Link` to `"#the-slug"` — points at a real anchor. An explicit `.. _label:` written in FRONT of the title takes that slot instead — the Sphinx convention for labelling a section, and what `Heading.ID` is documented for ("a LaTeX `\section` immediately followed by `\label`"). docutils does this in a transform (`references.PropagateTargets`, which hands an internal target's ids to the next node, so the section carries both); richdoc's `Heading` has one ID, so the label wins and every id that used to name that section — its slug, and any further labels — is remapped onto it, or a `label_` reference would point at an id nothing carries. A label in front of anything OTHER than a section is still dropped: no other richdoc block has an id to put it on |
| `paragraph` | `Paragraph` |
| `bullet_list` / `enumerated_list` | `List` (`Tight` true when every item is exactly one `Paragraph`) |
| `list_item` | `ListItem` |
| `block_quote` | `BlockQuote`, nested (docutils/rst v0.19.0+) when the source's own indentation varies within the run |
| `attribution` (docutils/rst v0.19.0+ — a block quote's trailing "-- text" line) | a plain trailing `Paragraph` inside the enclosing `BlockQuote` — richdoc has no dedicated attribution concept, and the generic block fallback can't reach it at all (its children are bare inline nodes, not block-level `Paragraph` wrappers, the same shape `<raw>` needed its own case for), so this has a dedicated case too, preserving the text rather than dropping it |
| `transition` | `ThematicBreak` |
| `literal_block` with `:number-lines:` | the numbers are DROPPED: docutils generates them into `<inline class="ln">` children of the block, and taking the node's whole text folded them into the code — `x = 1` came back as `1 x = 1`, no longer the program the author wrote, in valid reST that nothing reported. `richdoc.CodeBlock` has no line-numbering flag, so the OPTION cannot survive; that is a boundary, while generated numbers inside the code are wrong under any model. No corpus document uses the option, so the witness is the test alone |
| `literal_block`, `doctest_block` | `CodeBlock`, taking its `Language` from the class list `.. code::` leaves behind (`["code", <language>, ...]`); a plain `::` block has no classes and stays languageless. The literal/doctest distinction isn't preserved |
| `table` (simple or grid) | `Table` — a grid cell's row/column span carries through to `Cell.ColSpan`/`RowSpan` (richdoc v0.3.0+); a cell's own content, when it's more than one top-level block (a nested list, several paragraphs — grid tables allow full block content in a cell, `Cell` cannot), is flattened with each top-level block joined by a single space rather than the words running together |
| `emphasis` / `strong` / `literal` | `Emph` / `Strong` / `Code` |
| `title_reference` | `Emph` (the nearest common styling; richdoc has no dedicated node) |
| `math` (docutils' dedicated `:math:` node, not routed through `inline` at all) | `Math` |
| an INLINE `raw` (docutils/rst v0.16.0+ — a `.. role:: name(raw)`-registered role invoked as `` :name:`text` ``) | `RawInline`, Format the role's own real target format — the inline counterpart of the block-level `raw` case below |
| an INLINE `target` (`` _`text` ``, docutils/rst v0.4.0+ — a target inside a paragraph, as opposed to a block-level hyperlink target, see below) | `Anchor` — its "name" attribute (derived from its own visible text) becomes `Anchor.ID` |
| `reference` with a resolved `refuri` | `Link`; an unresolved ANONYMOUS one (docutils/rst never rewrites those — see its own README) falls back to plain inline content; a resolved same-document anchor (`refuri` starting with `#`, from an inline internal target above) is still just a `Link` — richdoc has no distinct "internal cross-reference by name" inline type of its own besides `CrossRef`, which this package reserves for its own `Write` output (see below) |
| `image` (`docutils/rst` v0.29.0+) | `Image` — reached both for a standalone block-level image (wrapped in a single-inline `Paragraph`, see below) and for a substitution definition's own embedded `image::` (inline-classified in real docutils too, the only reason it survives that definition's own Inline-only filter unflattened) |
| `problematic` (docutils/rst v0.13.0+ — a dangling NAMED reference rewritten in place; v0.18.0+ also an unclosed inline-markup start-string, e.g. an emphasis `*` with no closing `*`) / a trailing `<section class="system-messages">` | no dedicated case for either: `problematic`'s own text passes through as plain inline text via this package's generic inline fallback, and the section becomes an ordinary `Heading` + `Paragraph` (its `system_message` children have no case either, so their own `paragraph` child converts normally) the same way any other section would. A `system_message` itself is DROPPED by default (v0.117.0+): a diagnostic is about the source, not part of the document, and rendering it as a paragraph put text like "Explicit markup ends without a blank line; unexpected unindent." in the prose as though an author had written it. `ParseWithOptions` with `Options{KeepDiagnostics: true}` brings them back, for a tool that converts a document in order to report on it. Dropping them means dropping them from a `RawBlock` reconstruction too: every `raw*` helper walks its element's own children, so a `system_message` nested inside a construct that becomes reST source used to reach the output as PROSE — three real-world corpus files ended with a sentence like `Duplicate explicit target name: "versionadded".` sitting in the text as if an author had written it. `docutils/rst` v0.136.2 made that visible by moving a duplicate-name notice INSIDE the `.. note::` it belongs to; the leak itself was older and wider. Dropping it does NOT drop the author's text: docutils quotes the offending source inside the message as a `literal_block` (a malformed table carries its whole source that way, `docutils/rst` v0.121.0+), and that comes through as a `CodeBlock` while the diagnostic paragraph does not — but ONLY where the construct was REFUSED, which the message's LEVEL says: docutils refuses at ERROR and above (malformed table, unknown directive, invalid marker, missing or mismatched underline, a directive with no content) and keeps the construct at WARNING and below, where the quote is a second copy of text that is already in the document. Keeping it there put a heading in twice, once as the heading and once as a literal block nobody wrote; 3 of the 1564 real-world corpus files |
| `substitution_reference` | resolved against its `substitution_definition`'s value and inlined directly — a real resolution this package's sibling docutils/html and docutils/latex writers deliberately don't perform; an orphan reference falls back to its bare name |
| `footnote_reference` / `citation_reference` | resolved against its definition and inlined as a `Footnote` at the reference site — both reST forms are self-contained label+body constructs, unlike LaTeX's external-bibliography `\cite`; this includes auto-numbered `[#]_`/symbol `[*]_` forms as of `docutils/rst` v0.7.0, which assigns each an internal synthetic name so it resolves the same way as any other; a reference with genuinely no matching definition anywhere falls back to a verbatim `RawInline`. Whether a definition is inlined (and so dropped as a block) is decided in the pre-pass that collects the definitions themselves, NOT at the reference site, so it does not depend on which the conversion reaches first — before that it did, and a document whose definitions precede their references printed each one twice |
| `:strike:` role | `Strikethrough` — this package's own convention (reST has no native strikethrough at all); `Write` emits the same role name back |
| leading field list (the document's first block, looking PAST any hoisted `meta` nodes ahead of it — `docutils/rst` v0.136.5+ puts a `.. meta::` directive's own nodes there, and its DocInfo transform steps over them the same way) — plain, or (`docutils/rst` v0.12.0+) promoted to `docinfo` when it has a registered bibliographic name | `Document.Meta`, keyed by field name or, for a typed docinfo child, its own tag (`author`, `date`, `version`, ...); `authors` joins its names with `"; "`; a trailing `dedication`/`abstract` `topic` sibling (docutils' own DocInfo transform emits it right after docinfo, not inside it) is folded in as one more Meta entry, its own title dropped |
| a BLOCK-level hyperlink `target`, a `substitution_definition` | dropped — invisible bookkeeping whose consuming references are already resolved by the time this package sees the tree |
| the IMPLICIT `target` sibling a named phrase-reference-with-embedded-link emits (`` `text <uri>`_ ``/`` `text <alias_>`_ ``, `docutils/rst` v0.31.0+) | dropped, the same way a block-level target is — it carries no content of its own (real docutils constructs it with none, just `refuri`/`refname` for some OTHER reference elsewhere to resolve against), and the reference that produced it already carries its own resolved refuri/refname directly; distinguished from a real INLINE internal target (`` _`text` ``, below) by having no children at all |
| a standalone block-level `image` with a `:target:` (`docutils/rst` v0.127.0+) | `Paragraph` wrapping a single `Link` wrapping the `Image` — the badge every project README opens with; without this the link is silently dropped |
| a standalone block-level `image` (`docutils/rst` v0.29.0+) | `Paragraph` wrapping a single `Image` inline — richdoc has no bare block-level image concept of its own, so this is the nearest non-lossy placement (the same way CommonMark itself treats a standalone image), not a `RawBlock` fallback: unlike an admonition or a topic, nothing about "this was a directive" needs preserving here |
| `raw` (`docutils/rst` v0.15.0+, `Options.RawEnabled` — on by default there) | `RawBlock`, Format its real target format (`"html"`, `"latex"`, possibly several space-separated) — genuine target-format content docutils itself already tagged, not this package's own reST resynthesis, so `Write` reconstructs it as a real `.. raw:: FORMAT` directive rather than dropping it the way any OTHER non-`"rst"` `RawBlock` still is (see below) |

**Falls back to `RawBlock`/`RawInline` with Format `"rst"`** (so nothing is
silently lost, resynthesized from parsed structure rather than a verbatim
source slice — semantically equivalent, not necessarily byte-identical, see
the doc comment on `rawsource.go`; a reconstruction that carries OPTIONS
puts them on the line directly under the directive, since a blank line
there ends the directive's option region and makes docutils read
`:class: x` as a field list in the content instead — the round-trip test
is what holds that, not a fixture recording whatever the writer does): directives, comments, a non-leading field
list, definition lists, line blocks, option lists (man-page-style
`-f, --file=ARG` items), the nine generic admonitions
(`attention`/`caution`/`danger`/`error`/`hint`/`important`/`note`/`tip`/
`warning`, `docutils/rst` v0.27.0+) plus `.. admonition::` itself (richdoc
has no admonition/callout block type at all — its own `Block` interface is
a documented closed set — so this is preserved as a `RawBlock`, not
silently unwrapped to the bare content with no trace it was ever a
note/warning/etc.), `.. topic::`/`.. sidebar::` (`docutils/rst` v0.28.0+ —
same reasoning, no topic/sidebar block type either; a leading
dedication/abstract `topic` is the one exception, folded into
`Document.Meta` above instead), `figure` (`docutils/rst` v0.29.0+ —
richdoc has no figure/caption/legend concept either, so — unlike a bare
`image`, see above — this falls back to a `RawBlock` too, reconstructing
every image-level AND figure-level option on one `.. figure::` block,
matching real docutils' own single-directive shape), `meta`
(`docutils/rst` v0.30.0+ — HTML/head metadata, a DIFFERENT concept from
`Document.Meta` above even though the shape looks similar — falls back
to a `RawBlock` too, one `.. meta::` per element, matching the shape
upstream's own document-front hoisting already leaves them in: every
meta field its own sibling, never grouped back into the directive that
originally produced several of them together), subscript/superscript, any other
interpreted-text role, an unresolvable footnote/citation reference, and an
orphan footnote/citation definition (one no reference in the document ever
resolved to — preserved rather than dropped, in case a converter or a human
A role rebuilt this way has its content RE-ESCAPED on the way out
(`rawRole`): the text is content on one side of the reconstruction and
reST source on the other. A backslash is reST's escape, so content
`PC\python` written literally re-parses as `PCpython` — and writing
THAT again loses nothing more, so the damage compounded silently across
round trips rather than surfacing as an error. A backquote closes the
role, so content `` a`b `` ended the construct early. `*`, `|` and `_`
are inert inside the backquotes and are deliberately left alone.

reader still wants it). An abbreviation/acronym instead flattens straight to
its plain text: the visible content stays readable, only the "this was
marked" fact is lost.

The reST resynthesised for those fallbacks keeps the body's BLOCK
structure: a footnote, field, definition or option-list body with two
paragraphs, or with a list inside it, comes back with them. Until
v0.113.0 all four joined each child block's text with a single space, so
two paragraphs became one sentence and a list became a run-on — the same
flattening removed from five other places in v0.99.0 and left in these.
Inline STYLING is no longer lost where docutils parses it, and the reason
this note used to give for losing it — "it would need a second full
inline-to-reST emitter just for this fallback path" — was the obstacle that
did not exist: the emitter already exists on the WRITING side, and
`inlineSourceOf` reuses it by converting a node's children to richdoc inlines
and writing those. One spelling of the inline grammar, used in both
directions. A definition term `**Read the Docs**` used to come back as plain
`Read the Docs`; the same was true of a term's classifier, an admonition
title, a topic title, a sidebar subtitle, a rubric argument, a line block's
line and a non-leading field name. 78 of the 1564 real-world corpus files
change. Only the SOURCE-vS-OUTPUT judge can see this class, since the
flattened text is already in the first tree and a round-trip comparison
reproduces it.

Two containers deliberately stay verbatim, because docutils does not parse
them for markup either: an option-list flag and a comment. And one boundary
remains by construction: a LEADING field list becomes `Document.Meta`, a
`map[string]string` with nowhere to put markup. A PARAGRAPH inside one of these
fallbacks keeps its inline markup too: `rawChildSource` had no case for one,
so it fell to the text fallback and every link, emphasis, literal and role
inside a `.. note::`, topic, container, block quote or reconstructed list item
was dropped. PEP 6's own note reads "documented in
`` `the devguide <...>`__ `` and came back as "documented in the devguide",
the link simply gone: 237 of the 1564 files. A LITERAL block stays verbatim
beside it, because docutils does not parse markup inside one — which is why
this is a per-tag decision rather than "stop calling AsText". The block-level
STRUCTURE of these fallbacks is still flattened per child block, a different
question from the inline one.

### Write (richdoc → reST)

| richdoc node | reST output |
| --- | --- |
| `Heading` | underlined title (`=`, `-`, `~`, `"`, `^` by depth, clamped); a non-empty `ID` emits a leading `.. _id:` hyperlink target — unless the parser would produce that id by itself, which covers the title's own slug (`docutils/rst`'s exported `MakeID`, so the rule cannot drift from the one the parser uses) and an id that is that slug, or `section-`, plus only NUMERIC suffixes: those are what `claimID` appends for a repeated title and what a title with no ASCII at all gets instead. Writing one of those back claims the name, which pushes the section's own id one suffix further, and the next round trip lands back on the first — the document oscillates between two forms rather than settling. The underline is as long as the title LINE is WIDE, using `docutils/rst`'s `ColumnWidth` (v0.116.0+): an East Asian character occupies two columns and a combining mark none, and docutils compares those columns against the underline's length — a rune-count underline made a CJK heading come back as a warning and a literal block. The title line carries its MARKUP: writing the plain text instead dropped every emphasis, inline literal, role and reference inside a heading, and since the flattened form is stable on a second write, an output-comparing round-trip check called it fine. Comparing the round-tripped TREES over the corpus found it — 398 of the 1564 files did not come back as themselves, and this was 188 of them. The reasoning it replaces was that the underline should match the title's VISIBLE width; docutils only warns when an underline is SHORTER than its title line, so a longer one is legal and there was nothing to buy |
| `Paragraph` | inline text |
| `List` | `-` / `N.` items; a non-1 `Start` round-trips both ways as of `docutils/rst` v0.25.0+ (its own `enumerated_list` now carries a `start` attribute, read by `Parse`) — the list's own enumerator TYPE (alpha/roman) and format (`(N)`/`N)`) have no richdoc equivalent at all, so `Write` always re-renders as plain arabic `N.`, still a one-way gap on that narrower axis |
| `CodeBlock` | `.. code:: <language>` when it has one, plain `::` when it does not |
| `BlockQuote` | indented block |
| `Table` | a GRID table (`+---+`), column widths computed from actual cell content, measured with `docutils/rst`'s own `TableColumnWidth` (v0.110.0+) so an East Asian Wide character counts as the two columns the grid gives it — padding by rune count made a CJK cell overflow its column, and a table this package had just parsed did not survive being written back out |
| `MathBlock` | `.. math::` directive |
| `RawBlock` (block) | Format `""` or `"rst"` passes through verbatim; any OTHER format reconstructs as a real `.. raw:: FORMAT` directive — a general reST construct any reader can interpret, not something specific to this package |
| `RawInline` | Format `""` or `"rst"` passes through verbatim; any other format is dropped — unlike the block case above, reST HAS an inline raw construct (`docutils/rst` v0.16.0+'s `` :name:`text` `` where `name` is a `.. role:: name(raw)`-registered role), but using it means emitting that registration as its own block BEFORE the paragraph currently being written, which this package's inline-rendering functions (building one paragraph's text at a time) have no way to reach back and do — a real gap, not a "nothing to reconstruct from" one like the parallel block case's old excuse used to be |
| `Emph` / `Strong` / `Code` / `Strikethrough` / `Math` | `*x*` / `**x**` / `` ``x`` `` / `:strike:`x`` / `:math:`x`` |
| every `Text` | a newline inside it keeps the break and LOSES the indentation that followed. docutils keeps the source's line breaks in the text node and this reader passes them through, so re-emitting the break with its indentation made a continuation DEEPER than its own first line — which is a definition or a block quote, not a paragraph. PEP 262 wraps one with reST's escaped continuation (`...around 28` / `\   Dec 1999)`) and came back as a term plus its definition: 15 of the 1564 files. The break itself is harmless (reST folds it to a space) and keeping it keeps the author's layout, which `indentBlock` then re-indents uniformly — collapsing it to a space instead reflowed every reconstructed body onto one line, which two existing tests caught |
| every `Paragraph` | its FIRST line is escaped when it would otherwise read as a BLOCK: a bullet, an enumerator in any of its spellings, explicit markup, a field marker, an option list, a doctest, an adornment line or a table top. escapeText covers the inline markers wherever they appear; this is the positional half, and it had exactly one case (`\|`, which escapeText catches by accident) out of nine. A paragraph reading `B. Smith wrote this` came back as an enumerated list, `- Not a bullet either` as a bullet list, `:not: a field` as a field list, and `.. not a directive` as a COMMENT — invisible in every rendering, and the output parsed cleanly every time. Only the FIRST line is escaped, which is the rule and not a shortcut: docutils reads a whole text block before deciding what it is, so a later line shaped like a marker is content — escaping every line put a backslash inside an inline literal that spanned two source lines |
| every inline | joined with reST's NULL SEPARATOR (`\ `, a backslash-escaped space that renders nothing) wherever the join would put markup where reST cannot see it. Inline markup is markup only at a BOUNDARY — a start-string preceded by whitespace or one of ``-:/'"<([{``, an end-string followed by whitespace or one of ``-.,:;!?\/'")]}>`` — and richdoc puts no whitespace of its own between inlines. So `[Text("See it"), Emph("em"), Text(".")]` was written `See it*em*.` and read back as the literal TEXT `See it*em*.`: emphasis, strong, inline literal and footnote reference all stopped being markup, and only a phrase reference survived. Nothing reported it, because the output is valid reST that no longer says what the input said |
| `Link` | a bare URL round-trips through standalone-URI auto-recognition with no markup at all, and so does a bare EMAIL address, whose `mailto:` URI docutils supplies from the source rather than the source writing it — spelling that one out put an explicit reference where the author had plain text, and it was the largest single difference between the doctree of a source and the doctree of its own round trip over the 1564-file corpus; otherwise `` `text <url>`__ `` (ANONYMOUS, see writeLink's own comment for why) |
| `Anchor` with visible text | `` _`text` ``, reST's inline internal target — `Parse` reads this back (see above); a point anchor (no visible text) has no reST equivalent and renders to nothing |
| `Footnote` | an inline `[n]_` reference; its body is collected and emitted as a trailing `.. [n] ...` definition, numbered in reference order — the same accumulate-then-emit pattern `markdown`'s Write uses for `[^n]: ...`. EXPLICIT rather than auto (`[#]_`), and that choice was MEASURED: richdoc's `Footnote` carries no label, so one spelling has to serve both an auto-numbered source and a manually numbered one. The auto form preserves the 53 corpus files that use `[#]_` and costs more than it buys — a document whose labels are 1..N in order, much the commoner shape, round-trips exactly under the explicit form and becomes an auto reference under the other: source-vs-output equivalence 948 against 865 |
| `CrossRef` (label) | `` `text <target_>`_ ``, this package's own embedded-alias convention; (citation) a bare `[target]_` — reST citations are self-contained (unlike LaTeX's external-bibliography `\cite`), so without a matching `.. [target] ...` definition elsewhere in the same document this degrades gracefully to plain text on reparse, same as any other unresolved reference |
| `Document.Meta` | a leading field list (`:key: value`, sorted by key), the same convention `Parse` reads back |

**Known one-way gaps**, each because reST's core syntax has no construct for
it at all (not a bug, a real format-capability mismatch — the same category
as `latex`'s undepended-on `multirow` package for real LaTeX rowspan, or
`markdown`'s dropped `Anchor` id): an inline `Image` degrades to its alt
text (reST's only image construct, `.. image::`, is block-level, and can't
legally appear inside a paragraph) — a paragraph that is ONLY an image, or
only a link around one, is not in that category and is written back as the
`.. image::` directive it came from, carrying the link as `:target:`; a hard `LineBreak` emits a literal
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

The whole chain is now measured over the 1564-file real-world corpus rather
than a handful of shapes: **1553 of 1564 typeset** in `Strict` mode. The 11
that do not are all named — 8 are empty sources, so there is nothing to put on
a page, and 3 carry the author's own `.. raw:: latex` with sphinx-only markup
(`\sphinxsetup`, `\dimeval`) or `&` alignment in a formula docutils itself
also writes into `equation*`.

That measurement is why `Options.Strict` exists, and it found a defect upstream
(fixed in docutils v0.136.16) that the DEFAULT could not have shown. A footnote
opened with `\par\noindent` and its text followed with nothing between, so TeX
read `\noindentFirst` — a backslash takes the longest run of letters. Strict
stops there. Lenient, which is the default and the right default for a document
arriving from another format, **skips the command and takes the word with it**:
PEP 495 typeset 15 pages, exit 0, six words short, with nothing anywhere saying
so. A default that degrades rather than fails can only be measured through the
strict path, and only the strict path is a witness.

### Where the images are

docutils v0.137.0 stopped dropping images — an `<image>` had no case in the
latex writer's switch, so it rendered as nothing at all — and that turned a
silent content loss into a question this package had no way to answer. The
engine reads a figure with `os.ReadFile`, and its resolver seam
(`Options.Resolve`) serves classes, packages and `\input` files, not figures, so
a relative `\includegraphics` resolves against the PROCESS's working directory.
For a document that arrived from somewhere else that is almost never right: the
strict measure fell from 1553 to **1510**, and every one of the 43 new failures
was `includegraphics …: no such file or directory`.

`Options.BaseDir` closes it without touching the engine — each relative
reference is rewritten to sit under the directory the caller names, before the
source reaches the engine. With it, and with a placeholder written for every
reference a local file COULD satisfy (the corpus ships `.rst` files, not their
pictures), the chain is back to **1540 of 1564**, the same number the writer's
own compile sweep reaches. The 24 that remain are each accounted for:

| | |
|---|---|
| 13 | an image reference nothing local can satisfy — a remote URL, a `data:` URI, an absolute `/_static/…` path, or sphinx's own `image.*` language glob |
| 8 | an empty source: nothing to put on a page |
| 2 | the author's own `.. raw:: latex` with sphinx-only markup (`\sphinxsetup`, `\dimeval`) |
| 1 | `&` alignment in a formula docutils itself also writes into `equation*` |

Three kinds are deliberately left UNCHANGED by `BaseDir`: an absolute path (the
caller already said where), and a `http(s)` or `data:` reference, which no local
file can satisfy at any base. Rewriting those would turn "this cannot be
fetched" into "the base is wrong" — a worse error, about the wrong thing.

And the lenient default does here what it did for the control word: with no
`BaseDir` and no `Strict`, an unresolvable image costs the reader the picture and
reports nothing at all.

### Anchors, and the links that pointed at them

The round-trip probe's largest class was `Heading -> Heading`: 59 of the 1564
files came back with a different heading id. That is the symptom. The cause is
that a `.. _label:` had been dropped, and what a reader got was **a link that
goes nowhere** — which no content measure, no validity measure and no compile can
see, a dangling fragment being perfectly well-formed. So there is a probe for
exactly that (`/Users/Shared/rstcorpus/anchorprobe`): every same-document link in
the reconstruction must have something to land on.

**47 files holding 87 such links, down to 4 holding 10.** Three causes:

An anchor written between two same-level titles belongs to the SECOND section,
but docutils' section nesting makes it the LAST CHILD of the FIRST one — so
pairing a pending target with a section SIBLING never found it. Asked about that
input the reference answers `<section ids="plain-section plain">`:
`PropagateTargets` finds "the next node" in **document order** and crosses the
boundary. Walking in document order fixed 41 files' round trip (1374 → 1415).

An anchor whose next node is not a section had nowhere to go, richdoc giving no
Block but `Heading` an ID. It is now kept as its own reST source, which is what
`RawBlock` is documented for; an earlier note here said to revisit this "the day
a Paragraph grows one", and it did not need that day.

A bare target CHAINED onto one that carries a reference is not an anchor at all —
it is another name for that destination, which docutils resolves to the URL
(fixed upstream in v0.137.1). Writing it back invented an anchor the source never
had: `pep-0256`'s `.. _pythondoc:` ended up immediately before a section it never
preceded, and the next parse made it that section's anchor. **One file**, found
by set-diffing the corpus — the total had gone UP, so a total would have hidden
it — and the reason a test here checks the reconstruction is a FIXED POINT rather
than merely checking its shape.

The 10 links still dangling are two named kinds: a duplicated target name, whose
`-1`/`-3` disambiguation suffixes richdoc has no way to reproduce, and a
directive's own `:name:` option (`.. table:: :name: namedtabular`), which needs an
id on a Block that has none.

### Table cells

`Table -> Table` was then the largest class — 58 files. The coarse shape groups by
block type, which is too coarse to act on, so narrowing it to the first differing
CELL and naming the difference gave four causes instead of one. Three were worth
fixing and the fourth is a model boundary.

**A cell's escape was dropped.** PEP 624 writes `\(2)` in a cell precisely to stop
reST reading an enumerated list there. The escape was not re-emitted, so the
reconstruction read `(2)` as a list starting at 2 and put docutils' own
"Enumerated list start value not ordinal-1" INFO **into the cell**, where the
author had written `(2)`. `escapeText` already covered `*` and `|`, being inline
markers; `(`, a bullet `-` and the other seven block shapes are positional and
need `escapeBlockStart`, which a cell never got.

**A cell's own wrapping was collapsed to one line.** That corrupts nothing and
loses nothing readable — reST folds a wrap back to a space — but it re-wraps the
cell, so 45 files came back with a different tree. A grid row is now as many
source lines as its tallest cell, and column widths are measured against a cell's
widest LINE rather than its whole text.

**A multi-paragraph cell is separated by a blank line** rather than a space.
Either way the block structure is gone (`richdoc.Cell` has no `Blocks`) and either
way a consumer sees whitespace, so nothing is lost by the change — but a space
re-parses as one text and a blank line re-parses as two blocks, so only the blank
line is a fixed point. It also puts the break back into the reconstructed reST,
where a grid cell really can hold two paragraphs.

Round trip **1415 → 1449 of 1564**, and the table class from 58 files to 18.

### The probe that was too coarse to see a regression

The multi-line change made two files much worse and the round-trip probe reported
nothing, because it is BINARY: `pep-0307` and `pep-0720` already did not
round-trip, so they stayed "lossy" while gaining 1 and 26 diagnostics. A newline
inside a literal is CONTENT, not a wrap — written as a real line break, its
indentation reads as a block quote and the closing delimiter is lost, so
`Inline literal start-string without end-string` appeared **inside a cell**.

So there is a probe that counts what the reconstruction ADDS
(`/Users/Shared/rstcorpus/diagprobe`): parse the original, write it back, parse
that, and compare the `<system_message>` counts. **30 messages in 8 files before
this round, 54 after the multi-line change, 27 in 7 files with a guard that
flattens a cell whose newline sits inside verbatim content.** No file gained
diagnostics that did not already, and `pep-0624` stopped gaining them.

That trade is deliberate and it costs something: one cell cannot be half
multi-line, so a cell holding both a wrapped paragraph and a literal is flattened
whole, and one file's round trip was given up for it (1450 → 1449). A flattened
cell beats a corrupted one.

Two boundaries are left in this class and neither is a defect to fix here: 19
files whose cell holds several blocks, which `richdoc.Cell` cannot represent at
all, and 6 whose row-span the writer does not merge (its own doc comment says so).

### The reconstruction now says nothing the author did not

Ranking by the diagnostics a reconstruction ADDS rather than by tree diffs put a
different item at the top — 27 messages in 7 files, each one text docutils puts
into the document that the author never wrote — and it turned out to be four
causes, all of them cheap once named. **27 → 0.**

**A continuation at column 0.** One line-block `<line>` can span several source
lines, and it is the INDENT that says so: the reference reads

```rst
| ``__setitem__(integer | slice, integer) ->
  None``
```

as one `<line>` holding one `<literal>`. Written flush left the line block simply
ends there, so each wrapped line cost three messages — "Line block ends without a
blank line" plus the unterminated literal and emphasis the break left behind. 21
of the 27 were this, in PEP 368 alone. A docinfo field value has the same shape
(an address, a copyright notice) and the same cure.

**`.. contents::` came back as a contentless `.. topic::`.** docutils gives that
directive a `<topic>` whose only child besides the title is a `<pending>`, and
skipping the pending — which must never reach a reader — left a topic with a title
and no body, which docutils rejects. It also invented a `:name:` option out of the
implicit target the TITLE created, one `.. contents::` does not accept. The
options come back out of the pending's own details, and the tree distinguishes the
two spellings that both mean "no backlinks": `:backlinks: none` leaves
`backlinks: None` and a bare `:backlinks:` leaves `backlinks: ''`. The `contents`
and `local` CLASSES are deliberately not written back, docutils deriving them
itself.

**A commented-out directive came back switched ON.** `rawChildSource` had no
`TagComment` case, so the fallback returned the comment's text without the `..`
that makes it one: sphinx's own index page comments out a whole admonition
(`.. .. admonition:: …`), and nested inside a container the reconstruction wrote it
back as a real directive with no body. At the top level a dedicated `rawComment`
was used and the same document was fine, which is why only the nested spelling
showed it. A census of that switch against the `raw*` builders that exist added
`topic`, `sidebar`, `rubric`, `container` and `raw` at the same time — each one a
construct that, nested, lost the thing that said what it was.

Round trip **1449 → 1458 of 1564** as a side effect, source-vs-output 1314 → 1321,
9 fixed and nothing newly lossy.

Two notes on the measuring rather than the fixing. The first set-diff of this
round compared against a MID-round baseline and reported a regression that was
really the trade made deliberately in the previous one — a baseline has to be the
state that shipped. And `check-subjects.sh` now prints what the probes are
actually measuring, because a `git pull --ff-only -q` reported success and did
nothing: every number in the corpus directory came from a worktree two commits
behind, and the check found a second stale subject on its first run.

The coverage floor earned its keep on the same change. The census added five
cases to `rawChildSource`, and the floor failed because one of them was never
reached: docutils refuses a topic or a sidebar nested in a body element at all
(`The "topic" directive may not be used within topics or body elements.`), so
that case was dead code. The corpus had nothing to say either way — it contains
no such document, because no such document parses. It was removed rather than
covered by a test of something invalid, and the remaining four are witnessed by a
nested-construct case.

### Footnotes

richdoc carries a note BY VALUE: the body is inlined at every reference. That one
fact is behind four defects, found while narrowing fidelity's largest attribute
class, and one of them was a **crash**.

**A note that cites itself was a stack overflow.** `.. [1] A note citing itself
[1]_.` is reST docutils reads without complaint — one `<footnote>` with two
backrefs — and inlining a body that contains its own reference recursed until the
goroutine stack ran out. Nothing in the 1564-file corpus does it, which is why no
sweep had found it. A reference back into a note already being expanded now
degrades to the verbatim marker, which is what an unresolvable reference already
did.

**A note cited inside a note was never defined.** `writeFootnoteDefs` ranged over
its accumulator while writing a body could APPEND to it, and `range` fixes the
length at entry, so those notes were numbered at the reference site and never
defined.

**A note cited twice became two notes.** Three references arrive as three Footnote
values with the same blocks, and appending each produced three definitions with
three numbers — a document that gained two footnotes it never had, in 117 files.
The dedup key is the rendered body, which is all the model offers, and its blind
spot is stated: two genuinely distinct notes whose bodies render identically
become one. An EMPTY body is not a key, because PEP 653 writes nine notes whose
content docutils cannot attach, and keyed on the body all nine merged into one.

**A reference inside a rebuilt block consumed its definition.** A whole family of
constructs is rebuilt as reST source, and inside one of those a reference is
written back verbatim — the converter never sees it. Counting it as a reference
that consumes its definition dropped the definition and left the marker: PEP 302
ended with references `[10]_` and `[11]_` pointing at nothing and two of its nine
notes gone. 14 files lost 42 definitions this way, now 2 files and 4.

That last fix then created a collision, and it is worth saying how it was caught.
A verbatim definition keeps the author's label while the counter picks 1..N, and
nothing stopped the two from choosing the same one: PEP 550 came out with `.. [9]`
and `.. [10]` **twice**. The round trip had gone DOWN by three files while the
definition count went UP — a total would have read as a net gain, and only the
set-diff showed it. The counter now skips a label a verbatim definition holds.

Two measures were built to see any of this. `destprobe` asks whether a reference
still points at the **same place**, which took three attempts to make honest:
comparing the fragments reported 20 movers whose only difference was which of a
section's several ids had been chosen, and describing a bare `<target>` as itself
reported 41 whose id had merely landed on the other side of docutils'
`PropagateTargets` rule. Comparing what is THERE — a section's own title — leaves
3. `fnprobe` counts note definitions on each side, which `destprobe` cannot see at
all, counting `<reference>` and not `<footnote_reference>`.

And a last one found by a test rather than looked for: a figure's CAPTION was
rendered with `doctree.AsText`, so every link, literal, emphasis and role in it was
lost, and a note reference came out as its bare label — `Cites 1.` for
`Cites [1]_.`, which also stopped the definition from being emitted.

| | before | after |
|---|---|---|
| note definitions lost | 14 files / 42 | **2 files / 4** |
| references not pointing where they did | 20 files (34 gone, 3 moved) | **10 files (16 gone, 3 moved)** |
| round-trip to the same tree | 1458 / 1564 | **1460 / 1564** |
| source-vs-output equivalence | 1321 / 1564 | **1323 / 1564** |
| diagnostics the reconstruction adds | 0 | 0 |

### A citation is not a footnote

reST has both and they are different things. A footnote is a note at the foot of
the page; a CITATION names a bibliographic entry, and **its key is what the reader
sees** — `[CIT2002]`. Converting it as a footnote inlined the body and threw the
key away, so the reconstruction said `[1]_` and `.. [1]` where the author wrote
`[CIT2002]_` and `.. [CIT2002]`: a number in place of a key, in 15 of the 1564
files.

`richdoc.CrossRef` with `RefCite` is what models it, and the write side has always
emitted `[target]_` for that — only the parse side was missing. The target is the
reference's own TEXT, not its refname: docutils normalises a name to lower case
for matching, so the refname is `cit2002` while the document says `CIT2002`. And a
citation is never "consumed", because its reference now carries only the key, so
the definition is the only place the body can live.

Source-vs-output equivalence **1323 → 1339 of 1564**.

### The doctest block that is not worth keeping

reST has a third spelling for code: a paragraph opening with `>>>`, needing
neither `::` nor indentation. It comes out as a `::` literal block, and after this
round that is a DECIDED loss rather than an oversight — it renders the same, and it
costs something real, since a doctest is collected by test runners and a literal
block is not. 9 files.

Two ways to keep it were tried and both cost more:

| attempt | cost |
|---|---|
| the writer keys on the text starting with `>>>` | 110 literal blocks that SHOW a session became doctest blocks — twelve times as many as there are real ones. Fidelity 1323 → **1237** |
| the parse side marks the language `pycon` | collides with the authors' own `.. code-block:: pycon`, which arrives indistinguishable — a sentinel that means two things |
| a `RawBlock`, this package's usual answer | the code VANISHES for every consumer that is not reST |

A spelling change is the smallest of the three, and it is now written down where
the next reader will look for it instead of being tried again.

`<title_reference>` → `<emphasis>` (16 files) stays for the same kind of reason:
italics is the nearest rendering every format has, and preserving the reST
spelling as raw source would make the construct invisible to all of them.

### The null separator a role needed

reST attaches markup to an adjacent word with an ESCAPED SPACE, its null
separator. PEP 410 writes

```rst
10\ :sup:`-9`
```

and without that separator the writer produced `10:sup:`-9``, which docutils does
not read as a role at all: it is plain text, and **the superscript disappears**.
The separator logic knew the characters that start emphasis, a literal, a
reference and a substitution — `*`, `` ` ``, `[`, `|`, `_` — and not `:`, which is
how a role begins.

### The same continuation indent, a third time

A directive OPTION value can run to several lines. An image's `:alt:` is the
common case — PEP 495 has a two-line one — and a field body's continuation has to
be indented or the field ends there: the second line became a NEW option, so the
alt text was truncated at the break and the rest read as an unknown option.

This is the third home of one defect, after the line block and the docinfo field
list. It is fixed in `rawDirectiveSource`, the one place all two dozen
option-building sites funnel through, rather than at whichever one the next corpus
file happens to reach.

Round trip **1460 → 1471 of 1564**, equivalence 1339 → 1343, nothing newly lossy.

### Where an image's options have to be fixed, and it is not here

A standalone `.. image::` keeps its URI, its alt text and its `:target:`, and
loses `:align:`, `:width:`, `:height:`, `:scale:` and `:class:`. Measured over the
corpus: **43 of the 79 standalone images carry at least one of those, in 18
files** — more than half.

`richdoc.Image` has `URL`, `Alt` and `Title`, and no field for any of them, so
this package cannot carry them however it is written. The two workarounds both
cost more than the loss:

- reconstructing such an image as raw reST preserves every option and makes the
  **image itself vanish** for every consumer that is not reST — the same trade
  that decided the doctest block, and the image is the more valuable of the two.
- inventing a convention inside `Alt` or `Title` puts markup where a renderer
  expects text.

So the fix belongs in `richdoc`, as fields on `Image`, where every converter would
get it at once. Recorded here sized and located rather than approximated.

**Since resolved**: richdoc v0.4.0 added those fields and this converter carries
them — see "Carrying them" below. The paragraph above is kept as the measurement
that asked for them.

### What the equivalence number was counting

"1343 equivalent once the NAMED boundaries are removed" was not true to its own
wording: two boundaries named in this README were still being counted as
divergence. Subtracting them — in the ONE normaliser applied to both sides — gives
**1389 of 1564, and 175 genuinely unexplained**.

**A `class` richdoc has nowhere to put.** richdoc carries no presentational
attribute on any node: no `Class`, `Style`, `Width` or `Height` anywhere. A reST
`:class:` option, a `.. class::` directive and sphinx's `.. rst-class::` therefore
reach a construct that converts to a native richdoc node and are lost however this
package is written. Measured: **187 author-written class attributes in 57 files**
(`/Users/Shared/rstcorpus/classprobe`), on paragraphs (63), tables (83), images
(20) and a scattering of block quotes, literals and lists.

The subtraction is deliberately narrow. A class on an admonition, container,
rubric, topic or figure is NOT subtracted, because those are rebuilt as reST source
and do write `:class:` back — so a regression there still reports. Shown: making an
admonition drop its class took the number 1389 → 1386, and restoring returned it.

**`<title_reference>` → `<emphasis>`** is normalised on both sides, the text inside
still comparing.

Together with the image options this is one decision, not three: whether richdoc
should carry presentational attributes at all. **187 class attributes in 57 files
and 43 image options in 18** is what it is worth, and it is a question about the
shared model rather than about this converter.

### A note on reading the probe's own output

Twice in one round an LCS-aligned insertion was read as "the output gained a
block". It had not: the aligner places an insertion where the alignment is
cheapest, and that position is its choice rather than the document's. The diff is
sound; attributing a `+` line to the construct printed beside it is not. Checking
the source directly is what settled both.

### Carrying them (richdoc v0.4.0)

richdoc grew the fields, so this converter now carries them. `Classes []string` on
`Paragraph`, `List`, `BlockQuote`, `Table`, `CodeBlock`, `Code` and `Image`;
`Width`, `Height`, `Scale` and `Align` on `Image`.

The writer uses the CONTENT form of `.. class::` — the directive with the block
indented under it — and not the bare form. Asked about both, the reference applies a
bare `.. class::` to the next element through a transform (`misc.ClassAttribute`)
that this project's parser does not run: the bare form leaves a `<pending>` and the
paragraph gets no class at all, while the content form gives
`<paragraph class="foo bar">` in both parsers.

Three things had to be decided rather than assumed, and each was measured.

**A block quote is not wrapped.** It is written by INDENTING its content, and
indenting that again under `.. class::` loses the quote: the body lands at six
spaces and reparses as one PARAGRAPH carrying the class. Its own directive carries
it instead — and every block-quote class in the corpus is one of docutils' three
(6 `epigraph`, 1 `pull-quote`).

**A derived class never enters the model.** `colwidths-given` comes from
`.. table::`'s `:widths:` option and `colwidths-auto` from its absence — a plain
grid table carries neither — so 70 of the corpus's 83 table classes are docutils'
own working-out. Filtering them in the WRITER was the first attempt and cost 13
files' round trip: the model held `colwidths-given`, the reconstruction omitted it,
and the next parse had no class. A filter on the way IN is a fixed point; one on
the way out is not.

**A literal block's language travels with it.** The class work uncovered a loss
next to it: `rawChildSource` wrote a bare `::` for every literal block, so a
`.. code:: python` nested inside a list item, an admonition or a definition came
back unlabelled — 16 files, invisible while the class was being subtracted from the
comparison.

| | before | after |
|---|---|---|
| source-vs-output equivalence | 1358 / 1564 | **1395 / 1564** |
| round-trip to the same tree | 1471 / 1564 | 1471 / 1564 |
| diagnostics the reconstruction adds | 0 | 0 |

What still has nowhere to go: an INLINE literal's class, which comes from a custom
role and would need the role's own definition to write back; a class on a hyperlink
target, which this converter drops as bookkeeping; and the structural table parts
(`tgroup`, `entry`, `row`) richdoc models without an identity of their own.


### The image a container swallowed

`rawChildSource` — the fallback that rebuilds reST for a child of a block this
converter cannot map natively — had no case for a figure OR an image. Either one
nested in a `.. container::`, an admonition or a definition fell to its `AsText`
fallback, and an `<image>` has no text: the picture, every option and the
`:target:` link all disappeared, leaving at most a caption behind.

sphinx's own index page is the shape that showed it. Three logos sit in a
`.. container::`, each a figure with a `:target:`, and all three links went
missing — which the round trip could not see, because a tree with no image in it
rebuilds to a source with no image in it, consistently. `destprobe`, which asks
whether a reference still points at the same PLACE, is what named it.

Measured population: **6 figures (3 of them with a `:target:`) and 3 standalone
images, across 4 files.**

Three details decided the shape of the fix, and the reference settled each one.

The option list is now one function (`imageOptionLines`) serving both the figure
and the image case, emitting in `images.Image.option_spec`'s own order — `alt`,
`height`, `width`, `scale`, `align`, `target`, `loading`, `class`, `name`, read
from docutils rather than guessed, with `:target:` in its place among them instead
of appended after `:name:`.

Reading that list is what found `:align:`. The reference puts a FIGURE's align on
the `<figure>` element and a lone IMAGE's on the `<image>` itself, so the option
list inherited from `rawFigure` — which reads the figure's align from the figure —
had no `align` in it, and a standalone image's alignment would have been dropped
by the new path. It is in the shared list now; a figure's image never carries one,
so the figure case is unaffected.

And a standalone image carrying a `:target:` is not an `<image>` at all: the parser
wraps it in a `<reference>`, so `rawChildSource` needs a `TagReference` case too,
falling back to `inlineSourceOf` when the reference wraps no image.

| | before | after |
|---|---|---|
| references that no longer point where they did (`destprobe`) | 7 files | **6 files** |
| source-vs-output equivalence (`fidprobe`) | 1395 / 1564 | **1399 / 1564** |
| round-trip to the same tree (`rtprobe`) | 1472 / 1564 | 1472 / 1564 |
| diagnostics the reconstruction adds | 0 | 0 |

The round trip is unchanged and its 92-file lossy SET is identical, checked by
set-diff rather than by the total: a total that holds still can hide one file
gained against one file lost.

#### Two boundaries measured and declined

The same investigation found two more causes behind `destprobe`'s list, and both
were measured and then left alone, which is the part worth recording.

- **A parsed literal's inline markup.** `.. parsed-literal::` keeps its markup
  live, so a reference inside one is a real link; this converter writes the block
  as a plain literal and the link becomes text. Population: **3 blocks in 3
  files, out of 10613 literal blocks in the corpus** — a `richdoc.CodeBlock`
  holds a string, so carrying it would mean a new node, for three blocks.
- **An escaped `\::`.** Two files escape the colon that would otherwise open a
  literal block. The reference AGREES with what this parser does with them, so
  there is no defect here to fix: one observable consequence, and it is correct.
  Recorded so the next reading of the probe does not spend the afternoon on it
  again.


### The sentence no author wrote

A table cell is flattened to INLINES — `richdoc.Cell` has no `Blocks` — so every
child of an `<entry>` goes through `cellBlockInlines`, and a `<system_message>` in
one reached its `default:` branch and came back as its own TEXT:

    +-------------------------+------+-----------------------------------------------------+
    | Easy for humans to edit | yes  | Unexpected possible title overline or transition.   |
    |                         |      | Treating it as ordinary text because it's so short. |
    |                         |      |                                                     |
    |                         |      | ??                                                  |
    +-------------------------+------+-----------------------------------------------------+

That is PEP 518's comparison table, where the author wrote `??` and the reader gets
the parser's remark about it. `convertBlockNode`'s `TagSystemMessage` case — the one
that drops diagnostics by default — is never consulted on the cell path, the same
blind spot `rawSource` had (hence `withoutDiagnostics`) in a third place.

Measured over the corpus: **9 files, 29 occurrences** — 28 of them that one INFO
about a short line of punctuation, 1 a `:pep:` role whose argument was not a number.

What the reference says about dropping them. docutils' PARSER attaches the message
whatever its level: asked for the doctree of a simple table whose cell holds `??`,
`rst.Parser.parse` alone gives a `<system_message level="1">` and the PUBLISHED
document has none. The removal is a TRANSFORM, `universal.FilterMessages`, run by
the reader against `report_level`. So the message really is in the tree this
converter reads, and dropping it is what the reference does too — at the default
report level, which is what `KeepDiagnostics: false` means here. The same transform
corroborates the older choice two sections down: when it removes a message it
"convert[s] `<problematic>` nodes referencing removed messages to `<Text>` nodes",
which is exactly what this package does with a construct docutils refused.

The ERROR half of the policy holds inside a cell as well: at level 3 and above
docutils REFUSED to build the construct and quotes the author's source in a
`<literal_block>`, so a malformed table nested in a `.. list-table::` cell keeps its
own lines as `Code` and only the complaint goes.

| | before | after |
|---|---|---|
| a parser complaint written into the document as text (`leakprobe`) | 9 files, 29 occurrences | **0** |
| round-trip to the same tree (`rtprobe`) | 1472 / 1564 | **1476 / 1564** |
| source-vs-output equivalence (`fidprobe`) | 1399 / 1564 | **1403 / 1564** |
| diagnostics the reconstruction adds (`diagprobe`) | 0 | 0 |

#### Why `diagprobe` read zero for this

`diagprobe` counts `<system_message>` elements in the reparse, and **a message that
became a paragraph is no longer a message**. It was answering its own question
correctly and could not see this one; what found it was `fidprobe`, which compares
the two trees and had the sentence sitting in its `text -> text` bucket all along.

`leakprobe` asks the question directly: does the TEXT of a source `<system_message>`
appear in the output, and not already in the source (an author may quote docutils)?
Its first version searched the written reST and found **1 file**, because a leaked
message lands in a grid-table cell where the `|` borders cut every line of it. Over
the reparsed output's TEXT it finds 9. A probe gets its own control: it has to fire
on a file whose defect is already known.


### The backslash that became two

`:math:` was written through `rawRole`, which escapes a backslash because a role's
content is escape-processed — in general. docutils' inline parser replaces every
escaping backslash with a NUL and each role decides what to do with them, and only
**three** restore them: `raw`, `code` and `math`, via
`nodes.unescape(text, True)` — *"return a string with nulls … restored to
backslashes"*. Read in `parsers/rst/roles.py` rather than inferred, and the two
answers are what settle it:

| asked | the reference answers |
|---|---|
| ``:literal:`\emptyset` `` | `<literal>emptyset` |
| ``:math:`\emptyset` `` | `<math>\emptyset` |

So the escape doubled every TeX command that passed through: ``:math:`\emptyset` ``
came back as ``:math:`\\emptyset` ``, which is TeX for a line break followed by a
word. **16 formulas in 5 of the 14 corpus files that hold one.**

The fix is to write the formula VERBATIM, backtick included. Escaping the backtick
would be wrong for the same reason — the added backslash survives the restore, so
``a\`b`` came back ``a\\`b`` — and leaving it alone round-trips, because the
backslash an author already wrote in front of it is what keeps the role from ending
there. Two shapes cannot be written at all, here or in docutils: a formula holding a
BARE backtick, and one ENDING in a lone backslash; either way the closing backtick
is consumed, neither is valid TeX, and the corpus has none.

| | before | after |
|---|---|---|
| formulas that come back CHANGED (`mathprobe`) | 5 files, 16 formulas | **0** |
| round-trip to the same tree (`rtprobe`) | 1476 / 1564 | **1480 / 1564** |
| source-vs-output equivalence (`fidprobe`) | 1403 / 1564 | **1406 / 1564** |

`mathprobe` is new, and it needed its own correction first: parsing the two sides
with plain `docrst.Parse` while the converter parses with three report flags OFF
made sphinx's `.. math:: E = hv` with `:label:` an ERROR on the source side and a
formula on the output side, and the probe called two files changed that were
byte-for-byte right. A judge has to read the document the same way its subject does.


## Round-trip

`Parse(Write(Parse(src)))` reproduces `Parse(src)`'s tree for the natively
mapped constructs above (verified in `roundtrip_test.go` against a corpus
covering each one, using `docutils/rst.Parse` itself as the check — see
"Why delegate parsing" above). The `RawBlock`/`RawInline` fallback
constructs are covered separately in `unit_test.go` instead, since by design
they resynthesize reST rather than preserve a byte-exact slice.

### What a trip through richdoc cannot preserve

Two measurements over the 1564-file real-world corpus in
`/Users/Shared/rstcorpus`, both outside this package:

| measure | what it asks | state |
|---|---|---|
| `rtprobe` | does `Parse(Write(d))` give back `d`? | 1480 of 1564 |
| `fidprobe` | do the SOURCE and the OUTPUT parse to the same doctree? | 1406 of 1564 equivalent once the boundaries below are removed |

`fidprobe` is the stricter and the more useful of the two: it sits outside both
steps, so it sees a loss that happens on the way IN — which a round trip
structurally cannot, because the damage is already in the tree it starts from.

What the remaining difference is made of, established case by case rather than
assumed. Each of these is a limit of `richdoc`'s own model, not a defect here,
and each is checked NARROWLY in the probe so a real loss cannot hide behind it:

- **a footnote or citation LABEL.** `richdoc.Footnote` is a body placed at its
  reference and has no label field, so `[Aho86]_` comes back `[1]_` and a
  citation comes back as a footnote. The explicit spelling is used rather than
  auto (`[#]_`) because that was measured: 948 against 865.
- **an enumerated list's STYLE.** `Ordered` and `Start` are recorded; the
  enumerator's kind and punctuation are not, so `a)` comes back `1.`.
- **a table's column WIDTHS.** The writer lays the table out itself, so every
  column returns two wider. The column count is preserved.
- ~~**a `:class:`** on a paragraph, an image or anything else~~ — carried since
  richdoc v0.4.0; see "Carrying them" above. What remains uncarried is an INLINE
  literal's class and a class on a hyperlink target.
- **`:number-lines:`** on a code block: no flag for it. The generated numbers are
  dropped rather than folded into the code, which is the part that matters.
- **a `.. code::` with no language**, which is `CodeBlock{Language: ""}` exactly
  like a plain `::` block.
- **a title reference** (`` `text` ``, the default role), which maps to
  `Emph` — what docutils renders it as, and the only inline richdoc has for it.
- **a target's NAME**, where the target is resolved into its references and
  dropped: an anonymous or named target becomes an embedded URI. The probe
  verifies the uri against the source's own target rather than tolerating any.
- **a `<problematic>`'s markup**: the text of a construct docutils refused
  passes through as text, so other writers render something rather than dropping
  a `RawInline` they do not know.

## Testing

`go test ./...`. `go vet ./...` and `gofmt -l .` are clean; CI enforces a
94% coverage floor rather than 100% — see the comment in
`.github/workflows/ci.yml` for why this package's coverage bar differs from
its sibling converters'.

## License

BSD-3-Clause. See [LICENSE](LICENSE).
