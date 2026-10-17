package compile

import (
	"encoding/json"
	"fmt"
	htmltext "html"
	htmltemplate "html/template"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/Anonyfox/lfm/internal/check"
	"github.com/Anonyfox/lfm/internal/commercecheck"
	"github.com/Anonyfox/lfm/internal/diagnostic"
	"github.com/Anonyfox/lfm/internal/jsonsrc"
	"github.com/Anonyfox/lfm/internal/project"
	"github.com/Anonyfox/lfm/internal/templatecheck"
	"github.com/Anonyfox/lfm/internal/urlcheck"
)

const entryTemplate = project.TemplateFile

var canonicalAttributeTemplate = htmltemplate.Must(htmltemplate.New("canonical-url").Parse(`<link href="{{.}}">`))

// html/template percent-encodes Unicode/spaces inside URL attributes. Restore
// only that exact mechanical spelling to the configured canonical, preserving
// the user's byte-level identity without accepting different paths/hosts/queries.
func renderedURLAttribute(value string) (string, error) {
	var output strings.Builder
	if err := canonicalAttributeTemplate.Execute(&output, value); err != nil {
		return "", err
	}
	return htmltext.UnescapeString(strings.TrimSuffix(strings.TrimPrefix(output.String(), `<link href="`), `">`)), nil
}

type preparedTemplate struct {
	HTML, JSONLD []byte
	JSONLDSource diagnostic.Location
	Helpers      map[string]helperEntry
	Commerce     commercecheck.Facts
}

// All mapping checks happen before assets are bundled or output is staged.
func prepareTemplate(p project.Project, loaded project.Loaded) (preparedTemplate, check.Report, *urlcheck.Registry, error) {
	var prepared preparedTemplate
	var registry *urlcheck.Registry
	c, values, rawSources, configProblems := loaded.Config, loaded.Values, loaded.Sources, loaded.Problems
	sources := jsonsrc.Index(rawSources)
	files, issues := templateFiles(p)
	t, parsed := templatecheck.Parse(files, entryTemplate)
	for _, issue := range parsed.Diagnostics {
		if _, readable := files[issue.Location.File]; !readable {
			continue
		} // loader already explains the missing/unreadable entry
		issues = append(issues, issue)
	}
	issues = append(issues, sources.Issues...)
	for _, problem := range configProblems {
		// JSON token diagnostics already provide the exact syntax location per file.
		invalid := false
		for _, issue := range sources.Issues {
			if issue.Rule == "config.invalid" && issue.Location.File == problem.File {
				invalid = true
				break
			}
		}
		if invalid {
			continue
		}
		at := diagnostic.At(problem.File, rawSources[problem.File], problem.Offset)
		related := []diagnostic.Location(nil)
		if source, ok := sources.Nearest(problem.Path); ok {
			if source.File == problem.File && problem.Offset == 0 {
				at = source
			} else {
				related = append(related, source)
			}
		}
		issues = append(issues, diagnostic.Diagnostic{Rule: "config.invalid", Severity: diagnostic.Error, Location: at, Path: problem.Path, Related: related, Message: problem.Err.Error(), Hint: "Fix this config/data setting; custom page fields belong under data and paths mirror the JSON structure."})
	}
	helpers := newHelpers(p)
	options := templatecheck.Options{Entry: entryTemplate, Values: values, Files: files, Sources: sources, BrowserData: c.Build.BrowserData, Rules: c.Build.Validation.Rules, Funcs: helpers.functions(false)}
	if len(configProblems) == 0 {
		var commerceIssues []diagnostic.Diagnostic
		prepared.Commerce, commerceIssues = commercecheck.Check(c.Data, c.URLs, sources)
		issues = append(issues, diagnostic.Apply(commerceIssues, options.Rules)...)
		issues = append(issues, diagnostic.Apply(commercecheck.ConfigSecrets(values, sources), options.Rules)...)
	}
	if diagnostic.Summarize(issues).Errors == 0 {
		resolved, consumed, bindings := templatecheck.ResolveJSONLD(options)
		issues = append(issues, bindings.Diagnostics...)
		options.Consumed = append(consumed, prepared.Commerce.Consumed...)
		if resolved != nil && bindings.Errors == 0 {
			var err error
			prepared.JSONLD, err = json.Marshal(resolved)
			prepared.JSONLDSource = sources.Locations["jsonld"]
			if err != nil {
				issues = append(issues, diagnostic.Diagnostic{Rule: "jsonld.bindings", Severity: diagnostic.Error, Location: sources.Locations["jsonld"], Path: "jsonld", Message: "Cannot serialize resolved JSON-LD.", Hint: "Select JSON-compatible config values; numbers must be finite and objects/arrays must not be cyclic."})
			}
		}
		checked := templatecheck.Check(t, options)
		issues = append(issues, checked.Diagnostics...)
		registry = urlRegistry(c, sources)
		issues = append(issues, urlcheck.Apply(registry.Check(), options.Rules)...)
		issues = append(issues, urlcheck.Apply(urlcheck.Template(t, files, registry), options.Rules)...)
	}
	// Parse-level advisories obey the same policy as binding advisories.
	filtered := issues[:0]
	for _, issue := range issues {
		if issue.Severity == diagnostic.Warning {
			if level := options.Rules[issue.Rule]; level != "" {
				issue.Severity = level
			}
		}
		if issue.Severity != diagnostic.Off {
			filtered = append(filtered, issue)
		}
	}
	issues = filtered
	if diagnostic.Summarize(issues).Errors > 0 {
		report := templateReport(diagnostic.HardOnly(issues))
		failure := &diagnostic.Failure{Stage: "Template/config", Count: report.Errors}
		if len(configProblems) > 0 {
			return preparedTemplate{}, report, registry, fmt.Errorf("%w: %v", failure, configProblems[0].Err)
		}
		return preparedTemplate{}, report, registry, failure
	}
	options.Values = trustedURLValues(values, c.URLs)
	options.Funcs = helpers.functions(true)
	raw, renderIssue := templatecheck.Render(t, options)
	if renderIssue != nil {
		report := templateReport([]diagnostic.Diagnostic{*renderIssue})
		return preparedTemplate{}, report, registry, &diagnostic.Failure{Stage: "Template", Count: 1}
	}
	prepared.HTML = raw
	prepared.Helpers = helpers.entries
	return prepared, templateReport(issues), registry, nil
}

