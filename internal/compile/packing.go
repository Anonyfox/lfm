package compile

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/fontcheck"
	"github.com/Anonyfox/lfm/internal/htmlx"
	"github.com/Anonyfox/lfm/internal/urlcheck"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Policy constants are implementation defaults, not authoring knobs. This is a
// deterministic packing heuristic, not a claim about unknown network/layout cost.
const embeddedDocumentTarget = 256 * 1024
const deferredAssetThreshold = 4 * 1024

type packedAsset struct {
	URI      string
	Bytes    []byte
	Encoding string
}
type codeCandidate struct {
	node               *html.Node
	inputs             []bundleInput
	name, origin, kind string
}

// Decide after binary URL packing: a 40 KiB embedded image must not make an
// otherwise 30-byte stylesheet into a needless external CSS request.
func (b *builder) packCode() error {
	for _, candidate := range b.autoCode {
		n := candidate.node
		code := []byte(htmlx.RawText(n))
		limit := 20 * 1024
		if candidate.kind == "script" {
			limit = 50 * 1024
		}
		if len(code) <= limit || candidate.kind == "script" && strings.Contains(string(code), "import.meta") {
			continue
		}
		if err := b.emit(candidate.name, candidate.origin, code); err != nil {
			return err
		}
		for n.FirstChild != nil {
			n.RemoveChild(n.FirstChild)
		}
		ref := fileURL(candidate.name)
		if candidate.kind == "style" {
			n.Data, n.DataAtom = "link", atom.Link
			htmlx.SetAttr(n, "rel", "stylesheet")
			htmlx.SetAttr(n, "href", ref)
		} else {
			htmlx.SetAttr(n, "src", ref)
		}
		b.bundleReferences(candidate.inputs, ref)
	}
	return nil
}

type assetRole struct{ Responsive, Deferred, Media, Download bool }

type AssetDecision struct {
	Source           string   `json:"source"`
	Aliases          []string `json:"aliases,omitempty"`
	Pinned           bool     `json:"pinned,omitempty"`
	Mode             string   `json:"mode"`
	Bytes            int      `json:"bytes"`
	Contribution     int      `json:"encodedContributionBytes"`
	HTMLContribution int      `json:"htmlContributionBytes"`
	Occurrences      int      `json:"occurrences"`
	Reason           string   `json:"reason"`
	Output           string   `json:"output,omitempty"`
	Encoding         string   `json:"encoding,omitempty"`
}

type Cost struct {
	HTMLBytes        int `json:"htmlBytes"`
	GzipBytes        int `json:"htmlGzipLevel1Bytes"`
	SidecarBytes     int `json:"sidecarBytes"`
	TotalBytes       int `json:"totalRawBytes"`
	CSSBytes         int `json:"totalCSSBytes"`
	JSBytes          int `json:"totalJSBytes"`
	ExternalCSSBytes int `json:"externalCSSBytes"`
	ExternalJSBytes  int `json:"externalJSBytes"`
}

func measureCost(page []byte, doc *html.Node, files map[string]emitted) *Cost {
	c := &Cost{HTMLBytes: len(page)}
	var zipped bytes.Buffer
	w, _ := gzip.NewWriterLevel(&zipped, gzip.BestSpeed)
	_, _ = w.Write(page)
	_ = w.Close()
	c.GzipBytes = zipped.Len()
	// This traversal counts code exactly once, without another HTML serialization.
	htmlx.Walk(doc, func(n *html.Node) {
		if n.Data == "style" {
			c.CSSBytes += len(htmlx.RawText(n))
		}
		if n.Data == "script" && htmlx.Attr(n, "src") == "" {
			kind := strings.ToLower(strings.TrimSpace(htmlx.Attr(n, "type")))
			if kind == "" || kind == "module" || kind == "text/javascript" || kind == "application/javascript" {
				c.JSBytes += len(htmlx.RawText(n))
			}
		}
	})
	for _, f := range files {
		c.SidecarBytes += len(f.contents)
		if strings.Contains(f.origin, "style-bundle:") || f.origin == "release-complete-css" {
			c.ExternalCSSBytes += len(f.contents)
		}
		if strings.Contains(f.origin, "script-bundle:") {
			c.ExternalJSBytes += len(f.contents)
		}
	}
	c.CSSBytes += c.ExternalCSSBytes
	c.JSBytes += c.ExternalJSBytes
	c.TotalBytes = c.HTMLBytes + c.SidecarBytes
	return c
}

