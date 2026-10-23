# lfm

[![CI](https://github.com/Anonyfox/lfm/actions/workflows/ci.yml/badge.svg)](https://github.com/Anonyfox/lfm/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Anonyfox/lfm)](https://github.com/Anonyfox/lfm/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/Anonyfox/lfm.svg)](https://pkg.go.dev/github.com/Anonyfox/lfm)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**Sell before you build: one binary and a skill that make your coding agent research real buyers and
ship one checked, fast landing page with a payment link.**

```sh
curl -fsSL https://lfm.cash/install.sh | sh
```

`lfm` is an agent-first page compiler. Your coding agent researches real buyers, writes a design
concept from what they say, and builds one HTML template with its config and assets. `lfm` compiles that into a
standalone static folder and refuses to ship a page that is broken, slow, inaccessible, invisible to
search or full of generic AI design defaults. Every finding says how to fix it.

No framework, no platform, no runtime: one binary, one page, one folder you upload anywhere. No MCP
server either: a CLI with `--json` plus one SKILL.md is the whole agent interface.

## Ask your agent

Give any coding agent (Claude Code, Codex, Cursor, OpenCode, Gemini CLI and others) this request:

> Make a landing page for my sourdough course with lfm: https://lfm.cash/SKILL.md

[SKILL.md](SKILL.md) tells the agent how to install `lfm`, set up a project and follow the process.
To have every agent session pick it up by itself for requests like "generate me a landing page", install
it once as a user skill:

```sh
for d in ~/.agents/skills/lfm ~/.claude/skills/lfm; do mkdir -p "$d" && curl -fsSL https://lfm.cash/SKILL.md -o "$d/SKILL.md"; done
```

`~/.agents/skills` is read by Codex, Cursor, OpenCode, Gemini CLI and GitHub Copilot; `~/.claude/skills`
by Claude Code. Keep only the one your agents need.

On Grok Bot, add the [lfm.cash bot](https://x.ai/bot/HgpgiMSirsCBrsQpdLeDf): it installs lfm on its own computer and runs the same
process, asking you only what you decide. Its setup is in [docs/grok-bot.md](docs/grok-bot.md).

## Install

The script at the top downloads the [release](https://github.com/Anonyfox/lfm/releases) for macOS or Linux (amd64
or arm64), checks it against the release checksums and puts `lfm` in `~/.local/bin`, without sudo.
`LFM_VERSION=v0.0.2` pins a release and `LFM_INSTALL_DIR` changes the folder. With Homebrew:

```sh
brew install anonyfox/tap/lfm
```

With Go 1.26 or newer:

```sh
go install github.com/Anonyfox/lfm/cmd/lfm@latest
```

Optional: Chrome or Chromium for `lfm compile --release`, and Lighthouse for its scores. Missing tools
are reported and skipped, never installed.

## Start a page

```sh
lfm init sourdough-course   # a starter project that compiles cleanly
cd sourdough-course
lfm skills install          # the process skills, where your agent loads them
lfm compile                 # findings, output/ and the next step
```

Then ask your agent for the page. It works in three phases, each with its own skill:

1. **research**: real buyer voices and competitor pages, verbatim and dated, into `research/`.
2. **design**: `concept.md` from the research: buyer, mechanism, direction, palette, type, imagery,
   motion and copy, every decision cited to a source.
3. **implementation**: the page from `concept.md`, until `lfm compile --release` is clean and every
   saved frame holds up on phone and desktop.

The compiler checks each handover: missing research, an uncited concept or a page that ignores the
concept's palette, type and imagery show up as findings. Every report ends with the next step, such as
`Next (design): write concept.md from the research.`, and names the skill for it, so an agent working
unattended stays on the path. When it says deploy, upload `output/` to the URL in `lfm.json`.

## For agents

| Command | Use |
| --- | --- |
| `lfm` | What lfm is, how to start, every command |
| `lfm <command> -h` | Usage, flags and behaviour of one command |
| `lfm skills` | The process skills in the order a page needs them |
| `lfm skills <topic>` | One `SKILL.md`, rendered for the installed version |
| `lfm skills install` | All skills into the project's `.agents/skills` or `.claude/skills` |
| `lfm compile [--release] [--json]` | Check and build; the last line is the next step |
| `lfm serve --open` | Live preview that recompiles on every save |
| `lfm rules [id]` | Every check, or what one means and how to fix it |

`lfm skills install` updates the folders that already hold lfm skills. Otherwise it writes to
`.claude/skills` when Claude Code runs it or the project has only a `.claude` folder, and to
`.agents/skills` in every other case; `--agents`, `--claude` and `--global` choose explicitly. Each
installed `SKILL.md` records the lfm version and a digest of its text, so running it again after an
update replaces untouched copies and keeps copies the project changed (`--force` replaces them too).
`lfm compile` notes installed skills that are older than the binary.

`lfm compile --json` prints the result as one object with `errors`, `warnings`, the findings under
`validation`, `next` (`phase`, `do`, `skill`) and the installed `skills`. Exit status is 0 when
`output/` was written, 1 when errors kept it unchanged or there is no project, and 2 for a mistake in the
command line.

## Made with lfm

[lfm.cash](https://lfm.cash) is an lfm project in [site/](site/), built by an agent through the same three
phases: [120 buyer posts and 20 competitor pages](site/research/), a [concept](site/concept.md) that chose
one of ten directions with every decision cited, and a page that compiles with 0 errors and 0 warnings. Its
proof section compiles the page an agent wrote from one sentence without lfm; that
[page](site/proof/default-agent-page.html) and its [report](site/proof/default-agent-page.report.txt) are
there too.

## What a compile guarantees

- **Correct:** every template value exists and has the right shape; config mistakes point at their JSON
  key; links match the declared URLs; HTML parses without browser repairs.
- **Fast:** CSS and JS are bundled and minified, small assets inlined, large ones emitted as content-named
  files, images sized against layout shift, HTML compacted without touching text.
- **Findable and shareable:** canonical URL, metadata, headings, structured data and social previews are
  checked; sitemap and root robots files are generated; preview image sizes come from the real files.
- **Usable:** controls have names, labels and focus order work, forms and disclosures behave, text keeps
  its contrast.
- **Distinct:** typical generated-design defaults (stock palettes, eyebrow labels, emoji bullets, icon-card
  grids and more) and stock phrases of generated copy are reported, with an explicit way to keep one
  deliberately; the page is held to the palette, typefaces, imagery and phrases its `concept.md` decided.
- **Sellable:** a Stripe Payment Link or Checkout offer shows its price, and the page links the legal
  pages a purchase needs.
- **Seen whole:** `--release` scrolls the page at phone and desktop sizes, saves every frame and reports
  empty stretches, sideways scrolling, covered or unreadable text, split values and missing glyphs.
- **Safe to ship:** nothing is written until all checks pass, and a failed compile keeps the last good output.

## Docs

- [Authoring a project](docs/authoring.md): layout, `lfm.json`, templates, assets, research and concept.
- [What `lfm compile` does](docs/compiling.md): lifecycle, packing, output, release, preview.
- [The lfm.cash Grok Bot](docs/grok-bot.md): the handoff a Grok Bot follows to set up and share it.
- [Architecture](docs/architecture.md) and [contributing](CONTRIBUTING.md): building, testing, the package map.

## License

[MIT](LICENSE) · [lfm.cash](https://lfm.cash)
