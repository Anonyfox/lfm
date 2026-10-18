package book

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"math"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Anonyfox/lfm/internal/imaging"
	"github.com/Anonyfox/lfm/internal/term"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"golang.org/x/text/unicode/norm"
)

// Built is the output of a book build.
type Built struct {
	EPUB        []byte
	Print       []byte            // one HTML document for printing the PDF
	PrintAssets map[string][]byte // flat file names the print documents reference
	TrimInches  [2]float64
	PrintCover  string // flat name of the image for the printed front cover
	CoverPixels [2]int // its size

	plan                  Plan
	cfg                   Config
	listing               Listing
	language, style       string
	year                  int
	screenDocs, paperDocs []chapterDoc
}

// PrintOptions shape a print document beyond the downloadable PDF.
type PrintOptions struct {
	Paper  bool         // the paperback interior: no cover and no links, mirrored margins
	Inside float64      // inside margin of the paperback in inches
	Pages  map[int]int  // chapter number -> page, printed in the contents
	Blanks map[int]bool // chapter number -> a blank page comes before it
	PadEnd int          // blank pages at the end
}

// Document renders a print document: the downloadable PDF with zero options.
func (b Built) Document(o PrintOptions) []byte {
	docs, isbn := b.screenDocs, ""
	if o.Paper {
		docs, isbn = b.paperDocs, b.cfg.ISBNPaperback
	}
	return printDocument(b, docs, isbn, o)
}

// Reader reads a file of the book folder by its book-relative path.
type Reader func(rel string) ([]byte, error)

type chapterDoc struct {
	ch   Chapter
	body []byte // rendered XHTML body content
}

var cssURL = regexp.MustCompile(`url\(\s*(['"]?)([^'")]+)['"]?\s*\)`)

// Build renders the finished book. modified is the newest change of its sources.
func Build(r Report, read Reader, modified time.Time) (Built, error) {
	plan, cfg := r.Plan, r.Config
	b := Built{PrintAssets: map[string][]byte{}, TrimInches: Trims[plan.Trim]}
	if b.TrimInches == [2]float64{} {
		b.TrimInches = Trims["6x9in"]
	}
	language := cfg.Language
	if language == "" {
		language = "en"
	}

	// Every file the book needs, by book-relative path.
	images := map[string][]byte{}
	for rel, data := range r.Sources {
		if imageExt(rel) {
			images[rel] = data
		}
	}
	style := string(r.Sources[StyleFile])
	fonts := map[string][]byte{}
	for _, m := range cssURL.FindAllStringSubmatch(style, -1) {
		ref := m[2]
		if strings.Contains(ref, ":") || strings.HasPrefix(ref, "#") {
			continue
		}
		rel := path.Clean(ref)
		if outside(rel) {
			return b, fmt.Errorf("%s references %s, outside deliverables/book; copy it into fonts/", StyleFile, ref)
		}
		data, err := read(rel)
		if err != nil {
			return b, fmt.Errorf("%s references %s, which cannot be read: %w", StyleFile, ref, err)
		}
		if imageExt(rel) {
			images[rel] = data
		} else {
			fonts[rel] = data
		}
	}

	if _, ok := images[plan.Cover]; !ok {
		plan.Cover = "" // a missing cover is a book.images warning; the book builds without it
	}

	// Chapters, converted once per target because link and image paths differ.
	render := func(target string) ([]chapterDoc, error) {
		var docs []chapterDoc
		for _, ch := range plan.Chapters {
			src := r.Sources[ch.File]
			body, err := renderChapter(ch, src, plan, target, language)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", ch.File, err)
			}
			docs = append(docs, chapterDoc{ch, body})
		}
		return docs, nil
	}
	epubDocs, err := render("epub")
	if err != nil {
		return b, err
	}
	if b.screenDocs, err = render("print"); err != nil {
		return b, err
	}
	if b.paperDocs, err = render("paper"); err != nil {
		return b, err
	}
	b.plan, b.cfg, b.listing, b.language, b.style, b.year = plan, cfg, r.Listing, language, style, modified.Year()

	b.EPUB, err = writeEPUB(plan, cfg, r.Listing, language, epubDocs, images, fonts, style, modified)
	if err != nil {
		return b, err
	}
	b.Print = b.Document(PrintOptions{})
	for _, name := range []string{"images/cover-print.jpg", "images/cover-print.png", "images/cover-print.webp"} {
		if data, err := read(name); err == nil {
			images[name] = data
			b.PrintCover = flatName("img", name)
			info := imaging.Inspect(data)
			b.CoverPixels = [2]int{info.Width, info.Height}
			break
		}
	}
	if b.PrintCover == "" && plan.Cover != "" {
		b.PrintCover = flatName("img", plan.Cover)
		info := imaging.Inspect(images[plan.Cover])
		b.CoverPixels = [2]int{info.Width, info.Height}
	}
	for rel, data := range images {
		b.PrintAssets[flatName("img", rel)] = data
	}
	for rel, data := range fonts {
		b.PrintAssets[flatName("font", rel)] = data
	}
	return b, nil
}

