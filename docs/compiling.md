# What `lfm compile` does

One compile runs a fixed lifecycle. A stage starts only when the stages before it found no errors, and all
of them report through the same findings, so a single run lists everything that needs fixing.

| Stage | Work |
| --- | --- |
| load | Read `lfm.json` and `data/`; every config problem is reported at its JSON key. Check the research files and `concept.md`. |
| prepare | Check every template binding, link and offer statically, then render the page. |
| assemble | Resolve, bundle and pack every referenced resource; inspect images and fonts. |
| optimize | `--release` only: split critical CSS after measuring it in headless Chrome. |
| finalize | Serialize and compact the HTML; generate crawler discovery files. |
| verify | Run every check on the exact bytes that will be written. |
| deliver | When `deliverables/book/`, `deliverables/printable/`, `deliverables/spreadsheet/`, `deliverables/document/` or `deliverables/deck/` exists: build it, publish its download page when it sells through the page, and with `listing.md` assemble its store packages in `distribution/<unit>/`; then the bundle of the units it holds when one is configured. PDFs, a book's paperback and covers, a printable's and a spreadsheet's listing images and start PDFs are made in Chrome by `--release` and reused by every later compile while their inputs are unchanged. |
| audit | `--release` only: a runtime probe, the layout audit and, when installed, Lighthouse. |
| publish | Replace `output/` with one rename. |

Nothing is written before verify passes. A failed compile keeps the previous `output/` and exits with code 1.
Findings carry a rule ID, the location, what is wrong and how to fix it; `lfm rules <id>` explains a rule.

The report ends with the next step and the skill for it, in this order: fix errors; collect research when
`research/` is empty; write `concept.md` when it is missing; replace the starter page; resolve warnings;
run `--release` and review the frames; deploy. It also updates untouched lfm skills written by an older release (in the project and the folders above it) and notes copies the project changed.
`--json` prints the same result as one object (`errors`, `warnings`, `validation`, `next`, `skills`, and
the build details) and keeps progress messages on stderr.

## Packing

With the default policy the compiler decides per resource whether it is inlined into `index.html` or emitted
as a file beside it, and explains each choice:

- Fonts, lazily loaded media, responsive candidates and anything above 32 KiB encoded become files.
- The largest remaining resources move out until the page approaches 256 KiB.
- Small images become data URLs; SVGs use the smaller of percent and base64 encoding after compression.
- Compatible CSS above 20 KiB and module JS above 50 KiB become files.
- Social images, media and linked documents are always files with absolute URLs.

Automatic files get a short suffix hashed from their path and content, so every change gets a new URL and
hosts can cache files forever. `build.assets` in `lfm.json` overrides the policy globally or per path;
explicitly configured files keep their names.

HTML is compacted at the syntax level only: attribute quotes and empty values shrink, text and whitespace
stay exactly as written. Each compacted result is parsed and compared with the original tree.

## Output

`output/` contains `index.html`, emitted assets and any public files or deliverable folders, ready to
upload to any static host at the configured `url`. It is replaced as a whole; there are no stale files,
sources or reports in it. Every regular file under `public/` is copied unchanged at the same relative
path, including `.well-known/`; other dotfiles and dot-directories are skipped. Public files receive no
checks, rewriting, hashed names or inlining. Other than the discovery overrides below, a collision with
an emitted name or deliverable download folder is an error.

For an indexable page, `sitemap.xml` lists only the configured canonical `url`, with no timestamps.
It never lists unlisted deliverable download folders. At a host's root (`url` has path `/`), `robots.txt`
allows all crawling and points to that sitemap. Subfolder deployments get only the sitemap: crawlers
read `robots.txt` at the host root, so a subfolder's file would have no effect.
If the final page requests `noindex` or `none` in robots or supported crawler meta tags, lfm omits the
sitemap and the robots `Sitemap` line; crawling stays allowed so crawlers can read the restriction.
Neither generated file names private download paths. Put an authored `robots.txt` or `sitemap.xml`
in `public/` to override generation of that file, including for noindex or subfolder deployments.

`lfm compile --json` includes every deployed file in `files`, including emitted CSS and deliverable
folders. Each entry has its output-relative `name`, `bytes` and lowercase hex `sha256` of the exact
output bytes. This deployment manifest stays in the report; no manifest file is published.

