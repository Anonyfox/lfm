---
name: lfm-implementation
description: Builds or improves the lfm sales page from [[.ConceptFile]] - template, styles, scripts, imagery, motion, copy, metadata, sharing and Stripe checkout - and iterates until lfm compile is clean and every frame of the page holds up on phone and desktop. Use when concept.md exists and the page must be built, rebuilt, extended or polished, or when asked to implement, fix or optimize an lfm page. Not for research or for design decisions; those live in their own skills and files.
license: MIT
compatibility: Needs lfm, the project's research files and [[.ConceptFile]]. Uses image generation, a browser and screenshots when available, and asks the user for what it cannot produce.
metadata:
  generator: lfm [[.Version]]
---

# Implement an lfm page

[[.ConceptFile]] decided what the page is; this skill makes it real, then makes it excellent in a few
deliberate rounds. The compiler checks what can be measured; you look at what cannot, by viewing the
whole page as a visitor does, frame by frame, on a phone and on a desktop.

## Rules that always hold

1. **[[.ConceptFile]] is the brief.** Build its spine, palette, type, form, imagery, motion and copy. When a
   decision has to change, change [[.ConceptFile]] first with a changelog entry, citing research, then the page.
   `lfm compile` reports typefaces, colors and imagery that drift from it.
2. **Real or nothing.** Every fact, number, quote, price, person and photo is true and traceable to the
   research, the config or Stripe. No invented testimonials, ratings, customers, stock people, logos or
   statistics; no placeholders shipped. Research voices are not your customers: never show them as
   testimonials. A missing fact is asked for or left out.
3. **0 errors, 0 warnings.** The compiler is the floor. Fix the cause a finding names; never silence it.
   `data-design-reason="…"` is only for a choice [[.ConceptFile]] states, and its text says that reason.
4. **Look at the whole page after every change.** A fix that looks right in the edited spot often breaks a
   neighbour, the phone layout or a state you did not open. Chosen screenshots are not a review.
5. **Keep what works.** Preserve accepted copy, imagery and the `Qualities` of [[.ConceptFile]]. Fix
   locally; rewrite a whole section only when its job fails.

## Before you start

- Read [[.ConceptFile]] in full, then the voices it cites in [[.Voices]], then `lfm.json` and the template.
- Run `lfm compile`. If research or concept findings remain, or [[.ConceptFile]] is missing, finish those
  first (`lfm skills research`, `lfm skills design`); building without them produces the typical page.
  `concept.open` lists questions for the owner (price, deliverable, data to source): get the answers and
  write them into [[.ConceptFile]] before building the part they affect.
- Choose the mode:
  - **Build**: the template is still the starter. Build the spine first, then the rounds below.
  - **Rebuild**: a page exists and [[.ConceptFile]] has a `Diagnosis`. Build the new spine beside the old
    markup, move over only what the concept keeps, delete the rest.
  - **Grow**: [[.ConceptFile]] gained a changelog entry. Change only the sections its entry names, then run
    the full review, because local changes shift the whole page.
- Say in one line which capabilities you have: image generation, a browser that can screenshot at a set
  viewport, Stripe access. The rounds below name a fallback for each.

## Round 1: the working spine

Make the core loop work with real content before any polish.

1. **Facts into config.** Product values go into `lfm.json` under `data` (offer, price, inputs, delivery,
   FAQ answers); every destination into `urls`. The template renders them; it never repeats a literal.
2. **One section per spine item**, in order, each doing the job [[.ConceptFile]] gives it. Headings name
   what the section answers; one `h1`. Semantic landmarks, native controls, real `href`s.
3. **The mechanism first.** The tool, sample, configurator or game works with real data before anything is
   styled, and the initial HTML already shows a meaningful starting case (a worked example, filled defaults,
   a real excerpt), never an empty box, a zero result or a disabled button.
