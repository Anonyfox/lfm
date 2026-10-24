Notable changes per release. Older releases: https://github.com/Anonyfox/lfm/releases.

## 0.1.8 (unreleased)

- Add `lfm check` for built URLs, folders and HTML files, with optional browser checks and JSON reports.
- Include X-Robots-Tag response directives in the indexing and snippet checks.

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
