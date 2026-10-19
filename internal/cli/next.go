package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/compile"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/skills"
	"github.com/Anonyfox/lfm/internal/term"
)

// next is the step a compile points to: the phase a project is in, or what
// is left before it ships. Agents read it to stay on the intended path.
type next struct {
	Phase string   `json:"phase"` // research, design, implementation, book, printable, bundle, fix, review, deploy
	Do    string   `json:"do"`
	Skill string   `json:"skill,omitempty"` // the skill that covers the step
	Notes []string `json:"notes,omitempty"`
}

// nextStep orders the work: errors block everything; a missing phase comes
// before polishing warnings, because warnings on a page without research or a
// concept are the wrong work; then the release review, then deploying.
func nextStep(r compile.Result, err error, installed []skills.Copy) next {
	have := map[string]bool{}
	var notes []string
	for _, c := range installed {
		have[c.Name] = true
		if c.State == skills.Outdated {
			notes = append(notes, fmt.Sprintf("%s in %s is from lfm %s; 'lfm skills install' updates it.", c.Name, shortPath(r.Root, c.Path), c.Version))
		}
	}
	n := next{Notes: notes}
	skill := func(topic string) string {
		n.Skill = "lfm-" + topic
		if have[n.Skill] {
			return "Follow the " + n.Skill + " skill; if it is not loaded, 'lfm skills " + topic + "' prints it."
		}
		return "Follow 'lfm skills " + topic + "', or install them all with 'lfm skills install'."
	}
	switch {
	case err != nil || r.Errors() > 0:
		n.Phase, n.Do = "fix", "fix the errors above and compile again; output/ is unchanged until none remain."
	case r.Research.Empty():
		n.Phase, n.Do = "research", "collect real buyer voices and competitor pages into research/. "+skill("research")
	case !r.Concept.Present:
		n.Phase, n.Do = "design", "write concept.md from the research. "+skill("design")
	case r.Starter:
		n.Phase, n.Do = "implementation", "replace the starter page with the page concept.md describes. "+skill("implementation")
	case r.Printable != nil && !r.Printable.Concept:
		n.Phase, n.Do = "printable", "write deliverables/printable/concept.md: buyer, promise, contents, editions, look. "+skill("printable")
	case r.Printable != nil && !r.Printable.Template:
		n.Phase, n.Do = "printable", "write deliverables/printable/printable.html.tmpl: one <section class=\"page\"> per page, as the concept's Contents plans. "+skill("printable")
	case r.Printable != nil && ruleFamily(r, "printable.") > 0:
		n.Phase, n.Do = "fix", fmt.Sprintf("resolve the %s above. %s", term.Plural(ruleFamily(r, "printable."), "printable finding"), skill("printable"))
	case r.Bundle != nil && ruleFamily(r, "bundle.") > 0:
		n.Phase, n.Do = "fix", fmt.Sprintf("resolve the %s above. %s", term.Plural(ruleFamily(r, "bundle."), "bundle finding"), skill("printable"))
	case r.Printable != nil && !r.Printable.Listing && r.Printable.Sell != "bundle":
		n.Phase, n.Do = "printable", "write deliverables/printable/listing.md, the store texts lfm renders into every channel's guide under distribution/. "+skill("printable")
	case r.Book != nil && r.Book.Complete && !r.Book.Listing && r.Book.Sell != "bundle":
		n.Phase, n.Do = "book", "write deliverables/book/listing.md, the store texts lfm renders into every channel's package under distribution/. "+skill("book")
	case r.Book != nil && bookFindings(r) > 0:
		then := " before the next piece of the book"
		if r.Book.Complete {
			then = ""
		}
		n.Phase, n.Do = "fix", fmt.Sprintf("resolve the %s above%s. %s", term.Plural(bookFindings(r), "book finding"), then, skill("book"))
	case r.Book != nil && !r.Book.Concept:
		n.Phase, n.Do = "book", "write deliverables/book/concept.md from the research. "+skill("book")
	case r.Book != nil && r.Book.Next() != nil:
		ch := r.Book.Next()
		n.Phase = "book"
		n.Do = fmt.Sprintf("write deliverables/book/%s, chapter %d of %d, \"%s\" (about %d words): %s. %s", ch.File, ch.N, len(r.Book.Chapters), ch.Title, ch.Words, strings.TrimSuffix(ch.Job, "."), skill("book"))
		if r.Book.URL != "" {
			n.Notes = append(n.Notes, "The page can sell the book now: buyers who pay land on "+r.Book.URL+", which tells them their copy is being written.")
		}
		if machine := ruleCount(r, "book.machine"); machine > 0 {
			n.Notes = append(n.Notes, fmt.Sprintf("%s read as generated to AI-text detectors (book.machine); revise them in the whole-book pass.", term.Plural(machine, "chapter")))
		}
		if page := pageWarnings(r); page > 0 {
			n.Notes = append(n.Notes, fmt.Sprintf("The page itself has %s to resolve before it ships.", term.Plural(page, "warning")))
		}
	case r.Bundle != nil && !r.Bundle.Listing && r.Book != nil && r.Book.Complete:
		n.Phase, n.Do = "bundle", "write deliverables/bundle/listing.md, the shop texts for the book and the printable sold together. "+skill("printable")
	case r.Warnings() > 0:
		n.Phase, n.Do = "fix", "resolve the warnings above; a page is done at 0 errors and 0 warnings."
	case r.Release == nil:
		n.Phase, n.Do = "review", "run 'lfm compile --release' and look at every frame it saves, at phone and desktop size."
		if r.Book != nil && r.Book.Pending {
			n.Notes = append(n.Notes, "The release also prints the book's PDF and paperback; open them and look at every page.")
		}
		if r.Printable != nil && r.Printable.Pending {
			n.Notes = append(n.Notes, "The release also prints the printable's PDFs and listing images; open them and look at every page.")
		}
	default:
		where := "the static host"
		if r.PageURL != "" {
			where = r.PageURL
		}
		n.Phase, n.Do = "deploy", "upload output/ as is to "+where+"."
		for _, u := range sold(r) {
			if u.url != "" {
				n.Notes = append(n.Notes, "In Stripe, set the "+u.name+"'s Payment Link to redirect after payment to "+u.url+" and add that address to the invoice footer, so buyers also get it by email.")
			}
			if len(u.channels) > 0 {
				note := "To sell the " + u.name + " in shops, distribution/" + u.dir + "/README.md lists every channel; each guide in its stores/ names the files to upload and the fields to paste."
				if u.sell == "shops" {
					note += " Once a listing is live, put its address into urls and point the page's offer at it: the shop delivers the files."
				}
				n.Notes = append(n.Notes, note)
			}
		}
	}
	return n
}