4. **Tokens from the concept.** Declare the palette as custom properties named by role, the typefaces of
   the Type table and the Form rules (corners, edges, elevation) once; components use the tokens only.
   Body text meets 4.5:1 against its actual ground, including over images.
5. **Offer and checkout.** The Offer of [[.ConceptFile]]: price, what arrives, how and when, beside a native `<a href>` to the hosted
   checkout (see Commerce). Findable from the first screen without scrolling the whole story.

Compile now and fix everything before continuing.

## Round 2: pixels, motion and words

**Imagery.** Produce each image of the Imagery table for its job. Brief every image in writing before
generating it: job and placement; the one visual idea; fixed facts that must stay accurate; medium,
viewpoint, framing and focal point; light source and how the materials respond; palette roles; aspect
ratios with mobile crop and copy space; acceptance criteria. Take art direction from what the buyers
photograph and film (cited in [[.ConceptFile]]), not from stock aesthetics. Then:

- Explore a few distinct images, choose, then refine by changing one named fault at a time. Judge every
  revision against the brief, not only against the previous render.
- Inspect at full size and at the sizes it ships: anatomy, hands, contact and shadows, object counts,
  perspective, food and material texture, lettering, domain details. Reject waxy uniform surfaces, global
  glow or bokeh, repeated textures, generic attractive faces and spectacle that forgets the offer.
- Exact text, numbers, logos, charts and interface stay in HTML or SVG, layered over the image.
- Evidence (what the buyer receives or encounters) must be real material; generated images may illustrate,
  never stand in for the product, a place, a person or a result.
- Ship sensibly sized WebP or AVIF with honest dimensions; the hero is never lazy-loaded.
- Fallback ladder: an image generation tool → photos or files from the user → images rendered in code
  (SVG, canvas, CSS) when the concept allows → ask the user, and leave the row unbuilt rather than filling it
  with stock or placeholder services.

**Motion.** Build the one orchestrated moment and the feedback motion of [[.ConceptFile]]; nothing else
moves by default. Content is readable before and without any animation (no opacity-zero reveals hiding
text). Interrupting or reversing works from the current state. A `prefers-reduced-motion: reduce` rule
shows the concept's reduced state. Anything moving by itself for more than five seconds can be paused.

**Copy.** Write from the Copy section and the voices it cites: the buyers' own words, the stated tone.

- Lead with the recognizable situation or useful outcome; name the product plainly. Every section answers
  one of: what is it, is it for me, what do I get, why believe it, what happens next.
- Proof sits beside the claim it supports; conditions stay attached to the promise they limit.
- Action labels name the actual next step and outcome; say when the next step is payment.
- Cut every sentence whose removal changes nothing. FAQs only for real open questions.
- Replace each of these with the concrete point: "in today's world" openers, "seamless", "powerful" and
  other unearned superlatives, repeated "not just X, but Y", habitual triplets, "highlighting its
  importance" endings, recap paragraphs, "we understand how overwhelming…", automatic hedges, curiosity
  bait. `lfm compile` reports the common phrases; the rest is your read-aloud pass.
- In German, choose du or Sie from the voices and keep it; write native phrasing, not translation.

## Round 3: discovery, sharing and commerce

**Search and answers.** `title` names the task or product and its useful distinction; `description`
previews what the page delivers. The answer, offer, evidence and links are in the initial HTML, readable
without clicks or scripts. Each passage stays true when quoted alone: subject, numbers with units, dates,
conditions. Cite primary sources beside checkable claims. Structured data goes in `jsonld` in `lfm.json`,
with values matching the visible page; the compiler emits and checks the block.

**Sharing.** Decide the shared object (a result, comparison, plan or challenge), who receives it and what
they do next. Put only public state in the URL with `URLSearchParams`, restore it on load and handle bad
input; the canonical never changes. The social image shows the offer at thumbnail size with its alt text.
Offer the share after value appears, show what will be shared, and never gate a promised result behind it.
Add platform cards (X player, Discord, Farcaster, Snapchat) only when the audience is there; the compiler
checks their metadata.

