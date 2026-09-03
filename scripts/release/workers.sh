#!/bin/sh
# Build the two TypeScript workers into their dist folders, which is what "make
# build" and "make release" both need before there is anything to bundle.
#
# A worker whose dist is already newer than everything it is built from is left
# alone, so that a Go-only change does not pay for a TypeScript build. A machine
# with no npm is told so in one line and the script still succeeds: a Go
# developer must be able to run "make build" without installing Node, and "make
# release" checks for itself that the bundles are there before it packs one.
#
# usage: workers.sh
set -eu

repository_root=$(cd "$(dirname "$0")/../.." && pwd)
workers="browser desktop"

say() { printf '%s\n' "$*"; }

if ! command -v npm >/dev/null 2>&1; then
	say "npm is not on the PATH, so no worker bundle was built; install Node 22 or later to build them"
	exit 0
fi

for worker in $workers; do
	folder="$repository_root/worker/$worker"
	if [ ! -f "$folder/package.json" ]; then
		say "there is no worker/$worker, so nothing was built for it"
		continue
	fi

	# npm ci wipes and rebuilds node_modules from the lock file, so it is only
	# worth running when the lock file has moved on since the last one.
	if [ ! -f "$folder/node_modules/.package-lock.json" ] ||
		[ "$folder/package-lock.json" -nt "$folder/node_modules/.package-lock.json" ]; then
		say "installing the $worker worker's dependencies"
		(cd "$folder" && npm ci --no-audit --no-fund >/dev/null)
	fi

	if [ -f "$folder/dist/main.js" ] &&
		[ -z "$(find "$folder/src" "$folder/package.json" "$folder/tsconfig.json" -newer "$folder/dist/main.js" 2>/dev/null)" ]; then
		say "the $worker worker bundle is already up to date"
		continue
	fi

	say "building the $worker worker bundle"
	(cd "$folder" && npm run build >/dev/null)
done
