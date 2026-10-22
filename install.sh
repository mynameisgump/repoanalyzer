#!/usr/bin/env bash
# Builds repoanalyzer from this checkout and installs it onto your PATH.
#
#   ./install.sh                 install to ~/.local/bin
#   ./install.sh --dir /usr/local/bin
#   ./install.sh --uninstall
#
# Works with the bash 3.2 that ships with macOS.
set -euo pipefail

name=repoanalyzer
dir="${INSTALL_DIR:-$HOME/.local/bin}"
uninstall=false

usage() {
	sed -n '2,8p' "$0" | sed 's/^# \{0,1\}//'
}

while [ $# -gt 0 ]; do
	case "$1" in
	--dir)
		[ $# -ge 2 ] || { echo "--dir needs a directory" >&2; exit 2; }
		dir="$2"
		shift 2
		;;
	--dir=*)
		dir="${1#--dir=}"
		shift
		;;
	--uninstall)
		uninstall=true
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		echo "unknown option: $1" >&2
		usage >&2
		exit 2
		;;
	esac
done

# Expand a leading ~ in case it was quoted.
case "$dir" in "~"/*) dir="$HOME/${dir#"~/"}" ;; esac
dest="$dir/$name"

# Use sudo only when the target directory isn't writable (e.g. /usr/local/bin).
sudo=""
needs_sudo() {
	local d="$1"
	while [ ! -d "$d" ]; do d="$(dirname "$d")"; done
	[ ! -w "$d" ]
}
if needs_sudo "$dir"; then
	echo "$dir isn't writable by you; using sudo."
	sudo="sudo"
fi

if $uninstall; then
	if [ -e "$dest" ]; then
		$sudo rm -f "$dest"
		echo "Removed $dest"
	else
		echo "Nothing to remove at $dest"
	fi
	exit 0
fi

if ! command -v go >/dev/null 2>&1; then
	echo "Go isn't installed. It's needed to build $name." >&2
	if command -v brew >/dev/null 2>&1; then
		echo "Install it with:  brew install go" >&2
	else
		echo "Get it from https://go.dev/dl/ (or install Homebrew, then: brew install go)" >&2
	fi
	exit 1
fi

src="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Building $name with $(go version | awk '{print $3}')…"
# GOTOOLCHAIN=auto lets an older Go fetch the version go.mod asks for.
(cd "$src" && GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" go build -trimpath -ldflags "-s -w" -o "$tmp/$name" ./cmd/repoanalyzer)

$sudo mkdir -p "$dir"
$sudo install -m 0755 "$tmp/$name" "$dest"
echo "Installed $dest"

case ":$PATH:" in
*":$dir:"*)
	found="$(command -v "$name" || true)"
	if [ -n "$found" ] && [ "$found" != "$dest" ]; then
		echo "Note: $found comes earlier on your PATH and will run instead."
	fi
	;;
*)
	rc="$HOME/.zshrc"
	case "${SHELL:-}" in */bash) rc="$HOME/.bash_profile" ;; esac
	echo
	echo "$dir isn't on your PATH yet. Add it with:"
	echo "  echo 'export PATH=\"$dir:\$PATH\"' >> $rc && source $rc"
	;;
esac

if ! command -v gh >/dev/null 2>&1; then
	echo
	echo "Optional: install the GitHub CLI for pull request stats (brew install gh, then gh auth login)."
fi

echo
echo "Run it inside any git repo:  $name"
