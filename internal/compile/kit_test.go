package compile

import (
	"bytes"
	"context"
	"errors"
	"github.com/Anonyfox/lfm/internal/zipx"
	"reflect"
	"testing"
	"time"

	"github.com/Anonyfox/lfm/internal/deliver"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/release"
)

func TestListingImagesMaterializeOnlyDemandedMisses(t *testing.T) {
	for _, tc := range []struct {
		name, sell    string
		release, fail bool
		limit, want   int
		pending       bool
	}{
		{"bundle", deliver.SellBundle, true, false, 2, 2, false},
		{"shops", deliver.SellShops, true, false, 2, 3, false},
		{"ordinary", deliver.SellBundle, false, false, 2, 1, true},
		{"failure", deliver.SellBundle, true, true, 2, 1, true},
		{"no demand", deliver.SellBundle, true, false, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := newStock(context.Background(), project.Project{}, tc.release)
			if tc.release && tc.limit > 0 && testing.Short() {
				t.Skip("browser integration")
			}
			defer st.close()
			st.demand = map[string]int{"test": tc.limit}
			plans := []listingPlan{{Name: "a", Key: "a"}, {Name: "b", Key: "b"}, {Name: "c", Key: "c"}}
			hit := deliver.StampJPEG([]byte{255, 216, 255, 217}, "a")
			st.previous["a"] = hit
			calls := 0
			render := func(_ *release.Printer, missing []listingPlan) ([]listingPlan, map[string][]byte, error) {
				calls++
				for i, p := range missing {
					if p.Key == "a" || tc.sell == deliver.SellBundle && p.Key == "c" {
						t.Fatalf("undemanded render %s", p.Key)
					}
					missing[i].Doc = []byte(`<!doctype html><p>Listing image</p>`)
				}
				if tc.fail {
					return nil, nil, errors.New("failed")
				}
				return missing, nil, nil
			}
			pr := st.press()
			images := listingImages(pr, "test", tc.sell, plans, render)
			if len(images) != tc.want || pr.pending != tc.pending || tc.want > 0 && !bytes.Equal(images[0].Data, hit) {
				t.Fatalf("images=%v pending=%v", images, pr.pending)
			}
			if tc.release && tc.limit > 0 && calls != 1 {
				t.Fatal("dependency preparations", calls)
			}
		})
	}
}

func TestDocumentImageHitsDoNotCaptureShowcase(t *testing.T) {
	p, err := project.Open(documentProject(t))
	if err != nil {
		t.Fatal(err)
	}
	r, _ := checkDocument(p, project.Load(p, ruleCheck), checking{}.commerce)
	built := r.Jobs()
	plans := documentPlans(r, built, nil)
	if len(plans) == 0 {
		t.Fatal("no image plans")
	}
	st := newStock(context.Background(), p, true)
	st.started = true
	st.startErr = errors.New("must not start Chrome")
	for _, plan := range plans {
		st.previous[plan.Key] = deliver.StampJPEG([]byte{255, 216, 255, 217}, plan.Key)
	}
	var previous []deliver.Image
	for _, release := range []bool{false, true} {
		st.release = release
		pr := st.press()
		got := documentImages(pr, r, built, nil)
		if len(got) != len(plans) || pr.pending || previous != nil && !reflect.DeepEqual(previous, got) {
			t.Fatalf("release=%v images=%d pending=%v", release, len(got), pr.pending)
		}
		previous = got
	}
}

func TestPrintChecksUseSuccessfulLookup(t *testing.T) {
	if testing.Short() {
		t.Skip("browser integration")
	}
	st := newStock(context.Background(), project.Project{}, true)
	defer st.close()
	pr := st.press()
	st.files["key"] = "missing.pdf"
	checked := 0
	jobs := []printJob{{key: "key", slot: "pdf", doc: func() []byte { return []byte(`<!doctype html><p>Print</p>`) }, checks: func([]byte, *release.Printer) []finding { checked++; return nil }}}
	for _, want := range []int{1, 1} {
		pdfs, _ := printAll(pr, jobs, nil, "document.layout", &warnings{})
		if checked != want || pdfs["pdf"] == nil {
			t.Fatalf("checks=%d PDFs=%d", checked, len(pdfs))
		}
	}
}

func TestStockSeparatesArchiveAndMemberStamps(t *testing.T) {
	for _, key := range []string{"", "edition"} {
		t.Run("key="+key, func(t *testing.T) {
			pdf := []byte("%PDF-1.4\n/LfmBuild (0123456789abcdef)\n%%EOF\n")
			pack := func(body, key string) []byte {
				var b bytes.Buffer
				z := zipx.New(&b, time.Time{})
				if err := errors.Join(z.Store("body.txt", []byte(body)), z.Store("Start here.pdf", pdf), z.Close()); err != nil {
					t.Fatal(err)
				}
				if key != "" {
					return zipx.WithStamp(b.Bytes(), key)
				}
				return b.Bytes()
			}
			root := t.TempDir()
			data := pack("old", key)
			write(t, root, "output/edition.zip", string(data))
			st := newStock(context.Background(), project.Project{Root: root}, false)
			pr := st.press()
			if !bytes.Equal(pr.lookup("0123456789abcdef"), pdf) || !bytes.Equal(pr.lookup(key), map[bool][]byte{true: data}[key != ""]) {
				t.Fatal("archive reused as its PDF member")
			}
			nextKey := ""
			if key != "" {
				nextKey = "changed"
			}
			if holds(root+"/output", map[string][]byte{"edition.zip": pack("new", nextKey)}) {
				t.Fatal("changed archive hidden by its member stamp")
			}
		})
	}
}
