package check

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/fontcheck"
	"golang.org/x/net/html"
)

const fontByteBudget = 128 * 1024
const fontTotalBudget = 256 * 1024

func fontsRule(d *Document) []Diagnostic      { return fontRule(d, "perf.fonts") }
func fontAssetsRule(d *Document) []Diagnostic { return fontRule(d, "perf.font-assets") }
func fontRule(d *Document, rule string) []Diagnostic {
	if !d.fontAnalyzed {
		d.fontFindings = analyzeFonts(d)
		d.fontAnalyzed = true
	}
	var result []Diagnostic
	for _, issue := range d.fontFindings {
		if issue.Rule == rule {
			result = append(result, issue)
		}
	}
	return result
}

// These are advisory static checks. Conditional/alternate rules are retained;
// neither selectors nor today's text prove a font is unused by future states.
func analyzeFonts(d *Document) []Diagnostic {
	var issues []Diagnostic
	assets := map[string]fontcheck.Info{}
	for _, i := range d.Options.FontAssets {
		assets[i.Source] = i
	}
	cache := map[string]fontcheck.Info{}
	infoFor := func(ref string) fontcheck.Info {
		if i, ok := d.Options.FontAssets[ref]; ok {
			return i
		}
		base, _, _ := strings.Cut(ref, "#")
		if i, ok := d.Options.FontAssets[base]; ok {
			return i
		}
		if noQuery, _, _ := strings.Cut(base, "?"); strings.HasPrefix(noQuery, "./") {
			if i, ok := d.Options.FontAssets[noQuery]; ok {
				return i
			}
		}
		if strings.HasPrefix(ref, "data:") {
			key := fmt.Sprintf("inline font %x", sha256.Sum256([]byte(base)))
			if i, ok := cache[key]; ok {
				return i
			}
			i := fontcheck.Data(base)
			i.Source = key[:25]
			cache[key] = i
			assets[key] = i
			return i
		}
		return fontcheck.Info{Unknown: "external or dynamically supplied font; not fetched"}
	}
	pageBase := &url.URL{Scheme: "https", Host: "page.invalid", Path: "/" + strings.TrimPrefix(d.Options.File, "/")}
	if u, err := url.Parse(d.Options.URL); err == nil && u.IsAbs() {
		pageBase = u
	}
	resolve := func(raw string, base *url.URL) string {
		u, err := url.Parse(raw)
		if err != nil {
			return raw
		}
		return base.ResolveReference(u).String()
	}
	faceRefs := map[string][]fontcheck.Face{}
	unknownCSS := false
	type sheet struct {
		node   *html.Node
		raw    []byte
		file   string
		parsed fontcheck.Stylesheet
		err    error
	}
	var sheets []sheet
	familyDefaults := map[string]bool{}
	for _, n := range d.activeNodes("style, link") {
		var raw []byte
		file := ""
		if n.Namespace != "" {
			continue
		}
		if n.Data == "style" {
			if kind := strings.ToLower(strings.TrimSpace(Attr(n, "type"))); kind != "" && kind != "text/css" {
				continue
			}
			raw = []byte(Text(n))
		} else {
			if !relHas(n, "stylesheet") {
				continue
			}
			file = Attr(n, "href")
			var ok bool
			raw, ok = d.Options.Stylesheets[file]
			if !ok {
				unknownCSS = true
				continue
			}
		}
		parsed, err := fontcheck.ReadCSS(raw)
		for family := range parsed.FamilyDefaults {
			familyDefaults[family] = true
		}
		sheets = append(sheets, sheet{n, raw, file, parsed, err})
	}
	for _, s := range sheets {
		n, raw, file, faces, err := s.node, s.raw, s.file, s.parsed.Faces, s.err
		at := func(offset int, message, hint string) Diagnostic {
			x := d.At(n, Warning, message, hint)
			x.Rule = "perf.fonts"
			if file != "" {
				x.Location = diagnostic.At(strings.TrimPrefix(file, "./"), raw, offset)
			}
			return x
		}
		if err != nil {
			issues = append(issues, at(0, "Font inspection could not completely parse this stylesheet.", "Review the CSS parser/bundler diagnostic; rules and source bytes were retained."))
		}
		for _, f := range faces {
			problems := append([]string{}, f.Problems...)
			label := short(f.Family)
			if label == "" {
				label = "(unnamed)"
				problems = append(problems, "missing font-family; browser cannot use this face")
			}
			if len(f.Sources) == 0 {
				problems = append(problems, "missing/unrecognized src URL or local() source; verify the descriptor")
			}
			hasURL, woff2, opaque := false, false, false
			for _, s := range f.Sources {
				if s.Local {
					continue
				}
				hasURL = true
				if s.URL == "" {
					problems = append(problems, "empty font source URL")
					continue
				}
				sheetBase := pageBase
				if file != "" {
					u, err := url.Parse(file)
					if err == nil {
						sheetBase = pageBase.ResolveReference(u)
					}
				}
				key := resolve(s.URL, sheetBase)
				faceRefs[key] = append(faceRefs[key], f)
				i := infoFor(s.URL)
				if i.Format == "woff2" || strings.HasPrefix(strings.ToLower(s.Format), "woff2") {
					woff2 = true
				}
				if i.Format == "" {
					opaque = true
				}
				if s.Format != "" && i.Format != "" && !fontcheck.MatchesFormat(s.Format, i.Format) {
					problems = append(problems, fmt.Sprintf("format(%q) disagrees with %s bytes in %s; browser may skip this source", short(s.Format), i.Format, i.Source))
				}
			}
			if hasURL && (f.Display == "" && !familyDefaults[fontcheck.FamilyKey(f.Family)] || f.Display == "auto" || f.Display == "block") {
				policy := f.Display
				if policy == "" {
					policy = "auto (implicit)"
				}
				problems = append(problems, "font-display is "+policy+"; text may remain invisible while loading")
			}
			if hasURL && !woff2 && !opaque {
				problems = append(problems, "no WOFF2 source; consider a prepared WOFF2 asset to reduce transfer")
			}
			if len(problems) > 0 {
				issues = append(issues, at(f.Offset, "@font-face "+label+": "+strings.Join(problems, "; ")+".", "Keep family/weight/style, local() ordering, variable axes and Unicode subsets intentional. Choose font-display deliberately (swap may shift layout; optional may keep fallback); the compiler preserves these choices and font bytes."))
			}
		}
	}
	preloads := 0
	for _, n := range d.activeNodes("link") {
		if !relHas(n, "preload") {
			continue
		}
		if !strings.EqualFold(Attr(n, "as"), "font") {
			if len(faceRefs[resolve(Attr(n, "href"), pageBase)]) > 0 {
				x := d.At(n, Warning, "Font source preload has as="+short(Attr(n, "as"))+"; CSS font reuse requires as=font.", "Use as=font for CSS font preloads, or keep another destination deliberately for a separate JavaScript fetch. Authored request behavior is preserved.")
				x.Rule = "perf.fonts"
				issues = append(issues, x)
			}
			continue
		}
		preloads++
		ref := Attr(n, "href")
		i := infoFor(ref)
		var problems []string
		if !HasAttr(n, "crossorigin") {
			problems = append(problems, "missing anonymous crossorigin; preload may not be reused by CSS")
		}
		if strings.EqualFold(Attr(n, "crossorigin"), "use-credentials") {
			problems = append(problems, "use-credentials does not match anonymous CSS font fetching")
		}
		if kind := Attr(n, "type"); kind != "" && i.Format != "" && !fontMIMECompatible(kind, i.Format) {
			problems = append(problems, fmt.Sprintf("type=%q disagrees with inspected %s; preload may be skipped", short(kind), i.Format))
		}
		if faces := faceRefs[resolve(ref, pageBase)]; len(faces) > 0 {
			for _, f := range faces {
				local := false
				for _, s := range f.Sources {
					local = local || s.Local
				}
				if f.UnicodeRange != "" || local || f.Conditional {
					problems = append(problems, "preloading bypasses local(), unicode-range or conditional face selection; confirm this font is needed immediately")
					break
				}
			}
		} else if !unknownCSS {
			problems = append(problems, "no matching src in inspected active CSS; confirm intentional JavaScript/dynamic font use")
		}
		if len(problems) > 0 {
			x := d.At(n, Warning, "Font preload: "+strings.Join(problems, "; ")+".", "Match the exact selected CSS URL, format and anonymous CORS mode. Preload only fonts needed immediately; explicit type/credentials/selection remain authored.")
			x.Rule = "perf.fonts"
			issues = append(issues, x)
		}
	}
	if preloads > 2 {
		x := d.Whole(Warning, fmt.Sprintf("%d font preloads can compete with critical CSS/LCP resources.", preloads), "Keep only observed immediately required faces; preloads bypass demand-based font selection.")
		x.Rule = "perf.fonts"
		issues = append(issues, x)
	}
	keys := make([]string, 0, len(assets))
	for key := range assets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var total int
	for _, key := range keys {
		i := assets[key]
		total += i.Bytes
		var problems []string
		if i.Issue != "" {
			problems = append(problems, i.Issue)
		}
		if i.Unknown != "" {
			problems = append(problems, "inspection incomplete: "+i.Unknown)
		}
		if i.Bytes > fontByteBudget {
			problems = append(problems, fmt.Sprintf("%d bytes exceeds advisory per-font budget %d", i.Bytes, fontByteBudget))
		}
		if i.SFNTBytes > 8*1024*1024 {
			problems = append(problems, fmt.Sprintf("declared SFNT size %d bytes exceeds 8 MiB advisory budget (not a measured decode size)", i.SFNTBytes))
		}
		if len(problems) == 0 {
			continue
		}
		x := d.Whole(Warning, i.Source+": "+strings.Join(problems, "; ")+".", "Check the source font in a browser; this is bounded container inspection, not glyph/payload sanitization. Prepare smaller WOFF2 faces or subsets only when language, dynamic content, features and licensing permit; binaries are preserved.")
		x.Rule = "perf.font-assets"
		x.Path = i.Source
		issues = append(issues, x)
	}
	if total > fontTotalBudget {
		x := d.Whole(Warning, fmt.Sprintf("Font assets total %d bytes (advisory budget %d); selection determines actual transfer.", total, fontTotalBudget), "Review retained families/weights and language subsets. Static references do not prove every face downloads or that a face is unused.")
		x.Rule = "perf.font-assets"
		issues = append(issues, x)
	}
	return issues
}

func relHas(n *html.Node, token string) bool {
	for _, s := range strings.Fields(Attr(n, "rel")) {
		if strings.EqualFold(s, token) {
			return true
		}
	}
	return false
}
func fontMIMECompatible(mime, kind string) bool {
	mime = strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	if mime == fontcheck.MIME(kind) {
		return true
	}
	return kind == "woff" && mime == "application/font-woff" || kind == "truetype" && mime == "application/x-font-ttf" || kind == "opentype" && mime == "application/x-font-opentype"
}
