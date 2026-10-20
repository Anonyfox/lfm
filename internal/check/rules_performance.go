package check

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// HTMLLimit is the page size the HTML check allows. Decimal bytes deliberately
// form a conservative policy below any binary 2 MiB interpretation. The warning leaves headroom for HTTP headers and future edits.
const HTMLLimit = 2_000_000
const HTMLWarning = 1_800_000
const LargeEmbed = 32 * 1024

type Embed struct {
	Sources    []string `json:"sources,omitempty"`
	MIME       string   `json:"mime"`
	Bytes      int      `json:"bytes"`
	Count      int      `json:"count"`
	TotalBytes int      `json:"totalBytes"`
	Offset     int      `json:"offset"`
}

type Stats struct {
	HTMLBytes        int     `json:"htmlBytes"`
	Elements         int     `json:"elements"`
	MaxDepth         int     `json:"maxDepth"`
	CSSBytes         int     `json:"inlineCSSBytes"`
	JSBytes          int     `json:"inlineJSBytes"`
	ExternalCSSBytes int     `json:"externalCSSBytes,omitempty"`
	ExternalJSBytes  int     `json:"externalJSBytes,omitempty"`
	EmbeddedBytes    int     `json:"embeddedBytes"`
	Embeds           []Embed `json:"embeds"`
}

// InspectEmbeds inspects final byte occurrences, not build callbacks: repeated references and
// CSS/JS-embedded URLs must count exactly as they occur in the emitted document.
func InspectEmbeds(raw []byte, sources map[[32]byte][]string) []Embed {
	byHash := map[[32]byte]*Embed{}
	for offset := 0; offset < len(raw); {
		index := bytes.Index(raw[offset:], []byte("data:"))
		if index < 0 {
			break
		}
		start := offset + index
		comma := start + 5
		for comma < len(raw) && comma-start < 256 && raw[comma] != ',' && !uriDelimiter(raw[comma]) {
			comma++
		}
		if comma >= len(raw) || raw[comma] != ',' {
			offset = start + 5
			continue
		}
		header := string(raw[start+5 : comma])
		end := comma + 1
		isBase64 := strings.HasSuffix(strings.ToLower(header), ";base64")
		for end < len(raw) {
			if isBase64 {
				if !base64Byte(raw[end]) {
					break
				}
			} else if uriDelimiter(raw[end]) || raw[end] == '#' {
				break
			}
			end++
		}
		if end == comma+1 {
			offset = end
			continue
		}
		key := sha256.Sum256(raw[start:end])
		if e := byHash[key]; e != nil {
			e.Count++
			e.TotalBytes += end - start
		} else {
			mime := strings.ToLower(strings.Split(header, ";")[0])
			if mime == "" {
				mime = "text/plain"
			}
			paths := append([]string(nil), sources[key]...)
			sort.Strings(paths)
			byHash[key] = &Embed{Sources: paths, MIME: mime, Bytes: end - start, Count: 1, TotalBytes: end - start, Offset: start}
		}
		offset = end
	}
	embeds := make([]Embed, 0, len(byHash))
	for _, e := range byHash {
		embeds = append(embeds, *e)
	}
	sort.Slice(embeds, func(i, j int) bool {
		if embeds[i].TotalBytes != embeds[j].TotalBytes {
			return embeds[i].TotalBytes > embeds[j].TotalBytes
		}
		return embeds[i].Offset < embeds[j].Offset
	})
	return embeds
}

func scanEmbeds(raw []byte, sources map[[32]byte][]string) []Embed {
	return InspectEmbeds(raw, sources)
}

func base64Byte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '/' || c == '='
}
func uriDelimiter(c byte) bool { return c <= ' ' || strings.ContainsRune("\"'<>`)\\}", rune(c)) }

func causeHint(d *Document) string {
	parts := []string{"Move large media/fonts to file-mode assets; gzip/Brotli does not reduce this uncompressed HTML budget."}
	for i, e := range d.Stats.Embeds {
		if i == 5 {
			break
		}
		name := strings.Join(e.Sources, ", ")
		if name == "" {
			name = e.MIME + " data URL"
		}
		part := fmt.Sprintf("%s: %d bytes × %d = %d bytes", name, e.Bytes, e.Count, e.TotalBytes)
		for _, source := range e.Sources {
			part += fmt.Sprintf("; set build.assets.overrides[%q] to \"file\"", source)
		}
		parts = append(parts, part)
	}
	parts = append(parts, fmt.Sprintf("Inline CSS: %d bytes; inline JS: %d bytes; embedded URLs: %d bytes.", d.Stats.CSSBytes, d.Stats.JSBytes, d.Stats.EmbeddedBytes))
	return join(parts...)
}

