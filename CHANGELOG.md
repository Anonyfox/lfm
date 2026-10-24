Notable changes per release. Older releases: https://github.com/Anonyfox/lfm/releases.

## Unreleased

- Security: release archives now hold root-owned (0/0) entries instead of the build machine's uid 1001, and install.sh ignores the archive's owner, so a root install (sudo with `LFM_INSTALL_DIR`) writes a root-owned binary. CI checks both.
- install.sh warns when another lfm of a different version comes first on PATH, with both paths and versions.
- `lfm compile` updates untouched lfm skills written by an older release, including shared folders above the project, and names the folder it updated.
- New projects ignore `qa/` for review state such as screenshots and logs.
- copy.machine names the passages that drive its estimate, with location, excerpt and measured signals.
- Missing template key paths report the template file and line and the closest keys that exist.
- A call to an undefined partial names the missing file, where lfm looks for partials and the closest defined names.
- Add a `dict` template helper; data passed to partials through `dict`, `$` or variables is no longer reported unused, and keys guarded by `if` or `with` are optional. `lfm help template` lists every helper.
- Research minimums apply to every project's totals, whatever its first batch is called; a shortfall note waives voice and peer minimums alike and shows in the summary.
- Imported research items keep their original fetch dates through an `Imported:` line.
- Concept facts may cite an existing project file with a relative link.
- `lfm check` on a folder without index.html, or on a bare URL path, says what to pass instead.
- `lfm check` on a localhost preview judges canonical, og:url, og:image and link findings against the page's declared canonical origin.
- `design.palette` counts only colours the page uses, so unused framework variables (e.g. Bootstrap's `--bs-*`) no longer fire.
- `copy.machine` skips tables and role=table/grid content.
- New error rule `design.ground`: warm off-white page grounds (OKLCH L ≥ 0.88, C 0.004–0.045, hue 45–115°, e.g. cream, beige, ivory) and paper, parchment or notebook ground textures (SVG noise, ruled or squared lines, paper image files) fail the build in `lfm compile` and `lfm check`. `data-design-reason` does not exempt it. `concept.ground` rejects such colours in the concept palette; the cream-and-terracotta palette warning is replaced by these rules.
- The reference example, the research skill's example palette and the design skill no longer show or suggest warm off-white or paper grounds.

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
