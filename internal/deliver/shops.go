package deliver

import (
	"archive/zip"
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/term"
)

// Shop limits that shape a package.
const (
	EtsyFiles     = 5
	EtsyFileBytes = 20 << 20
	EtsyImages    = 20
)

// ShopPackage is a product sold in digital shops (Etsy, Gumroad, Payhip,
// Ko-fi): its files, listing images and texts.
type ShopPackage struct {
	Name     string
	Source   string // the folder it is built from, e.g. deliverables/printable
	Listing  ShopListing
	Files    []string            // every download, in order; their bytes are in the files map
	Groups   map[string][]string // when Etsy's five files do not hold Files: ZIP name -> files in it
	Images   []string            // listing images under images/, in order
	Facts    []string            // what the files hold, from the build
	Decide   []string            // the README's decisions for the owner
	Pending  bool                // prints wait for the next release
	External []string            // outside links inside the files
}

// Issue is a reason a shop would refuse what was built.
type Issue struct{ Message, Hint string }

type shop struct {
	key, name, url string
	description    string // "plain" or "markdown"
	images         int    // how many listing images it takes
	notes          []string
}

var shopChannels = []shop{
	{key: "etsy", name: "Etsy", url: "https://www.etsy.com/your/shops/me/listing-editor/create", description: "plain", images: EtsyImages, notes: []string{
		"Listing type: digital files, instant download. Who made it: I did. What is it: a finished product. No physical item is shipped; say so in the description.",
		"Etsy has no variations for digital files: every size and variant goes into this one listing.",
		"The first image is the product alone, without text: Etsy ranks single-product photos without overlays higher.",
		"Etsy forbids QR codes and links that send buyers off Etsy (October 2026): the files must not promote another shop or site.",
	}},
	{key: "gumroad", name: "Gumroad", url: "https://gumroad.com/products/new", description: "markdown", images: 8, notes: []string{
		"Gumroad is the merchant of record and collects VAT and sales tax. It keeps 10% plus $0.50 per sale, 30% on Discover sales.",
		"Upload the images as covers; the first one is the thumbnail.",
	}},
	{key: "payhip", name: "Payhip", url: "https://payhip.com/product/add", description: "markdown", images: 4, notes: []string{
		"Payhip remits EU and UK VAT, but payments run through your own Stripe or PayPal, so you remain the seller.",
	}},
	{key: "ko-fi", name: "Ko-fi Shop", url: "https://ko-fi.com/shop/settings", description: "markdown", images: 20, notes: []string{
		"On Ko-fi you are the seller: register for EU VAT (OSS) once cross-border EU sales pass €10,000 a year.",
		"Add the withdrawal-waiver line to the shop's terms; digital downloads lose the right of withdrawal only with the buyer's consent.",
	}},
}

// Shops returns the keys of the digital shops lfm prepares guides for.
func Shops() []string {
	keys := make([]string, len(shopChannels))
	for i, c := range shopChannels {
		keys[i] = c.key
	}
	return keys
}

// Package writes the shop guides and the README of pkg into files, which
// already holds the downloads and images/, adding a ZIP per group when Etsy's
// five files do not hold the downloads. It returns the shops prepared and
// what a shop would refuse.
func Package(pkg ShopPackage, files map[string][]byte, day string) ([]string, []Issue) {
	var issues []Issue
	for _, name := range pkg.Files {
		if n := len(files[name]); n > EtsyFileBytes {
			issues = append(issues, Issue{fmt.Sprintf("%s is %s, over Etsy's 20 MB per file.", name, term.Bytes(n)), "Compress the images or split the product; large PDFs also lag in note apps."})
		}
	}
	etsy := pkg.Files
	if len(pkg.Files) > EtsyFiles && len(pkg.Groups) > 0 {
		etsy = nil
		names := make([]string, 0, len(pkg.Groups))
		for name := range pkg.Groups {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			z, err := zipFiles(pkg.Groups[name], files)
			if err != nil {
				issues = append(issues, Issue{"The ZIP " + name + " could not be written: " + err.Error(), "Compile again; report it if it persists."})
				continue
			}
			files[name] = z
			etsy = append(etsy, name)
			if len(z) > EtsyFileBytes {
				issues = append(issues, Issue{fmt.Sprintf("%s is %s, over Etsy's 20 MB per file.", name, term.Bytes(len(z))), "Offer fewer variants, or compress the images."})
			}
		}
	}
	if len(etsy) > EtsyFiles {
		issues = append(issues, Issue{fmt.Sprintf("Etsy takes %d files; the product needs %d.", EtsyFiles, len(etsy)), "Offer fewer files on Etsy and sell the rest on another channel."})
	}
	if len(pkg.External) > 0 {
		issues = append(issues, Issue{fmt.Sprintf("The files link to %d outside addresses, such as %s; Etsy forbids sending buyers off Etsy.", len(pkg.External), pkg.External[0]), "Remove shop and social links from the files; a link to a tool the product needs is fine elsewhere."})
	}
	var built []string
	var table strings.Builder
	table.WriteString("| Channel | Guide | Files | Images |\n| --- | --- | --- | --- |\n")
	for _, ch := range shopChannels {
		upload := pkg.Files
		if ch.key == "etsy" {
			upload = etsy
		}
		guide := "stores/" + ch.key + ".md"
		n := min(len(pkg.Images), ch.images)
		files[guide] = []byte(shopGuide(ch, pkg, upload, pkg.Images[:n], files, day))
		fmt.Fprintf(&table, "| %s | [`%s`](%s) | %d | %d |\n", ch.name, guide, guide, len(upload), n)
		built = append(built, ch.key)
	}
	files["README.md"] = []byte(shopReadme(pkg, table.String(), day))
	return built, issues
}

