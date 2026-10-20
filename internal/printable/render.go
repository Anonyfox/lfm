package printable

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"text/template/parse"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// View is what printable.html.tmpl sees for one edition.
type View struct {
	Edition  Edition
	Name     string
	Promise  string
	Parts    []Part
	Language string
	Shop     string
	Version  string
	Data     map[string]any // data/<name>.json as .Data.<name>
}

// Page is one rendered page as the checks see it.
type Page struct {
	N        int
	Part     string
	Role     string
	IDs      []string
	Links    []string // internal targets, without "#"
	External []string
	Images   []string // as written in src
}

// Rendered is one edition rendered to its print document.
type Rendered struct {
	Edition Edition
	Doc     []byte
	Pages   []Page
	Links   int // internal links, the hyperlink count buyers are told
	Stray   int // elements outside a page section
}

// memo keeps each generator result by its arguments: the generators are
// deterministic, and a template often asks for the same calendar on every
// page and for the same puzzles in every edition.
type memo struct {
	mu   sync.Mutex
	done map[string]any
}

func remember[T any](m *memo, key string, make func() (T, error)) (T, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.done[key]; ok {
		return v.(T), nil
	}
	v, err := make()
	if err == nil {
		m.done[key] = v
	}
	return v, err
}

func funcs(language string) template.FuncMap {
	m := &memo{done: map[string]any{}}
	return template.FuncMap{
		"calendar": func(from, start string, months ...int) (Calendar, error) {
			n := 0
			if len(months) > 0 {
				n = months[0]
			}
			return remember(m, fmt.Sprint("calendar", from, start, n), func() (Calendar, error) { return NewCalendar(from, start, n, language) })
		},
		"bingo": func(items any, count, size int, free bool, seed int) (Bingo, error) {
			list := strs(items)
			return remember(m, fmt.Sprint("bingo", list, count, size, free, seed), func() (Bingo, error) { return NewBingo(list, count, size, free, seed) })
		},
		"wordsearch": func(words any, size int, difficulty string, seed int) (WordSearch, error) {
			list := strs(words)
			return remember(m, fmt.Sprint("wordsearch", list, size, difficulty, seed), func() (WordSearch, error) {
				return NewWordSearch(list, size, difficulty, language, seed)
			})
		},
		"sudoku": func(difficulty string, seed int) (Sudoku, error) {
			return remember(m, fmt.Sprint("sudoku", difficulty, seed), func() (Sudoku, error) { return NewSudoku(difficulty, seed) })
		},
		"maze": func(width, height, seed int) (Maze, error) {
			return remember(m, fmt.Sprint("maze", width, height, seed), func() (Maze, error) { return NewMaze(width, height, seed) })
		},
		"seq": func(n int) ([]int, error) {
			if n > 10000 {
				return nil, fmt.Errorf("seq: %d; use up to 10000", n)
			}
			out := make([]int, max(0, n))
			for i := range out {
				out[i] = i + 1
			}
			return out, nil
		},
		"chunk": func(list any, n int) ([][]any, error) {
			v := reflect.ValueOf(list)
			if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
				return nil, fmt.Errorf("chunk: %T is not a list", list)
			}
			if n < 1 {
				return nil, fmt.Errorf("chunk: size %d", n)
			}
			var out [][]any
			for i := 0; i < v.Len(); i += n {
				var group []any
				for j := i; j < min(i+n, v.Len()); j++ {
					group = append(group, v.Index(j).Interface())
				}
				out = append(out, group)
			}
			return out, nil
		},
		"dict": func(pairs ...any) (map[string]any, error) {
			if len(pairs)%2 != 0 {
				return nil, fmt.Errorf("dict: odd number of arguments")
			}
			m := map[string]any{}
			for i := 0; i < len(pairs); i += 2 {
				k, ok := pairs[i].(string)
				if !ok {
					return nil, fmt.Errorf("dict: key %v is not a string", pairs[i])
				}
				m[k] = pairs[i+1]
			}
			return m, nil
		},
		"add": func(a, b int) (int, error) { return bounded("add", a+b) },
		"sub": func(a, b int) (int, error) { return bounded("sub", a-b) },
		"mul": func(a, b int) (int, error) { return bounded("mul", a*b) },
		"mod": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a % b
		},
	}
}

// strs turns a JSON list or a []string into strings.
func strs(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, it := range x {
			out = append(out, fmt.Sprint(it))
		}
		return out
	case string:
		return strings.Split(x, ",")
	}
	return nil
}

// Templates cannot be stopped once they run, so what they can ask for is
// bounded: a loop over a number takes at most loops rounds, arithmetic
// stays within numbers, and a render writes at most rendered bytes.
const (
	loops    = 10000
	numbers  = 100000
	rendered = 64 << 20
)

