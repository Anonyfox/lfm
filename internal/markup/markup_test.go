package markup

import (
	"slices"
	"strings"
	"testing"
)

var docOptions = Options{Lang: "en", Directives: []string{"title", "columns", "form", "signature", "center"}}

func parse(t *testing.T, src string, o Options) ([]Block, []Problem) {
	t.Helper()
	return Parse(Lines([]byte(src)), 0, o)
}

func clean(t *testing.T, src string, o Options) []Block {
	t.Helper()
	blocks, problems := parse(t, src, o)
	if len(problems) > 0 {
		t.Fatalf("problems for %q: %+v", src, problems)
	}
	return blocks
}

func kinds(blocks []Block) []string {
	out := make([]string, len(blocks))
	for i, b := range blocks {
		out[i] = b.Kind
	}
	return out
}

func hasProblem(problems []Problem, text string) bool {
	for _, p := range problems {
		if strings.Contains(p.Message, text) {
			return true
		}
	}
	return false
}

func TestFrontMatterWithoutLeadingRuleIsBody(t *testing.T) {
	settings, start, problems := FrontMatter(Lines([]byte("title: Invoice\n\nText")))
	if len(settings) != 0 || start != 0 || problems != nil {
		t.Fatalf("got %v, %d, %v", settings, start, problems)
	}
	if settings, start, _ := FrontMatter(nil); len(settings) != 0 || start != 0 {
		t.Fatalf("empty input: %v, %d", settings, start)
	}
}

func TestFrontMatterUnclosedIsReportedAtLineOne(t *testing.T) {
	lines := Lines([]byte("---\ntitle: Invoice\n\nText"))
	_, start, problems := FrontMatter(lines)
	if start != len(lines) {
		t.Errorf("body starts at %d, want %d", start, len(lines))
	}
	if len(problems) != 1 || problems[0].Line != 1 || !strings.Contains(problems[0].Message, "no closing ---") {
		t.Fatalf("problems %+v", problems)
	}
}

func TestFrontMatterReportsLinesThatAreNotKeyValue(t *testing.T) {
	settings, start, problems := FrontMatter(Lines([]byte("---\ntitle: Invoice\njust words\n\n: no key\n---\nBody")))
	if start != 6 {
		t.Errorf("body starts at %d, want 6", start)
	}
	if len(settings) != 1 || settings["title"].Value != "Invoice" || settings["title"].Line != 2 {
		t.Errorf("settings %v", settings)
	}
	if len(problems) != 2 || problems[0].Line != 3 || problems[1].Line != 5 {
		t.Fatalf("problems %+v", problems)
	}
}

func TestFrontMatterLowercasesKeysAndTrimsQuotes(t *testing.T) {
	settings, _, problems := FrontMatter(Lines([]byte("---\r\n  Title :  \"Hello: World\"  \r\nFooter: Page 1\r\n---\r\n")))
	if problems != nil {
		t.Fatal(problems)
	}
	if got := settings["title"].Value; got != "Hello: World" {
		t.Errorf("title %q", got)
	}
	if got := settings["footer"].Value; got != "Page 1" {
		t.Errorf("footer %q", got)
	}
}

func TestFrontMatterKeepsKeysInLineOrder(t *testing.T) {
	settings, _, _ := FrontMatter(Lines([]byte("---\nzeta: 1\nalpha: 2\nmid: 3\nbeta: 4\n---")))
	if got, want := Keys(settings), []string{"zeta", "alpha", "mid", "beta"}; !slices.Equal(got, want) {
		t.Fatalf("keys %v, want %v", got, want)
	}
}

func TestFrontMatterAfterByteOrderMark(t *testing.T) {
	settings, start, _ := FrontMatter(Lines([]byte("\ufeff---\ntitle: Invoice\n---\nBody")))
	if settings["title"].Value != "Invoice" || start != 3 {
		t.Fatalf("settings %v, start %d", settings, start)
	}
}

