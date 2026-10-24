#!/bin/sh
# Fails when a release archive holds an entry not owned by uid and gid 0.
#
#   sh scripts/archive-owner.sh dist/*.tar.gz
set -eu

[ "$#" -gt 0 ] || { echo "usage: archive-owner.sh <archive.tar.gz>..."; exit 2; }
for a in "$@"; do
	bad="$(tar --numeric-owner -tvzf "$a" | awk '$2 != "0/0"')"
	if [ -n "$bad" ]; then
		echo "$a has entries not owned by 0/0:"
		echo "$bad"
		exit 1
	fi
	echo "$a: every entry is 0/0"
done
