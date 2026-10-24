---
name: lfm
description: Makes landing pages and sales pages with lfm, a compiler that turns one agent-written template into a fast, checked, static page - from buyer research through design to a page with copy, imagery, motion and optional Stripe checkout. Use when asked to generate, create, build, design or redo a landing page, sales page, product page, launch page, waitlist page, one-pager or single-page site that presents or sells something, or when a folder holds lfm.json. Not for multi-page websites, blogs, documentation, web apps or dashboards.
license: MIT
compatibility: Needs macOS or Linux, a shell and internet access to install lfm; uses web search, a browser and image generation when available.
---

# lfm: landing pages

lfm turns one page template, its config and assets into a static `output/` folder, and refuses to ship
pages that are broken, slow, inaccessible or full of generated defaults. Its own skills carry the
process; this one gets you there.

## 1. Have the binary

Run `lfm version`. If the command is missing, install it and run `lfm version` again:

```sh
curl -fsSL https://lfm.cash/install.sh | sh
```

The same script is at https://raw.githubusercontent.com/Anonyfox/lfm/main/install.sh. With Go 1.26 or newer
instead: `go install github.com/Anonyfox/lfm/cmd/lfm@latest`. The installer puts
the binary in `~/.local/bin`; if that is not on `PATH`, call it as `~/.local/bin/lfm`. When neither
works, tell the user what failed and stop: a page made without lfm is not this skill.

## 2. Have a project

- A folder with `lfm.json` is a project: work there.
- Otherwise create one in a new folder named after the offer, `lfm init <offer-name>`, then
  `cd` into it. Inside an unrelated repository, ask where the page should live; without anyone to ask,
  use a new subfolder of the current folder. Never initialize into a folder holding other work.
- Set `language` in `lfm.json` to the language the buyers speak (`de` for buyers in Germany).

## 3. Install the process skills

Run `lfm skills install` in the project. It installs `lfm-research`, `lfm-design`,
`lfm-implementation`, `lfm-book`, `lfm-printable`, `lfm-spreadsheet`, `lfm-document` and `lfm-deck` where your agent
loads skills and says where. If they do not appear in this session, read each one when its phase starts
with `lfm skills research`, `lfm skills design`, `lfm skills implementation`, `lfm skills book`, `lfm skills
printable`, `lfm skills spreadsheet`, `lfm skills document` and `lfm skills deck`: they match the installed
binary exactly. The last five are for what the page sells, in `deliverables/book/`, `deliverables/printable/`,
`deliverables/spreadsheet/`, `deliverables/document/` and `deliverables/deck/`.

## 4. Follow the compiler

Run `lfm compile`. The report ends with `Next (<phase>): …` and the skill for that phase. Do that step,
compile again, and repeat until it says deploy:

1. **research**: real buyer voices and competitor pages, verbatim, into `research/`.
2. **design**: `concept.md`, every decision cited to the research.
3. **implementation**: the page from `concept.md`.
4. **fix** and **review**: 0 errors, 0 warnings, then `lfm compile --release` and every saved frame.
5. **deploy**: upload `output/` as is. lfm does not host pages; ask the user where it goes.

`lfm compile --json` gives the same result as one JSON object, with `next.phase` and `next.skill`.

`lfm check <url> --json` checks any existing page, from any framework, with the same page rules.

## Rules

- Do not skip research or design to save time: the page without them is the generic page lfm warns about.
- Ask the user only what they decide: product, price, delivery, the page's language, legal pages and the
  payment link. Start research while waiting; the design skill records what is still open.
- Edit the template, `lfm.json` and `assets/`; never edit `output/`.
- Each finding says how to fix it; `lfm rules <id>` explains one.
