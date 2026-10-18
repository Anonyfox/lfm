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
  "urls": { "checkout": "https://buy.stripe.com/…" },
  "data": { "offer": { "price": 29 } },
  "build": {
    "browserData": ["data.offer"],
    "assets": { "overrides": { "assets/images/hero.webp": "file" } },
    "validation": { "rules": { "design.mono": "off" } }
  }
}
```

| Field | Meaning |
| --- | --- |
| `schemaVersion` | Always `1`. |
| `url` | Absolute HTTPS URL of the folder the page is deployed to, ending in `/`, without query or fragment. It is the page's canonical URL, and social media URLs are resolved against it. |
| `language`, `title`, `description` | Required page facts. Render them in the template. |
| `socialImage` | Optional preview image source and alt text. |
| `urls` | Named, complete URLs: checkout links, contacts, navigation, remote resources. Render with `{{.urls.checkout}}`; rendered links are checked against this list. |
| `links` | Optional free-form labels or key references for links. |
| `jsonld` | Optional structured data object or array; see below. |
| `data` | Every product value the page uses: offers, prices, copy blocks, FAQ entries. |
| `build.browserData` | Dot paths copied into `<script type="application/json" id="product-data">` for scripts, keeping their nesting: `"data.offer"` arrives as `{"data":{"offer":…}}`. |
| `build.assets` | Packing policy: `default` is `auto` (the compiler decides), `inline` or `file`; `overrides` pins single source paths. |
| `build.validation.rules` | Per-rule levels: `off`, `warn` or `error`. Required rules stay errors. See `lfm rules`. |

Reserved fields use exact casing; unknown keys are errors with a hint. Custom values belong under `data`.
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
  "publisher": "Baker Books", "imprint": "Baker Books, Example Street 1, 12345 City",
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

`publisher`, `imprint` (the lines of the copyright page, such as a German Impressum), `paper` (`white` or
`cream`, which sets the paperback's spine) and the ISBNs are optional; an ISBN is checked against its check
digit and printed on the copyright page, and the paperback's draws its barcode on the back cover.
`listing.md` is checked against the strictest store's limits (`book.listing`) and rendered into every
channel's format; see [compiling](compiling.md#store-packages).

