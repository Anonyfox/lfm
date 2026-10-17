# Config ↔ template gate

Runs after loading sources, before contextual rendering, assets or output staging.
Uses Go's `html/template` / `text/template/parse` trees, `encoding/json` tokens and
cached `text/template` expression probes. The adjacent [link gate](../urlcheck/README.md)
adds URL registry/source checks at the same preflight moment. No second expression language or parser.

1. Load all readable config/data/template sources; collect independent input errors.
2. Parse every template file; collect one syntax error per invalid file. Index JSON
   keys/array items with original file/line/byte columns; reject duplicate keys.
3. If inputs are sound, walk reachable templates against actual JSON shapes. Track
   dot, `$`, variables/assignments, if/with/range scopes, partial calls and pipeline
   results. Go evaluates builtin arguments/types, bounds and short-circuit semantics.
   The optional [JSON-LD subtree](../jsonldcheck/README.md) resolves references once
   in the same preflight stage; its dependencies count as usage too.
4. Validate every `build.browserData` path, count used values and collect advisories.
5. On hard errors, report **only hard config/template findings** and skip rendering.
   Otherwise render once with `html/template`; contextual escaping/execution failures
   become a hard diagnostic and partial output is discarded.
6. Merge successful preflight warnings with the final HTML gate into one report.
   `validation.stage` is `template` on early rejection and `html` after final analysis.

Diagnostics use the shared `internal/diagnostic` format: stable rule ID, severity,
source location, config path when known, explanation, fix and related JSON/definition
locations. The default readable report carries every finding. Config values are not
printed; paths, key names and types provide the evidence.

| Rule | Default |
| --- | --- |
| `config.invalid` | Required error: invalid JSON/schema, ambiguous sources or unreadable inputs |
| `config.duplicate-key` | Required error: duplicate JSON keys, with both definitions |
| `template.syntax` | Required error: malformed templates, undefined variables/functions |
| `template.bindings` | Required error: active missing paths, wrong shapes/types, invalid operations, recursion/resource limits |
| `template.browser-data` | Required error: invalid runtime-data selections, collected across all paths |
| `template.render` | Required error: contextual HTML escaping or execution failure; rejected URL/value marker |
| `template.inactive` | Warning: binding failures in dormant branches/short-circuited arguments |
| `template.empty` | Warning: active empty strings/nulls, missing `index` keys, empty range/with |
| `template.value` | Warning: direct object/array interpolation, formatting markers, definition overrides |
| `template.unused` | Warning: unconsumed authored JSON leaves or uncalled nonempty definitions/partials |
| `jsonld.bindings` | Required error: invalid/self/missing JSON-LD references, unsupported template programs or root shape |
| `jsonld.empty` | Warning: selected null or empty string in JSON-LD |

`build.validation.rules` supports `off`/`warn`/`error` for advisory rules. Required
rules accept only `error`; they protect execution rather than style preferences.
`lfm rules` lists every stage, marks required checks and includes lfm.json opt-out syntax.

## Usage and emptiness

- Explicit `false` and zero are valid; tests/branches do not produce empty-output
  warnings merely for selecting the false case. Empty optional sections may be
  intentional: use the advisory override if that is the product's policy.
- Inactive branches count as usage and their binding issues warn. Undefined partial
  calls remain hard even when inactive: HTML contextual escaping inspects both arms.
- Empty collections provide no item schema. Warn once about the empty range; inspect
  its body with unknown item values without inventing missing-field errors.
- Check every actual array/map entry, including heterogeneous arrays. Collapse
  repeated unused array fields into `data.rows[*].field` with a value count.
- `schemaVersion`, `url` and `build` are compiler-consumed. Entire valid browserData
  selections count as used, including JS-only values. Injected defaults do not create
  unused warnings; only authored config/dataset leaves do.
  Resolved `jsonld` and its referenced leaves/subtrees count as compiler consumption.
- Field/variable/index paths retain provenance. Opaque transformations (e.g. printf,
  slice, comparisons) conservatively consume referenced subtrees; this avoids false
  unused warnings but can hide stale children behind those operations.
- Reserved config keys have exact casing; arbitrary keys belong under `data`/`links`.
  JSON numbers remain `json.Number`, matching the existing renderer. `printf "%s"`
  prints their original representation; Go numeric comparisons need deliberate
  conversion (e.g. `.data.count.Int64`). Browser JSON retains numeric JSON values.

## Organization and limits

- `sources.go`: JSON token locations, duplicate keys and path utilities.
- `template.go`: parse/merge partials, rule catalog and safe contextual rendering.
- `assets.go`: bind script/style helper calls to their original template file;
  dry probes and final rendering share the compiler-supplied helper functions.
- `check.go`: scope/control-flow walk and output-value checks.
- `eval.go`, `probe.go`: reference tracing plus standard Go command evaluation.
- `usage.go`: browserData paths, consumption accounting and unused findings.
- `jsonld.go`: structured one-pass JSON-LD references using the same Go probes.
- Tests use tiny templates/configs without the build pipeline. Integration tests
  verify combined reporting, hard-only rejection and preservation of previous output.

Bounded to 100,000 walked nodes/iterations and call depth 128. Recursive calls must
have verifiable progress to a different JSON path; opaque recursion is rejected.
One syntax error per file and one final contextual render error reflect standard Go
parser/escaper limits. A failed expression makes dependent values unknown; independent
expressions continue. This is data-aware preflight, not a proof for every future
configuration, browser execution or arbitrary custom Go function environment.

```sh
go test ./internal/templatecheck -run TestScopesAndUsage
go test ./internal/compile -run 'TestMapping|TestConfigProblems'
go test ./internal/templatecheck -run '^$' -fuzz FuzzBindingsMatchGo -fuzztime 15s
go test ./internal/templatecheck -run '^$' -bench Preflight -benchmem
```
