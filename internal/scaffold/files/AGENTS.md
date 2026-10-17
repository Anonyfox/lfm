# __TITLE__

An lfm project: one sales page, compiled by the `lfm` CLI into `output/`.

## Commands

- `lfm compile` checks and builds. Fix every error and warning it prints; each one says how.
- `lfm compile --release` also measures the page in headless Chrome, and with Lighthouse when installed.
- `lfm serve --open` previews the page and recompiles on every save.
- `lfm rules` lists every check; `lfm rules <id>` explains one.
- `lfm skills` lists process guides; `lfm skills <topic>` prints one to follow or install as a skill.

## Layout

| Path | Holds |
| --- | --- |
| `lfm.json` | Page facts: url, language, title, description, social image, named urls, data |
| `index.html.tmpl` | The page, in Go `html/template` syntax; optional `partials/` |
| `assets/` | Styles, scripts, images and fonts referenced from the template |
| `data/` | Optional JSON files, available as `.data.<file name>` |
| `research/` | `human-voices.md` and `competitor-analysis.md`: real sources, verbatim |
| `concept.md` | The design decisions, written by the design skill and cited to research |
| `output/` | The compiled page. Never edit it; deploy it as is |

## Working rules

Every page goes through three phases, each with its own skill (`lfm skills <topic>` prints it):

1. **research**: real buyer voices and competitor pages into `research/`, verbatim and dated.
2. **design**: `concept.md` from the research, the only brief for the page; every decision cites research.
3. **implementation**: the page from `concept.md`, iterated until it compiles clean and every frame of
   `lfm compile --release` holds up on phone and desktop.

- Never skip a phase: a page built without research and a concept is the typical generated page.
- New customer signals go into research first, then into `concept.md` with a changelog entry, then the page.
- Reference local files from the template; `{{style "…"}}` and `{{script "…"}}` bundle CSS and JS.
- Keep product facts in `lfm.json` under `data`, not hardcoded in markup.
- The page must compile with 0 errors and 0 warnings. Silence a design check only with
  `data-design-reason="…"` naming the real reason.
