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
