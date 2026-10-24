# aitext

Estimates whether AI-text detectors would flag prose, and why. Pure Go, no model download, no network:
about 2 ms for a page and 6 ms for a 2,400-word chapter.

## How it works

No language model runs: each 500-word window becomes 29 measurements, two small fitted formulas
(logistic regression) turn them into a score, and a text of several windows is judged on the mean of its
windows. Only the weights (about 15 KB of generated Go), the preludes and short word lists are vendored.

- **Measurements** (`Measure`): sentence and paragraph burstiness, mean sentence length, the 8–20-word
  sentence band, the shares of very short and very long sentences, commas, semicolons, parentheses,
  colons, questions, dashes and asides per sentence, punctuation entropy, one-sentence paragraphs,
  moving-average type-token ratio, word-length spread, raw compressibility, compression against two
  model-written preludes used as preset deflate dictionaries (ZipPy's method: `preludes/generic-*` is the
  stereotype of generated copy, `preludes/current-*` what Claude and GPT write when asked plainly),
  sentence-opener diversity, repeated trigrams, top-word share, participle clauses after a comma
  (Reinhart et al., PNAS 2025), abstract-noun share, first- and second-person rate, the habits of current
  models (corrective framing, signposting, the over-used vocabulary of Juzek 2026) and the fillers and
  particles of people writing (`languages.go`).
- **Two profiles**: `old` fits the repetitive, even prose of older models; `modern` the polished prose of
  current ones, which never repeats a phrase, varies its words more than people do and drops asides,
  hedges and particles. They deviate from human writing in opposite directions.
- **Pooled across languages**: every measurement is standardized against human writing in its language;
  shared weights are fitted on all languages and each language refines them on its own data, held close
  to the shared fit. Fillers count only when present (formal human prose lacks them too).
- **Languages**: English, German, French, Spanish, Italian, Portuguese, Dutch and Polish use everything.
  Any other language is read with the language-agnostic measurements only.
- **Documents**: a text of two or more windows is judged on the mean of its windows' log-odds against a
  threshold calibrated on whole human documents.

## Measured performance (2026-10-18)

Every source is scored by a model fitted without it, as text from a source the model never saw. Each
language's threshold keeps at most 1% of those unseen human windows above it, and at most 2% of the human
windows in the genres lfm checks (books, essays, landing pages, web copy).

| Unseen sources | en | de | fr | es | it | pt | nl | pl | other |
|---|---|---|---|---|---|---|---|---|---|
| Current Claude and GPT, 500-word windows flagged | 69% | 57% | 69% | 63% | 76% | 67% | 45% | 76% | 19% |
| Their documents of 2+ windows flagged | 93% | 87% | 73% | 89% | – | 67% | 30% | – | 28% |
| Human windows flagged | 1.1% | 1.0% | 1.1% | 1.1% | 1.1% | 1.0% | 1.1% | 1.1% | 1.0% |
| Human documents flagged | 1.1% | 1.3% | 1.9% | 1.6% | 1.9% | 1.7% | 1.0% | 1.9% | 1.1% |

Older models (GPT-3.5, Mixtral, Llama) are caught less often at the same false-alarm rate (13% of English
windows): they were a minor target. A quiet result means little; a flag means the text shares the
measurable habits of generated prose. Commercial detectors run neural models and catch more.

## Retraining

`TestTrain` fits the models from a corpus of JSON lines (`text`, `label` 1 for generated, `lang`,
`group` naming the source, `split`) and writes `models_gen.go`:

```bash
LFM_DETECTOR_CORPUS=corpus.jsonl LFM_DETECTOR_WRITE=1 go test ./internal/aitext -run TestTrain -v
```

The corpus used for the shipped models (10,436 windows) is not in the repository; its sources were:

- **Human:**
  - the COLING 2025 multilingual MGT dev set (Hugging Face `Jinyan1/COLING_2025_MGT_multingual`)
  - the textsightai 2026 benchmark human splits
  - multilingual C4 (`allenai/c4`, crawled before 2022), filtered for boilerplate and spam
  - Wikipedia (`wikimedia/wikipedia`, 2023-11 dump) and Project Gutenberg in all eight languages
  - Paul Graham and Derek Sivers essays, zeitzuleben.de articles 2013–2016
  - 2019 Wayback snapshots of English, German, French, Spanish, Italian, Portuguese, Dutch and Polish
    product pages, with paragraphs repeated across a site removed
- **Generated:** about 450 texts written for this by Claude Opus, Sonnet and Haiku and by GPT (gpt-6-luna,
  gpt-6-sol) in the genres lfm checks (book chapters, landing pages, posts, product copy, newsletters,
  columns), in all eight languages, half asked to sound human; plus older-model text from COLING.

The preludes are separate texts on topics the corpus does not reuse. Academic generated text (the 2026
benchmark) was left out: its register pulled the weights away from the genres lfm checks.

Split long texts into 500-word chunks and keep sources as groups: an evaluation that mixes chunks of one
source into training and test overstates accuracy. `TestGroups` prints feature medians per group.
`testdata/` holds the licensed fixtures the regular tests run on; see `testdata/SOURCES.md`.

## Passage reporting

`Passages` ranks blocks by the decrease in their original window's score when
omitted, with fixed window boundaries. If omission leaves fewer than 150 words,
attribution is unavailable and its contribution is zero; ties retain source order.
Signals belong to the window, since rhythm and vocabulary measurements depend on
several paragraphs. This additional analysis is for reporting; `Analyze` and its
thresholds are unchanged. The page check reports up to three excerpts with their
nearest HTML source file and line (the compiled HTML when original source mapping
is unavailable), plus window signals such as paragraph-length uniformity.
