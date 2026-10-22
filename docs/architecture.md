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
| `skills` | Agent Skills embedded in the binary, rendered with its facts, stamped, installed and found on disk. |
| `aitext` | The AI-text estimate: language-agnostic features (burstiness, punctuation, lexical diversity, compression against model-written text, repetition) plus the habits and fillers of eight western languages, scored by models fitted on labelled human and generated text in those languages. Used by `copy.machine` and the book's prose checks. |
| `deliver` | What every deliverable shares: the unlisted download path, file names, prices, the download page. |
| `pdfx` | Reads and stamps the PDFs Chrome prints: page counts, outline targets, transparency, the input key that lets a compile reuse a print. |
| `printable` | The printable deliverable: its concept, the page template and generators (calendar, bingo, word search, sudoku, maze), page checks per edition, listing images and the store guides. |
| `formula` | The spreadsheet formula language: placeholders, parsing, printing to the stored syntax, the portable functions and an evaluator with Excel semantics. |
| `spreadsheet` | The spreadsheet deliverable: its concept and workbook, layout per edition, computing and checking every formula, the Excel files, previews, the LibreOffice check, the start PDF, listing images and store guides. |
| `deck` | The slide deck deliverable: the concept, Markdown decks per language, the slide model and its font-measured fit check, the .pptx writer with master, layouts, notes and embedded fonts, the Chrome prints of slides, notes and quiz sheets, previews, start guides, listing images and store guides. |
| `typeface`, `theme`, `markup` | What the editable deliverables share: font files read, measured and embedded (ODTTF, EOT); the Look a concept decides on a typesetting base, color checks and the Office theme part; the Markdown source model with its directives and typography. |
| `document` | The editable document deliverable: the concept and its Look, Markdown sources per language, font inspection and embedding, the .docx writer, previews, the print check, start guides, listing images and store guides. |
| `office` | Runs LibreOffice headless (finds it, converts a batch in one run) and, on a Mac, drives Microsoft Word and Excel to open, print and recalculate files, catching repair prompts in their logs. |
| `ooxml` | Puts every Office Open XML part into the order the schemas require, which Word and Excel enforce, and runs Microsoft's Open XML SDK validator in tests. |
| `bundle` | Two or more deliverables sold together: its config and units, listing, download page, hero image and shop packages. |
| `book` | The book deliverable: its concept, chapter, prose and listing checks, the EPUB, the print documents, the cover wrap and the download page, and the store packages per channel. |
| `compile` | The compile lifecycle and the assemble stage: bundling with esbuild, packing, images, fonts, hints. |
| `research` | Parses, checks and summarizes the two research files. |
| `concept` | Parses `concept.md`, the design handover, and checks it against the research. |
| `color`, `stock` | Color parsing, OKLab distance and WCAG contrast; the stock values of generated pages. |
| `templatecheck` | Static analysis of templates against the actual config values, then rendering. |
| `urlcheck` | URL syntax, the named-URL registry and destination matching across config, template and output. |
| `commercecheck` | The optional offer and Stripe checkout contract in config. |
| `jsonldcheck` | Offline JSON-LD expansion with a bundled Schema.org context. |
| `check` | Every check on the final document: one parse, many rules, one report. |
| `fontcheck`, `imaging` | Bounded inspection of font containers and image headers, without decoding. |
| `htmlcompact` | Syntax-only HTML compaction, proven by comparing parsed trees. |
| `dataurl` | Data URL encoding and decoding. |
| `zipx` | The one ZIP writer behind every EPUB, Office file and download: sizes in every header, no data descriptors or extra fields, flagged UTF-8 names, a fixed time, and the input-key stamp in the comment. |
| `paper` | The paper sizes editable files print on, and which a language prints on by default. |
| `buildid` | The identity of the running lfm build, part of every built file's key, so an upgrade rebuilds files but keeps prints. |
| `htmlx`, `jsonsrc`, `diagnostic` | Shared node helpers, JSON source locations, and the finding format. |
| `release` | Headless Chrome for critical CSS, the runtime probe and the frame-by-frame layout audit, and Lighthouse. |
| `serve` | The preview server, the polling watcher and live reload. |
| `golden`, `testtmp`, `testproject` | For tests only: manifests of whole outputs held to `testdata/golden` (`LFM_UPDATE_GOLDEN=1` rewrites them), a temp directory of their own for browser-backed tests, and a project per deliverable for the compile and report tests. |

Dependencies point one way, in layers: `cli` drives `serve`, `compile`, `scaffold` and `skills`; `compile`
drives the checkers, `research`, `concept` and `release`; everything may use the shared base (`project`, `diagnostic`,
`htmlx`, `jsonsrc`, `term`). Checkers never import `compile`, and nothing imports `cli`.

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
Golden manifests pin every deliverable's output byte for byte, and an allocation budget in `compile` holds
an ordinary compile to its memory. `BenchmarkCompile` in `compile` tracks compile speed, optionally on any
project via `LFM_BENCH_DIR`.