func shortPath(root, path string) string {
	rel, err := filepath.Rel(root, filepath.Dir(filepath.Dir(path)))
	if err != nil {
		return filepath.Dir(filepath.Dir(path))
	}
	return filepath.ToSlash(rel)
}

func renderNext(w io.Writer, s term.Style, n next) {
	indent := strings.Repeat(" ", len(n.Phase)+9)
	do, follow, _ := strings.Cut(n.Do, ". Follow ")
	fmt.Fprintf(w, "%s %s\n", s.Bold("Next ("+n.Phase+"):"), term.Wrap(strings.TrimSuffix(do, ".")+".", term.Width-len(indent), indent))
	if follow != "" {
		fmt.Fprintf(w, "%s%s\n", indent, term.Wrap("Follow "+follow, term.Width-len(indent), indent))
	}
	for _, note := range n.Notes {
		fmt.Fprintln(w, s.Dim(term.Wrap(note, term.Width, "")))
	}
}

// printJSONError reports a compile that could not start, such as a folder
// that is no project, in the same shape agents parse for results.
func printJSONError(w io.Writer, err error) error {
	out := struct {
		Error string `json:"error"`
		Next  next   `json:"next"`
	}{Error: err.Error(), Next: next{Phase: "fix", Do: err.Error()}}
	if errors.Is(err, project.ErrNoProject) {
		out.Next = next{Phase: "init", Do: "create the project with 'lfm init <folder>', then run 'lfm skills install' in it."}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// printJSON writes the compile result, the next step and the installed skills.
func printJSON(w io.Writer, r compile.Result, n next, installed []skills.Copy) error {
	out := struct {
		compile.Result
		Errors     int           `json:"errors"`
		Warnings   int           `json:"warnings"`
		DurationMS int64         `json:"durationMs"`
		Next       next          `json:"next"`
		Skills     []skills.Copy `json:"skills"`
	}{r, r.Errors(), r.Warnings(), r.Duration.Milliseconds(), n, installed}
	if out.Skills == nil {
		out.Skills = []skills.Copy{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// ruleCount counts the findings of one rule.
func ruleCount(r compile.Result, rule string) int {
	if r.Validation == nil {
		return 0
	}
	n := 0
	for _, d := range r.Validation.Diagnostics {
		if d.Rule == rule {
			n++
		}
	}
	return n
}

// bookFindings counts the warnings about the book, which come before writing
// the next piece of it.
func bookFindings(r compile.Result) int {
	if r.Validation == nil {
		return 0
	}
	n := 0
	for _, d := range r.Validation.Diagnostics {
		if strings.HasPrefix(d.Rule, "book.") && d.Rule != "book.open" && d.Rule != "book.machine" {
			n++
		}
	}
	return n
}

// pageWarnings counts the warnings that are not about the book.
func pageWarnings(r compile.Result) int {
	if r.Validation == nil {
		return 0
	}
	n := 0
	for _, d := range r.Validation.Diagnostics {
		if d.Severity == diagnostic.Warning && !strings.HasPrefix(d.Rule, "book.") && !strings.HasPrefix(d.Rule, "printable.") && !strings.HasPrefix(d.Rule, "bundle.") {
			n++
		}
	}
	return n
}

// ruleFamily counts the warnings and errors whose rule starts with prefix.
func ruleFamily(r compile.Result, prefix string) int {
	if r.Validation == nil {
		return 0
	}
	n := 0
	for _, d := range r.Validation.Diagnostics {
		if strings.HasPrefix(d.Rule, prefix) {
			n++
		}
	}
	return n
}

// unit is one thing the project sells, for the deploy notes.
type unit struct {
	name, dir, url, sell string
	channels             []string
}

func sold(r compile.Result) []unit {
	var out []unit
	if b := r.Book; b != nil {
		out = append(out, unit{"book", "book", b.URL, b.Sell, b.Channels})
	}
	if p := r.Printable; p != nil {
		out = append(out, unit{"printable", "printable", p.URL, p.Sell, p.Channels})
	}
	if b := r.Bundle; b != nil {
		out = append(out, unit{"bundle", "bundle", b.URL, b.Sell, b.Channels})
	}
	return out
}
