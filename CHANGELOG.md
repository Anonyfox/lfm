Notable changes per release. Older releases: https://github.com/Anonyfox/lfm/releases.

## Unreleased

- `lfm compile` updates untouched lfm skills written by an older release, including shared folders above the project, and names the folder it updated.
- New projects ignore `qa/` for review state such as screenshots and logs.
- copy.machine names the passages that drive its estimate, with location, excerpt and measured signals.
- Missing template key paths report the template file and line and the closest keys that exist.
- Add a `dict` template helper; data passed to partials through `dict`, `$` or variables is no longer reported unused, and keys guarded by `if` or `with` are optional. `lfm help template` lists every helper.
- Research minimums apply to every project's totals, whatever its first batch is called; a shortfall note waives voice and peer minimums alike and shows in the summary.
- Imported research items keep their original fetch dates through an `Imported:` line.
- Concept facts may cite an existing project file with a relative link.

## 0.1.8 (2026-10-24)

- Add `lfm check` for built URLs, folders and HTML files, with optional browser checks and JSON reports.
- Include X-Robots-Tag response directives in the indexing and snippet checks.
- Resolve font preloads against stylesheet URLs when checking CSS font sources.
- Match styles faster on pages with large stylesheets.
- Report an unavailable local port during release checks instead of panicking.
- Identify placeholder checkout links and point to the real Stripe Payment Link.
- Release critical-CSS analysis bounds each browser step with a precise error, without a shared deadline that rejects progressing builds on loaded machines.

## 0.1.7 (2026-10-23)

- Generate sitemap.xml and root robots.txt, respecting noindex and public/ overrides.
- Copy public/ files verbatim at fixed URLs, including .well-known/ and verification keys.
- Publish a Homebrew cask through Anonyfox/homebrew-tap.
- Optional X handle fills twitter:site and twitter:creator.
- Include sha256 in compile --json files.
- FORCE_COLOR=0/false now disables color.
