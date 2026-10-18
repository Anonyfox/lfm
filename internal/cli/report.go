package cli

import (
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Anonyfox/lfm/internal/book"
	"github.com/Anonyfox/lfm/internal/compile"
	"github.com/Anonyfox/lfm/internal/concept"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/release"
	"github.com/Anonyfox/lfm/internal/research"
	"github.com/Anonyfox/lfm/internal/term"
)

// renderDiagnostics prints findings most severe first:
//
//	warning  design.eyebrows  page <p>
//	         Small tracked uppercase label directly above a heading.
//	         fix: Delete the label or merge its information into the heading.
func renderDiagnostics(w io.Writer, s term.Style, issues []diagnostic.Diagnostic) {
	const indent = "         "
	for _, d := range issues {
		label := s.Bold(s.Yellow("warning"))
		if d.Severity == diagnostic.Error {
			label = s.Bold(s.Red("error  "))
		}
		header := label + "  " + s.Cyan(d.Rule)
		if at := where(d); at != "" {
			header += "  " + s.Dim(at)
		}
		fmt.Fprintln(w, header)
		message := d.Message
		if d.Path != "" && !strings.Contains(message, d.Path) {
			message = d.Path + ": " + message
		}
		fmt.Fprintln(w, indent+term.Wrap(message, term.Width-len(indent), indent))
		if d.Hint != "" {
			fmt.Fprintln(w, indent+s.Green("fix: ")+term.Wrap(d.Hint, term.Width-len(indent)-5, indent+"     "))
		}
		for _, r := range d.Related {
			fmt.Fprintln(w, indent+s.Dim("see "+position(r)))
		}
	}
}

// where names a finding's location in authoring terms. Positions inside the
// compiled page mean little to an author, so those show the element instead.
func where(d diagnostic.Diagnostic) string {
	element := ""
	if d.Element != "" {
		element = "<" + d.Element + ">"
	}
	switch d.Location.File {
	case "", "index.html":
		if element == "" {
			return "page"
		}
		return "page " + element
	case "rendered-template.html":
		return strings.TrimSpace("rendered template, line " + fmt.Sprint(d.Location.Line) + " " + element)
	}
	return strings.TrimSpace(position(d.Location) + " " + element)
}

func position(l diagnostic.Location) string {
	if l.Line == 0 {
		return l.File
	}
	return fmt.Sprintf("%s:%d:%d", l.File, l.Line, l.Column)
}

// renderResult prints a whole compile: findings, then what was produced.
func renderResult(w io.Writer, s term.Style, r compile.Result, err error) {
	if r.Validation != nil && len(r.Validation.Diagnostics) > 0 {
		defer fmt.Fprintln(w, s.Dim("'lfm rules <id>' explains any rule above."))
	}
	if r.Validation != nil {
		renderDiagnostics(w, s, r.Validation.Diagnostics)
		if len(r.Validation.Diagnostics) > 0 {
			fmt.Fprintln(w)
		}
	}
	name := filepath.Base(r.Root)
	if err != nil {
		counts := findings(r)
		if counts == "" {
			counts = "it could not finish"
		}
		fmt.Fprintf(w, "%s %s failed: %s; %s/ unchanged\n", s.Bold(s.Red("✗")), s.Bold(name), counts, "output")
		return
	}
	verb := "compiled"
	if r.Published {
		verb += " → output/"
	}
	line := fmt.Sprintf("%s %s %s in %s", s.Bold(s.Green("✓")), s.Bold(name), verb, term.Duration(r.Duration))
	if counts := findings(r); counts != "" {
		line += ", " + counts
	}
	fmt.Fprintln(w, line)
	renderFiles(w, s, r)
	renderBook(w, s, r.Book, r.Files)
	renderResearch(w, s, r.Research)
	renderConcept(w, s, r.Concept)
	renderRelease(w, s, r.Release)
}

func findings(r compile.Result) string {
	var parts []string
	if n := r.Errors(); n > 0 {
		parts = append(parts, term.Plural(n, "error"))
	}
	if n := r.Warnings(); n > 0 {
		parts = append(parts, term.Plural(n, "warning"))
	}
	return strings.Join(parts, ", ")
}

