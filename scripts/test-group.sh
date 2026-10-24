#!/bin/sh
# Prints the packages of one CI test group, so the race tests spread over
# parallel jobs: compile, book (book, release and skills) or rest. Every package
# not named in the first two groups is in rest, so new packages are always tested.
set -eu
mod=$(go list -m)
case ${1:?usage: test-group.sh compile|book|rest} in
compile) echo "$mod/internal/compile" ;;
book) printf '%s\n' "$mod/internal/book" "$mod/internal/release" "$mod/internal/skills" ;;
rest) go list ./... | grep -vxF -e "$mod/internal/compile" -e "$mod/internal/book" -e "$mod/internal/release" -e "$mod/internal/skills" ;;
*) echo "unknown group $1" >&2; exit 2 ;;
esac
