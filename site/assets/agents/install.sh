#!/bin/sh
# Installs the lfm binary for macOS or Linux (amd64, arm64) without sudo.
#
#   curl -fsSL https://raw.githubusercontent.com/Anonyfox/lfm/main/install.sh | sh
#
# Settings, all optional:
#   LFM_VERSION       release tag such as v0.0.2 (default: the latest release)
#   LFM_INSTALL_DIR   where the binary goes (default: ~/.local/bin)
#   LFM_RELEASES_URL  base of the release downloads, for mirrors
#                     (default: https://github.com/Anonyfox/lfm/releases)
#
# The archive is checked against the release's checksums.txt. When no release
# can be downloaded and Go is installed, it falls back to go install.
set -eu

releases="${LFM_RELEASES_URL:-https://github.com/Anonyfox/lfm/releases}"
version="${LFM_VERSION:-latest}"
dir="${LFM_INSTALL_DIR:-$HOME/.local/bin}"

say() { printf 'lfm install: %s\n' "$*" >&2; }
fail() { say "$*"; exit 1; }

case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) fail "no build for $(uname -s); lfm runs on macOS and Linux" ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "no build for $(uname -m); lfm runs on amd64 and arm64" ;;
esac

if [ "$version" = latest ]; then
	base="$releases/latest/download"
else
	base="$releases/download/$version"
fi
archive="lfm_${os}_${arch}.tar.gz"

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL "$1" -o "$2"
	elif command -v wget >/dev/null 2>&1; then
		wget -q "$1" -O "$2"
	else
		fail "needs curl or wget"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

go_install() {
	command -v go >/dev/null 2>&1 || return 1
	say "no release download; building with go install instead"
	GOBIN="$dir" go install "github.com/Anonyfox/lfm/cmd/lfm@$version" || return 1
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM
mkdir -p "$dir"

if fetch "$base/$archive" "$tmp/$archive" && fetch "$base/checksums.txt" "$tmp/checksums.txt"; then
	want="$(grep " $archive\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
	[ -n "$want" ] || fail "checksums.txt has no entry for $archive"
	[ "$(sha256 "$tmp/$archive")" = "$want" ] || fail "checksum mismatch for $archive; nothing installed"
	# Ignore the archive's owner, and write a new file so it belongs to the
	# installing user (root under sudo), not to the build machine's uid.
	tar --no-same-owner -xzf "$tmp/$archive" -C "$tmp" lfm
	rm -f "$dir/lfm.new"
	install -m 755 "$tmp/lfm" "$dir/lfm.new"
	if [ "$(id -u)" = 0 ]; then
		chown 0:0 "$dir/lfm.new"
	fi
	mv -f "$dir/lfm.new" "$dir/lfm"
elif ! go_install; then
	fail "could not download $base/$archive, and Go 1.26 or newer is not available to build it"
fi

installed="$("$dir/lfm" version)"
say "installed $installed to $dir/lfm"
case ":$PATH:" in
*":$dir:"*) ;;
*) say "$dir is not on PATH; add it, for example: export PATH=\"$dir:\$PATH\"" ;;
esac
found="$(command -v lfm || true)"
if [ -n "$found" ] && [ "$found" != "$dir/lfm" ]; then
	other="$("$found" version 2>/dev/null || echo "unknown version")"
	if [ "$other" != "$installed" ]; then
		say "warning: the shell runs $found ($other), not $dir/lfm ($installed), because it comes first on PATH; remove it or put $dir before it"
	fi
fi
