#!/bin/sh
# Plugin build step. Fetches the prebuilt binary for this platform so Go is
# not required; falls back to building from source when there is no asset for
# it, the checksum does not match, or the binary does not run here.
set -eu

REPO="DnzzL/herdr-idle"
OUT="bin/herdr-idle"
VERSION=$(sed -n 's/^version *= *"\(.*\)"/\1/p' herdr-plugin.toml | head -1)
# Overridable so the fallback paths can be exercised against a local server.
BASE="${HERDR_IDLE_RELEASE_BASE:-https://github.com/${REPO}/releases/download/v${VERSION}}"

# Exits the script: either a source build or a clear error.
build_from_source() {
	if ! command -v go >/dev/null 2>&1; then
		echo "herdr-idle: no usable prebuilt binary and Go is not installed." >&2
		echo "Install Go (https://go.dev/dl/) and reinstall the plugin." >&2
		exit 1
	fi
	echo "herdr-idle: building from source"
	mkdir -p bin
	go build -ldflags "-X main.version=${VERSION}" -o "$OUT" .
	exit 0
}

case "$(uname -s)" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) build_from_source ;;
esac

case "$(uname -m)" in
arm64 | aarch64) arch=arm64 ;;
x86_64 | amd64) arch=amd64 ;;
*) build_from_source ;;
esac

if ! command -v curl >/dev/null 2>&1; then
	echo "herdr-idle: curl not found"
	build_from_source
fi

asset="herdr-idle_${os}_${arch}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if ! curl -fsSL "${BASE}/${asset}" -o "${tmp}/${asset}"; then
	echo "herdr-idle: no release asset ${asset} for v${VERSION}"
	build_from_source
fi

# Never trust a download we could not verify.
if ! curl -fsSL "${BASE}/checksums.txt" -o "${tmp}/checksums.txt"; then
	echo "herdr-idle: no checksums.txt for v${VERSION}, refusing the download" >&2
	build_from_source
fi
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "${tmp}/${asset}" | cut -d' ' -f1)
elif command -v shasum >/dev/null 2>&1; then
	actual=$(shasum -a 256 "${tmp}/${asset}" | cut -d' ' -f1)
else
	echo "herdr-idle: no sha256 tool to verify the download" >&2
	build_from_source
fi
expected=$(awk -v a="$asset" '$2 == a || $2 == "*" a { print $1 }' "${tmp}/checksums.txt")
if [ -z "$expected" ] || [ "$actual" != "$expected" ]; then
	echo "herdr-idle: checksum mismatch for ${asset}, refusing the download" >&2
	build_from_source
fi

mkdir -p bin
mv "${tmp}/${asset}" "$OUT"
chmod +x "$OUT"

# A binary that can't run here (wrong libc, bad download) must not ship.
if ! "./$OUT" version >/dev/null 2>&1; then
	echo "herdr-idle: prebuilt binary did not run here"
	rm -f "$OUT"
	build_from_source
fi

echo "herdr-idle: installed prebuilt v${VERSION} (${os}/${arch})"
