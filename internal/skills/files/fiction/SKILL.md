---
name: lfm-fiction
description: Plans and writes a novel, novella or story collection as the book deliverable of an lfm project - [[.Book.Concept]] with premise, cast, world, arc and voice, chapters written one at a time against their outline row, front and back matter, then the same EPUB, PDF, download page and store packages as every lfm book. Use when deliverables.book.kind is fiction, or when asked to outline, draft, continue or revise a story sold as a book. Not for nonfiction books, guides or handbooks, which use lfm-book; cover, delivery and stores follow lfm-book too.
license: MIT
compatibility: Needs lfm; Chrome or Chromium for the PDF. Uses image generation and a browser when available, and asks the owner for what it cannot decide.
---

# Write fiction with lfm

A story a model drafts from its own knowledge reads like the average of its genre. This one is planned in
one lean file the owner can argue with, then written one chapter at a time with that plan, the previous
ending and the next row in view. `lfm compile` holds every chapter to its row, the voice and the cast,
and builds the same files and store packages as any lfm book.

Read `lfm skills book` once: its rules, phases 4 to 6 (cover, files, delivery, stores) and the listing
format apply unchanged. This skill replaces its research step and phases 1 to 3.

## Rules that always hold

1. **The concept is the brief.** Every chapter does its row's job. When the story changes, change
   [[.Book.Concept]] first, with a Changelog line, then the chapters.
2. **Invented story, real facts.** People, places and events of the story are yours; facts about the real
   world (history, medicine, law, a real place) are true or marked `Open:` for the owner.
3. **One chapter at a time, the whole in mind.** Before each chapter, reread Premise, Cast, World, Arc,
   Voice, the chapter's row, the end of the previous chapter and the next row.
4. **0 errors, 0 warnings.** Fix a `book.copy` finding by rewriting the moment, never by swapping a synonym.
5. **The owner decides** the title, pen name, price, content limits, ISBN and where the book is sold.

## Setup

- In `lfm.json`: `"deliverables": {"book": {"kind": "fiction", "author": "…", "sell": "shops"}}` and the
  publisher as `"seller": {"name": "…", "imprint": "…"}`; `sell` and `path` work as in lfm-book.
- Research is optional. Fiction has no minimum voices or peers; IDs you cite must exist in `research/`.
  Reader comments on comparable books help with Reader, Hooks and the listing.

## Phase 1: the concept

Write [[.Book.Concept]] in exactly this format; the words are yours:

```markdown
# Book: <working name>
## Reader
<who reads this, which books they already love and why>
Not for: <who should not pick it up, content they would not want>
## Premise
<who wants what, what stands in the way, what is at stake, in two or three sentences>
## Cast
- <Name>: <role, what they want, what they hide or fear>
## World
<setting, time and the rules the story keeps: society, law, technology, anything that limits choices>
## Arc
<the arc or timeline: the turning points in order and how it ends>
## Hooks
- Title: "<title>"
- Subtitle: "<optional>"
- Opening: <the first page: whose moment, what is wrong>
## Voice
POV: <whose eyes: first person, close third, alternating between named characters>
Tense: <past | present>
- Avoid: "<a phrase the book never says>"
- Use: "<optional: a phrase readers search for; the listing must carry it>"
- <rhythm, dialogue, humor, how much is shown on the page>
## Chapters
| # | Title | Job | Beats | Words | File | Status |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | <title> | <what changes for whom> | <beats with cast names>; Hook: "<closing line>" | 3000 | [01-name](chapters/01-name.md) | planned |
## Matter
| Part | Place | File |
| --- | --- | --- |
| Dedication | front | [dedication](matter/dedication.md) |
| Author's note | back | [note](matter/note.md) |
## Imagery
| Image | Chapter | Job | Art direction | File |
| --- | --- | --- | --- | --- |
| Cover | cover | cover | <what the cover shows, readable at thumbnail size> | [cover](images/cover.jpg) |
## Look
Trim: <5x8in | 5.5x8.5in | 6x9in | A5 | A4 | letter>
Body: <family>, <fallback families>, <generic family>
Headings: <family>, <fallback families>, <generic family>
Accent: <#rrggbb>
## Qualities
## Changelog
```

Work through it in order:

1. **Reader, Premise, Cast, World, Arc**: concrete enough that two writers would tell the same story.
   Each cast member wants something the story can refuse. The Arc names its ending.
2. **Hooks**: the title in quotes, notes on an `Open:` line below it, never after the closing quote.
3. **Voice**: one POV and tense decision, exact `Avoid:` phrases; `lfm compile` holds chapters to them.
4. **Chapters**: at least [[.Book.MinChapters]], each with a job of four words or more that says what
   changes. Name the cast members on stage in Job or Beats; the compile checks the chapter mentions them.
   A quoted `Hook:` in Beats must appear in the chapter's last 300 words. Word targets from
   [[.Book.MinWords]] to [[.Book.MaxWords]], met within [[.Book.Tolerance]]%.
5. **Status**: `planned` rows may have no file or a placeholder and skip prose checks; `draft` (or empty)
   and `done` rows are checked in full. The book builds once no row is planned; until then the compile
   writes a preview of the written chapters.
6. **Matter**: front or back parts, each `matter/<name>.md` starting with `# <Part>`. Any cell with
   `Open:` waits for the owner. Matter has no length check; Avoid phrases apply.
7. **Imagery and Look** as in lfm-book. A cover row whose cell says `Open:` reserves the cover.

Then reread it as the writer of the middle chapter would. Could each chapter be written from its row,
the Cast and the Arc alone? Run `lfm compile` and fix every `book.concept` finding; `book.open` stays
until the owner answers.

## Phase 2: one chapter at a time

The Next line names the chapter to write. For it:

1. Reread as rule 3 says. In your notes, write the obvious version of the scene first, then what only
   this cast in this world would do instead.
2. Write `[[.Book.Dir]]/chapters/<file>.md`: `# <the planned title>`, scene breaks as `##` headings or
   paragraphs, no lists unless the story shows one. Set the row's Status to `draft`.
3. Run `lfm compile` and fix every `book.chapter` and `book.copy` finding in this chapter. Treat a
   `book.machine` finding as in lfm-book: rewrite the named passage once, then move on.
4. When the owner or the whole-book pass has accepted it, set Status to `done`.

How a chapter earns its place:

- **Enter late, leave early.** Start inside the scene and end on the change, the question or the Hook.
- **Someone wants something on the page**, and something resists. A chapter where nothing is decided goes.
- **Concrete over named feelings.** The object, the gesture, the words said; let the reader name the feeling.
- **Each voice its own.** Characters differ in what they notice and how they speak; cut dialogue tags
  that only repeat what the line shows.
- **Keep the world's rules.** A rule broken without cost breaks the reader's trust; check World and earlier
  chapters for names, ages, places and timeline.

## Phase 3: the whole book

Once no row is planned, read the book in one pass: the first tenth that stores show as a sample, every
setup paid off, every cast member's want answered by the end, the timeline against Arc. Write three
one-star reviews in your notes (the genre reader, the reader who guessed the ending, the reader who lost
track of who is who) and fix what each would quote. Ask the owner to read the opening, a middle chapter
and the ending. Then continue with phase 4 of lfm-book; in listing.md, Tools lists names, optionally
with the model in parentheses, and the Statement names each tool.