func flatName(prefix, rel string) string {
	return prefix + "-" + strings.NewReplacer("/", "-", " ", "-").Replace(rel)
}

func chapterFile(n int) string { return fmt.Sprintf("ch%02d.xhtml", n) }

// renderChapter converts one chapter to XHTML body content for a target:
// "epub" (files in text/, images in ../images/), "print" (one document,
// flat asset names, chapters as #ch01 anchors) or "paper" (as print, without
// links).
func renderChapter(ch Chapter, src []byte, plan Plan, target, language string) ([]byte, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table, extension.Strikethrough, typographerFor(language),
			extension.NewFootnote(extension.WithFootnoteIDPrefix([]byte(fmt.Sprintf("c%d-", ch.N))))),
		goldmark.WithRendererOptions(gmhtml.WithXHTML()),
	)
	doc := md.Parser().Parse(text.NewReader(src))
	files := map[string]int{}
	for _, c := range plan.Chapters {
		files[c.File] = c.N
	}
	dir := path.Dir(ch.File)
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Image:
			rel := path.Clean(path.Join(dir, string(v.Destination)))
			if target == "epub" {
				v.Destination = []byte("../" + rel)
			} else {
				v.Destination = []byte(flatName("img", rel))
			}
		case *ast.Link:
			dest := string(v.Destination)
			if strings.Contains(dest, "://") || strings.HasPrefix(dest, "mailto:") || strings.HasPrefix(dest, "#") {
				return ast.WalkContinue, nil
			}
			file, frag, _ := strings.Cut(dest, "#")
			if n, ok := files[path.Clean(path.Join(dir, file))]; ok {
				switch {
				case target == "epub" && frag != "":
					v.Destination = []byte(chapterFile(n) + "#" + frag)
				case target == "epub":
					v.Destination = []byte(chapterFile(n))
				case frag != "":
					v.Destination = []byte("#" + frag)
				default:
					v.Destination = []byte(fmt.Sprintf("#ch%02d", n))
				}
			}
		}
		return ast.WalkContinue, nil
	})
	var out bytes.Buffer
	if err := md.Renderer().Render(&out, src, doc); err != nil {
		return nil, err
	}
	if target == "paper" {
		return paperLinks(out.Bytes()), nil
	}
	return out.Bytes(), nil
}

var (
	backLink   = regexp.MustCompile(`\s*<a [^>]*class="footnote-backref"[^>]*>.*?</a>`)
	anyLink    = regexp.MustCompile(`(?s)<a href="([^"]*)"[^>]*>(.*?)</a>`)
	linkScheme = regexp.MustCompile(`^https?://`)
)

// paperLinks turns links into text for print, where nothing can be clicked
// and the printer wants no annotations: web addresses follow their text.
func paperLinks(body []byte) []byte {
	body = backLink.ReplaceAll(body, nil)
	var out bytes.Buffer
	last := 0
	for _, m := range anyLink.FindAllSubmatchIndex(body, -1) {
		out.Write(body[last:m[0]])
		last = m[1]
		text := body[m[4]:m[5]]
		href := html.UnescapeString(string(body[m[2]:m[3]]))
		shown := strings.TrimSuffix(linkScheme.ReplaceAllString(href, ""), "/")
		out.Write(text)
		switch {
		case !linkScheme.MatchString(href) || strings.Contains(html.UnescapeString(string(text)), shown):
		case last < len(body) && body[last] == ')':
			// Already in parentheses: the address joins them.
			out.WriteString(`, <span class="url">` + esc(shown) + `</span>`)
		default:
			out.WriteString(` <span class="url">(` + esc(shown) + `)</span>`)
		}
	}
	out.Write(body[last:])
	return out.Bytes()
}

