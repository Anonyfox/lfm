# Checking a built page

`lfm check <url | folder | file.html> [--release] [--json]` checks the initial HTML of
any built page with compile's page rules: HTML, links, SEO, sharing, accessibility,
forms, performance, design and copy. It reads without changing files or executing scripts.
A client-rendered shell is checked as the HTML received, before JavaScript runs.

Use an HTTP or HTTPS URL (including localhost and private hosts), a folder containing
index.html, or a .html file. A source folder containing lfm.json points to `lfm compile`
instead. Config, templates, research, concept and deliverables are outside this check.
The five page rules needing project facts are listed in `skipped`: links.declared,
jsonld.block, commerce.discovery, commerce.structured-data and concept.fidelity.
Static mode also lists release.* and lighthouse.* as skipped.

The reader follows at most five redirects and fetches embedded resources across
origins using GET, without cookies or credentials. It does not crawl links, inspect
iframe or audio/video sources, or fetch srcset alternatives. Stylesheet font faces
and one level of CSS imports are included. Folder reads never use the network and
are confined to the folder, including symlinks. Absolute resource URLs on the
canonical origin map into the folder; other remote files are listed as not inspected.

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
production URL. Diagnostic locations use the URL path without its leading slash
(index.html for `/`), or the local file name. X-Robots-Tag response headers use the
same crawler policy as robots meta tags in seo.indexability and seo.snippets.
robots.txt and sitemap.xml are not checked.

`--release` adds runtime errors, a frame-by-frame layout walk and Lighthouse when
installed. For a URL, Chrome visits that URL directly with a fresh temporary profile
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
usage error. An unreadable page with --json produces `{"error": "…", "next": {…}}`.

Resources are discovered in parallel waves. File-count admission follows discovery order;
truncation at the shared byte limit may depend on response timing.
