---
name: lfm-design
description: Turns an lfm project's research into concept.md, the single brief implementation builds from - buyer, decision path, mechanism, design axes, the chosen direction, page spine, palette, type, form, imagery, motion and copy, every decision cited to research IDs. Use after research is collected and before building or rebuilding a page, when new research batches should reshape an existing page, or when asked to design, concept or art-direct an lfm page. Not for collecting research or writing the page's code.
license: MIT
compatibility: Needs the research files of the project; works with any toolset. Running lfm compile checks the result.
metadata:
  generator: lfm [[.Version]]
---

# Design for an lfm page

The page a model designs from its own taste is the typical page. This skill designs from the research
instead: what buyers wrote decides what to match, what competitors show decides what to break from, and
every decision in [[.ConceptFile]] names the evidence it stands on. [[.ConceptFile]] is the only file you write; it is the
whole brief for implementation, so a decision missing from it is a decision nobody made.

## Rules that always hold

1. **The research is the only source of taste.** Read [[.Voices]] and [[.Competitors]] in full and cite
   them by ID (`V012`, `P003`) wherever a decision rests on them. Your own preferences, other projects and
   example pages are not evidence; they pull every page toward the same average.
2. **Derive, do not recall.** Work from the files as they are now. When a concept exists, read it, but
   re-derive the voices and axes from the research rather than copying the old interpretation forward.
3. **Write only [[.ConceptFile]].** No markup, styles or assets in this phase; implementation follows the file.
4. **Strike before you choose.** The first two directions you write are the most probable ones; they are
   always struck. Choose by recorded criteria, never by which direction feels best, because preference
   favours the typical.
5. **Name no banned looks while ideating.** Generator defaults are checked by `lfm compile` after you
   choose; listing them in your head first makes them likelier.

## Before you start

- Run `lfm compile` if you can. The research lines must show the minimums met ([[.MinVoices]] voices from
  [[.MinPlatforms]] platforms, [[.MinPeers]] peers) or a named shortfall. With less, collect research first
  (`lfm skills research`).
- Choose the mode:
  - **New**: no concept yet. Write every section.
  - **Rebuild**: a page exists without a concept. Write `## Diagnosis` first (what is generic, what is
    strong, what the visitor came to do), then set the old page aside until a direction is chosen.
  - **Grow**: research gained a batch since the concept's last changelog entry. Re-derive voices and axes
    from the whole files, then classify each change: depth (a new phrase, objection or proof: edit the
    affected sections), shift (an axis moves because the balance of evidence flipped, not because new
    items are vivid: redo only the decisions on that axis), or reset (the buyer or the core job was wrong:
    start a new concept). Keep the `## Qualities` intact unless the evidence says otherwise, and log it.

## Workflow

Work through the sections in order; each builds on the one before.

1. **Voices.** Code the voices: the phrases buyers use, the feelings they want and dread, their rituals
   and in-jokes, what they photograph and film and how, their complaints about current options. At least
   [[.Design.MinVoiceNotes]] observations, each citing its voices.
2. **Buyer.** Who arrives, from where, what just happened, what they compare against, what they must
   believe to pay, and the hesitation most likely to stop them.
3. **Decision path.** The visitor's questions in order, each with the job of the section that answers it:
   recognize the offer, get first value, see proof beside the claim, meet the objection at the decision,
   the offer itself, what happens after paying. At least [[.Design.MinPathSteps]] steps.
4. **Mechanism.** What the visitor does on the page: a tool or calculator, a configurator, a sample to read
   or use, an explainer, a diagnostic or quiz, a comparison, a game, a simulation, or a plain answer. Score
   the candidates on speed to first value, strength of proof, effort, honesty risk and share value. Choose
   one, reject at least [[.Design.MinRejected]] with reasons. A plain page wins when it serves best.
5. **Facts.** At least [[.Design.MinFacts]] checked facts true of this subject and hardly any other: numbers, units
   and timings, textures, sounds and light, tools and vocabulary, failure modes and rituals, places. Genre
   words such as modern, premium or clean are not facts. Each fact cites its source.
6. **Axes.** At least [[.Design.MinAxes]] design axes on which the data separates. Per axis: the market
   position with the peers that hold it, the voice position with the voices that hold it, and your chosen
   position: break from the market where it clusters, match the voices. Then imagine the page a generator
   makes for this topic with no research; if a chosen position lands there, it is no break, unless it is
   the subject's literal material and the voices support it.
7. **Sparks.** [[.Design.MinSparks]] to [[.Design.MaxSparks]] details that are specific, surprising, recur in the voices (two or more
   IDs each) and appear in no peer. Check [[.Competitors]] before you call something unused.
