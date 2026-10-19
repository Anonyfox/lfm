package compile

import (
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/book"
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
		cfg = bundle.Config{Name: b.Name, Path: b.Path, Offer: b.Offer, Sell: b.Sell, Language: b.Language}
	}
	if cfg.Language == "" {
		cfg.Language = c.Language
	}
	u := bundle.Units{Book: book.Present(p), Printable: printable.Present(p)}
	if b := c.Deliverables.Book; b != nil {
		u.BookSell = b.Sell
	}
	if pr := c.Deliverables.Printable; pr != nil {
		u.PrintableSell = pr.Sell
	}
	if !configured && deliver.Sell(u.BookSell) != deliver.SellBundle && deliver.Sell(u.PrintableSell) != deliver.SellBundle {
		return nil, nil
	}
	r := bundle.Check(p, cfg, configured, u, offerPrices(commerce), c.URL)
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

// deliverBundle publishes the bundle's download page with every file of
// both deliverables when it sells through the page, and assembles its shop
// packages from them when it has a listing.
func deliverBundle(p project.Project, r *bundle.Report, bk *book.Report, pr *printable.Report, books, prints shipped, images []printable.Image, page []byte, isRelease bool, result *Result) []diagnostic.Diagnostic {
	pageSale := r.Summary.Sell == deliver.SellPage && r.Summary.URL != ""
	if pageSale {
		if leak := leaks(page, r.Config.Path, bundle.RuleDelivery, "bundle"); leak != nil {
			return leak
		}
	}
	all := shipped{Data: map[string][]byte{}}
	r.Summary.Ready = len(books.Files) > 0 && len(prints.Files) > 0
	r.Summary.Pending = bk.Summary.Pending || pr.Summary.Pending
	if r.Summary.Ready {
		for _, s := range []shipped{books, prints} {
			for _, f := range s.Files {
				all.add(f, s.Data[f.Name])
				r.Summary.Files = append(r.Summary.Files, f.Name)
			}
		}
	}
	if pageSale {
		downloadPage(result, r.Config.Path, bundle.DownloadPage(*r, pr.Plan.Accent, all.Files), all)
	}
	if !r.Summary.Ready || !r.Listing.Present {
		result.distribute("bundle", map[string][]byte{})
		return nil
	}
	// Etsy takes five files: when the bundle has more, the ebook files go
	// into one ZIP and the printable's into one per paper size.
	groups := map[string][]string{}
	stem := deliver.Slug(r.Config.Name, "bundle")
	for _, f := range books.Files {
		groups[stem+"-ebook.zip"] = append(groups[stem+"-ebook.zip"], f.Name)
	}
	for _, ed := range pr.Editions {
		if prints.Data[pr.FileName(ed.Edition)] != nil {
			name := stem + "-" + ed.Edition.Size.Key + ".zip"
			groups[name] = append(groups[name], pr.FileName(ed.Edition))
		}
	}
	listingImages, heroErr := bundleImages(p, r, bk, pr, images, isRelease)
	r.Summary.Images = len(listingImages)
	var formats []string
	for _, f := range books.Files {
		formats = append(formats, strings.ToUpper(strings.TrimPrefix(path.Ext(f.Name), ".")))
	}
	facts := append([]string{fmt.Sprintf("%s: %s, %d chapters", bk.Plan.Title, strings.Join(formats, ", "), len(bk.Plan.Chapters))},
		append([]string{pr.Plan.Name + ":"}, printable.Facts(*pr)...)...)
	dist, channels, issues := bundle.Distribution(*r, all.Files, all.Data, groups, listingImages, facts, r.Summary.Pending, time.Now().Format("2006-01-02"))
	if heroErr != nil {
		issues = append(issues, diagnostic.Diagnostic{Rule: bundle.RuleChannels, Severity: diagnostic.Warning, Location: diagnostic.Location{File: bundle.ListingFile, Line: 1, Column: 1},
			Message: "The bundle's first image, both covers together, could not be rendered: " + heroErr.Error(), Hint: "Check the book's cover file and the printable's first page, then run 'lfm compile --release' again."})
	}
	result.distribute("bundle", dist)
	r.Summary.Channels = channels
	return issues
}

// bundleImages are the bundle's listing images: both covers together first,
// then the printable's images. The first is rendered by a release and reused
// while both covers are unchanged.
func bundleImages(p project.Project, r *bundle.Report, bk *book.Report, pr *printable.Report, images []printable.Image, isRelease bool) ([]bundle.Image, error) {
	var out []bundle.Image
	var heroErr error
	cover := bk.Sources[bk.Plan.Cover]
	if cover != nil && len(pr.Editions) > 0 {
		first := pr.Editions[0]
		coverName := "book-cover" + path.Ext(bk.Plan.Cover)
		doc := bundle.HeroDoc(coverName, "printable-cover.png", pr.Plan.Accent, first.Edition.Size.Width/first.Edition.Size.Height)
		key := printKey("bundle hero", append(append([]byte{}, doc...), first.Doc...), map[string][]byte{coverName: cover})
		press := newPress(p, isRelease)
		defer press.close()
		hero, err := press.job(key, func(printer *release.Printer) ([]byte, error) {
			raster, err := printer.PageImages(first.Doc, pr.Assets, mmPixels(first.Edition.Size.Width), mmPixels(first.Edition.Size.Height), printable.PageScale, []int{1})
			if err != nil {
				return nil, err
			}
			png, err := printer.Picture(doc, map[string][]byte{coverName: cover, "printable-cover.png": raster[0]}, printable.ImageWidth, printable.ImageHeight, printable.ImageScale)
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
	for _, img := range images {
		if len(out) >= deliver.EtsyImages {
			break
		}
		_, rest, _ := strings.Cut(img.Name, "-") // renumbered after the bundle's own
		out = append(out, bundle.Image{Name: fmt.Sprintf("%02d-%s", len(out)+1, rest), Data: img.Data})
	}
	return out, heroErr
}