func renderFiles(w io.Writer, s term.Style, r compile.Result) {
	var page compile.File
	var assets []compile.File
	total := 0
	for _, f := range r.Files {
		if f.Name == "index.html" {
			page = f
			continue
		}
		if strings.Contains(f.Name, "/") {
			continue // deliverables, listed by their own line
		}
		assets = append(assets, f)
		total += f.Bytes
	}
	detail := ""
	if c := r.Cost; c != nil {
		detail = fmt.Sprintf("%s gzip · CSS %s · JS %s", term.Bytes(c.GzipBytes), term.Bytes(c.CSSBytes), term.Bytes(c.JSBytes))
	}
	fmt.Fprintf(w, "  %s %s  %s\n", term.Pad("index.html", 12), term.Pad(term.Bytes(page.Bytes), 10), s.Dim(detail))
	if len(assets) == 0 {
		return
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Bytes > assets[j].Bytes })
	var names []string
	for i, f := range assets {
		if i == 3 {
			names = append(names, fmt.Sprintf("+%d more", len(assets)-3))
			break
		}
		names = append(names, f.Name)
	}
	fmt.Fprintf(w, "  %s %s  %s\n", term.Pad(term.Plural(len(assets), "asset"), 12), term.Pad(term.Bytes(total), 10), s.Dim(strings.Join(names, ", ")))
	for _, external := range r.External {
		fmt.Fprintf(w, "  %s %s\n", term.Pad("external", 12), s.Dim(external))
	}
}

func renderRelease(w io.Writer, s term.Style, r *release.Result) {
	if r == nil {
		return
	}
	if r.Applied {
		fmt.Fprintf(w, "  %s critical CSS %s of %s blocks rendering; the rest loads after\n", term.Pad("release", 12), term.Bytes(r.CriticalBytes), term.Bytes(r.OriginalBytes))
	} else if r.Reason != "" {
		fmt.Fprintf(w, "  %s %s\n", term.Pad("release", 12), s.Dim(r.Reason))
	}
	if r.LazyImages > 0 || r.PriorityImage != "" {
		var parts []string
		if r.LazyImages > 0 {
			verb := " load lazily"
			if r.LazyImages == 1 {
				verb = " loads lazily"
			}
			parts = append(parts, term.Plural(r.LazyImages, "image")+" below the first screen"+verb)
		}
		if r.PriorityImage != "" {
			parts = append(parts, path.Base(r.PriorityImage)+" is fetched first")
		}
		fmt.Fprintf(w, "  %s %s\n", term.Pad("images", 12), strings.Join(parts, "; "))
	}
	if r.Audit == nil {
		return
	}
	if rt := r.Audit.Runtime; rt != nil && rt.Status == "ok" {
		fmt.Fprintf(w, "  %s %s, %s in %s\n", term.Pad("browser", 12), rt.Chrome, term.Plural(len(rt.Problems), "runtime problem"), fmt.Sprintf("%d ms", rt.DurationMS))
	}
	if rt := r.Audit.Runtime; rt != nil && rt.Layout != nil && rt.Layout.Count > 0 {
		l := rt.Layout
		line := term.Plural(l.Count, "screenshot")
		switch {
		case !l.Previous:
		case len(l.Changed) == 0:
			line += ", none changed since the last run"
		case len(l.Changed) <= 8:
			line += fmt.Sprintf(", %d changed since the last run: %s", len(l.Changed), strings.Join(l.Changed, " "))
		default:
			line += fmt.Sprintf(", %d changed since the last run", len(l.Changed))
		}
		if len(rt.Problems) > 0 {
			line += " (the page threw errors on load; frames may show a broken state)"
		}
		fmt.Fprintf(w, "  %s %s\n", term.Pad("frames", 12), line)
		fmt.Fprintf(w, "  %s %s\n", term.Pad("", 12), s.Dim(l.Frames))
	}
	if lh := r.Audit.Lighthouse; lh != nil && lh.Status == "ok" {
		fmt.Fprintf(w, "  %s performance %s · accessibility %s · best practices %s · SEO %s\n", term.Pad("lighthouse", 12),
			score(lh.Scores["performance"]), score(lh.Scores["accessibility"]), score(lh.Scores["best-practices"]), score(lh.Scores["seo"]))
		fmt.Fprintf(w, "  %s LCP %s · TBT %s · CLS %s · %s\n", term.Pad("", 12), lh.Metrics["largest-contentful-paint"], lh.Metrics["total-blocking-time"], lh.Metrics["cumulative-layout-shift"], term.Plural(len(lh.Findings), "open audit"))
		if len(lh.Host) > 0 {
			var titles []string
			for _, f := range lh.Host {
				titles = append(titles, strings.ToLower(f.Title[:1])+f.Title[1:])
			}
			fmt.Fprintf(w, "  %s %s\n", term.Pad("", 12), s.Dim(term.Wrap("set on your host, not the page: "+strings.Join(titles, "; "), term.Width-15, strings.Repeat(" ", 15))))
		}
		if lh.Report != "" {
			fmt.Fprintf(w, "  %s %s\n", term.Pad("", 12), s.Dim("full report: "+lh.Report))
		}
	}
}

func score(n *int) string {
	if n == nil {
		return "–"
	}
	return fmt.Sprint(*n)
}

