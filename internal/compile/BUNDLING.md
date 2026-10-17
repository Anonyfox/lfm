# Bundling contract

HTML script/style helpers declare entries → esbuild's Go API resolves
the graph → one minified JS blob and one CSS blob → inlined by default.
No source files, package manifests or dependency trees are deployed.

## Sources

```gotemplate
{{style "theme.css"}}
{{script "features/calculator.ts"}}
{{script "installed-browser-package"}}
{{style "installed-browser-package/theme.css"}}
```

One string argument; config bindings work too. Existing local files resolve first
from the source template/partial; bare paths also try the project root. Explicit
`./` and `../` remain source-relative. Remaining specifiers use esbuild's native
package resolver. Paths may live anywhere inside the project; `assets/` is only
a convention. Absolute paths are accepted only inside that same source boundary.
Sources may import each other normally; nothing is copied to synthetic source files.

Helpers must occupy an HTML element position. Repeated declarations register once
per active page or dormant HTML template, preserving first registration order.
Inactive branches are checked without registering their assets; failures there warn.
All active resolution/type failures join template preflight diagnostics before staging.


```js
// assets/scripts/main.js — both sources can coexist
import clsx from "clsx";                  // installed package
import helper from "./vendor/helper.cjs"; // vendored file
import "../styles/main.css";
```

- Optional `package.json` + lockfile at project root; run `npm ci` during setup
  (`npm install` to create/update the lockfile). The compiler does not install,
  download or run package scripts. No Node executable is needed to build afterward.
- Relative imports resolve vendored files anywhere in the project; package imports resolve
  local `node_modules/`. Nested dependencies/scoped packages, `exports`, `browser`,
  `module`/`main`, JSON, TS/TSX/JSX, `.mjs`/`.cjs` and `.mts`/`.cts` use esbuild's
  resolver/loaders. Browser platform defaults and production minification are shared.
- Package metadata/file extensions still describe the source accurately. There
  are no per-library ESM/CommonJS switches. Node-only APIs are not polyfilled;
  choose a browser build. JSX uses esbuild defaults; framework macros need their
  own precompilation. ES2020 remains the syntax target (no top-level await).
- Imported package fonts/images follow the same inline/file rules as local assets.
  Symlinks must remain inside the project; external workspace links need local
  vendoring/copies. Only actually emitted dependency assets become sidecars.
- HTTP/site-root/existing data references remain explicit external references.
  Remote SDKs/stylesheets are not downloaded or magically merged into local blobs.

## Composition and semantics

- Prefer `{{script "assets/scripts/main.js"}}` and `{{style "assets/styles/main.css"}}`.
  Script helpers are module-graph entries (including plain JS and CommonJS).
  Native HTML tags remain available for classic globals and loading attributes.
  Untyped ESM/CommonJS entries are detected and emitted as modules automatically.
- Compatible local module tags share one synthetic entry/graph: shared modules
  initialize once, import order follows esbuild's module semantics, splitting is
  disabled, and HTML-only entry exports can be removed. Explicit page entries
  remain roots even if a package marks files `sideEffects:false`.
- CSS links/style blocks and CSS extracted from JS feed a shared CSS graph.
  Authored order is preserved; JS-imported CSS follows authored styles. Media
  attributes become conditional imports/wrappers, preserving cascade conditions.
  esbuild owns CSS imports, nesting, layers and minification. `.module.css` imports
  from JS expose generated local class mappings; style helpers expect global CSS.
  CSS Modules share one JS graph; independent CSS-module graphs fail explicitly
  because separate minifiers can generate colliding class names. Use one helper
  graph/asset mode, or global CSS for deliberately separate loading groups.
- Async, remote, classic-global and independently addressable tags are boundaries.
  IDs, CSP nonces, alternate/disabled sheets and other behavioral attributes are
  not discarded to force a blob count. Native alternate/addressable linked sheets
  remain file-backed; their title/media/disabled behavior is retained.
  Deferred/async classic sources and their load/error handlers retain file-backed
  loading too, since inlining a classic script would ignore those semantics.
- Plain classic global scripts retain names/scope and remain separate. For a
  standalone UMD browser-global script, use `data-lfm-classic` on its script tag;
  it prevents CommonJS wrapping from selecting the wrong UMD branch. This marker
  is removed and is incompatible with `type=module`. Prefer importing UMD into
  the application when its global branch is not needed.
