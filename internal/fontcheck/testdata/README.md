# Font regression fixtures

`Go-Regular.ttf` is the unmodified Go Regular font from
`golang.org/x/image v0.43.0/font/gofont/ttfs`; see `LICENSE-Go.txt`.

`Go-Regular.woff2` was prepared with FontTools 4.67.0 / Brotli 1.2.0, retaining
glyphs and typography. Reproduce with `python generate-woff2.py` in an environment
containing those optional preparation tools.
This fixture generator is not part of the compiler or test runtime. Browser tests
verify actual loading, preload reuse, layout and pixels; unit tests only screen
container metadata and bounds. No font conversion/subsetting occurs in builds.
