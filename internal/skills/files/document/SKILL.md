---
name: lfm-document
description: Plans, writes and ships a set of editable document templates as the deliverable of an lfm project - letters, invoices and quotes, meeting minutes, programs, recipe pages, binders, contracts, handbooks, application sets, any document a buyer fills in Word, Google Docs, Pages or LibreOffice. One Markdown file per document and language, compiled with a theme into .docx files whose look lives in real styles, per language and paper size, printed and checked on a release, shipped as a ZIP per edition with PDFs, fonts, a start guide and listing images. Use when the product sold is an editable document or a set of them, or when asked to design, fix or list one. Not for the sales page, slide decks, printables meant to be printed as they are, spreadsheets or ebooks; those have their own skills.
license: MIT
compatibility: Needs lfm, and Chrome or Chromium for the release's prints, page checks, start guides and listing images. LibreOffice, when installed, adds a second opinion. Fonts come from Google Fonts.
---

# Build and ship editable documents with lfm

Buyers pay for a document that looks finished the moment it opens and stays easy to change. Their top
complaint about competing templates: everything laid out by hand, text boxes that fall apart, spacing
faked with empty lines, so typing breaks the page. Next come layouts that break in Google Docs, fonts that
will not install, sample content that is obviously fake, and files that do not match the preview. lfm
owns all of that: the theme becomes Word's own styles, layout is flowing text and borderless tables,
fonts are embedded, and a release prints every file the way other apps will. The work that stays yours
is the content and the plan.

## Rules that always hold

1. **The concept is the brief.** [[.Document.Concept]] plans every document; when the set changes,
   change the concept first, with a Changelog line.
2. **Write what a real buyer has.** Sample content reads like the real thing: real-sounding names,
   places, dates and amounts that add up. Never John Doe, Acme or lorem ipsum.
3. **Mark every field.** What the buyer replaces is `==Margaret Ellison==`; lfm highlights it in every
   app, and the start guide says how to clear the highlight when done.
4. **Never format by hand.** No spaces or empty lines for layout, no manual bold headings, no tabs. The
   theme sets sizes, spacing, margins and tables; you choose structure.
5. **Content is the value** for contracts, SOPs, handbooks and binders: pre-written, explained text with
   choices sells; layout-only versions of generic forms do not. Legal documents say they are no legal advice.
6. **Ask the owner** for the price, the seller name and the Google Docs step; never invent reviews.

## What sells

Proven beyond CVs: recipe pages and cookbooks, meeting agenda and minutes suites, letterhead with invoice
and quote, estate and emergency binders, book interiors for self-publishers, small-business kits for one
trade (photographer, event planner, daycare) with pre-written content, and German sets (Bewerbung with
Deckblatt, Anschreiben, Lebenslauf; DIN 5008 letters; Vereinsvorlagen; Hochzeitszeitung), which few
sellers serve. Generic single invoices, certificates and menus are dumped or owned by Canva. Sets and
matching suites carry the price; a lone simple page caps out at a few euros.


## Before you start

- Read the page's research (`lfm skills research`) for the buyers' words and complaints. Search Etsy for
  the product; open the top listings, what they include, their photos and reviews.
- Add `"deliverables": {"document": {"path": "<at least [[.Document.MinPath]] random lowercase letters and
  digits>", "version": "2027.1"}}` and `"seller": {"name": "…"}` to `lfm.json`. `lfm help config
  deliverables.document` lists every option; papers default to US Letter and A4 for `en`, A4 elsewhere.

## Phase 1: the concept

```markdown
# Documents: <product name>
## Buyer
<who fills these in, when, and in which app>
- Not for: <who it does not serve>
## Promise
<one sentence: what they have done once the set is filled>
## Documents
| Document | Job |
| --- | --- |
| <file name> | <what the buyer does with it> |
## Look
Theme: <one of [[.Document.Themes]]>
Accent: #rrggbb
## Changelog
```

Themes: classic (EB Garamond, indented paragraphs), modern (Inter), editorial (Source Serif 4 with Source
Sans 3 headings), minimal (IBM Plex Sans), warm (Lora), technical (Source Sans 3, grid tables). `Accent:`
recolors any theme; `Text font:` and `Heading font:` swap families, Google Fonts only. One theme serves
the whole set, so the documents match.

## Phase 2: fonts

