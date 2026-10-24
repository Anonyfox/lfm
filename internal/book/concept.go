package book

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/Anonyfox/lfm/internal/concept"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/research"
)

// Sections of the book concept in their required order. The format follows
// what makes a useful nonfiction book: a scope (reader, promise, who it is not
// for, what it will not cover), one idea the market lacks, hooks, a voice with
// exact phrases, recurring devices, and chapters that each have one job.
var Sections = []concept.SectionSpec{
	{Name: "Reader"},
	{Name: "Promise"},
	{Name: "Big idea"},
	{Name: "Objections"},
	{Name: "Hooks"},
	{Name: "Voice"},
	{Name: "Devices"},
	{Name: "Cases", Optional: true},
	{Name: "Chapters"},
	{Name: "Matter", Optional: true},
	{Name: "Imagery"},
	{Name: "Look"},
	{Name: "Qualities", Optional: true},
	{Name: "Changelog", Optional: true},
}

// FictionSections describe a story plan.
var FictionSections = []concept.SectionSpec{
	{Name: "Reader"}, {Name: "Premise"}, {Name: "Cast"}, {Name: "World"},
	{Name: "Arc"}, {Name: "Hooks"}, {Name: "Voice"}, {Name: "Chapters"},
	{Name: "Matter", Optional: true}, {Name: "Imagery"}, {Name: "Look"},
	{Name: "Qualities", Optional: true}, {Name: "Changelog", Optional: true},
}

// Matter is one unnumbered front or back part.
type Matter struct {
	Part, Place, File string
	Line              int
	Exists            bool
}

// Columns of the tables.
var (
	ChapterColumns = []string{"#", "Title", "Job", "Beats", "Words", "File"}
	ImageColumns   = []string{"Image", "Chapter", "Job", "Art direction", "File"}
)

// Trims are the page formats the PDF can be printed in, with their size in inches.
var Trims = map[string][2]float64{
	"5x8in":     {5, 8},
	"5.5x8.5in": {5.5, 8.5},
	"6x9in":     {6, 9},
	"A5":        {5.83, 8.27},
	"A4":        {8.27, 11.69},
	"letter":    {8.5, 11},
}

// Chapter is one planned chapter.
type Chapter struct {
	Status string `json:"status"`
	N      int    `json:"n"`
	Title  string `json:"title"`
	Job    string `json:"job"`
	Beats  string `json:"-"`
	Words  int    `json:"words"`
	File   string `json:"file"` // relative to the book folder, e.g. chapters/01-start.md
	Line   int    `json:"-"`
	Exists bool   `json:"written"`
}

// Objection is a reader's doubt and the chapter that answers it.
type Objection struct {
	Text    string
	Chapter int
	Line    int
}

// Image is one planned image.
type Image struct {
	Open                                bool
	Name, Chapter, Job, Direction, File string
	Line                                int
}

// Plan is what the concept decided, as the compiler uses it.
type Plan struct {
	Kind        string
	Cast        []string
	POV, Tense  string
	Matter      []Matter
	CoverOpen   bool
	Name        string // working name from the title line
	Title       string
	Subtitle    string
	Chapters    []Chapter
	Images      []Image
	Cover       string // book-relative path of the cover image
	Trim        string
	Body        string // font stack
	Headings    string
	Accent      string
	Use, Avoid  []string
	UseLines    []int  // the concept line of each Use phrase
	Text        string // the concept as written, for names it declares
	Objections  []Objection
	LookLine    int
	Description string // the promise, as the EPUB description
}

var (
	linkCell   = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	hexColor   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	quotedText = regexp.MustCompile(`"([^"]+)"|“([^”]+)”`)
)

// linkTarget returns the target of the first Markdown link in a cell, or the
// cell itself when it holds a bare path.
func linkTarget(cell string) string {
	if m := linkCell.FindStringSubmatch(cell); m != nil {
		return m[1]
	}
	return strings.TrimSpace(cell)
}

func quotedPhrases(text string) []string {
	var out []string
	for _, m := range quotedText.FindAllStringSubmatch(text, -1) {
		if p := strings.TrimSpace(m[1] + m[2]); p != "" {
			out = append(out, p)
		}
	}
	return out
}