// Collect author intent before URLs are compiled. Dormant template media and
// explicitly lazy media are deferrable; DOM order alone cannot establish LCP.
func (b *builder) collectAssetRoles(doc *html.Node) {
	b.roles = map[string]assetRole{}
	htmlx.Walk(doc, func(n *html.Node) {
		role := assetRole{Deferred: strings.EqualFold(htmlx.Attr(n, "loading"), "lazy"), Download: htmlx.HasAttr(n, "download"), Media: n.Data == "video" || n.Data == "audio"}
		for p := n.Parent; p != nil; p = p.Parent {
			if p.Data == "template" {
				role.Deferred = true
			}
			if p.Data == "picture" {
				role.Responsive = true
			}
			if p.Data == "video" || p.Data == "audio" {
				role.Media = true
			}
		}
		if htmlx.Attr(n, "srcset") != "" {
			role.Responsive = true
		}
		add := func(ref string, role assetRole) {
			if remote, err := b.remote(ref); err != nil || remote {
				return
			}
			path, _, err := b.assetPath(ref, b.root)
			if err != nil {
				return
			} // the normal resolver produces the contextual error.
			old := b.roles[path]
			b.roles[path] = assetRole{old.Responsive || role.Responsive, old.Deferred || role.Deferred, old.Media || role.Media, old.Download || role.Download}
		}
		for _, key := range []string{"src", "poster", "data", "href"} {
			if ref := htmlx.Attr(n, key); ref != "" && (key != "href" || role.Download) {
				add(ref, role)
			}
		}
		for _, candidate := range urlcheck.Candidates(htmlx.Attr(n, "srcset")) {
			add(candidate.URL, role)
		}
	})
}

// Cheap per-file decisions precede encoding/bundling; a large video/font must
// not become a giant temporary data URL just to be externalized moments later.
func (b *builder) resourcePolicy(path string, size int64) string {
	switch b.mode(path) {
	case "inline":
		return ""
	case "file":
		return "explicit file policy"
	}
	r := b.roles[path]
	if r.Responsive {
		return "native responsive selection"
	}
	if r.Media || strings.HasPrefix(httpMIME(nil, path), "video/") || strings.HasPrefix(httpMIME(nil, path), "audio/") {
		return "independent media loading/seeking"
	}
	if r.Download {
		return "native download URL"
	}
	if fontExtension(path) && size > deferredAssetThreshold {
		return "font selection/cache avoids initial HTML bytes"
	}
	if r.Deferred && size > deferredAssetThreshold {
		return "lazy/dormant media can transfer later"
	}
	if size > 24*1024 {
		return "individual encoded size exceeds 32 KiB"
	}
	return ""
}

func (b *builder) resourceFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return b.mode(path) == "file"
	}
	reason := b.resourcePolicy(path, info.Size())
	if reason != "" {
		b.rememberFile(path, reason)
	}
	return reason != ""
}
func (b *builder) rememberFile(path, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.fileReasons == nil {
		b.fileReasons = map[string]string{}
	}
	b.fileReasons[path] = reason
}
func (b *builder) fileDecisions(decisions []AssetDecision) []AssetDecision {
	seen := map[string]bool{}
	for _, d := range decisions {
		seen[d.Source] = true
		for _, alias := range d.Aliases {
			seen[alias] = true
		}
	}
	for path, reason := range b.fileReasons {
		rel, _ := filepath.Rel(b.root, path)
		source := filepath.ToSlash(rel)
		if seen[source] {
			continue
		}
		// The file loader may have eliminated this import. Only surviving emitted
		// files get a decision/report row, never an otherwise unused sidecar.
		for _, f := range b.files {
			if f.origin == path {
				decisions = append(decisions, AssetDecision{Source: source, Mode: "file", Bytes: len(f.contents), Reason: reason, Output: f.name})
				break
			}
		}
	}
	sort.Slice(decisions, func(i, j int) bool { return decisions[i].Source < decisions[j].Source })
	return decisions
}