func TestParseBodyAfterByteOrderMark(t *testing.T) {
	blocks, _ := parse(t, "\ufeff# Heading", docOptions)
	if len(blocks) != 1 || blocks[0].Kind != Heading {
		t.Fatalf("blocks %+v", blocks)
	}
}

func TestLinesSplitsCRLF(t *testing.T) {
	if got := Lines([]byte("a\r\nb\nc")); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("lines %q", got)
	}
}

func TestParseCRLFInputReadsLikeLF(t *testing.T) {
	src := "# Title\n\nOne line\nnext line\n\n- a\n- b\n\n::: center\nMiddle\n:::\n"
	lf := clean(t, src, docOptions)
	crlf := clean(t, strings.ReplaceAll(src, "\n", "\r\n"), docOptions)
	if !slices.Equal(kinds(lf), kinds(crlf)) {
		t.Fatalf("kinds %v vs %v", kinds(lf), kinds(crlf))
	}
	for i := range lf {
		if Plain(lf[i].Inlines) != Plain(crlf[i].Inlines) || lf[i].Line != crlf[i].Line {
			t.Errorf("block %d: %q@%d vs %q@%d", i, Plain(lf[i].Inlines), lf[i].Line, Plain(crlf[i].Inlines), crlf[i].Line)
		}
	}
}

func TestParseBlocksCarrySourceLinesWithOffset(t *testing.T) {
	blocks, _ := Parse([]string{"", "# Heading", "", "Text"}, 10, docOptions)
	if len(blocks) != 2 || blocks[0].Line != 12 || blocks[1].Line != 14 {
		t.Fatalf("blocks %+v", blocks)
	}
}

func TestParseTitleDirective(t *testing.T) {
	blocks := clean(t, "::: title\nAnnual report\n\nFiscal year 2026\n:::\nBody", docOptions)
	if got := kinds(blocks); !slices.Equal(got, []string{Title, Subtitle, Para}) {
		t.Fatalf("kinds %v", got)
	}
	if Plain(blocks[0].Inlines) != "Annual report" || blocks[0].Line != 2 || blocks[1].Line != 4 {
		t.Errorf("title %+v subtitle %+v", blocks[0], blocks[1])
	}
	_, problems := parse(t, "::: title\nOne\nTwo\nThree\n:::", docOptions)
	if len(problems) != 1 || problems[0].Line != 4 || !strings.Contains(problems[0].Message, "one line of title") {
		t.Fatalf("problems %+v", problems)
	}
}

func TestParseColumnsSplitOnPlusLines(t *testing.T) {
	blocks := clean(t, "::: columns 1 2\nLeft\n+++\n## Right\n\nMore\n:::", docOptions)
	if len(blocks) != 1 || blocks[0].Kind != Columns {
		t.Fatalf("blocks %+v", blocks)
	}
	b := blocks[0]
	if len(b.Cols) != 2 || !slices.Equal(b.Widths, []float64{1, 2}) {
		t.Fatalf("cols %d widths %v", len(b.Cols), b.Widths)
	}
	if got := kinds(b.Cols[1]); !slices.Equal(got, []string{Heading, Para}) || b.Cols[1][0].Line != 4 || b.Cols[1][1].Line != 6 {
		t.Errorf("right column %+v", b.Cols[1])
	}
}

func TestParseColumnsWithoutSharesAreEqual(t *testing.T) {
	b := clean(t, "::: columns\nA\n+++\nB\n+++\nC\n:::", docOptions)[0]
	if !slices.Equal(b.Widths, []float64{1, 1, 1}) {
		t.Fatalf("widths %v", b.Widths)
	}
}

