package printable

import (
	"fmt"
	"strings"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/diagnostic"
)

// Printed is what the release printed for the printable: a PDF per edition
// and the listing images, in their order.
type Printed struct {
	PDFs    map[string][]byte // edition key -> PDF
	Images  []Image
	Pending bool // some prints wait for the next release
}

// Image is one listing image.
type Image = deliver.Image

// EtsyImages is how many listing images Etsy takes.
const EtsyImages = deliver.EtsyImages

// Distribution assembles the shop packages: every PDF and listing image
// once, a ZIP per paper size only when Etsy's five files would not hold the
// PDFs, a guide per shop in stores/, and a README.
func Distribution(r Report, p Printed, day string) (map[string][]byte, []string, []diagnostic.Diagnostic) {
	files := map[string][]byte{}
	pkg := deliver.ShopPackage{Name: r.Plan.Name, Source: Dir, Listing: r.Listing, Facts: Facts(r), Pending: p.Pending, Groups: map[string][]string{},
		Decide: []string{"**Licence.** The guide page states personal use. Sell classroom or commercial use as separate listings at a higher price, never by changing this file."}}
	for _, ed := range r.Editions {
		if data := p.PDFs[ed.Edition.Key]; data != nil {
			name := r.FileName(ed.Edition)
			files[name] = data
			pkg.Files = append(pkg.Files, name)
			group := r.Slug() + "-" + ed.Edition.Size.Key + ".zip"
			pkg.Groups[group] = append(pkg.Groups[group], name)
		}
		for _, pg := range ed.Pages {
			pkg.External = append(pkg.External, pg.External...)
		}
	}
	for _, img := range p.Images {
		files["images/"+img.Name] = img.Data
		pkg.Images = append(pkg.Images, img.Name)
	}
	channels, found := deliver.Package(pkg, files, day)
	var issues []diagnostic.Diagnostic
	for _, f := range found {
		issues = append(issues, at(ListingFile, 1, RuleChannels, diagnostic.Warning, f.Message, f.Hint))
	}
	return files, channels, issues
}

// Facts are the product's numbers, taken from the build so the listing never
// promises more pages, sizes or links than the files hold.
func Facts(r Report) []string {
	var sizes []string
	seen := map[string]bool{}
	for _, ed := range r.Editions {
		if !seen[ed.Edition.Size.Key] {
			seen[ed.Edition.Size.Key] = true
			sizes = append(sizes, ed.Edition.Size.Label)
		}
	}
	w := wordsFor(r.Config.Language)
	out := []string{fmt.Sprintf(w.PagesPerFile, r.Summary.Pages, strings.Join(sizes, ", "))}
	for _, axis := range r.Plan.Axes {
		out = append(out, axis.Name+": "+strings.Join(axis.Values, ", "))
	}
	for _, part := range r.Plan.Parts {
		out = append(out, part.Name+": "+w.count(part.Pages))
	}
	if r.Summary.Links > 0 {
		out = append(out, fmt.Sprintf(w.LinksChecked, r.Summary.Links))
	}
	if cut := r.Summary.Cut; cut > 0 {
		out = append(out, fmt.Sprintf(w.ToCut, cut))
	}
	return out
}

// DownloadPage renders the unlisted page buyers land on after paying: one
// file per edition, or a note while the files wait for their first print.
func DownloadPage(r Report, files []deliver.File) []byte {
	w := wordsFor(r.Config.Language)
	name := r.Plan.Name
	if name == "" {
		name = "Your download"
	}
	return deliver.Page(deliver.Download{Language: r.Config.Language, Title: name, Subtitle: r.Config.Shop, Accent: r.Plan.Accent,
		Heading: fmt.Sprintf(w.Heading, name), Files: files, Thanks: w.Thanks, Pending: w.Pending, Keep: w.Keep})
}

// FileNote is the line under an edition's download button.
func FileNote(r Report, e Edition) string {
	if e.Size.Digital {
		return wordsFor(r.Config.Language).DigitalNote
	}
	return wordsFor(r.Config.Language).PrintNote
}
