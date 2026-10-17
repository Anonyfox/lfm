# What `lfm compile` does

One compile runs a fixed lifecycle. A stage starts only when the stages before it found no errors, and all
of them report through the same findings, so a single run lists everything that needs fixing.

| Stage | Work |
| --- | --- |
| load | Read `lfm.json` and `data/`; every config problem is reported at its JSON key. Check the research files and `concept.md`. |
| prepare | Check every template binding, link and offer statically, then render the page. |
| assemble | Resolve, bundle and pack every referenced resource; inspect images and fonts. |
| optimize | `--release` only: split critical CSS after measuring it in headless Chrome. |
| finalize | Serialize and compact the HTML. |
| verify | Run every check on the exact bytes that will be written. |
| audit | `--release` only: a runtime probe, the layout audit and, when installed, Lighthouse. |
| publish | Replace `output/` with one rename. |

Nothing is written before verify passes. A failed compile keeps the previous `output/` and exits with code 1.
Findings carry a rule ID, the location, what is wrong and how to fix it; `lfm rules <id>` explains a rule.

## Packing

With the default policy the compiler decides per resource whether it is inlined into `index.html` or emitted
as a file beside it, and explains each choice:

- Fonts, lazily loaded media, responsive candidates and anything above 32 KiB encoded become files.
- The largest remaining resources move out until the page approaches 256 KiB.
- Small images become data URLs; SVGs use the smaller of percent and base64 encoding after compression.
- Compatible CSS above 20 KiB and module JS above 50 KiB become files.
- Social images, media and linked documents are always files with absolute URLs.

Automatic files get a short suffix hashed from their path and content, so every change gets a new URL and
hosts can cache files forever. `build.assets` in `lfm.json` overrides the policy globally or per path;
explicitly configured files keep their names.

HTML is compacted at the syntax level only: attribute quotes and empty values shrink, text and whitespace
stay exactly as written. Each compacted result is parsed and compared with the original tree.

## Output

`output/` contains `index.html` plus the emitted files, flat, ready to upload to any static host at the
configured `url`. It is replaced as a whole; there are no stale files, sources or reports in it.

Compiles of the same project are serialized across processes by locking the project directory; no lock
file is created, and a second compile waits and says so. Nothing is written into the project except
`output/` and, only while publishing, a sibling `output.next-*` folder that the swap removes again.
Ctrl-C stops a compile between stages; a stopped compile never publishes.

## Release

`lfm compile --release` adds three bounded stages that need installed tools:

1. **Critical CSS** (45 s limit): the first screen is rendered at 390×844, 768×1024 and 1440×900; the rules it
   uses stay in the head and the complete stylesheet loads after. The split is verified pixel by pixel and
   applied only when it removes at least 2 KiB and 20% of render-blocking CSS.
2. **Runtime probe and layout audit** (90 s): the page runs with its scripts in a fresh Chrome profile;
   exceptions and missing files are reported. Then the page is scrolled by half a screen at a time at 390×844,
   1280×680 and 1440×900; finite animations settle, looping ones pause at their start, and every frame is
   saved as a screenshot in the system temp directory. The report prints the folder and, from the second run
   on, the frames whose content changed. `release.layout` reports a first screen without a legible heading or
   any control, stretches without legible content, sideways scrolling, text a pinned layer always covers,
   text under 9 px, text under the WCAG contrast minimum on a certain flat ground, numbers split from their
   units, what still moves when the page is loaded again with reduced motion requested, and a heading or most of the text missing when it is loaded without JavaScript. Outside
   requests are blocked; nothing is clicked, typed or submitted.
3. **Lighthouse** (45 s, when installed): one mobile lab run. Scores, metrics and every failing audit are
   reported; the complete JSON is saved in the system temp directory.

Chrome or Chromium is found on `PATH` and in standard locations, or set `LFM_CHROME`. Lighthouse is found on
`PATH`, in `node_modules/.bin`, or via `LFM_LIGHTHOUSE` (`npm install -g lighthouse`). A missing tool is a
warning and its stage is skipped; lfm never installs anything.

## Preview

`lfm serve` compiles, serves the page on `127.0.0.1` and recompiles when a file changes. The browser reloads
only when the compiled page actually changed, keeping the scroll position. A failed compile shows its
findings in the browser and keeps serving the last good page. The preview serves compiled files only, never
sources, and injects its reload script into responses, never into `output/`.
