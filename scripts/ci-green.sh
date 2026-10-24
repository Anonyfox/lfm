#!/bin/sh
# Prints green=true when a completed CI workflow run on exactly the given commit
# succeeded and every one of its jobs succeeded; green=false otherwise. The
# macOS job runs only for tags and manual runs, so on main it may be skipped.
set -eu
sha=${1:?usage: ci-green.sh <commit sha>}
repo=${GITHUB_REPOSITORY:?}
for run in $(gh run list -R "$repo" --workflow ci.yml --commit "$sha" --status success --json databaseId -q '.[].databaseId'); do
	if [ "$(gh run view "$run" -R "$repo" --json headSha,jobs -q "(.headSha == \"$sha\") and (.jobs | length > 0) and all(.jobs[]; .conclusion == \"success\" or (.name == \"test-macos\" and .conclusion == \"skipped\"))")" = true ]; then
		echo "CI run $run passed on $sha" >&2
		echo green=true
		exit 0
	fi
done
echo "no fully green CI run on $sha; the release runs the tests" >&2
echo green=false
