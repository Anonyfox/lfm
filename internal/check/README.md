# HTML quality gate

`build.Run` calls `Analyze` on the final serialization buffer before writing
`index.html` or replacing `output/`. Errors preserve the previous output; warnings
ship with actionable diagnostics. `build` and every `serve` rebuild share this gate.

Parsing: [`x/net/html`](https://pkg.go.dev/golang.org/x/net/html).
CSS selectors: [`Cascadia`](https://pkg.go.dev/github.com/andybalholm/cascadia).
Raw attributes: [`parse/v2/html`](https://pkg.go.dev/github.com/tdewolff/parse/v2/html).
All run locally in the Go binary. No browser, Node, Java or HTTP validator service.
The earlier [config/template gate](../templatecheck/README.md) shares diagnostics
and overrides; its warnings join this final report. The [link gate](../urlcheck/README.md)
adds `links.*` policy groups spanning config, template source and final output;
fragment discovery extends them in the final DOM gate.

## Architecture

- `document.go`: one final DOM, source tokens/byte locations, selectors and shared facts.
- `catalog.go`: the rule catalog with every rule's summary and help.
- `validate.go`: overrides, deterministic diagnostics and failure status.
- The catalog also registers build/bundle/hint/compaction/release notices; an AST
  regression test catches uncatalogued emitted IDs. Lighthouse owns a dynamic
  `lighthouse.<audit-id>` namespace, described by the `lighthouse.*` family entry.
- `rules_html.go`: syntax/repair detection, content models, IDs, references and JSON.
- `rules_jsonld.go`: block ownership and cached JSON-goLD expansion of exact script text.
- `rules_commerce.go`, `commerce_jsonld.go`: optional offer/checkout discovery, concrete
  Stripe secret shapes and focused semantic agreement on the same expanded JSON-LD.
  `internal/commercecheck` checks the optional public config contract before rendering.
- `rules_seo.go`, `rules_social.go`, `rules_discovery.go`: metadata, preview identity,
  snippet controls, readable structure and fragment discovery; shared static target index.
- `sharing.go`, `rules_sharing.go`, `rules_share_media.go`, `rules_discord.go`, `rules_farcaster.go`: declared
  platform contracts, strict JSON, initial metadata and bounded emitted-media inspection.
- `accessibility.go`: cached ID/label/composite facts, native disabled/focus helpers.
- `rules_accessibility.go`, `rules_relationships.go`, `rules_focus.go`, `rules_aria.go`:
  names, typed relationships, focus candidates and focused widget state checks.
- `rules_interaction.go`: form intent/numbers, native disclosures and autoplay requests.
- `rules_performance.go`: independent checks.
- `rules_fonts.go`: cached final CSS/font/preload analysis; exact emitted sidecars
  and already-read asset metadata supplied through Options. [Scope](../fontcheck/README.md).
- Matching `_test.go` files: small fixtures, individual rules, boundaries and fuzz seeds.
- `internal/compile/validation_test.go`: publication safety and actionable asset remedies.

Each rule is `Rule{RuleInfo{ID, Summary, Help}, Check func(*Document) []Diagnostic}`.
Register it in `DefaultRules` (`catalog.go`) with its one-line Summary and a 2 to 5
sentence Help (trigger, why it matters, how to resolve); `lfm rules <id>` prints both
and a test rejects missing or malformed help. Adding a rule does not change the build pipeline. Findings
carry severity, stable rule ID, file, UTF-8 byte offset/column, line, element,
explanation and fix. No source/data-URL payloads are dumped into diagnostics.

```go
doc, err := validate.Parse([]byte(`<img src="hero.webp">`), validate.Options{})
if err != nil { t.Fatal(err) }
// Select a single rule; fragments do not need metadata for unrelated rules.
for _, rule := range validate.DefaultRules() {
    if rule.ID == "a11y.images" {
        report := validate.Run(doc, []validate.Rule{rule}, nil)
        // Assert report.Diagnostics, severity, location and edge cases.
    }
}
nodes, err := doc.Query(`main > section:nth-child(2) img:not([alt])`)
```

## Default rules

| ID | Default findings |
| --- | --- |
| `links.syntax` | Errors for broken URL syntax/protocols; warnings for browser repairs and suspicious spelling |
| `links.declared` | Errors for rendered destinations missing from the project URL registry |
| `links.behavior` | Warnings for crawlability, mixed content, ineffective downloads and full same-page fragment targets |
| `links.fragments` | Warnings: linked same-document targets blocked by explicit `hidden` on themselves/ancestors; one per target with link/blocker locations. Native closed details and `hidden=until-found` remain allowed. |
| `html.syntax` | Errors: malformed attributes/tokens, duplicate attributes, invalid head content, structural browser repairs |
| `html.document` | Errors: missing/legacy doctype, invalid/duplicate/late charset; missing charset warns |
| `html.content-model` | Errors: supported restricted containers and invalid interactive/phrasing nesting |
| `html.ids` | Errors: duplicate/empty/whitespace IDs in the active document |
| `html.references` | Errors: missing static ID references; unresolved fragment navigation warns |
| `html.json` | Errors: invalid JSON, JSON-LD, import maps or speculation-rules JSON |
| `jsonld.block` | Errors: handwritten/extra JSON-LD blocks in projects; one optional generated block |
| `jsonld.structure` | Errors: invalid JSON-LD expansion/keyword shapes, bounded input limits or processor failure |
| `jsonld.context` | Warnings: expansion coverage skipped for an unbundled remote context; no network requests |
| `jsonld.dropped` | Warnings: undefined properties discarded by expansion or no resulting node data |
| `commerce.checkout` | Errors: declared checkout is not HTTPS/public-buyer navigation or its standard-host query is malformed. Warnings: test/session destinations, suspicious hosted paths, unsupported/repeated/invalid parameters, conflicting prefills and form/download handoffs. Custom domains are preserved. |
| `commerce.discovery` | Warnings: optional declared offer has no initially usable native checkout anchor; fixed-price snapshot has no identifiable initial Schema.org Offer. Explicit HTML state is checked; CSS/JS visibility requires browser review. |
| `commerce.structured-data` | Errors: negative Offer price, exact identifiable same-currency config mismatch, non-public/non-HTTPS potential BuyAction target. Warnings: incomplete/ambiguous price facts, alternate currency, absent/different action target. Offline expanded aliases and same-graph ID references are supported; ambiguous identities/context/precision are not guessed. |
| `commerce.secrets` | Errors: recognizable Stripe sk_/rk_/whsec_ secret shapes in config, final HTML or emitted text sidecars, with redacted values. Public pk_ keys/object IDs are allowed; this is not a general credential/PII scanner. |
| `seo.metadata` | Errors: duplicate/out-of-head HTML titles; missing/duplicate/out-of-head canonical, non-public/non-HTTPS canonical, any query/fragment, or spelling different from configured url. Missing title/description/lang, duplicate/misplaced description and unrecognized BCP 47 language warn. SVG titles and inert templates are excluded. |
| `seo.indexability` | Warnings: explicit noindex/none and page-wide nofollow, including Googlebot News; restrictions in body still count. |
| `seo.snippets` | Warnings: nosnippet and nonnegative max-snippet limits for recognized robots names; -1/unknown/malformed directives remain untouched. Repeated limits report the most restrictive value per tag. |
| `seo.social` | Errors: invalid/non-absolute/data/local/private preview-identity or image URLs. Warnings: missing OG core fields, misplaced/wrong-attribute/empty metadata, repeated scalar fields and page identity mismatch. Public-state query variants, OG image arrays/scoped properties and different listing wording/images are allowed. |
| `sharing.discovery` | Errors: repeated/empty known scalar platform fields, invalid public audio/video/stream/oEmbed URLs or nonpositive media dimensions. Warnings: misplaced/wrong-attribute declarations, metadata after 32 KiB, incomplete Slack label/data pairs, unrecognized/legacy X card types and ineffective meta CSP frame-ancestors. Arrays and optional unused platforms remain allowed. |
| `sharing.media` | Warnings: missing per-image OG alt/dimensions, missing twitter:image:alt or dimension hints disagreeing with inspected sidecars. Image arrays and deliberately empty alternatives remain allowed; remote media is not fetched. |
| `sharing.x-player` | Errors for a declared player missing HTTPS player/poster or positive integer dimensions; warnings for missing title/description fallback or a player target without card=player. Query/anchor launch state is preserved; host framing/client behavior is not certified. |
| `sharing.discord` | Errors: duplicate/misplaced/ill-typed declarations; unrelated linked-JSON host; malformed/duplicate-key JSON, >3,000 encoded bytes, wrong root/nesting/known parent shapes, unsupported types/button keys/styles, missing names/URLs, >40 components or >10 total gallery items. Linked emitted JSON is inspected; remote JSON and absent native return button warn. |
| `sharing.farcaster` | Errors: malformed/duplicate-key MiniAppEmbed JSON, wrong version/button/action, invalid HTTPS targets, image URL >1,024 characters or known emitted image format/byte/dimension violations. Uninspected/uncertain media warns. Optional inherited action URL, launch_frame and legacy fc:frame=vNext remain supported. |
| `sharing.snapchat` | Errors: invalid public sticker URL or known emitted sticker format/byte/dimension violations. Remote/uninspectable media warns. Transparency/animation and native-client tapping need review. |
| `seo.headings` | Warnings: missing visible h1, unreadable headings or skipped levels; hidden/inert headings do not establish the hierarchy. Multiple h1 remains allowed. |
| `seo.content` | Warnings: absent/multiple visible main landmarks, no detectable initial primary text, or all such text excluded by supported data-nosnippet containers. No length/quality score. |
| `a11y.images` | Warnings: absent alt; intentional `alt=""` is allowed |
| `a11y.controls` | Warnings: unnamed native/custom controls, summaries, dialogs, frames and supported informative roles; native labels, ARIA references, image alternatives and SVG titles considered. |
| `a11y.relationships` | Errors: known non-labelable for targets, extra/mismatched controls inside labels, wrong form/list/popover/table-header target types. Missing IDs belong to html.references; form-associated custom elements are not rejected. |
| `a11y.aria` | Warnings: focused ARIA token/numeric grammar, ranges and missing checked/slider/combobox state. Native state/defaults, optional empty states, unknown/new attributes and indeterminate progress remain allowed. |
| `a11y.focus` | Warnings: invalid/positive tabindex, duplicate autofocus within a scope, tab candidates under aria-hidden, custom standalone controls without a static focus path, and non-semantic click targets. Inactive HTML, disabled fieldsets/legend exceptions, managed composites and delegated container handlers are distinguished. |
| `a11y.link-purpose` | Warnings: two or more visible links with one accessible name and different href values. |
| `ux.forms` | Warnings: ambiguous/invalid form-button types and malformed numeric bounds/steps/starting values; native invokers and explicit form owners are recognized. |
| `ux.disclosures` | Warning: missing authored summary. Errors: multiple/misplaced native summaries or built-in dialog/popover commands targeting the wrong element type. |
| `ux.media` | Warnings: media requests autoplay without initial mute; actual audio, duration and playback policy need browser review. |
| `ux.reduced-motion` | Warning: moving keyframes, scroll-driven animation, smooth scrolling, transform transitions ≥400 ms or scripted transform animations without any prefers-reduced-motion query in styles or scripts. Opacity/color animation is not motion. |
| `perf.html-size` | Warning at 1,800,000 bytes; error above 2,000,000 uncompressed bytes |
| `perf.inline-assets` | Warnings: >32 KiB embedded contributions and embedded srcset candidates |
| `perf.images` | Warnings: unreserved image dimensions, conflicting lazy/high priority, substantial lazy inline images |
| `perf.image-assets` | Warnings: bounded metadata failures; sources >512 KiB, edges >4096 px or area >12 MP; >2× an unambiguous authored box on both axes. No guessed CSS slots or density-candidate comparisons. Source attribution follows packed URLs. |
| `perf.fonts` | Warnings: missing/invalid family/src/display/range, source-format mismatches, no known WOFF2 source, preload CORS/type/selection conflicts and >2 font preloads. Typography choices are preserved. |
| `perf.font-assets` | Warnings: bounded container failures/inspection limits; >128 KiB per retained font, >256 KiB total, >8 MiB declared SFNT size. No glyph/payload sanitization or proven download count. |
| `perf.viewport` | Warnings: missing/duplicate viewport or disabled zoom |
| `perf.budgets` | Warnings: total inline + file CSS >20 KiB, JS >50 KiB, >1,500 elements, depth >32 |
| `design.palette` | Warnings: stock Tailwind indigo/violet/purple accents (v3 hex and v4 OKLCH), gradients whose colored stops are all violet/blue/pink, and the assistant cream + terracotta pair. Rainbow/iridescent gradients, lone off-white paper and lone terracotta stay quiet. |
| `design.gradient-text` | Warnings: text filled by a gradient through `background-clip:text` (CSS or Tailwind classes). Image-filled text is not flagged. |
| `design.glow` | Warnings: two or more heavily blurred (≥24px) round or radial decorative shapes; zero-offset colored glows on three or more elements. A single orb or glow only feeds `design.defaults`. |
| `design.eyebrows` | Warnings: a short uppercase label, tracked or small (or named eyebrow/kicker/overline), directly before an h1–h3. Labels containing digits or currency (time, step, price) only feed `design.defaults`. |
| `design.type` | Warnings: one stock sans (Inter, Roboto, Arial, system UI, Geist…) for the headline and body with no distinct heading face; an italic-serif or gradient accent word inside a longer headline. Roster display faces and color-only accents are weak. |
| `design.emoji` | Warnings: emoji (Emoji_Presentation) in headings, buttons, short links, summaries, labels, legends, table/definition headers or as list bullets; ✨ explained separately. Typographic ornaments, check marks, stars and body-copy emoji are allowed. |
| `design.hero` | Warnings: a pill or "New/Introducing/✨" badge before the first h1; the centered formula (badge + centered h1 + two actions). A merely centered hero is weak. |
| `design.cards` | Warnings: three or more structurally identical siblings, each led by an icon (svg/img/i/icon class) and a heading. Plain equal cards and 01/02/03 markers are weak; nav is excluded. |
| `design.stats` | Warnings: three or more sibling stat blocks with big round numbers (10k+, 99.9%, 50+). Plain number strips are weak; forms, tables and outputs are excluded. |
| `design.stripes` | Warnings: a 2–8px colored left/inline-start border as the only border on two or more surfaced boxes. Quotations, lists, table cells and diffs are excluded. |
| `design.glass` | Warnings: `backdrop-filter: blur()` on three or more elements. One or two floating layers only feed `design.defaults`. |
| `design.surfaces` | Warnings: Tailwind's default soft shadow geometry (or shadow-md…2xl with rounded classes) on three or more boxes. Uniform radius+shadow, pills everywhere and dot grids are weak. |
| `design.motion` | Warnings: scroll-reveal hooks (data-aos, reveal/fade-up classes…) on four or more blocks. Hover lift on many elements is weak; scroll timelines and bespoke motion are not judged. |
| `design.arrows` | Warnings: arrow glyphs ending at least half of four or more buttons/links. External ↗ links and links inside prose are excluded. |
| `design.chrome` | Warnings: four or more "A · B · C" meta strings; fake red/yellow/green window dots. Class names alone never trigger the dots check. |
| `design.mono` | Warnings: monospace on eight or more non-code labels/captions. Code, outputs, times, data, tables, form controls and SVG text are excluded; three to seven is weak. |
| `design.proof` | Warnings: generic "trusted by / as seen on / join 10,000+ teams" claims; placeholder image services (pravatar, randomuser, ui-avatars, picsum, placeholder hosts). "Most popular" badges and tilted 3D product mockups are weak. |
| `design.sections` | Warnings: four or more stock sections (how it works, testimonials, pricing, FAQ, logo strip, feature grid, stat strip). Three is weak. |
| `design.defaults` | Warning: four or more distinct generator-default signals including at least one reported finding, or five weak ones; lists both groups and counts justified elements. Weak signals never warn alone. |
| `copy.tells` | Warnings: narrow stock phrases of generated copy (English and German), repeated not-just-but and it's-not-it's constructions (German nicht nur … sondern from three), and ≥4 em dashes above one per hundred words. Quotations, code and data-design-reason subtrees are exempt. |
| `copy.placeholder` | Warnings: lorem ipsum, TODO/TBD/FIXME (case-sensitive), "your … here" and [insert …] filler in text, alt, title and description metadata; after the starter page is replaced, the lfm init description, social alt text and share image. |
| `concept.fidelity` | Warning: with a concept.md, font families not declared in its Type table or declared and unused, CSS colors outside the palette (tints and translucent black/white allowed), planned imagery missing from the page. |

Size thresholds use decimal bytes conservatively, following the current
[Googlebot limit](https://developers.google.com/search/docs/crawling-indexing/googlebot).
The warning leaves headroom; this is a compiler policy, not a ranking guarantee.
CSS/JS/DOM budgets are advisory authoring defaults, not standards limits.

The report ranks data-URL occurrences by their total contribution, counts repeated
embeds, and associates compiler-produced URLs with every known source asset.
Automatic packing externalizes eligible resources before this final gate. Remaining
size warnings/errors identify pins/pre-authored embeds and suggest source/override fixes.
Unknown/pre-authored data URLs still have MIME, bytes and a final-document location.
Compression does not change the HTML size gate.

## Generator-default audit

Implements the deterministic subset of UI/UX generator defaults.
`style.go` builds one small cascade from every final stylesheet (inline and emitted sidecars):
Cascadia matching, specificity/source order, `!important`, inline styles, inheritance of
typographic properties and simple `var()` lookups. It resolves the desktop/base context and
ignores print, reduced-motion, dark-scheme and max-width-only rules. Colors are compared in
OKLab. No layout, rendering or browser is involved.

- **Strong signals** are recognizable stock signatures and warn individually with the exact
  element or stylesheet location, why it reads as generated and the smallest alternative.
- **Weak signals** (single orb or glass layer, centered hero, pills everywhere, roster fonts,
  plain equal cards, grey body text on a dark solid backdrop…) only count in `design.defaults`.
- **Deliberate use:** `data-design-reason="why this subject needs it"` on an element or ancestor
  exempts that subtree; the combination finding reports how many signals were justified.
  Whole rules follow `build.validation.rules` like every other ID.
- **Limits:** class/selector heuristics cannot judge taste, layout geometry, scripted styles,
  canvas output or whether a pattern suits the subject. Review with the guide's swap and
  deletion tests; the audit removes the mechanical part of that review.

## SEO/GEO audit coverage

The SEO/GEO guide defines the authoring contract;
Content/copy owns wording. Normal builds enforce
the reliable local subset on final bytes without rewriting content or adding knobs.

| Guide requirement | Automatic coverage / remaining review |
| --- | --- |
| Initial useful answer/tool explanation | `seo.content` detects an empty static shell; intent, usefulness, factual accuracy, evidence and sufficient explanation require review. Heading text alone can satisfy the structural floor. |
| Document and reading structure | `html.*`, `perf.viewport`, `a11y.*`, `seo.headings`, `seo.content`; computed CSS visibility, visual order and full accessibility require browser review. |
| Title/description/canonical/language | `seo.metadata`: exactly one initial-head canonical equals configured url, HTTPS/public with no query/fragment; config also rejects empty ?/#. Wording relevance and displayed engine rewrites require review. No character limits. |
| Shared identity and public preview | `seo.social` checks OG title/type/url/image, URL identity and metadata shape. It neither requires identical headlines nor downloads/decodes public previews. |
| Crawlable links and direct entry | `links.*`, `html.references`; IDs, legacy named anchors, percent-encoded IDs and text-fragment directives are distinguished. Shared state queries retain the page's base canonical. CSS/JS scroll resets, handler-driven reveal and incoming site links require review. |
| Snippet/index restrictions | `seo.indexability`, `seo.snippets`, `seo.content`; intentional restrictions are retained. HTTP headers, robots.txt, status codes and real crawler access are publication checks. |
| Structured-data agreement | `jsonld.*` validates one optional block and offline JSON-LD structure; `links.*` owns destinations. `commerce.*` checks identifiable offer-price/config agreement and purchase targets. Freeform visible-fact agreement, entity/type appropriateness and engine eligibility require review. No schema/citation-score invention. |
| Intent, clusters, extraction, outcomes | Semantic task fit, source quality, internal-link relevance/orphans, retrieval/citations, real listings and conversion measurement remain author/publication work. |

Known-private URL rejection covers literal local/private IPs and reserved local/test
hostnames; it is not a DNS or reachability test. Initial text checks exclude scripts,
styles, templates, JS-disabled fallbacks, hidden/ARIA-hidden content, canvas/SVG/media
and form-value text. Closed details contribute their summary; open details contribute
their content; unopened dialogs are excluded. Ordinary static CSS is not interpreted. Heading/accessibility names
may use image alt or ARIA labels without treating those labels as an explanatory body.
Fragment warnings describe explicit HTML blockers; CSS can override hidden states.

Contracts: [Open Graph fields/arrays](https://ogp.me/),
[robots/snippet controls](https://developers.google.com/search/docs/crawling-indexing/robots-meta-tag),
[native fragment reveal](https://html.spec.whatwg.org/multipage/interaction.html#the-hidden-attribute).

## UI/UX audit coverage

The UI/UX guide owns interaction/accessibility requirements.
Every normal build checks the reliable HTML subset below, without changing authored
behavior, adding a browser dependency or expanding CLI/config syntax.

| Guide requirement | Automatic coverage / remaining review |
| --- | --- |
| Named semantic controls | `a11y.controls`, `a11y.images`, `links.behavior`: native/custom names, native label associations, ARIA name references, image alternatives and SVG titles. Placeholder/current value is not a field label. Full accessible-name computation, CSS-generated text and shadow/ElementInternals semantics need browser review. |
| Labels and related state | `a11y.relationships`, `html.references`: labelable target types, one associated control per label, form/datalist/table-header/popup/command targets, ARIA error/detail/flow references. Semantic instruction quality and truthful displayed state remain author work. |
| Keyboard and focus order | `a11y.focus`: invalid/positive tabindex, duplicate autofocus per native scope, custom standalone focus entry, non-semantic click targets and tab candidates under aria-hidden. Hidden/inert views, disabled fieldsets and their first-legend exception, native summaries, roving negative tabindex and active-descendant owners are distinguished. Actual keyboard handlers, focus visibility/order, modal containment/return and gesture alternatives require browser tests. |
| Explicit widget/status state | `a11y.aria`: common boolean/token/live-region values, finite numeric values/bounds, checked state, current slider value and combobox expansion/popup association. Native state/defaults, optional empty ARIA, indeterminate progress and future/unknown vocabulary are retained. Live announcements, state updates and full ARIA role/ownership conformance need runtime review. |
| Forms, inputs and recovery | `ux.forms`: implicit/invalid button types, explicit form owners and malformed/out-of-bounds numeric defaults/steps. Command invokers are recognized. Input necessity, units, precision, step mismatch, validation timing, error recovery and persistence remain task/browser checks. |
| Accordions, dialogs and popovers | `ux.disclosures`, `a11y.controls`, `a11y.focus`: native summary structure, names, target kinds and autofocus scopes. Native HTML and custom patterns remain available; close/Escape, trapping, background interaction and return behavior need testing. |
| Media and reduced motion | `ux.media` warns on an unmuted autoplay request. It does not infer audio tracks/duration, silence media or require a player for every decorative clip. Captions/equivalents, pause/stop, reduced motion and interruption need browser/media review. |
| Layout, mobile and touch | Existing document/viewport/image gates apply. Contrast, target spacing, zoom/reflow, clipping, software-keyboard clearance, sticky overlays and real assistive/in-app-browser behavior require rendered checks. Optional release/Lighthouse findings supplement this. |
| Task flow, taste and conversion | Narrative/composition, truthful results/progress, back/edit/cancel, sharing, checkout transfer and business outcomes remain authored acceptance tests; no generic aesthetic/conversion score. |

Hard findings identify broken native structure/associations. Warnings identify
supported static evidence or a behavior to verify; delegated JS, custom pause controls
and CSS visibility cannot be inferred from markup alone. Findings include fixes and
final-file locations; relationship/focus conflicts add the other element's location.
Naming also checks closed native dialogs/disclosures; explicitly hidden/ARIA-hidden
controls are excluded. Focus-candidate checks account for initial native inactive/disabled
state. Inert templates are excluded.
No handler execution, CSS layout or resource fetching occurs in normal validation.

Contracts: [ARIA 1.2](https://www.w3.org/TR/wai-aria-1.2/),
[accessible names](https://www.w3.org/TR/accname-1.2/),
[native forms](https://html.spec.whatwg.org/multipage/form-elements.html),
[native focus](https://html.spec.whatwg.org/multipage/interaction.html#the-autofocus-attribute),
[keyboard patterns](https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/).

## Sharing/distribution audit coverage

The sharing guide owns platform design.
Normal builds validate declared integrations only; they do not require a social
SDK, account or extra metadata for platforms a product does not use.

| Contract | Static coverage / remaining review |
| --- | --- |
| Canonical and state | `seo.metadata` enforces exact base identity; `seo.social` permits intentional preview query variants. The compiler restores only Go's mechanical URL-attribute escaping of the configured canonical, including Unicode. Recipient JS, query inheritance, editing/history and private-data omission need authored tests. |
| Crawler discovery | `sharing.discovery`, `sharing.media`, `seo.social`: initial fields/placement, image alternatives/dimension hints, public media, Slack pairs and early-metadata advisory. Meta frame-ancestors cannot set HTTP framing policy. Actual headers, cache behavior, endpoint status and client permissions require deployment review. |
| X player | `sharing.x-player`: declared target, dimensions, poster and text fallback. It cannot establish inline play, framing, form/API acceptance or a return link inside a separately hosted player. |
| Discord components | `sharing.discord`: focused component structure, byte/count limits, typed buttons, media/return targets, linked host relationship and emitted JSON. Optional/new component fields are not a complete remote schema check. Markdown links are retained, not parsed as entire URLs; their meaning/destinations and actual tappable return path need review. |
| Farcaster / Snapchat | `sharing.farcaster`, `sharing.snapchat`: declared payloads and bounded inspection of emitted images. Remote media warns as uninspected. Manifest/SDK registration, signed identity, transparency, animation and real recipient/client behavior need review. |
| Other surfaces | Existing public metadata, named links, JSON-LD and native control gates apply. Teams eligibility, Standard.site records, Mastodon domain verification, mini-app packaging, Web Share support, exports and platform SDK actions need their respective acceptance tests. |
| Distribution outcome | Recipient value, attribution legibility, invitation placement, stateful continuation, human visits and commercial results remain product review/measurement. No guessed virality or mandatory generic share button. |

Already-read sidecars are supplied in `Options.Assets`; no file/network reads or
pixel decoding occur in validation. Byte limits apply to exact emitted JSON/media;
image dimensions use bounded headers. Unknown remote resources are not reported as
verified. Asset packing resolves known local media fields in inline platform JSON
to public sidecars, preserving navigation/state and unknown fields. Ambiguous JSON
is never repaired before validation; linked JSON contents stay authored.
Both Discord JSON MIME aliases are recognized; use `application/json`
for inline Go-template interpolation so raw script strings receive JSON escaping.
The vendor MIME can otherwise leave `&amp;` literally inside JSON URLs; known config
URL mismatches get a specific repair hint. Existing severity overrides apply.

Contracts checked **2026-10-16**:
[Discord link-preview payloads](https://docs.discord.com/developers/link-previews/component-embeds),
[component fields](https://docs.discord.com/developers/components/reference),
[Farcaster embeds](https://miniapps.farcaster.xyz/docs/guides/sharing),
[Snapchat stickers](https://developers.snap.com/snap-kit/creative-kit/web).
X's observed compatibility boundaries and examples remain in the sharing guide.

## Commerce audit coverage

Commerce coverage follows Commerce.
Config-stage `commerce.config`, `commerce.price` and `commerce.fields` are registered
alongside final rules in `lfm rules`; the same overrides apply before rendering.
The optional `data.commerce.offers` convention keeps unknown fields, quote/variable
prices, custom domains and handwritten layouts available. Compiler-consumed operational
fields do not produce unused-value noise. No Stripe requests or Caddy file generation
occur during a build. Published price/billing/tax basis, live provider state, client
capabilities and deployment/fulfilment remain acceptance checks.

## Overrides and testing

Everything runs automatically. Optional config under `build.validation.rules`
maps rule IDs to `"off"`, `"warn"` or `"error"`; unknown IDs/levels fail configuration.
An override changes all findings from that rule. Required config/template execution
checks accept only `error`; advisory template checks can also be tuned. Omitting it keeps default levels.
Use `lfm rules` for the catalog and lfm.json syntax; `lfm compile` always reports diagnostics.

Lifecycle notices obey the same controls. `"release.browser": "off"` suppresses
that setup advisory; setting it to `error` rejects a release before output replacement.
`"lighthouse.*": "off"` controls all installed-version audit findings; an exact
`lighthouse.<audit-id>` override takes precedence. The audit still runs and reports
scores/manual checks. Unknown fixed IDs and invalid levels remain config errors.

```json
{"build":{"validation":{"rules":{"a11y.images":"error","perf.budgets":"off"}}}}
```

```sh
go test ./internal/check -run TestSyntax
go test ./internal/compile -run TestValidationGateAndExactBytes
go test ./internal/check -fuzz FuzzAnalyze -fuzztime 10s
go test ./internal/check -run '^$' -bench Analyze -benchmem
```

## Scope and source evidence

`x/net/html` intentionally repairs malformed markup. We additionally compare its
parsed/serialized structure with original token events, allowing legal optional
end tags and implied document/table containers. Raw attribute checks use a lexer
because x/net drops duplicate attributes. A rendered-template check prevents the
compiler's first parse from hiding authoring errors before the final gate.

Final findings name `index.html`. Supplementary findings name the virtual
`rendered-template.html`: lines belong to expanded template output, not original
partial/template line numbers. Analysis never mutates the serialization buffer.

This is a focused, extensible static validator, **not complete WHATWG conformance
certification or an accessibility audit**. The repair detector relies on x/net's
tree builder; foreign text is delegated to it. Content models cover the listed
restricted structures; arbitrary attributes, every HTML context, CSS layout,
computed accessible names, browser API behavior, network resources and live Core
Web Vitals require additional checks. Inert template content is excluded from
active metadata/ID/reference/name checks. No network request is made.
