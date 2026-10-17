# lfm

**Looking for more. Looking for money.**

`lfm` compiles sales pages built by AI agents into fast, checked, standalone static output. The agent
writes one HTML template, a little config and its assets; `lfm` bundles and packs everything into a
folder that deploys anywhere, and refuses to ship pages that are broken, slow, inaccessible, invisible to
search or full of generic AI design defaults. Every finding says how to fix it.

No framework, no platform, no runtime: one binary, one page, one folder.

## Install

Prebuilt binaries for macOS and Linux (amd64 and arm64) are attached to every
[release](https://github.com/Anonyfox/lfm/releases). With Go 1.26 or newer:

```sh
go install github.com/Anonyfox/lfm/cmd/lfm@latest
```

## Use

```sh
lfm init my-page        # a starter project that compiles cleanly
cd my-page
lfm serve --open        # live preview, recompiles on every save
lfm compile             # check everything and write output/
lfm compile --release   # also measure in headless Chrome and Lighthouse
lfm rules               # every check; lfm rules <id> explains one
lfm skills              # the agent skills: research, design, implementation
```

An agent builds a page in three phases, each with a skill it prints or installs: **research** collects
real buyer voices and competitor pages into `research/`, **design** turns them into `concept.md` with
every decision cited, and **implementation** builds the page from it and reviews every frame. The
compiler checks each handover, so a phase that skips its homework shows up as findings.

`output/` is the finished page: upload it to any static host at the URL configured in `lfm.json`.

## What a compile guarantees

- **Correct:** every template value exists and has the right shape; config mistakes point at their JSON
  key; links match the declared URLs; HTML parses without browser repairs.
- **Fast:** CSS and JS are bundled and minified, small assets inlined, large ones emitted as content-named
  files, images sized against layout shift, HTML compacted without touching text.
- **Findable and shareable:** canonical URL, metadata, headings, structured data and social previews are
  checked; preview image sizes are filled in from the real files.
- **Usable:** controls have names, labels and focus order work, forms and disclosures behave.
- **Distinct:** typical generated-design defaults (stock palettes, eyebrow labels, emoji bullets, icon-card
  grids and more) and stock phrases of generated copy are reported, with an explicit way to keep one
  deliberately; the page is held to the palette, typefaces and imagery its `concept.md` decided.
- **Seen whole:** `--release` scrolls the page at phone and desktop sizes, saves every frame and reports
  empty stretches, sideways scrolling and split values.
- **Safe to ship:** nothing is written until all checks pass, and a failed compile keeps the last good output.

## Docs

- [Authoring a project](docs/authoring.md): layout, `lfm.json`, templates, assets.
- [What `lfm compile` does](docs/compiling.md): lifecycle, packing, output, release, preview.
- [Contributing](CONTRIBUTING.md): building, testing and the package map.

## License

[MIT](LICENSE) · [lfm.cash](https://lfm.cash)
