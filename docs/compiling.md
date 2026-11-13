# What `lfm compile` does

One compile runs a fixed lifecycle. A stage starts only when the stages before it found no errors, and all
of them report through the same findings, so a single run lists everything that needs fixing.

| Stage | Work |
| --- | --- |
| load | Read `lfm.json` and `data/`; every config problem is reported at its JSON key. Check the research files and `concept.md`. |
| prepare | Check every template binding, link and offer statically, then render the page. |
| assemble | Resolve, bundle and pack every referenced resource; inspect images and fonts. |
| optimize | `--release`: reuse cached optimization, or measure and split critical CSS in low-priority Chrome. |
| finalize | Serialize and compact the HTML; generate crawler discovery files. |
| verify | Run every check on the exact bytes that will be written. |
| deliver | When `deliverables/book/`, `deliverables/printable/`, `deliverables/spreadsheet/`, `deliverables/document/` or `deliverables/deck/` exists: build it, publish its download page when it sells through the page, and with `listing.md` assemble its store packages in `distribution/<unit>/`; then the bundle of the units it holds when one is configured. PDFs, a book's paperback and covers, a printable's and a spreadsheet's listing images and start PDFs are made in Chrome by `--release` and reused by every later compile while their inputs are unchanged. Missing prints are rendered by `--release`. |
| audit | `--validate`: runtime probe, layout audit and Lighthouse. `--release`: reuse the exact artifact’s cached findings, or skip silently. |
| publish | Replace `output/` with one rename. |

Nothing is written before verify passes. A failed compile keeps the previous `output/` and exits with code 1.
Findings carry a rule ID, the location, what is wrong and how to fix it; `lfm rules <id>` explains a rule.

The report ends with the next step and the skill for it, in this order: fix errors; collect research when
`research/` is empty; write `concept.md` when it is missing; replace the starter page; resolve warnings;
run `--release --validate` and review the frames; deploy. It never writes skill files. After an upgrade, run the printed `lfm skills install` command to update older copies.
`--json` prints the same result as one object (`errors`, `warnings`, `validation`, `next`, `skills`, and
the build details, including `validated` for the requested browser-validation mode) and keeps progress messages on stderr.

## Release and validation

`--release` builds the shippable page: critical CSS and prints, with low-priority Chrome only on cache misses, never Lighthouse.
`--validate` (implies `--release`) adds Lighthouse, browser audits and layout checks for opt-in proof/QA runs.

Optimization and prints are cached by their exact inputs; cold and warm optimization output bytes match.
Critical-CSS extraction failures block publishing and preserve the previous output. Legitimate full-CSS
fallbacks (small sheets, external resources/CSP, independent loading semantics or insufficient savings)
remain valid. Cached audit findings are reused when available; a miss is silent. Validation checks
reused prints too. Tool versions are probed once per executable and remembered in the cache.

Browser work is limited so several builds on one machine queue instead of stacking:

- One browser pool at a time per machine: every Chrome and Lighthouse launch holds an exclusive lock on
  `/tmp/lfm-browser.lock` (`LFM_BROWSER_LOCK` changes the path); other lfm processes wait and say so on
  stderr. The lock does not depend on `TMPDIR` or `XDG_CACHE_HOME`. Windows has no lock.
- `--browser-jobs N` (or `LFM_BROWSER_JOBS`, default 2, minimum 1) bounds concurrent browser launches
  within one compile.
- On Linux, Chrome and Lighthouse start through `nice -n 10` and `ionice -c 2 -n 7` when those programs
  exist; otherwise they start unchanged.

HTML social cards (`socialImage.src` pointing at HTML) render on a cache miss in every mode, under the same
lock and priority, because the page cannot ship without its card.

## Research and concept

