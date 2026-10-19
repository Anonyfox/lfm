package compile

import (
	"fmt"
	"os"
	"strings"
	"sync"
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

	// Every print is keyed by what Chrome prints, every deck by its inputs
	// and the binary: an unchanged set reuses its complete ZIPs as they are,
	// and a newer lfm writes the decks anew but keeps the PDFs.
	jobs := r.Jobs()
	fonts, fontKey := r.GuideAssets(), r.Digest()
	guideDocs := make([][]byte, len(r.Editions))
	guideKeys := make([]string, len(r.Editions))
	guideNames := make([]string, len(r.Editions))
	for i, e := range r.Editions {
		guideDocs[i] = r.Guide(e)
		guideKeys[i] = printKey("deck guide "+e.Key, append(guideDocs[i], fontKey...), nil)
		guideNames[i] = r.GuideName(e)
	}
	printKeys := map[string]string{}
	for _, b := range jobs {
		for _, pp := range b.Prints() {
			printKeys[b.PrintKey(pp)] = printKey("deck print "+pp.Kind+" "+pp.Paper, append(r.PrintDoc(b, pp), fontKey...), nil)
		}
	}
	zipKeys := make([]string, len(r.Editions))
	reuse := !st.release
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
		reuse = reuse && pr.has(zipKeys[i])
	}
	pictures := pictured(r.Listing.Present, r.Summary.Sell)
	if reuse {
		var images []deck.Image
		if pictures {
			images = deckImages(pr, r, jobs, printKeys, nil)
		}
		return finishDeck(r, result, out, w.out, s.page, func(i int) []byte { return pr.lookup(zipKeys[i]) }, images, pr.pending)
	}

	built, err := r.Build(func(key string) []byte { return pr.lookup(key) })
	if err != nil {
		return out, []diagnostic.Diagnostic{{Rule: deck.RuleSource, Severity: diagnostic.Error, Location: diagnostic.Location{File: deck.Dir, Line: 1, Column: 1}, Message: "The decks could not be written: " + err.Error(), Hint: "Fix what the error names, then compile again."}}
	}
	var prints []printJob
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
					over, _ := printer.Overflows(r.PrintDoc(b, pp), fonts, sw, sh)
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
		}
		// LibreOffice, when present, opens the real files too; a release
		// needs none of it. LFM_OFFICE, a switch for lfm's own tests, has
		// PowerPoint on a Mac open every deck too; it opens windows, so no
		// user gets it.
		apps := []opinion{libreOffice(func(soffice string) ([]finding, error) {
			f, err := r.Verify(soffice, built)
			return deckFindings(f), err
		}, "LibreOffice shows the deck differently; check the slide's text and fonts.")}
		if os.Getenv("LFM_OFFICE") != "" && office.HasPowerPoint() {
			apps = append(apps, opinion{app: "PowerPoint", run: func() ([]finding, error) {
				f, err := r.PowerPoint(built)
				return deckFindings(f), err
			}, failHint: "Close PowerPoint's open dialogs and run the release again.", differHint: "PowerPoint refuses or shows differently what the print shows; fix the deck, then release again."})
		}
		r.Summary.Layout = strings.Join(append(checked, secondOpinions(deck.Dir, deck.RuleLayout, apps, w)...), " · ")
	}

	// The listing images show the real slides, notes and quiz sheet.
	var shots []deck.Shot
	if st.release && pictures {
		if printer, err := pr.printerFor(); err == nil {
			shots = deckShots(printer, r, built, fonts)
		}
	}
	var guides [][]byte
	var images []deck.Image
	var wg sync.WaitGroup
	wg.Go(func() {
		guides = printGuides(pr, guideKeys, guideDocs, guideNames, fonts, deck.Dir, deck.RuleChannels, w)
	})
	if pictures {
		wg.Go(func() { images = deckImages(pr, r, built, printKeys, shots) })
	}
	wg.Wait()

	zips := make([][]byte, len(r.Editions))
	for i, e := range r.Editions {
		complete := guides[i] != nil
		for _, b := range built {
			for _, pp := range b.Prints() {
				if b.Edition.Key == e.Key && pdfs[b.PrintKey(pp)] == nil {
					complete = false
				}
			}
		}
		zips[i], err = editionZip(pr, zipKeys[i], complete, func(key string) ([]byte, error) { return r.Zip(e, built, pdfs, guides[i], key) })
		if err != nil {
			w.add(deck.Dir, deck.RuleChannels, "The ZIP of "+e.Key+" could not be written: "+err.Error(), "Fix what the error names, then compile again.")
		}
	}
	return finishDeck(r, result, out, w.out, s.page, func(i int) []byte { return zips[i] }, images, pr.pending)
}

