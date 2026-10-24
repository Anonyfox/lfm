#!/bin/sh
# Prints the go test arguments of one CI test group, so the race tests spread
# over parallel jobs. The compile package is split by test name: compile-a runs
# the slow browser and bundle tests named below, compile-b skips exactly those,
# so every compile test runs in one of the two and a new or renamed test lands
# in compile-b. book holds book, release and skills; rest is every other
# package, so new packages are always tested.
set -eu
mod=$(go list -m)
slow='^(TestPrintableIsPublishedBehindItsPath|TestDecksAreBuiltCheckedAndDelivered|TestSpreadsheetIsBuiltCheckedAndDelivered|TestBookIsPublishedBehindItsPath|TestReleaseBuildValidatedWithFullCSSFallback|TestReleaseReusesFileModeStylesheetWithEscapedName|TestPublicCollisionsKeepPreviousOutput|TestANewerLfmRebuildsFilesButKeepsItsPrints|TestReleaseLeavesNothingRunning)$'
case ${1:?usage: test-group.sh compile-a|compile-b|book|rest} in
compile-a) printf '%s\n' "-run=$slow" "$mod/internal/compile" ;;
compile-b) printf '%s\n' "-skip=$slow" "$mod/internal/compile" ;;
book) printf '%s\n' "$mod/internal/book" "$mod/internal/release" "$mod/internal/skills" ;;
rest) go list ./... | grep -vxF -e "$mod/internal/compile" -e "$mod/internal/book" -e "$mod/internal/release" -e "$mod/internal/skills" ;;
*) echo "unknown group $1" >&2; exit 2 ;;
esac
