package aitext

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type sample struct {
	Text  string `json:"text"`
	Label int    `json:"label"` // 1 generated
	Lang  string `json:"lang"`
	Group string `json:"group"`
	Split string `json:"split"` // train or test
	Doc   int    `json:"doc"`   // the document the window comes from
}

func loadCorpus(t *testing.T, path string) []sample {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []sample
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var s sample
		if json.Unmarshal(sc.Bytes(), &s) == nil {
			out = append(out, s)
		}
	}
	return out
}

// fit trains L2-regularized logistic regression with balanced class weights.
func fit(x [][]float64, y []int, sw []float64, l2 float64) ([]float64, float64) {
	return fitPrior(x, y, sw, l2, nil, 0)
}

// fitPrior trains L2-regularized logistic regression with balanced class
// weights, pulling the weights towards prior (nil: towards zero).
func fitPrior(x [][]float64, y []int, sw []float64, l2 float64, prior []float64, priorBias float64) ([]float64, float64) {
	d := len(x[0])
	w := make([]float64, d)
	b := priorBias
	copy(w, prior)
	var pos, neg float64
	for i, l := range y {
		if l == 1 {
			pos += sw[i]
		} else {
			neg += sw[i]
		}
	}
	cw := map[int]float64{1: (pos + neg) / (2 * pos), 0: (pos + neg) / (2 * neg)}
	lr := 0.3
	for epoch := 0; epoch < 1200; epoch++ {
		gw := make([]float64, d)
		gb := 0.0
		for i, xi := range x {
			z := b
			for j := range xi {
				z += w[j] * xi[j]
			}
			p := 1 / (1 + math.Exp(-z))
			e := (p - float64(y[i])) * cw[y[i]] * sw[i]
			for j := range xi {
				gw[j] += e * xi[j]
			}
			gb += e
		}
		n := float64(len(x))
		for j := range w {
			p := 0.0
			if prior != nil {
				p = prior[j]
			}
			w[j] -= lr * (gw[j]/n + l2*(w[j]-p))
		}
		b -= lr * gb / n
	}
	return w, b
}

func langProfiles(p *[2]profile, list func([]float64) string) string {
	if p == nil || (p[0].weight == nil && p[1].weight == nil) {
		return ""
	}
	out := "\t\t\tweights: [2]profile{\n"
	for _, pr := range p {
		if pr.weight == nil {
			out += "\t\t\t\t{},\n"
			continue
		}
		out += fmt.Sprintf("\t\t\t\t{weight: []float64%s, bias: %.6g},\n", list(pr.weight), pr.bias)
	}
	return out + "\t\t\t},\n"
}

// weightOf weighs human text in the genres lfm checks above forum posts and
// news, so the thresholds hold on polished human prose.
func weightOf(group string) float64 {
	for _, p := range []string{"essays", "landing", "zeitzuleben", "dewiki", "wiki", "gutenberg", "baking", "ts2026-literature", "zenhabits", "web-c4"} {
		if strings.HasPrefix(group, p) {
			return 4
		}
	}
	return 1
}

func median(v []float64) float64 {
	s := append([]float64{}, v...)
	sort.Float64s(s)
	if len(s) == 0 {
		return 0
	}
	return s[len(s)/2]
}