// renderTime is how long one edition's render may take; real ones take
// milliseconds.
var renderTime = 10 * time.Second

func bounded(name string, n int) (int, error) {
	if n > numbers || n < -numbers {
		return 0, fmt.Errorf("%s: %d is beyond ±%d", name, n, numbers)
	}
	return n, nil
}

// parseTemplate parses the template once; it renders every edition.
func parseTemplate(src []byte, language string) (*template.Template, error) {
	t, err := template.New(TemplateName).Funcs(funcs(language)).Option("missingkey=error").Parse(string(src))
	if err != nil {
		return nil, err
	}
	var tree *parse.Tree
	var check func(parse.Node) error
	check = func(n parse.Node) error {
		switch n := n.(type) {
		case *parse.ListNode:
			if n == nil {
				return nil
			}
			for _, c := range n.Nodes {
				if err := check(c); err != nil {
					return err
				}
			}
		case *parse.RangeNode:
			if p := n.Pipe; p != nil && len(p.Cmds) == 1 && len(p.Cmds[0].Args) == 1 {
				if num, ok := p.Cmds[0].Args[0].(*parse.NumberNode); ok && num.IsInt && num.Int64 > loops {
					at, _ := tree.ErrorContext(n)
					return fmt.Errorf("%s: range over %d; use up to %d", at, num.Int64, loops)
				}
			}
			return errors.Join(check(n.List), check(n.ElseList))
		case *parse.IfNode:
			return errors.Join(check(n.List), check(n.ElseList))
		case *parse.WithNode:
			return errors.Join(check(n.List), check(n.ElseList))
		}
		return nil
	}
	for _, d := range t.Templates() {
		if tree = d.Tree; tree != nil {
			if err := check(tree.Root); err != nil {
				return nil, err
			}
		}
	}
	return t, nil
}

// limited stops a render that writes more than its limit, or that was
// given up on.
type limited struct {
	bytes.Buffer
	left    int
	stopped atomic.Bool
}

func (c *limited) Write(p []byte) (int, error) {
	if c.stopped.Load() {
		return 0, errors.New("the render was given up on")
	}
	if len(p) > c.left {
		return 0, fmt.Errorf("the template writes more than %d MB", rendered>>20)
	}
	c.left -= len(p)
	return c.Buffer.Write(p)
}

var templateLine = regexp.MustCompile(`printable\.html\.tmpl:(\d+)`)

// errorLine finds the template line an error names, or 1.
func errorLine(err error) int {
	if m := templateLine.FindStringSubmatch(err.Error()); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 1
}

// baseCSS fixes every page to the edition's size, the safe margin and the
// Look's fonts and accent; printable.css builds on it.
func baseCSS(plan Plan, e Edition) string {
	accent := plan.Accent
	if accent == "" {
		accent = "#222222"
	}
	return fmt.Sprintf(`@page { size: %[1]s %[2]s; margin: 0; }
:root { --page-width: %[1]s; --page-height: %[2]s; --safe: %[3]s; --accent: %[4]s; --body-font: %[5]s; --heading-font: %[6]s; }
*, *::before, *::after { box-sizing: border-box; }
html, body { margin: 0; padding: 0; }
body { font-family: var(--body-font); -webkit-print-color-adjust: exact; print-color-adjust: exact; }
h1, h2, h3, h4, h5, h6 { font-family: var(--heading-font); }
.page { position: relative; width: var(--page-width); height: var(--page-height); overflow: hidden; padding: var(--safe); break-after: page; }
.page:last-child { break-after: auto; }
a { color: inherit; text-decoration: none; }
`, mm(e.Size.Width), mm(e.Size.Height), mm(Safe), accent, cssList(plan.Body), cssList(plan.Headings))
}

func cssList(s string) string {
	if s == "" {
		return "system-ui, sans-serif"
	}
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(";{}<>", r) {
			return -1
		}
		return r
	}, s)
}

// flat is the name an asset is served under while printing.
func flat(rel string) string { return strings.ReplaceAll(path.Clean(rel), "/", "-") }

var cssURL = regexp.MustCompile(`url\(\s*(['"]?)([^'")]+)(['"]?)\s*\)`)

