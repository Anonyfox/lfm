---
name: lfm-research
description: Collects verbatim posts, comments and reviews from real buyers and records how competing pages open and sell, keeping [[.Voices]] and [[.Competitors]] exact. Use before designing or rewriting a sales page, when new customer signals arrive, or when asked for voice-of-customer research, review mining, what people say about a topic, or a competitor analysis in an lfm project. Not for writing copy or making design decisions; those read these files.
license: MIT
compatibility: Works with any toolset. Uses web search, page fetching, a browser or data connectors when available, and asks the user for sources otherwise.
metadata:
  generator: lfm [[.Version]]
---

# Research for an lfm page

A model asked for an original page returns the typical page. Real human writing is the input that breaks
this: specific, uneven and different for every topic. This skill collects it, plus a factual record of what
competitors already do, into two files that every later design decision cites by ID.

## Rules that always hold

1. **Verbatim or nothing.** Copy the relevant passage exactly as written, typos included; mark cuts with
   `…`. Paraphrase, translation or cleanup turns a buyer's voice into model text, which is what this
   research exists to avoid.
2. **Never write from memory.** Every item comes from a source you opened in this session or the user
   supplied. A fluent invented quote cannot be told apart from a real one later, so a gap is always
   recorded as unreachable, never filled.
3. **Read before write, append only.** Open the existing file first and add a new dated batch. Existing
   entries are never edited, reordered or deleted, because later decisions cite their IDs.
4. **Every item is retrievable.** URL, platform, author type, post date when shown, fetch date and how it
   was fetched. If someone cannot reopen it, it is not evidence.
5. **Public access only.** Search engines, public pages and threads. Never log in, bypass a wall or scrape
   at volume; record the platform as unreachable instead.
6. **Facts only in these files.** Coding, themes, axes and conclusions belong in the project brief and are
   derived again from these files each time, never from an earlier interpretation.

## Step 0: check what you can reach

Look at the tools you actually have; do not assume. Use the highest rung available for each source and
say in one line which mode you are in before starting:

1. A connector or API for the platform (social, reviews, analytics).
2. Web search plus opening the result pages.
3. A browser for pages that need JavaScript or render comments late.
4. Sources the user pastes or exports into the conversation.
5. None of the above for a platform: list it as unreachable in the batch header.

The output format is the same in every mode; only how sources are obtained changes.

## Choose the mode

- **From zero** when a file is missing or below its minimum. Create it with the header from the format
  section, then collect until both minimums hold.
- **Ongoing** when a new signal arrives (customer messages, reviews, analytics, launch reactions, a new
  competitor) or one aspect needs depth (an objection, a section, a platform not yet covered). Append one
  batch to the file it concerns.

## Workflow

Copy this checklist into your notes and tick it off:

```
- [ ] Step 0 done, mode stated
- [ ] Both files read in full; next free IDs noted
- [ ] Queries planned, including ones that could contradict the current view
- [ ] Voices collected and written as a new batch
- [ ] Peers visited and written as a new batch, grid and baseline updated
- [ ] Validation passed for both files
- [ ] Gaps and unreachable platforms reported to the user
```

**Voices.** Prefer the platforms visitors arrive from: short video, image and microblog platforms first,
then communities and reviews (forums, app and marketplace reviews, review sites, Q&A threads). Seek the
buyers' own words about the problem, the current options, the result they want, what they make and post,
and what annoys them. Keep the mix honest:

- at most 40% of a batch from one platform and at most two items per author, so one loud corner cannot
  dominate;
- praise, complaints, rituals, questions and posts people made themselves, recent and older;
- at least one search per batch aimed at disagreement with the majority or the current page.

Skip AI-generated posts, marketing posing as users, and affiliate or promotional content; when unsure,
leave it out. For an image or video post, copy the caption or spoken words verbatim and add a factual
observer note: subject, framing, colour, light, text overlay, format, pace.

**Peers.** Pick pages the buyer would compare against, including some that rank in search and some that
circulate where the buyer looks. Open each at phone width when you can and record the facts below. Then
recount the grid across all peers, update the market baseline (the most frequent value per column: the
page this market already has) and list gaps no peer uses.

**Minimums from zero:** [[.MinVoices]] voices from [[.MinPlatforms]] platforms, including an image or video
platform when one is reachable, and [[.MinPeers]] peers. **Stop** when the minimums hold and the last round
of searches mostly returned kinds of items already in the file. Ongoing batches have no minimum; they end
when the trigger is covered.

## [[.Voices]] format

Use exactly this structure, so batches from different sessions and tools stay comparable:

```markdown
# Human voices: <project>

## Batch <YYYY-MM-DD> · <from zero | ongoing: trigger> · queries: "<query>", "<query>" · unreachable: <platforms or none>

### V<NNN> · <platform> <place> · <buyer | creator | brand | expert> · posted <YYYY-MM-DD | unknown> · fetched <YYYY-MM-DD> · via <connector | search | browser | user>
> <exact text>
<https://source.url>
Observer note: <only for image and video posts>
```

IDs are `V` plus three digits, continuing across batches (`V001`, `V002`, …), never reused. Numbers that
describe behaviour (sales, clicks, survey counts) go into their own table under the batch heading, each
row with its source.

## [[.Competitors]] format

```markdown
# Competitor analysis: <project>

## Batch <YYYY-MM-DD> · <from zero | ongoing: trigger>

### P<NNN> · <name> · <https://peer.url> · fetched <YYYY-MM-DD> · via <connector | search | browser | user>
- Opening: <headline claim>; <hero>; <first action>
- Palette: … · Type: … · Imagery: … · Motion: …
- Mechanism: … · Proof: … · Price: … · Tone: … · Takeaway: …

### Grid after this batch

| Column | Values with counts |
| --- | --- |
| Opening | <value> 7, <value> 3, … |

### Baseline

<One paragraph: the most frequent value per column.>

### Gaps

- <a value no peer uses>
```

## Validate before saving

Run this against each file and fix anything that fails before you finish:

- headings and field order match the format exactly; dates are `YYYY-MM-DD`;
- IDs are unique, consecutive and continue from the previous batch;
- every entry has a URL, every quote sits in a `>` block, and no field is an empty placeholder;
- no earlier batch changed (compare against what you read);
- from zero: the minimums and the diversity limits hold, or the shortfall is named in the batch header.

## When a source is unreachable

Name it in the batch header's `unreachable` list and move to the next rung or platform. If nothing is
reachable at all, ask the user to paste or export sources, and stop rather than write anything else.

## Gotchas

- Search result snippets are not the post. Open the page; snippets are cut and reordered.
- Rendered comment threads often load late. Use a browser, or record the platform as unreachable.
- Quoting your own summary of a page as a voice is fabrication with extra steps.
- The same post often appears on several sites. Keep the original and skip the copies.

## Examples

<example>
### V014 · Reddit r/pourover · buyer · posted 2026-01-13 · fetched 2026-10-17 · via search
> Numbering feels way less intimidating than words. And yeah, if someone suddenly goes 'I loved #3' it makes the follow up so much easier.
<https://www.reddit.com/r/pourover/comments/1qbely1/>
</example>

<example>
### V031 · TikTok · creator · posted unknown · fetched 2026-10-17 · via browser
> the payoff is coming I can feel it
<https://www.tiktok.com/@example/video/7656547532020976910>
Observer note: phone screen recording of a code editor, white caption box at the top, talk-show still inserted after two seconds, fast cuts.
</example>

<example>
## Batch 2026-10-18 · ongoing: first refund request · queries: "refund", "didn't work for me" · unreachable: Instagram (login wall), X (login wall)
</example>