// TestTrain fits the model from the corpus in LFM_DETECTOR_CORPUS (JSON
// lines of text, label, lang, group, split) and writes models_gen.go when
// LFM_DETECTOR_WRITE is set.
//
// Features are standardized against each language's human writing, all
// languages are pooled, and two profiles are fitted: older models against
// people, and current models against people. Every source is scored by a
// fit without it (leave one source out), and each language's threshold
// keeps at most 1% of its unseen human texts above it, and at most 2% of
// those in the genres lfm checks.
func TestTrain(t *testing.T) {
	path := os.Getenv("LFM_DETECTOR_CORPUS")
	if path == "" {
		t.Skip("set LFM_DETECTOR_CORPUS")
	}
	modern := func(group string) bool {
		for _, p := range []string{"claude", "gpt-", "ts2026-ai"} {
			if strings.HasPrefix(group, p) {
				return true
			}
		}
		return false
	}
	type item struct {
		raw   []float64
		label int
		lang  string
		group string // lang/source
		w     float64
		doc   int
	}
	var items []item
	skip := os.Getenv("LFM_DETECTOR_SKIP")
	for _, s := range loadCorpus(t, path) {
		if skip != "" && strings.HasPrefix(s.Group, skip) {
			continue
		}
		f, n := Measure(Paragraphs(s.Text), s.Lang)
		if n < MinWords {
			continue
		}
		items = append(items, item{f, s.Label, s.Lang, s.Lang + "/" + s.Group, weightOf(s.Group), s.Doc})
	}
	d := len(FeatureNames)

	// baselines: mean and scale of human text per language, and pooled for "other"
	baselines := map[string]*baseline{}
	stats := func(rows [][]float64) *baseline {
		b := &baseline{mean: make([]float64, d), scale: make([]float64, d), human: make([]float64, d)}
		for j := 0; j < d; j++ {
			var col []float64
			for _, r := range rows {
				col = append(col, r[j])
			}
			m, sq := 0.0, 0.0
			for _, v := range col {
				m += v
			}
			m /= float64(len(col))
			for _, v := range col {
				sq += (v - m) * (v - m)
			}
			sd := math.Sqrt(sq / float64(len(col)))
			if sd < 1e-9 {
				sd = 1
			}
			b.mean[j], b.scale[j], b.human[j] = m, sd, median(col)
		}
		return b
	}
	humanRows := map[string][][]float64{}
	for _, it := range items {
		if it.label == 0 {
			humanRows[it.lang] = append(humanRows[it.lang], it.raw)
			humanRows["other"] = append(humanRows["other"], it.raw)
		}
	}
	langs := []string{}
	for l := range humanRows {
		baselines[l] = stats(humanRows[l])
		if l != "other" {
			langs = append(langs, l)
		}
	}
	sort.Strings(langs)
	zOf := func(it item, agnostic bool) []float64 {
		b := baselines[it.lang]
		if agnostic {
			b = baselines["other"]
		}
		z := make([]float64, d)
		for j := range z {
			if agnostic && languageSpecific[j] {
				continue
			}
			z[j] = (it.raw[j] - b.mean[j]) / b.scale[j]
			if capped[j] && z[j] < 0 {
				z[j] = 0
			}
		}
		return z
	}

	var weights [2][2]profile // [agnostic][old, modern]
	langWeights := map[string]*[2]profile{}
	for _, l := range langs {
		langWeights[l] = &[2]profile{}
	}
	thresholds := map[string]*[2]float64{}
	docThresholds := map[string]*[2]float64{}
	for _, l := range append(langs, "other") {
		thresholds[l] = &[2]float64{0.5, 0.5}
		docThresholds[l] = &[2]float64{}
	}
	for ag := 0; ag < 2; ag++ {
		for pr, name := range []string{"old", "modern"} {
			var set []item
			forum := 0
			for _, it := range items {
				if it.label == 1 && modern(it.group[strings.Index(it.group, "/")+1:]) != (pr == 1) {
					continue
				}
				// the modern profile judges books, pages and essays: a sample
				// of forum and Q&A text keeps it honest there
				if pr == 1 && it.label == 0 && it.w == 1 {
					if forum++; forum%8 != 0 {
						continue
					}
				}
				set = append(set, it)
			}
			x := make([][]float64, len(set))
			y := make([]int, len(set))
			sw := make([]float64, len(set))
			for i, it := range set {
				x[i], y[i], sw[i] = zOf(it, ag == 1), it.label, it.w
			}
			w, b := fit(x, y, sw, 0.03)
			weights[ag][pr] = profile{w, b}
			// Each language refines the shared weights on its own data, held
			// close to them: a language with little data stays near the
			// shared fit.
			pull := 0.3
			if v := os.Getenv("LFM_DETECTOR_PULL"); v != "" {
				pull, _ = strconv.ParseFloat(v, 64)
			}
			langFit := func(lang string, exclude string, prior []float64, priorBias float64) ([]float64, float64, bool) {
				var lx [][]float64
				var ly []int
				var lw []float64
				classes := map[int]bool{}
				for i, it := range set {
					if it.lang == lang && it.group != exclude {
						lx, ly, lw = append(lx, x[i]), append(ly, y[i]), append(lw, sw[i])
						classes[y[i]] = true
					}
				}
				if len(classes) < 2 || len(lx) < 60 {
					return prior, priorBias, false
				}
				// languages with much data follow their own data more
				p := pull * math.Min(1, 600/float64(len(lx)))
				fw, fb := fitPrior(lx, ly, lw, p, prior, priorBias)
				return fw, fb, true
			}
			if ag == 0 {
				for _, l := range langs {
					if fw, fb, ok := langFit(l, "", w, b); ok {
						langWeights[l][pr] = profile{fw, fb}
					}
				}
			}
			// leave one source out
			unseen := make([]float64, len(set))
			sources := map[string][]int{}
			for i, it := range set {
				sources[it.group] = append(sources[it.group], i)
			}
			for g, members := range sources {
				var gx [][]float64
				var gy []int
				var gw []float64
				classes := map[int]bool{}
				for i := range set {
					if set[i].group != g {
						gx, gy, gw = append(gx, x[i]), append(gy, y[i]), append(gw, sw[i])
						classes[y[i]] = true
					}
				}
				if len(classes) < 2 {
					for _, i := range members {
						unseen[i] = math.NaN()
					}
					continue
				}
				lw, lb := fit(gx, gy, gw, 0.03)
				if ag == 0 {
					// the held-out source's language, refined without it
					if fw, fb, ok := langFit(set[members[0]].lang, g, lw, lb); ok {
						lw, lb = fw, fb
					}
				}
				for _, i := range members {
					s := lb
					for j, v := range x[i] {
						s += lw[j] * v
					}
					unseen[i] = 1 / (1 + math.Exp(-s))
				}
			}
			// thresholds per language (agnostic: one for "other")
			targets := langs
			if ag == 1 {
				targets = []string{"other"}
			}
			for _, l := range targets {
				var all, matched []float64
				for i, it := range set {
					if it.label == 0 && (l == "other" || it.lang == l) && !math.IsNaN(unseen[i]) {
						all = append(all, unseen[i])
						if it.w > 1 {
							matched = append(matched, unseen[i])
						}
					}
				}
				if len(all) == 0 {
					continue
				}
				sort.Float64s(all)
				sort.Float64s(matched)
				thr := all[int(math.Ceil(0.99*float64(len(all))))-1]
				if len(matched) > 0 {
					thr = math.Max(thr, matched[int(math.Ceil(0.98*float64(len(matched))))-1])
				}
				thr = math.Max(thr, 0.5)
				thresholds[l][pr] = thr
				fmt.Printf("[%s %s%s] threshold %.3f\n", l, name, map[bool]string{true: " agnostic", false: ""}[ag == 1], thr)
				groups := map[string][2]int{}
				for i, it := range set {
					if l != "other" && it.lang != l {
						continue
					}
					g := groups[it.group]
					g[1]++
					if !math.IsNaN(unseen[i]) && unseen[i] >= thr {
						g[0]++
					}
					groups[it.group] = g
				}
				var keys []string
				for k := range groups {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				var aiHit, aiN, huHit, huN int
				for _, k := range keys {
					label := 0
					for i := range set {
						if set[i].group == k {
							label = set[i].label
							break
						}
					}
					if label == 1 {
						aiHit += groups[k][0]
						aiN += groups[k][1]
					} else {
						huHit += groups[k][0]
						huN += groups[k][1]
					}
					if os.Getenv("LFM_DETECTOR_GROUPS") != "" {
						fmt.Printf("    %-40s %d %d/%d\n", k, label, groups[k][0], groups[k][1])
					}
				}
				fmt.Printf("    generated flagged %d/%d (%.0f%%), human flagged %d/%d (%.1f%%)\n", aiHit, aiN, 100*float64(aiHit)/math.Max(1, float64(aiN)), huHit, huN, 100*float64(huHit)/math.Max(1, float64(huN)))

				// Documents of two or more windows: the mean of their windows'
				// unseen-source log-odds, with its own threshold.
				type doc struct {
					sum     float64
					n       int
					label   int
					matched bool
				}
				docs := map[int]*doc{}
				for i, it := range set {
					if (l != "other" && it.lang != l) || math.IsNaN(unseen[i]) {
						continue
					}
					d := docs[it.doc]
					if d == nil {
						d = &doc{label: it.label, matched: it.w > 1}
						docs[it.doc] = d
					}
					p := math.Min(math.Max(unseen[i], 1e-9), 1-1e-9)
					d.sum += math.Log(p / (1 - p))
					d.n++
				}
				var humanDocs, matchedDocs []float64
				for _, d := range docs {
					if d.n >= 2 && d.label == 0 {
						humanDocs = append(humanDocs, d.sum/float64(d.n))
						if d.matched {
							matchedDocs = append(matchedDocs, d.sum/float64(d.n))
						}
					}
				}
				if len(humanDocs) >= 20 {
					sort.Float64s(humanDocs)
					sort.Float64s(matchedDocs)
					dthr := humanDocs[int(math.Ceil(0.99*float64(len(humanDocs))))-1]
					if len(matchedDocs) > 0 {
						dthr = math.Max(dthr, matchedDocs[int(math.Ceil(0.98*float64(len(matchedDocs))))-1])
					}
					docThresholds[l][pr] = dthr
					var aiD, aiDn, huD, huDn int
					for _, d := range docs {
						if d.n < 2 {
							continue
						}
						hit := d.sum/float64(d.n) >= dthr
						if d.label == 1 {
							aiDn++
							if hit {
								aiD++
							}
						} else {
							huDn++
							if hit {
								huD++
							}
						}
					}
					fmt.Printf("    documents of 2+ windows: generated flagged %d/%d (%.0f%%), human flagged %d/%d (%.1f%%), threshold %.2f log-odds\n", aiD, aiDn, 100*float64(aiD)/math.Max(1, float64(aiDn)), huD, huDn, 100*float64(huD)/math.Max(1, float64(huDn)), dthr)
				}
			}
		}
	}

	list := func(v []float64) string {
		parts := make([]string, len(v))
		for i, x := range v {
			parts[i] = fmt.Sprintf("%.6g", x)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	var gen strings.Builder
	gen.WriteString("// Code generated by TestTrain from the detector corpus; DO NOT EDIT.\n\npackage aitext\n\nvar fittedModel = fitted{\n")
	for ag, name := range []string{"full", "agnostic"} {
		fmt.Fprintf(&gen, "\t%s: [2]profile{\n", name)
		for pr := 0; pr < 2; pr++ {
			fmt.Fprintf(&gen, "\t\t{weight: []float64%s, bias: %.6g},\n", list(weights[ag][pr].weight), weights[ag][pr].bias)
		}
		gen.WriteString("\t},\n")
	}
	gen.WriteString("\tlangs: map[string]baseline{\n")
	for _, l := range append(langs, "other") {
		b := baselines[l]
		fmt.Fprintf(&gen, "\t\t%q: {\n\t\t\tmean:      []float64%s,\n\t\t\tscale:     []float64%s,\n\t\t\thuman:     []float64%s,\n\t\t\tthreshold: [2]float64{%.4g, %.4g},\n\t\t\tdocument:  [2]float64{%.4g, %.4g},\n%s\t\t},\n", l, list(b.mean), list(b.scale), list(b.human), thresholds[l][0], thresholds[l][1], docThresholds[l][0], docThresholds[l][1], langProfiles(langWeights[l], list))
	}
	gen.WriteString("\t},\n}\n")
	if os.Getenv("LFM_DETECTOR_WRITE") != "" {
		if err := os.WriteFile("models_gen.go", []byte(gen.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestGroups prints the median of every feature per corpus group.
func TestGroups(t *testing.T) {
	path := os.Getenv("LFM_DETECTOR_CORPUS")
	if path == "" {
		t.Skip("set LFM_DETECTOR_CORPUS")
	}
	cols := map[string][][]float64{}
	for _, s := range loadCorpus(t, path) {
		f, n := Measure(Paragraphs(s.Text), s.Lang)
		if n < MinWords {
			continue
		}
		key := s.Lang + " " + s.Group
		cols[key] = append(cols[key], f)
	}
	keys := make([]string, 0, len(cols))
	for k := range cols {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("%-30s", "group")
	for _, n := range FeatureNames {
		fmt.Printf(" %6.6s", n)
	}
	fmt.Println()
	for _, k := range keys {
		fmt.Printf("%-30s", k)
		for j := range FeatureNames {
			var v []float64
			for _, f := range cols[k] {
				v = append(v, f[j])
			}
			fmt.Printf(" %6.3f", median(v))
		}
		fmt.Println()
	}
}