Research is checked before template preparation: `lfm compile` (or `lfm compile --json`)
reports `research.*` findings even when templates are incomplete. Minimums apply to totals across
all batches: 60 voices from 5 platforms and 12 peers. Refresh batches add to those totals.
A `shortfall: <what and why>` on any batch waives both minimums; its text remains in the summary.
Imported entries may keep original fetch dates with an `Imported: YYYY-MM-DD from <origin>` line
(see `lfm skills research`). A concept fact may cite an existing project file with a relative
Markdown link; missing files and paths escaping the project are reported.

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
explicitly configured binary assets keep their names. Every emitted stylesheet and script, including
explicit file mode, uses the first 12 hex digits of its content SHA-256 before the extension
(`bundle-<hash>.css`, `app-<hash>.js`).

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
contents page. `--release` prints missing or stale PDFs; ordinary compiles also render the screen PDF when needed. Every PDF it writes carries
the key of the inputs it was printed from in its document information, and any later compile, `lfm serve`
included, reuses a PDF from `output/` or `distribution/` whose key still matches. An ordinary compile keeps the EPUB and screen PDF current; paperback prints are built by `--release`. The page
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
copy link from `deliverables.spreadsheet.sheets`. `--validate` recalculates every file in LibreOffice when
it is installed (`spreadsheet.compat`). The download page lists the copy links first, then the files. With
`deliverables/spreadsheet/listing.md`, `distribution/spreadsheet/` holds every file once, `images/` with
the listing images (the dashboard alone, every tab, each tab, what it works with) and a guide per shop;
for Etsy, more than five files are zipped per edition.

### Document packages

A document set builds one `.docx` per document, language and paper size on every compile, each stamped
with the key of its inputs and reused while they are unchanged, and ships one ZIP per edition
(`<slug>-<language>-<paper>.zip`): the documents named by their titles, the fonts with their licence and,
after a release, `PDF/` with the print of every document and the start guide. `--release` prints each
document's print layout, which follows the .docx line for line, in Chrome, renders the listing images from the first pages and prints
the start guides; later compiles reuse all of it from the published ZIPs. A release needs Chrome alone:
`--validate` checks planned pages, paper and fonts (`document.layout`); LibreOffice, when installed,
opens every .docx as a second opinion. The report names optional
helpers that are not installed and what each would add; none is a warning. With
`deliverables/document/listing.md`, `distribution/document/` holds the ZIPs, the images and a guide per
shop.

### Deck packages

A deck set builds one `.pptx` per deck and language on every compile: a 16:9 slide master with six layouts
(title, title and content, section, two content, picture, statement) whose placeholders every slide fills,
the Look's fonts embedded, and speaker notes from `::: notes` on notes pages. Every compile measures each
slide's text with the fonts and warns where it would not fit (`deck.design`). It ships one ZIP per language
(`<slug>-<language>.zip`); `--release` adds `PDF/` with the slides, the speaker notes beside every slide on
each of the language's papers and, when the deck asks questions, a quiz sheet with its answer key. It renders
the listing images from the real slides, notes and quiz sheet and prints the start guides. `--validate`
checks every slide's boxes against what they show (`deck.layout`); LibreOffice, when installed, opens
every .pptx as a second opinion.

Compiles of the same project are serialized across processes by locking the project directory; no lock
file is created, and a second compile waits and says so. Nothing is written into the project except
`output/`, `distribution/` for deliverables with a listing, and, only while publishing, sibling `*.next-*`
folders that the swap removes again.
Ctrl-C stops a compile at once, Chrome and LibreOffice included; a stopped compile never publishes.
LibreOffice's verdict on files it already opened is kept in the user cache under `lfm/`, so a release of
unchanged documents, decks or workbooks does not open them again.

## Release

`lfm compile --release` builds critical CSS and prints. `--validate` adds the runtime/layout audit and Lighthouse stages below:

1. **Critical CSS** (45 s per browser operation): the first screen is rendered at 390×844, 768×1024 and 1440×900; the rules it
   uses stay in the head and the complete stylesheet loads after. A rule counts by the elements it styles:
   pseudo-elements by their element, states such as `:hover` as they hold on a first visit, and rules that hide
   or move an element away (a closed menu, a skip link) where that element would otherwise appear. Every
   measurement waits for fonts and first-screen images, lays the page out afresh and repeats until two
   readings agree; the first screen with the critical rules alone, and with both, must match the full page in
   every computed style, box and pixel at every size, or the complete stylesheet stays blocking.
   Unsupported stylesheet semantics (including multiple or external sheets/imports), authored CSP, failed web fonts, unsized external media and unverifiable rendering keep the full CSS and report a `release.critical` warning. The split is
   applied only when it removes at least 2 KiB and 20% of render-blocking CSS.
