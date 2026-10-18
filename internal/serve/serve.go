// Package serve previews compiled pages and recompiles them on every change.
// Only HTTP responses receive the reload client; deployable output stays untouched.
package serve

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Anonyfox/lfm/internal/compile"
	"github.com/Anonyfox/lfm/internal/project"
	xhtml "golang.org/x/net/html"
)

type Options struct {
	Port         int
	ExplicitPort bool
	Open         bool
	Log          io.Writer
	Waiting      func() // another compile of the project holds its lock
	OnBuild      func(compile.Result, error)
}

type state struct {
	Version uint64            `json:"version"`
	Path    string            `json:"path"`
	Files   map[string][]byte `json:"-"`
	Problem string            `json:"-"`
}

type preview struct {
	mu          sync.Mutex
	current     state
	subscribers map[chan state]bool
	prefix      string
	host        string
	ctx         context.Context
}

func Serve(ctx context.Context, directory string, options Options) error {
	if options.Port < 0 || options.Port > 65535 {
		return fmt.Errorf("preview port must be between 0 and 65535")
	}
	p0, err := project.Open(directory)
	if err == nil {
		err = p0.Expect(directory)
	}
	if err != nil {
		return err
	}
	root := p0.Root
	log := options.Log
	if log == nil {
		log = io.Discard
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(options.Port)))
	if err != nil && !options.ExplicitPort {
		fmt.Fprintf(log, "port %d is busy, using a free one\n", options.Port)
		listener, err = net.Listen("tcp4", "127.0.0.1:0")
	}
	if err != nil {
		return fmt.Errorf("preview port %d: %w; choose another with --port", options.Port, err)
	}
	defer listener.Close()
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return err
	}
	p := &preview{subscribers: map[chan state]bool{}, prefix: "/__lfm_" + hex.EncodeToString(token), host: listener.Addr().String(), ctx: ctx, current: state{Path: "/"}}
	monitor := newMonitor(root)
	previous := monitor.snapshot()
	cspReported := false
	rebuild := func() {
		report, err := compile.Compile(ctx, root, compile.Options{Publish: true, Waiting: options.Waiting})
		if errors.Is(err, context.Canceled) {
			return
		}
		// Newly resolved package files join the baseline, so watching them
		// does not look like an edit on the next tick.
		for _, path := range monitor.include(report.Inputs) {
			if s, ok := monitor.stamp(path); ok {
				previous[path] = s
			}
		}
		if options.OnBuild != nil {
			options.OnBuild(report, err)
		}
		p.publish(report, err)
		if !cspReported && reloadBlocked(p.snapshot().Files["index.html"]) {
			fmt.Fprintln(log, "note: the page's Content-Security-Policy blocks live reload; refresh by hand. The policy itself is kept.")
			cspReported = true
		}
	}
	rebuild()
	server := &http.Server{Handler: p, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	stopped := make(chan error, 1)
	done := make(chan struct{})
	go func() { defer close(done); stopped <- server.Serve(listener) }()
	defer func() { server.Close(); <-done }()
	address := "http://" + p.host + p.snapshot().Path
	fmt.Fprintf(log, "serving %s, reloading on every change (Ctrl-C stops)\n\n", address)
	var opened <-chan error
	if options.Open {
		opened, err = openBrowser(address)
		if err != nil {
			fmt.Fprintf(log, "could not open a browser: %v; open %s yourself\n", err, address)
		}
	}
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	var pending bool
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-stopped:
			if err == http.ErrServerClosed {
				return nil
			}
			return err
		case err := <-opened:
			if err != nil {
				fmt.Fprintf(log, "the browser launcher failed: %v; open %s yourself\n", err, address)
			}
			opened = nil
		case <-ticker.C:
			current := monitor.snapshot()
			if changed(previous, current) {
				previous = current
				pending = true
				continue
			}
			if pending {
				pending = false
				rebuild()
			}
		}
	}
}

func (p *preview) snapshot() state { p.mu.Lock(); defer p.mu.Unlock(); return p.current }

// publish makes a compile result current. Browsers reload only when what they
// would show changed, so edits to notes or research never interrupt a preview.
func (p *preview) publish(report compile.Result, err error) {
	previous := p.snapshot()
	next := previous
	next.Problem = ""
	if err == nil {
		next.Files = report.Artifact
		if target, e := url.Parse(report.PageURL); e == nil && target.Path != "" {
			next.Path = target.EscapedPath()
		}
	}
	if err != nil {
		if report.Validation != nil {
			for _, issue := range report.Validation.Diagnostics {
				next.Problem += issue.String() + "\n\n"
			}
		}
		if next.Problem == "" {
			next.Problem = err.Error()
		}
	}
	if next.Problem == previous.Problem && next.Path == previous.Path && sameFiles(next.Files, previous.Files) && previous.Version > 0 {
		return
	}
	next.Version++
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current = next
	for subscriber := range p.subscribers {
		select {
		case subscriber <- next:
		default:
		}
	}
}

