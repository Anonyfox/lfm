package compile

import (
	"bytes"
	"fmt"
	"image"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/bundle"
	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
	"github.com/Anonyfox/lfm/internal/printable"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
)

// checkBundle checks deliverables.bundle against the deliverables it holds.
func checkBundle(p project.Project, loaded project.Loaded, commerce commercecheck.Facts) (*bundle.Report, []diagnostic.Diagnostic) {
	c := loaded.Config
	var cfg bundle.Config
	configured := c.Deliverables.Bundle != nil
	if configured {
		b := c.Deliverables.Bundle
		cfg = bundle.Config{Name: b.Name, Path: b.Path, Offer: b.Offer, Sell: b.Sell, Language: b.Language, Units: b.Units}
	}
	if cfg.Language == "" {
		cfg.Language = c.Language
	}
	d := c.Deliverables
	units := map[string]bundle.Unit{}
	for _, k := range kinds {
		units[k.name] = bundle.Unit{Present: k.present(p)}
	}
	sell := func(kind, s string) {
		u := units[kind]
		u.Sell = s
		units[kind] = u
	}
	if d.Book != nil {
		sell("book", d.Book.Sell)
	}
	if d.Printable != nil {
		sell("printable", d.Printable.Sell)
	}
	if d.Spreadsheet != nil {
		sell("spreadsheet", d.Spreadsheet.Sell)
	}
	if d.Document != nil {
		sell("document", d.Document.Sell)
	}
	if d.Deck != nil {
		sell("deck", d.Deck.Sell)
	}
	bundled := false
	for _, u := range units {
		bundled = bundled || u.Present && deliver.Sell(u.Sell) == deliver.SellBundle
	}
	if !configured && !bundled {
		return nil, nil
	}
	r := bundle.Check(p, cfg, configured, units, offerPrices(commerce), c.URL)
	sources := jsonsrc.Index(loaded.Sources)
	for i := range r.Diagnostics {
		if r.Diagnostics[i].Location.File == "lfm.json" {
			if loc, ok := sources.Nearest("deliverables"); ok {
				r.Diagnostics[i].Location = loc
			}
		}
	}
	if !configured {
		return nil, r.Diagnostics
	}
	return &r, r.Diagnostics
}

// saleFindings warns when a unit's sell mode and the page's offer disagree:
// a unit sold in shops whose buy button goes to a checkout, or one sold
// through the page whose buy button goes to a shop.
func saleFindings(commerce commercecheck.Facts, field, sell, offer, rule string) []diagnostic.Diagnostic {
	if offer == "" {
		return nil
	}
	for _, o := range commerce.Offers {
		if strings.TrimPrefix(o.Path, "data.commerce.offers.") != offer || o.CheckoutURL == "" {
			continue
		}
		shop := deliver.Shop(o.CheckoutURL)
		host := o.CheckoutURL
		if u, err := url.Parse(o.CheckoutURL); err == nil {
			host = u.Hostname()
		}
		warn := func(message, hint string) []diagnostic.Diagnostic {
			return []diagnostic.Diagnostic{{Rule: rule, Severity: diagnostic.Warning, Location: diagnostic.Location{File: "lfm.json", Line: 1, Column: 1}, Message: message, Hint: hint}}
		}
		switch {
		case deliver.Sell(sell) == deliver.SellShops && shop == "":
			return warn(fmt.Sprintf("%s sells in shops, but its offer %q links to %s.", field, offer, host), "Link the page's buy button to the shop listing (put its address in urls), or set sell to page to sell through the checkout.")
		case deliver.Sell(sell) == deliver.SellPage && shop != "":
			return warn(fmt.Sprintf("%s sells through the page's checkout, but its offer %q links to %s.", field, offer, shop), "Set sell to shops when the shop delivers the files; otherwise point the offer at the checkout that redirects to the download page.")
		}
	}
	return nil
}

// part is one unit of a bundle as it was delivered: its files, its
// listing images, the facts that describe it, and how Etsy's five files
// group it when the bundle has more.
type part struct {
	kind, name string
	accent     string
	plans      []listingPlan
	render     func(*release.Printer) (map[string][]byte, error)
	files      shipped
	facts      []string
	groups     map[string][]string // ZIP name suffix -> files
	cover      *bundle.Cover       // the picture the hero shows, with its bytes
	coverData  []byte              // the cover's bytes, when it has them before a release renders it
	coverKey   []byte              // what a cover rendered during the release is made from
	pending    bool
}