func TestParseColumnsProblems(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"::: columns\nOnly one\n:::", "Columns hold 1 columns."},
		{"::: columns\nA\n+++\nB\n+++\nC\n+++\nD\n:::", "Columns hold 4 columns."},
		{"::: columns 1 x\nA\n+++\nB\n:::", `Column share "x" is not a number.`},
		{"::: columns 1 0\nA\n+++\nB\n:::", `Column share "0" is not a number.`},
		{"::: columns 1 2 3\nA\n+++\nB\n:::", "Columns name 3 widths for 2 columns."},
	} {
		blocks, problems := parse(t, tc.src, docOptions)
		if !hasProblem(problems, tc.want) {
			t.Errorf("%q: problems %+v, want %q", tc.src, problems, tc.want)
		}
		if b := blocks[0]; len(b.Widths) != len(b.Cols) {
			t.Errorf("%q: %d widths for %d columns", tc.src, len(b.Widths), len(b.Cols))
		}
	}
}

func TestParseFormSplitsLabelAndValue(t *testing.T) {
	b := clean(t, "::: form\nName: ==Jane Smith==\n\nDate: 1 May 2026\nNotes\n:::", docOptions)[0]
	if b.Kind != Form || len(b.Fields) != 3 {
		t.Fatalf("block %+v", b)
	}
	if Plain(b.Fields[0].Label) != "Name" || len(b.Fields[0].Value) != 1 || !b.Fields[0].Value[0].Fill {
		t.Errorf("first field %+v", b.Fields[0])
	}
	if Plain(b.Fields[2].Label) != "Notes" || b.Fields[2].Value != nil {
		t.Errorf("label-only field %+v", b.Fields[2])
	}
	_, problems := parse(t, "::: form\n\n:::", docOptions)
	if !hasProblem(problems, "::: form has no lines.") {
		t.Errorf("empty form: %+v", problems)
	}
}

func TestParseSignatureKeepsWholeLinesAsLabels(t *testing.T) {
	b := clean(t, "::: signature\nPlace: date\nSigned\n:::", docOptions)[0]
	if b.Kind != Signature || len(b.Fields) != 2 || Plain(b.Fields[0].Label) != "Place: date" || b.Fields[0].Value != nil {
		t.Fatalf("block %+v", b)
	}
	_, problems := parse(t, "::: signature\nA\nB\nC\nD\n:::", docOptions)
	if !hasProblem(problems, "at most three lines") {
		t.Errorf("four signers: %+v", problems)
	}
}

func TestParseCenterMarksEveryInnerBlock(t *testing.T) {
	blocks := clean(t, "Before\n\n::: center\n# Welcome\n\nTo the club\n:::\n\nAfter", docOptions)
	if got := kinds(blocks); !slices.Equal(got, []string{Para, Heading, Para, Para}) {
		t.Fatalf("kinds %v", got)
	}
	for i, want := range []bool{false, true, true, false} {
		if blocks[i].Center != want {
			t.Errorf("block %d center %v", i, blocks[i].Center)
		}
	}
}

func TestParseNotesHoldTheirBlocks(t *testing.T) {
	o := Options{Lang: "en", Directives: []string{"notes"}, Slides: true}
	blocks := clean(t, "# Slide\n\n::: notes\nSay hello.\n\n- point\n:::", o)
	if got := kinds(blocks); !slices.Equal(got, []string{Heading, Notes}) {
		t.Fatalf("kinds %v", got)
	}
	if got := kinds(blocks[1].Children); !slices.Equal(got, []string{Para, List}) || blocks[1].Line != 3 {
		t.Errorf("notes %+v", blocks[1])
	}
}

func TestParseRejectsDirectivesNotAllowedWithHint(t *testing.T) {
	blocks, problems := parse(t, "::: notes\nHidden\n:::\nAfter", docOptions)
	if len(problems) != 1 || problems[0].Line != 1 || problems[0].Message != "Directive ::: notes is unknown here." {
		t.Fatalf("problems %+v", problems)
	}
	if !strings.Contains(problems[0].Hint, "::: title, ::: columns, ::: form, ::: signature, ::: center") {
		t.Errorf("hint %q", problems[0].Hint)
	}
	if got := kinds(blocks); !slices.Equal(got, []string{Para}) || Plain(blocks[0].Inlines) != "After" {
		t.Errorf("the rejected block's content leaked: %+v", blocks)
	}
}

