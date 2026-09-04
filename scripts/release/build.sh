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

# Compiling the workers is the build target's job, so a release only checks that
# it was done rather than doing it a second way.
for worker in browser desktop; do
	[ -f "$repository_root/worker/$worker/dist/main.js" ] ||
		die "worker/$worker/dist/main.js is not there, so there is no $worker worker to bundle; run \"make build\", which compiles both, and try again"
done

mkdir -p "$output"
rm -f "$output"/nerdgenie-*.tar.gz "$output/SHA256SUMS" "$output/manifest.json"
staging="$output/.staging"
rm -rf "$staging"

# stage_worker puts one worker's bundle and the dependencies it needs at run time
# into the release tree. The dependencies are installed fresh from the lock file
# rather than copied out of the checkout, for two reasons: the release then
# carries what the worker needs to run and none of the tools it was built and
# tested with, and npm is told which machine the release is for. That second one
# matters because the desktop worker's driver ships a compiled library per
# architecture, so a tree installed here and copied would put this machine's
# x86_64 library in the arm64 archive, where nothing could load it.
stage_worker() {
	worker=$1
	into=$2
	processor=$3
	# The bundle sits at workers/<name>/main.js, the same place bin/workers puts it
	# in a checkout, so one path finds a worker in both.
	cp -R "$repository_root/worker/$worker/dist" "$into"
	cp "$repository_root/worker/$worker/package.json" "$into/package.json"
	cp "$repository_root/worker/$worker/package-lock.json" "$into/package-lock.json"
	(cd "$into" && npm ci --omit=dev --os=linux --cpu="$processor" --no-audit --no-fund >/dev/null)
	# npm leaves node_modules/.bin full of links to the programs a package ships,
	# and a release runs none of them: it starts a worker as "node main.js". The
	# folder goes, because internal/update refuses an archive holding any link at
	# all, and a link is a way to write outside the folder being unpacked.
	rm -rf "$into/node_modules/.bin"
}

built=""
for architecture in $architectures; do
	say "building nerdgenie $version for $architecture"
	runtime=$(sh "$repository_root/scripts/release/node.sh" "$architecture")
	# Go and npm have different words for the same two machines.
	case "$architecture" in
	amd64) processor=x64 ;;
	*) processor=$architecture ;;
	esac

	name="nerdgenie-$version-$architecture"
	tree="$staging/$name"
	rm -rf "$tree"
	mkdir -p "$tree/node/bin" "$tree/workers"

	CGO_ENABLED=0 GOOS=linux GOARCH="$architecture" \
		go build -trimpath -ldflags "-X main.version=$version" -o "$tree/nerdgenie" ./cmd/nerdgenie
	printf '%s\n' "$version" >"$tree/VERSION"
	cp "$runtime/bin/node" "$tree/node/bin/node"
	chmod 755 "$tree/node/bin/node"
	for worker in browser desktop; do
		stage_worker "$worker" "$tree/workers/$worker" "$processor"
	done

	# Any link left in the tree is a release internal/update would refuse on the
	# user's machine, so it is found here instead, where there is somebody to fix it.
	links=$(find "$tree" -type l)
	[ -z "$links" ] || die "the release tree still holds links, and a release archive may hold none, so take these out of scripts/release/build.sh's staging step: $links"

	# The archive has no folder of its own at the top: internal/update unpacks it
	# into a folder it has already made and then looks for nerdgenie in it, and the
	# installer does the same, so a wrapper folder would only be stripped twice.
	tar -czf "$output/$name.tar.gz" -C "$tree" nerdgenie VERSION node workers
	rm -rf "$tree"
	built="$built $architecture"
	say "wrote dist/$name.tar.gz"
done
rm -rf "$staging"

# The checksum file is written last and never lists itself, because a line for
# the file you are reading proves nothing.
(cd "$output" && sha256sum nerdgenie-*.tar.gz >SHA256SUMS)
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
	# The sums are keyed by the archive's own name, which is the shape
	# internal/update's ParseManifest reads: it looks one up by the name it works
	# out for this machine, so a map says what a list would only imply.
	printf '  "checksums": {\n'
	first=yes
	for architecture in $built; do
		[ "$first" = yes ] || printf ',\n'
		first=no
		name="nerdgenie-$version-$architecture.tar.gz"
		sum=$(sha256sum "$output/$name" | cut -d ' ' -f 1)
		printf '    "%s": "%s"' "$name" "$sum"
	done
	printf '\n  }\n}\n'
} >"$output/manifest.json"
say "wrote dist/manifest.json for nerdgenie $version on$built"
