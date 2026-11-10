package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Anonyfox/lfm/internal/release"
)

var doctorCommand = command{
	name: "doctor", args: "[directory]", summary: "Report installed release tool paths",
	flags: []flagSpec{{name: "json", usage: "print machine-readable JSON"}},
	run: func(_ context.Context, env Env, p parsed) error {
		root, err := p.directory("doctor")
		if err != nil {
			return err
		}
		type tool struct {
			Path   string `json:"path,omitempty"`
			Reason string `json:"reason,omitempty"`
		}
		report := struct {
			Chrome     tool   `json:"chrome"`
			Lighthouse tool   `json:"lighthouse"`
			Next       string `json:"next"`
		}{Next: "Run 'lfm compile --release' to verify the page; install any missing tools first."}
		report.Chrome.Path, err = release.BrowserPath()
		if err != nil {
			report.Chrome.Reason = err.Error()
		}
		report.Lighthouse.Path, err = release.LighthousePath(root)
		if err != nil {
			report.Lighthouse.Reason = err.Error()
		}
		if p.has("json") {
			return json.NewEncoder(env.Stdout).Encode(report)
		}
		for _, entry := range []struct {
			name string
			tool tool
		}{{"Chrome", report.Chrome}, {"Lighthouse", report.Lighthouse}} {
			value := entry.tool.Path
			if value == "" {
				value = entry.tool.Reason
			}
			fmt.Fprintf(env.Stdout, "%s: %s\n", entry.name, value)
		}
		fmt.Fprintln(env.Stdout, report.Next)
		return nil
	},
}
