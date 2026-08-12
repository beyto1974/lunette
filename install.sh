#!/bin/sh
#
# Install lunette from a published release.
#
#   curl -fsSL https://raw.githubusercontent.com/beyto1974/lunette/main/install.sh | sh
#
# Environment:
#   LUNETTE_VERSION   tag to install (default: the latest release)
#   PREFIX            install under PREFIX/bin (default: ~/.local/bin)
#
# The archive is checked against the checksums.txt published beside it and the
# install is abandoned on a mismatch. That check is the whole reason this is
# safe to pipe into a shell; if you would rather read it first, download it and
# run it separately.
#
set -eu

REPO=beyto1974/lunette
BINARY=lunette

say() { printf '%s\n' "$*"; }
die() { printf 'install: %s\n' "$*" >&2; exit 1; }

need() {
	command -v "$1" >/dev/null 2>&1 || die "$1 is required but not installed"
}

# detect_platform maps uname output onto the names the release archives use,
# setting os and arch in the caller. It deliberately does not run in a command
# substitution: die's exit would only end the subshell, and the script would
# carry on and try to download an archive with an empty platform in its name.
detect_platform() {
	os=$(uname -s | tr '[:upper:]' '[:lower:]')
	arch=$(uname -m)

	case "$os" in
	linux | darwin) ;;
	mingw* | msys* | cygwin*)
		die "on Windows, take the zip from https://github.com/$REPO/releases"
		;;
	*)
		die "no build for $os; see https://github.com/$REPO/releases"
		;;
	esac

	case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*)
		die "no build for $arch; see https://github.com/$REPO/releases"
		;;
	esac
}

# latest follows the /releases/latest redirect and reads the tag out of it,
# which needs no API token and is not rate limited.
latest() {
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
		"https://github.com/$REPO/releases/latest") ||
		die "could not reach GitHub to find the latest release"
	tag=${url##*/}
	if [ -z "$tag" ] || [ "$tag" = "latest" ]; then
		die "could not work out the latest version"
	fi
	printf '%s\n' "$tag"
}

# verify checks one file against the published checksums.
verify() {
	archive=$1
	if command -v sha256sum >/dev/null 2>&1; then
		grep " $archive\$" checksums.txt | sha256sum -c - >/dev/null 2>&1
	elif command -v shasum >/dev/null 2>&1; then
		grep " $archive\$" checksums.txt | shasum -a 256 -c - >/dev/null 2>&1
	else
		die "need sha256sum or shasum to check the download"
	fi
}

main() {
	need curl
	need tar

	detect_platform

	version=${LUNETTE_VERSION:-$(latest)}
	number=${version#v}
	archive="${BINARY}_${number}_${os}_${arch}.tar.gz"
	base="https://github.com/$REPO/releases/download/$version"

	prefix=${PREFIX:-$HOME/.local}
	bindir="$prefix/bin"

	say "installing $BINARY $version ($os/$arch) into $bindir"

	work=$(mktemp -d)
	trap 'rm -rf "$work"' EXIT INT TERM
	cd "$work"

	curl -fsSLO "$base/$archive" ||
		die "could not download $archive from $base"
	curl -fsSLO "$base/checksums.txt" ||
		die "could not download checksums.txt from $base"

	if ! verify "$archive"; then
		die "checksum mismatch on $archive - refusing to install it"
	fi

	tar xzf "$archive" "$BINARY" || die "could not unpack $archive"

	mkdir -p "$bindir" 2>/dev/null ||
		die "could not create $bindir; set PREFIX to somewhere writable, or use sudo"
	if [ ! -w "$bindir" ]; then
		die "$bindir is not writable; set PREFIX to somewhere writable, or use sudo"
	fi
	install -m 0755 "$BINARY" "$bindir/$BINARY" 2>/dev/null ||
		{ cp "$BINARY" "$bindir/$BINARY" && chmod 0755 "$bindir/$BINARY"; } ||
		die "could not install into $bindir"

	say "installed: $("$bindir/$BINARY" version)"

	case ":$PATH:" in
	*":$bindir:"*) ;;
	*) say "note: $bindir is not on your PATH" ;;
	esac
}

main "$@"
