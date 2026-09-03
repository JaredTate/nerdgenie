#!/bin/sh
# Build one release: the binary for each architecture, the two worker bundles,
# and the pinned Node runtime, packed into one archive per architecture with a
# checksum file and a manifest beside them. This is what "make release" runs.
#
# Everything lands in dist/. The updater of brief 6.3 reads manifest.json to find
# out what the newest version is and what its archives hash to, and the installer
# reads SHA256SUMS to check what it downloaded.
#
# Borrowed design: one flat SHA256SUMS covering every asset, written last, and one
# archive named after the target it was built for, from ZeroClaw's release
# workflow at ~/Code/zeroclaw/.github/workflows/release-stable-manual.yml.
#
# usage: build.sh [--version <version>] [--arch amd64,arm64] [--out <folder>]
set -eu

repository_root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$repository_root"

architectures="amd64 arm64"
output="$repository_root/dist"
version=""

say() { printf '%s\n' "$*"; }
die() { printf '%s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
	case "$1" in
	--version | --arch | --out)
		[ $# -ge 2 ] || die "build.sh's flag $1 needs a value after it"
		;;
	esac
	case "$1" in
	--version) version=$2; shift 2 ;;
	--arch) architectures=$(printf '%s' "$2" | tr ',' ' '); shift 2 ;;
	--out) output=$2; shift 2 ;;
	*) die "build.sh does not know the flag \"$1\"; it takes --version, --arch, and --out" ;;
	esac
done
[ -n "$architectures" ] || die "build.sh was given an empty --arch, so name at least one of amd64 and arm64"

if [ -z "$version" ]; then
	version=$(git -C "$repository_root" describe --tags --always --dirty 2>/dev/null || echo dev)
fi
for architecture in $architectures; do
	case "$architecture" in
	amd64 | arm64) ;;
	*) die "build.sh was asked for the architecture \"$architecture\", and a release is built for amd64 and arm64 only, because those are the two Node runtimes that are pinned" ;;
	esac
done

# The Node version lives in node.sh and is read back out of it, so that raising
# the pin is one edit in one file.
node_version=$(sed -n 's/^node_version=//p' "$repository_root/scripts/release/node.sh" | head -1)
[ -n "$node_version" ] || die "scripts/release/node.sh no longer sets node_version, so build.sh cannot say which runtime it bundled"

command -v npm >/dev/null 2>&1 ||
	die "npm is not on the PATH, and a release bundles each worker with the dependencies it needs at run time, so install Node 22 or later and run make release again"

sh "$repository_root/scripts/release/workers.sh"
for worker in browser desktop; do
	[ -f "$repository_root/worker/$worker/dist/main.js" ] ||
		die "worker/$worker/dist/main.js is not there, so there is no $worker worker to bundle; run \"npm ci && npm run build\" in worker/$worker and try again"
done

mkdir -p "$output"
rm -f "$output"/coeus-*.tar.gz "$output/SHA256SUMS" "$output/manifest.json"
staging="$output/.staging"
rm -rf "$staging"

# stage_worker puts one worker's bundle and the dependencies it needs at run time
# into the release tree. The dependencies are installed fresh from the lock file
# rather than copied out of the checkout, so that the release carries what the
# worker needs to run and none of the tools it was built and tested with.
stage_worker() {
	worker=$1
	into=$2
	mkdir -p "$into"
	cp -R "$repository_root/worker/$worker/dist" "$into/dist"
	cp "$repository_root/worker/$worker/package.json" "$into/package.json"
	cp "$repository_root/worker/$worker/package-lock.json" "$into/package-lock.json"
	(cd "$into" && npm ci --omit=dev --no-audit --no-fund >/dev/null)
}

built=""
for architecture in $architectures; do
	say "building coeus $version for $architecture"
	runtime=$(sh "$repository_root/scripts/release/node.sh" "$architecture")

	name="coeus-$version-$architecture"
	tree="$staging/$name"
	rm -rf "$tree"
	mkdir -p "$tree/node/bin" "$tree/worker"

	CGO_ENABLED=0 GOOS=linux GOARCH="$architecture" \
		go build -trimpath -ldflags "-X main.version=$version" -o "$tree/coeus" ./cmd/coeus
	printf '%s\n' "$version" >"$tree/VERSION"
	cp "$runtime/bin/node" "$tree/node/bin/node"
	chmod 755 "$tree/node/bin/node"
	for worker in browser desktop; do
		stage_worker "$worker" "$tree/worker/$worker"
	done

	tar -czf "$output/$name.tar.gz" -C "$staging" "$name"
	rm -rf "$tree"
	built="$built $architecture"
	say "wrote dist/$name.tar.gz"
done
rm -rf "$staging"

# The checksum file is written last and never lists itself, because a line for
# the file you are reading proves nothing.
(cd "$output" && sha256sum coeus-*.tar.gz >SHA256SUMS)
say "wrote dist/SHA256SUMS"

{
	printf '{\n'
	printf '  "version": "%s",\n' "$version"
	printf '  "date": "%s",\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	printf '  "node_version": "%s",\n' "$node_version"
	printf '  "architectures": ['
	first=yes
	for architecture in $built; do
		[ "$first" = yes ] || printf ', '
		first=no
		printf '"%s"' "$architecture"
	done
	printf '],\n'
	printf '  "artifacts": [\n'
	first=yes
	for architecture in $built; do
		[ "$first" = yes ] || printf ',\n'
		first=no
		name="coeus-$version-$architecture.tar.gz"
		sum=$(sha256sum "$output/$name" | cut -d ' ' -f 1)
		printf '    { "architecture": "%s", "file": "%s", "sha256": "%s" }' \
			"$architecture" "$name" "$sum"
	done
	printf '\n  ]\n}\n'
} >"$output/manifest.json"
say "wrote dist/manifest.json for coeus $version on$built"