type conceptChecker struct {
	add     func(rule string, line int, message, hint string)
	voices  map[string]research.Voice
	peers   map[string]bool
	fiction bool
}

// checkConcept parses the concept and returns the plan it describes.
func checkConcept(text string, voices *research.Voices, competitors *research.Competitors, add func(rule string, line int, message, hint string), kind string) Plan {
	c := conceptChecker{add: add, voices: map[string]research.Voice{}, peers: map[string]bool{}}
	if voices != nil {
		for _, v := range voices.Entries {
			c.voices[fmt.Sprintf("V%03d", v.ID)] = v
		}
	}
	if competitors != nil {
		for _, p := range competitors.Entries {
			c.peers[fmt.Sprintf("P%03d", p.ID)] = true
		}
	}
	c.fiction = kind == "fiction"
	if !c.fiction {
		kind = "nonfiction"
	}
	specs := Sections
	if c.fiction {
		specs = FictionSections
	}
	d, problems := concept.ParseFormat(text, "# Book: ", "<working name>", specs)
	for _, p := range problems {
		add(RuleConcept, p.Line, p.Message, p.Hint)
	}
	plan := Plan{Name: d.Title, Text: text, Kind: kind}
	c.scope(d, &plan)
	c.objections(d, &plan)
	c.hooks(d, &plan)
	c.voice(d, &plan)
	c.chapters(d, &plan)
	c.matter(d, &plan)
	if c.fiction {
		c.story(d, &plan)
	}
	planned := map[int]bool{}
	for _, ch := range plan.Chapters {
		planned[ch.N] = true
	}
	for _, o := range plan.Objections {
		if !planned[o.Chapter] {
			add(RuleConcept, o.Line, fmt.Sprintf("Objection %s points at chapter %d, which the plan does not have.", preview(o.Text), o.Chapter), "Name the chapter that answers it, or plan one.")
		}
	}
	c.imagery(d, &plan)
	c.look(d, &plan)
	c.evidence(d)
	for _, sec := range d.Order {
		for _, l := range sec.Lines {
			if _, question, ok := strings.Cut(l.Text, "Open:"); ok && strings.TrimSpace(question) != "" {
				question, _, _ = strings.Cut(question, "|")
				add(RuleOpen, l.N, "Open question in "+sec.Name+": "+preview(question), "Get the answer from the owner or a source, then replace the line with the decision.")
			}
		}
	}
	return plan
}

func (c conceptChecker) need(s *concept.Section, ok bool, message, hint string) {
	if ok {
		return
	}
	line := 1
	if s != nil {
		line = s.Line
	}
	c.add(RuleConcept, line, message, hint)
}

var chapterRef = regexp.MustCompile(`(?:→|->)\s*(?:chapter\s*|kapitel\s*)?(\d+)`)

