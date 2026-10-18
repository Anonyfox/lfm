package aitext

// profile is one fitted weight set over standardized features.
type profile struct {
	weight []float64
	bias   float64
}

// baseline standardizes the features of one language against its human
// writing: mean and scale of human text, the human median of each feature
// for the explanations, and the score from which a text counts as likely
// generated, per profile (old, modern).
type baseline struct {
	mean, scale, human []float64
	threshold          [2]float64
	// document is the log-odds threshold for the mean over the windows of a
	// text of two windows or more, per profile; 0 when not calibrated
	document [2]float64
	// weights are the language's own refinement of the shared weights, per
	// profile; empty where the language had too little data
	weights [2]profile
}

// fitted is the whole model: shared weights for the old and the modern
// profile, with every feature (full) and without the language-specific ones
// (agnostic, for languages lfm has no baseline for), and a baseline per
// language plus "other".
type fitted struct {
	full, agnostic [2]profile
	langs          map[string]baseline
}

// languageSpecific marks the features that need the language: the two
// prelude compressions, the habits and the fillers.
var languageSpecific = func() []bool {
	out := make([]bool, len(FeatureNames))
	for i, n := range FeatureNames {
		switch n {
		case "model-likeness", "claude-likeness", "habits", "fillers", "participles", "nominals", "person":
			out[i] = true
		}
	}
	return out
}()

// capped marks the features that only count as evidence of a person when
// present: formal human prose lacks fillers too, so their absence must not
// make a text look generated.
var capped = func() []bool {
	out := make([]bool, len(FeatureNames))
	for i, n := range FeatureNames {
		if n == "fillers" {
			out[i] = true
		}
	}
	return out
}()