func templateReport(issues []diagnostic.Diagnostic) check.Report {
	s := diagnostic.Summarize(issues)
	return check.Report{Stage: "template", Diagnostics: s.Diagnostics, Errors: s.Errors, Warnings: s.Warnings}
}

func templateFiles(p project.Project) (map[string][]byte, []diagnostic.Diagnostic) {
	files := map[string][]byte{}
	var issues []diagnostic.Diagnostic
	read := func(name string) {
		raw, err := p.Read(name)
		if err != nil {
			issues = append(issues, diagnostic.Diagnostic{Rule: "template.syntax", Severity: diagnostic.Error, Location: diagnostic.At(name, nil, 0), Message: err.Error(), Hint: "Make this template source readable inside the project."})
			return
		}
		files[name] = raw
	}
	read(entryTemplate)
	if dir, err := p.OptionalDir(project.PartialsDir); err != nil {
		issues = append(issues, diagnostic.Diagnostic{Rule: "template.syntax", Severity: diagnostic.Error, Location: diagnostic.At("partials", nil, 0), Message: err.Error(), Hint: "Keep partials in a readable directory inside the project."})
	} else if dir != "" {
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if strings.HasPrefix(entry.Name(), ".") {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(p.Root, path)
			if err != nil {
				return err
			}
			read(filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			issues = append(issues, diagnostic.Diagnostic{Rule: "template.syntax", Severity: diagnostic.Error, Location: diagnostic.At("partials", nil, 0), Message: err.Error(), Hint: "Fix the partials source tree before rendering."})
		}
	}
	return files, issues
}

func urlRegistry(c project.Config, sources jsonsrc.Sources) *urlcheck.Registry {
	declarations := []urlcheck.Declaration{{Path: "url", Value: c.URL, Location: sources.Locations["url"]}}
	if c.SocialImage.Src != "" {
		declarations = append(declarations, urlcheck.Declaration{Path: "socialImage.src", Value: c.SocialImage.Src, Location: sources.Locations["socialImage.src"]})
	}
	for key, raw := range c.URLs {
		path := jsonsrc.Child("urls", key)
		declarations = append(declarations, urlcheck.Declaration{Path: path, Value: raw, Location: sources.Locations[path]})
	}
	return urlcheck.NewRegistry(c.URL, declarations)
}

// Named config URLs are explicit URI values. This permits tel:/application
// schemes without disabling contextual HTML escaping. Script pseudo-URLs and
// control characters never receive the trusted URL type.
func trustedURLValues(values map[string]any, urls map[string]string) map[string]any {
	copied := make(map[string]any, len(values))
	for key, value := range values {
		copied[key] = value
	}
	if len(urls) > 0 {
		named := map[string]any{}
		for key, value := range urls {
			if urlcheck.Trustable(value) {
				named[key] = htmltemplate.URL(value)
			} else {
				named[key] = value
			}
		}
		copied["urls"] = named
	}
	return copied
}
