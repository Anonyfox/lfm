# Shared copy rules

`lfm compile`, `lfm check URL`, and `lfm check folder/index.html` use the same
embedded engine as `lfm check listing.txt` and `lfm check post.md`.
Warnings do not change exit status; errors exit 1. `--json` uses the same
`validation.diagnostics`, `errors`, `warnings` and `next` fields for every target.
Text targets run only copy rules, not page or browser checks; `--release` requires HTML.

Language precedence is `--lang`, HTML `<html lang>` or text front matter
(`---` on its own line, then `lang: de` or `language: en`, then `---`), then
function-word detection. Regional tags such as `de-DE` use the primary language.
Unknown/other languages run only `any` rules. `copy.tells` retains its bilingual
legacy behavior, including on mixed-language pages. Language is document-wide;
inline language switches do not change it.

## Rule reference

IDs describe ideas rather than individual words. Pattern lists are embedded from
`internal/slop/rules.json` and `tells.json`; no external checker or service is needed.
Each table carries the default level, reason and concrete fix. These are editorial
signals, not proof of authorship or factual inaccuracy.

| ID | Language | Default | Trigger/example | Fix |
| --- | --- | --- | --- | --- |
| `copy.tells` | any (DE/EN) | warn | Narrow stock phrases such as “unlock the power”; “nicht nur … sondern” and “not just … but” from **two** uses; ≥4 em dashes above 1/100 words outside German | State the fact or capability. |
| `copy.reveal` | any | warn | It's not a course, it's a kit. | State the positive claim directly. |
| `copy.negative-list` | any | warn | No fluff, no jargon, no distractions. | Name what the buyer receives. |
| `copy.superlative` | de | warn | Eine traumhafte Aussicht. | Describe the observable feature. |
| `copy.hype` | en | warn | World-class service. | Give the measured difference and its source. |
| `copy.food-filler` | de | warn | Kulinarische Vielfalt. | Name the dish, ingredient or preparation. |
| `copy.trailing-clause` | any | warn | It saves the file, making work effortless. | State a specific consequence in its own sentence. |
| `copy.triad` | any | warn | Reliable, useful and powerful. | Keep the property that matters and explain it. |
| `copy.comma-chain` | any | warn | Das Haus, das direkt am Hafen steht, die Gäste lange beherbergt, weil es seit Jahren geöffnet bleibt. | Split the sentence at the change of subject. |
| `copy.unsourced-number` | any | warn | 80% of companies use this. | Link the study and name its date and sample. |
| `copy.emoji` | any | warn | Buy now 🚀 | Use words that state the meaning. |
| `copy.internal-leak` | any | error | As an AI language model, I cannot answer. | Remove the internal instruction or path before publishing. |
| `copy.german-dash` | de | warn | Das Haus — direkt am Hafen. | Use a comma, colon or full stop. |
| `copy.duplicate-sentence` | any | warn | The workbook includes six exercises for planning each week with your team. The workbook includes six exercises for planning each week with your team. | Keep it once and link to the explanation. |
| `copy.rhythm` | any | warn | We plan the next week. You write the first task. They make the next choice. We check the final list. You start the next step. They read the last page. | Combine related claims and vary length where the meaning calls for it. |
| `copy.filler` | any | warn | Here's the thing: it works. | Start with the specific fact or action. |
| `copy.vague-source` | any | warn | Studies show that it works. | Name and link the study or expert. |

`copy.german-dash` excludes numeric ranges (`13—14`). `copy.duplicate-sentence`
compares case-insensitive sentences of at least nine words within one document,
preserving numbers so different quantities remain distinct;
it does not crawl a site or mask page-specific names. Both sentence rules require
terminal punctuation after a letter and exclude unterminated labels, numeric data
and middle-dot citation rows. `copy.rhythm` requires six consecutive sentences of at least four words, whose lengths differ by at most one.
`copy.comma-chain` requires three subordinate clauses in one sentence.
`copy.triad` matches adjective endings, not ordinary three-item inventories.
`copy.unsourced-number` only matches population percentages, not prices, dates or
product specifications. It and `copy.vague-source` stay quiet with attribution or
a URL in the same block; they cannot establish that a source supports a claim.

HTML checks include body copy, title and description/social metadata (identical
metadata values checked once). Hidden text, code, quotations and existing
`data-design-reason` exemptions remain skipped. `data-slop-skip` skips an element's
whole subtree, including nested paragraphs. For example:

```html
<div data-slop-skip>Verbatim licence or imported third-party quotation.</div>
```

Markdown skips front matter, code blocks, inline code, blockquotes and raw HTML
blocks. Inline formatting is read as text, and findings point at the source block's
first line. Plain text checks each nonempty line as a block. Phrases never join
unrelated blocks.

## Project policy

Existing `build.validation.rules` values remain `warn`, `error` or `off`.
“Block” means `error` in configuration and JSON. No new severity spelling is needed.
Use `--project path` to apply a project's policy to standalone checks, including
text; compile uses its own project's policy. A bare check does not search parent
directories for config.

```json
{
  "build": {
    "validation": {
      "rules": {"copy.reveal": "error", "copy.emoji": "off"},
      "copyAllow": [
        {"rule": "copy.superlative", "phrase": "Traumhaft Hotel", "reason": "Registered business name"},
        {"rule": "copy.german-dash", "reason": "Verbatim bilingual exhibition transcript"}
      ]
    }
  }
}
```

