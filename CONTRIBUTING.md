# Contributing

Go 1.26 or newer. Chrome or Chromium is optional; browser-backed tests skip without it.
Compiler and CLI tests use a local Lighthouse fixture; release commands require an installed Lighthouse.

```sh
go build ./cmd/lfm          # the binary
go test ./...               # everything, including browser tests
go test -short ./...        # without browser tests
go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
```

[Architecture](docs/architecture.md) maps the packages and explains how to add a check. CI runs lint,
race-enabled tests on Linux and macOS, every fuzz target briefly, cross-compiles and a release check.
Tags `vX.Y.Z` publish binaries for macOS and Linux through GoReleaser.

## Fuzzing

Every parser and analyzer that reads author or LLM input has native Go fuzz
targets (`FuzzXxx` in `*_test.go`). Their seed corpora and every minimized
crash under `testdata/fuzz/` run as ordinary tests with `go test ./...`.

To fuzz every target briefly, as CI does:

```sh
for pkg in $(go list ./internal/...); do
  for name in $(go test -list '^Fuzz' "$pkg" | grep '^Fuzz'); do
    go test -run '^$' -fuzz "^$name\$" -fuzztime 5s "$pkg" || exit 1
  done
done
```

Raise `-fuzztime` (e.g. `5m`) to work on one target. A failure writes its input
to `testdata/fuzz/FuzzXxx/`: fix the root cause and commit that file as the
regression case.