Put the static TrueType files of every family the theme uses in [[.Document.Fonts]]/, with the family's
`OFL.txt`. The Google Fonts API lists them: `curl -s "https://fonts.googleapis.com/css2?family=Lora:ital,wght@0,400;0,700;1,400;1,700"`
prints one `.ttf` link per face; download each as `Lora-Regular.ttf`, `Lora-Bold.ttf`, `Lora-Italic.ttf`,
`Lora-BoldItalic.ttf`. lfm embeds them in every .docx (Word, LibreOffice), Google Docs knows the family
by name, and the ZIP ships the files for Pages users.

## Phase 3: the documents

One file per document and language: `[[.Document.Dir]]/en/invoice.md`, `[[.Document.Dir]]/de/invoice.md`.

```markdown
---
title: Plot Fee Invoice
pages: 1
header: Riverside Allotment Society
---
::: columns 3 2
::: title
Invoice
Riverside Allotment Society
:::
+++
**Invoice no.** ==2025-117==
**Due** ==1 December 2025==
:::

| Description | Qty | Price | Amount |
| --- | --- | --- | --- |
| Plot 31, half plot, 2026 season | 1 | 48.00 | 48.00 |
| **Total due** | | | **48.00** |
```

- Front matter: `title` (names the file), `pages` (the pages it must print on; leave out when it may
  grow), `kind: ceremonial` (programs, invitations, certificates may center text), `cover: true` (no
  header or footer on page one), `header`, `footer` (`{page} / {pages}` is the default beyond one page,
  `none` for none).
- `#`, `##`, `###` are Heading 1 to 3; a new line is a new line, a blank line a new paragraph.
- `**bold**`, `*italic*`, `[link](https://…)`, lists, `- [ ]` checkboxes, `> text` a box for notes.
- Tables: leave a column's alignment out and lfm aligns numbers right; a row starting in bold is a total
  with a rule above it. At most seven columns.
- `![alt](images/logo.png "40mm")` places a PNG or JPEG from [[.Document.Dir]]/images; give a width in mm
  or %, at least 150 dpi at that size.
- `---` alone on a line starts a new page; `* * *` is a section ornament.
- Directives: `::: title` (title line, optional subtitle line), `::: columns 1 2` (columns separated by a
  line `+++`), `::: form` (`Label: value` rows on fill-in lines), `::: signature` (one signer per line,
  up to three), `::: center` (ceremonial only). Close each with `:::`.
- Quotes, dashes and ellipses become the language's typographic ones by themselves.

Run `lfm compile` after every change and fix every `document.source`, `document.fonts`,
`document.design` and `document.content` finding. `lfm serve` shows every edition at
`/<path>/preview/<edition>.html`, or `/document/preview/` when sold in shops: read every document there.

## Phase 4: release

`lfm compile --release` prints every document in Chrome, laid out as its .docx: `document.layout` names a
document that runs over its planned pages, lands on another paper, or shows a font the theme does not
have (a missing character); LibreOffice, when installed, checks the real .docx too, and the report's
`optional` line names helpers that are not installed and what each would add, for the owner to decide. It ships each print
as a PDF in the edition's ZIP, prints the start guide (open in Word,
Google Docs, Pages and LibreOffice; fill, restyle, print; licence) and renders the listing images from the
real first pages. Open one .docx in Word, Pages or Google Docs yourself and type into it.

## Phase 5: Google Docs, optional, the owner's step

Only the owner's Google account can make the masters: upload a .docx to Google Drive, open it with Google
Docs, share it as anyone with the link, viewer, and add its address to `deliverables.document.docs` as
`"en/invoice"`. The start guide and the download page then carry a copy link. Never edit a master after
selling.

## Phase 6: listing, delivery, shops

Write [[.Document.Listing]] (Title, Description, Tags, Price, AI). The description opens with the buyer's
situation, lists the documents, the apps, the editions (paper sizes, languages) and that nothing is
shipped. Run `lfm compile` until `document.listing` and `document.channels` are clean.

- Sold through the page (`sell: page`), the report prints the download address: one ZIP per edition.
- Sold in shops (`sell: shops`), [[.Document.Distribution]]/ holds every ZIP, the images and a guide per
  shop with every field; link the live listing back to the page's offer.
- Sold with the project's deck or other deliverables as one kit (`sell: bundle`), `deliverables.bundle`
  lists them in its `units` and its own listing; `lfm help config deliverables.bundle` shows how.
- Hand the owner the README: accounts, prices, tax and uploads are theirs.
