# Working on lfm

lfm is one Go binary that compiles an agent-built landing page (and the deliverables it sells) into a
static folder, and refuses to ship a page that is broken, slow, inaccessible, invisible to search or
full of generic AI design defaults. Read [docs/architecture.md](docs/architecture.md) before changing
code; it maps every package and the compile lifecycle.

This file is for working on lfm itself. `internal/scaffold/files/AGENTS.md` is a different file: the
template `lfm init` writes into user projects.

## Principles

- **General, not niche.** A feature must help every page. Prefer a refactor that removes a special case
  over adding one; no flags for a single project.
- **Fast.** An ordinary compile takes milliseconds. `compile` has an allocation budget and
  `BenchmarkCompile`; keep both green.
- **Report, never rewrite.** Checks only report. Automatic repairs (image sizes, favicons, resource
  hints) belong in the assemble stage, where the compile report shows them.
- **Nothing is written until verify passes.** Output stays in memory and is swapped in at publish.
- **Dependencies earn their place.** Ask before adding a module.
- **Agent-first CLI.** Every finding has a rule ID, a message and a concrete fix; every report ends with
  the next step; `--json` mirrors the human output.

## Changing things

- **A check**: follow "Adding a check" in [docs/architecture.md](docs/architecture.md). Register the rule
  with an ID, summary and help; `TestCatalogRulesHaveStandardHelp` and
  `TestEveryReportedRuleIsInTheCatalog` enforce it. Test the smallest triggering case and the nearest
  legitimate case that must stay quiet.
- **Config (`lfm.json`)**: document every field in [docs/config.md](docs/config.md); config mistakes point
  at their JSON key.
- **Output files**: golden manifests in `testdata/golden` pin them byte for byte. Rewrite them with
  `LFM_UPDATE_GOLDEN=1 go test ./...` and review the diff.
- **Skill text**: edit `internal/skills/files/<topic>/SKILL.md`; `TestSkillsFollowTheSpec` checks it.
  The root [SKILL.md](SKILL.md) is the entry point agents fetch from lfm.cash.
- **Parsers and analyzers of author or LLM input** need a native fuzz target; see
  [CONTRIBUTING.md](CONTRIBUTING.md).
- **The website** is an lfm project in `site/`; `.github/workflows/pages.yml` compiles it with the lfm
  built from the same commit and deploys lfm.cash.
- **User-visible changes** get a line in [CHANGELOG.md](CHANGELOG.md).

## Verify before you say done

```sh
gofmt -l . && go mod tidy -diff && go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 -checks all ./...
go test -race ./...
```

Use the Go version in `go.mod` (CI does), for example `GOTOOLCHAIN=go1.26.9`. Browser-backed tests
skip without Chrome; `-short` skips them explicitly.
