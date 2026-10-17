# JSON-LD

`lfm.json` → resolve authored string leaves once → `encoding/json` → one static
script near the end of body → validate exact final bytes → publish on success.

```json
"jsonld": {
  "@context": "https://schema.org",
  "@type": "WebPage",
  "url": "{{.url}}",
  "name": "{{.title}}",
  "description": "{{.description}}"
}
```

## Authoring contract

- Optional object/array; omitted or null emits nothing. Use `@graph` or a top-level
  array for multiple entities. One block is our maintenance policy, not a search-engine mandate.
- Only string **values inside jsonld** are template source. Keys and other config
  stay literal. References mirror config paths, including `data/*.json` datasets.
- `{{.data.value}}`, `{{$.title}}`, `{{index .data "key-name"}}` and nested index
  selections work. Control flow, definitions, assignments, function calls and
  pipelines are rejected. Keep structural programs in ordinary HTML authoring.
- A value consisting of one action preserves its JSON type, including exact JSON
  numbers. Text around an action makes a string; objects/arrays/null require a
  whole-value selection. False and zero are valid; selected null/empty strings warn.
- Selected content is never parsed again. A title containing `{{.anything}}`
  remains literal. Self/whole-config references are rejected. Missing `index` keys
  fail too; intentional absence must be explicit null.
- Never splice serialized JSON into HTML templates. `encoding/json` escapes quotes,
  newlines, `<`, `>`, `&`, U+2028/U+2029 and script closers safely. The shipped JSON
  is not the processor's expanded/normalized form.
- The compiler owns the script. Remove handwritten JSON-LD blocks. The
  `jsonld.block` policy can be tuned like other final HTML policies.
- Page URLs and destinations still belong in `url`/`urls`; vocabulary/context IRIs
  are exempt. Full deliberate `@id` fragment variants need named URLs. Repeated
  `@id` references are valid and are not rejected as duplicate entity definitions.
- JSON-LD uses published asset URLs, not local source paths. A `.socialImage.src`
  file path is an input to the asset compiler, not an image URL for structured data;
  declare the published image URL in `urls` when needed. JSON-LD does not copy assets.
- Only put public page facts in this subtree: its complete resolved content ships.

## Validation boundaries

Preflight `jsonld.bindings` is required: every independent binding/syntax/shape
failure is collected before asset work, with config locations and related selected
fields. `jsonld.empty` is tunable. Referenced data counts as used by the template gate.

The final gate uses [JSON-goLD v0.9.1](https://github.com/piprate/json-gold/tree/v0.9.1)
for [JSON-LD expansion](https://www.w3.org/TR/json-ld11-api/#expansion-algorithm),
not a custom keyword validator. It checks `@type`/`@id`, context definitions,
value/list objects and other expansion errors. Rule tests can invoke `Check`
without HTML/filesystem inputs; final HTML rules share one cached analysis.

The full official Schema.org **30.1 context**, including term coercions, is embedded
with [provenance/checksum](schemaorg/README.md). HTTP/HTTPS and documented context
URL variants resolve locally. Inline custom contexts work. Other remote contexts
produce a `jsonld.context` coverage warning and skip expansion for that block.
Builds never fetch URLs or read context files; use inline contexts for offline coverage.

Inputs are bounded to depth 128 and 100,000 values before processor recursion;
processor panics become diagnostics. Structural failures block by default;
final `jsonld.*` rules accept normal `off`/`warn`/`error` policy overrides.
Safe-mode property loss is advisory, since null term mappings may be intentional.
No context or expansion result is used to rewrite the artifact.

This is **not full Schema.org or Google rich-result validation**. Unknown
Schema.org terms can expand successfully via `@vocab`; the processor does not
validate vocabulary membership, domain/range, visible-content truth or rich-result
eligibility. JSON-goLD documents incomplete compaction/framing/RDF features and a
historical test-suite score; we use expansion only and do not claim complete 1.1
conformance from that score.

## Reuse research — 2026-10-15

| Candidate | Fit / decision |
| --- | --- |
| [JSON-goLD](https://github.com/piprate/json-gold) | Pure Go, standard JSON-LD processing, maintained v0.9.1; adopted for expansion. |
| [ldlint v0.3.1](https://github.com/KevHQ/ldlint/tree/v0.3.1) | Go CLI with full-vocabulary inheritance, term/domain and superseded checks. Implementation packages are `internal`, so not importable as a normal library. Uses a live cached vocabulary and treats domain mismatch as error; our future gate should pin data and make domain advice configurable. Useful reference/optional independent judge; not integrated. |
| [Adobe structured-data-validator](https://github.com/adobe/structured-data-validator) | JS library with vocabulary and Google profiles. Requires a JS runtime/extractor; not a fit for this standalone Go binary. Consider as a separate development judge. |
| [goRDFlib](https://github.com/tggo/goRDFlib) | RDF/SPARQL/SHACL stack. Shapes can enforce chosen application constraints, but do not supply universal Schema.org requirements. Larger surface than needed for this stage. |
| [Schema.org Markup Validator](https://schema.org/docs/validator.html) | Hosted reference tool, not a documented embeddable Go library; unsuitable as a mandatory offline build dependency. |

Next vocabulary layer: vendor a versioned official release, generate a compact
index of terms, type/subproperty ancestry, domainIncludes/rangeIncludes,
enumerations and supersededBy, then inspect **expanded IRIs** so aliases and custom
contexts behave correctly. Unknown terms/deprecations and domain/range expectations
should be separate rules, with external vocabularies left outside Schema.org's scope.
Preserve a provenance map when adding per-property findings.

[Schema.org's data model](https://schema.org/docs/datamodel.html) explicitly allows
mixed types, text for entity values and extensions. Its vocabulary is not a set of
closed-world required-field shapes. Required/recommended properties for Google
features belong in separate, dated, opt-in profiles sourced from
[Google's feature documentation](https://developers.google.com/search/docs/appearance/structured-data/intro-structured-data).
Those profiles cannot prove visible truth or guarantee a rich result.