func (p *preview) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Serve generated names only; never expose source/config/scratch directories.
	if r.Host != p.host && r.Host != "localhost:"+strings.Split(p.host, ":")[1] {
		http.Error(w, "preview is local only", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "read-only preview", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case p.prefix + "/events":
		p.events(w, r)
		return
	case p.prefix + "/client.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		if r.Method != http.MethodHead {
			fmt.Fprintf(w, client, p.prefix)
		}
		return
	case p.prefix + "/error.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		if r.Method != http.MethodHead {
			io.WriteString(w, errorCSS)
		}
		return
	}
	current := p.snapshot()
	base, err := url.PathUnescape(current.Path)
	if err != nil {
		base = "/"
	}
	if r.URL.Path == "/" && base != "/" {
		http.Redirect(w, r, current.Path, http.StatusTemporaryRedirect)
		return
	}
	if r.URL.Path == strings.TrimSuffix(base, "/") && base != "/" {
		http.Redirect(w, r, current.Path, http.StatusTemporaryRedirect)
		return
	}
	if !strings.HasPrefix(r.URL.Path, base) {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, base)
	if name == "" || name == "index.html" {
		page := current.Files["index.html"]
		if current.Problem != "" {
			page = []byte(`<!doctype html><html lang=en><head><meta charset=utf-8><meta name=viewport content="width=device-width"><title>Build needs a fix</title><link rel=stylesheet href="` + p.prefix + `/error.css"></head><body><main><h1>Build needs a fix</h1><p>Last successful output is preserved. Fix the source below; this preview reloads automatically.</p><pre>` + html.EscapeString(current.Problem) + `</pre></main></body></html>`)
		}
		script := []byte(fmt.Sprintf(`<script src="%s/client.js?v=%d"></script>`, p.prefix, current.Version))
		// Generated HTML comes from the HTML5 serializer. Do not mutate output files.
		where := bytes.LastIndex(page, []byte("</body>"))
		if where < 0 {
			where = len(page)
		}
		rendered := make([]byte, 0, len(page)+len(script))
		rendered = append(rendered, page[:where]...)
		rendered = append(rendered, script...)
		rendered = append(rendered, page[where:]...)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.Method != http.MethodHead {
			w.Write(rendered)
		}
		return
	}
	// Deliverables live one folder deep, such as a book's unlisted download path.
	if strings.HasSuffix(name, "/") {
		name += "index.html"
	} else if _, ok := current.Files[name+"/index.html"]; ok {
		http.Redirect(w, r, r.URL.Path+"/", http.StatusTemporaryRedirect)
		return
	}
	if path.Clean(name) != name || strings.Contains(name, `\`) || strings.HasPrefix(name, "..") || strings.Count(name, "/") > 1 {
		http.NotFound(w, r)
		return
	}
	if content, ok := current.Files[name]; ok {
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(content))
		return
	}
	http.NotFound(w, r)
}

func (p *preview) events(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	flush, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	updates := make(chan state, 1)
	p.mu.Lock()
	p.subscribers[updates] = true
	initial := p.current
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.subscribers, updates); p.mu.Unlock() }()
	send := func(s state) bool {
		raw, _ := json.Marshal(s)
		_, err := fmt.Fprintf(w, "data: %s\n\n", raw)
		flush.Flush()
		return err == nil
	}
	io.WriteString(w, "retry: 1000\n\n")
	if !send(initial) {
		return
	}
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-p.ctx.Done():
			return
		case s := <-updates:
			if !send(s) {
				return
			}
		case <-heartbeat.C:
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			flush.Flush()
		}
	}
}

const client = `(()=>{const v=new URL(document.currentScript.src).searchParams.get('v'),key='lfm-scroll';try{const old=JSON.parse(sessionStorage.getItem(key)||'null');sessionStorage.removeItem(key);if(old&&old.path===location.pathname)addEventListener('load',()=>requestAnimationFrame(()=>scrollTo(old.x,old.y)),{once:true})}catch{}const events=new EventSource('%s/events');events.onmessage=e=>{const next=JSON.parse(e.data);if(String(next.version)!==v){try{sessionStorage.setItem(key,JSON.stringify({path:next.path,x:scrollX,y:scrollY}))}catch{}location.replace(next.path+location.hash)}}})();`
const errorCSS = `:root{color-scheme:dark}body{margin:0;background:#17191d;color:#f4f5f8;font:16px/1.5 system-ui,sans-serif}main{max-width:1000px;padding:32px;margin:auto}h1{color:#ff9c8e}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#22252b;padding:24px;border-radius:8px;font:14px/1.6 ui-monospace,monospace}`

// Report definite CSP exclusions; leave authored policy untouched. This is not a
// complete CSP evaluator (nonces, strict-dynamic and origin patterns can vary).
func reloadBlocked(page []byte) bool {
	z := xhtml.NewTokenizer(bytes.NewReader(page))
	for {
		kind := z.Next()
		if kind == xhtml.ErrorToken {
			return false
		}
		if kind != xhtml.StartTagToken && kind != xhtml.SelfClosingTagToken {
			continue
		}
		token := z.Token()
		if token.Data != "meta" {
			continue
		}
		var equivalent, content string
		for _, a := range token.Attr {
			if a.Key == "http-equiv" {
				equivalent = a.Val
			}
			if a.Key == "content" {
				content = a.Val
			}
		}
		if !strings.EqualFold(equivalent, "content-security-policy") {
			continue
		}
		policy := map[string][]string{}
		for _, part := range strings.Split(content, ";") {
			fields := strings.Fields(part)
			if len(fields) > 0 {
				key := strings.ToLower(fields[0])
				if _, exists := policy[key]; !exists {
					policy[key] = fields[1:]
				}
			}
		}
		for _, keys := range [][]string{{"script-src-elem", "script-src", "default-src"}, {"connect-src", "default-src"}} {
			for _, key := range keys {
				if values, exists := policy[key]; exists {
					if len(values) == 0 {
						return true
					}
					if len(values) == 1 && values[0] == "'none'" {
						return true
					}
					break
				}
			}
		}
	}
}

func sameFiles(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for name, content := range a {
		other, ok := b[name]
		if !ok || !bytes.Equal(content, other) {
			return false
		}
	}
	return true
}
