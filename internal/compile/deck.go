package compile

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/buildid"
	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/deck"
	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
	"github.com/Anonyfox/lfm/internal/office"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/term"
)

// checkDeck checks deliverables/deck against lfm.json and the offers.
func checkDeck(p project.Project, loaded project.Loaded, commerce commercecheck.Facts) (*deck.Report, []diagnostic.Diagnostic) {
	c := loaded.Config
	var cfg deck.Config
	configured := c.Deliverables.Deck != nil
	if configured {
		d := c.Deliverables.Deck
		cfg = deck.Config{Path: d.Path, Offer: d.Offer, Language: d.Language, Sell: d.Sell, Version: d.Version, Papers: d.Papers, Slides: d.Slides}
	}
	cfg.Seller = c.Seller.Name
	if cfg.Language == "" {
		cfg.Language = c.Language
	}
	r := deck.Check(p, cfg, configured, offerPrices(commerce), c.URL)
	var sources *jsonsrc.Sources
	for i, d := range r.Diagnostics {
		if d.Location.File == "lfm.json" {
			if sources == nil {
				index := jsonsrc.Index(loaded.Sources)
				sources = &index
			}
			if loc, ok := sources.Nearest("deliverables.deck"); ok {
				r.Diagnostics[i].Location = loc
			}
		}
	}
	return &r, r.Diagnostics
}