2. **Runtime probe and layout audit** (90 s): the page runs with its scripts in a fresh Chrome profile;
   exceptions and missing files are reported. Then the page is scrolled by half a screen at a time at 390×844,
   1280×680 and 1440×900, each size in its own tab at the same time (the book's prints and layout check run
   alongside); finite animations settle, looping ones pause at their start, and every frame is
   saved as a screenshot in the user cache under `lfm/`. The report prints the folder and, from the second run
   on, the frames whose content changed. `release.layout` reports a first screen without a legible heading or
   any control, stretches without legible content, sideways scrolling, text a pinned layer always covers,
   text under 9 px, text under the WCAG contrast minimum on a certain flat ground, numbers split from their
   units, web fonts that fail to load or lack glyphs the page uses, what still moves when the page is loaded again with reduced motion requested, and a heading or most of the text missing when it is loaded without JavaScript. Outside
   requests are blocked; nothing is clicked, typed or submitted.
   Images get the hints the author left out: those below the first screen at every size load lazily, and
   the largest file image of the phone's first screen is fetched first; authored hints are kept.
3. **Lighthouse**: one mobile lab run on the fast path. Timing findings
   trigger up to three runs. Audit metrics and category scores
   use per-audit medians; the report shows run count and metric ranges. Deterministic
   findings do not trigger retries. The saved full JSON is the last run, rather than the
   synthesized median. Metric warnings use mobile good thresholds: LCP 2.5 s, TBT 200 ms,
   FCP 1.8 s, speed index 3.4 s and CLS 0.1. CLS details name shifting elements and scores
   when Lighthouse supplies them. Zero-weight diagnostics are informational. URL-bearing
   third-party rows are informational and name their hosts; mixed audits warn for first-party
   rows only. Scores and findings are reported; the complete JSON is saved in the
   user cache under `lfm/lighthouse/`. Audits of what the static host
   decides (cache lifetimes, HTTPS, latency) are listed as host settings instead of warnings. Text goes out
   gzip-compressed, as every common static host sends it, so the metrics describe the page visitors get;
   the report's host line gives the text as sent and as built, and about how much later a phone gets it
   from a host that does not compress, and the deploy step names the check for it.

`LFM_CHROME` and `LFM_LIGHTHOUSE` override automatic discovery, including when invalid. Otherwise Chrome
is found on `PATH` in this order: `google-chrome`, `google-chrome-stable`, `chromium`, `chromium-browser`,
`chrome`, then headless shells and other Chromium browsers. Lighthouse uses `lighthouse` on `PATH`.
Both then check standard Linux/macOS install paths and finally the bin directory from `npm prefix -g`
(`npm install -g lighthouse`). Lighthouse also checks the project’s `node_modules/.bin` before npm global
bin. `lfm doctor [directory]` reports both resolved absolute paths or missing-tool reasons; `--json`
contains the same paths, reasons and next step. Missing
validation tools produce a validation error with setup guidance; lfm never installs anything.

Lighthouse runs serialize across processes sharing the user cache, using an advisory `flock` on
`<user-cache>/lfm-lighthouse.lock` (outside the pruned artifacts). The lock covers Chrome startup,
all measurements and cleanup; process exit or a crash releases it. A contender prints
`waiting for Lighthouse lock (held by pid N)` to stderr and waits at most ten minutes;
caller cancellation also stops the wait. Never remove the lock file while builds are running.
Each measurement, including Chrome startup, has a separate timeout: 45 seconds multiplied by
`max(1, one-minute host load / logical CPUs)`, capped at five minutes. Linux supplies the host
load; other platforms without `/proc/loadavg` use 45 seconds. Set `LFM_LIGHTHOUSE_TIMEOUT=3m`
(or another positive Go duration such as `90s`) to override this per-measurement budget.
The audit budget starts after acquiring the lock; a caller's earlier deadline still wins.
Timeout errors name the budget, host load and CPU count and suggest waiting or an override.
Serialization prevents competing lfm Lighthouse runs; unrelated host load can still affect scores.

Release checks start Chrome and open local listeners for artifact HTTP and Chrome DevTools.
Sandboxed jobs must allow listening and connecting on loopback (127.0.0.1); otherwise run
`compile --release --validate` or `check --release` outside the sandbox. Ordinary static checks need
neither Chrome nor local sockets. Use `LFM_CHROME` and `LFM_LIGHTHOUSE` to select installed binaries.