A book adds one folder named after `deliverables.book.path`: an `index.html` download page (marked
noindex, sending no referrer) and, once every chapter is written, `<slug>.epub` and `<slug>.pdf`, named after the title. The
EPUB is EPUB 3 with a navigation document and an NCX for older readers, built reproducibly and checked by
lfm itself: mimetype first and stored, every manifest item present and every file declared, every link
resolving. The PDF is printed at the concept's trim size with page numbers, a cover page and a linked
contents page. Printing takes Chrome and seconds, so only `--release` prints: every PDF it writes carries
the key of the inputs it was printed from in its document information, and any later compile, `lfm serve`
included, reuses a PDF from `output/` or `distribution/` whose key still matches. An ordinary compile of a
changed book keeps the EPUB current and leaves the PDF out until the next release; the report says so. The page
itself must never contain the path (`book.delivery`). Stripe's after-payment redirect and the invoice
email are how buyers get there; `lfm compile` prints the address.

### Store packages

With `deliverables/book/listing.md`, the same compile writes `distribution/book/` next to `output/`
(each sellable unit has its folder: `book/`, `printable/`, `spreadsheet/`, `document/`, `deck/`, `bundle/`):

```text
distribution/book/
  README.md                 every channel, what is ready, the decisions left to the owner
  <slug>.epub               the ebook, named after the title
  <slug>.pdf                the PDF, for stores that sell one
  cover.jpg                 the store cover
  paperback-interior.pdf    the print interior
  paperback-cover-kdp.pdf   a full cover wrap per spine formula a channel uses (kdp, lulu)
  stores/kdp.md             one guide per channel
```

Each file is there once. A guide in `stores/` (Amazon KDP, Apple Books, Google Play Books, Kobo, tolino,
Barnes & Noble, Draft2Digital, IngramSpark, Lulu, Etsy, Gumroad, Payhip) links the files its channel takes
and holds every field in its format, ready to paste: KDP's HTML description, plain text for Apple and
Kobo, the Etsy title and tags, royalty bands for the listed prices and the AI disclosure each store asks
for. Channels that refuse AI-generated books are left out when the listing declares generated text.

- The EPUB carries EPUB Accessibility 1.1 metadata, the BISAC subjects and the ebook ISBN.
- `cover.jpg` is the cover as an RGB JPEG under 5 MB.
- `paperback-interior.pdf` is printed at the trim with mirrored margins, an inside margin that grows with
  the page count as KDP requires, chapters on right-hand pages, page numbers in the contents, no links or
  bookmarks, and an even page count ending on a blank page. A measuring print finds the pages first.
- `paperback-cover-<formula>.pdf` is the full wrap with bleed: back cover text and barcode, spine (titled
  from 79 pages), front. It is laid out for KDP's and Lulu's spine formulas when one of them takes the
  paperback; the other printers use their own templates.

`book.channels` warns where a channel would refuse a file: too few pages, a front cover below 300 dpi
(add `images/cover-print.jpg`), a trim KDP does not print, transparency in a print file. `distribution/` is
never deployed, never read as a source and ignored by `lfm serve`; the scaffold's `.gitignore` excludes it.
It is removed when `listing.md` is.

### Printable packages

A printable publishes one PDF per edition (`<slug>-<size>-<variant>.pdf`) behind its unlisted path, with
a download page that tells buyers to print at actual size. With `deliverables/printable/listing.md`,
`distribution/printable/` holds every PDF once, `images/` with the listing images (3000×2250 JPEGs under 1 MB: the
cover alone, everything included, each part, the sizes to scale), and a guide per shop in `stores/` (Etsy,
Gumroad, Payhip, Ko-fi) naming the files and images to upload and every field ready to paste, with the
page count, sizes and link count taken from the build. Etsy takes five files: when the PDFs are more, lfm
adds a ZIP per paper size. `printable.channels` warns about files over 20 MB and links that send Etsy
buyers elsewhere.

### Spreadsheet packages

A spreadsheet builds two Excel files per edition on every compile: `<slug>-<edition>.xlsx`, the blank
template, and `<slug>-<edition>-example.xlsx`, filled with the example dataset. Every formula is written
with its computed value, so the files open with numbers in any app, and a rebuild of unchanged sources is
byte-identical. `--release` adds `<slug>-<edition>-start.pdf`, the start-here guide with the Google Sheets
copy link from `deliverables.spreadsheet.sheets`, and recalculates every file in LibreOffice when it is
installed (`spreadsheet.compat`). The download page lists the copy links first, then the files. With
`deliverables/spreadsheet/listing.md`, `distribution/spreadsheet/` holds every file once, `images/` with
the listing images (the dashboard alone, every tab, each tab, what it works with) and a guide per shop;
for Etsy, more than five files are zipped per edition.

### Document packages