func sizeRule(d *Document) []Diagnostic {
	if d.Stats.HTMLBytes < HTMLWarning {
		return nil
	}
	level := Warning
	if d.Stats.HTMLBytes > HTMLLimit {
		level = Error
	}
	diagnostic := d.Whole(level, fmt.Sprintf("HTML is %d uncompressed bytes (warning at %d; limit %d).", d.Stats.HTMLBytes, HTMLWarning, HTMLLimit), causeHint(d))
	diagnostic.Reference = "https://developers.google.com/search/docs/crawling-indexing/googlebot"
	return []Diagnostic{diagnostic}
}

func inlineRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for _, embed := range d.Stats.Embeds {
		if embed.TotalBytes <= LargeEmbed {
			continue
		}
		hint := "Large embedded assets cannot be independently cached or lazily transferred. Use automatic packing (omit build.assets.default) or a file override; an explicit inline policy pins bytes."
		if len(embed.Sources) > 0 {
			hint += " Sources: " + strings.Join(embed.Sources, ", ") + "."
		}
		diagnostic := d.Whole(Warning, fmt.Sprintf("Embedded %s occupies %d HTML bytes across %d occurrence(s).", embed.MIME, embed.TotalBytes, embed.Count), hint)
		diagnostic.Location = location("index.html", d.lines, embed.Offset)
		issues = append(issues, diagnostic)
	}
	for _, n := range d.selectNodes("[srcset]") {
		if strings.Contains(Attr(n, "srcset"), "data:") {
			issues = append(issues, d.At(n, Warning, "Responsive image candidates are embedded in the document.", "Use file-mode srcset candidates so the browser transfers the selected image rather than every candidate's bytes."))
		}
	}
	return issues
}

func imagePerformanceRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for index, n := range d.selectNodes("img") {
		width, werr := strconv.Atoi(Attr(n, "width"))
		height, herr := strconv.Atoi(Attr(n, "height"))
		if werr != nil || herr != nil || width <= 0 || height <= 0 {
			hint := "Set intrinsic width and height, or deliberately reserve the correct aspect ratio in CSS; CSS layout cannot be verified by this static rule."
			if index < len(d.Options.Images) {
				image := d.Options.Images[index]
				hint += " Automatic inference skipped: " + image.Reason + ". Source: " + image.Source + "."
			}
			issues = append(issues, d.At(n, Warning, "Image has no valid width/height pair to reserve layout space.", hint))
		}
		if strings.EqualFold(Attr(n, "loading"), "lazy") && strings.EqualFold(Attr(n, "fetchpriority"), "high") {
			issues = append(issues, d.At(n, Warning, "Image is both lazy-loaded and high priority.", "For a hero/LCP image, remove loading=lazy; for offscreen media, remove high priority."))
		}
		if strings.EqualFold(Attr(n, "loading"), "lazy") && strings.HasPrefix(Attr(n, "src"), "data:") && len(Attr(n, "src")) > LargeEmbed {
			issues = append(issues, d.At(n, Warning, "Lazy-loaded image bytes are embedded in HTML.", "Lazy loading may defer rendering, but the bytes still arrive with HTML. Use file mode when saving transfer is the goal."))
		}
	}
	return issues
}

func viewportRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	nodes := metaNodes(d, "name", "viewport")
	if len(nodes) == 0 {
		return []Diagnostic{d.Whole(Warning, "Mobile viewport metadata is missing.", "Use <meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">.")}
	}
	if len(nodes) > 1 {
		issues = append(issues, d.At(nodes[1], Warning, "Multiple viewport declarations.", "Keep one deliberate mobile viewport declaration."))
	}
	for _, n := range nodes {
		for _, part := range strings.Split(strings.ToLower(Attr(n, "content")), ",") {
			key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			if strings.TrimSpace(key) == "user-scalable" && (value == "no" || value == "0") {
				issues = append(issues, d.At(n, Warning, "Viewport disables user zoom.", "Allow zoom so visitors can enlarge text and controls."))
			}
		}
	}
	return issues
}

func budgetsRule(d *Document) []Diagnostic {
	var issues []Diagnostic
	for _, budget := range []struct {
		name         string
		value, limit int
		hint         string
	}{
		{"CSS (inline + files)", d.Stats.CSSBytes + d.Stats.ExternalCSSBytes, 20 * 1024, "Inspect dependencies/unused styles. File packing changes scheduling, not total CSS bytes."},
		{"JavaScript (inline + files)", d.Stats.JSBytes + d.Stats.ExternalJSBytes, 50 * 1024, "Inspect dependencies and startup work; moving code to a file does not remove its CPU cost."},
		{"DOM elements", d.Stats.Elements, 1500, "Reduce repeated markup or inspect rendering costs on a representative phone."},
		{"DOM depth", d.Stats.MaxDepth, 32, "Flatten unnecessary wrappers; this is an advisory budget, not an HTML validity limit."},
	} {
		if budget.value > budget.limit {
			issues = append(issues, d.Whole(Warning, fmt.Sprintf("%s exceeds the advisory budget: %d > %d.", budget.name, budget.value, budget.limit), budget.hint))
		}
	}
	return issues
}
