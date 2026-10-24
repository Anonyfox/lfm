package check

import (
	"strings"
	"testing"
)

func TestImmersiveMedia(t *testing.T) {
	for _, tc := range []struct {
		body, head string
		want       int
	}{
		{`<main><section><h2>Workshop</h2><p>Text</p></section></main>`, "", 1},
		{`<main><section><h2>Workshop</h2><img src=a></section><footer>Legal</footer></main>`, "", 0},
		{`<main><section><h2>Workshop</h2><svg width=24 height=24></svg></section></main>`, "", 1},
		{`<main><section><svg width=80 height=24></svg></section></main>`, "", 0},
		{`<main><section><canvas></canvas></section></main>`, "", 0},
		{`<main><section><video></video></section></main>`, "", 0},
		{`<main><section class=photo><h2>Workshop</h2></section></main>`, `.photo{background-image:url(a.webp)}`, 0},
		{`<main><h1>Place</h1><img src=a><h2>Workshop</h2><p>Text</p></main>`, "", 1},
		{`<main><section><h2>Workshop</h2><section><img src=a></section></section></main>`, "", 0},
	} {
		for _, intensity := range []string{"", "restrained", "balanced", "immersive"} {
			got := runOn(t, sectionMediaRule, "<style>"+tc.head+"</style>", tc.body, Options{Concept: &ConceptFacts{Intensity: intensity}})
			want := 0
			if intensity == "immersive" {
				want = tc.want
			}
			if len(got) != want {
				t.Errorf("%s %s: %v", intensity, tc.body, got)
			}
			if len(got) > 0 && !strings.Contains(got[0].Message, "Workshop") {
				t.Error(got)
			}
		}
	}
}

func TestVideoLoading(t *testing.T) {
	for _, tc := range []struct {
		attrs string
		first bool
		want  int
	}{
		{`autoplay muted playsinline`, false, 1},
		{`autoplay muted playsinline preload=none poster=a`, false, 1},
		{`autoplay muted playsinline preload=metadata poster=a`, false, 1},
		{`autoplay muted playsinline preload=none`, false, 1},
		{`preload=auto`, false, 1},
		{`controls preload=none`, false, 0},
		{`autoplay`, true, 1},
		{`autoplay muted playsinline`, true, 0},
		{`autoplay muted`, true, 1},
		{`autoplay playsinline`, true, 0},
	} {
		video := `<video ` + tc.attrs + `></video>`
		body := `<main><section><h1>First</h1>`
		if tc.first {
			body += video
		}
		body += `</section><section><h2>Next</h2>`
		if !tc.first {
			body += video
		}
		body += `</section></main>`
		if got := runOn(t, eagerVideoRule, "", body, Options{}); len(got) != tc.want {
			t.Errorf("%+v: %v", tc, got)
		}
	}
}

func TestMediaBudgets(t *testing.T) {
	body := `<main><section><h1>First</h1><img src="a.webp"><img src="a.webp"><video preload=none poster=p.webp><source src=v.mp4></video></section><section><h2>Next</h2><img src=b.webp></section></main>`
	assets := map[string][]byte{"a.webp": make([]byte, 200<<10), "p.webp": make([]byte, 100<<10), "b.webp": make([]byte, 300<<10), "v.mp4": make([]byte, 900<<10)}
	d, err := Parse([]byte(body), Options{Assets: assets})
	if err != nil {
		t.Fatal(err)
	}
	media, video, first := mediaBytes(d)
	if media != 600<<10 || video != 900<<10 || first != 300<<10 {
		t.Fatalf("%d %d %d", media, video, first)
	}
	if got := budgetsRule(d); len(got) != 0 {
		t.Fatal(got)
	}
	d.Options.Budgets = Budgets{ImageKiB: 500, VideoKiB: 800, FirstScreenKiB: 200}
	if got := budgetsRule(d); len(got) != 3 {
		t.Fatal(got)
	}
	d.Options.Budgets = Budgets{}
	assets["a.webp"] = make([]byte, 401<<10)
	if got := budgetsRule(d); len(got) != 1 || !strings.Contains(got[0].Hint, "firstScreenKiB") {
		t.Fatal(got)
	}
	assets["v.mp4"] = make([]byte, 8001<<10)
	if got := budgetsRule(d); len(got) != 2 {
		t.Fatal(got)
	}
}

func FuzzMediaRules(f *testing.F) {
	f.Add(`<main><section><h1>First</h1><video autoplay></video></section></main>`)
	f.Fuzz(func(t *testing.T, raw string) {
		d, err := Parse([]byte(raw), Options{Concept: &ConceptFacts{Intensity: "immersive"}})
		if err != nil {
			return
		}
		sectionMediaRule(d)
		eagerVideoRule(d)
		mediaBytes(d)
	})
}