// baseCSS is lfm's book typography; book.css follows it and wins.
func baseCSS(plan Plan) string {
	body, headings, accent := plan.Body, plan.Headings, plan.Accent
	if body == "" {
		body = "Georgia, serif"
	}
	if headings == "" {
		headings = body
	}
	if accent == "" {
		accent = "#222222"
	}
	return fmt.Sprintf(`:root { --body: %s; --headings: %s; --accent: %s; }
html { font-family: var(--body); line-height: 1.5; }
body { margin: 0 4%%; }
h1, h2, h3, h4 { font-family: var(--headings); line-height: 1.2; break-after: avoid; page-break-after: avoid; }
h1 { font-size: 1.8em; margin: 1.5em 0 1em; }
h2 { font-size: 1.3em; margin: 1.6em 0 0.6em; }
h3 { font-size: 1.1em; margin: 1.4em 0 0.4em; }
p { margin: 0 0 0.8em; orphans: 2; widows: 2; hyphens: auto; -webkit-hyphens: auto; }
a { color: var(--accent); }
img { max-width: 100%%; height: auto; }
figure, .figure { margin: 1.2em 0; text-align: center; }
blockquote { margin: 1em 1.5em; }
pre { white-space: pre-wrap; overflow-wrap: anywhere; font-size: 0.85em; }
code { font-size: 0.9em; overflow-wrap: anywhere; }
table { border-collapse: collapse; width: 100%%; font-size: 0.9em; }
th, td { border-bottom: 1px solid #999; padding: 0.3em 0.4em; text-align: left; vertical-align: top; }
hr { border: 0; border-top: 1px solid #999; margin: 2em 20%%; }
.chapter-label { margin: 2em 0 0; font-family: var(--headings); font-size: 0.85em; letter-spacing: 0.04em; color: var(--accent); }
.chapter-label + h1 { margin-top: 0.2em; }
.footnotes { font-size: 0.85em; }
.title-page { text-align: center; margin-top: 30%%; }
.title-page h1 { font-size: 2.2em; margin: 0 0 0.4em; }
.title-page .subtitle { font-size: 1.15em; margin: 0 0 2em; }
.title-page .author { font-size: 1em; }
.cover { margin: 0; padding: 0; text-align: center; }
.cover img { max-height: 100%%; }
`, cssValue(body), cssValue(headings), cssValue(accent))
}

// cssValue keeps a concept value from breaking out of its declaration.
func cssValue(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ';' || r == '{' || r == '}' || r == '<' || r == '>' {
			return -1
		}
		return r
	}, s)
}

