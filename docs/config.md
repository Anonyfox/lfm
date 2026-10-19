# lfm.json reference

Generated from `internal/project/spec.go` by `go test ./internal/project` with `LFM_UPDATE_DOCS=1`; edit the spec, not this file. `lfm help config [field]` prints the same in the terminal, and every config finding quotes the field it is about.

| Field | Takes | Default | Meaning |
| --- | --- | --- | --- |
| `schemaVersion` | 1 | required | The format of this file; always 1. Example: `1`. |
| `url` | string | required | The absolute HTTPS URL of the folder the page is deployed to, ending in /: its canonical URL; download paths hang off it. Example: `"https://example.com/my-page/"`. |
| `language` | string | required | The page's language as a BCP 47 tag; deliverables default to it. Example: `"en"`. |
| `title` | string | required | The page title, shown in the browser tab, search results and share previews. Example: `"Keep your sourdough alive"`. |
| `description` | string | required | One sentence for search results and share previews: who it is for and what they get. Example: `"The jar check and the rescue feeding for a starter that smells wrong."`. |
| `socialImage` | object |  | The share preview image. |
| `socialImage.src` | string |  | Project path of the image, 1200×630 pixels. Example: `"assets/images/share.jpg"`. |
| `socialImage.alt` | string |  | What the image shows, for people who cannot see it. Example: `"A jar of starter on a kitchen shelf"`. |
| `seller` | object |  | Who sells: printed on the book's copyright page and the printable's guide and licence. |
| `seller.name` | string |  | The seller or publisher as buyers see it. Example: `"Brot & Glas Verlag"`. |
| `seller.imprint` | string |  | The lines of the legal notice (Impressum): address, register, VAT ID; printed on the book's copyright page. Example: `"Brot & Glas Verlag, Musterstraße 1, 12345 Berlin"`. |
| `urls` | map |  | Named complete URLs: checkout links, shop listings, contacts. Render them as {{.urls.<name>}}; every rendered link is checked against them. Example: `{"checkout": "https://buy.stripe.com/…"}`. |
| `urls.*` | string |  | One complete URL. Example: `"https://www.etsy.com/listing/123456/my-planner"`. |
| `links` | any |  | Optional free-form labels or key references for links. |
| `jsonld` | any |  | A structured data object or array the compiler emits as JSON-LD; strings may reference config, such as "{{.title}}". |
| `data` | any |  | Every product value the page uses: offers, prices, copy, FAQ. Each data/<name>.json adds .data.<name>. |
| `data.commerce` | object |  | The commerce facts lfm checks: the offers and their checkout links. |
| `data.commerce.offers` | map |  | The offers the page sells, by key; deliverables name theirs in offer. Checked by `commerce.config`. |
| `data.commerce.offers.*` | object |  | One offer. Example: `{"sku": "planner-2027", "name": "Planner 2027", "checkoutUrlKey": "checkout", "price": {"amountMinor": 990, "currency": "EUR"}}`. |
| `data.commerce.offers.*.sku` | string |  | A stable product code, unique across offers. Example: `"planner-2027"`. |
| `data.commerce.offers.*.name` | string |  | The offer's name as the checkout shows it. Example: `"Planner 2027"`. |
| `data.commerce.offers.*.checkoutUrlKey` | string |  | The key in urls of where the buy button goes: a Stripe Payment Link, or a shop listing for a deliverable sold in shops. Example: `"checkout"`. |
| `data.commerce.offers.*.price` | object |  | The price as the checkout charges it. Example: `{"amountMinor": 990, "currency": "EUR"}`. |
| `data.commerce.offers.*.price.amountMinor` | integer |  | The amount in minor units: 990 is 9.90. Example: `990`. |
| `data.commerce.offers.*.price.currency` | string |  | A three-letter currency code. Example: `"EUR"`. |
| `data.commerce.offers.*.stripe` | any |  | Optional Stripe IDs (productId, priceId, paymentLinkId) and the customFields snapshot of the Payment Link. Checked by `commerce.fields`. |
| `build` | object |  | How lfm builds the page. |
| `build.assets` | object |  | Whether resources are inlined into the page or emitted as files. |
| `build.assets.default` | auto | inline | file | auto: lfm decides per resource | The packing policy for every resource. Example: `"auto"`. |
| `build.assets.overrides` | map |  | Packing for single source paths; always wins over default. Example: `{"assets/images/hero.webp": "file"}`. |
| `build.assets.overrides.*` | inline | file |  | How this source path is packed. Example: `"file"`. |
| `build.browserData` | list of strings |  | Dot paths copied into <script type="application/json" id="product-data"> for scripts, keeping their nesting. Example: `["data.offer"]`. |
| `build.validation` | object |  | Rule levels. |
| `build.validation.rules` | map |  | Per-rule levels; required rules stay errors. `lfm rules` lists them. Example: `{"design.mono": "off"}`. |
| `build.validation.rules.*` | off | warn | error |  | The level of this rule. Example: `"warn"`. |
| `deliverables` | object |  | What buyers receive besides the page: a book, a printable, a spreadsheet, a set of editable documents, and the book and printable also as a bundle. |
| `deliverables.book` | object |  | A book in deliverables/book, written with `lfm skills book`. |
| `deliverables.book.author` | string | required | The name on the cover. Example: `"Lena Brot"`. |
| `deliverables.book.sell` | page | shops | bundle | page | How buyers get the files: page = your checkout redirects to an unlisted download page (needs path); shops = Etsy, Gumroad, Amazon and others deliver, the page's offer links to the listing; bundle = only inside deliverables.bundle. Example: `"page"`. |
| `deliverables.book.path` | string |  | The unlisted download folder, at least 16 lowercase letters, digits or hyphens; needed when sell is page. It is the only lock on the files; a compile without it prints a fresh one. Example: `"k3x9m2q7w8e4r5t6y1u0"`. Checked by `book.config`. |
| `deliverables.book.offer` | string |  | The key of the offer in data.commerce.offers that sells it; its price holds the listing's price, and its link must match sell. Example: `"book"`. Checked by `book.config`. |
| `deliverables.book.language` | string | the page's language | The book's language, when it differs from the page's. Example: `"de"`. |
| `deliverables.book.formats` | list of epub | pdf | ["epub", "pdf"] | The files the download page offers. Example: `["epub", "pdf"]`. |
| `deliverables.book.paper` | white | cream | white | The paperback's paper; it sets the spine width. Example: `"cream"`. |
| `deliverables.book.isbn` | object |  | ISBNs per edition; each is optional, ebooks need none in most stores. |
| `deliverables.book.isbn.ebook` | string |  | The ebook's ISBN-13, printed on the copyright page. Example: `"978-3-16-148410-0"`. Checked by `book.config`. |
| `deliverables.book.isbn.paperback` | string |  | The paperback's ISBN-13, printed on the copyright page and as the barcode on the back cover. Example: `"978-3-16-148411-7"`. Checked by `book.config`. |
| `deliverables.printable` | object |  | A printable in deliverables/printable (planner, game, puzzle pack), built with `lfm skills printable`. |
| `deliverables.printable.sell` | page | shops | bundle | page | How buyers get the files: page = your checkout redirects to an unlisted download page (needs path); shops = Etsy, Gumroad, Amazon and others deliver, the page's offer links to the listing; bundle = only inside deliverables.bundle. Example: `"shops"`. |
| `deliverables.printable.path` | string |  | The unlisted download folder, at least 16 lowercase letters, digits or hyphens; needed when sell is page. It is the only lock on the files; a compile without it prints a fresh one. Example: `"p7m2x9q4w8e3r6t5y1u0"`. Checked by `printable.config`. |
| `deliverables.printable.offer` | string |  | The key of the offer in data.commerce.offers that sells it; its price holds the listing's price, and its link must match sell. Example: `"planner"`. Checked by `printable.config`. |
| `deliverables.printable.language` | string | the page's language | The printable's language, when it differs from the page's. Example: `"de"`. |
| `deliverables.printable.version` | string |  | The version printed in the guide, so buyers see they have the latest. Example: `"2027.1"`. |
| `deliverables.spreadsheet` | object |  | A spreadsheet template in deliverables/spreadsheet (budget, bookkeeping, tracker), built with `lfm skills spreadsheet`. |
| `deliverables.spreadsheet.sell` | page | shops | bundle | page | How buyers get the files: page = your checkout redirects to an unlisted download page (needs path); shops = Etsy, Gumroad, Amazon and others deliver, the page's offer links to the listing; bundle = only inside deliverables.bundle. Example: `"shops"`. |
| `deliverables.spreadsheet.path` | string |  | The unlisted download folder, at least 16 lowercase letters, digits or hyphens; needed when sell is page. It is the only lock on the files; a compile without it prints a fresh one. Example: `"s7m2x9q4w8e3r6t5y1u0"`. Checked by `spreadsheet.config`. |
| `deliverables.spreadsheet.offer` | string |  | The key of the offer in data.commerce.offers that sells it; its price holds the listing's price, and its link must match sell. Example: `"budget"`. Checked by `spreadsheet.config`. |
| `deliverables.spreadsheet.language` | string | the page's language | The language of the start guide and download page, when it differs from the page's. Example: `"de"`. |
| `deliverables.spreadsheet.version` | string |  | The version printed in the start guide. Example: `"2027.1"`. |
| `deliverables.spreadsheet.sheets` | map |  | The Google Sheets master of every edition (and of its example as <key>-example): upload the .xlsx to Google Drive, open it with Google Sheets, share it as anyone with the link can view, paste its address here. Buyers get a copy link to it. Example: `{"en": "https://docs.google.com/spreadsheets/d/…/edit"}`. Checked by `spreadsheet.sheets`. |
| `deliverables.spreadsheet.sheets.*` | string |  | The address of one Google Sheets master. Example: `"https://docs.google.com/spreadsheets/d/1AbC…/edit"`. |
| `deliverables.document` | object |  | A set of editable documents in deliverables/document (letters, invoices, minutes, programs, binders), built with `lfm skills document`. |
| `deliverables.document.sell` | page | shops | bundle | page | How buyers get the files: page = your checkout redirects to an unlisted download page (needs path); shops = Etsy, Gumroad, Amazon and others deliver, the page's offer links to the listing; bundle = only inside deliverables.bundle. Example: `"shops"`. |
| `deliverables.document.path` | string |  | The unlisted download folder, at least 16 lowercase letters, digits or hyphens; needed when sell is page. It is the only lock on the files; a compile without it prints a fresh one. Example: `"k3n8q2w7x5r9t4m6p1"`. Checked by `document.config`. |
| `deliverables.document.offer` | string |  | The key of the offer in data.commerce.offers that sells it; its price holds the listing's price, and its link must match sell. Example: `"kit"`. Checked by `document.config`. |
| `deliverables.document.language` | string | the page's language | The language of the download page and the shop facts, when it differs from the page's. Example: `"de"`. |
| `deliverables.document.version` | string |  | The version printed in the start guide. Example: `"2027.1"`. |
| `deliverables.document.papers` | list of a4 | letter | US Letter and A4 for American English, A4 for every other language | The paper sizes every language is built in; each size is its own edition and ZIP. Example: `["a4", "letter"]`. |
| `deliverables.document.docs` | map |  | Optional Google Docs masters by "<language>/<document>": upload the .docx to Google Drive, open it with Google Docs, share it as anyone with the link can view, paste its address. The start guide and download page then carry a copy link. Example: `{"en/invoice": "https://docs.google.com/document/d/…/edit"}`. Checked by `document.config`. |
| `deliverables.document.docs.*` | string |  | The address of one Google Docs master. Example: `"https://docs.google.com/document/d/1AbC…/edit"`. |
| `deliverables.deck` | object |  | Slide decks in deliverables/deck (training briefings, workshops, quiz games) with speaker notes, built with `lfm skills deck`. |
| `deliverables.deck.sell` | page | shops | bundle | page | How buyers get the files: page = your checkout redirects to an unlisted download page (needs path); shops = Etsy, Gumroad, Amazon and others deliver, the page's offer links to the listing; bundle = only inside deliverables.bundle. Example: `"shops"`. |
| `deliverables.deck.path` | string |  | The unlisted download folder, at least 16 lowercase letters, digits or hyphens; needed when sell is page. It is the only lock on the files; a compile without it prints a fresh one. Example: `"k3n8q2w7x5r9t4m6p1"`. Checked by `deck.config`. |
| `deliverables.deck.offer` | string |  | The key of the offer in data.commerce.offers that sells it; its price holds the listing's price, and its link must match sell. Example: `"briefing"`. Checked by `deck.config`. |
| `deliverables.deck.language` | string | the page's language | The language of the download page and the shop facts, when it differs from the page's. Example: `"de"`. |
| `deliverables.deck.version` | string |  | The version printed in the start guide. Example: `"2027.1"`. |
| `deliverables.deck.papers` | list of a4 | letter | US Letter and A4 for American English, A4 for every other language | The paper sizes the speaker notes and quiz sheets print on; every language ships one ZIP with each. Example: `["a4"]`. |
| `deliverables.deck.slides` | map |  | Optional Google Slides masters by "<language>/<deck>": upload the .pptx to Google Drive, open it with Google Slides, share it as anyone with the link can view, paste its address. The start guide and download page then carry a copy link. Example: `{"en/briefing": "https://docs.google.com/presentation/d/…/edit"}`. Checked by `deck.config`. |
| `deliverables.deck.slides.*` | string |  | The address of one Google Slides master. Example: `"https://docs.google.com/presentation/d/1AbC…/edit"`. |
| `deliverables.bundle` | object |  | Two or more deliverables sold together, such as a deck with its handout documents or a book with its printable: its own name, listing in deliverables/bundle/listing.md, and download page or shop listing with every file of each. |
| `deliverables.bundle.units` | list of book | printable | spreadsheet | document | deck | every deliverable of the project | The deliverables the bundle holds, in the order its download page and listing show them. A deliverable whose sell is bundle sells only here. Example: `["deck", "document"]`. Checked by `bundle.config`. |
| `deliverables.bundle.name` | string | required | The bundle's name as buyers see it. Example: `"Starter Kit: book and workbook"`. |
| `deliverables.bundle.sell` | page | shops | page | How buyers get the bundle: page = your checkout redirects to an unlisted download page with every file; shops = the shops deliver. Example: `"page"`. |
| `deliverables.bundle.path` | string |  | The unlisted download folder, at least 16 lowercase letters, digits or hyphens; needed when sell is page. It is the only lock on the files; a compile without it prints a fresh one. Example: `"b7m2x9q4w8e3r6t5y1u0"`. Checked by `bundle.config`. |
| `deliverables.bundle.offer` | string |  | The key of the offer in data.commerce.offers that sells it; its price holds the listing's price, and its link must match sell. Example: `"kit"`. Checked by `bundle.config`. |
| `deliverables.bundle.language` | string | the page's language | The bundle's language, when it differs from the page's. Example: `"de"`. |

## Moved keys

An lfm.json of an earlier version is told where these went:

- `deliverables.book.imprint` is now `seller.imprint`
- `deliverables.book.publisher` is now `seller.name`
- `deliverables.printable.shop` is now `seller.name`