func TestMediaKnownSizesAndLazyCandidates(t *testing.T) {
	body := `<main><section><h1>First</h1><picture><source srcset="a.avif 1x, b.avif 2x"><img loading=lazy src=a.webp></picture><img src=remote.webp><img src=unknown.webp></section></main>`
	d, err := Parse([]byte(body), Options{Assets: map[string][]byte{"a.avif": make([]byte, 10), "b.avif": make([]byte, 20), "a.webp": make([]byte, 30), "unknown.webp": make([]byte, 5)}, AssetBytes: map[string]int{"remote.webp": 100, "unknown.webp": -1}})
	if err != nil {
		t.Fatal(err)
	}
	media, video, first := mediaBytes(d)
	if media != 160 || video != 0 || first != 100 {
		t.Fatalf("%d %d %d", media, video, first)
	}
}

func TestImmersiveLinkedStyles(t *testing.T) {
	opts := Options{Concept: &ConceptFacts{Intensity: "immersive"}, Stylesheets: map[string][]byte{"styles/main.css": []byte(`.photo{background-image:url(../scene.webp)}`)}, Assets: map[string][]byte{"scene.webp": make([]byte, 200)}}
	body := `<main><section class=photo><h1>Scene</h1></section></main>`
	if got := runOn(t, sectionMediaRule, `<link rel=stylesheet href=styles/main.css>`, body, opts); len(got) != 0 {
		t.Fatal(got)
	}
	d, err := Parse([]byte(`<link rel=stylesheet href=styles/main.css>`+body), opts)
	if err != nil {
		t.Fatal(err)
	}
	media, _, first := mediaBytes(d)
	if media != 200 || first != 200 {
		t.Fatalf("%d %d", media, first)
	}
}

func TestFirstScreenVideoBytes(t *testing.T) {
	for _, attrs := range []struct {
		value string
		eager bool
	}{
		{"", false},
		{"preload=none", false},
		{"preload=metadata", false},
		{"preload=auto", true},
		{"autoplay preload=none", true},
		{"autoplay preload=metadata", true},
		{"autoplay", true},
	} {
		for _, source := range []bool{false, true} {
			video := "<video poster=p.webp " + attrs.value
			if source {
				video += "><source src=v.mp4></video>"
			} else {
				video += " src=v.mp4></video>"
			}
			d, err := Parse([]byte("<main><section>"+video+"</section></main>"), Options{Assets: map[string][]byte{"p.webp": make([]byte, 100), "v.mp4": make([]byte, 4000<<10)}})
			if err != nil {
				t.Fatal(err)
			}
			image, videoBytes, first := mediaBytes(d)
			want := 100
			if attrs.eager {
				want += 4000 << 10
			}
			if image != 100 || videoBytes != 4000<<10 || first != want {
				t.Fatalf("%s source=%v: %d %d %d", attrs.value, source, image, videoBytes, first)
			}
			if !attrs.eager && len(budgetsRule(d)) != 0 {
				t.Fatalf("deferred video charged to image budget: %v", budgetsRule(d))
			}
		}
	}
}

func TestTextWalls(t *testing.T) {
	words := func(n int) string { return strings.Repeat("word ", n) }
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"over", "<p>" + words(151) + "</p><div><p>" + words(150) + "</p></div>", 1},
		{"background section", `<div style="background-image:url(scene.webp)"><p>` + words(301) + `</p></div>`, 1},
		{"boundary", "<p>" + words(300) + "</p>", 0},
		{"list", "<p>" + words(200) + "</p><ul><li><p>" + words(200) + "</p></li></ul><p>" + words(200) + "</p>", 0},
		{"heading", "<p>" + words(200) + "</p><h3>Break</h3><p>" + words(200) + "</p>", 0},
		{"media", "<p>" + words(200) + "</p><img src=a><p>" + words(200) + "</p>", 0},
		{"icon", "<p>" + words(200) + "</p><svg width=24 height=24></svg><p>" + words(200) + "</p>", 1},
		{"hidden", "<p>" + words(300) + "<span hidden> extra</span></p><p hidden>" + words(400) + "</p><template><p>" + words(400) + "</p></template><nav><p>" + words(400) + "</p></nav><footer><p>" + words(400) + "</p></footer>", 0},
		{"separate runs", "<p>" + words(301) + "</p><hr><h3>Break</h3><p>" + words(301) + "</p>", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runOn(t, sectionMediaRule, "", "<main><section><h2>Story</h2>"+tc.body+"</section></main>", Options{})
			if len(got) != tc.want {
				t.Fatalf("%+v", got)
			}
			count := "301 words"
			if tc.name == "icon" {
				count = "400 words"
			}
			for _, d := range got {
				if !strings.Contains(d.Message, "Story") || !strings.Contains(d.Message, count) {
					t.Fatal(d)
				}
			}
		})
	}
	for _, tag := range []string{"table", "figure", "blockquote", "details", "form"} {
		body := "<main><p>" + words(200) + "</p><" + tag + ">Break</" + tag + "><p>" + words(200) + "</p></main>"
		if got := runOn(t, sectionMediaRule, "", body, Options{}); len(got) != 0 {
			t.Errorf("%s: %+v", tag, got)
		}
	}
}
