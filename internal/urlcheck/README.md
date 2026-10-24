# Named URLs and link guardrails

One config name per destination; reuse the name at every placement. The compiler
checks authoring sources before rendering and the exact final HTML before writing.
URL policy uses the same diagnostics/overrides as template and HTML validation.
No network requests, silent URL fixes or query rewriting.

```json
{
  "url": "https://example.org/product/",
  "urls": {
    "checkout": "https://buy.stripe.com/replace-me",
    "contact": "mailto:hello@example.org?subject=Product%20question",
    "phone": "tel:+4940123456",
    "calculator": "#calculator",
    "relatedHotels": "../meeting-hotels/"
  },
  "links": {
    "related": [{"urlKey":"relatedHotels","label":"Find meeting hotels"}]
  }
}
```

```html
<a href="{{.urls.checkout}}">Buy</a>
<a href="{{.urls.calculator}}">Calculate</a>
{{range .links.related}}<a href="{{index $.urls .urlKey}}">{{.label}}</a>{{end}}
```

`urls` is a flat map of unique JSON names to raw strings. Use purpose-based names,
not domain names or placement names such as `heroLink1`. Separate complete variants
when their destinations/query/fragment differ deliberately. `links` remains free
metadata for lists/labels/key references; put the destination itself in `urls`.
Existing top-level `url` and `socialImage.src` are already named owners: do not copy
those values into `urls` just to satisfy a check. Duplicate JSON keys are hard errors.

## Link rules

| Rule | Defaults |
| --- | --- |
| `links.syntax` | Error: browser parse failure, invalid port/host/IPv6/UTF-8, controls, likely protocol typo, malformed HTTP separator, script pseudo-URL, unresolved placeholder or contextual
  escaping that invalidates/changes the configured destination. Warn: browser repairs, whitespace/percent encoding, suspicious hostname, credentials, unknown application scheme, likely missing protocol, mailto/tel mistakes and pre-escaped ampersands. |