// Plan once on the actual bundled document. Rewriting compiler-generated data
// URL tokens also updates bundled JS/CSS; source text is never rewritten. A trie
// replacer handles all selected assets together instead of N whole-file passes.
func (b *builder) planAssets(doc *html.Node) ([]AssetDecision, error) {
	var snapshot bytes.Buffer
	if err := html.Render(&snapshot, doc); err != nil {
		return nil, err
	}
	counts := map[[32]byte]int{}
	count := func(raw []byte) {
		for _, embed := range check.InspectEmbeds(raw, nil) {
			counts[sha256.Sum256(raw[embed.Offset:embed.Offset+embed.Bytes])] += embed.Count
		}
	}
	htmlx.Walk(doc, func(n *html.Node) {
		for _, a := range n.Attr {
			if resourceAttribute(n, a.Key) {
				count([]byte(a.Val))
			}
		}
		if n.Data == "style" || n.Data == "script" && executableScript(n) {
			count([]byte(htmlx.RawText(n)))
		}
	})
	htmlCounts := map[[32]byte]int{}
	for hash, count := range counts {
		htmlCounts[hash] = count
	}
	for _, f := range b.files {
		if generatedCode(f) {
			for _, embed := range check.InspectEmbeds(f.contents, nil) {
				counts[sha256.Sum256(f.contents[embed.Offset:embed.Offset+embed.Bytes])] += embed.Count
			}
		}
	}
	paths := make([]string, 0, len(b.packed))
	for path := range b.packed {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	aliases := map[string][]string{}
	for _, path := range paths {
		aliases[b.packed[path].URI] = append(aliases[b.packed[path].URI], path)
	}
	decisions := make([]AssetDecision, 0, len(paths))
	remaining := snapshot.Len()
	for _, path := range paths {
		a := b.packed[path]
		if aliases[a.URI][0] != path {
			continue
		}
		count := counts[sha256.Sum256([]byte(a.URI))]
		if count == 0 {
			for _, source := range aliases[a.URI] {
				if strings.HasPrefix(b.references[source], "data:") {
					delete(b.references, source)
				}
			}
			continue
		} // esbuild removed an unused import.
		rel, _ := filepath.Rel(b.root, path)
		d := AssetDecision{Source: filepath.ToSlash(rel), Mode: "inline", Encoding: a.Encoding, Bytes: len(a.Bytes), Occurrences: count, Contribution: count * len(a.URI), HTMLContribution: htmlCounts[sha256.Sum256([]byte(a.URI))] * len(a.URI), Reason: "small asset; inline bias"}
		mode := b.mode(path)
		role := b.roles[path]
		for _, other := range aliases[a.URI] {
			if b.mode(other) == "inline" {
				mode = "inline"
			}
			r := b.roles[other]
			role = assetRole{role.Responsive || r.Responsive, role.Deferred || r.Deferred, role.Media || r.Media, role.Download || r.Download}
			if other != path {
				relative, _ := filepath.Rel(b.root, other)
				d.Aliases = append(d.Aliases, filepath.ToSlash(relative))
			}
		}
		if mode == "inline" {
			d.Reason = "explicit inline policy"
		}
		d.Pinned = mode == "inline"
		if mode == "auto" {
			switch {
			case role.Responsive:
				d.Mode, d.Reason = "file", "native responsive selection"
			case role.Download:
				d.Mode, d.Reason = "file", "native download URL"
			case role.Media || strings.HasPrefix(httpMIME(a.Bytes, path), "video/") || strings.HasPrefix(httpMIME(a.Bytes, path), "audio/"):
				d.Mode, d.Reason = "file", "independent media loading/seeking"
			case (fontExtension(path) || inspectedFont(a.Bytes)) && len(a.Bytes) > deferredAssetThreshold:
				d.Mode, d.Reason = "file", "font selection/cache avoids initial HTML bytes"
			case role.Deferred && len(a.Bytes) > deferredAssetThreshold:
				d.Mode, d.Reason = "file", "lazy/dormant media can transfer later"
			case d.Contribution > check.LargeEmbed:
				d.Mode, d.Reason = "file", "encoded contribution exceeds 32 KiB"
			}
		}
		if d.Mode == "file" {
			remaining -= d.HTMLContribution
		}
		decisions = append(decisions, d)
	}
	// Greedy largest contribution first minimizes the number of extra requests
	// needed to approach the whole-document target. Ties use stable source paths.
	order := make([]int, len(decisions))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return decisions[order[i]].HTMLContribution > decisions[order[j]].HTMLContribution
	})
	for _, i := range order {
		d := &decisions[i]
		if remaining <= embeddedDocumentTarget {
			break
		}
		path := filepath.Join(b.root, filepath.FromSlash(d.Source))
		if d.Mode == "inline" && !d.Pinned && b.mode(path) == "auto" && d.HTMLContribution > 1024 {
			d.Mode, d.Reason = "file", "whole HTML exceeds 256 KiB packing target"
			remaining -= d.HTMLContribution
		}
	}
	var pairs []string
	seen := map[string]string{}
	for i := range decisions {
		d := &decisions[i]
		if d.Mode != "file" {
			continue
		}
		path := filepath.Join(b.root, filepath.FromSlash(d.Source))
		a := b.packed[path]
		name := b.resourceName(path)
		name = b.autoAssetName(name, path, a.Bytes)
		if err := b.emit(name, path, a.Bytes); err != nil {
			return nil, err
		}
		d.Output = name
		d.Encoding = ""
		// esbuild may have emitted an unquoted CSS url(data:...). The replacement
		// must remain safe there as well as inside JS strings and HTML attributes.
		ref := strings.NewReplacer("(", "%28", ")", "%29", "'", "%27").Replace(fileURL(name))
		for _, source := range append([]string{d.Source}, d.Aliases...) {
			b.references[filepath.Join(b.root, filepath.FromSlash(source))] = ref
		}
		// Identical bytes in distinct sources share the same inline token. Pick
		// one emitted URL deterministically; avoid duplicate replacement keys.
		if _, exists := seen[a.URI]; !exists {
			seen[a.URI] = ref
			pairs = append(pairs, a.URI, ref)
		}
	}
	if len(pairs) == 0 {
		return decisions, nil
	}
	replace := strings.NewReplacer(pairs...)
	htmlx.Walk(doc, func(n *html.Node) {
		for i := range n.Attr {
			if resourceAttribute(n, n.Attr[i].Key) {
				n.Attr[i].Val = replace.Replace(n.Attr[i].Val)
			}
		}
		// Only compiled executable code/CSS contains generated resource tokens;
		// inert JSON and visible/copy text are deliberately untouched.
		if n.Data == "style" || n.Data == "script" && executableScript(n) {
			htmlx.SetText(n, replace.Replace(htmlx.RawText(n)))
		}
	})
	for key, f := range b.files {
		if !generatedCode(f) {
			continue
		}
		changed := []byte(replace.Replace(string(f.contents)))
		if !bytes.Equal(changed, f.contents) {
			f.contents = changed
			b.files[key] = f
		}
	}
	return decisions, nil
}

