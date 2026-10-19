package compile

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/bundle"
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
func deliverDeck(p project.Project, r *deck.Report, page []byte, isRelease bool, result *Result) (shipped, []diagnostic.Diagnostic) {
	out := shipped{Data: map[string][]byte{}}
	pageSale := r.Summary.Sell == deliver.SellPage && deliver.ValidPath(r.Config.Path) && r.Summary.URL != ""
	if r.Summary.Sell == deliver.SellPage && !pageSale {
		return out, nil
	}
	if pageSale {
		if leak := leaks(page, r.Config.Path, deck.RuleDelivery, "deck"); leak != nil {
			return out, leak
		}
	}
	var issues []diagnostic.Diagnostic
	warn := func(file, rule, message, hint string) {
		issues = append(issues, diagnostic.Diagnostic{Rule: rule, Severity: diagnostic.Warning, Location: diagnostic.Location{File: file, Line: 1, Column: 1}, Message: message, Hint: hint})
	}
	root := "deck"
	if pageSale {
		root = r.Config.Path
	}
	if result.Previews == nil {
		result.Previews = map[string][]byte{}
	}
	if !r.Buildable {
		if pageSale {
			downloadPage(result, r.Config.Path, deck.DownloadPage(*r, nil), out)
		}
		return out, issues
	}

	var pr *press
	previews := make([][]byte, len(r.Editions))
	var prep sync.WaitGroup
	prep.Go(func() { pr = newPress(p, isRelease) })
	for i, e := range r.Editions {
		prep.Go(func() { previews[i] = r.Preview(e) })
	}
	prep.Wait()
	defer pr.close()
	for i, e := range r.Editions {
		result.Previews[root+"/preview/"+e.Key+".html"] = previews[i]
	}
	for name, data := range r.PreviewAssets() {
		result.Previews[root+"/preview/"+name] = data
	}

	// An unchanged set reuses its complete ZIPs as they are.
	jobs := r.Jobs()
	guideDocs := make([][]byte, len(r.Editions))
	guideKeys := make([]string, len(r.Editions))
	fonts := r.GuideAssets()
	fontKey := r.Digest()
	for i, e := range r.Editions {
		guideDocs[i] = r.Guide(e)
		guideKeys[i] = printKey("deck guide "+e.Key, append(guideDocs[i], fontKey...), nil)
	}
	zipKeys := make([]string, len(r.Editions))
	reuse := !isRelease
	for i, e := range r.Editions {
		keys := []string{guideKeys[i]}
		for _, b := range jobs {
			if b.Edition.Key == e.Key {
				keys = append(keys, b.Key)
			}
		}
		zipKeys[i] = printKey("deck zip "+deck.PrintFormat+" "+e.Key, []byte(strings.Join(keys, ",")), nil)
		reuse = reuse && pr.has(zipKeys[i])
	}
	if reuse {
		var images []deck.Image
		if pictured(r.Listing.Present, r.Summary.Sell) {
			images = deckImages(pr, r, jobs, nil)
		}
		return finishDeck(r, result, out, issues, pageSale, func(i int) []byte { return pr.lookup(zipKeys[i]) }, images, pr.pending)
	}

	built, err := r.Build(func(key string) []byte { return pr.lookup(key) })
	if err != nil {
		return out, append(issues, diagnostic.Diagnostic{Rule: deck.RuleSource, Severity: diagnostic.Error, Location: diagnostic.Location{File: deck.Dir, Line: 1, Column: 1}, Message: "The decks could not be written: " + err.Error(), Hint: "Fix what the error names, then compile again."})
	}

	// Every PDF is Chrome's print of the deck's own layout: stamped and
	// reused while the deck and the layout are unchanged, held to the plan
	// when printed.
	pdfs := map[string][]byte{}
	var mu sync.Mutex
	var found []deck.Finding
	var printing sync.WaitGroup
	for _, b := range built {
		for _, pp := range b.Prints() {
			printing.Go(func() {
				key := printKey("deck print "+deck.PrintFormat+" "+pp.Kind+" "+pp.Paper, []byte(b.Key), nil)
				fresh := !pr.has(key)
				pdf, err := pr.job(key, func(printer *release.Printer) ([]byte, error) {
					return printer.Print(r.PrintDoc(b, pp), fonts, false)
				})
				var over []string
				if err == nil && pdf != nil && fresh && pp.Kind == "slides" {
					if printer, perr := pr.printerFor(); perr == nil {
						w, h := deck.ShotPixels()
						over, _ = printer.Overflows(r.PrintDoc(b, pp), fonts, w, h)
					}
				}
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err != nil:
					warn(b.Deck.File, deck.RuleLayout, "The PDF of "+b.Name+" could not be printed: "+err.Error(), "Install Chrome or Chromium, or set LFM_CHROME; then run 'lfm compile --release'.")
				case pdf != nil:
					pdfs[b.PrintKey(pp)] = pdf
					if fresh {
						found = append(found, r.Inspect(b, pp, pdf)...)
					}
				}
				for _, n := range over {
					found = append(found, deck.Finding{File: b.Deck.File, Message: fmt.Sprintf("%s (%s): slide %s holds more than its box shows; the print cuts it off as PowerPoint would spill it.", b.Deck.Title, b.Edition.Key, n)})
				}
			})
		}
	}
	printing.Wait()
	if isRelease {
		for _, f := range found {
			warn(f.File, deck.RuleLayout, f.Message, "Cut the slide's text, move detail into the notes, or split the slide; check the theme's fonts draw every character.")
		}
		checked := []string{fmt.Sprintf("%s print as planned", term.Plural(len(built)-len(found), "deck"))}
		if len(found) > 0 {
			checked = []string{term.Plural(len(found), "layout finding")}
		}
		// LibreOffice, when present, opens the real files too; a release
		// needs none of it.
		if soffice, err := office.Find(); err == nil {
			cross, err := r.Verify(soffice, built)
			switch {
			case err != nil:
				warn(deck.Dir, deck.RuleLayout, "LibreOffice failed: "+err.Error(), "Run 'lfm compile --release' again; reinstall LibreOffice if it keeps failing.")
			case len(cross) == 0:
				checked = append(checked, "LibreOffice agrees")
			default:
				for _, f := range cross {
					warn(f.File, deck.RuleLayout, f.Message, "LibreOffice shows the deck differently; check the slide's text and fonts.")
				}
				checked = append(checked, term.Plural(len(cross), "finding")+" in LibreOffice")
			}
		}
		// LFM_OFFICE, a switch for lfm's own tests, has PowerPoint on a Mac
		// open every deck too; it opens windows, so no user gets it.
		if os.Getenv("LFM_OFFICE") != "" && office.HasPowerPoint() {
			cross, err := r.PowerPoint(built)
			switch {
			case err != nil:
				warn(deck.Dir, deck.RuleLayout, "PowerPoint did not check the decks: "+err.Error(), "Close PowerPoint's open dialogs and run the release again.")
			case len(cross) == 0:
				checked = append(checked, "PowerPoint agrees")
			default:
				for _, f := range cross {
					warn(f.File, deck.RuleLayout, f.Message, "PowerPoint refuses or shows differently what the print shows; fix the deck, then release again.")
				}
				checked = append(checked, term.Plural(len(cross), "finding")+" in PowerPoint")
			}
		}
		r.Summary.Layout = strings.Join(checked, " · ")
	}

	// The listing images show the real slides, notes and quiz sheet.
	var shots []deck.Shot
	if isRelease && pictured(r.Listing.Present, r.Summary.Sell) {
		if printer, err := pr.printerFor(); err == nil {
			w, h := deck.ShotPixels()
			for _, b := range built {
				if b.Edition.Key != r.Showcase().Key {
					continue
				}
				cover, err := printer.Picture(r.SlideShot(b, 1), fonts, w, h, 2)
				if err != nil {
					continue
				}
				shot := deck.Shot{Key: b.Key, Title: b.Deck.Title, Cover: cover, Facts: deck.Facts(b)}
				for n := 2; n <= min(7, len(b.Deck.Slides)); n++ {
					if png, err := printer.Picture(r.SlideShot(b, n), fonts, w, h, 1); err == nil {
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
		}
	}

	guides := make([][]byte, len(r.Editions))
	var images []deck.Image
	var wg sync.WaitGroup
	for i, e := range r.Editions {
		wg.Go(func() {
			pdf, err := pr.job(guideKeys[i], func(printer *release.Printer) ([]byte, error) {
				return printer.Print(guideDocs[i], fonts, false)
			})
			if err != nil {
				warn(deck.Dir, deck.RuleChannels, "The start guide "+r.GuideName(e)+" could not be printed: "+err.Error(), "Install Chrome or Chromium, or set LFM_CHROME; then run 'lfm compile --release'.")
			}
			guides[i] = pdf
		})
	}
	if pictured(r.Listing.Present, r.Summary.Sell) {
		wg.Go(func() { images = deckImages(pr, r, built, shots) })
	}
	wg.Wait()

	zips := make([][]byte, len(r.Editions))
	for i, e := range r.Editions {
		// A complete ZIP, every PDF and the guide in it, carries the key of
		// what it holds and is reused while that is unchanged.
		key := zipKeys[i]
		if guides[i] == nil {
			key = ""
		}
		for _, b := range built {
			for _, pp := range b.Prints() {
				if b.Edition.Key == e.Key && pdfs[b.PrintKey(pp)] == nil {
					key = ""
				}
			}
		}
		if key != "" {
			zips[i] = pr.lookup(key)
		}
		if zips[i] == nil {
			zips[i], err = r.Zip(e, built, pdfs, guides[i], key)
			if err != nil {
				warn(deck.Dir, deck.RuleChannels, "The ZIP of "+e.Key+" could not be written: "+err.Error(), "Fix what the error names, then compile again.")
			}
		}
	}
	return finishDeck(r, result, out, issues, pageSale, func(i int) []byte { return zips[i] }, images, pr.pending)
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
	for _, img := range images {
		out.Images = append(out.Images, bundle.Image{Name: img.Name, Data: img.Data})
	}
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

// deckImages are the listing images, keyed by the decks they show: reused
// while those are unchanged, rendered by a release from the real slides.
func deckImages(pr *press, r *deck.Report, built []deck.Built, shots []deck.Shot) []deck.Image {
	var keys []string
	var placeholder []deck.Shot // the plan with blank shots names and keys the images
	for _, b := range built {
		if b.Edition.Key == r.Showcase().Key {
			keys = append(keys, b.Key)
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
	var out []deck.Image
	all := true
	for _, plan := range names {
		key := printKey("deck image "+plan.Name, basis, nil)
		if jpg := pr.lookup(key); jpg != nil {
			out = append(out, deck.Image{Name: plan.Name, What: plan.What, Data: jpg})
		} else {
			all = false
		}
	}
	if all || len(shots) == 0 {
		if !all {
			pr.mu.Lock()
			pr.pending = true
			pr.mu.Unlock()
		}
		return out
	}
	printer, err := pr.printerFor()
	if err != nil {
		return out
	}
	out = nil
	for _, plan := range r.Images(shots) {
		png, err := printer.Picture(plan.Doc, r.GuideAssets(), 1500, 1125, 2)
		if err != nil {
			continue
		}
		jpg, err := jpegUnder(png, 1<<20, printKey("deck image "+plan.Name, basis, nil))
		if err != nil {
			continue
		}
		out = append(out, deck.Image{Name: plan.Name, What: plan.What, Data: jpg})
	}
	return out
}
