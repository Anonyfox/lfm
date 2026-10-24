package compile

import (
	"testing"

	"github.com/Anonyfox/lfm/internal/project"
)

func TestConceptIntensityReachesPageFacts(t *testing.T) {
	for _, intensity := range []string{"restrained", "balanced", "immersive"} {
		t.Run(intensity, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "concept.md", "# Concept: Workshop\n## Motion\nIntensity: "+intensity+"\nMoment: A; B\nFeedback: Button\nReduced motion: A still; B end state\n")
			write(t, dir, project.TemplateFile, "<main><section><h1>Workshop</h1></section></main>")
			p, err := project.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			var result Result
			_, facts, _ := checkResearch(p, &result, nil)
			if facts == nil || facts.Intensity != intensity || result.Concept.Intensity != intensity {
				t.Fatalf("facts %+v summary %+v", facts, result.Concept)
			}
		})
	}
}
