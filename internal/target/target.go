// Package target reads a built page and its embedded resources for page checks.
package target

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/fontcheck"
	"github.com/Anonyfox/lfm/internal/htmlx"
	"github.com/Anonyfox/lfm/internal/imaging"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/urlcheck"
)

const (
	pageLimit      = 8 << 20
	fileLimit      = 8 << 20
	imageLimit     = 512 << 10
	totalLimit     = 32 << 20
	fileCount      = 256
	parallelism    = 8
	requestTimeout = 10 * time.Second
	runTimeout     = 30 * time.Second
)

type limits struct {
	page, file, image, total, count, parallel int
	request, run                              time.Duration
}

var defaultLimits = limits{pageLimit, fileLimit, imageLimit, totalLimit, fileCount, parallelism, requestTimeout, runTimeout}

// ErrUsage identifies unsupported targets and source project directories.
var ErrUsage = errors.New("invalid check target")

// File records inspected bytes; a truncated hash covers only the bytes read.
type File struct {
	Name      string `json:"name"`
	Bytes     int    `json:"bytes"`
	SHA256    string `json:"sha256"`
	Status    int    `json:"status"`
	Error     string `json:"error,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

// Result contains metadata and the inputs shared by static and browser checks.
type Result struct {
	URL       string          `json:"url"`
	Status    int             `json:"status,omitempty"`
	Redirects []string        `json:"redirects"`
	Canonical string          `json:"canonical"`
	Identity  string          `json:"identity"`
	Lang      string          `json:"lang"`
	Files     []File          `json:"files"`
	Page      []byte          `json:"-"`
	Options   check.Options   `json:"-"`
	Browser   release.Target  `json:"-"`
	Document  *check.Document `json:"-"`
}

type resource struct {
	u         *url.URL
	raw, kind string
	depth     int
}
type fetched struct {
	File
	body      []byte
	final     *url.URL
	redirects []string
	header    http.Header
}
type reader struct {
	ctx     context.Context
	client  *http.Client
	root    *os.Root
	remote  bool
	origin  *url.URL
	lim     limits
	version string
}

// Read performs GETs for URLs, or confined disk reads for local targets.
func Read(ctx context.Context, argument, version string) (Result, error) {
	return read(ctx, argument, version, defaultLimits)
}

func read(ctx context.Context, argument, version string, lim limits) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, lim.run)
	defer cancel()
	r := &reader{ctx: ctx, lim: lim, version: version}
	result := Result{Redirects: []string{}, Files: []File{}}
	var u *url.URL
	var err error
	if urlcheck.Scheme(argument) != "" {
		u, err = url.Parse(argument)
		if err != nil {
			return result, fmt.Errorf("%w: %v", ErrUsage, err)
		}
	} else {
		u = &url.URL{}
	}
	var pageName string
	if u.Scheme != "" {
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return result, fmt.Errorf("%w: use an http(s) URL, folder or .html file", ErrUsage)
		}
		u.User = nil
		u.Fragment = ""
		r.remote = true
		transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext, ForceAttemptHTTP2: true, MaxResponseHeaderBytes: 1 << 20, MaxIdleConns: 16, MaxIdleConnsPerHost: 8, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: lim.request}
		defer transport.CloseIdleConnections()
		r.client = &http.Client{Transport: transport, Timeout: lim.request}
	} else {
		info, statErr := os.Stat(argument)
		if statErr != nil {
			return result, fmt.Errorf("read page: %w", statErr)
		}
		dir := argument
		pageName = "index.html"
		if info.IsDir() {
			if _, err := os.Stat(filepath.Join(dir, "lfm.json")); err == nil {
				return result, fmt.Errorf("%w: this is an lfm project: run 'lfm compile', or check its 'output/' folder", ErrUsage)
			}
		} else {
			if !strings.EqualFold(filepath.Ext(argument), ".html") {
				return result, fmt.Errorf("%w: local files must have a .html extension", ErrUsage)
			}
			dir = filepath.Dir(argument)
			pageName = filepath.Base(argument)
		}
		r.root, err = os.OpenRoot(dir)
		if err != nil {
			return result, err
		}
		defer r.root.Close()
		u = &url.URL{Scheme: "https", Host: "local.invalid", Path: "/" + pageName}
	}
	page := r.fetch(resource{u: u, kind: "page"}, lim.page)
	if page.Error != "" {
		return result, fmt.Errorf("read page %s: %s", argument, page.Error)
	}
	if page.Truncated {
		return result, fmt.Errorf("read page %s: page exceeds %d bytes", argument, lim.page)
	}
	if r.remote && !isHTML(page.header.Get("Content-Type"), page.body) {
		return result, fmt.Errorf("read page %s: response is not HTML", argument)
	}
	result.Page = page.body
	result.Status = page.Status
	result.Redirects = page.redirects
	if result.Redirects == nil {
		result.Redirects = []string{}
	}
	base := page.final
	result.URL = argument
	if r.remote {
		result.URL = base.String()
	}
	doc, err := check.Parse(page.body, check.Options{})
	if err != nil {
		return result, err
	}
	for _, n := range doc.Elements {
		if n.Namespace != "" || htmlx.Dormant(n) {
			continue
		}
		if n.Data == "html" {
			result.Lang = check.Attr(n, "lang")
		}
		if n.Data == "link" && hasWord(check.Attr(n, "rel"), "canonical") && result.Canonical == "" {
			c, e := url.Parse(check.Attr(n, "href"))
			if e == nil {
				c = base.ResolveReference(c)
				if (c.Scheme == "http" || c.Scheme == "https") && c.Host != "" {
					c.User = nil
					result.Canonical = c.String()
				}
			}
		}
	}
	result.Identity = result.Canonical
	if result.Identity == "" && r.remote {
		result.Identity = result.URL
	}
	r.origin = base
	if !r.remote && result.Canonical != "" {
		r.origin, _ = url.Parse(result.Canonical)
	}
	opts := check.Options{URL: result.Identity, File: strings.TrimPrefix(base.Path, "/"), RobotsHeader: page.header.Values("X-Robots-Tag"), Stylesheets: map[string][]byte{}, FontAssets: map[string]fontcheck.Info{}, Assets: map[string][]byte{}}
	if opts.File == "" {
		opts.File = "index.html"
	}
	result.Files = append(result.Files, page.File)
	all := map[string]fetched{u.String(): page}
	seen := map[string]bool{u.String(): true, base.String(): true}
	var queue, bindings []resource
	add := func(raw, kind string, base *url.URL, depth int) {
		v, e := url.Parse(strings.TrimSpace(raw))
		if e != nil || raw == "" || strings.HasPrefix(raw, "#") {
			return
		}
		if v.Scheme != "" && v.Scheme != "http" && v.Scheme != "https" {
			return
		}
		// Validate before URL resolution can erase traversal segments.
		if !r.remote && !confinedPath(base.Path, v.Path) {
			return
		}
		v = base.ResolveReference(v)
		v.User = nil
		v.Fragment = ""
		key := v.String()
		bindings = append(bindings, resource{u: v, raw: raw, kind: kind, depth: depth})
		if seen[key] {
			return
		}
		seen[key] = true
		queue = append(queue, resource{u: v, raw: raw, kind: kind, depth: depth})
	}
	kinds := map[string]string{}
	for _, n := range doc.Elements {
		switch n.Data {
		case "link":
			if hasWord(check.Attr(n, "rel"), "stylesheet") {
				kinds[check.Attr(n, "href")] = "css"
			}
		case "script":
			kinds[check.Attr(n, "src")] = "js"
		}
	}
	for _, ref := range urlcheck.AllReferences(page.body) {
		if !ref.Resource || ref.Tag == "video" || ref.Tag == "audio" || ref.Tag == "source" || ref.Tag == "iframe" || ref.Attribute == "srcset" || ref.Attribute == "imagesrcset" {
			continue
		}
		kind := "asset"
		if ref.Tag == "img" || ref.Tag == "meta" || ref.Attribute == "poster" {
			kind = "image"
		}
		if ref.Tag == "script" {
			kind = "js"
		}
		switch strings.ToLower(path.Ext(strings.SplitN(ref.Value, "?", 2)[0])) {
		case ".css":
			kind = "css"
		case ".js", ".mjs":
			kind = "js"
		case ".woff", ".woff2", ".ttf", ".otf":
			kind = "font"
		case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".svg", ".ico":
			kind = "image"
		}
		if declared := kinds[ref.Value]; declared != "" {
			kind = declared
		}
		add(ref.Value, kind, base, 0)
	}
	// Inline font faces use the page URL as their base.
	for _, n := range doc.Elements {
		if n.Data == "style" {
			faces, _ := fontcheck.ReadCSS([]byte(check.Text(n)))
			for _, f := range faces.Faces {
				for _, src := range f.Sources {
					if !src.Local {
						add(src.URL, "font", base, 0)
					}
				}
			}
		}
	}
	total := len(page.body)
	artifacts := map[string][]byte{}
	for start := 0; start < len(queue); {
		end := start
		reserved := 0
		for end < len(queue) && end-start < lim.parallel {
			cap := lim.file
			if queue[end].kind == "image" {
				cap = lim.image
			}
			if end > start && reserved+cap > lim.total-total {
				break
			}
			reserved += cap
			end++
		}
		batch := queue[start:end]
		results := make([]fetched, len(batch))
		jobs := make(chan int)
		var wg sync.WaitGroup
		// Allocate deterministic byte allowances before launching concurrent reads.
		allowances := make([]int, len(batch))
		remaining := max(0, lim.total-total)
		for i, item := range batch {
			cap := lim.file
			if item.kind == "image" {
				cap = lim.image
			}
			if start+i+1 >= lim.count {
				continue
			}
			allowances[i] = min(cap, remaining)
			remaining -= allowances[i]
		}
		for w := 0; w < min(lim.parallel, len(batch)); w++ {
			wg.Go(func() {
				for i := range jobs {
					if allowances[i] == 0 {
						results[i] = fetched{File: File{Name: r.name(batch[i].u), Error: "not inspected: file count or total byte limit reached"}}
						continue
					}
					results[i] = r.fetch(batch[i], allowances[i])
				}
			})
		}
		for i := range batch {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		for i, f := range results {
			item := batch[i]
			result.Files = append(result.Files, f.File)
			all[item.u.String()] = f
			total += len(f.body)
			if f.Error != "" {
				continue
			}
			name := strings.TrimPrefix(item.u.Path, "/")
			if !f.Truncated {
				artifacts[name] = f.body
			}
			if f.Truncated {
				if item.kind != "image" {
					continue
				}
				info := imaging.Inspect(f.body)
				if info.Issue != "" || info.Unknown != "" {
					continue
				}
			}
			// Sharing resolves within the page identity; other origins retain
			// absolute keys for file-content checks without path collisions.
			identity, _ := url.Parse(result.Identity)
			key := item.u.String()
			if sameOrigin(item.u, base) || sameOrigin(item.u, identity) {
				key = name
				if identity != nil {
					directory := path.Dir(identity.Path) + "/"
					if directory == "//" {
						directory = "/"
					}
					if strings.HasPrefix(item.u.Path, directory) {
						key = strings.TrimPrefix(item.u.Path, directory)
					}
				}
			}
			opts.Assets[key] = f.body
			switch item.kind {
			case "css":
				opts.Stylesheets[item.raw] = f.body
				opts.ExternalCSSBytes += f.Bytes
				faces, _ := fontcheck.ReadCSS(f.body)
				for _, face := range faces.Faces {
					for _, src := range face.Sources {
						if !src.Local {
							add(src.URL, "font", f.final, item.depth)
						}
					}
				}
				// CSS URL discovery uses the existing tokenizer, including string @imports.
				if item.depth == 0 {
					for _, ref := range urlcheck.CSSReferences(f.body, 0) {
						if ref.Import {
							add(ref.Value, "css", f.final, 1)
						}
					}
				}
			case "js":
				opts.ExternalJSBytes += f.Bytes
			case "font":
				info := fontcheck.Inspect(f.body)
				info.Source = item.raw
				opts.FontAssets[item.raw] = info
			}
		}
		start = end
	}
	// Preserve img order, including entries whose bytes could not be inspected.
	for _, n := range doc.Elements {
		if n.Data != "img" {
			continue
		}
		raw := check.Attr(n, "src")
		v, e := url.Parse(raw)
		var info imaging.Info
		if e == nil {
			v = base.ResolveReference(v)
			v.User = nil
			v.Fragment = ""
			if f, ok := all[v.String()]; ok && f.Error == "" {
				info = imaging.Inspect(f.body)
				info.Bytes = f.Bytes
			}
		}
		info.Source = raw
		opts.Images = append(opts.Images, imaging.Decision{Info: info})
	}
	// Dedupe aliases while retaining their raw stylesheet/font lookup keys.
	for _, item := range bindings {
		f := all[item.u.String()]
		if f.Error != "" || f.Truncated {
			continue
		}
		if item.kind == "css" {
			opts.Stylesheets[item.raw] = f.body
		}
		if item.kind == "font" {
			info := fontcheck.Inspect(f.body)
			info.Source = item.raw
			opts.FontAssets[item.raw] = info
		}
	}
	result.Options = opts
	doc.Options = opts
	doc.Stats.ExternalCSSBytes = opts.ExternalCSSBytes
	doc.Stats.ExternalJSBytes = opts.ExternalJSBytes
	result.Document = doc
	result.Browser = release.Target{Page: page.body, Assets: artifacts}
	if r.remote {
		result.Browser = release.Target{URL: result.URL}
	}
	// The page stays first; sidecars have a stable order independent of latency.
	sort.SliceStable(result.Files[1:], func(i, j int) bool { return result.Files[i+1].Name < result.Files[j+1].Name })
	return result, nil
}

func (r *reader) name(u *url.URL) string {
	if r.remote {
		return u.String()
	}
	return strings.TrimPrefix(u.Path, "/")
}
func (r *reader) fetch(item resource, cap int) fetched {
	f := fetched{File: File{Name: r.name(item.u)}, final: item.u}
	var body io.ReadCloser
	size := int64(-1)
	if err := r.ctx.Err(); err != nil {
		f.Error = "not inspected: " + err.Error()
		return f
	}
	if r.remote {
		client := *r.client
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			req.URL.User = nil
			req.Header.Del("Authorization")
			req.Header.Del("Cookie")
			if len(via) > 5 {
				return errors.New("more than 5 redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("redirect requires http(s)")
			}
			f.redirects = append(f.redirects, req.URL.String())
			return nil
		}
		ctx, cancel := context.WithTimeout(r.ctx, r.lim.request)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.u.String(), nil)
		if err != nil {
			f.Error = err.Error()
			return f
		}
		req.Header.Set("User-Agent", "lfm/"+r.version+" (+https://lfm.cash)")
		response, err := client.Do(req)
		if err != nil {
			f.Error = err.Error()
			return f
		}
		defer response.Body.Close()
		f.Status = response.StatusCode
		f.header = response.Header
		f.final = response.Request.URL
		if f.Status < 200 || f.Status >= 300 {
			f.Error = fmt.Sprintf("HTTP %d", f.Status)
			return f
		}
		body = response.Body
		size = response.ContentLength
	} else {
		if item.u.Host != "local.invalid" && (r.origin == nil || !sameOrigin(item.u, r.origin)) {
			f.Error = "not inspected: remote resource in folder mode"
			return f
		}
		name := strings.TrimPrefix(item.u.Path, "/")
		if unsafePath(name) {
			f.Error = "not inspected: unsafe resource path"
			return f
		}
		file, err := r.root.Open(name)
		if err != nil {
			f.Error = err.Error()
			return f
		}
		defer file.Close()
		stat, err := file.Stat()
		if err != nil {
			f.Error = err.Error()
			return f
		}
		if !stat.Mode().IsRegular() {
			f.Error = "not inspected: not a regular file"
			return f
		}
		size = stat.Size()
		body = file
	}
	raw, err := io.ReadAll(io.LimitReader(body, int64(cap)+1))
	if err != nil {
		f.Error = err.Error()
		return f
	}
	f.Truncated = len(raw) > cap
	raw = raw[:min(len(raw), cap)]
	f.body = raw
	f.Bytes = len(raw)
	if f.Truncated && size >= 0 {
		f.Bytes = int(min(size, int64(^uint(0)>>1)))
	}
	hash := sha256.Sum256(raw)
	f.SHA256 = fmt.Sprintf("%x", hash)
	return f
}
func unsafePath(value string) bool {
	if strings.ContainsAny(value, "\\\x00") {
		return true
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}
func hasWord(value, word string) bool {
	for _, v := range strings.Fields(value) {
		if strings.EqualFold(v, word) {
			return true
		}
	}
	return false
}
func isHTML(kind string, raw []byte) bool {
	media, _, _ := mime.ParseMediaType(kind)
	switch media {
	case "text/html", "application/xhtml+xml":
		return true
	case "", "application/octet-stream":
		return strings.HasPrefix(http.DetectContentType(raw), "text/html")
	default:
		return false
	}
}

func sameOrigin(a, b *url.URL) bool {
	return a != nil && b != nil && strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

// confinedPath allows parent segments only while they stay inside the read root.
// net/url has already decoded escapes; validate before it removes dot segments.
func confinedPath(base, reference string) bool {
	if strings.ContainsAny(reference, "\\\x00") {
		return false
	}
	depth := 0
	if !strings.HasPrefix(reference, "/") {
		for _, part := range strings.Split(path.Dir(base), "/") {
			if part != "" && part != "." {
				depth++
			}
		}
	}
	for _, part := range strings.Split(reference, "/") {
		switch part {
		case "", ".":
		case "..":
			depth--
			if depth < 0 {
				return false
			}
		default:
			depth++
		}
	}
	return true
}
