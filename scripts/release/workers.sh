#!/bin/sh
# Build the two TypeScript workers and put them where the agent looks for them,
# which is bin/workers/<name>/main.js. That is what "make build" needs, and the
# same shape a release carries, so one path works in a checkout and in an install.
#
# A worker whose dist is already newer than everything it is built from is left
# alone, so that a Go-only change does not pay for a TypeScript build. A machine
# with no npm is told so in one line and the script still succeeds: a Go developer
# must be able to run "make build" without installing Node, and "make release"
# checks for itself that the bundles are there before it packs one.
#
# usage: workers.sh
set -eu

repository_root=$(cd "$(dirname "$0")/../.." && pwd)
say() { printf '%s\n' "$*"; }

have_npm=yes
command -v npm >/dev/null 2>&1 || have_npm=no

for worker in browser desktop; do
	folder="$repository_root/worker/$worker"
	if [ ! -f "$folder/package.json" ]; then
		say "there is no worker/$worker, so nothing was built for it"
		continue
	fi

	if [ "$have_npm" = no ]; then
		say "npm is not on the PATH, so the $worker worker was not built; install Node 22 or later to build it"
	else
		# npm ci wipes and rebuilds node_modules from the lock file, so it is only
		# worth running when the lock file has moved on since the last one.
		if [ ! -f "$folder/node_modules/.package-lock.json" ] ||
			[ -n "$(find "$folder/package-lock.json" -newer "$folder/node_modules/.package-lock.json" 2>/dev/null)" ]; then
			say "installing the $worker worker's dependencies"
			(cd "$folder" && npm ci --no-audit --no-fund >/dev/null)
		fi
		if [ -f "$folder/dist/main.js" ] &&
			[ -z "$(find "$folder/src" "$folder/package.json" "$folder/tsconfig.json" -newer "$folder/dist/main.js" 2>/dev/null)" ]; then
			say "the $worker worker bundle is already up to date"
		else
			say "building the $worker worker bundle"
			(cd "$folder" && npm run build >/dev/null)
		fi
	fi

	# The bundle is copied rather than linked, and its dependencies are linked
	# rather than copied: the first so that bin/ holds a whole worker, the second
	# because node_modules is hundreds of megabytes that a build must not double.
	if [ -f "$folder/dist/main.js" ]; then
		into="$repository_root/bin/workers/$worker"
		rm -rf "$into"
		mkdir -p "$repository_root/bin/workers"
		cp -R "$folder/dist" "$into"
		ln -sfn "$folder/node_modules" "$into/node_modules"
		say "the $worker worker is at bin/workers/$worker/main.js"
	fi
done
