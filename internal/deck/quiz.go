package deck

import (
	"fmt"
	"html"
	"strings"

	"github.com/Anonyfox/lfm/internal/markup"
	"github.com/Anonyfox/lfm/internal/typeface"
)

// question is a quiz slide as the sheet asks it: the text above its
// answers, or the slide's title when it has none.
func question(s Slide) []Inline {
	var out []Inline
	for _, b := range s.Body {
		if b.Kind == Para {
			if out != nil {
				out = append(out, Inline{Text: " "})
			}
			out = append(out, b.Inlines...)
		}
	}
	if out == nil {
		return s.Title
	}
	return out
}

// quizPrint is a deck's quiz as a sheet to hand out: every question with
// its answers to tick, a name and date line, and the answer key on a page
// of its own, so the presenter can take it off before copying.
func quizPrint(r render) []byte {
	k := r.k
	w := wording(r.deck.Lang)
	open, _, boxFont := typeface.Checkboxes(k.text)
	var b strings.Builder
	fmt.Fprintf(&b, `<!doctype html><html lang="%s"><head><meta charset="utf-8"><title>%s</title><style>%s
@page{size:%.2fmm %.2fmm;margin:20mm 20mm 22mm}
body{margin:0;font:12pt/1.45 %q,serif;color:%s}
h1{font:%s 22pt/1.15 %q,serif;color:%s;margin:0 0 4mm}
.who{display:flex;gap:10mm;margin:0 0 9mm;color:%s;font-size:10pt}
.who span{flex:1;border-bottom:.5pt solid %s;padding-bottom:6mm}
ol{margin:0;padding-left:7mm}
li.q{margin:0 0 6mm;break-inside:avoid}
.opts{list-style:none;padding:0;margin:2mm 0 0}
.opts li{margin:1.2mm 0}
.box{font-family:%q;font-size:1.35em;line-height:1;vertical-align:-.12em;margin-right:2mm}
.key{break-before:page}
.key td{padding:1.5mm 6mm 1.5mm 0;border-bottom:.5pt solid %s}
@media screen{body{padding:20mm;background:#fff}}
</style></head><body>`, html.EscapeString(r.deck.Lang), html.EscapeString(r.deck.Title), typeface.FontFaces("", k.text, k.heading),
		r.paper.Width, r.paper.Height, k.Text, k.Ink, map[bool]string{true: "700", false: "400"}[k.HeadingBold], k.Heading, map[bool]string{true: k.Accent, false: k.Ink}[k.HeadingAccent], k.Muted, k.Muted, boxFont, k.Stroke)
	fmt.Fprintf(&b, `<h1>%s: %s</h1><div class="who"><span>%s</span><span>%s</span></div><ol>`, html.EscapeString(r.deck.Title), html.EscapeString(w.Quiz), html.EscapeString(w.Name), html.EscapeString(w.Date))
	var key strings.Builder
	n := 0
	for _, s := range r.deck.Slides {
		if s.Quiz == nil {
			continue
		}
		n++
		b.WriteString(`<li class="q">`)
		markup.HTML(&b, question(s))
		b.WriteString(`<ul class="opts">`)
		for i, it := range s.Quiz.Items {
			fmt.Fprintf(&b, `<li><span class="box">%s</span>%c. `, open, 'A'+i)
			markup.HTML(&b, it.Inlines)
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul></li>`)
		fmt.Fprintf(&key, `<tr><td>%d</td><td>%s</td></tr>`, n, html.EscapeString(answer(s.Quiz)))
	}
	fmt.Fprintf(&b, `</ol><div class="key"><h1>%s</h1><table>%s</table></div></body></html>`, html.EscapeString(w.AnswerKey), key.String())
	return []byte(b.String())
}