// objections are the doubts readers bring, each answered by a chapter.
func (c conceptChecker) objections(d concept.Document, plan *Plan) {
	s := d.Sections["Objections"]
	if s == nil {
		return
	}
	items := s.Items()
	c.need(s, len(items) >= MinObjections, fmt.Sprintf("Objections lists %d doubts; list at least %d.", len(items), MinObjections), "Collect the doubts readers voice in the research (\"I tried that\", \"no time\", \"mine is different\") and say which chapter answers each.")
	for _, it := range items {
		if strings.HasPrefix(it.Text, "Open:") {
			continue
		}
		if len(concept.Citations(it.Text)) == 0 {
			c.add(RuleConcept, it.N, "Objection "+preview(it.Text)+" cites no voice.", "Quote or paraphrase the reader who said it and cite their ID.")
		}
		m := chapterRef.FindStringSubmatch(strings.ToLower(it.Text))
		if m == nil {
			c.add(RuleConcept, it.N, "Objection "+preview(it.Text)+" names no chapter that answers it.", "End the line with → <chapter number>.")
			continue
		}
		plan.Objections = append(plan.Objections, Objection{Text: it.Text, Chapter: atoi(m[1]), Line: it.N})
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func (c conceptChecker) scope(d concept.Document, plan *Plan) {
	reader, promise := d.Sections["Reader"], d.Sections["Promise"]
	if reader != nil {
		notFor, _ := reader.Field("Not for")
		c.need(reader, notFor != "", "Reader has no \"Not for:\" line.", "Name who this book is not for; a book for everyone helps nobody in particular.")
		c.need(reader, c.fiction || len(concept.Citations(reader.Text())) > 0, "Reader cites no voice.", "Describe the reader from what readers wrote, citing their IDs.")
	}
	if promise != nil {
		wont, _ := promise.Field("Won't cover")
		c.need(promise, wont != "", "Promise has no \"Won't cover:\" line.", "Name what the book leaves out, so the promise stays one the book can keep.")
		c.need(promise, len(concept.Citations(promise.Text())) > 0, "Promise cites no voice.", "The promise answers what readers want; cite the voices that want it.")
		before, _ := promise.Field("Before")
		after, _ := promise.Field("After")
		c.need(promise, before != "" && after != "", "Promise has no \"Before:\" and \"After:\" lines.", "State the change in observable terms: what the reader does or has before the book and after it. Every chapter's job adds up to it.")
	}
	if s := d.Sections["Big idea"]; s != nil {
		c.need(s, strings.TrimSpace(concept.StripCitations(s.Text())) != "", "Big idea is empty.", "State the one framing that makes this book different from what readers already have.")
	}
	if s := d.Sections["Devices"]; s != nil {
		c.need(s, len(s.Items()) > 0, "Devices lists no recurring device.", "Name the touches that recur through the book: a running case, a tool, a ritual from the research.")
	}
}

func (c conceptChecker) hooks(d concept.Document, plan *Plan) {
	s := d.Sections["Hooks"]
	if s == nil {
		return
	}
	title, line := s.Field("Title")
	plan.Title = c.hookValue(title, line)
	subtitle, subline := s.Field("Subtitle")
	plan.Subtitle = c.hookValue(subtitle, subline)
	opening, _ := s.Field("Opening")
	c.need(s, plan.Title != "", "Hooks has no \"Title:\" line.", "Write the book's title as Title: \"…\".")
	c.need(s, c.fiction || plan.Subtitle != "", "Hooks has no \"Subtitle:\" line.", "Write the subtitle as Subtitle: \"…\"; it carries the promise in the reader's words.")
	c.need(s, opening != "", "Hooks has no \"Opening:\" line.", "Decide the first page: the scene, question or claim the book opens with.")
	if line > 0 && len([]rune(plan.Title)) > 80 {
		c.add(RuleConcept, line, "The title runs over 80 characters.", "Shorten it; the subtitle can carry the rest.")
	}
}

func (c conceptChecker) voice(d concept.Document, plan *Plan) {
	s := d.Sections["Voice"]
	if s == nil {
		return
	}
	for _, item := range s.Items() {
		text := item.Text
		switch {
		case strings.HasPrefix(text, "Use: "):
			for _, phrase := range quotedPhrases(text) {
				plan.Use = append(plan.Use, phrase)
				plan.UseLines = append(plan.UseLines, item.N)
			}
		case strings.HasPrefix(text, "Avoid: "):
			plan.Avoid = append(plan.Avoid, quotedPhrases(text)...)
		}
	}
	c.need(s, c.fiction || len(plan.Use) > 0, "Voice has no Use: \"…\" phrase.", "Write the readers' own phrases the book uses as Use: \"…\" lines, citing their voices.")
	c.need(s, len(plan.Avoid) > 0, "Voice has no Avoid: \"…\" phrase.", "Write the words the book must not use as Avoid: \"…\" lines.")
}

func (c conceptChecker) chapters(d concept.Document, plan *Plan) {
	s := d.Sections["Chapters"]
	if s == nil {
		return
	}
	columns := append(append([]string{}, ChapterColumns...), "Status")
	rows, header := s.Table(columns)
	if !header {
		columns = ChapterColumns
		rows, header = s.Table(columns)
	}
	if !header {
		c.add(RuleConcept, s.Line, "Chapters needs a table with the columns "+strings.Join(ChapterColumns, " | ")+".", "Plan every chapter as one row.")
		return
	}
	titles, files := map[string]int{}, map[string]int{}
	jobs := map[string][]int{}
	firstJob := map[string]Chapter{}
	var jobOrder []string
	for i, row := range rows {
		if len(row.Cells) != len(columns) {
			c.add(RuleConcept, row.N, fmt.Sprintf("Chapter row has %d cells; the table has %d columns.", len(row.Cells), len(columns)), "Fill every column; write \"none\" only in Beats.")
			continue
		}
		ch := Chapter{Title: strings.TrimSpace(row.Cells[1]), Job: concept.StripCitations(row.Cells[2]), Beats: row.Cells[3], File: path.Clean(linkTarget(row.Cells[5])), Line: row.N}
		ch.Status = "draft"
		if len(row.Cells) == 7 && strings.TrimSpace(row.Cells[6]) != "" {
			ch.Status = strings.TrimSpace(row.Cells[6])
		}
		if ch.Status != "draft" && ch.Status != "done" && ch.Status != "planned" {
			c.add(RuleConcept, row.N, fmt.Sprintf("Chapter %d has invalid Status %q.", i+1, ch.Status), "Use planned, draft or done.")
		}
		if n, err := strconv.Atoi(strings.TrimSpace(row.Cells[0])); err != nil || n != i+1 {
			c.add(RuleConcept, row.N, fmt.Sprintf("Chapter number %q should be %d.", row.Cells[0], i+1), "Number the chapters 1, 2, 3 in reading order.")
		}
		ch.N = i + 1
		words, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(row.Cells[4]), ",", ""))
		if err != nil || words < MinWords || words > MaxWords {
			c.add(RuleConcept, row.N, fmt.Sprintf("Word target %q for chapter %d is not a number from %d to %d.", row.Cells[4], ch.N, MinWords, MaxWords), "Give every chapter a target length the compiler can hold it to.")
		}
		ch.Words = words
		if ch.Title == "" {
			c.add(RuleConcept, row.N, fmt.Sprintf("Chapter %d has no title.", ch.N), "Name the chapter.")
		} else if prev, dup := titles[strings.ToLower(ch.Title)]; dup {
			c.add(RuleConcept, row.N, fmt.Sprintf("Chapter %d repeats the title of chapter %d.", ch.N, prev), "Give each chapter its own title.")
		}
		titles[strings.ToLower(ch.Title)] = ch.N
		n := len(strings.Fields(ch.Job))
		if n == 0 {
			c.add(RuleConcept, row.N, fmt.Sprintf("Chapter %d has no job.", ch.N), "Write the chapter's job in at least 4 words.")
		} else if n < MinJobWords {
			c.add(RuleConcept, row.N, fmt.Sprintf("Chapter %d's job %q is too short to plan from (%d words, write at least 4).", ch.N, ch.Job, n), "Describe what this chapter does.")
		}
		key := strings.ToLower(ch.Job)
		if _, ok := jobs[key]; !ok {
			jobOrder = append(jobOrder, key)
			firstJob[key] = ch
		}
		jobs[key] = append(jobs[key], ch.N)
		switch {
		case !strings.HasPrefix(ch.File, ChaptersDir+"/") || path.Ext(ch.File) != ".md" || strings.Count(ch.File, "/") != 1:
			c.add(RuleConcept, row.N, fmt.Sprintf("Chapter %d's file %q is not chapters/<name>.md.", ch.N, ch.File), "Link the chapter file as [name](chapters/01-name.md).")
		default:
			if prev, dup := files[ch.File]; dup {
				c.add(RuleConcept, row.N, fmt.Sprintf("Chapter %d uses the file of chapter %d.", ch.N, prev), "Give each chapter its own file.")
			}
			files[ch.File] = ch.N
		}
		plan.Chapters = append(plan.Chapters, ch)
	}
	for _, job := range jobOrder {
		ns := jobs[job]
		if job == "" || len(ns) < 2 {
			continue
		}
		names := make([]string, len(ns))
		for i, n := range ns {
			names[i] = strconv.Itoa(n)
		}
		label := strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
		c.add(RuleConcept, firstJob[job].Line, fmt.Sprintf("Chapters %s share the job %q.", label, firstJob[job].Job), "Give each chapter a distinct job.")
	}
	if len(rows) < MinChapters {
		c.add(RuleConcept, s.Line, fmt.Sprintf("The plan has %d chapters; a book needs at least %d.", len(rows), MinChapters), "Split the promise into the steps the reader needs.")
	}
}