A document set builds one `.docx` per document, language and paper size on every compile, each stamped
with the key of its inputs and reused while they are unchanged, and ships one ZIP per edition
(`<slug>-<language>-<paper>.zip`): the documents named by their titles, the fonts with their licence and,
after a release, `PDF/` with the print of every document and the start guide. `--release` prints each
document's print layout, which follows the .docx line for line, in Chrome and holds it to its planned
pages, paper and fonts (`document.layout`), renders the listing images from the first pages and prints
the start guides; later compiles reuse all of it from the published ZIPs. A release needs Chrome alone:
LibreOffice, when installed, opens every .docx as a second opinion. The report names optional
helpers that are not installed and what each would add; none is a warning. With
`deliverables/document/listing.md`, `distribution/document/` holds the ZIPs, the images and a guide per
shop.

### Deck packages

A deck set builds one `.pptx` per deck and language on every compile: a 16:9 slide master with six layouts
(title, title and content, section, two content, picture, statement) whose placeholders every slide fills,
the Look's fonts embedded, and speaker notes from `::: notes` on notes pages. Every compile measures each
slide's text with the fonts and warns where it would not fit (`deck.design`). It ships one ZIP per language
(`<slug>-<language>.zip`); `--release` adds `PDF/` with the slides, the speaker notes beside every slide on
each of the language's papers and, when the deck asks questions, a quiz sheet with its answer key, holds
every slide's boxes to what they show (`deck.layout`), renders the listing images from the real slides,
notes and quiz sheet and prints the start guides. LibreOffice, when installed, opens every .pptx as a
second opinion.

Compiles of the same project are serialized across processes by locking the project directory; no lock
file is created, and a second compile waits and says so. Nothing is written into the project except
`output/`, `distribution/` for deliverables with a listing, and, only while publishing, sibling `*.next-*`
folders that the swap removes again.
Ctrl-C stops a compile at once, Chrome and LibreOffice included; a stopped compile never publishes.
LibreOffice's verdict on files it already opened is kept in the system temp directory, so a release of
unchanged documents, decks or workbooks does not open them again.

## Release

`lfm compile --release` adds three bounded stages that need installed tools:

1. **Critical CSS** (45 s limit): the first screen is rendered at 390×844, 768×1024 and 1440×900; the rules it
   uses stay in the head and the complete stylesheet loads after. A rule counts by the elements it styles:
   pseudo-elements by their element, states such as `:hover` as they hold on a first visit, and rules that hide
   or move an element away (a closed menu, a skip link) where that element would otherwise appear. Every
   measurement waits for fonts and first-screen images, lays the page out afresh and repeats until two
   readings agree; the first screen with the critical rules alone, and with both, must match the full page in
   every computed style, box and pixel at every size, or the complete stylesheet stays blocking. The split is
   applied only when it removes at least 2 KiB and 20% of render-blocking CSS.
2. **Runtime probe and layout audit** (90 s): the page runs with its scripts in a fresh Chrome profile;
   exceptions and missing files are reported. Then the page is scrolled by half a screen at a time at 390×844,
   1280×680 and 1440×900, each size in its own tab at the same time (the book's prints and layout check run
   alongside); finite animations settle, looping ones pause at their start, and every frame is
   saved as a screenshot in the system temp directory. The report prints the folder and, from the second run
   on, the frames whose content changed. `release.layout` reports a first screen without a legible heading or
   any control, stretches without legible content, sideways scrolling, text a pinned layer always covers,
   text under 9 px, text under the WCAG contrast minimum on a certain flat ground, numbers split from their
   units, web fonts that fail to load or lack glyphs the page uses, what still moves when the page is loaded again with reduced motion requested, and a heading or most of the text missing when it is loaded without JavaScript. Outside
   requests are blocked; nothing is clicked, typed or submitted.
   Images get the hints the author left out: those below the first screen at every size load lazily, and
   the largest file image of the phone's first screen is fetched first; authored hints are kept.
3. **Lighthouse** (45 s, when installed): one mobile lab run. Scores, metrics and every failing audit are
   reported; the complete JSON is saved in the system temp directory. Audits of what the static host
   decides (cache lifetimes, HTTPS, latency) are listed as host settings instead of warnings. Text goes out
   gzip-compressed, as every common static host sends it, so the metrics describe the page visitors get;
   the report's host line gives the text as sent and as built, and about how much later a phone gets it
   from a host that does not compress, and the deploy step names the check for it.

Chrome or Chromium is found on `PATH` and in standard locations, or set `LFM_CHROME`. Lighthouse is found on
`PATH`, in `node_modules/.bin`, or via `LFM_LIGHTHOUSE` (`npm install -g lighthouse`). A missing tool is a
warning and its stage is skipped; lfm never installs anything.

## Preview

`lfm serve` compiles, serves the page on `127.0.0.1` and recompiles when a file changes. The browser reloads
only when the compiled page actually changed, keeping the scroll position. A failed compile shows its
findings in the browser and keeps serving the last good page. The preview serves compiled files only, never
sources, and injects its reload script into responses, never into `output/`.