// deliverDeck builds the .pptx files on every compile, or reuses them while
// their inputs are unchanged. A release prints each deck's slides in Chrome
// as PowerPoint lays them out, its speaker notes and quiz sheet on paper,
// holds every slide's boxes to what they show, renders the listing images
// and prints the start guides; later compiles reuse those. Each language
// ships as one ZIP.
func deliverDeck(p project.Project, r *deck.Report, page []byte, st *stock, result *Result) (shipped, []diagnostic.Diagnostic) {
	out := shipped{Data: map[string][]byte{}}
	s, stop := startSale(r.Summary.Sell, r.Config.Path, r.Summary.URL, page, deck.RuleDelivery, "deck")
	if s.skip || stop != nil {
		return out, stop
	}
	w := &warnings{}
	if !r.Buildable {
		if s.page {
			downloadPage(result, r.Config.Path, deck.DownloadPage(*r, nil), out)
		}
		return out, nil
	}
	keys := make([]string, len(r.Editions))
	for i, e := range r.Editions {
		keys[i] = e.Key
	}
	pr := s.open(st, result, keys, func(i int) []byte { return r.Preview(r.Editions[i]) }, r.PreviewAssets())

	// Reuse complete ZIPs and keep print keys independent of the writer.
	jobs := r.Jobs()
	fonts, fontKey := r.GuideAssets(), r.Digest()
	guidesPlan := make([]printJob, len(r.Editions))
	guideKeys := make([]string, len(r.Editions))
	for i, e := range r.Editions {
		doc := r.Guide(e)
		guideKeys[i] = printKey("deck guide "+e.Key, append(append([]byte{}, doc...), fontKey...), nil)
		guidesPlan[i] = printJob{key: guideKeys[i], slot: e.Key, file: deck.Dir, name: "start guide " + r.GuideName(e), doc: func() []byte { return doc }}
	}
	printKeys := map[string]string{}
	for _, b := range jobs {
		for _, pp := range b.Prints() {
			printKeys[b.PrintKey(pp)] = printKey("deck print "+pp.Kind+" "+pp.Paper, append(r.PrintDoc(b, pp), fontKey...), nil)
		}
	}
	zipKeys := make([]string, len(r.Editions))
	for i, e := range r.Editions {
		parts := []string{buildid.Writer(), guideKeys[i]}
		for _, b := range jobs {
			if b.Edition.Key == e.Key {
				parts = append(parts, b.Key)
				for _, pp := range b.Prints() {
					parts = append(parts, printKeys[b.PrintKey(pp)])
				}
			}
		}
		zipKeys[i] = printKey("deck zip "+e.Key, []byte(strings.Join(parts, ",")), nil)
	}
	built, err := r.Build(func(key string) []byte { return pr.lookup(key) })
	if err != nil {
		return out, []diagnostic.Diagnostic{{Rule: deck.RuleSource, Severity: diagnostic.Error, Location: diagnostic.Location{File: deck.Dir, Line: 1, Column: 1}, Message: "The decks could not be written: " + err.Error(), Hint: "Fix what the error names, then compile again."}}
	}
	var prints []printJob
	var unchecked bool // a slide check did not run
	for _, b := range built {
		for _, pp := range b.Prints() {
			prints = append(prints, printJob{key: printKeys[b.PrintKey(pp)], slot: b.PrintKey(pp), file: b.Deck.File, name: b.Name,
				doc: func() []byte { return r.PrintDoc(b, pp) },
				checks: func(pdf []byte, printer *release.Printer) []finding {
					found := deckFindings(r.Inspect(b, pp, pdf))
					if pp.Kind != "slides" || printer == nil {
						return found
					}
					// A slide's box that cuts its text off, as Chrome lays it out.
					sw, sh := deck.ShotPixels()
					over, err := printer.Overflows(r.PrintDoc(b, pp), fonts, sw, sh)
					if err != nil {
						unchecked = true
						w.add(b.Deck.File, deck.RuleLayout, fmt.Sprintf("%s (%s): the check for slides that cut their text off could not run: %v", b.Deck.Title, b.Edition.Key, err), "Install Chrome or Chromium, or set LFM_CHROME; then run 'lfm compile --release'.")
					}
					for _, n := range over {
						found = append(found, finding{b.Deck.File, fmt.Sprintf("%s (%s): slide %s holds more than its box shows; the print cuts it off as PowerPoint would spill it.", b.Deck.Title, b.Edition.Key, n)})
					}
					return found
				}})
		}
	}
	pdfs, found := printAll(pr, prints, fonts, deck.RuleLayout, w)
	if st.release {
		for _, f := range found {
			w.add(f.File, deck.RuleLayout, f.Message, "Cut the slide's text, move detail into the notes, or split the slide; check the theme's fonts draw every character.")
		}
		checked := []string{fmt.Sprintf("%s print as planned", term.Plural(len(built)-len(found), "deck"))}
		if len(found) > 0 {
			checked = []string{term.Plural(len(found), "layout finding")}
		} else if unchecked {
			checked = nil
		}
		// LibreOffice, when present, opens the real files too; a release
		// reuses remembered verdicts on unchanged files. LFM_OFFICE, a switch for lfm's own tests, has
		// PowerPoint on a Mac open every deck too; it opens windows, so no
		// user gets it.
		apps := []opinion{libreOffice(func(soffice string) ([]finding, error) {
			return remembered(soffice, "deck", builtData(built, func(b deck.Built) []byte { return b.Data }), func() ([]finding, error) {
				f, err := r.Verify(pr.ctx, soffice, built)
				return deckFindings(f), err
			})
		}, "LibreOffice shows the deck differently; check the slide's text and fonts.")}
		if os.Getenv("LFM_OFFICE") != "" && office.HasPowerPoint() {
			apps = append(apps, opinion{app: "PowerPoint", run: func() ([]finding, error) {
				f, err := r.PowerPoint(built)
				return deckFindings(f), err
			}, failHint: "Close PowerPoint's open dialogs and run the release again.", differHint: "PowerPoint refuses or shows differently what the print shows; fix the deck, then release again."})
		}
		r.Summary.Layout = strings.Join(append(checked, secondOpinions(deck.Dir, deck.RuleLayout, apps, w)...), " · ")
	}

	var images []deck.Image
	guides, _ := printAll(pr, guidesPlan, fonts, deck.RuleChannels, w)
	if pictured(r.Listing.Present, r.Summary.Sell) {
		images = deckImages(pr, r, built, printKeys)
	}

	for i, e := range r.Editions {
		complete := guides[e.Key] != nil
		for _, b := range built {
			for _, pp := range b.Prints() {
				if b.Edition.Key == e.Key && pdfs[b.PrintKey(pp)] == nil {
					complete = false
				}
			}
		}
		data, err := editionZip(pr, zipKeys[i], complete, func(key string) ([]byte, error) { return r.Zip(e, built, pdfs, guides[e.Key], key) })
		if err != nil {
			w.add(deck.Dir, deck.RuleChannels, "The ZIP of "+e.Key+" could not be written: "+err.Error(), "Fix what the error names, then compile again.")
		}
		if data != nil {
			out.add(deliver.File{Name: r.ZipName(e), Label: strings.ToUpper(e.Lang), Note: r.FileNote(e)}, data)
		}
	}
	r.Summary.Pending = pr.pending
	r.Summary.Images = len(images)
	out.Images = images
	r.Summary.Files = allFiles(out)
	if ownStores(r.Listing.Present, r.Summary.Sell) {
		dist, channels, found := deck.Distribution(*r, out.Files, out.Data, images, pr.pending, time.Now().Format("2006-01-02"))
		result.distribute("deck", dist)
		r.Summary.Channels = channels
		w.out = append(w.out, found...)
	} else {
		result.distribute("deck", map[string][]byte{})
	}
	if s.page {
		downloadPage(result, r.Config.Path, deck.DownloadPage(*r, out.Files), out)
	}
	return out, w.out
}