// zipFiles packs files reproducibly: fixed times, sorted names, stored PDFs.
func zipFiles(names []string, files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	for _, name := range sorted {
		w, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store, Modified: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(files[name]); err != nil {
			return nil, err
		}
	}
	err := z.Close()
	return buf.Bytes(), err
}

func shopGuide(ch shop, pkg ShopPackage, upload, images []string, files map[string][]byte, day string) string {
	l := pkg.Listing
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: %s\n\nCreate the listing at %s. Built by lfm compile on %s from %s; edit the product and compile again rather than this file.\n\n## Files\n\nUpload these from this package's folder:\n\n", ch.name, pkg.Name, ch.url, day, pkg.Source)
	for _, f := range upload {
		fmt.Fprintf(&b, "- [`%s`](../%s) (%s)\n", f, f, term.Bytes(len(files[f])))
	}
	if len(upload) == 0 {
		b.WriteString("- (none yet: run `lfm compile --release` to print the files)\n")
	}
	b.WriteString("\n## Images\n\nIn this order:\n\n")
	for _, img := range images {
		fmt.Fprintf(&b, "- [`images/%s`](../images/%s)\n", img, img)
	}
	if len(images) == 0 {
		b.WriteString("- (none yet: run `lfm compile --release` to render them)\n")
	}
	b.WriteString("\n## Fields\n\n")
	if l.Title != "" {
		fmt.Fprintf(&b, "- **Title:** %s\n", l.Title)
	}
	if l.Price != nil {
		fmt.Fprintf(&b, "- **Price:** %s\n", l.Price.Text)
	}
	if ch.key == "etsy" && len(l.Tags) > 0 {
		b.WriteString("\n## Tags\n\nOne per box:\n\n")
		for _, t := range l.Tags {
			fmt.Fprintf(&b, "- `%s`\n", t)
		}
	}
	desc := l.Description
	if l.AIUsed() && l.AIStatement != "" {
		desc += "\n\n" + l.AIStatement
	}
	facts := make([]string, len(pkg.Facts))
	for i, f := range pkg.Facts {
		facts[i] = "• " + f
	}
	if ch.description == "plain" {
		fmt.Fprintf(&b, "\n## Description\n\nPaste as plain text:\n\n```text\n%s\n\n%s\n```\n", PlainText(desc), strings.Join(facts, "\n"))
	} else {
		fmt.Fprintf(&b, "\n## Description\n\n%s\n\n%s\n", desc, strings.Join(facts, "\n"))
	}
	notes := ch.notes
	if ch.key == "etsy" && l.AIUsed() {
		notes = append(append([]string(nil), notes...), "AI: Etsy requires the AI use stated in the description; the statement closes the description above.")
	}
	b.WriteString("\n## Before you publish\n\n")
	for _, n := range notes {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	b.WriteString("- Once the listing is live, put its address into lfm.json `urls` and point the page's offer at it when the page sells through this shop.\n")
	return b.String()
}

// PlainText turns listing Markdown into the plain text Etsy shows.
func PlainText(md string) string {
	r := strings.NewReplacer("**", "", "__", "", "### ", "", "## ", "")
	var out []string
	for _, line := range strings.Split(md, "\n") {
		line = r.Replace(line)
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") {
			line = "• " + t[2:]
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func shopReadme(pkg ShopPackage, table, day string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s: shop packages\n\nBuilt by `lfm compile` on %s from `%s`. Every file and listing image is here once; each guide in `stores/` names the files and images its shop takes and every field ready to paste. This folder is never deployed; keep it out of git.\n\n%s\n## What the files hold\n\n", pkg.Name, day, pkg.Source, table)
	for _, f := range pkg.Facts {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	b.WriteString("\n## Decide once\n\n")
	for _, d := range append(pkg.Decide,
		"**One price everywhere.** Your page and every shop sell the same files; keep the price the same.",
		"**VAT.** Etsy, Gumroad and Payhip collect EU VAT; on your own page and Ko-fi you are the seller.",
		"**Withdrawal.** EU buyers lose the right of withdrawal on downloads only when they consent at checkout; Etsy shows that checkbox, your own Stripe checkout needs the wording in its terms.") {
		fmt.Fprintf(&b, "- %s\n", d)
	}
	if pkg.Pending {
		b.WriteString("\n## Not built\n\n- The product changed since the last release: `lfm compile --release` prints the files and renders the images again. Ordinary compiles reuse them while the product is unchanged.\n")
	}
	return b.String()
}
