# Contributing

## Fuzzing

Every parser and analyzer that reads author or LLM input has native Go fuzz
targets (`FuzzXxx` in `*_test.go`). Their seed corpora and every minimized
crash under `testdata/fuzz/` run as ordinary tests with `go test ./...`.

To fuzz every target briefly, as CI does:

```sh
for pkg in $(go list ./internal/...); do
  for name in $(go test -list '^Fuzz' "$pkg" | grep '^Fuzz'); do
    go test -run '^$' -fuzz "^$name\$" -fuzztime 10s "$pkg" || exit 1
  done
done
```

Raise `-fuzztime` (e.g. `5m`) to work on one target. A failure writes its input
to `testdata/fuzz/FuzzXxx/`: fix the root cause and commit that file as the
regression case.
