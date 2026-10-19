// Package bundle sells two or more of the project's deliverables together,
// such as a deck with its handouts or a book with its workbook: one name,
// one listing, one download page or shop listing with every file of each.
package bundle

import (
	"fmt"
	"html"
	"math"
	"slices"
	"strings"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/term"
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
		{ID: RuleConfig, Required: true, Summary: "A bundle holds two or more deliverables and says how it sells",
			Help: "Errors when deliverables.bundle holds fewer than two deliverables of the project (its units, or every deliverable when it names none), names a unit the project does not have, has no name, when its sell is not page or shops, when it sells through the page without a download path of at least 16 lowercase letters, digits and hyphens, when its offer is not in data.commerce.offers, or when a deliverable sells only in the bundle (sell: bundle) but no bundle is configured. A bundle buyer gets every file of each unit; a misconfigured one sells nothing."},
		{ID: RuleDelivery, Required: true, Summary: "The bundle's download path stays unlisted",
			Help: "Errors when the compiled page contains the bundle's download path. Buyers reach it through the checkout's after-payment redirect and the invoice email only."},
		{ID: RuleListing, Summary: "deliverables/bundle/listing.md holds shop texts every channel accepts",
			Help: "Warns where a shop would refuse the bundle's listing: a title over 140 characters, more than 13 tags or one over 20 characters, a description under 200 characters or with links, a price that differs from the page's offer, or an AI section without the use, tools and statement. lfm renders it into every shop's guide with the facts of every unit."},
		{ID: RuleChannels, Summary: "The bundle's shop packages meet each shop's limits",
			Help: "Warns when a shop would refuse what lfm built for the bundle: a file over Etsy's 20 MB or more files than Etsy's five once zipped. A refused upload costs the sales the bundle was made for."},
	}
}

// Config is deliverables.bundle in lfm.json.
type Config struct {
	Name, Path, Offer, Sell, Language string
	Units                             []string // the deliverables it holds; every one when empty
}

// Kinds are the deliverables a bundle can hold, in the order it lists them.
var Kinds = []string{"book", "printable", "spreadsheet", "document", "deck"}

