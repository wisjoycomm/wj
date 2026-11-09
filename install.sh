#!/bin/sh
set -eu
repo="wisjoycomm/wj"
version="latest"
run_setup=0
while [ "$#" -gt 0 ]; do
    case "$1" in
        --version) [ "$#" -ge 2 ] || { echo "--version needs a tag" >&2; exit 1; }; version="$2"; shift 2 ;;
        --run) run_setup=1; shift ;;
        *) echo "Usage: install.sh [--version vX.Y.Z] [--run]" >&2; exit 1 ;;
    esac
done
os=$(uname -s)
arch=$(uname -m)
case "$os-$arch" in
    Darwin-arm64) file="wj-macos-arm64" ;;
    Darwin-x86_64) file="wj-macos-amd64" ;;
    Linux-x86_64) file="wj-linux-amd64" ;;
    Linux-aarch64|Linux-arm64) file="wj-linux-arm64" ;;
    *) echo "No build for $os $arch" >&2; exit 1 ;;
esac
if [ "$version" = latest ]; then
    release=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest")
    version=${release##*/}
fi
case "$version" in *[!a-zA-Z0-9._-]*|"") echo "Invalid release tag" >&2; exit 1 ;; esac
base="https://github.com/$repo/releases/download/$version"
dir="$HOME/.local/bin"
mkdir -p "$dir"
download=$(mktemp "$dir/.wj.XXXXXX")
checksums=$(mktemp "$dir/.wj-checksums.XXXXXX")
trap 'rm -f "$download" "$checksums"' EXIT HUP INT TERM
curl -fsSL "$base/$file" -o "$download"
curl -fsSL "$base/checksums.txt" -o "$checksums"
expected=$(awk -v name="$file" '$2 == name {print $1}' "$checksums")
if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$download" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$download" | awk '{print $1}')
else
    echo "Install sha256sum or shasum to verify the download." >&2
    exit 1
fi
if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
    echo "SHA-256 missing or mismatched; existing installation was not replaced." >&2
    exit 1
fi
chmod 755 "$download"
mv -f "$download" "$dir/wj"
echo "Installed $version: $dir/wj (SHA-256 verified)"
case ":$PATH:" in
    *":$dir:"*) ;;
    *) echo "Add to your shell profile: export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac
echo "Installed. Run wj when you want it (add --run to open it right away): OAuth token, API key, custom gateway or browser login."
if [ "$run_setup" -eq 1 ]; then "$dir/wj" < /dev/tty; fi