func (c conceptChecker) imagery(d concept.Document, plan *Plan) {
	s := d.Sections["Imagery"]
	if s == nil {
		return
	}
	rows, header := s.Table(ImageColumns)
	if !header {
		c.add(RuleConcept, s.Line, "Imagery needs a table with the columns "+strings.Join(ImageColumns, " | ")+".", "Plan the cover and every image as one row.")
		return
	}
	for _, row := range rows {
		if len(row.Cells) != len(ImageColumns) {
			c.add(RuleConcept, row.N, "Image row does not fill every column.", "Fill Image, Chapter, Job, Art direction and File.")
			continue
		}
		img := Image{Name: row.Cells[0], Chapter: row.Cells[1], Job: strings.ToLower(strings.TrimSpace(row.Cells[2])), Direction: row.Cells[3], File: path.Clean(linkTarget(row.Cells[4])), Line: row.N}
		img.Open = strings.Contains(strings.Join(row.Cells, " "), "Open:")
		if img.Job == "cover" {
			plan.CoverOpen = img.Open
			if linkCell.MatchString(row.Cells[4]) || (img.File != "." && !strings.Contains(row.Cells[4], "Open:")) {
				plan.Cover = img.File
			}
		}
		if img.Open {
			plan.Images = append(plan.Images, img)
			continue
		}
		if !strings.HasPrefix(img.File, ImagesDir+"/") || !imageExt(img.File) {
			c.add(RuleConcept, row.N, fmt.Sprintf("Image file %q is not images/<name>.jpg, .png, .webp or .svg.", img.File), "Store images in deliverables/book/images and link them as [name](images/cover.jpg).")
		}
		if img.Job == "cover" {
			plan.Cover = img.File
		}
		plan.Images = append(plan.Images, img)
	}
	c.need(s, plan.Cover != "" || plan.CoverOpen, "Imagery plans no cover.", "Add a row whose Job is cover: the image readers see in the store and their library.")
}