// render runs the template for one edition and turns its output into the
// print document: lfm's page CSS, the product's CSS with flat asset names,
// page numbers filled in, and the pages described for the checks.
func render(t *template.Template, view View, plan Plan, style string) (Rendered, error) {
	body := &limited{left: rendered}
	// A template cannot be stopped from outside; one that runs past
	// renderTime is given up on, and stops at its next write.
	done := make(chan error, 1)
	go func() { done <- t.Execute(body, view) }()
	select {
	case err := <-done:
		if err != nil {
			return Rendered{}, err
		}
	case <-time.After(renderTime):
		body.stopped.Store(true)
		return Rendered{}, fmt.Errorf("the template did not finish within %s; check its loops", renderTime)
	}
	nodes, err := html.ParseFragment(&body.Buffer, &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return Rendered{}, err
	}
	r := Rendered{Edition: view.Edition}
	ids := map[string]int{} // id -> page number
	var pages []*html.Node
	for _, n := range nodes {
		switch {
		case n.Type == html.ElementNode && n.DataAtom == atom.Section && hasClass(n, "page"):
			pages = append(pages, n)
		case n.Type == html.ElementNode, n.Type == html.TextNode && strings.TrimSpace(n.Data) != "":
			r.Stray++
		}
	}
	for i, n := range pages {
		pg := Page{N: i + 1, Part: attr(n, "data-part"), Role: attr(n, "data-role")}
		walk(n, func(e *html.Node) {
			if id := attr(e, "id"); id != "" {
				pg.IDs = append(pg.IDs, id)
				if _, dup := ids[id]; !dup {
					ids[id] = pg.N
				}
			}
			switch e.DataAtom {
			case atom.A:
				href := attr(e, "href")
				switch {
				case strings.HasPrefix(href, "#") && len(href) > 1:
					pg.Links = append(pg.Links, href[1:])
				case strings.HasPrefix(href, "http://"), strings.HasPrefix(href, "https://"), strings.HasPrefix(href, "mailto:"):
					pg.External = append(pg.External, href)
				}
			case atom.Img:
				if src := attr(e, "src"); src != "" && !strings.HasPrefix(src, "data:") {
					pg.Images = append(pg.Images, src)
					setAttr(e, "src", flat(src))
				}
			}
		})
		r.Pages = append(r.Pages, pg)
		r.Links += len(pg.Links)
	}
	// Page numbers: .page-number and .page-count, and a .page-ref gets the
	// number of the page its data-ref (or href) points to, so contents pages
	// never drift.
	for i, n := range pages {
		walk(n, func(e *html.Node) {
			switch {
			case hasClass(e, "page-number"):
				setText(e, strconv.Itoa(i+1))
			case hasClass(e, "page-count"):
				setText(e, strconv.Itoa(len(pages)))
			case hasClass(e, "page-ref"):
				ref := attr(e, "data-ref")
				if ref == "" {
					ref = attr(e, "href")
				}
				if target, ok := ids[strings.TrimPrefix(ref, "#")]; ok {
					setText(e, strconv.Itoa(target))
				}
			}
		})
	}
	css := cssURL.ReplaceAllStringFunc(style, func(m string) string {
		parts := cssURL.FindStringSubmatch(m)
		ref := parts[2]
		if strings.Contains(ref, ":") || strings.HasPrefix(ref, "#") {
			return m
		}
		return "url(" + flat(ref) + ")"
	})
	var doc bytes.Buffer
	fmt.Fprintf(&doc, "<!doctype html>\n<html lang=\"%s\" class=\"%s\"><head><meta charset=\"utf-8\"><title>%s</title>\n<style>\n%s\n%s\n</style></head><body>\n",
		template.HTMLEscapeString(view.Language), template.HTMLEscapeString(editionClasses(view.Edition)), template.HTMLEscapeString(view.Name), baseCSS(plan, view.Edition), css)
	for _, n := range pages {
		if err := html.Render(&doc, n); err != nil {
			return Rendered{}, err
		}
		doc.WriteByte('\n')
	}
	doc.WriteString("</body></html>\n")
	r.Doc = doc.Bytes()
	return r, nil
}

func walk(n *html.Node, f func(*html.Node)) {
	if n.Type == html.ElementNode {
		f(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, f)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func setAttr(n *html.Node, key, val string) {
	for i, a := range n.Attr {
		if a.Key == key {
			n.Attr[i].Val = val
			return
		}
	}
}

func hasClass(n *html.Node, class string) bool {
	return n.Type == html.ElementNode && strings.Contains(" "+attr(n, "class")+" ", " "+class+" ")
}

func setText(n *html.Node, text string) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		n.RemoveChild(c)
		c = next
	}
	n.AppendChild(&html.Node{Type: html.TextNode, Data: text})
}

// loadData decodes data/*.json for .Data.
func loadData(files map[string][]byte) (map[string]any, error) {
	out := map[string]any{}
	for name, raw := range files {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, fmt.Errorf("%s/%s: %w", DataDir, name, err)
		}
		out[strings.TrimSuffix(name, ".json")] = v
	}
	return out, nil
}

// editionClasses names the edition on <html> for CSS: the size key and every
// option value, such as "a4 ink-saver monday".
func editionClasses(e Edition) string {
	classes := []string{e.Size.Key}
	for _, v := range e.Options {
		classes = append(classes, v)
	}
	sort.Strings(classes[1:])
	return strings.Join(classes, " ")
}
