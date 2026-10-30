package compile

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/project"
)

// ProjectCheckOptions pairs an already built page with the same project facts
// and validation settings used by Compile. Resource bytes stay those of the target.
func ProjectCheckOptions(dir string, options check.Options) (check.Options, error) {
	p, err := project.Open(dir)
	if err != nil {
		return options, err
	}
	if err := p.Expect(dir); err != nil {
		return options, err
	}
	loaded := project.Load(p, ruleCheck)
	prepared, preliminary, registry, err := prepareTemplate(p, loaded)
	if err != nil {
		if len(preliminary.Diagnostics) > 0 {
			return options, fmt.Errorf("%w\n%s", err, preliminary.Diagnostics[0].String())
		}
		return options, err
	}
	var result Result
	_, facts, _ := checkResearch(p, &result, loaded.Config.Build.Validation.Rules)
	options.GeneratedAssets = make(map[string]bool, len(options.Assets))
	for name := range options.Assets {
		options.GeneratedAssets[name] = true
	}
	// Output is a separate read boundary from editable source. Its own files
	// remain generated resources even when a live host cannot serve them yet.
	output, openErr := os.OpenRoot(p.Output())
	if openErr == nil {
		defer output.Close()
		if err := fs.WalkDir(output.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() && entry.Type().IsRegular() {
				options.GeneratedAssets[name] = true
			}
			return nil
		}); err != nil {
			return options, err
		}
	} else if !os.IsNotExist(openErr) {
		return options, openErr
	}
	options.RenderedSource = prepared.HTML
	options.Registry, options.Commerce, options.Concept = registry, prepared.Commerce, facts
	options.JSONLDGenerated, options.JSONLDSource = len(prepared.JSONLD) > 0, &prepared.JSONLDSource
	options.Rules = loaded.Config.Build.Validation.Rules
	options.CopyAllow = loaded.Config.Build.Validation.CopyAllow
	options.Budgets = check.Budgets(loaded.Config.Build.Validation.Budgets)
	return options, nil
}

// ProjectCopyOptions loads copy policy without requiring templates or research.
func ProjectCopyOptions(dir string, options check.Options) (check.Options, error) {
	p, err := project.Open(dir)
	if err != nil {
		return options, err
	}
	loaded := project.Load(p, ruleCheck)
	if len(loaded.Problems) > 0 {
		return options, fmt.Errorf("%s: %s: %v", loaded.Problems[0].File, loaded.Problems[0].Path, loaded.Problems[0].Err)
	}
	options.Rules = loaded.Config.Build.Validation.Rules
	options.CopyAllow = loaded.Config.Build.Validation.CopyAllow
	return options, nil
}