func imageExt(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".svg":
		return true
	}
	return false
}

func (c conceptChecker) look(d concept.Document, plan *Plan) {
	s := d.Sections["Look"]
	if s == nil {
		return
	}
	plan.LookLine = s.Line
	plan.Trim, _ = s.Field("Trim")
	plan.Body, _ = s.Field("Body")
	plan.Headings, _ = s.Field("Headings")
	plan.Accent, _ = s.Field("Accent")
	if _, ok := Trims[plan.Trim]; !ok {
		c.add(RuleConcept, s.Line, "Look has no valid \"Trim:\" (5x8in, 5.5x8.5in, 6x9in, A5, A4 or letter).", "Pick the printed page size of the PDF.")
	}
	c.need(s, plan.Body != "", "Look has no \"Body:\" font stack.", "Name the body typeface as a stack: Body: <family>, <fallback families>, <generic family>.")
	c.need(s, plan.Headings != "", "Look has no \"Headings:\" font stack.", "Name the heading typeface as a stack.")
	if !hexColor.MatchString(plan.Accent) {
		c.add(RuleConcept, s.Line, fmt.Sprintf("Look has no valid \"Accent:\" color (got %q).", plan.Accent), "Give the one accent color as #rrggbb.")
		plan.Accent = ""
	}
}

func (c conceptChecker) evidence(d concept.Document) {
	if !c.fiction && len(c.voices) == 0 && len(c.peers) == 0 {
		c.add(RuleEvidence, 1, "The book concept has no research to stand on.", "Collect research first (lfm skills research), including the books readers already compare.")
		return
	}
	voices, peers := map[string]bool{}, map[string]bool{}
	for _, s := range d.Order {
		for _, l := range s.Lines {
			for _, id := range concept.Citations(l.Text) {
				_, isVoice := c.voices[id]
				switch {
				case isVoice:
					voices[id] = true
				case c.peers[id]:
					peers[id] = true
				default:
					c.add(RuleEvidence, l.N, id+" does not exist in the research files.", "Cite only IDs present in research/; never cite from memory.")
				}
			}
		}
	}
	if c.fiction {
		return
	}
	if want := min(MinVoices, len(c.voices)); len(voices) < want {
		c.add(RuleEvidence, 1, fmt.Sprintf("The book concept cites %d distinct voices; cite at least %d.", len(voices), want), "Ground the reader, promise, hooks and chapters in what readers wrote.")
	}
	if want := min(MinPeers, len(c.peers)); len(peers) < want {
		c.add(RuleEvidence, 1, fmt.Sprintf("The book concept cites %d competing books or guides; cite at least %d.", len(peers), want), "Add the books readers compare as peers in research/competitor-analysis.md and cite where the big idea departs from them.")
	}
}