// renderResearch summarizes the evidence the page stands on.
func renderResearch(w io.Writer, s term.Style, r research.Summary) {
	if r.Empty() {
		return
	}
	line := fmt.Sprintf("%s · %s · %s", term.Plural(r.Voices, "voice"), term.Plural(len(r.Platforms), "platform"), term.Plural(r.Peers, "peer"))
	if !r.Latest.IsZero() {
		line += " · latest batch " + r.Latest.Format(time.DateOnly)
	}
	fmt.Fprintf(w, "  %s %s\n", term.Pad("research", 12), line)
	var detail []string
	for i, p := range r.Platforms {
		if i == 3 {
			detail = append(detail, fmt.Sprintf("+%d more", len(r.Platforms)-3))
			break
		}
		detail = append(detail, fmt.Sprintf("%s %d%%", p.Name, 100*p.N/r.Voices))
	}
	if r.Dated > 0 {
		span := r.Oldest.Format("2006")
		if newest := r.Newest.Format("2006"); newest != span {
			span += " to " + newest
		}
		detail = append(detail, fmt.Sprintf("posted %s, %d%% within a year", span, 100*r.Recent/r.Dated))
	}
	if len(detail) > 0 {
		fmt.Fprintf(w, "  %s %s\n", term.Pad("", 12), s.Dim(strings.Join(detail, " · ")))
	}
}

// renderConcept summarizes the decisions the page is held to.
// directionName shortens a chosen direction to its name: the part before the
// first colon or dash, without citations, at most 60 characters.
func directionName(direction string) string {
	name := concept.StripCitations(direction)
	for _, sep := range []string{": ", " — ", " – ", " - "} {
		if head, _, ok := strings.Cut(name, sep); ok && head != "" {
			name = head
			break
		}
	}
	if r := []rune(name); len(r) > 60 {
		name = string(r[:59]) + "…"
	}
	return name
}

func renderConcept(w io.Writer, s term.Style, c concept.Summary) {
	if !c.Present {
		return
	}
	chosen := directionName(c.Chosen)
	if chosen == "" {
		chosen = "no direction chosen yet"
	}
	if c.Open > 0 {
		chosen += s.Dim(" · " + term.Plural(c.Open, "open question"))
	}
	fmt.Fprintf(w, "  %s %s\n", term.Pad("concept", 12), chosen)
	fmt.Fprintf(w, "  %s %s\n", term.Pad("", 12), s.Dim(fmt.Sprintf("cites %s from %s and %s · %s · %s · %s",
		term.Plural(c.Voices, "voice"), term.Plural(c.Platforms, "platform"), term.Plural(c.Peers, "peer"),
		term.Plural(c.Colors, "color"), term.Plural(c.Faces, "typeface"), term.Plural(c.Imagery, "planned image"))))
}

// renderBook summarizes the book deliverable: progress while it is written,
// its files once built, and where buyers download it.
func renderBook(w io.Writer, s term.Style, b *book.Summary, files []compile.File) {
	if b == nil {
		return
	}
	if !b.Concept {
		fmt.Fprintf(w, "  %s %s\n", term.Pad("book", 12), s.Dim("no deliverables/book/concept.md yet"))
		return
	}
	title := b.Title
	if title == "" {
		title = "untitled"
	}
	line := fmt.Sprintf("%s · %d of %d chapters · %s of %s words", title, b.Written, len(b.Chapters), groupDigits(b.Words), groupDigits(b.Target))
	fmt.Fprintf(w, "  %s %s\n", term.Pad("book", 12), line)
	var built []string
	for _, f := range files {
		for _, name := range b.Files {
			if strings.HasSuffix(f.Name, "/"+name) {
				built = append(built, strings.ToUpper(strings.TrimPrefix(path.Ext(name), "."))+" "+term.Bytes(f.Bytes))
			}
		}
	}
	detail := "presale: the download page says the copy is being written"
	if len(built) > 0 {
		detail = strings.Join(built, ", ")
	}
	if b.Pending {
		detail += " · the PDF and paperback print with --release"
	}
	if b.Layout != "" {
		detail += " · " + b.Layout
	}
	if b.URL != "" {
		detail += " · " + b.URL
	}
	fmt.Fprintf(w, "  %s %s\n", term.Pad("", 12), s.Dim(detail))
	if len(b.Channels) > 0 {
		fmt.Fprintf(w, "  %s %s\n", term.Pad("stores", 12), fmt.Sprintf("distribution/ · %d channels", len(b.Channels)))
		fmt.Fprintf(w, "  %s %s\n", term.Pad("", 12), s.Dim(strings.Join(b.Channels, ", ")))
	}
}

func groupDigits(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