**Commerce.** One Stripe Payment Link per fixed offer, opened by a native link; no Stripe.js, no backend.
Record each offer under `data.commerce.offers` with SKU, name, `checkoutUrlKey` naming a `urls` entry, the
price in minor units with currency and the Stripe IDs read back from Stripe; add a `BuyAction` to the offer in
`jsonld`. Delivery-critical inputs are collected in Stripe (at most three custom fields). Never put secret
keys in the project. Without Stripe access, ask the user for the link and IDs; do not invent them.

## Round 4: review and fix

Run `lfm compile --release`: it adds headless Chrome checks (runtime errors, the layout of every half
viewport at phone and desktop widths, Lighthouse when installed). Then review by looking:

1. **Frames.** Screenshot every half viewport, top to bottom, at 390×844, 1280×680 and 1440×900, with the
   default state untouched, and read each at full size. Every frame shows legible content or an obvious
   continuation. Then exercise the mechanism: empty, extreme and invalid input, back, reload, a shared URL.
2. **Passes**, each over the whole page:
   - *Conversion*: in five seconds a visitor knows who it is for, what they get and what to do; first
     value without hunting; price, delivery and the next step findable; objections met at the decision.
   - *Research fidelity*: go through the Axes table of [[.ConceptFile]] row by row; each chosen position is
     visible beyond the first screen. Each spark used is traceable. Nothing resembles the market baseline.
   - *Coherence*: every section speaks the concept; none falls back to a template layout; the concept
     never blocks the task or the purchase.
   - *Specificity*: swap a competitor's product into each section; if it still works, it is a theme.
   - *Detail*: the failure modes below, all states, keyboard and focus, 200% zoom.
3. **Findings.** At most five, ranked by buyer impact, each with frame, cause and the quality to keep.
   Fix causes, recompile, rerun every frame. Repeat until no finding is above detail level, usually one or
   two rounds. With a second agent available, let it review without your build context: it reads the
   research, [[.ConceptFile]] and the frames, and nothing else.
4. **Ceiling.** One last pass that raises the signature moment and the weakest section by one level, then
   a final full review.

Without a browser that screenshots, rely on `lfm compile --release` findings, ask the user for phone and
desktop screenshots, and report the visual review as not done rather than passed.

## Detail failure modes

These ship on pages that looked finished in chosen screenshots; check each in the frames.

| Failure | Countermeasure |
| --- | --- |
| Empty or half-faded frames in scroll scenes | Each item's legible span exceeds the distance to the next; fade whole units; keep a continuation cue in pinned parts |
| Label above every heading | Keep one only when it adds what the heading lacks: time, step, price, status |
| Decoration cut at a section edge | Keep it inside its section or fade it before the clipping edge |
| Numbers and units split across lines | Render values as one unbreakable token; test the longest value at 320–390 px |
| Positioned labels clipped on phones | Place per breakpoint or let them flow |
| Empty default state dominates | Show a labelled example the visitor overwrites |
| Pinned object covers text on phones | Give phones a reading zone it never overlaps |
| Long uniform stretch | Fold secondary detail into disclosures; vary each screen's composition |
| One rich asset beside rough secondary art | Raise the secondary art or present it typographically |
| Orphans in short text | `text-wrap: balance` for four lines or fewer; tighten the copy first |

## Gotchas

- `lfm serve --open` recompiles on save; the preview is the compiled output, not the template.
- Every rendered link must be a named `urls` entry; add the URL to `lfm.json`, not to the markup.
- Fonts: WOFF2 files with `font-display`, a small set of weights; the concept's faces, nothing else.
- Values that change with input use tabular numerals and units in the same element.
- A warning you cannot fix without breaking the concept means the concept needs a change; edit
  [[.ConceptFile]] first, then the page.