- Styles/scripts inside HTML `<template>` are compiled separately and stay inside
  the template. They must not affect the active page before instantiation.
- Normal packing favors inline; large compatible groups can automatically become
  files. Legacy `build.assets.default` / entry-path overrides pin a policy.
  Different explicit modes are separate groups. Single-entry groups keep their
  basename; merged groups use `bundle.js` / `bundle.css`; separate groups number
  those names. Template-local file groups also receive a template prefix.
- CSS is placed at the end of head; runtime JSON/JSON-LD precede executable JS
  near the end of body. Inline style attributes remain attributes.

## Elimination

- ESM declaration tree shaking, constant/unreachable-code folding and minification
  are on. Shared dependencies deduplicate within a graph. Effects are retained;
  legal comments are retained inline. CommonJS supports bundling/interoperability
  but cannot offer ESM's fine-grained export elimination.
- Normal `sideEffects` / pure annotations are honored. A package falsely declaring
  its CSS/effectful modules side-effect-free can break itself; include CSS in its
  sideEffects list. We do not globally override dependency annotations.
- Unused imported file assets are emitted through esbuild's file loader, so removed
  imports do not leave useless sidecars. Unreferenced source directories stay out.
- CSS minification is **not** a proof that a selector is unused. Quiz results,
  dialogs, hover/focus, responsive states and JS-created classes need rules that
  may not match the initial static DOM. No automatic selector purge is enabled.

## Automatic packing and resource hints

No asset settings are needed for the normal workflow. `auto` is the implicit
default; legacy global inline/file policies and per-source overrides remain escape
hatches. Algorithm constants are maintained in Go, not copied into every product.

- Collect native intent before rewriting: responsive/picture candidates, lazy
  loading, media, download links and dormant template ancestry. No guessed viewport
  geometry or automatic hero priority from document order.
- Decide obvious file candidates before data-URL encoding. esbuild's native file
  loader handles large imported resources so unused imports still emit nothing.
  Auto binary URLs use deterministic source-path suffixes shared by HTML/CSS/JS;
  this prevents cross-import basename collisions and keeps URLs stable across
  content updates. Explicit file policy retains basenames; content hashing is separate.
- Compile shared graphs first. Count generated data URLs in resource attributes,
  CSS and executable JS, including generated sidecars. Ignore visible text/inert
  JSON for rewrite purposes; unused imported assets stay out.
- Deduplicate identical resource bytes and aggregate their occurrences/roles.
  An explicit inline pin protects identical-token aliases too.
- File candidates: responsive alternatives/media; fonts and lazy/dormant media
  >4 KiB; total encoded resource contribution >32 KiB. Otherwise keep inline.
- Under whole-document pressure, greedily move the largest eligible HTML
  contributions out toward a 256 KiB soft target. Keep tiny/pinned resources.
  These are reproducible heuristics, not universal transfer-latency optima.
- Reconcile all selected tokens together; preserve fragment references and encode
  filenames safely for unquoted CSS URLs. Explicit file-mode collisions still fail.
  Report source/aliases, original
  bytes, encoded contribution, occurrences, mode, emitted name and reason.
- After resource packing, externalize compatible CSS >20 KiB / module JS >50 KiB.
  Preserve native classic/addressable boundaries and `import.meta` modules.
  Media packing must not strand a now-tiny stylesheet in an unnecessary file.
- Rewrite/dedupe authored preload/modulepreload/prefetch references against final
  files. Drop hints for embedded/dead resources; unused hints warn. Responsive
  candidates follow selected file URLs; authored font preloads get missing anonymous
  CORS for local/remote URLs and missing MIME type for inspected local containers.
  Explicit credential/type choices remain authored; ineffective choices warn.
- Inspect retained font containers and exact final font-face CSS. Recognized renamed
  fonts follow font packing/MIME policy too. Display/metrics/source order/subsets stay
  authored; no decompression, subsetting, format conversion or guessed preloads.
  [Font scope and checks](../fontcheck/README.md).
- Add at most one modulepreload for the first generated body-end startup module.
  Native CSS already in head needs no duplicate hint; unknown LCP/font/background
  needs are not guessed. Dormant templates never trigger eager hints.