func xhtmlPage(language, title, css string, body string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE html>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" xml:lang="%[1]s" lang="%[1]s">
<head>
<meta charset="utf-8"/>
<title>%[2]s</title>
<link rel="stylesheet" type="text/css" href="%[3]s"/>
</head>
<body>
%[4]s
</body>
</html>
`, esc(language), esc(title), css, body)
	return b.Bytes()
}

func esc(s string) string { return html.EscapeString(s) }

// copyrightPage is the front-matter page with the rights line, the publisher,
// the ISBN of this edition and the imprint text, line by line.
func copyrightPage(cfg Config, l Listing, year int, isbn string, xhtml bool) string {
	w := Words(cfg.Language)
	holder := cfg.Author
	var b strings.Builder
	open := `<section class="copyright">`
	if xhtml {
		open = `<section class="copyright" epub:type="copyright-page">`
	}
	b.WriteString(open)
	fmt.Fprintf(&b, "<p>© %d %s. %s</p>", year, esc(holder), esc(w.Rights))
	if cfg.Publisher != "" {
		fmt.Fprintf(&b, "<p>%s</p>", esc(cfg.Publisher))
	}
	if isbn != "" {
		fmt.Fprintf(&b, "<p>ISBN %s</p>", esc(isbn))
	}
	for _, line := range strings.Split(strings.TrimSpace(cfg.Imprint), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			fmt.Fprintf(&b, "<p>%s</p>", esc(line))
		}
	}
	if l.AIUsed() && l.AIStatement != "" {
		fmt.Fprintf(&b, "<p>%s</p>", esc(l.AIStatement))
	}
	b.WriteString("</section>")
	return b.String()
}

// accessibilityMeta is the EPUB Accessibility 1.1 discovery metadata and
// conformance claim. lfm's checks back each statement: every image has a text
// alternative, headings never skip a level, the language is declared, the
// reading order is the spine and the text reflows. The claim is a
// self-assessment by the publisher, or the author without one.
func accessibilityMeta(cfg Config, language string, pictures bool) string {
	features := []string{"structuralNavigation", "tableOfContents", "readingOrder", "displayTransformability"}
	modes := []string{"textual"}
	if pictures {
		features = append(features, "alternativeText")
		modes = append(modes, "visual")
	}
	var b strings.Builder
	for _, m := range modes {
		fmt.Fprintf(&b, "<meta property=\"schema:accessMode\">%s</meta>\n", m)
	}
	b.WriteString("<meta property=\"schema:accessModeSufficient\">textual</meta>\n")
	for _, f := range features {
		fmt.Fprintf(&b, "<meta property=\"schema:accessibilityFeature\">%s</meta>\n", f)
	}
	b.WriteString("<meta property=\"schema:accessibilityHazard\">none</meta>\n")
	fmt.Fprintf(&b, "<meta property=\"schema:accessibilitySummary\">%s</meta>\n", esc(Words(language).Accessible))
	certifier := cfg.Publisher
	if certifier == "" {
		certifier = cfg.Author
	}
	b.WriteString("<meta property=\"dcterms:conformsTo\">EPUB Accessibility 1.1 - WCAG 2.2 Level AA</meta>\n")
	fmt.Fprintf(&b, "<meta property=\"a11y:certifiedBy\">%s</meta>\n", esc(certifier))
	return b.String()
}

// wellFormed parses XML strictly, as reading systems do.
func wellFormed(name string, data []byte) error {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.Strict = true
	for {
		_, err := d.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s is not well-formed XHTML: %w", name, err)
		}
	}
}

var mediaTypes = map[string]string{
	".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp", ".gif": "image/gif", ".svg": "image/svg+xml",
	".woff2": "font/woff2", ".woff": "font/woff", ".ttf": "font/ttf", ".otf": "font/otf",
}

func writeEPUB(plan Plan, cfg Config, listing Listing, language string, docs []chapterDoc, images, fonts map[string][]byte, style string, modified time.Time) ([]byte, error) {
	type item struct {
		id, href, media, props string
		data                   []byte
	}
	var items []item
	add := func(id, href, media, props string, data []byte) {
		items = append(items, item{id, href, media, props, data})
	}
	css := baseCSS(plan) + "\n" + cssURL.ReplaceAllStringFunc(style, func(m string) string {
		sub := cssURL.FindStringSubmatch(m)
		if strings.Contains(sub[2], ":") || strings.HasPrefix(sub[2], "#") {
			return m
		}
		return "url(\"../" + path.Clean(sub[2]) + "\")"
	})
	add("css", "css/book.css", "text/css", "", []byte(css))
	keys := func(m map[string][]byte) []string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	for i, rel := range keys(images) {
		props := ""
		if rel == plan.Cover {
			props = "cover-image"
		}
		media := mediaTypes[strings.ToLower(path.Ext(rel))]
		add(fmt.Sprintf("img%d", i+1), rel, media, props, images[rel])
	}
	for i, rel := range keys(fonts) {
		media := mediaTypes[strings.ToLower(path.Ext(rel))]
		if media == "" {
			return nil, fmt.Errorf("%s references %s, which is not a WOFF2, WOFF, TTF or OTF font", StyleFile, rel)
		}
		add(fmt.Sprintf("font%d", i+1), rel, media, "", fonts[rel])
	}
	var spine []string
	if plan.Cover != "" {
		body := fmt.Sprintf(`<section class="cover" epub:type="cover"><img src="../%s" alt="%s"/></section>`, esc(plan.Cover), esc(fmt.Sprintf(Words(language).CoverOf, plan.Title)))
		add("cover", "text/cover.xhtml", "application/xhtml+xml", "", xhtmlPage(language, plan.Title, "../css/book.css", body))
		spine = append(spine, "cover")
	}
	title := fmt.Sprintf(`<section class="title-page" epub:type="titlepage"><h1>%s</h1>`, esc(plan.Title))
	if plan.Subtitle != "" {
		title += fmt.Sprintf(`<p class="subtitle">%s</p>`, esc(plan.Subtitle))
	}
	title += fmt.Sprintf(`<p class="author">%s</p></section>`, esc(cfg.Author))
	add("title", "text/title.xhtml", "application/xhtml+xml", "", xhtmlPage(language, plan.Title, "../css/book.css", title))
	spine = append(spine, "title")
	add("copyright", "text/copyright.xhtml", "application/xhtml+xml", "", xhtmlPage(language, Words(language).Copyright, "../css/book.css", copyrightPage(cfg, listing, modified.Year(), cfg.ISBNEbook, true)))
	spine = append(spine, "copyright")
	var nav, ncx strings.Builder
	for _, d := range docs {
		id := fmt.Sprintf("ch%02d", d.ch.N)
		label := fmt.Sprintf(`<p class="chapter-label">%s</p>`, esc(fmt.Sprintf(Words(language).Chapter, d.ch.N)))
		page := xhtmlPage(language, d.ch.Title, "../css/book.css", fmt.Sprintf(`<section epub:type="chapter" id="%s">%s%s</section>`, id, label, d.body))
		if err := wellFormed(d.ch.File, page); err != nil {
			return nil, err
		}
		add(id, "text/"+chapterFile(d.ch.N), "application/xhtml+xml", "", page)
		spine = append(spine, id)
		fmt.Fprintf(&nav, "<li><a href=\"%s\">%s</a></li>\n", chapterFile(d.ch.N), esc(d.ch.Title))
		fmt.Fprintf(&ncx, "<navPoint id=\"np%d\" playOrder=\"%d\"><navLabel><text>%s</text></navLabel><content src=\"text/%s\"/></navPoint>\n", d.ch.N, d.ch.N, esc(d.ch.Title), chapterFile(d.ch.N))
	}
	w := Words(language)
	navBody := fmt.Sprintf(`<nav epub:type="toc" id="toc"><h1>%s</h1><ol>
%s</ol></nav>
<nav epub:type="landmarks" hidden=""><ol><li><a epub:type="toc" href="nav.xhtml#toc">%s</a></li><li><a epub:type="bodymatter" href="%s">%s</a></li></ol></nav>`, esc(w.Contents), nav.String(), esc(w.Contents), chapterFile(docs[0].ch.N), esc(w.Start))
	add("nav", "text/nav.xhtml", "application/xhtml+xml", "nav", xhtmlPage(language, w.Contents, "../css/book.css", navBody))
	for _, it := range items {
		if strings.HasSuffix(it.href, ".xhtml") {
			if err := wellFormed(it.href, it.data); err != nil {
				return nil, err
			}
		}
	}

	sum := sha1.Sum([]byte(plan.Title + "\x00" + cfg.Author))
	sum[6] = sum[6]&0x0f | 0x50 // a name-based UUID: the same book keeps its identifier
	sum[8] = sum[8]&0x3f | 0x80
	uuid := fmt.Sprintf("%x-%x-%x-%x-%x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
	stamp := modified.UTC().Truncate(time.Second).Format("2006-01-02T15:04:05Z")
	ncxDoc := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<ncx xmlns="http://www.daisy.org/z3986/2005/ncx/" version="2005-1"><head><meta name="dtb:uid" content="urn:uuid:%s"/></head>
<docTitle><text>%s</text></docTitle><navMap>
%s</navMap></ncx>
`, uuid, esc(plan.Title), ncx.String())
	add("ncx", "toc.ncx", "application/x-dtbncx+xml", "", []byte(ncxDoc))

	var manifest, itemrefs strings.Builder
	for _, it := range items {
		props := ""
		if it.props != "" {
			props = ` properties="` + it.props + `"`
		}
		fmt.Fprintf(&manifest, "<item id=\"%s\" href=\"%s\" media-type=\"%s\"%s/>\n", it.id, esc(it.href), it.media, props)
	}
	for _, id := range spine {
		fmt.Fprintf(&itemrefs, "<itemref idref=\"%s\"/>\n", id)
	}
	meta := ""
	if listing.Short != "" {
		meta += fmt.Sprintf("<dc:description>%s</dc:description>\n", esc(listing.Short))
	}
	if cfg.Publisher != "" {
		meta += fmt.Sprintf("<dc:publisher>%s</dc:publisher>\n", esc(cfg.Publisher))
	}
	if cfg.ISBNEbook != "" {
		meta += fmt.Sprintf("<dc:identifier id=\"isbn\">urn:isbn:%s</dc:identifier>\n", strings.NewReplacer("-", "", " ", "").Replace(cfg.ISBNEbook))
	}
	for _, subject := range listing.BISAC {
		meta += fmt.Sprintf("<dc:subject>%s</dc:subject>\n", esc(subject))
	}
	meta += accessibilityMeta(cfg, language, len(images) > 1 || (len(images) == 1 && plan.Cover == ""))
	coverMeta := ""
	for _, it := range items {
		if it.props == "cover-image" {
			coverMeta = fmt.Sprintf(`<meta name="cover" content="%s"/>`+"\n", it.id)
		}
	}
	titleFull := plan.Title
	opf := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="bookid" xml:lang="%[1]s">
<metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:identifier id="bookid">urn:uuid:%[2]s</dc:identifier>
<dc:title>%[3]s</dc:title>
<dc:creator>%[4]s</dc:creator>
<dc:language>%[1]s</dc:language>
%[5]s<meta property="dcterms:modified">%[6]s</meta>
%[7]s</metadata>
<manifest>
%[8]s</manifest>
<spine toc="ncx">
%[9]s</spine>
</package>
`, esc(language), uuid, esc(titleFull), esc(cfg.Author), meta, stamp, coverMeta, manifest.String(), itemrefs.String())
	if err := wellFormed("content.opf", []byte(opf)); err != nil {
		return nil, err
	}
	container := `<?xml version="1.0" encoding="utf-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>
`

	var out bytes.Buffer
	z := zip.NewWriter(&out)
	write := func(name string, data []byte, method uint16) error {
		w, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: method, Modified: modified.UTC()})
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	// The mimetype entry comes first and uncompressed, so readers can identify the file.
	if err := write("mimetype", []byte("application/epub+zip"), zip.Store); err != nil {
		return nil, err
	}
	if err := write("META-INF/container.xml", []byte(container), zip.Deflate); err != nil {
		return nil, err
	}
	if err := write("OEBPS/content.opf", []byte(opf), zip.Deflate); err != nil {
		return nil, err
	}
	for _, it := range items {
		method := uint16(zip.Deflate)
		if strings.HasPrefix(it.media, "image/") && it.media != "image/svg+xml" {
			method = zip.Store // already compressed
		}
		if err := write("OEBPS/"+it.href, it.data, method); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	if err := verifyEPUB(out.Bytes()); err != nil {
		return nil, fmt.Errorf("the EPUB failed its own check: %w", err)
	}
	return out.Bytes(), nil
}

// Margins of the printed page in inches. The paperback's inside margin grows
// with the page count; see Gutter.
const (
	marginTop     = 0.75
	marginBottom  = 0.85
	marginOutside = 0.6
	marginScreen  = 0.7
)

// Gutter is the inside margin of a paperback with pages pages: KDP's minimum
// gutter for the page count plus a quarter inch, and never below three
// quarters of an inch, which also meets IngramSpark's half-inch margins.
func Gutter(pages int) float64 {
	g := 0.375
	switch {
	case pages > 700:
		g = 0.875
	case pages > 500:
		g = 0.75
	case pages > 300:
		g = 0.625
	case pages > 150:
		g = 0.5
	}
	return max(0.75, g+0.25)
}

// bookCSS is the book's style with its url() references rewritten to the
// flat asset names of the print documents.
func (b Built) bookCSS() string {
	return cssURL.ReplaceAllStringFunc(b.style, func(m string) string {
		sub := cssURL.FindStringSubmatch(m)
		if strings.Contains(sub[2], ":") || strings.HasPrefix(sub[2], "#") {
			return m
		}
		rel := path.Clean(sub[2])
		prefix := "font"
		if imageExt(rel) {
			prefix = "img"
		}
		return "url(\"" + flatName(prefix, rel) + "\")"
	})
}

func printDocument(bk Built, docs []chapterDoc, isbn string, o PrintOptions) []byte {
	plan, cfg, language, trim := bk.plan, bk.cfg, bk.language, bk.TrimInches
	w := Words(language)
	margins := fmt.Sprintf("margin: %.3fin %.3fin %.3fin;", marginTop, marginScreen, marginBottom)
	if o.Paper {
		margins = fmt.Sprintf("margin: %.3fin %.3fin %.3fin; } @page :left { margin-left: %.3fin; margin-right: %.3fin; } @page :right { margin-left: %.3fin; margin-right: %.3fin;",
			marginTop, marginOutside, marginBottom, marginOutside, o.Inside, o.Inside, marginOutside)
	}
	css := baseCSS(plan) + fmt.Sprintf(`
