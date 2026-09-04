#!/bin/sh
# Make sure the pinned Node runtime for one architecture is in the cache, and
# print the folder holding it. Everything else this script says goes to standard
# error, so that a caller can read the path straight out of its output.
#
# A release bundles this runtime beside the two TypeScript workers, so that a
# person installing Nerd Genie does not have to install Node first. It is downloaded
# once per machine and kept, because the release for the second architecture
# wants the same file again.
#
# Borrowed design: the download-then-verify order, and the refusal to keep a file
# whose checksum does not match, come from ZeroClaw's installer at
# ~/Code/zeroclaw/install.sh, which fetches the checksum list before the asset it
# is going to check.
#
# usage: node.sh <amd64|arm64>
set -eu

# The Node runtime every release carries. Raising it means fetching the new
# checksums from https://nodejs.org/dist/v<version>/SHASUMS256.txt and writing
# them below; docs/DEPENDENCIES.md says why this runtime is pinned at all.
node_version=24.18.0

amd64_file="node-v${node_version}-linux-x64.tar.xz"
amd64_sum="55aa7153f9d88f28d765fcdad5ae6945b5c0f98a36881703817e4c450fa76742"
arm64_file="node-v${node_version}-linux-arm64.tar.xz"
arm64_sum="58c9520501f6ae2b52d5b210444e24b9d0c029a58c5011b797bc1fe7105886f6"

# The cache sits where a machine keeps downloads rather than inside the checkout,
# so that a release never makes the working tree dirty and "make clean" does not
# throw away two hundred megabytes that would only be fetched again.
cache=${NERDGENIE_NODE_CACHE:-${XDG_CACHE_HOME:-$HOME/.cache}/nerdgenie-release}

say() { printf '%s\n' "$*" >&2; }
die() { printf '%s\n' "$*" >&2; exit 1; }

architecture=${1:-}
case "$architecture" in
amd64)
	tarball=$amd64_file
	expected=$amd64_sum
	;;
arm64)
	tarball=$arm64_file
	expected=$arm64_sum
	;;
*)
	die "node.sh was asked for the architecture \"$architecture\", and the only Node runtimes pinned are amd64 and arm64, so ask for one of those"
	;;
esac

unpacked="$cache/node-$node_version-$architecture"
if [ -x "$unpacked/bin/node" ]; then
	printf '%s\n' "$unpacked"
	exit 0
fi

mkdir -p "$cache"
download="$cache/$tarball"

if [ ! -f "$download" ]; then
	say "downloading the Node $node_version runtime for $architecture from nodejs.org"
	if ! curl --fail --silent --show-error --location --retry 3 --max-time 900 \
		--output "$download.part" "https://nodejs.org/dist/v$node_version/$tarball"; then
		rm -f "$download.part"
		die "the Node $node_version runtime for $architecture could not be downloaded from nodejs.org; check the network and run make release again"
	fi
	mv "$download.part" "$download"
fi

measured=$(sha256sum "$download" | cut -d ' ' -f 1)
if [ "$measured" != "$expected" ]; then
	die "the checksum of $download is $measured and the pinned one is $expected, so delete that file and run make release again; if it happens twice, the pin in scripts/release/node.sh is out of date"
fi

# Only the program itself is kept. The rest of the tarball is npm, the C headers,
# and the documentation, none of which a running worker ever reads.
staging="$cache/.unpacking-$architecture"
rm -rf "$staging"
mkdir -p "$staging"
inside=$(printf '%s' "$tarball" | sed 's/\.tar\.xz$//')
if ! tar -xJf "$download" -C "$staging" "$inside/bin/node"; then
	rm -rf "$staging"
	die "the Node runtime $download holds no $inside/bin/node, so delete that file and run make release again"
fi

rm -rf "$unpacked"
mkdir -p "$unpacked/bin"
mv "$staging/$inside/bin/node" "$unpacked/bin/node"
chmod 755 "$unpacked/bin/node"
rm -rf "$staging"

say "the Node $node_version runtime for $architecture is in $unpacked"
printf '%s\n' "$unpacked"