func TestParseAllowedButUnimplementedDirective(t *testing.T) {
	_, problems := parse(t, "::: aside\nx\n:::", Options{Directives: []string{"aside"}})
	if !hasProblem(problems, "Directive ::: aside is unknown.") {
		t.Fatalf("problems %+v", problems)
	}
}

func TestParseUnclosedDirective(t *testing.T) {
	blocks, problems := parse(t, "Before\n\n::: center\nNever closed", docOptions)
	if len(problems) != 1 || problems[0].Line != 3 || !strings.Contains(problems[0].Message, "::: center has no closing :::") {
		t.Fatalf("problems %+v", problems)
	}
	if got := kinds(blocks); !slices.Equal(got, []string{Para}) {
		t.Errorf("blocks %v", got)
	}
}

func TestParseNestedDirectivesCloseInOrder(t *testing.T) {
	blocks := clean(t, "::: columns\n::: center\nLeft\n:::\n+++\n::: form\nName: x\n:::\n:::\nAfter", docOptions)
	if got := kinds(blocks); !slices.Equal(got, []string{Columns, Para}) {
		t.Fatalf("kinds %v", got)
	}
	cols := blocks[0].Cols
	if len(cols) != 2 || len(cols[0]) != 1 || !cols[0][0].Center || cols[1][0].Kind != Form {
		t.Fatalf("columns %+v", cols)
	}
	if blocks[1].Line != 10 {
		t.Errorf("after the block at line %d", blocks[1].Line)
	}
}

func TestParseSlidesSplitOnRuleEvenAfterParagraph(t *testing.T) {
	o := Options{Lang: "en", Directives: []string{"notes"}, Slides: true}
	blocks := clean(t, "# One\nIntro line\n---\n# Two\n\n---\n# Three", o)
	if got := kinds(blocks); !slices.Equal(got, []string{Heading, Para, PageBreak, Heading, PageBreak, Heading}) {
		t.Fatalf("kinds %v", got)
	}
	if blocks[2].Line != 3 || blocks[3].Line != 4 || blocks[4].Line != 6 || blocks[5].Line != 7 {
		t.Errorf("lines %d %d %d %d", blocks[2].Line, blocks[3].Line, blocks[4].Line, blocks[5].Line)
	}
	if Plain(blocks[1].Inlines) != "Intro line" {
		t.Errorf("the paragraph became %q", Plain(blocks[1].Inlines))
	}
}

func TestParseSlidesKeepRuleInsideDirective(t *testing.T) {
	o := Options{Lang: "en", Directives: []string{"notes"}, Slides: true}
	blocks := clean(t, "# One\n\n::: notes\nFirst\n\n---\n\nSecond\n:::\n---\n# Two", o)
	if got := kinds(blocks); !slices.Equal(got, []string{Heading, Notes, PageBreak, Heading}) {
		t.Fatalf("kinds %v", got)
	}
	if got := kinds(blocks[1].Children); len(got) != 3 || got[0] != Para || got[2] != Para {
		t.Errorf("notes %v", got)
	}
}

func TestParseWithoutSlidesReadsSetextHeading(t *testing.T) {
	blocks := clean(t, "Intro line\n---", docOptions)
	if len(blocks) != 1 || blocks[0].Kind != Heading || blocks[0].Level != 2 {
		t.Fatalf("blocks %+v", blocks)
	}
}

func TestParseRuleAtStartIsPageBreak(t *testing.T) {
	blocks := clean(t, "---\n\nA", docOptions)
	if got := kinds(blocks); !slices.Equal(got, []string{PageBreak, Para}) || blocks[0].Line != 1 {
		t.Fatalf("blocks %+v", blocks)
	}
}

