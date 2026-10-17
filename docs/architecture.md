# Architecture

`lfm` is one Go binary. `cmd/lfm` only wires signals and exit codes; everything else lives in `internal/`,
one package per responsibility, each testable on its own.

## Packages

| Package | Responsibility |
| --- | --- |
| `cli` | Commands, flag parsing, help texts and how results read in a terminal. |
| `term` | Color detection, wrapping, padding and human units. No knowledge of lfm. |
| `project` | The fixed layout, the source boundary every read goes through, and `lfm.json` loading. |
| `scaffold` | The embedded starter project written by `lfm init`. |
| `compile` | The compile lifecycle and the assemble stage: bundling with esbuild, packing, images, fonts, hints. |
| `templatecheck` | Static analysis of templates against the actual config values, then rendering. |
| `urlcheck` | URL syntax, the named-URL registry and destination matching across config, template and output. |
| `commercecheck` | The optional offer and Stripe checkout contract in config. |
| `jsonldcheck` | Offline JSON-LD expansion with a bundled Schema.org context. |
| `check` | Every check on the final document: one parse, many rules, one report. |
| `fontcheck`, `imaging` | Bounded inspection of font containers and image headers, without decoding. |
| `htmlcompact` | Syntax-only HTML compaction, proven by comparing parsed trees. |
| `dataurl` | Data URL encoding and decoding. |
| `htmlx`, `jsonsrc`, `diagnostic` | Shared node helpers, JSON source locations, and the finding format. |
| `research` | Parses, checks and summarizes the two research files. |
| `skills` | Agent Skills embedded in the binary and rendered with its facts. |
| `release` | Headless Chrome for critical CSS and the runtime probe, and Lighthouse. |
| `serve` | The preview server, the polling watcher and live reload. |

Dependencies point down this table: `cli` uses `compile`, `compile` uses the checkers, nothing below
imports anything above it.

## The lifecycle

`compile.Compile` runs load, prepare, assemble, optimize, finalize, verify, audit and publish in that order
(see [compiling](compiling.md)). Two rules keep it predictable:

- **Everything reports through `diagnostic.Diagnostic`.** A finding has a rule ID, severity, location,
  message and a hint saying how to fix it. Operational failures become the required `build.failed` rule, so
  a user sees one consistent report whatever went wrong.
- **Nothing touches the project before verify passes.** Emitted files are kept in memory; publish writes
  them next to `output/` and swaps them in with renames.

## Adding a check

1. Pick the stage that has the facts: config and template problems belong in `templatecheck`,
   `urlcheck` or `commercecheck`; everything visible in the final HTML belongs in `check`.
2. Register the rule with an ID (`family.name`), a one-line summary and help that explains what triggers
   it, why it matters and how to resolve it. `TestCatalogRulesHaveStandardHelp` enforces the format.
3. Every diagnostic carries a message stating the observed problem and a hint with the concrete fix.
4. Test the rule on the smallest HTML or template that triggers it, and on the nearest legitimate case
   that must stay quiet.

Rules report; they never rewrite the page. Automatic repairs (image sizes, social image dimensions,
favicons, resource hints) belong in the assemble stage, where they are visible in the compile report.

## Testing

`go test ./...` runs everything; browser-backed tests skip when Chrome is missing, and `-short` skips them
explicitly. Parsers and analyzers that read untrusted input have fuzz targets (`go test -list Fuzz ./...`).
`BenchmarkCompile` in `compile` tracks compile speed, optionally on any project via `LFM_BENCH_DIR`.