@page { size: %.3fin %.3fin; %s }
@page { @bottom-center { content: counter(page); font-family: var(--body); font-size: 9pt; } }
@page front { @bottom-center { content: none; } }
@page blank { @bottom-center { content: none; } }
html { font-size: 10.5pt; }
body { margin: 0; }
.cover { page: front; height: %.3fin; break-after: page; overflow: hidden; }
.cover img { width: 100%%; height: 100%%; object-fit: cover; max-height: none; }
.title-page, .toc, .copyright { page: front; break-after: page; }
.copyright { display: flex; flex-direction: column; justify-content: flex-end; min-height: %.3fin; font-size: 0.85em; }
.copyright p { margin: 0 0 0.3em; }
.toc ol { list-style: none; padding: 0; }
.toc li { margin: 0.45em 0; }
.toc .entry { display: flex; align-items: baseline; gap: 0.4em; color: inherit; }
.toc .t { flex: 0 1 auto; }
.toc .dots { flex: 1 1 auto; border-bottom: 1px dotted #8c8c8c; min-width: 1em; }
.toc .pg { flex: 0 0 3ch; text-align: right; font-variant-numeric: tabular-nums; }
.chapter { break-before: page; }
.blank { page: blank; break-before: page; height: 1px; }
.url { font-size: 0.85em; overflow-wrap: anywhere; hyphens: manual; -webkit-hyphens: manual; }
a { text-decoration: none; }
`, trim[0], trim[1], margins, trim[1], trim[1]-marginTop-marginBottom-0.1)
	if !o.Paper {
		css += "@page :first { margin: 0; @bottom-center { content: none; } }\n"
	}
	css += "\n" + bk.bookCSS()
	var b strings.Builder
	fmt.Fprintf(&b, "<!doctype html>\n<html lang=\"%s\"><head><meta charset=\"utf-8\"><title>%s</title><style>%s</style></head><body>\n", esc(language), esc(plan.Title), css)
	if plan.Cover != "" && !o.Paper {
		fmt.Fprintf(&b, "<section class=\"cover\"><img src=\"%s\" alt=\"%s\"></section>\n", flatName("img", plan.Cover), esc(fmt.Sprintf(w.CoverOf, plan.Title)))
	}
	fmt.Fprintf(&b, "<section class=\"title-page\"><h1>%s</h1>", esc(plan.Title))
	if plan.Subtitle != "" {
		fmt.Fprintf(&b, "<p class=\"subtitle\">%s</p>", esc(plan.Subtitle))
	}
	fmt.Fprintf(&b, "<p class=\"author\">%s</p></section>\n%s\n<nav class=\"toc\"><h2>%s</h2><ol>\n", esc(cfg.Author), copyrightPage(cfg, bk.listing, bk.year, isbn, false), esc(w.Contents))
	for _, d := range docs {
		page := ""
		if n := o.Pages[d.ch.N]; n > 0 {
			page = fmt.Sprint(n)
		}
		entry := fmt.Sprintf(`<span class="t">%s</span><span class="dots"></span><span class="pg">%s</span>`, esc(d.ch.Title), page)
		if o.Paper {
			fmt.Fprintf(&b, "<li><span class=\"entry\">%s</span></li>\n", entry)
		} else {
			fmt.Fprintf(&b, "<li><a class=\"entry\" href=\"#ch%02d\">%s</a></li>\n", d.ch.N, entry)
		}
	}
	b.WriteString("</ol></nav>\n")
	for _, d := range docs {
		if o.Blanks[d.ch.N] {
			b.WriteString("<div class=\"blank\"></div>\n")
		}
		fmt.Fprintf(&b, "<section class=\"chapter\" id=\"ch%02d\">\n<p class=\"chapter-label\">%s</p>\n%s</section>\n", d.ch.N, esc(fmt.Sprintf(w.Chapter, d.ch.N)), d.body)
	}
	for range o.PadEnd {
		b.WriteString("<div class=\"blank\"></div>\n")
	}
	b.WriteString("</body></html>\n")
	return []byte(b.String())
}

// ChapterStarts maps chapter numbers to the pages they start on, read from
// the outline of a PDF printed from Document: the chapters are its last
// top-level bookmarks. It returns nil when the outline does not fit.
func (b Built) ChapterStarts(pdf []byte) map[int]int {
	pages := OutlinePages(pdf)
	n := len(b.plan.Chapters)
	if len(pages) < n {
		return nil
	}
	starts := map[int]int{}
	for i, ch := range b.plan.Chapters {
		starts[ch.N] = pages[len(pages)-n+i]
	}
	return starts
}

// Recto plans the blank pages that make every chapter start on a right-hand
// page, from where the chapters start without them, and the page each then
// starts on.
func (b Built) Recto(starts map[int]int) (blanks map[int]bool, pages map[int]int) {
	blanks, pages = map[int]bool{}, map[int]int{}
	shift := 0
	for _, ch := range b.plan.Chapters {
		p := starts[ch.N] + shift
		if p%2 == 0 {
			blanks[ch.N] = true
			shift++
			p++
		}
		pages[ch.N] = p
	}
	return blanks, pages
}

// Spine widths in inches per page, by paper, as KDP prints them.
var spinePerPage = map[string]float64{"white": 0.002252, "cream": 0.0025}

// KDPSpine is the spine width of a KDP paperback.
func KDPSpine(pages int, paper string) float64 {
	per, ok := spinePerPage[paper]
	if !ok {
		per = spinePerPage["white"]
	}
	return float64(pages) * per
}

// LuluSpine is the spine width of a Lulu perfect-bound paperback.
func LuluSpine(pages int) float64 { return float64(pages)/444 + 0.06 }

// Bleed is the margin beyond the trim that print covers extend into.
const Bleed = 0.125

// CoverDocument renders the full wrap of the paperback cover: back, spine
// and front side by side with bleed on every edge. The spine carries title
// and author when spineText is set; the back carries the listing's back cover
// text and, with a paperback ISBN, its barcode.
func (bk Built) CoverDocument(spine float64, spineText bool) []byte {
	plan, cfg := bk.plan, bk.cfg
	tw, th := bk.TrimInches[0], bk.TrimInches[1]
	width, height := 2*Bleed+2*tw+spine, th+2*Bleed
	accent := plan.Accent
	if accent == "" {
		accent = "#222222"
	}
	ink := "#ffffff"
	if luminance(accent) > 0.45 {
		ink = "#111111"
	}
	css := baseCSS(plan) + fmt.Sprintf(`
@page { size: %.4fin %.4fin; margin: 0; }
html, body { margin: 0; padding: 0; }
body { width: %.4fin; height: %.4fin; position: relative; overflow: hidden; background: %s; color: %s; -webkit-print-color-adjust: exact; print-color-adjust: exact; }
.back { position: absolute; left: 0; top: 0; width: %.4fin; height: %.4fin; box-sizing: border-box; padding: %.4fin 0.5in 1.8in %.4fin; font-size: 10.5pt; }
.back h2 { margin: 0 0 0.6em; font-size: 1.35em; }
.back p { margin: 0 0 0.7em; hyphens: auto; }
.back .author { margin-top: 1.2em; font-family: var(--headings); }
.barcode { position: absolute; left: %.4fin; top: %.4fin; width: 1.8in; height: 1.1in; background: #fff; }
.spine { position: absolute; left: %.4fin; top: 0; width: %.4fin; height: %.4fin; display: flex; align-items: center; justify-content: center; }
.spine span { writing-mode: vertical-rl; white-space: nowrap; font-family: var(--headings); font-size: %.1fpt; }
.front { position: absolute; left: %.4fin; top: 0; width: %.4fin; height: %.4fin; overflow: hidden; }
.front img { width: 100%%; height: 100%%; object-fit: cover; display: block; }
`, width, height, width, height, cssValue(accent), ink,
		tw+Bleed, height, Bleed+0.5, Bleed+0.5,
		Bleed+tw-0.25-1.8, Bleed+th-0.25-1.1,
		Bleed+tw, spine, height, min(14, spine*72*0.5),
		Bleed+tw+spine, tw+Bleed, height) + "\n" + bk.bookCSS()
	var b strings.Builder
	fmt.Fprintf(&b, "<!doctype html>\n<html lang=\"%s\"><head><meta charset=\"utf-8\"><title>%s</title><style>%s</style></head><body>\n<section class=\"back\"><h2>%s</h2>", esc(bk.language), esc(plan.Title), css, esc(plan.Title))
	for _, p := range paragraphs(PlainText(bk.listing.BackCover)) {
		fmt.Fprintf(&b, "<p>%s</p>", esc(p))
	}
	fmt.Fprintf(&b, "<p class=\"author\">%s</p></section>\n", esc(cfg.Author))
	if svg := barcodeSVG(cfg.ISBNPaperback); svg != "" {
		fmt.Fprintf(&b, "<div class=\"barcode\">%s</div>\n", svg)
	}
	b.WriteString("<section class=\"spine\">")
	if spineText {
		fmt.Fprintf(&b, "<span>%s · %s</span>", esc(plan.Title), esc(cfg.Author))
	}
	b.WriteString("</section>\n<section class=\"front\">")
	if bk.PrintCover != "" {
		fmt.Fprintf(&b, "<img src=\"%s\" alt=\"\">", bk.PrintCover)
	}
	b.WriteString("</section>\n</body></html>\n")
	return []byte(b.String())
}

// luminance is the relative luminance of a #rrggbb color.
func luminance(hex string) float64 {
	var r, g, b int
	if _, err := fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b); err != nil {
		return 0
	}
	lin := func(v int) float64 {
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// File is one downloadable format.
type File struct {
	Name, Label, Note string
	Size              int
}

// DownloadPage renders the unlisted page buyers land on after paying. With no
// files the book is still being written, and the page says so.
func DownloadPage(plan Plan, cfg Config, files []File) []byte {
	language := cfg.Language
	if language == "" {
		language = "en"
	}
	title := plan.Title
	if title == "" {
		title = "Your book"
	}
	accent := plan.Accent
	if accent == "" {
		accent = "#222222"
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html>
<html lang="%s"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex, nofollow"><meta name="referrer" content="no-referrer">
<title>%s</title>
<style>
body { margin: 0; font: 17px/1.55 system-ui, sans-serif; color: #1a1a1a; background: #fafaf8; }
main { max-width: 34rem; margin: 0 auto; padding: 3rem 1.25rem; }
h1 { font-size: 1.7rem; line-height: 1.2; margin: 0 0 0.3rem; }
.sub { margin: 0 0 2rem; color: #4a4a4a; }
ul { list-style: none; padding: 0; margin: 0 0 2rem; }
li { margin: 0 0 1rem; }
a.file { display: inline-block; padding: 0.7rem 1rem; color: #fff; background: %s; text-decoration: none; font-weight: 600; }
a.file:focus-visible { outline: 3px solid #1a1a1a; outline-offset: 2px; }
.note { display: block; margin-top: 0.3rem; font-size: 0.9rem; color: #4a4a4a; }
</style></head><body><main>
<h1>%s</h1>
`, esc(language), esc(fmt.Sprintf(Words(language).DownloadTitle, title)), cssValue(accent), esc(title))
	sub := cfg.Author
	if plan.Subtitle != "" {
		sub = plan.Subtitle + " · " + cfg.Author
	}
	fmt.Fprintf(&b, "<p class=\"sub\">%s</p>\n", esc(sub))
	w := Words(language)
	if len(files) == 0 {
		fmt.Fprintf(&b, "<p>%s</p>\n", esc(w.Writing))
	} else {
		fmt.Fprintf(&b, "<p>%s</p>\n<ul>\n", esc(w.Thanks))
		for _, f := range files {
			fmt.Fprintf(&b, "<li><a class=\"file\" href=\"%s\" download>%s (%s)</a><span class=\"note\">%s</span></li>\n", esc(f.Name), esc(f.Label), term.Bytes(f.Size), esc(f.Note))
		}
		fmt.Fprintf(&b, "</ul>\n<p>%s</p>\n", esc(w.Keep))
	}
	b.WriteString("</main></body></html>\n")
	return []byte(b.String())
}

// Slug is a file name for the book: lowercase ASCII, accents folded and the
// letters that do not decompose spelled out, so "Größer leben" is
// groesser-leben.
func Slug(title string) string {
	var b strings.Builder
	dash := false
	folded := strings.NewReplacer("ß", "ss", "ä", "ae", "ö", "oe", "ü", "ue", "æ", "ae", "œ", "oe", "ø", "o", "ł", "l", "đ", "d", "þ", "th").Replace(strings.ToLower(title))
	for _, r := range norm.NFD.String(folded) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "book"
	}
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	return s
}