// deliverBundle publishes the bundle's download page with every file of
// every unit when it sells through the page, and assembles its shop
// packages from them when it has a listing.
func deliverBundle(p project.Project, r *bundle.Report, parts []part, accent string, page []byte, st *stock, result *Result, heroDoc func(printer *release.Printer) (map[string][]byte, error)) []diagnostic.Diagnostic {
	pageSale := r.Summary.Sell == deliver.SellPage && r.Summary.URL != ""
	if pageSale {
		if leak := leaks(page, r.Config.Path, bundle.RuleDelivery, "bundle"); leak != nil {
			return leak
		}
	}
	all := shipped{Data: map[string][]byte{}}
	r.Summary.Ready = true
	for _, pt := range parts {
		r.Summary.Ready = r.Summary.Ready && len(pt.files.Files) > 0
		r.Summary.Pending = r.Summary.Pending || pt.pending
	}
	if r.Summary.Ready {
		for _, pt := range parts {
			for _, f := range pt.files.Files {
				all.add(f, pt.files.Data[f.Name])
				r.Summary.Files = append(r.Summary.Files, f.Name)
			}
		}
	}
	if pageSale {
		downloadPage(result, r.Config.Path, bundle.DownloadPage(*r, accent, all.Files), all)
	}
	if !r.Summary.Ready || !r.Listing.Present {
		result.distribute("bundle", map[string][]byte{})
		return nil
	}
	// Etsy takes five files: when the bundle has more, each unit's files go
	// into ZIPs of their own.
	groups := map[string][]string{}
	stem := deliver.Slug(r.Config.Name, "bundle")
	var facts []string
	for _, pt := range parts {
		for suffix, files := range pt.groups {
			groups[stem+"-"+suffix+".zip"] = append(groups[stem+"-"+suffix+".zip"], files...)
		}
		facts = append(facts, pt.name+":")
		facts = append(facts, pt.facts...)
	}
	listingImages, heroErr := bundleImages(p, r, parts, accent, st, heroDoc)
	r.Summary.Images = len(listingImages)
	dist, channels, issues := bundle.Distribution(*r, all.Files, all.Data, groups, listingImages, facts, r.Summary.Pending, time.Now().Format("2006-01-02"))
	if heroErr != nil {
		issues = append(issues, diagnostic.Diagnostic{Rule: bundle.RuleChannels, Severity: diagnostic.Warning, Location: diagnostic.Location{File: bundle.ListingFile, Line: 1, Column: 1},
			Message: "The bundle's first image, every unit's cover together, could not be rendered: " + heroErr.Error(), Hint: "Check each unit's cover or first page, then run 'lfm compile --release' again."})
	}
	result.distribute("bundle", dist)
	r.Summary.Channels = channels
	return issues
}

// bundleImages are the bundle's listing images: every unit's cover
// together first, then each unit's own images in turn. The first is
// rendered by a release and reused while the covers are unchanged.
func bundleImages(p project.Project, r *bundle.Report, parts []part, accent string, st *stock, render func(printer *release.Printer) (map[string][]byte, error)) ([]bundle.Image, error) {
	var out []bundle.Image
	var heroErr error
	var covers []bundle.Cover
	assets := map[string][]byte{}
	var made []byte
	for _, pt := range parts {
		if pt.cover != nil {
			covers = append(covers, *pt.cover)
			if pt.coverData != nil {
				assets[pt.cover.Name] = pt.coverData
			}
			made = append(made, pt.coverKey...)
		}
	}
	if len(covers) >= 2 {
		doc := bundle.HeroDoc(covers, accent)
		key := printKey("bundle hero", append(append([]byte{}, doc...), made...), assets)
		press := st.press()
		hero, err := press.job(key, func(printer *release.Printer) ([]byte, error) {
			more, err := render(printer)
			if err != nil {
				return nil, err
			}
			for name, data := range more {
				assets[name] = data
			}
			png, err := printer.Picture(doc, assets, printable.ImageWidth, printable.ImageHeight, printable.ImageScale, true)
			if err != nil {
				return nil, err
			}
			return jpegUnder(png, 1<<20, key)
		})
		switch {
		case err != nil:
			heroErr = err
		case hero != nil:
			out = append(out, bundle.Image{Name: "01-bundle.jpg", Data: hero})
		}
		if press.pending {
			r.Summary.Pending = true
		}
	}
	// Each unit's images in turn, first ones first, so every unit shows
	// before any shows twice.
	for round := 0; len(out) < deliver.EtsyImages; round++ {
		added := false
		for _, pt := range parts {
			if round < len(pt.files.Images) && len(out) < deliver.EtsyImages {
				img := pt.files.Images[round]
				_, rest, _ := strings.Cut(img.Name, "-") // renumbered after the bundle's own
				out = append(out, bundle.Image{Name: fmt.Sprintf("%02d-%s-%s", len(out)+1, pt.kind, rest), Data: img.Data})
				added = true
			}
		}
		if !added {
			break
		}
	}
	return out, heroErr
}