func TestParseStarsAfterTextAreOrnament(t *testing.T) {
	blocks := clean(t, "A\n\n* * *\n\nB\n\n___", docOptions)
	if got := kinds(blocks); !slices.Equal(got, []string{Para, Ornament, Para, Ornament}) {
		t.Fatalf("kinds %v", got)
	}
}

func TestParseRuleAfterTextIsPageBreakAtItsLine(t *testing.T) {
	blocks := clean(t, "A\n\n---\n\nB", docOptions)
	if got := kinds(blocks); !slices.Equal(got, []string{Para, PageBreak, Para}) || blocks[1].Line != 3 {
		t.Fatalf("kinds %v, line %d", kinds(blocks), blocks[1].Line)
	}
}

func TestParseStarsAfterListAreOrnament(t *testing.T) {
	blocks := clean(t, "- item\n\n***", docOptions)
	if got := kinds(blocks); !slices.Equal(got, []string{List, Ornament}) {
		t.Fatalf("kinds %v", got)
	}
}

func TestParseHeadingsDeeperThanThreeAreClamped(t *testing.T) {
	blocks, problems := parse(t, "#### Deep\n\n###### Deeper", docOptions)
	if len(problems) != 2 || problems[0].Line != 1 || problems[1].Line != 3 || !strings.Contains(problems[0].Message, "Heading level 4") {
		t.Fatalf("problems %+v", problems)
	}
	for _, b := range blocks {
		if b.Kind != Heading || b.Level != 3 {
			t.Errorf("block %+v", b)
		}
	}
}

func TestParseRejectsCodeAndRawHTML(t *testing.T) {
	for _, tc := range []struct {
		src  string
		line int
		want string
	}{
		{"Text\n\n```\ncode\n```", 4, "Code blocks"}, // the first line of code
		{"Text\n\n    indented code", 3, "Code blocks"},
		{"<div>\nhello\n</div>", 1, "Raw HTML"},
		{"Some <b>bold</b> text", 1, "Raw HTML"},
	} {
		_, problems := parse(t, tc.src, docOptions)
		if len(problems) == 0 || problems[0].Line != tc.line || !strings.Contains(problems[0].Message, tc.want) {
			t.Errorf("%q: problems %+v", tc.src, problems)
		}
	}
}

func TestParseImageAloneIsFigure(t *testing.T) {
	for _, tc := range []struct {
		src          string
		width, share float64
	}{
		{"![Map of the site](images/map.png)", 0, 0},
		{`![Map](images/map.png "80mm")`, 80, 0},
		{`![Map](images/map.png "50 %")`, 0, 0.5},
		{`![Map](images/map.png "a caption")`, 0, 0},
	} {
		blocks := clean(t, tc.src, docOptions)
		if len(blocks) != 1 || blocks[0].Kind != ImageKind {
			t.Fatalf("%q: blocks %+v", tc.src, blocks)
		}
		img := blocks[0].Image
		if img.Src != "images/map.png" || img.Width != tc.width || img.Share != tc.share {
			t.Errorf("%q: figure %+v", tc.src, img)
		}
	}
	if img := clean(t, "![Map of the site](a.png)", docOptions)[0].Image; img.Alt != "Map of the site" {
		t.Errorf("alt %q", img.Alt)
	}
}

func TestParseInlineImageIsRejected(t *testing.T) {
	blocks, problems := parse(t, "See ![map](a.png) here", docOptions)
	if len(problems) != 1 || !strings.Contains(problems[0].Message, "Images go on a line of their own.") {
		t.Fatalf("problems %+v", problems)
	}
	if blocks[0].Kind != Para {
		t.Errorf("kind %s", blocks[0].Kind)
	}
}