## Preview

`lfm serve` compiles, serves the page on `127.0.0.1` and recompiles when a file changes. The browser reloads
only when the compiled page actually changed, keeping the scroll position. A failed compile shows its
findings in the browser and keeps serving the last good page. The preview serves compiled files only, never
sources, and injects its reload script into responses, never into `output/`.

`socialImage.src` accepts a local HTML card as well as a PNG or JPEG. Compile bundles the card's relative
images, fonts and CSS, renders it through Chrome at 1200×630 CSS pixels, and emits a hashed image URL.
This also runs without `--release`; an unchanged card and its assets reuse the previous render in
`output/`. Set `LFM_CHROME` if Chrome is not in PATH. A render has a 60-second tab timeout.

Release optimization and audits are cached under the system user cache directory in `lfm/release/`.
The keys include the served bytes, lfm build identity, Chrome and Lighthouse versions and audit settings.
Public files are read once per invocation; the finalized page and sidecars are fixed before delivery.
Delivery finishes and its browser closes before the page audit begins. Bundle image slots are selected
before rendering; each demanded image is reused independently. Units with their own stores retain their
full image sets.

An unchanged artifact reuses its critical CSS decision, complete runtime/layout result and Lighthouse result; the
report gives the original audit date. Changing any served byte or tool version requires a fresh audit.
Live URLs checked with `lfm check --release` always run fresh audits. Cache failures do not block a build;
tool-version failures are retried once and reported if a successful audit cannot be cached. Incomplete
layout walks are never reused. Entries expire after 30 days and at most 128 are kept.

Review frames, Lighthouse reports and LibreOffice verdicts live under the user cache in
`lfm/frames/<project-hash>/`, `lfm/lighthouse/<project-hash>.json` and `lfm/libreoffice/`. Each category
keeps at most 128 entries and removes entries unused for 30 days. If the user cache cannot be located,
lfm uses the system temp directory. Working directories are removed on exit; startup removes owned
scratch older than six hours. Chrome, Lighthouse and LibreOffice child processes receive private
working directories as `TMPDIR`, `TMP` and `TEMP`, so their temporary files are removed with those
directories on success, failure or cancellation. These directories use `os.TempDir()`, honoring the
caller’s `TMPDIR`; set it to a project-owned scratch directory to keep work off the shared system temp.

Published image files use a shared content-addressed cache at `<user-cache>/lfm/img/`.
Identical bytes share an inode across projects, worktrees, output and distribution;
across filesystems publication copies the bytes. Cache files and linked outputs are
read-only: edit source images and compile again rather than editing output in place.
The next successful compile migrates existing output copies to cache links.
On Unix, entries with more than one hardlink are in use: pruning neither expires
these entries nor charges them against its 300 MiB eviction budget. Once their
outputs are removed, entries expire after two days and compete for the shared
300 MiB cache budget. On other platforms,
pruning may remove the cache name; existing output hardlinks still retain the bytes.

Existing authored image variants remain ordinary referenced assets. Do not delete
variants while templates, stylesheets or scripts still refer to them. To remove
committed variants, first replace those references with retained originals, compile
and review the result, then remove the unused files from Git. The compiler does
not currently generate replacement width variants or rewrite authored srcsets.


## Fast releases and validation

`lfm compile --release` builds critical CSS and missing prints with low-priority Chrome on cache
misses, never Lighthouse. Warm optimization and prints are reused without Chrome.
`lfm compile --validate` adds Lighthouse, browser audits and deliverable layout/office checks,
including checks on reused prints. Audit cache misses are silent in plain releases.
HTML social cards retain their content cache in all modes.

Every browser and Lighthouse launch takes the box-wide `/tmp/lfm-browser.lock`
(`LFM_BROWSER_LOCK` overrides it). Waiting is cancellable; nested work in the
same process shares the lock. Non-Unix systems have no box-wide lock.
Linux children use `nice -n 10` and `ionice -c 2 -n 7` when available.
`--browser-jobs N` (`LFM_BROWSER_JOBS`, default 2, minimum 1) bounds browser
work. Viewports, frames, print jobs and pictures currently run sequentially.