| `links.registry` | Error: empty name. Warn: awkward identifiers, duplicate normalized destinations and URL literals in visible copy. |
| `links.literal` | Error: hardcoded destinations in template attributes, URL-like JSON fields or inline CSS, even when the same URL is declared in config. Checks inactive branches and parsed partials too. |
| `links.declared` | Error: final destination does not match a named URL after browser URL resolution. Requires the project registry; isolated HTML checks can omit it. |
| `links.behavior` | Warn: handler/role-based links without href, script/placeholder navigation, mixed HTTP resources, cross-origin download attributes and unresolved full same-document fragments. |
| `links.fragments` | Final DOM warnings for explicitly hidden same-document destinations; see the [HTML gate](../validate/README.md#default-rules). |

All are policy checks: `build.validation.rules` accepts `off`/`warn`/`error`.
Required template/schema safety checks remain mandatory. For a product that deliberately
constructs destinations, tune the narrow relevant rule, e.g. `links.declared: "warn"`.
Overrides never rewrite destinations or disable contextual HTML escaping.

Errors during preflight suppress advisories and preserve output; successful preflight
warnings join final HTML findings. Final findings can include the named config path
and related source location. Parser errors are reported by category, without dumping
signed query strings, passwords or complete URLs.

## Asset and vocabulary exceptions

- Local resource paths (`assets/...`, CSS/script files, emitted sidecars) stay normal
  file references. The asset compiler owns them. Navigation URLs such as `/other/`,
  `../help/`, `?mode=x` and anchor `#calculator` still need names.
- Generated sidecars match only actual emitted filenames on the configured origin
  and directory. An arbitrary undeclared `./whatever` navigation link is not exempt.
- `data:`/`blob:` resources and SVG/CSS `#symbol` references are exempt from ownership.
  Their codecs/payloads are not audited as link destinations.
- XML namespaces, CSS `@namespace`, and JSON-LD `@context`/`@type`/`@vocab` subtrees
  identify vocabularies; they are not managed destinations. JSON-LD blank-node `@id` values are also identifiers. Normal JSON-LD `url`,
  `@id`, `sameAs`, image and other URL-like values are checked.
- Code samples/comments/executable JavaScript are not scanned with URL regexes.
  Ordinary visible URL copy gets an advisory; examples inside code/pre are exempt.

## Parsing and behavior

Browser URL resolution uses the pure Go
[`nlnwa/whatwg-url`](https://github.com/nlnwa/whatwg-url) parser, pinned in go.mod,
with validation-error reporting. HTML uses x/net/html plus parse/v2 attribute tokens;
CSS token boundaries use parse/v2. Go's template AST separates literal HTML from
expressions, including quoted braces and trim markers. Hardcoded URLs also receive syntax
checks in that preflight batch; an existing config owner is suggested by exact name. URL lists and srcset candidates
are tokenized independently; commas inside data URLs are preserved.

Names compare by WHATWG-resolved serialization: relative/absolute forms, IDNs,
host casing and default ports can match. Go's actual href/srcset rendering forms
also match, accounting for context-specific percent encoding without URL prefix
matching or query rewriting. Query order, repeated parameters, case,
tracking fields and fragments stay significant. The compiler never sorts, strips
or decodes signed query parameters. Store raw `&` in JSON; html/template escapes it.

Explicit config `urls` values receive Go's URL type during contextual rendering,
allowing `tel:`, `sms:`, `webcal:` and custom application schemes. Script pseudo-URLs
and controls never receive that trust. This does not turn strings into trusted HTML,
JS or CSS. Unknown schemes warn rather than imposing a closed handler whitelist.
HTTP navigation is allowed; HTTP resources on HTTPS pages warn. Modern `_blank`
links do not get an obsolete mandatory noopener warning; outbound nofollow/sponsored
choices are not invented by the compiler.

## Runtime and scope

Expose runtime destinations with `build.browserData: ["urls.checkout"]` (or `"urls"`)
and read `JSON.parse(document.querySelector('#product-data').textContent).urls` in JS.
JS/CSS files are not template inputs. External URLs surviving CSS bundling into HTML must also
match declarations; prefer local imported assets or HTML-bound CSS values to avoid
copying endpoints into an untemplated stylesheet.

The final gate checks URL-bearing HTML attributes, srcset/imagesrcset, ping/archive
lists, supported social metadata, declarative refresh destinations, embedded JSON-LD and supported sharing JSON and CSS url()/@import/image-set
references in style elements/attributes. Arbitrary application/json data, including browserData, is not a link declaration. It validates actual rendered destinations,
including ones that a partial or expression assembled.

Source literal checks are an authoring lint, not a proof of provenance across arbitrary
computed strings or partials spanning attribute boundaries. Final matching plus unused
config warnings provides the backstop; opaque code can still duplicate a declared
value. Arbitrary JS URL construction/fetch calls, external CSS file contents, redirect
chains, DNS availability, HTTP status, certificates, mailbox existence and live crawler
behavior require runtime/integration checks. A syntactically valid URL can still 404.
Inline style findings name the containing tag; decoded attribute/list offsets can be
approximate after HTML character references. Other ordinary attributes name the value.
CSS descriptor conformance and opaque embedded media are outside this link gate.

```sh
go test ./internal/urlcheck
go test ./internal/check -run 'TestFinalURL|TestDeclaredURL|TestURLBehavior'
go test ./internal/compile -run 'TestNamedURL|TestURL|TestMultipleNamedURL'
go test ./internal/urlcheck -run '^$' -fuzz FuzzURLPolicy -fuzztime 15s
go test ./internal/urlcheck -run '^$' -fuzz FuzzHTMLReferences -fuzztime 15s
```

References: [WHATWG URL](https://url.spec.whatwg.org/),
[HTML links](https://html.spec.whatwg.org/multipage/links.html),
[Google crawlable links](https://developers.google.com/search/docs/crawling-indexing/links-crawlable),
[Go URL trust type](https://pkg.go.dev/html/template#URL).
