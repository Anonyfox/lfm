#!/bin/sh
# Tests install.sh against a local release: run as root it installs a
# root-owned binary even when the archive says uid 1001, and without root it
# warns when an older lfm comes first on PATH.
#
#   go build -o lfm ./cmd/lfm && sh scripts/install-test.sh ./lfm
#
# Needs GNU tar and passwordless sudo (as on GitHub's Linux runners).
set -eu

bin="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
repo="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'sudo rm -rf "$work"' EXIT INT TERM
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
*) arch=arm64 ;;
esac
archive="lfm_linux_${arch}.tar.gz"

mkdir -p "$work/release/download/v0.0.0" "$work/stage"
cp "$bin" "$work/stage/lfm"
tar --owner=1001 --group=1001 --numeric-owner -czf "$work/release/download/v0.0.0/$archive" -C "$work/stage" lfm
(cd "$work/release/download/v0.0.0" && sha256sum "$archive" > checksums.txt)
run() {
	env LFM_RELEASES_URL="file://$work/release" LFM_VERSION=v0.0.0 LFM_INSTALL_DIR="$1" sh "$repo/install.sh"
}

sudo env PATH="$PATH" LFM_RELEASES_URL="file://$work/release" LFM_VERSION=v0.0.0 LFM_INSTALL_DIR="$work/root" sh "$repo/install.sh"
owner="$(stat -c %u:%g "$work/root/lfm")"
[ "$owner" = 0:0 ] || { echo "root install left the binary owned by $owner"; exit 1; }

mkdir -p "$work/old"
printf '#!/bin/sh\necho "lfm 0.0.1"\n' > "$work/old/lfm"
chmod 755 "$work/old/lfm"
out="$(PATH="$work/old:$work/user:$PATH" run "$work/user" 2>&1)"
echo "$out"
echo "$out" | grep -q "warning: the shell runs $work/old/lfm (lfm 0.0.1)" || { echo "no warning about the older lfm on PATH"; exit 1; }
echo "install.sh ok"
