#!/usr/bin/env bash
# Builds the npm package that lets `bunx @mynameisgump/repoanalyzer` (or npx) run
# a prebuilt binary. It carries a binary for each platform below, and its
# launcher (npm/repoanalyzer/bin/repoanalyzer.js) runs the one for the machine.
#
#   scripts/npm-build.sh 0.1.0             build into dist/npm/repoanalyzer
#   scripts/npm-build.sh 0.1.0 --publish   build, then npm publish it
#
# Needs Go and Node.
set -euo pipefail

# npm's names for each platform: process.platform-process.arch.
platforms="darwin-arm64 darwin-x64 linux-arm64 linux-x64 win32-x64"

usage() {
	sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'
}

version="${1:-}"
publish=false
case "${2:-}" in
"") ;;
--publish) publish=true ;;
*)
	usage >&2
	exit 2
	;;
esac
if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
	usage >&2
	exit 2
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
pkg="$root/dist/npm/repoanalyzer"
rm -rf "$pkg"
mkdir -p "$(dirname "$pkg")"
cp -R "$root/npm/repoanalyzer" "$pkg"
cp "$root/README.md" "$pkg/README.md"
VERSION="$version" node -e '
	const fs = require("fs");
	const file = process.argv[1];
	const pkg = JSON.parse(fs.readFileSync(file, "utf8"));
	pkg.version = process.env.VERSION;
	fs.writeFileSync(file, JSON.stringify(pkg, null, 2) + "\n");
' "$pkg/package.json"

for target in $platforms; do
	os="${target%-*}"
	cpu="${target#*-}"
	case "$os" in
	win32) goos=windows exe=repoanalyzer.exe ;;
	*) goos="$os" exe=repoanalyzer ;;
	esac
	case "$cpu" in
	x64) goarch=amd64 ;;
	*) goarch="$cpu" ;;
	esac

	echo "Building $target ($goos/$goarch)…"
	(cd "$root" && CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" \
		go build -trimpath -ldflags "-s -w" -o "$pkg/bin/$target/$exe" ./cmd/repoanalyzer)
done

echo "Package is in $pkg"

if $publish; then
	if npm view "@mynameisgump/repoanalyzer@$version" version >/dev/null 2>&1; then
		echo "@mynameisgump/repoanalyzer@$version is already published; bump the version." >&2
		exit 1
	fi
	(cd "$pkg" && npm publish --access public)
fi
