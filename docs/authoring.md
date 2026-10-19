# Authoring a project

An lfm project is a folder with two required files and whatever assets the page uses.

```text
my-page/
  lfm.json            page facts and build settings
  index.html.tmpl     the page (Go html/template)
  partials/           optional template pieces
  data/               optional JSON files, merged into .data
  assets/             styles, scripts, images, fonts (a convention, not a rule)
  research/           human-voices.md, competitor-analysis.md (never compiled)
  concept.md          the design decisions the page is built from (never compiled)
  deliverables/book/  optional: a book the page sells (concept.md, chapters/, images/, fonts/, book.css)
  deliverables/printable/ optional: a printable (concept.md, printable.html.tmpl, data/)
  deliverables/spreadsheet/ optional: a spreadsheet template (concept.md, workbook.json)
  deliverables/document/  optional: editable documents (concept.md, <language>/<document>.md, fonts/)
  deliverables/deck/      optional: slide decks (concept.md, <language>/<deck>.md, fonts/)
  deliverables/bundle/    optional: listing.md of two or more deliverables sold together
  output/             the compiled page; replaced on every successful compile
  distribution/       the book's store packages, when it has a listing.md; never deployed
```

Sources may live anywhere inside the folder, except `output/` and `distribution/`. Symlinks may not leave it.

## lfm.json

```json
{
  "schemaVersion": 1,
  "url": "https://example.com/my-page/",
  "language": "en",
  "title": "My page",
  "description": "One sentence: who this page is for and what they get from it.",
  "socialImage": { "src": "assets/images/share.png", "alt": "What the image shows" },
  "seller": { "name": "Baker Books", "imprint": "Baker Books, Example Street 1, 12345 City" },
  "urls": { "checkout": "https://buy.stripe.com/…" },
  "data": { "offer": { "price": 29 } },
  "build": {
    "browserData": ["data.offer"],
    "assets": { "overrides": { "assets/images/hero.webp": "file" } },
    "validation": { "rules": { "design.mono": "off" } }
  }
}
```

Every field, with its type, allowed values, default and meaning, is in the [lfm.json reference](config.md),
generated from the same table the compiler checks against; `lfm help config [field]` prints it in the
terminal, and every config finding quotes the field it is about.

Reserved fields use exact casing; unknown keys, wrong types and values outside a field's list are errors
that name the field's meaning and allowed values. Custom values belong under `data`.
Each `data/<name>.json` becomes `.data.<name>`; a name already in `lfm.json` is an error.

## The template