func deckPlaceholder(r *deck.Report, b deck.Built) deck.Shot {
	s := deck.Shot{Key: b.Key, Title: b.Deck.Title, Cover: []byte{}, Facts: deck.Facts(b), Slides: make([][]byte, min(6, max(0, len(b.Deck.Slides)-1))), Notes: []byte{}}
	if r.QuizShot(b) != nil {
		s.Quiz = []byte{}
	}
	return s
}

func deckFindings(in []deck.Finding) []finding {
	out := make([]finding, len(in))
	for i, f := range in {
		out[i] = finding{f.File, f.Message}
	}
	return out
}

// deckPlans retains the print-based keys and the listing layout order.
func deckPlans(r *deck.Report, built []deck.Built, printKeys map[string]string) []listingPlan {
	var keys []string
	var placeholder []deck.Shot
	for _, b := range built {
		if b.Edition.Key == r.Showcase().Key {
			for _, pp := range b.Prints() {
				keys = append(keys, printKeys[b.PrintKey(pp)])
			}
			s := deckPlaceholder(r, b)
			placeholder = append(placeholder, s)
		}
	}
	basis := []byte(strings.Join(keys, ",") + "|" + strings.Join(r.Facts(), "|"))
	names := r.Images(placeholder)
	for _, plan := range names {
		basis = append(basis, plan.Doc...)
	}
	planned := make([]listingPlan, len(names))
	for i, plan := range names {
		planned[i] = listingPlan{Name: plan.Name, Doc: plan.Doc, Key: printKey("deck image "+plan.Name, basis, nil)}
	}
	return planned
}

func deckImages(pr *press, r *deck.Report, built []deck.Built, printKeys map[string]string) []deck.Image {
	planned := deckPlans(r, built, printKeys)
	return listingImages(pr, "deck", r.Summary.Sell, planned, func(printer *release.Printer, missing []listingPlan) ([]listingPlan, map[string][]byte, error) {
		var refs []deck.Shot
		var shots []picturePlan
		sw, sh := deck.ShotPixels()
		for _, b := range built {
			if b.Edition.Key != r.Showcase().Key {
				continue
			}
			shot := deckPlaceholder(r, b)
			add := func(name string, doc []byte, w, h int64, scale float64) []byte {
				marker := []byte(b.Key + " " + name)
				shots = append(shots, picturePlan{marker, doc, w, h, scale})
				return marker
			}
			shot.Cover = add("cover", r.SlideShot(b, 1), sw, sh, 2)
			for i := range shot.Slides {
				shot.Slides[i] = add(fmt.Sprintf("slide %d", i+2), r.SlideShot(b, i+2), sw, sh, 1)
			}
			pw, ph := deck.PaperPixels(b.Edition.Papers[0])
			shot.Notes = add("notes", r.NotesShot(b, min(2, len(b.Deck.Slides))), pw, ph, 1)
			if shot.Quiz != nil {
				shot.Quiz = add("quiz", r.QuizShot(b), pw, ph, 1)
			}
			refs = append(refs, shot)
		}
		var plans []listingPlan
		for _, p := range r.Images(refs) {
			plans = append(plans, listingPlan{Name: p.Name, Doc: p.Doc})
		}
		assets := r.GuideAssets()
		plans, err := captureShots(printer, plans, missing, assets, shots)
		return plans, assets, err
	})
}
