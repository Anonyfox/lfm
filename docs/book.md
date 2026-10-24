# Book concepts

Set deliverables.book.kind to nonfiction (the default) or fiction.

Fiction sections appear in this order: Reader, Premise, Cast, World, Arc, Hooks, Voice,
Chapters, Matter (optional), Imagery, Look, Qualities (optional), Changelog (optional).
Reader requires Not for; research citations are optional. Premise, World and Arc must be nonempty.
World describes setting, time and rules. Arc describes the story arc or timeline.
Cast has at least one list item: Name: role and want. Name (age): role and want is also accepted.
Hooks requires Title and Opening; Subtitle is optional. Voice requires POV, Tense and quoted
Avoid phrases. Quoted Use phrases are optional. The other sections use the nonfiction format.

A quoted Title or Subtitle ends at its closing quote. Put notes on an Open: line.
Both kinds use the chapter columns #, Title, Job, Beats, Words, File and optional Status.
Status is planned, draft or done; empty means draft. Jobs need at least four words.
Planned chapters count toward the word target but skip prose checks and may have no file.
A book is complete only when all chapters are draft or done and written.
The next chapter is the first unwritten draft or done row, then the first planned row.

For written fiction chapters, cast names in Job or Beats must appear in the prose.
A quoted Hook: sentence in Beats must appear in the last 300 words.

## Matter

Matter follows Chapters. Files are read and checked but are not rendered yet.

| Part | Place | File |
| --- | --- | --- |
| Dedication | front | [dedication](matter/dedication.md) |
| Author's note | back | [note](matter/note.md) |

Front matter can include dedication, epigraph, content notes and foreword.
Back matter can include afterword, author's note, acknowledgements, about the author and credits.
Each file starts with # followed by its Part. Avoid phrases apply; length, copy quality,
value and machine checks do not. A row containing Open: is skipped and reports its question.

## Fiction checks

Fiction has no minimum voice or peer counts. Cited research IDs must still exist.
book.value does not apply; cross-chapter repetition is reported as book.copy.
Fiction skips nonfiction readability bands, Wiener Sachtextformel, invented-example names,
unsourced claims, generated nonfiction lines, question openings, one-line paragraph share,
minimum paragraph length, fragments, hedged promises, ending recaps and abstract-noun share.
Other prose checks, including AI tells and book.machine, still apply.
Voice Use phrases are checked only when listing.md exists, against its description, short
description, back cover, keywords and tags. Avoid phrases still apply to chapters and matter.

Tools in listing.md is a comma list of names, optionally followed by a model in parentheses.
Statement names each tool without needing its model. It is printed on the copyright page.
An Open: Statement reports that it would be printed.

An Imagery row containing Open: waits for the owner and has no missing-image finding.
A cover row with Open: reserves the cover; a linked cover path is retained.