8. **Directions.** Write at least [[.Design.MinDirections]] directions fast, each built from a spark and three facts,
   seen through ordinary people in the topic rather than a visionary. Make them differ in representation
   (scene, notation, data, type, sound, plain tool), interaction, time and what the visitor takes away.
   Push each one further before judging any. Then strike, with the reason: the first two; any whose
   one-line description still works with a competitor's product swapped in; any that matches the market
   baseline on opening, palette, imagery or mechanism; any that slows the first value; any that is merely
   tasteful where the voices are not. Mark exactly one as chosen.
9. **Concept.** The chosen direction, pushed once more: what would make it astonishing without slowing
   the visitor? Say how it builds on its sparks.
10. **Spine.** The page's sections in order, each with its job and the evidence it shows.
11. **Visual system.** Derive palette, type, form, imagery and motion from the concept and the axes. For
    each, list about eight candidates privately with the chance a typical generator picks each, and choose
    one below roughly 10% that serves the spine and cites a voice, peer or fact. Declare every color the
    page will use, with at least one text and ground pair at 4.5:1 contrast. Plan the images by job
    (protagonist, proof, atmosphere, explanation, share preview), with art direction taken from what the
    buyers photograph and film. Decide one orchestrated motion moment, the feedback motion, and what the
    page shows with reduced motion.
12. **Copy.** The buyers' own words to use, the tone, and the words to avoid, each citing voices.

## [[.ConceptFile]] format

Use exactly these headings in this order; the words under them are yours. `Diagnosis`, `Qualities` and
`Changelog` are optional; every other section is required. Replace every `<…>`.

```markdown
# Concept: <project>

## Diagnosis
<only when rebuilding: what is generic, what is strong, what the visitor came to do …>

## Voices
- <observation in your words …> V001 V014

## Buyer
<who arrives and why, citing voices …>

## Decision path
1. <question the visitor asks …> → <job of the section that answers it …>

## Mechanism
Chosen: <mechanism and why …>
Rejected: <alternative …>: <why …>

## Facts
- <fact true of this subject …> V007 or P002 or https://source

## Axes
| Axis | Market | Voices | Chosen |
| --- | --- | --- | --- |
| <axis …> | <position …> P001 P004 | <position …> V003 V019 | <position …> |

## Sparks
- <recurring detail no peer uses …> V011 V023

## Directions
1. <direction …> V011 → struck: <reason …>
2. <direction …> V023 → chosen

## Concept
<the chosen direction pushed once more, built on its sparks …> V011

## Spine
1. <section …>: <its job and the evidence it shows …>

## Palette
| Role | Color | Source |
| --- | --- | --- |
| <role …> | #rrggbb | <where it comes from …> V005 |

## Type
| Role | Family | Source |
| --- | --- | --- |
| <role …> | <family or stack …> | <why this face …> |

## Form
<shape, corners, edges, elevation and the layout idea …>

## Imagery
| Image | Job | Art direction | Source |
| --- | --- | --- | --- |
| <image …> | <job …> | <art direction …> V008 | <generated, photographed, drawn or rendered …> |

## Motion
Moment: <the one orchestrated moment …>
Feedback: <motion that answers the visitor …>
Reduced motion: <what shows instead …>

## Copy
- <words to use, tone or words to avoid …> V002

## Qualities
- <approved quality implementation must keep …>

## Changelog
- <YYYY-MM-DD>: <what changed, from which research batch, what was kept …>
```

- Citations are bare IDs anywhere in the text: `V003`, `P012`. Every ID must exist in the research files.
- A deliberately imageless page writes `None: <reason>` under Imagery instead of the table.
- Every list item stays on one line or continues on lines indented by two spaces.

## Validate before finishing

Run `lfm compile` and fix every `concept.*` finding; it checks the sections, the citations and their
spread across voices, platforms and peers, and the generator defaults in palette, type and imagery. Then
reread [[.ConceptFile]] as the implementer will:

- Could someone build the page from this file alone, without asking you anything?
- Does every axis's chosen position show up in the spine, the visual system or the copy?
- Would the concept still work with a competitor's product swapped in? Then it is a theme; go back to the
  directions.
- Is anything justified by taste, another project or an example page rather than an ID or a fact?

## Gotchas

- A spark that appears in a peer is the market, not a spark.
- Citing many IDs on one line does not make a weak claim strong; cite the voices that actually say it.
- Colors from the tool or kit you would normally use are not research; trace each one to a source.
- After implementation starts, `lfm compile` holds the page to this file's palette, typefaces and imagery.
  Change the concept first, with a changelog entry, when a decision itself changes.
