# Checking a built page

`lfm check [<url | folder | file.html>] [--project <dir>] [--release] [--json]`
checks the initial HTML of any built page with compile's page rules: HTML, links,
SEO, sharing, accessibility, forms, performance, design and copy. It reads without
changing files or executing scripts. A client-rendered shell is checked as the
HTML received, before JavaScript runs. An empty mount element, a module script and
little or no body text trigger `html.client-shell`. Missing initial content and
metadata are listed in that finding, retaining error severity for a missing
canonical. Independent problems remain separate. Prerendering or server rendering
makes the content available to readers and preview clients without JavaScript.

Use an HTTP or HTTPS URL (including localhost and private hosts), a folder
containing index.html, or a .html file. A folder without index.html needs a built output
folder, an explicit .html file, or the served full URL (for example
http://localhost:8080/path/). A source folder containing lfm.json points
to `lfm compile` instead. Use `--project <dir>` to pair a built target with its
lfm.json, concept, research and templates. Project-only page rules then run with
the same facts and validation settings as compile. The JSON report includes the
absolute project path. Declared concept faces exempt intentional monospace labels
and can change the `design.defaults` count; without project context those findings
explain how `--project` supplies the declarations and validation settings.

Without `--project`, the five page rules needing project facts are listed in
`skipped`: links.declared, jsonld.block, commerce.discovery,
commerce.structured-data and concept.fidelity. With `--project` and no target, the
target is `<dir>/output/`. An in-memory non-release compile compares the full
output tree against the current source and reports one build.stale warning naming
added, changed or removed files. No stamp files are written. Release-specific
optimizations can also produce differences from this non-release baseline.
Static mode also lists release.* and lighthouse.* as skipped.

The reader follows at most five redirects and fetches embedded resources across
origins using GET, without cookies or credentials. It does not crawl links, inspect
iframe or audio/video sources, or fetch srcset alternatives. Stylesheet font faces
and one level of CSS imports are included. Folder reads never use the network and
are confined to the folder, including symlinks. Absolute resource URLs under the
canonical directory map into the folder; other remote files are listed as not inspected.

Limits: 8 MiB for the page and each file, 512 KiB read per image, 32 MiB total,
512 inspected files, eight concurrent requests, ten seconds per request and thirty
seconds for the static read. A subresource exceeding a limit is truncated or listed
as not inspected. The page exceeding its limit is unreadable. Image byte sizes use
Content-Length when available. A truncated file's hash covers only its inspected bytes.
Explicit text/plain responses are rejected even if their body resembles HTML;
missing or application/octet-stream content types are sniffed.

The report's `url` is the final fetched URL, or the local target argument. `canonical`
is the first HTTP(S) canonical resolved against the page URL; `identity` uses that
canonical, otherwise the final URL, or an empty string for local pages without one.
Rules compare metadata against identity, so a localhost preview can declare its
production URL. For pages served from localhost, 127.0.0.0/8 or ::1, URLs on the
served origin are judged against the declared public canonical origin. Without a
public canonical, public-host and mixed-content findings for loopback URLs are
deferred to the live URL. The report includes one note explaining this policy;
other URL syntax findings remain active. Live pages retain every check. Diagnostic locations use the URL path without its leading slash
(index.html for `/`), or the local file name. X-Robots-Tag response headers use the
same crawler policy as robots meta tags in seo.indexability and seo.snippets.
robots.txt and sitemap.xml are not checked.

`--release` adds runtime errors, a frame-by-frame layout walk and Lighthouse.
Chrome, Lighthouse and loopback sockets are required; unavailable capabilities
produce one clear error with setup guidance. See [release checks](compiling.md#release)
for sandbox requirements. For a URL, Chrome visits that URL directly with a fresh temporary profile
and no proxy. The page's JavaScript runs, including analytics; the checks do not
click controls or submit forms. Local pages use the sealed artifact server, which
accepts only GET requests to supplied files. Lighthouse host checks remain in
`hostChecks`, separate from page findings.

`--json` shares compile's validation, release's audit and next-step types. For example
(shortened; files contain sizes, hashes, HTTP status and optional error/truncated):

```json
{
  "target": "site/output",
  "url": "site/output",
  "redirects": [],
  "canonical": "https://lfm.cash/",
  "identity": "https://lfm.cash/",
  "lang": "en",
  "files": [],
  "validation": {"stage": "html", "stats": {}, "diagnostics": [], "errors": 0, "warnings": 0},
  "skipped": [{"rule": "links.declared", "reason": "needs lfm.json urls"}],
  "errors": 0,
  "warnings": 0,
  "durationMs": 15,
  "next": {"phase": "done", "do": "no findings remain.", "notes": ["'lfm rules <id>' explains a rule", "run with --release for runtime, layout and Lighthouse"]}
}
```

The page is first in files; other files are sorted by name. Remote names are absolute
URLs; local names are folder-relative slash paths, and a file on another origin
keeps its absolute URL. Local status is zero. Exit codes:
0 means no errors (warnings allowed), 1 means errors or an unreadable page, 2 means a
usage error. An unreadable page with `--json` produces the same report object, with one
`build.failed` error diagnostic and a fix next step. The top-level `status` and
page file retain the HTTP status when a response was received (including 404/500);
connection, DNS and timeout failures have no HTTP status. Invalid project context
still uses the operational JSON error envelope.

Resources are discovered in parallel waves. File-count admission follows discovery order;
truncation at the shared byte limit may depend on response timing.

Palette checks count custom-property colors only through declarations used on the page,
including indirect var() references. Unused framework tokens do not count.
The machine-copy estimate excludes table elements and subtrees with role=table or role=grid.

`design.ground` errors on warm-light backgrounds on any element (cards, panels and sections count, not only the page ground), paper, parchment or notebook textures on page grounds and full-cover overlays, and torn, zigzag or perforated edges on any element: small gradient tiles (32 px or less) repeated along one axis, made of diagonal hard-stop teeth or radial dots, in a background or mask. Warm background-colour findings exclude tiny elements (area below 256 px², or a lone known dimension at most 4 px), hidden, clipped and entirely off-screen elements; page grounds and full-cover wrappers remain protected from size and position exclusions. Unknown sizes are not assumed small. It checks the default light scheme using the static CSS cascade, including variables and shorthands. Warm-light means OKLCH L ≥ 0.88, C 0.004–0.045 and hue 45–115°; transparent colours do not count, and translucent colours are composited over white. Use white, cool or neutral grey, dark, or saturated subject colours. `data-design-reason` never exempts this rule; `build.validation.rules` can disable it. `concept.ground` applies the same colour restriction to every declared palette colour.

See the [design guidance](design.md#generator-defaults) for the ground and decoration restrictions.

`concept.fidelity` accepts simple English s/es/ed/ing forms of the final word in a
`Use:` phrase when `html lang` is English. Other languages keep the existing word
matching and receive a near-match hint when an English-style suffix is found.
`Avoid:` bans only the leading quoted list, before explanation text or evidence IDs;
for example, `Avoid: "body search"; say "FTS5 search"` bans only "body search".
`copy.placeholder` ignores code elements and filename tokens such as `TODO.md`.
Ordinary `application/json` data is not treated as declared links; JSON-LD and
supported sharing payloads still are.

## Standalone copy

`lfm check listing.txt --lang de --json` and `lfm check post.md` run the same
[shared copy rules](copy-rules.md) as page checks. Markdown front matter can set
`lang` or `language`; `--lang` overrides it or HTML `lang`, then detection is the
fallback. Use `--project dir` for rule levels and reasoned phrase exceptions from
that project's `lfm.json`. These targets skip page/browser checks and keep the same
JSON report and error exit status. `--release` requires an HTML target.

## Site copy

A local **folder** also scans its HTML files recursively, in path order, for
`copy.triplets`, `copy.cross-page` and project `copy.banned` entries (with
`--project`). Other page rules, resources, statistics and
release checks still describe the root index.html. Explicit files and URLs check
one page; URLs are not crawled. The aggregate scan is serial and confined to the
folder (including symlink reads), with no HTML page-count cap, 8 MiB per page and
32 MiB of HTML total; an incomplete scan reports `build.failed`. Empty files
are ignored by the aggregate scan and contribute no page budget, keeping parsed
page retention bounded by the HTML byte limit.

`copy.triplets` warns at **every occurrence** when the total exceeds **10 × the
number of checked HTML pages**. Each page contributes ten endings to the site
budget: 72 pages allow 720, and one page allows ten. This keeps the average
budget constant as a site grows; a folder scan does not enforce a separate
per-page cap. Single-page checks and compile retain the ten-ending limit.
It recognizes sentence/heading endings such as “Sand, Wind und Meer” and
“tools, notes and maps”, including the Oxford comma. This is a soft editorial
signal, not proof of generated text. Each warning has a page, block location,
excerpt and rewrite hint. A single page with eleven endings also triggers it.

`copy.cross-page` emits **one warning per group** of sentences or headings of
**nine or more words** appearing on **two or more pages**. Case, whitespace and
punctuation are normalized; tokens containing digits become `#`. The message
contains the normalized text and the hint lists **every page:line:column**, even
multiple occurrences on a page. Locations identify the containing copy block.
Both rules reuse the existing visible-copy exclusions (hidden/inert content,
quotations, code, `data-slop-skip` and `data-design-reason`). Header, footer and navigation
subtrees (including role=banner/contentinfo/navigation) are excluded. Mark other
intentionally repeated content with `data-lfm-shared`, for example
`<section data-lfm-shared>…legal or office text…</section>` or a repeated card.
The attribute works on inline elements too and skips their entire subtree for
both site rules, including the single-page triplet check. It does not exempt
project bans or other copy rules. Ordinary repeated main/article copy remains
eligible; repetition alone never establishes shared chrome.
Findings use the normal JSON diagnostics and `build.validation.rules` overrides.

These thresholds deliberately differ from Helgoland's `slopcheck.py` duplicates
(more than three pages, prose only, H1 words masked, metadata included): headings
and two-page repetition matter here, while H1 masking could merge different facts.
The single-page triplet limit matches its ten-ending gate, with English and
Oxford-comma support added; the site budget scales with page count. Its `.review-tile` and `.pull-quote` class exemptions are local;
lfm requires quotation markup or `data-slop-skip` instead of inferring authorship
from class names. Metadata is excluded because these checks concern visible copy.
Helgoland's `dupcheck.py` measures cluster-specific five-word shingle similarity,
not identical sentences, so its counts are a separate metric. Agentpreneur's
`slopgate.py` also checks project-specific banned regexes, fragment triplets and
other prose hints; it is not an equivalent comma-list/duplicate gate. Extra banned
phrases belong in `lfm.json`; domain-specific bans stay local.

Project copy bans live in `build.validation.copyBans` in `lfm.json`:

```json
{
  "build": {
    "validation": {
      "copyBans": [
        {"phrase": "hidden gem", "severity": "error", "hint": "Name the place and what visitors find there."},
        {"regex": "\\b(traumhaft|malerisch)[\\p{L}]*", "severity": "warning"}
      ]
    }
  }
}
```

Set exactly one nonblank `phrase` or `regex` per entry, a required `severity`
(`error` or `warning`), and an optional `hint`. Phrases are literal,
case-insensitive and bounded by Unicode letters/digits, so `Oase` does not match
`Oasen` or `Öoase`. Surrounding phrase whitespace is trimmed. Regexes are
case-insensitive Go expressions: add boundaries yourself when needed. Go's `\b`
and `\w` are ASCII; use `\p{L}` for Unicode letters. Lookaround and backreferences
are unsupported. JSON requires doubling regex backslashes.

`lfm compile` and `lfm check --project <dir>` report these as `copy.banned`, with
the matched wording, source location and entry's hint (or a concrete default),
in human and `--json` reports. An error blocks compile before publishing;
warnings are advisory and use the existing report severity `warn`. The existing
`build.validation.rules` override for `copy.banned` applies to all entries.
Text/Markdown checks with `--project` also use the list.

Bans use the built-in copy scope: visible authored body text, title and description
metadata, with phrases joined across inline markup but never across blocks.
Quotes, code, hidden text and `data-slop-skip` subtrees are skipped. The shared
engine normalizes curly apostrophes and nonbreaking spaces and lowercases text
before matching. Bans apply in every language. They do not scan JSON-LD, scripts,
source files or deliverables; `copyAllow` exempts built-in rules only.

An optional `selector` scopes one ban to matching HTML elements and their
visible descendants. For example, a travel-only CTA policy can use:

```json
{"regex":"mehr erfahren|jetzt buchen", "selector":".travel", "severity":"error"}
```

Only apply that selector to the site's travel sections. A calculator link saying
“Mehr erfahren” inside `div.ctabox` then stays outside the travel ban. Class names
have no built-in meaning: without `selector`, this regex explicitly bans the
phrase everywhere, including product links. Keep production-note bans unscoped
when they should apply site-wide. Selectors accept CSS selector lists; invalid
selectors report `build.validation.copyBans[N].selector`. Scoped bans exclude
metadata and do not run on standalone text/Markdown, which has no DOM. Use an
unscoped entry for those targets. Shared markers do not bypass bans.

For project builds, `lfm compile --release` reuses cached browser results;
`lfm compile --release` builds critical CSS and prints; `--validate` adds browser audits and layout checks. The standalone
`lfm check --release` command continues to run browser checks.
