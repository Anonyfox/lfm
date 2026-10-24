package release

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func metricReport(t *testing.T, id string, value, score float64) *LighthouseResult {
	t.Helper()
	raw := fmt.Sprintf(`{"lighthouseVersion":"13.5.0","categories":{"performance":{"score":%g,"auditRefs":[{"id":%q,"weight":1}]},"accessibility":{"score":1},"best-practices":{"score":1},"seo":{"score":1}},"audits":{%q:{"title":"Metric","score":0.7,"numericValue":%g}}}`, score, id, id, value)
	r, err := parseLighthouse([]byte(raw), "https://page.example")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMetricGoodThresholds(t *testing.T) {
	for id, limit := range metricGood {
		for _, value := range []float64{limit * .74, limit, limit * 1.01} {
			r := metricReport(t, id, value, 1)
			want := 0
			if value > limit {
				want = 1
			}
			if len(r.Findings) != want {
				t.Fatalf("%s %g: %+v", id, value, r.Findings)
			}
		}
	}
}

func TestTimingMedianAndFastPath(t *testing.T) {
	for _, tc := range []struct {
		id             string
		values         []float64
		runs, findings int
	}{
		{"total-blocking-time", []float64{300, 60, 190}, 3, 0},
		{"total-blocking-time", []float64{300, 250, 190}, 3, 1},
		{"largest-contentful-paint", []float64{2000}, 1, 0},
		{"cumulative-layout-shift", []float64{.2}, 1, 1},
	} {
		calls := 0
		result := measureLighthouse(context.Background(), func(context.Context) *LighthouseResult {
			value := tc.values[calls]
			calls++
			return metricReport(t, tc.id, value, float64(100-calls)/100)
		})
		if calls != tc.runs || result.Runs != calls || len(result.Findings) != tc.findings {
			t.Fatalf("%+v: calls %d result %+v", tc, calls, result)
		}
		if calls == 3 && (!strings.Contains(result.Metrics[tc.id], "median of 3") || *result.Scores["performance"] != 98) {
			t.Fatal(result)
		}
	}
}

func TestThirdPartyRowsAndShiftSources(t *testing.T) {
	r := metricReport(t, "cumulative-layout-shift", .2, 1)
	var report lhReport
	if err := json.Unmarshal(r.raw, &report); err != nil {
		t.Fatal(err)
	}
	report.Audits["layout-shift-elements"] = lhAudit{Details: json.RawMessage(`{"items":[{"node":{"selector":"#hero","snippet":"<img id=hero>"},"score":0.2}]}`)}
	zero := 0.0
	report.Audits["legacy-javascript"] = lhAudit{Title: "Legacy", Score: &zero, Details: json.RawMessage(`{"items":[{"url":"https://beacon.example/x.js","wastedBytes":10000}]}`)}
	raw, _ := json.Marshal(report)
	got, err := parseLighthouse(raw, "https://page.example")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ThirdParty) != 1 || len(got.Findings) != 1 || !strings.Contains(strings.Join(got.Findings[0].Samples, " "), "#hero") || !strings.Contains(got.Findings[0].Samples[0], "score=0.2") {
		t.Fatalf("%+v", got)
	}
	if len(LighthouseDiagnostics(got)) != 1 {
		t.Fatal("third party became warning")
	}
}

func FuzzLighthouseReport(f *testing.F) {
	f.Add([]byte(fakeReport), "https://example.org")
	f.Add([]byte(`{"audits":{}}`), "")
	f.Fuzz(func(t *testing.T, raw []byte, origin string) {
		result, err := parseLighthouse(raw, origin)
		if err == nil {
			_ = LighthouseDiagnostics(result)
			_ = medianLighthouse([]*LighthouseResult{result, result, result}, 0)
		}
	})
}

func TestDeterministicAuditsDoNotRetry(t *testing.T) {
	for _, id := range []string{"color-contrast", "legacy-javascript", "unused-javascript", "uses-responsive-images"} {
		calls := 0
		result := measureLighthouse(context.Background(), func(context.Context) *LighthouseResult {
			calls++
			return metricReport(t, id, 10000, 1)
		})
		if calls != 1 || result.Runs != 1 {
			t.Fatal(id, calls, result)
		}
	}
}

func TestTimingDiagnosticDoesNotRemeasure(t *testing.T) {
	calls := 0
	result := measureLighthouse(context.Background(), func(context.Context) *LighthouseResult {
		calls++
		raw := strings.Replace(fakeReport, `"unused-css-rules"`, `"max-potential-fid"`, -1)
		raw = strings.Replace(raw, `"weight":1`, `"weight":0`, 1)
		r, err := parseLighthouse([]byte(raw), "http://local")
		if err != nil {
			t.Fatal(err)
		}
		return r
	})
	if calls != 1 || result.Runs != 1 || len(result.Findings) != 0 || len(result.Info) != 1 || len(LighthouseDiagnostics(result)) != 0 {
		t.Fatal(calls, result)
	}
}