Gzip-level-1 reporting is a cheap estimate, not deployment compression. Total code
budgets count file-backed work. SVGs choose lossless percent/base64 encoding with
a pooled fast-DEFLATE estimate; source bytes and URL fragments remain intact.
After transforms, conservative HTML attribute-syntax compaction must reproduce
the complete parsed DOM before its exact bytes enter validation/audits.
Text whitespace and raw fallback content are preserved. Reports expose encoding
and syntax savings; both scripting modes have regression coverage.
Dependency attribution and browser-observed critical-resource tuning remain open.

## Optional release pass

`lfm compile --release` uses installed Chrome/Chromium through native Go
CDP. `LFM_CHROME` can point to a nonstandard executable. Normal builds do no
browser work. Missing/broken optional browsers warn and retain ordinary output.
Separate release navigation checks enable application JS; installed Lighthouse
adds real lab scores/actionable findings. See the
[release/reporting contract](../../docs/compiling.md#release).

- Only one ordinary local sheet is eligible; keep native boundaries, authored CSP,
  remote dependencies and small sheets (<8 KiB) unchanged.
- Observe static first paint at mobile, tablet and desktop widths. Union rules for
  visible elements/ancestors; retain pseudo/dynamic selectors and global/layer/
  scope/container definitions conservatively. Media/supports nesting stays ordered.
- Minify the candidate **before** checking critical-only and critical+full versions
  against original computed styles, layout and exact viewport pixels. Freeze
  animations. If ancestor geometry changes, retry with all statically matched
  rules included (`selection: document`); remaining differences retain full CSS.
- Require ≥2 KiB and ≥20% less blocking CSS. Report applied/skipped and byte counts.
- Inline critical rules, emit/reuse the **complete original** minified CSS sidecar,
  load it via native `media=print`/onload, and include a noscript stylesheet fallback.
  No selector purging; all later interactions/responsive states keep their CSS.
- Analysis uses a fresh temporary profile and an artifact-only loopback handler.
  Preview CSP blocks application JS; request interception rejects all other URLs.
  45-second total deadline; browser profiles/processes/staging clean up on exit.

This is static first-screen optimization, not proof of arbitrary JS-generated
initial states/all widths. It can increase total transfer while reducing blocking
head bytes. Deploy the whole folder. Server CSP must permit generated inline CSS
and the onload handler; ordinary builds are appropriate for stricter policies.

Primary references: [esbuild content types](https://esbuild.github.io/content-types/),
[tree shaking](https://esbuild.github.io/api/#tree-shaking),
[chromedp](https://github.com/chromedp/chromedp),
[Penthouse](https://github.com/pocketjoso/penthouse) (design comparison).

## Validation evidence

Go fixtures cover package browser/import/require branches, vendored ESM/CommonJS/TS,
shared initialization, production mode, unused code/assets, package CSS/font/image
handling in inline/file modes, media/cascade order, native loading boundaries,
UMD globals, inert HTML templates and failed-build preservation/path escapes.
Helper fixtures additionally cover partial origins, first-registration deduplication,
config/branch diagnostics, arbitrary source folders, package roots and CSS Modules.
Installed-Chrome tests exercise extraction, automatic layout-rule promotion, visual equivalence, full CSS retention
and final HTML validation, emitted loader/no-script behavior and file-mode reuse;
unavailable browsers skip those integration cases.
Optional Node execution checks exercise produced bundles; Go-only tests still work.
Real npm `clsx@2.1.1` (ESM) + `dayjs@1.11.13` (CommonJS) were also installed in
ignored scratch, compiled with Node excluded from PATH, and executed successfully.

2026-10-15, Apple M5, warm native CLI (25 checks): example median 18.46 ms,
p95 19.27 ms; helper conversion preserves the 10,327-byte HTML artifact with zero
warnings. Release stress fixture (example + 1,000 unused CSS rules): 41,072 →
2,194 blocking CSS bytes in 2.57 s, full CSS retained, zero warnings. These are
local measurements, not guarantees for arbitrary source/dependency/page sizes.

After automatic packing/favicon/audit integration: 10,410 B example HTML plus
321 B favicon and 26,682 B social preview; 25 warm CLI checks measured 17.68 ms
median / 18.70 ms p95 wall time. Integrated optional release took 10.97 s and
reported Lighthouse 100/100/100/100, no startup problems, one delivery-compression
diagnostic and 11 manual checks. See current evidence.
