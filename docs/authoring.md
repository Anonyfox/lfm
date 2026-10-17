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
  output/             the compiled page; replaced on every successful compile
```

Sources may live anywhere inside the folder, except `output/`. Symlinks may not leave it.

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
| `build.browserData` | Dot paths copied into `<script type="application/json" id="product-data">` for scripts. |
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
(`concept.fidelity`). `Open: <question>` lines list what only the owner can answer.
