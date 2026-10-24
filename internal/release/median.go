package release

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Mobile good thresholds, rather than a perfect score on Lighthouse's curve.
// https://github.com/GoogleChrome/lighthouse/tree/v13.5.0/core/audits/metrics
// Lighthouse 13.5.0 core/audits/metrics/{first-contentful-paint,largest-contentful-paint,
// total-blocking-time,speed-index,cumulative-layout-shift}.js and
// https://developer.chrome.com/docs/lighthouse/performance/performance-scoring
// Speed index rounds the 3387 ms mobile curve control point to the documented 3.4 s.
var metricGood = map[string]float64{
	"first-contentful-paint": 1800, "largest-contentful-paint": 2500,
	"total-blocking-time": 200, "speed-index": 3400, "cumulative-layout-shift": .1,
}

func timingAudit(id string) bool {
	if _, ok := metricGood[id]; ok {
		return id != "cumulative-layout-shift"
	}
	for _, part := range []string{"potential-fid", "interaction-to-next-paint", "mainthread", "main-thread", "bootup", "long-tasks"} {
		if strings.Contains(id, part) {
			return true
		}
	}
	return false
}

func runLighthouse(ctx context.Context, target Target, browser, root string) *LighthouseResult {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	return measureLighthouse(ctx, func(ctx context.Context) *LighthouseResult {
		return runLighthouseOnce(ctx, target, browser, root)
	})
}

func measureLighthouse(ctx context.Context, run func(context.Context) *LighthouseResult) *LighthouseResult {
	start := time.Now()
	first := run(ctx)
	first.Runs = 1
	retry := false
	for _, f := range first.Findings {
		retry = retry || timingAudit(f.ID)
	}
	if first.Status != "ok" || !retry {
		return first
	}
	runs := []*LighthouseResult{first}
	for len(runs) < 3 && ctx.Err() == nil {
		next := run(ctx)
		if next.Status != "ok" {
			first.Warnings = append(first.Warnings, "Timing remeasurement failed: "+next.Reason)
			break
		}
		runs = append(runs, next)
	}
	if len(runs) == 1 {
		first.Runs = len(runs)
		return first
	}
	return medianLighthouse(runs, time.Since(start).Milliseconds())
}

func medianLighthouse(runs []*LighthouseResult, duration int64) *LighthouseResult {
	var reports []lhReport
	for _, run := range runs {
		var report lhReport
		normalized := strings.ReplaceAll(string(run.raw), run.origin, runs[0].origin)
		if json.Unmarshal([]byte(normalized), &report) != nil {
			return runs[0]
		}
		reports = append(reports, report)
	}
	merged := reports[0]
	for id := range merged.Audits {
		audits := make([]lhAudit, len(reports))
		numeric, scored := true, true
		for i, report := range reports {
			audits[i] = report.Audits[id]
			numeric = numeric && audits[i].NumericValue != nil
			scored = scored && audits[i].Score != nil
		}
		middle := len(audits) / 2
		if numeric {
			sort.Slice(audits, func(i, j int) bool { return *audits[i].NumericValue < *audits[j].NumericValue })
			a := audits[middle]
			if len(audits)%2 == 0 {
				value := *audits[middle-1].NumericValue/2 + *a.NumericValue/2
				a.NumericValue = &value
				if audits[middle-1].Score != nil && a.Score != nil {
					score := *audits[middle-1].Score/2 + *a.Score/2
					a.Score = &score
				}
			}
			a.DisplayValue = fmt.Sprintf("median of %d runs: %g %s; runs %g–%g %s", len(audits), *a.NumericValue, a.NumericUnit, *audits[0].NumericValue, *audits[len(audits)-1].NumericValue, a.NumericUnit)
			merged.Audits[id] = a
		} else if scored {
			sort.Slice(audits, func(i, j int) bool { return *audits[i].Score < *audits[j].Score })
			a := audits[middle]
			if len(audits)%2 == 0 {
				score := *audits[middle-1].Score/2 + *a.Score/2
				a.Score = &score
			}
			merged.Audits[id] = a
		}
	}
	for name := range runs[0].Scores {
		category := merged.Categories[name]
		scores := make([]float64, len(reports))
		for i, report := range reports {
			scores[i] = *report.Categories[name].Score
		}
		sort.Float64s(scores)
		middle := len(scores) / 2
		score := scores[middle]
		if len(scores)%2 == 0 {
			score = (scores[middle-1] + score) / 2
		}
		category.Score = &score
		merged.Categories[name] = category
	}
	raw, _ := json.Marshal(merged)
	result, err := parseLighthouse(raw, runs[0].origin)
	if err != nil {
		return runs[0]
	}
	result.Runs, result.DurationMS = len(runs), duration
	result.Report = runs[len(runs)-1].Report
	result.Scope = runs[0].Scope + fmt.Sprintf(" Timing results and category scores use per-audit medians of %d runs; saved full report is the last run.", len(runs))
	result.Warnings = append(result.Warnings, runs[0].Warnings[len(reports[0].RunWarnings):]...)
	return result
}