func allFiles(s shipped) []string {
	var names []string
	for _, f := range s.Files {
		names = append(names, f.Name)
	}
	return names
}

func (u *bookUnit) part() part {
	bk := u.r
	kind := "book"
	var formats []string
	for _, f := range u.out.Files {
		formats = append(formats, strings.ToUpper(strings.TrimPrefix(path.Ext(f.Name), ".")))
	}
	pt := part{kind: kind, name: bk.Plan.Title, files: u.out, pending: bk.Summary.Pending,
		facts:  []string{fmt.Sprintf("%s, %d chapters", strings.Join(formats, ", "), len(bk.Plan.Chapters))},
		groups: map[string][]string{"ebook": allFiles(u.out)}}
	if cover := bk.Sources[bk.Plan.Cover]; cover != nil {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(cover)); err == nil && cfg.Height > 0 {
			pt.cover = &bundle.Cover{Name: "book-cover" + path.Ext(bk.Plan.Cover), Ratio: float64(cfg.Width) / float64(cfg.Height)}
			pt.coverData = cover
		}
	}
	return pt
}

func (u *printableUnit) part() part {
	pr := u.r
	kind := "printable"
	pt := part{kind: kind, name: pr.Plan.Name, files: u.out, pending: pr.Summary.Pending, facts: printable.Facts(*pr), groups: map[string][]string{}}
	for _, ed := range pr.Editions {
		if u.out.Data[pr.FileName(ed.Edition)] != nil {
			pt.groups[ed.Edition.Size.Key] = append(pt.groups[ed.Edition.Size.Key], pr.FileName(ed.Edition))
		}
	}
	if len(pr.Editions) > 0 {
		first := pr.Editions[0]
		pt.cover = &bundle.Cover{Name: "printable-cover.png", Ratio: first.Edition.Size.Width / first.Edition.Size.Height}
		pt.coverKey = first.Doc
		// The printable's first page is drawn by the printer.
		pt.render = func(printer *release.Printer) (map[string][]byte, error) {
			raster, err := printer.PageImages(first.Doc, pr.Assets, mmPixels(first.Edition.Size.Width), mmPixels(first.Edition.Size.Height), printable.PageScale, []int{1})
			if err != nil {
				return nil, err
			}
			return map[string][]byte{"printable-cover.png": raster[0]}, nil
		}
	}
	if pictured(pr.Listing.Present, pr.Summary.Sell) {
		for _, plan := range pr.Images() {
			pt.plans = append(pt.plans, listingPlan{Name: plan.Name, Doc: plan.Doc})
		}
	}
	pt.accent = pr.Plan.Accent
	return pt
}

func listingPart(pt part, group string, pictures bool) part {
	if !pictures {
		pt.plans = nil
	}
	pt.groups = map[string][]string{group: allFiles(pt.files)}
	if len(pt.files.Images) > 0 {
		pt.cover = &bundle.Cover{Name: pt.kind + "-cover.jpg", Ratio: 4.0 / 3}
		pt.coverData = pt.files.Images[0].Data
	}
	return pt
}

func (u *spreadsheetUnit) part() part {
	g := u.r
	pt := part{kind: "spreadsheet", name: g.Plan.Name, files: u.out, pending: g.Summary.Pending, facts: g.Facts(), accent: g.Plan.Accent}
	for _, p := range g.Images() {
		pt.plans = append(pt.plans, listingPlan{Name: p.Name, Doc: p.Doc})
	}
	return listingPart(pt, "spreadsheet", pictured(g.Listing.Present, g.Summary.Sell))
}

func (u *documentUnit) part() part {
	d := u.r
	return listingPart(part{kind: "document", name: d.Plan.Name, files: u.out, pending: d.Summary.Pending, facts: d.Facts(), accent: d.Theme.Accent, plans: documentPlans(d, d.Jobs(), nil)}, "documents", pictured(d.Listing.Present, d.Summary.Sell))
}

func (u *deckUnit) part() part {
	d := u.r
	return listingPart(part{kind: "deck", name: d.Plan.Name, files: u.out, pending: d.Summary.Pending, facts: d.Facts(), accent: d.Theme.Accent, plans: deckPlans(d, d.Jobs(), nil)}, "decks", pictured(d.Listing.Present, d.Summary.Sell))
}