func preview(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 60 {
		return string(r[:60]) + "…"
	}
	return s
}

func at(file string, line int, rule string, sev diagnostic.Severity, message, hint string) diagnostic.Diagnostic {
	return diagnostic.Diagnostic{Rule: rule, Severity: sev, Location: diagnostic.Location{File: file, Line: line, Column: 1}, Message: message, Hint: hint}
}

func (c conceptChecker) hookValue(value string, line int) string {
	value = strings.TrimSpace(value)
	for _, pair := range [][2]string{{"\"", "\""}, {"“", "”"}, {"„", "“"}, {"‘", "’"}, {"'", "'"}} {
		if !strings.HasPrefix(value, pair[0]) {
			continue
		}
		rest := strings.TrimPrefix(value, pair[0])
		if i := strings.Index(rest, pair[1]); i >= 0 {
			tail := strings.TrimSpace(rest[i+len(pair[1]):])
			if tail != "" {
				c.add(RuleConcept, line, "Text after the quoted title is ignored: "+tail, "Move notes to an Open: line.")
			}
			return rest[:i]
		}
	}
	return strings.Trim(concept.StripCitations(value), "\"“” ")
}

func (c conceptChecker) story(d concept.Document, plan *Plan) {
	for _, name := range []string{"Premise", "World", "Arc"} {
		s := d.Sections[name]
		c.need(s, s != nil && strings.TrimSpace(s.Text()) != "", name+" is empty.", "Write the story's "+strings.ToLower(name)+".")
	}
	if s := d.Sections["Premise"]; s != nil {
		plan.Description = s.Text()
	}
	if s := d.Sections["Cast"]; s != nil {
		for _, item := range s.Items() {
			name, role, ok := strings.Cut(item.Text, ":")
			if !ok || strings.TrimSpace(role) == "" {
				c.add(RuleConcept, item.N, "Cast needs Name: role and want.", "Give each cast member a name, role and want.")
				continue
			}
			name, _, _ = strings.Cut(strings.TrimSpace(name), " (")
			if name != "" {
				plan.Cast = append(plan.Cast, strings.TrimSpace(name))
			}
		}
		c.need(s, len(plan.Cast) > 0, "Cast lists no names.", "Add at least one Name: role and want item.")
	}
	s := d.Sections["Voice"]
	if s != nil {
		plan.POV, _ = s.Field("POV")
		plan.Tense, _ = s.Field("Tense")
	}
	c.need(s, plan.POV != "", "Voice has no POV: field.", "State the point of view.")
	c.need(s, plan.Tense != "", "Voice has no Tense: field.", "State the narrative tense.")
}

func (c conceptChecker) matter(d concept.Document, plan *Plan) {
	s := d.Sections["Matter"]
	if s == nil {
		return
	}
	rows, header := s.Table([]string{"Part", "Place", "File"})
	c.need(s, header, "Matter needs Part | Place | File columns.", "Plan each unnumbered part as a row.")
	if !header {
		return
	}
	for _, row := range rows {
		if strings.Contains(strings.Join(row.Cells, " "), "Open:") {
			continue
		}
		if len(row.Cells) != 3 {
			c.add(RuleConcept, row.N, "Matter row needs three cells.", "Fill Part, Place and File.")
			continue
		}
		m := Matter{Part: row.Cells[0], Place: row.Cells[1], File: path.Clean(linkTarget(row.Cells[2])), Line: row.N}
		if m.Part == "" || (m.Place != "front" && m.Place != "back") || !strings.HasPrefix(m.File, "matter/") || strings.Count(m.File, "/") != 1 || path.Ext(m.File) != ".md" {
			c.add(RuleConcept, row.N, "Matter needs a part, front or back, and matter/<name>.md.", "Fill the part name, place and file link.")
			continue
		}
		plan.Matter = append(plan.Matter, m)
	}
}
