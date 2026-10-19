package deliver

import (
	"archive/zip"
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func aiListing(t *testing.T) ShopListing {
	t.Helper()
	p := cleanParts()
	p.ai = "- Text: assisted\n- Images: none\n- Tools: Claude\n- Statement: The text was drafted with Claude and edited by hand."
	l, problems := check(t, p.text(), nil)
	if len(problems) > 0 {
		t.Fatalf("%+v", problems)
	}
	return l
}

func images(n int) []string {
	var out []string
	for i := range n {
		out = append(out, fmt.Sprintf("%02d-image.jpg", i+1))
	}
	return out
}

func TestPackageWritesAGuidePerShopWithTheFacts(t *testing.T) {
	pkg := ShopPackage{
		Name:    "Weekly Planner",
		Source:  "deliverables/printable",
		Listing: aiListing(t),
		Files:   []string{"planner-a4.pdf", "planner-letter.pdf"},
		Images:  images(6),
		Facts:   []string{"12 pages, A4 and US Letter", "Undated"},
		Decide:  []string{"**Bundle.** Decide whether the planner joins the bundle."},
	}
	files := map[string][]byte{"planner-a4.pdf": make([]byte, 1234), "planner-letter.pdf": make([]byte, 2048)}
	built, issues := Package(pkg, files, "2026-10-19")
	if len(issues) > 0 {
		t.Fatalf("%+v", issues)
	}
	if !slices.Equal(built, Shops()) || !slices.Equal(Shops(), []string{"etsy", "gumroad", "payhip", "ko-fi"}) {
		t.Fatalf("built %q, shops %q", built, Shops())
	}
	for _, key := range built {
		guide := string(files["stores/"+key+".md"])
		for _, want := range []string{"Built by lfm compile on 2026-10-19 from deliverables/printable", "- [`planner-a4.pdf`](../planner-a4.pdf) (1.2 KiB)", "- [`planner-letter.pdf`](../planner-letter.pdf) (2.0 KiB)",
			"- [`images/01-image.jpg`](../images/01-image.jpg)", "- **Title:** Weekly Planner Printable, Undated, A4 and Letter", "- **Price:** 7.99 EUR",
			"• 12 pages, A4 and US Letter\n• Undated", "The text was drafted with Claude and edited by hand.", "put its address into lfm.json `urls`"} {
			if !strings.Contains(guide, want) {
				t.Errorf("%s guide lacks %q:\n%s", key, want, guide)
			}
		}
		if hasTags := strings.Contains(guide, "## Tags"); hasTags != (key == "etsy") {
			t.Errorf("%s guide lists tags: %v", key, hasTags)
		}
	}
	etsy := string(files["stores/etsy.md"])
	if !strings.HasPrefix(etsy, "# Etsy: Weekly Planner\n\nCreate the listing at https://www.etsy.com/") {
		t.Fatalf("etsy heading:\n%s", etsy)
	}
	if !strings.Contains(etsy, "Paste as plain text:\n\n```text\nA weekly planner you print at home.") || !strings.Contains(etsy, "• Undated pages for any year") {
		t.Errorf("etsy description is not plain text:\n%s", etsy)
	}
	if !strings.Contains(etsy, "- `weekly planner`\n- `printable planner`\n- `undated planner`") || !strings.Contains(etsy, "AI: Etsy requires the AI use stated") {
		t.Errorf("etsy tags or AI note:\n%s", etsy)
	}
	if !strings.Contains(etsy, "images/06-image.jpg") {
		t.Errorf("etsy takes all six images:\n%s", etsy)
	}
	gumroad := string(files["stores/gumroad.md"])
	if !strings.Contains(gumroad, "A **weekly planner** you print at home.") || strings.Contains(gumroad, "```text") || strings.Contains(gumroad, "AI: Etsy") {
		t.Errorf("gumroad keeps Markdown and only its own notes:\n%s", gumroad)
	}
	if !strings.Contains(gumroad, "Gumroad is the merchant of record") || !strings.Contains(string(files["stores/ko-fi.md"]), "On Ko-fi you are the seller") {
		t.Error("shop notes missing")
	}
	payhip := string(files["stores/payhip.md"])
	if !strings.Contains(payhip, "images/04-image.jpg") || strings.Contains(payhip, "images/05-image.jpg") {
		t.Errorf("payhip takes four images:\n%s", payhip)
	}

	readme := string(files["README.md"])
	for _, want := range []string{"# Weekly Planner: shop packages", "Built by `lfm compile` on 2026-10-19 from `deliverables/printable`",
		"| Etsy | [`stores/etsy.md`](stores/etsy.md) | 2 | 6 |", "| Gumroad | [`stores/gumroad.md`](stores/gumroad.md) | 2 | 6 |",
		"| Payhip | [`stores/payhip.md`](stores/payhip.md) | 2 | 4 |", "| Ko-fi Shop | [`stores/ko-fi.md`](stores/ko-fi.md) | 2 | 6 |",
		"- 12 pages, A4 and US Letter\n- Undated\n", "## Decide once\n\n- **Bundle.** Decide whether the planner joins the bundle.\n- **One price everywhere.**", "**VAT.**", "**Withdrawal.**"} {
		if !strings.Contains(readme, want) {
			t.Errorf("README lacks %q:\n%s", want, readme)
		}
	}
	if strings.Contains(readme, "## Not built") {
		t.Error("a built package says it is not built")
	}
}

func TestPackageWithoutAIUseKeepsTheDescriptionAlone(t *testing.T) {
	l, _ := check(t, cleanParts().text(), nil)
	files := map[string][]byte{}
	Package(ShopPackage{Name: "P", Listing: l}, files, "2026-10-19")
	etsy := string(files["stores/etsy.md"])
	if strings.Contains(etsy, "AI: Etsy requires") {
		t.Fatalf("no AI use, yet the AI note:\n%s", etsy)
	}
}

func TestPackageGroupsFilesIntoZIPsBeyondEtsysFive(t *testing.T) {
	names := []string{"a4.pdf", "a5.pdf", "letter.pdf", "half-letter.pdf", "a4-dark.pdf", "a5-dark.pdf", "letter-dark.pdf"}
	files := map[string][]byte{}
	for _, n := range names {
		files[n] = []byte("%PDF " + n)
	}
	pkg := ShopPackage{Name: "P", Files: names, Groups: map[string][]string{
		"planner-light.zip": {"letter.pdf", "a4.pdf", "a5.pdf", "half-letter.pdf"},
		"planner-dark.zip":  {"a4-dark.pdf", "a5-dark.pdf", "letter-dark.pdf"},
	}}
	if _, issues := Package(pkg, files, "2026-10-19"); len(issues) > 0 {
		t.Fatalf("%+v", issues)
	}
	etsy := string(files["stores/etsy.md"])
	dark, light := strings.Index(etsy, "`planner-dark.zip`"), strings.Index(etsy, "`planner-light.zip`")
	if dark < 0 || light < dark || strings.Contains(etsy, "`a4.pdf`") {
		t.Fatalf("etsy uploads the ZIPs in name order, not the files:\n%s", etsy)
	}
	if gumroad := string(files["stores/gumroad.md"]); strings.Contains(gumroad, ".zip") || strings.Count(gumroad, ".pdf`") != 7 {
		t.Fatalf("other shops take every file:\n%s", gumroad)
	}
	if !strings.Contains(string(files["README.md"]), "| Etsy | [`stores/etsy.md`](stores/etsy.md) | 2 | 0 |") {
		t.Fatalf("README table:\n%s", files["README.md"])
	}
	z, err := zip.NewReader(bytes.NewReader(files["planner-light.zip"]), int64(len(files["planner-light.zip"])))
	if err != nil {
		t.Fatal(err)
	}
	var inside []string
	for _, f := range z.File {
		inside = append(inside, f.Name)
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		r.Close()
		if buf.String() != "%PDF "+f.Name {
			t.Fatalf("%s holds %q", f.Name, buf.String())
		}
	}
	if strings.Join(inside, ",") != "a4.pdf,a5.pdf,half-letter.pdf,letter.pdf" {
		t.Fatalf("zip entries %q", inside)
	}

	again := map[string][]byte{}
	for _, n := range names {
		again[n] = files[n]
	}
	Package(pkg, again, "2026-10-19")
	if !bytes.Equal(again["planner-light.zip"], files["planner-light.zip"]) {
		t.Fatal("the same files packed twice differ")
	}
}

func TestPackageRefusesWhatEtsyRefuses(t *testing.T) {
	t.Run("more than five files without groups", func(t *testing.T) {
		names := []string{"1.pdf", "2.pdf", "3.pdf", "4.pdf", "5.pdf", "6.pdf"}
		files := map[string][]byte{}
		_, issues := Package(ShopPackage{Name: "P", Files: names}, files, "d")
		if len(issues) != 1 || issues[0].Message != "Etsy takes 5 files; the product needs 6." || issues[0].Hint == "" {
			t.Fatalf("%+v", issues)
		}
	})
	t.Run("five files fit", func(t *testing.T) {
		_, issues := Package(ShopPackage{Name: "P", Files: []string{"1", "2", "3", "4", "5"}}, map[string][]byte{}, "d")
		if len(issues) > 0 {
			t.Fatalf("%+v", issues)
		}
	})
	t.Run("a file over 20 MB", func(t *testing.T) {
		files := map[string][]byte{"big.pdf": make([]byte, EtsyFileBytes+1), "small.pdf": make([]byte, EtsyFileBytes)}
		_, issues := Package(ShopPackage{Name: "P", Files: []string{"big.pdf", "small.pdf"}}, files, "d")
		if len(issues) != 1 || !strings.HasPrefix(issues[0].Message, "big.pdf is 20.00 MiB, over Etsy's 20 MB per file.") {
			t.Fatalf("%+v", issues)
		}
	})
	t.Run("a ZIP over 20 MB", func(t *testing.T) {
		names := []string{"1.pdf", "2.pdf", "3.pdf", "4.pdf", "5.pdf", "6.pdf"}
		files := map[string][]byte{}
		for _, n := range names {
			files[n] = make([]byte, 4<<20)
		}
		pkg := ShopPackage{Name: "P", Files: names, Groups: map[string][]string{"all.zip": names}}
		_, issues := Package(pkg, files, "d")
		if len(issues) != 1 || !strings.HasPrefix(issues[0].Message, "all.zip is ") || !strings.Contains(issues[0].Message, "over Etsy's 20 MB") {
			t.Fatalf("%+v", issues)
		}
	})
	t.Run("outside links", func(t *testing.T) {
		pkg := ShopPackage{Name: "P", External: []string{"https://instagram.com/me", "https://shop.example"}}
		_, issues := Package(pkg, map[string][]byte{}, "d")
		if len(issues) != 1 || !strings.Contains(issues[0].Message, "2 outside addresses, such as https://instagram.com/me") {
			t.Fatalf("%+v", issues)
		}
	})
}

func TestPackageBeforeTheFirstReleaseSaysWhatIsMissing(t *testing.T) {
	files := map[string][]byte{}
	Package(ShopPackage{Name: "P", Source: "deliverables/printable", Pending: true}, files, "d")
	for _, key := range Shops() {
		guide := string(files["stores/"+key+".md"])
		if !strings.Contains(guide, "- (none yet: run `lfm compile --release` to print the files)") || !strings.Contains(guide, "- (none yet: run `lfm compile --release` to render them)") {
			t.Errorf("%s guide:\n%s", key, guide)
		}
		if strings.Contains(guide, "**Title:**") || strings.Contains(guide, "**Price:**") {
			t.Errorf("%s guide shows empty fields:\n%s", key, guide)
		}
	}
	if !strings.Contains(string(files["README.md"]), "## Not built\n\n- The product changed since the last release") {
		t.Fatalf("pending README:\n%s", files["README.md"])
	}
}
