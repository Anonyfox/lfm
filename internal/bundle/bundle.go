// Package bundle sells the project's book and printable together: one name,
// one listing, one download page or shop listing with every file of both.
package bundle

import (
	"fmt"
	"html"
	"strings"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
)

// The bundle's files under deliverables/.
const (
	Dir         = "deliverables/bundle"
	ListingFile = Dir + "/listing.md"
)

// Rule IDs.
const (
	RuleConfig   = "bundle.config"
	RuleDelivery = "bundle.delivery"
	RuleListing  = "bundle.listing"
	RuleChannels = "bundle.channels"
)

// Rules describes every bundle rule for the catalog.
func Rules() []diagnostic.RuleInfo {
	return []diagnostic.RuleInfo{
		{ID: RuleConfig, Required: true, Summary: "A bundle names both deliverables and how it sells",
			Help: "Errors when deliverables.bundle exists without both a book and a printable in the project, when it has no name, when its sell is not page or shops, when it sells through the page without a download path of at least 16 lowercase letters, digits and hyphens, when its offer is not in data.commerce.offers, or when a deliverable sells only in the bundle (sell: bundle) but no bundle is configured. A bundle buyer gets every file of both; a misconfigured one sells nothing."},
		{ID: RuleDelivery, Required: true, Summary: "The bundle's download path stays unlisted",
			Help: "Errors when the compiled page contains the bundle's download path. Buyers reach it through the checkout's after-payment redirect and the invoice email only."},
		{ID: RuleListing, Summary: "deliverables/bundle/listing.md holds shop texts every channel accepts",
			Help: "Warns where a shop would refuse the bundle's listing: a title over 140 characters, more than 13 tags or one over 20 characters, a description under 200 characters or with links, a price that differs from the page's offer, or an AI section without the use, tools and statement. lfm renders it into every shop's guide with the facts of both deliverables."},
		{ID: RuleChannels, Summary: "The bundle's shop packages meet each shop's limits",
			Help: "Warns when a shop would refuse what lfm built for the bundle: a file over Etsy's 20 MB or more files than Etsy's five once zipped. A refused upload costs the sales the bundle was made for."},
	}
}

// Config is deliverables.bundle in lfm.json.
type Config struct {
	Name, Path, Offer, Sell, Language string
}

// Summary is what the report and --json show about the bundle.
type Summary struct {
	Name     string   `json:"name"`
	Sell     string   `json:"sell"`
	URL      string   `json:"url,omitempty"`
	Files    []string `json:"files,omitempty"`
	Ready    bool     `json:"ready"` // both deliverables have their files
	Listing  bool     `json:"listing"`
	Channels []string `json:"channels,omitempty"`
	Images   int      `json:"images,omitempty"`
	Pending  bool     `json:"pending,omitempty"`
}

// Report is the checked bundle.
type Report struct {
	Config      Config
	Summary     Summary
	Diagnostics []diagnostic.Diagnostic
	Listing     deliver.ShopListing
}

// Units says which deliverables the project has and how each sells.
type Units struct {
	Book, Printable         bool
	BookSell, PrintableSell string
}