// deckShots renders what the listing images show of each deck of the
// showcase language: the cover, the slides after it, a notes page and the
// quiz sheet.
func deckShots(printer *release.Printer, r *deck.Report, built []deck.Built, fonts map[string][]byte) []deck.Shot {
	var shots []deck.Shot
	sw, sh := deck.ShotPixels()
	for _, b := range built {
		if b.Edition.Key != r.Showcase().Key {
			continue
		}
		cover, err := printer.Picture(r.SlideShot(b, 1), fonts, sw, sh, 2)
		if err != nil {
			continue
		}
		shot := deck.Shot{Key: b.Key, Title: b.Deck.Title, Cover: cover, Facts: deck.Facts(b)}
		for n := 2; n <= min(7, len(b.Deck.Slides)); n++ {
			if png, err := printer.Picture(r.SlideShot(b, n), fonts, sw, sh, 1); err == nil {
				shot.Slides = append(shot.Slides, png)
			}
		}
		pw, ph := deck.PaperPixels(b.Edition.Papers[0])
		if png, err := printer.Picture(r.NotesShot(b, min(2, len(b.Deck.Slides))), fonts, pw, ph, 1); err == nil {
			shot.Notes = png
		}
		if doc := r.QuizShot(b); doc != nil {
			if png, err := printer.Picture(doc, fonts, pw, ph, 1); err == nil {
				shot.Quiz = png
			}
		}
		shots = append(shots, shot)
	}
	return shots
}

func deckFindings(in []deck.Finding) []finding {
	out := make([]finding, len(in))
	for i, f := range in {
		out[i] = finding{f.File, f.Message}
	}
	return out
}

// finishDeck publishes the editions' ZIPs, the shop packages and the
// download page.
func finishDeck(r *deck.Report, result *Result, out shipped, issues []diagnostic.Diagnostic, pageSale bool, zip func(int) []byte, images []deck.Image, pending bool) (shipped, []diagnostic.Diagnostic) {
	for i, e := range r.Editions {
		if data := zip(i); data != nil {
			out.add(deliver.File{Name: r.ZipName(e), Label: strings.ToUpper(e.Lang), Note: r.FileNote(e)}, data)
		}
	}
	r.Summary.Pending = pending
	r.Summary.Images = len(images)
	out.Images = images
	for _, f := range out.Files {
		r.Summary.Files = append(r.Summary.Files, f.Name)
	}
	if ownStores(r.Listing.Present, r.Summary.Sell) {
		dist, channels, found := deck.Distribution(*r, out.Files, out.Data, images, pending, time.Now().Format("2006-01-02"))
		result.distribute("deck", dist)
		r.Summary.Channels = channels
		issues = append(issues, found...)
	} else {
		result.distribute("deck", map[string][]byte{})
	}
	if pageSale {
		downloadPage(result, r.Config.Path, deck.DownloadPage(*r, out.Files), out)
	}
	return out, issues
}

// deckImages are the listing images: the plan with blank shots names and
// keys them, a release renders them from the real slides. They show the
// printed slides, notes and quiz, so they key on the prints: a newer lfm
// that writes the decks anew keeps them.
func deckImages(pr *press, r *deck.Report, built []deck.Built, printKeys map[string]string, shots []deck.Shot) []deck.Image {
	var keys []string
	var placeholder []deck.Shot
	for _, b := range built {
		if b.Edition.Key == r.Showcase().Key {
			for _, pp := range b.Prints() {
				keys = append(keys, printKeys[b.PrintKey(pp)])
			}
			s := deck.Shot{Key: b.Key, Title: b.Deck.Title, Cover: []byte{}, Facts: deck.Facts(b), Slides: make([][]byte, min(6, max(0, len(b.Deck.Slides)-1))), Notes: []byte{}}
			if r.QuizShot(b) != nil {
				s.Quiz = []byte{}
			}
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
		planned[i] = listingPlan{Name: plan.Name, Key: printKey("deck image "+plan.Name, basis, nil)}
	}
	return listingImages(pr, planned, func(*release.Printer) ([]listingPlan, map[string][]byte, error) {
		if len(shots) == 0 {
			return nil, nil, nil
		}
		var plans []listingPlan
		for _, plan := range r.Images(shots) {
			plans = append(plans, listingPlan{Name: plan.Name, Doc: plan.Doc})
		}
		return plans, r.GuideAssets(), nil
	})
}