func TestParseInlineStyles(t *testing.T) {
	ins := clean(t, "Plain **bold** *italic* [site](https://example.org) <https://x.org> `code`", docOptions)[0].Inlines
	var bold, italic, link, auto bool
	for _, in := range ins {
		bold = bold || in.Bold && in.Text == "bold"
		italic = italic || in.Italic && in.Text == "italic"
		link = link || in.Link == "https://example.org" && in.Text == "site"
		auto = auto || in.Link == "https://x.org" && in.Text == "https://x.org"
	}
	if !bold || !italic || !link || !auto || !strings.Contains(Plain(ins), "code") {
		t.Fatalf("inlines %+v", ins)
	}
}

func TestParseLineBreaksStayInParagraph(t *testing.T) {
	ins := clean(t, "Main Street 1\n12345 Town", docOptions)[0].Inlines
	if Plain(ins) != "Main Street 1\n12345 Town" {
		t.Fatalf("plain %q", Plain(ins))
	}
}

func TestParseListsAndTasks(t *testing.T) {
	b := clean(t, "- [ ] open\n- [x] done\n  - nested\n    - deeper", docOptions)[0]
	if b.Kind != List || b.Ordered || len(b.Items) != 4 {
		t.Fatalf("list %+v", b)
	}
	if !b.Items[0].Check || b.Items[0].Checked || !b.Items[1].Checked || b.Items[2].Level != 1 || b.Items[3].Level != 2 {
		t.Errorf("items %+v", b.Items)
	}
	if Plain(b.Items[0].Inlines) != "open" {
		t.Errorf("item text %q", Plain(b.Items[0].Inlines))
	}
	if o := clean(t, "1. a\n2. b", docOptions)[0]; !o.Ordered {
		t.Error("ordered list read as bullets")
	}
	_, problems := parse(t, "- a\n  - b\n    - c\n      - d", docOptions)
	if !hasProblem(problems, "Lists nest at most three levels.") {
		t.Errorf("four levels: %+v", problems)
	}
}

func TestParseBlockquoteIsBox(t *testing.T) {
	b := clean(t, "> Note\n>\n> - item", docOptions)[0]
	if b.Kind != Box || !slices.Equal(kinds(b.Children), []string{Para, List}) {
		t.Fatalf("box %+v", b)
	}
}

func TestParseTable(t *testing.T) {
	b := clean(t, "| Item | Qty | Note |\n|:--|--:|:-:|\n| Tea | 2 | hot |\n| Cake | 10 | |", docOptions)[0]
	if b.Kind != TableKind || b.Table == nil {
		t.Fatalf("block %+v", b)
	}
	tb := b.Table
	if !slices.Equal(tb.Align, []string{"left", "right", "center"}) || len(tb.Head) != 3 || len(tb.Rows) != 2 {
		t.Fatalf("table %+v", tb)
	}
	if Plain(tb.Rows[1][0]) != "Cake" || Plain(tb.Head[1]) != "Qty" {
		t.Errorf("cells %q %q", Plain(tb.Rows[1][0]), Plain(tb.Head[1]))
	}
}

func TestFieldsSplitMarkedRuns(t *testing.T) {
	got := Fields([]Inline{{Text: "Dear ==name==, your ==order== ships.", Bold: true}})
	want := []Inline{{Text: "Dear ", Bold: true}, {Text: "name", Bold: true, Fill: true}, {Text: ", your ", Bold: true}, {Text: "order", Bold: true, Fill: true}, {Text: " ships.", Bold: true}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %+v", got)
	}
	if got := Fields([]Inline{{Text: "==only=="}}); !slices.Equal(got, []Inline{{Text: "only", Fill: true}}) {
		t.Errorf("whole run: %+v", got)
	}
}

func TestFieldsLeaveUnmarkedRuns(t *testing.T) {
	in := []Inline{{Text: "a == b"}, {Text: "plain"}, {Break: true, Text: "==x=="}, {Text: "===="}}
	if got := Fields(in); !slices.Equal(got, in) {
		t.Fatalf("got %+v", got)
	}
}