func inspectedFont(raw []byte) bool {
	if fontcheck.Kind(raw) == "" {
		return false
	}
	i := fontcheck.Inspect(raw)
	return i.Issue == "" && i.Unknown == ""
}

// Auto resources use one source-derived name in HTML and esbuild's file loader.
// This prevents basename collisions BEFORE esbuild reconciles imported assets,
// and stays stable when content changes. Explicit file policy retains basenames.
func (b *builder) resourceName(path string) string {
	name := filepath.Base(path)
	if b.mode(path) != "auto" {
		return name
	}
	rel, _ := filepath.Rel(b.root, path)
	hash := sha256.Sum256([]byte(filepath.ToSlash(rel)))
	ext := filepath.Ext(name)
	return fmt.Sprintf("%s-%x%s", strings.TrimSuffix(name, ext), hash[:6], ext)
}

func (b *builder) autoAssetName(name, path string, raw []byte) string {
	occupied := func(name string) bool {
		if strings.EqualFold(name, "index.html") {
			return true
		}
		old, ok := b.files[strings.ToLower(name)]
		return ok && old.origin != path
	}
	if !occupied(name) {
		return name
	}
	hash := sha256.Sum256(raw)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	short := fmt.Sprintf("%s-%x%s", stem, hash[:6], ext)
	if !occupied(short) {
		return short
	}
	return fmt.Sprintf("%s-%x%s", stem, hash, ext)
}

func resourceAttribute(n *html.Node, key string) bool {
	switch key {
	case "src", "href", "srcset", "imagesrcset", "poster", "style":
		return true
	case "data":
		return n.Data == "object"
	}
	return false
}

func generatedCode(f emitted) bool {
	return strings.Contains(f.origin, "style-bundle:") || strings.Contains(f.origin, "script-bundle:")
}
func executableScript(n *html.Node) bool {
	switch strings.ToLower(strings.TrimSpace(htmlx.Attr(n, "type"))) {
	case "", "module", "text/javascript", "application/javascript":
		return true
	}
	return false
}
