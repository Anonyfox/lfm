# Fonts

Normal builds inspect already-read local font assets and final `@font-face` rules.
CSS uses the existing parse/v2 grammar/tokenizer; no regex CSS parser, network,
glyph rasterization, decompression, font conversion or new runtime dependency.

- Sniff WOFF/WOFF2/SFNT/TTC signatures, lengths, bounded directory/table offsets;
  detect `fvar` presence without claiming supported axes or glyph coverage.
  Directory work caps at 1,024 entries. Collection internals remain opaque.
- WOFF2 `totalSfntSize` is informational, not an exact reconstructed size or an
  allocation request. Nonzero WOFF2 reserved bits do not invalidate a font.
- Container inspection cannot certify compressed payloads, checksums, shaping,
  glyph completeness or browser sanitizer acceptance. `--release`/browser testing
  remains useful. Full decoding libraries such as [tdewolff/font][gofont] do more
  work than this bounded screen needs; no partial sanitizer claim is made.
- Warn on missing/invalid family/source, invalid Unicode bounds, missing/auto/block
  display policy, inspected source/format mismatches, no known WOFF2 alternative,
  preload credential/type/selection mismatches and excessive preload counts.
  Conditional/alternate CSS and fallback `src` declarations remain authored.
  Recognized family-wide display defaults suppress the implicit-auto advisory;
  conditional/layered family-policy cascade and browser support are not computed.
- Budgets: warn above 128 KiB per retained font, 256 KiB total, or 8 MiB declared
  SFNT size. Totals describe retained resources, not proven network transfers.
- Safe optimizations: content-aware MIME/packing for recognized renamed fonts;
  fonts >4 KiB prefer files in auto mode; missing anonymous CORS on authored font
  preloads; missing type for inspected local fonts; equivalent hint deduplication.
  Explicit credential/type choices are preserved and diagnosed, never guessed away.
- Preserve font bytes, names, weights, axes, metrics overrides, source order,
  `local()`, `format()`/`tech()`, Unicode subsets, display timing and explicit pins.
  No static-text subsetting, source reordering, guessed preloads or font purging.

`perf.fonts` and `perf.font-assets` run at the exact final-byte quality gate, using
the exact emitted CSS sidecars and source-attributed asset metadata. Individual
rules share cached analysis and existing severity controls. The default readable
report summarizes fonts and includes every actionable finding. Remote CSS/fonts and dynamic JS font
construction are not statically inspected or fetched. Authored data URLs are
inspected within the HTML budget but keep their explicit delivery.

Tests cover malformed containers/encodings, real TTF/WOFF2, nested/escaped CSS,
Unicode boundaries, optional/variable/subset declarations, fallback sources,
packing, source attribution, pin/credential preservation, rule overrides and fuzz
inputs. Chromium verifies loaded pixels/geometry at 390/1280 px and one network
fetch with a repaired preload. The fixture preparation tools are optional and
are never invoked by builds or tests.

[CSS Fonts][cssfonts], [WOFF][woff], [WOFF2][woff2], [OpenType][opentype],
[loading guidance][loading].

[cssfonts]: https://www.w3.org/TR/css-fonts-4/
[woff]: https://www.w3.org/TR/WOFF/
[woff2]: https://www.w3.org/TR/WOFF2/
[opentype]: https://learn.microsoft.com/en-us/typography/opentype/spec/otff
[loading]: https://web.dev/articles/font-best-practices
[gofont]: https://github.com/tdewolff/font