Every allowance requires a known shared copy rule ID and a nonblank reason.
An omitted phrase exempts the rule; a phrase exempts only its case-insensitive
literal span at word boundaries, never all other matches in the paragraph.
An empty phrase is equivalent to omission. Unknown keys and malformed values are
config errors located at their JSON keys. Proper names are not blanket-exempt:
ordinary “Juwelier Kaufmann” stays quiet because broad substring bans such as
`juwel*` were not imported. Register a phrase allowance for a name that actually
contains a flagged expression.

## Migration from local gates

The original projects remain read-only. This mapping distinguishes coverage from
checks that cannot be faithfully implemented in a single-document copy gate.

| Source | Local idea | Shared rule / disposition |
| --- | --- | --- |
| MICE `research/slopcheck.sh`, `banned-phrases.txt` | Contrast reveals | `copy.reveal`; repeated not-only constructions remain `copy.tells` (two uses). |
| MICE | Negative triplets; adjective triads; subordinate comma chains; trailing benefit clauses | `copy.negative-list`, `copy.triad`, `copy.comma-chain`, `copy.trailing-clause`. Short three-sentence slogans stay with statistical `copy.machine`; ordinary short sentences are legitimate. |
| MICE | Stock DE/EN phrases, rhetorical filler, empty superlatives | `copy.tells`, `copy.filler`, `copy.superlative`, `copy.hype`. Broad bans on honest, guaranteed, seamless, discover and every rhetorical question were narrowed/dropped to avoid ordinary factual uses. |
| MICE | Unsourced numbers and authorities | `copy.unsourced-number`, `copy.vague-source`. No claim is declared false; nearby attribution suppresses the warning. |
| MICE | Emoji, em dashes and reasoned allow-list | `copy.emoji`, `copy.german-dash`, dash density in `copy.tells`; `copyAllow` replaces path globs with per-project policy. No automatic legal-directory exemption. |
| Helgoland `scripts/slop-rules.json`, `slopcheck.py`, `internal/web/slop_test.go` | nicht-sondern; nicht-nur-sondern-auch; kein-kein | `copy.reveal`, `copy.tells`, `copy.negative-list` (three negatives, not an ordinary pair). |
| Helgoland | superlative, perfekt-ideal, paradies, verzaubern, entdecke, fuer-alle, laedt-ein, filler-intro, welt-der | Narrow phrases via `copy.superlative`, `copy.filler`, `copy.tells`; broad bans on ideal, perfect, paradise, jewel, invites, discover and booking CTAs dropped because these can be factual names or actions. Generic action labels already use `copy.actions`. |
| Helgoland | em-dash, trailing-ideal, rhythm, duplicate sentence groups | `copy.german-dash`, `copy.trailing-clause`, `copy.rhythm`, `copy.duplicate-sentence` within a document. Cross-page normalized groups and site-wide triplet limits are deferred to site analysis; this command still checks one page. Commit messages are outside public-copy scope. |
| Helgoland | Proper names, quotes, review tiles, licence text | Word boundaries plus `copyAllow`; standard quote tags and explicit `data-slop-skip`. No site-specific class-name exemptions. |
| Helgoland | CSS palette | Existing `design.ground` / `design.palette`; no duplicate copy rule. |
| Agentpreneur taste anchors `banned-phrases.txt` | Stock phrases, intros, unnamed experts, hedging, encouragement | `copy.tells`, `copy.filler`, `copy.vague-source`; ordinary “can help”, chapter navigation and encouragement dropped as universal bans. `copy.machine` keeps its trained statistical estimate; its features are not duplicated as blocking rules. |
| Agentpreneur `ai-tells-warn.txt` | Optional warn list | No such file was present anywhere under `/workspace/agentpreneur` during this job; cannot claim exact parity. |
| Cosmolytic | `slopgate.py` and checklists | No script exists under `/workspace/cosmolytic-studio`; `tools/lfm-gate.py` and deploy/checklist references call `/workspace/agentpreneur/lab-build/build/slopgate.py`, which is also absent. The README pre-ship checklist maps reveals, negative triplets and trailing clauses to `copy.reveal`, `copy.negative-list` and `copy.trailing-clause`. Narrow stock phrases map to `copy.tells` / `copy.filler`; astrology-specific cosmic/manifest phrases, tool-credit bans, defensive-disclaimer bans and visual provenance stay product/editorial policy. No `Stand:` ban imported. Product-specific “AI” bans and legal disclaimers are not universal rules. |
| Submission Desk `SUBMIT.md` Slop Gate | Reveals, negative lists, trailing clauses, triads, hype, dash chains, emoji | `copy.reveal`, `copy.negative-list`, `copy.trailing-clause`, `copy.triad`, `copy.hype` / `copy.tells`, `copy.german-dash` / `copy.tells`, `copy.emoji`. English dash density retains its conservative threshold; ordinary paired dashes are legitimate. |
| Submission Desk | Internals and claim verification | `copy.internal-leak` blocks explicit generation instructions and workspace paths. Generic words “bot”, “agent”, “box”, tool names and “AI” are legitimate product copy, so not universally banned. Numbers not on another linked page require external evidence comparison and remain manual. A first-person voice and concrete detail are editorial review, not mechanically enforceable. |
| Submission Desk | Visual policy | Existing design checks cover generic defaults; real screenshot provenance and project-specific image policies stay editorial. |
| Fresh Bites | vielseitig, kulinarisch | `copy.food-filler` warns on inflections in German. |

The three requested 2026-10-25 sections in `/workspace/toolbox/IMPROVEMENTS.md`
are represented above. Cross-site sameness remains its separately listed future
work. Local checkers should only be retired after their owners accept the narrower
false-positive policy and the explicitly deferred checks.