// Summary is what the report and --json show about the bundle.
type Summary struct {
	Name     string   `json:"name"`
	Units    []string `json:"units"`
	Sell     string   `json:"sell"`
	URL      string   `json:"url,omitempty"`
	Files    []string `json:"files,omitempty"`
	Ready    bool     `json:"ready"` // every unit has its files
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

// Unit is one deliverable of the project: whether it is there and how
// it sells.
type Unit struct {
	Present bool
	Sell    string
}

// Check reads the bundle: configured tells whether lfm.json has
// deliverables.bundle; units are the project's deliverables by kind.
func Check(p project.Project, cfg Config, configured bool, units map[string]Unit, offers map[string]*deliver.Price, site string) Report {
	r := Report{Config: cfg}
	fail := func(message, hint string) {
		r.Diagnostics = append(r.Diagnostics, diagnostic.Diagnostic{Rule: RuleConfig, Severity: diagnostic.Error, Location: diagnostic.Location{File: "lfm.json", Line: 1, Column: 1}, Message: message, Hint: hint})
	}
	if !configured {
		for _, kind := range Kinds {
			if u := units[kind]; u.Present && deliver.Sell(u.Sell) == deliver.SellBundle {
				fail(fmt.Sprintf("deliverables.%s.sell is bundle, but lfm.json has no deliverables.bundle.", kind), `Add "bundle": {"name": "…", "path": "…"} to deliverables, or sell the deliverable on its own (page or shops). 'lfm help config deliverables.bundle' lists the options.`)
			}
		}
		return r
	}
	r.Summary.Name = cfg.Name
	r.Summary.Sell = deliver.Sell(cfg.Sell)
	ok := true
	if strings.TrimSpace(cfg.Name) == "" {
		ok = false
		fail("deliverables.bundle has no name.", "Name it as buyers see it, such as \"Fire Safety Kit: briefing and handouts\".")
	}
	chosen := cfg.Units
	if len(chosen) == 0 {
		for _, kind := range Kinds {
			if units[kind].Present {
				chosen = append(chosen, kind)
			}
		}
	}
	seen := map[string]bool{}
	for _, kind := range chosen {
		switch u, known := units[kind]; {
		case !known:
			ok = false
			fail(fmt.Sprintf("deliverables.bundle.units names %q.", kind), "Name deliverables lfm builds: "+strings.Join(Kinds, ", ")+".")
		case !u.Present:
			ok = false
			fail(fmt.Sprintf("deliverables.bundle.units names %s, but the project has no deliverables/%s.", kind, kind), "Add the deliverable, or take it out of the bundle's units.")
		case seen[kind]:
		default:
			seen[kind] = true
			r.Summary.Units = append(r.Summary.Units, kind)
		}
	}
	for _, kind := range Kinds {
		if u := units[kind]; u.Present && deliver.Sell(u.Sell) == deliver.SellBundle && !seen[kind] {
			ok = false
			fail(fmt.Sprintf("deliverables.%s.sell is bundle, but the bundle does not hold it.", kind), "Add it to deliverables.bundle.units, or sell it on its own (page or shops).")
		}
	}
	if len(r.Summary.Units) < 2 && ok {
		ok = false
		fail(fmt.Sprintf("deliverables.bundle holds %s; a bundle needs two or more.", term.Plural(len(r.Summary.Units), "deliverable")), "Add another deliverable to the project and the bundle's units, or remove the bundle.")
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

// Holds reports whether the bundle holds the deliverable of kind.
func (r Report) Holds(kind string) bool { return slices.Contains(r.Summary.Units, kind) }

// DownloadPage renders the bundle's unlisted download page with every file
// of every unit, or a note while they are not all built.
func DownloadPage(r Report, accent string, files []deliver.File) []byte {
	w := deliver.Words(r.Config.Language)
	return deliver.Page(deliver.Download{Language: r.Config.Language, Title: r.Config.Name, Accent: accent,
		Heading: fmt.Sprintf(w.Heading, r.Config.Name), Files: files, Thanks: w.Thanks, Pending: w.Pending, Keep: w.Keep})
}

// Cover is one unit's picture in the bundle's first listing image: a book
// cover, a printable's first page, another deliverable's first listing
// image, by the name the hero loads it as.
type Cover struct {
	Name  string
	Ratio float64 // width / height
}

// HeroDoc is the bundle's first listing image: every unit's cover side by
// side, 3000×2250 pixels at twice the density.
func HeroDoc(covers []Cover, accent string) []byte {
	if accent == "" {
		accent = "#444444"
	}
	// The covers share 1400 pixels of width at one height, at most 860.
	sum := 0.0
	for _, c := range covers {
		sum += c.Ratio
	}
	gap := 48.0
	h := 860.0
	if sum > 0 {
		h = math.Min(860, (1400-gap*float64(len(covers)-1))/sum)
	}
	var imgs strings.Builder
	for _, c := range covers {
		fmt.Fprintf(&imgs, `<img src="%s" style="height:%.0fpx;width:%.0fpx" alt="">`, html.EscapeString(c.Name), h, h*c.Ratio)
	}
	return []byte(fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:1500px;height:1125px;overflow:hidden}
body{display:flex;align-items:center;justify-content:center;gap:%.0fpx;background:radial-gradient(circle at 50%% 40%%,#fff 0,color-mix(in srgb,%s 10%%,#f4f2ee) 75%%)}
img{display:block;box-shadow:0 2px 6px rgba(0,0,0,.18),0 18px 40px rgba(0,0,0,.16);background:#fff}
</style></head><body>%s</body></html>`, gap, accent, imgs.String()))
}

// Image is one listing image of the bundle.
type Image struct {
	Name string
	Data []byte
}

// Distribution assembles the bundle's shop packages from the files of every
// unit: groups names the ZIPs Etsy gets when the files are more than
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