func TestTimingRemeasurementFailureAndCancellation(t *testing.T) {
	for _, successful := range []int{1, 2} {
		calls := 0
		result := measureLighthouse(context.Background(), func(context.Context) *LighthouseResult {
			calls++
			if calls > successful {
				return &LighthouseResult{Status: "skipped", Reason: "audit deadline exceeded"}
			}
			return metricReport(t, "total-blocking-time", float64(400-calls*50), 1)
		})
		if result.Runs != successful || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "remeasurement failed") || len(result.Findings) != 1 {
			t.Fatal(result)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	result := measureLighthouse(ctx, func(context.Context) *LighthouseResult {
		calls++
		cancel()
		return metricReport(t, "total-blocking-time", 300, 1)
	})
	if calls != 1 || result.Runs != 1 {
		t.Fatal(calls, result)
	}
	calls = 0
	result = measureLighthouse(context.Background(), func(context.Context) *LighthouseResult {
		calls++
		return &LighthouseResult{Status: "skipped", Reason: "missing tool"}
	})
	if calls != 1 || result.Status != "skipped" {
		t.Fatal(calls, result)
	}
}

func TestMedianNormalizesArtifactOrigins(t *testing.T) {
	var runs []*LighthouseResult
	for i, value := range []float64{300, 250, 400} {
		r := metricReport(t, "total-blocking-time", value, 1)
		origin := fmt.Sprintf("http://127.0.0.1:%d", 10000+i)
		var report lhReport
		if err := json.Unmarshal(r.raw, &report); err != nil {
			t.Fatal(err)
		}
		unscored := report.Categories["performance"]
		unscored.Score = nil
		report.Categories["unscored"] = unscored
		score := 0.5
		report.Audits["unused-javascript"] = lhAudit{Title: "Unused JavaScript", Score: &score, Details: json.RawMessage(fmt.Sprintf(`{"items":[{"url":%q}]}`, origin+"/app.js"))}
		raw, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parseLighthouse(raw, origin)
		if err != nil {
			t.Fatal(err)
		}
		runs = append(runs, parsed)
	}
	result := medianLighthouse(runs, 1)
	if result.Runs != 3 || len(result.ThirdParty) != 0 || len(result.Info) != 1 || result.Info[0].Samples[0] != "url=/app.js" {
		t.Fatal(result)
	}
}

func TestAuditsSplitFirstAndThirdPartyRows(t *testing.T) {
	const own = `{"url":"https://page.example/app.js","wastedBytes":100}`
	const foreign = `{"sourceURL":"https://beacon.example/beacon.js","wastedBytes":10000}`
	for _, tc := range []struct {
		rows            string
		findings, third int
	}{{own, 1, 0}, {foreign, 0, 1}, {own + "," + foreign, 1, 1}} {
		var report lhReport
		raw := strings.ReplaceAll(fakeReport, "unused-css-rules", "unused-javascript")
		if err := json.Unmarshal([]byte(raw), &report); err != nil {
			t.Fatal(err)
		}
		audit := report.Audits["unused-javascript"]
		audit.Details = json.RawMessage(`{"items":[` + tc.rows + `]}`)
		report.Audits["unused-javascript"] = audit
		encoded, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		result, err := parseLighthouse(encoded, "https://page.example")
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Findings) != tc.findings || len(result.ThirdParty) != tc.third || len(LighthouseDiagnostics(result)) != tc.findings {
			t.Fatal(result)
		}
		if tc.findings > 0 && (result.Findings[0].Items != 1 || strings.Contains(strings.Join(result.Findings[0].Samples, " "), "beacon.example")) {
			t.Fatal(result.Findings)
		}
		if tc.third > 0 && !strings.Contains(result.ThirdParty[0].Samples[0], "beacon.example") {
			t.Fatal(result.ThirdParty)
		}
	}
}

func TestPartialRemeasurementUsesArithmeticMedian(t *testing.T) {
	calls := 0
	result := measureLighthouse(context.Background(), func(context.Context) *LighthouseResult {
		calls++
		switch calls {
		case 1:
			return metricReport(t, "total-blocking-time", 300, .8)
		case 2:
			return metricReport(t, "total-blocking-time", 100, 1)
		default:
			return &LighthouseResult{Status: "skipped", Reason: "deadline exceeded"}
		}
	})
	if result.Runs != 2 || len(result.Findings) != 0 || *result.Scores["performance"] != 90 || !strings.Contains(result.Metrics["total-blocking-time"], "median of 2 runs: 200") || len(result.Warnings) != 1 {
		t.Fatal(result)
	}
}

func TestNonNumericAuditMedian(t *testing.T) {
	var runs []*LighthouseResult
	for _, score := range []string{"0.2", "0.8"} {
		raw := strings.Replace(fakeReport, `"score":0.5`, `"score":`+score, 1)
		result, err := parseLighthouse([]byte(raw), "https://page.example")
		if err != nil {
			t.Fatal(err)
		}
		runs = append(runs, result)
	}
	result := medianLighthouse(runs, 1)
	if result.Runs != 2 || len(result.Findings) != 1 || *result.Findings[0].Score != .5 {
		t.Fatal(result)
	}
}
