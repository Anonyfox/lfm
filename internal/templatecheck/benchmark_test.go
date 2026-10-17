package templatecheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Anonyfox/lfm/internal/jsonsrc"
)

func BenchmarkPreflight(b *testing.B) {
	for _, rows := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("rows%d", rows), func(b *testing.B) {
			var dataset strings.Builder
			dataset.WriteByte('[')
			for i := 0; i < rows; i++ {
				if i > 0 {
					dataset.WriteByte(',')
				}
				fmt.Fprintf(&dataset, `{"id":"q%d","question":"Question %d","answer":"Answer %d"}`, i, i, i)
			}
			dataset.WriteByte(']')
			raw := []byte(dataset.String())
			files := map[string][]byte{"index.html.tmpl": []byte(`{{range .data.faq}}<details id="{{.id}}"><summary>{{.question}}</summary><p>{{.answer}}</p></details>{{end}}`)}
			values := map[string]any{"data": map[string]any{"faq": func() any {
				var items any
				d := json.NewDecoder(bytes.NewReader(raw))
				d.UseNumber()
				if err := d.Decode(&items); err != nil {
					b.Fatal(err)
				}
				return items
			}()}}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sources := jsonsrc.Index(map[string][]byte{"data/faq.json": raw})
				template, parsed := Parse(files, "index.html.tmpl")
				if parsed.Errors > 0 {
					b.Fatal(messages(parsed))
				}
				s := Check(template, Options{Entry: "index.html.tmpl", Files: files, Sources: sources, Values: values})
				if s.Errors > 0 || s.Warnings > 0 {
					b.Fatal(messages(s))
				}
			}
		})
	}
}