// Check reads the bundle: configured tells whether lfm.json has
// deliverables.bundle.
func Check(p project.Project, cfg Config, configured bool, u Units, offers map[string]*deliver.Price, site string) Report {
	r := Report{Config: cfg}
	fail := func(message, hint string) {
		r.Diagnostics = append(r.Diagnostics, diagnostic.Diagnostic{Rule: RuleConfig, Severity: diagnostic.Error, Location: diagnostic.Location{File: "lfm.json", Line: 1, Column: 1}, Message: message, Hint: hint})
	}
	if !configured {
		for _, x := range []struct {
			name string
			on   bool
			sell string
		}{{"book", u.Book, u.BookSell}, {"printable", u.Printable, u.PrintableSell}} {
			if x.on && deliver.Sell(x.sell) == deliver.SellBundle {
				fail(fmt.Sprintf("deliverables.%s.sell is bundle, but lfm.json has no deliverables.bundle.", x.name), `Add "bundle": {"name": "…", "path": "…"} to deliverables, or sell the deliverable on its own (page or shops). 'lfm help config deliverables.bundle' lists the options.`)
			}
		}
		return r
	}
	r.Summary.Name = cfg.Name
	r.Summary.Sell = deliver.Sell(cfg.Sell)
	ok := true
	if !u.Book || !u.Printable {
		ok = false
		fail("deliverables.bundle needs both deliverables/book and deliverables/printable.", "Add the missing deliverable, or remove the bundle.")
	}
	if r.Summary.Sell == deliver.SellPage && !deliver.ValidPath(cfg.Path) {
		ok = false
		fail(fmt.Sprintf("deliverables.bundle.path %q is not an unguessable folder name.", cfg.Path),
			fmt.Sprintf("Use at least %d lowercase letters, digits or single hyphens, for example %q. Anyone who knows the path can download the bundle.", deliver.MinPathLength, deliver.NewPath()))
	}
	var offer *deliver.Price
	if cfg.Offer != "" {
		o, found := offers[cfg.Offer]
		if !found {
			ok = false
			fail(fmt.Sprintf("deliverables.bundle.offer %q names no offer in data.commerce.offers.", cfg.Offer), "Use the key of the offer that sells the bundle.")
		}
		offer = o
	}
	if ok && r.Summary.Sell == deliver.SellPage {
		r.Summary.URL = strings.TrimSuffix(site, "/") + "/" + cfg.Path + "/"
	}
	if raw, err := p.Read(ListingFile); err == nil {
		r.Listing = deliver.CheckShopListing(string(raw), offer, func(line int, message, hint string) {
			r.Diagnostics = append(r.Diagnostics, diagnostic.Diagnostic{Rule: RuleListing, Severity: diagnostic.Warning, Location: diagnostic.Location{File: ListingFile, Line: max(1, line), Column: 1}, Message: message, Hint: hint})
		})
		r.Summary.Listing = true
	}
	return r
}

// DownloadPage renders the bundle's unlisted download page with every file
// of both deliverables, or a note while they are not all built.
func DownloadPage(r Report, accent string, files []deliver.File) []byte {
	w := deliver.Words(r.Config.Language)
	return deliver.Page(deliver.Download{Language: r.Config.Language, Title: r.Config.Name, Accent: accent,
		Heading: fmt.Sprintf(w.Heading, r.Config.Name), Files: files, Thanks: w.Thanks, Pending: w.Pending, Keep: w.Keep})
}

// HeroDoc is the bundle's first listing image: the book's cover and the
// printable's cover side by side, 3000×2250 pixels at twice the density.
func HeroDoc(bookCover, printableCover, accent string, printableRatio float64) []byte {
	if accent == "" {
		accent = "#444444"
	}
	return []byte(fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:1500px;height:1125px;overflow:hidden}
body{display:flex;align-items:center;justify-content:center;gap:48px;background:radial-gradient(circle at 50%% 40%%,#fff 0,color-mix(in srgb,%s 10%%,#f4f2ee) 75%%)}
img{display:block;box-shadow:0 2px 6px rgba(0,0,0,.18),0 18px 40px rgba(0,0,0,.16);background:#fff}
</style></head><body><img src="%s" style="height:860px;width:auto" alt=""><img src="%s" style="height:800px;width:%gpx" alt=""></body></html>`,
		accent, html.EscapeString(bookCover), html.EscapeString(printableCover), 800*printableRatio))
}

// Image is one listing image of the bundle.
type Image struct {
	Name string
	Data []byte
}

// Distribution assembles the bundle's shop packages from the files of both
// deliverables: groups names the ZIPs Etsy gets when the files are more than
// five; facts describe what each deliverable holds.
func Distribution(r Report, files []deliver.File, data map[string][]byte, groups map[string][]string, images []Image, facts []string, pending bool, day string) (map[string][]byte, []string, []diagnostic.Diagnostic) {
	out := map[string][]byte{}
	pkg := deliver.ShopPackage{Name: r.Config.Name, Source: Dir, Listing: r.Listing, Groups: groups, Facts: facts, Pending: pending}
	for _, f := range files {
		out[f.Name] = data[f.Name]
		pkg.Files = append(pkg.Files, f.Name)
	}
	for _, img := range images {
		out["images/"+img.Name] = img.Data
		pkg.Images = append(pkg.Images, img.Name)
	}
	channels, found := deliver.Package(pkg, out, day)
	var issues []diagnostic.Diagnostic
	for _, f := range found {
		issues = append(issues, diagnostic.Diagnostic{Rule: RuleChannels, Severity: diagnostic.Warning, Location: diagnostic.Location{File: ListingFile, Line: 1, Column: 1}, Message: f.Message, Hint: f.Hint})
	}
	return out, channels, issues
}