`index.html.tmpl` is Go [`html/template`](https://pkg.go.dev/html/template): values are escaped for their
context automatically. Every value the template uses must exist; missing or wrongly shaped values, unused
config fields and invalid paths are reported with their source location before anything renders.

```gotemplate
<title>{{.title}}</title>
<a href="{{.urls.checkout}}">Buy</a>
{{range .data.faq}}<details><summary>{{.question}}</summary>{{.answer}}</details>{{end}}
{{template "partials/footer.html.tmpl" .}}
```

The compiler emits the JSON-LD block from `jsonld` itself; do not write one by hand. String values may select
config values: `"name": "{{.title}}"`. A whole-value reference keeps numbers, booleans, arrays and objects.

## Styles and scripts

```gotemplate
{{style "assets/styles/main.css"}}
{{script "assets/scripts/main.js"}}
{{script "some-installed-package"}}
```

Each helper takes one path or package name and must stand alone where an HTML element could. Local files
resolve relative to the calling template first, then the project root; other names resolve from
`node_modules/` (install packages yourself; lfm never downloads anything). esbuild bundles each graph:
TypeScript, JSX, CSS imports, nesting and `.module.css` work. Plain `<script>` and `<link>` tags work too.

Imports of images and fonts from JS or CSS (`import url from "../images/a.webp"`, `url(...)`) become emitted
assets. Dynamically built URLs and `fetch()` targets are not rewritten; reference assets statically.

Scripts move to the end of the body in source order; styles stay in the head. Compatible scripts share one
module graph and compatible styles one stylesheet. Use `data-lfm-classic` on a `<script>` that must keep
standalone browser globals (UMD).

## Images, fonts and media

Reference local files from HTML or CSS like any web page. The compiler inspects them without decoding pixels:

- `<img>` sizes are filled in when the file and the layout leave no doubt, which prevents layout shift.
- Open Graph images get `og:image:width` and `og:image:height` from the real file.
- Oversized images and fonts, missing `font-display` and mismatched preloads are reported.
- A missing favicon gets a small generated one, so browsers never request `/favicon.ico`.

Social preview images, media and linked documents always become real files with absolute URLs.

## Research

`research/` holds the durable inputs of the design process: `human-voices.md` (verbatim posts with source
links) and `competitor-analysis.md`. `lfm skills research` prints the process and the exact format. The
files are never part of the page, but every compile parses them, reports format and substance problems
(`research.*` rules) and summarizes them: voices, platforms, peers, posting dates.

## Concept

`concept.md` is the handover from design to implementation: fixed sections in a fixed order (buyer,
decision path, mechanism, offer, axes, directions, spine, palette, type, form, imagery, motion, copy), free
words inside them, every decision citing research IDs such as `V012` or `P003`. `lfm skills design` prints
the process and the format. Every compile checks the file (`concept.*` rules) against the research; once the
starter page is replaced, the page is held to its palette, typefaces and planned imagery
(`concept.fidelity`), including the Copy section's exact `Use: "…"` and `Avoid: "…"` phrases. `Open:
<question>` lines list what only the owner can answer (`concept.open`).

## Deliverables: a book

A project can ship a book alongside the page. Its folder is `deliverables/book/`, and `lfm skills book`
prints the whole process: a book research batch, the concept, one chapter at a time, the finished files,
delivery after payment and the store packages.

```text
deliverables/book/
  concept.md          reader, promise (with before and after), big idea, objections, hooks, voice,
                      devices, cases, chapters, imagery, look
  chapters/01-name.md one Markdown file per planned chapter, starting with "# <planned title>"
  images/             cover (1600×2560 or larger) and the images the concept plans
  fonts/              WOFF2 files for the typefaces the concept names
  book.css            optional: @font-face rules and refinements of lfm's book styles
  listing.md          store texts: descriptions, back cover, author, keywords, tags, categories, prices, AI use
```

`lfm.json` holds the facts that are not creative decisions:

```json
"deliverables": {"book": {"author": "Ada Baker", "path": "k3x9m2q7w8e4r5t6y1u0", "offer": "book",
  "paper": "cream", "isbn": {"ebook": "978-…", "paperback": "978-…"}}}
```

`path` is the unlisted folder the files are served from: at least 16 lowercase letters, digits or single
hyphens, and the only lock on them. A compile without it prints a fresh one. `offer` names the entry in
`data.commerce.offers` that sells the book; `formats` defaults to `["epub", "pdf"]`, `language` to the
page's. The concept is checked like the page's (`book.concept`, `book.evidence`, `book.open`), every written
chapter against its row (`book.chapter`) and for how it reads
(`book.copy`): the stock phrases and habits of generated prose, from corrective framing ("It isn't X.
It's Y.") and announcing the chapter to recap endings, bold labels and pet words, and the estimate of
lfm's AI-text detector. `book.value` holds each chapter to what readers of practical books ask for: an
opening inside a moment rather than a generalization, something to use within the first few hundred
words and none of the long stretches without it, a source a reader can check, names and numbers rather
than generic advice, and no point repeated from another chapter. The language-specific checks (phrase lists,
readability formulas, the detector's habits) cover English, German, French, Spanish, Italian, Portuguese,
Dutch and Polish; other languages get the structural and statistical checks. The book is built once every chapter exists; until then its download page tells buyers that
their copy is being written, so the page can sell it first.

The copyright page prints `seller.name` as the publisher and `seller.imprint` (the lines of a German
Impressum, for example). `paper` (`white` or `cream`, which sets the paperback's spine) and the ISBNs are
optional; an ISBN is checked against its check
digit and printed on the copyright page, and the paperback's draws its barcode on the back cover.
`listing.md` is checked against the strictest store's limits (`book.listing`) and rendered into every
channel's format; see [compiling](compiling.md#store-packages).

## Deliverables: a printable

A printable is a planner, tracker, game, puzzle pack or worksheet bundle: fixed-size pages a buyer prints
at home or opens in a note app. `lfm skills printable` prints the process.

```text
deliverables/printable/
  concept.md            buyer, promise, Contents (part, exact pages, job), Editions (sizes, variants), look
  printable.html.tmpl   Go template rendering every page of one edition
  printable.css         the look; lfm sets the page size, the 6.35 mm safe margin, fonts and accent
  data/*.json           word lists, items, questions, available as .Data.<name>
  images/  fonts/       own art and embeddable fonts
  listing.md            store texts: title, description, tags, price, AI use
```

`lfm.json` holds `"deliverables": {"printable": {"path": "…", "offer": "…", "version": "…"}}`; the guide and
licence name `seller.name`.
Editions multiply the sizes (`a4`, `letter`, `a5`, `half-letter`, and `tablet` at 3:4 for note apps) by
every variant line, such as `Ink: color, ink-saver`; the template sees `.Edition`, and `<html>` carries the
size and values as classes. Each page is a `<section class="page" data-part="…">` with an optional
`data-role` (cover, guide, contents, puzzle, key, cut, certificate, extra). lfm fills `.page-number`,
`.page-count` and `.page-ref[data-ref]` after rendering, and offers seeded generators that verify
themselves: `calendar` (localized, ISO weeks), `bingo` (unique, balanced cards and the median first win),
`wordsearch` (each word exactly once), `sudoku` (exactly one solution) and `maze` (perfect, with its
solution as SVG).

Every compile renders every edition and holds the pages to the plan (`printable.pages`): page counts per
part, the cover first, a guide page, a key for puzzles, unique ids, every internal link and image. A
release prints each edition, checks every page for overflow and content in the safe margin and the PDF
against the rendered page count (`printable.layout`), and renders the listing images. `lfm serve` shows
every edition as HTML at `/<path>/preview/<edition>.html`; previews are never published.

## Deliverables: a spreadsheet

A spreadsheet is a budget, bookkeeping template, calculator or tracker for Excel and Google Sheets.
`lfm skills spreadsheet` prints the process.

```text
deliverables/spreadsheet/
  concept.md            buyer, promise, Tabs (sheet key, job), look
  workbook.json         editions, words, sheets, datasets and expected values
  listing.md            store texts: title, description, tags, price, AI use
```

`lfm.json` holds `"deliverables": {"spreadsheet": {"path": "…", "version": "…", "sheets": {…}}}`.
`workbook.json` describes the sheets by kind: `guide` (lines of text), `form` (labelled inputs and
results), `table` (typed columns, the rows it holds, seed rows) and `report` (cells at positions,
charts). Editions pick a language and currency; any text is one string or one per language. Formulas
name things, never cells: `[log.amount]` a table column, `[.amount]` the same row, `[income]` a named
cell, `[@needs]` a word in the edition's language. Only functions that Excel, Google Sheets and
LibreOffice compute alike are accepted.

Every compile lays out every edition, computes every formula with the blank template and every dataset,
and holds the results to `expect` (`spreadsheet.formulas`): errors, cycles and expected values each name
the cell and the formula. `lfm serve` shows every edition computed with the example at
`/<path>/preview/<edition>.html`; previews are never published. Only the owner can create the Google
Sheets masters buyers copy: upload each file, save it as Google Sheets, share it as viewable by link, and
put the address into `deliverables.spreadsheet.sheets`.

## Deliverables: editable documents

A document set is letters, invoices, minutes, programs, binders, contracts or any document buyers fill in
Word, Google Docs, Pages or LibreOffice. `lfm skills document` prints the process.

```text
deliverables/document/
  concept.md            buyer, promise, Documents (file name, job), Look (theme, accent, fonts)
  en/invoice.md         one Markdown file per document and language
  fonts/                the theme's Google Fonts families as static .ttf files, with OFL.txt
  images/               logos and pictures the documents place
  listing.md            store texts: title, description, tags, price, AI use
```

`lfm.json` holds `"deliverables": {"document": {"path": "…", "version": "…"}}`; `papers` overrides the
paper sizes (US Letter and A4 for `en`, A4 elsewhere) and `docs` names optional Google Docs masters.
The concept's Look picks one of the themes (classic, editorial, minimal, modern, technical, warm), which
the compiler turns into Word's built-in styles: sizes on a modular scale, leading, spacing, margins from
the line length, table rules and the accent. Documents are Markdown with front matter (`title`, `pages`,
`kind`, `cover`, `header`, `footer`) and a few directives: `::: title`, `::: columns` (separated by
`+++`), `::: form`, `::: signature` and `::: center`. `==text==` marks a field the buyer replaces;
quotes, dashes and non-breaking spaces follow the language.

Every compile checks the fonts (static, embeddable, licensed, drawing every character), every document
against the format, the theme's contrast and line length on every paper, and the sample content for
filler and emoji. `lfm serve` shows every edition at `/<path>/preview/<edition>.html`.

## Deliverables: slide decks

A deck set is training briefings, workshops, courses or quiz games buyers present from PowerPoint,
Keynote or Google Slides. `lfm skills deck` prints the process.

```text
deliverables/deck/
  concept.md            buyer, promise, Decks (file name, job), Look (theme, accent, fonts)
  en/briefing.md        one Markdown file per deck and language
  fonts/                the theme's Google Fonts families as static .ttf files, with OFL.txt
  images/               pictures the slides show
  listing.md            store texts: title, description, tags, price, AI use
```

`lfm.json` holds `"deliverables": {"deck": {"path": "…", "version": "…"}}`; `papers` sets the paper the
speaker notes and quiz sheets print on (US Letter and A4 for `en`, A4 elsewhere) and `slides` names
optional Google Slides masters. The Look picks the same themes documents use, so a deck and its handouts
match. A deck is Markdown with front matter (`title`, `minutes`, `slides`); `---` starts a slide, the first
`#` heading is the cover, later `#` headings open sections and `##` titles a slide. `::: columns`, `:::
notes` and `::: title` are its directives; a checklist with `- [x]` answers is a quiz question.

Every compile measures each slide's text with the fonts against its box, and checks titles, word and
point counts, speaker notes against `minutes`, quiz answers, pictures for a 1080p projector, translations
slide for slide, and sample content. `lfm serve` shows every language at `/<path>/preview/<language>.html`.

## Selling: page, shops or a bundle

Each deliverable says how it sells with `sell` in its `lfm.json` entry:

- `page` (the default): the page's checkout (a Stripe Payment Link) redirects buyers to an unlisted
  download page lfm publishes at the deliverable's `path`.
- `shops`: shops such as Etsy, Gumroad, Payhip or Amazon deliver the files. No download page is published
  and no `path` is needed; the store packages in `distribution/` hold everything to upload. Once a
  listing is live, its address goes into `urls` and the page's offer points at it, so the buy button
  opens the shop. lfm warns when a shop-sold deliverable's offer links to a checkout, or a page-sold one's
  to a shop.
- `bundle`: sold only inside the bundle.

A project with two or more deliverables can sell them together, such as a deck with its handout
documents or a book with its printable:

```json
"deliverables": {
  "deck": {"sell": "bundle"},
  "document": {"sell": "bundle"},
  "bundle": {"name": "Fire Safety Kit", "path": "…", "offer": "kit", "sell": "page", "units": ["deck", "document"]}
}
```

The bundle has its own name, `path` (for `sell: page`), offer and `deliverables/bundle/listing.md` in the
shop listing format; `units` names the deliverables it holds, every one of the project when left out.
Its download page and store packages hold every file of each unit, its README the facts of each, and
its first listing image shows every unit's cover together, followed by the units' own images in turn.
Each unit still sells on its own when its `sell` is `page` or `shops`; with `sell: bundle` it sells only
inside the bundle and gets no shop package of its own.