func TestParseFieldsAreFills(t *testing.T) {
	ins := clean(t, "Pay ==€ 120== by ==date==", docOptions)[0].Inlines
	var fills []string
	for _, in := range ins {
		if in.Fill {
			fills = append(fills, in.Text)
		}
	}
	if !slices.Equal(fills, []string{"€ 120", "date"}) {
		t.Fatalf("fills %q in %+v", fills, ins)
	}
}

func TestTie(t *testing.T) {
	const nb = " "
	for in, want := range map[string]string{
		"5 kg of flour":        "5" + nb + "kg of flour",
		"costs 12 €":           "costs 12" + nb + "€",
		"at 10 Uhr.":           "at 10" + nb + "Uhr.",
		"5 %":                  "5" + nb + "%",
		"5 kgs":                "5 kgs",
		"5 mice":               "5 mice",
		"z. B. heute":          "z." + nb + "B. heute",
		"Musikverein e. V.":    "Musikverein e." + nb + "V.",
		"nach § 14 BGB":        "nach §" + nb + "14 BGB",
		"Nr. 5 und S. 12":      "Nr." + nb + "5 und S." + nb + "12",
		"am 1. Dezember":       "am 1." + nb + "Dezember",
		"Kapitel 1. dann":      "Kapitel 1. dann",
		"nospace":              "nospace",
		"Seite 2 von 3":        "Seite 2 von 3",
		"(z. B. 5 kg)":         "(z." + nb + "B. 5" + nb + "kg)",
		"price 3 EUR and 4 m.": "price 3" + nb + "EUR and 4" + nb + "m.",
	} {
		if got := Tie(in); got != want {
			t.Errorf("Tie(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTiesText(t *testing.T) {
	if got := Plain(clean(t, "Due by 1. Dezember, 5 kg", docOptions)[0].Inlines); got != "Due by 1. Dezember, 5 kg" {
		t.Fatalf("got %q", got)
	}
}

func TestTypographicQuotesFollowTheLanguage(t *testing.T) {
	for _, tc := range []struct{ lang, want string }{
		{"en", "“Hi” ‘you’ it’s – … —"},
		{"en-GB", "“Hi” ‘you’ it’s – … —"},
		{"de", "„Hi“ ‚you‘ it’s – … —"},
		{"de-AT", "„Hi“ ‚you‘ it’s – … —"},
		{"fr", "« Hi » ‘you’ it’s – … —"},
		{"pl", "„Hi” ‘you’ it’s – … —"},
		{"", "“Hi” ‘you’ it’s – … —"},
	} {
		blocks := clean(t, `"Hi" 'you' it's -- ... ---`, Options{Lang: tc.lang})
		if got := Plain(blocks[0].Inlines); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.lang, got, tc.want)
		}
	}
}

func cells(rows ...string) *Table {
	t := &Table{}
	for _, r := range rows {
		var row [][]Inline
		for _, c := range strings.Split(r, "|") {
			row = append(row, []Inline{{Text: c}})
		}
		t.Rows = append(t.Rows, row)
	}
	if len(t.Rows) > 0 {
		t.Align = make([]string, len(t.Rows[0]))
	}
	return t
}

func TestNumericColumns(t *testing.T) {
	tb := cells("Tea|€ 4.50|12 %|(1,200)|ok", "Cake|1.234,56 EUR|−3|$12|2 kg", "Pie||7|£ 1 000|x")
	for col, want := range []bool{false, true, true, true, false} {
		if got := Numeric(tb, col); got != want {
			t.Errorf("column %d numeric %v", col, got)
		}
	}
	if Numeric(cells("|", "|"), 0) {
		t.Error("an empty column counts as numeric")
	}
	if Numeric(tb, 9) {
		t.Error("a column past the rows counts as numeric")
	}
}

func TestAlignsKeepWrittenAndRightAlignNumbers(t *testing.T) {
	tb := cells("Tea|4|5|x", "Cake|6|7|y")
	tb.Align = []string{"", "", "center", "right"}
	if got := Aligns(tb); !slices.Equal(got, []string{"left", "right", "center", "right"}) {
		t.Fatalf("aligns %v", got)
	}
}

func TestWidthsShareTheTableByContent(t *testing.T) {
	tb := cells("A long description of the item that goes on|3", "short|12")
	tb.Head = [][]Inline{{{Text: "Item"}}, {{Text: "Qty"}}}
	w := Widths(tb)
	if len(w) != 2 || w[0] <= w[1] {
		t.Fatalf("widths %v", w)
	}
	if sum := w[0] + w[1]; sum < 0.9999 || sum > 1.0001 {
		t.Errorf("widths sum to %v", sum)
	}
	// Prose caps at 36 characters, and the narrowest column at 6, each plus 3.
	if want := 39.0 / 48; w[0] < want-1e-9 || w[0] > want+1e-9 {
		t.Errorf("prose share %v, want %v", w[0], want)
	}
}

func TestHTMLEscapesTextAndLinks(t *testing.T) {
	var b strings.Builder
	HTML(&b, []Inline{
		{Text: "a < b & \"c\""},
		{Text: "bold", Bold: true, Italic: true},
		{Text: "fill", Fill: true},
		{Text: "x", Link: `https://e.org/?a=1&b="2"`},
		{Text: "next", Break: true},
	})
	want := `a &lt; b &amp; &#34;c&#34;<em><strong>bold</strong></em><span class="fill">fill</span><a href="https://e.org/?a=1&amp;b=&#34;2&#34;">x</a><br>next`
	if b.String() != want {
		t.Fatalf("got  %s\nwant %s", b.String(), want)
	}
}

func TestPlainJoinsRunsWithBreaks(t *testing.T) {
	if got := Plain([]Inline{{Text: "a"}, {Text: "b", Bold: true}, {Break: true}, {Text: "c"}}); got != "ab\nc" {
		t.Fatalf("plain %q", got)
	}
}

func TestSampleFlagsFiller(t *testing.T) {
	issues := Sample("Contact John Doe at Acme", "")
	if len(issues) != 1 || !strings.Contains(issues[0].Message, `"John Doe"`) {
		t.Fatalf("issues %+v", issues)
	}
	if issues := Sample("Lorem ipsum dolor", ""); len(issues) != 1 {
		t.Errorf("lorem: %+v", issues)
	}
	if issues := Sample("Maria Keller, Lindenweg 4, Bremen", ""); len(issues) != 0 {
		t.Errorf("real sample flagged: %+v", issues)
	}
}

func TestSampleFlagsShouting(t *testing.T) {
	if issues := Sample("THIS OFFER ENDS VERY SOON TODAY", ""); len(issues) != 1 || !strings.Contains(issues[0].Message, "capitals") {
		t.Fatalf("issues %+v", issues)
	}
	if issues := Sample("The NASA and ESA budget", ""); len(issues) != 0 {
		t.Errorf("acronyms flagged: %+v", issues)
	}
}

func TestSampleFlagsEmojiButNotAllowedMarks(t *testing.T) {
	if issues := Sample("Great job 🎉", ""); len(issues) != 1 || !strings.Contains(issues[0].Message, "emoji") {
		t.Fatalf("issues %+v", issues)
	}
	if issues := Sample("☐ open ❧ © ® ™", "☐❧"); len(issues) != 0 {
		t.Errorf("allowed marks flagged: %+v", issues)
	}
	if issues := Sample("☐ open", ""); len(issues) != 1 {
		t.Errorf("unlisted mark passed: %+v", issues)
	}
	if issues := Sample("🎉 🚀 ✨", ""); len(issues) != 1 {
		t.Errorf("one issue per sample, got %+v", issues)
	}
}
