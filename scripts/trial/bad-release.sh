#!/bin/sh
# Stages two releases in a folder for the human trial's last check: a good one
# built from this tree, and a bad one whose binary quits at once, so a person can
# watch "nerdgenie update" take the bad one, fail its readiness check, and go back
# to the good one within sixty seconds. It prints the two commands to run.
#
# Usage: scripts/trial/bad-release.sh [folder]   (default /tmp/nerdgenie-releases)
set -eu

folder="${1:-/tmp/nerdgenie-releases}"
good="${GOOD_VERSION:-0.9.0}"
bad="${BAD_VERSION:-0.9.1}"
architecture="$(go env GOARCH)"
here="$(cd "$(dirname "$0")/../.." && pwd)"

say() { printf '%s\n' "$*"; }

say "building the good release $good into dist/ (this runs make release)"
(cd "$here" && VERSION="$good" make release >/dev/null)
rm -rf "$folder"
mkdir -p "$folder/good" "$folder/bad"
cp "$here"/dist/nerdgenie-*.tar.gz "$here/dist/SHA256SUMS" "$here/dist/manifest.json" "$folder/good/"

say "making the bad release $bad, whose binary quits at once"
tree="$(mktemp -d)"
printf '#!/bin/sh\necho "this release is broken on purpose" >&2\nexit 1\n' >"$tree/nerdgenie"
chmod 755 "$tree/nerdgenie"
printf '%s\n' "$bad" >"$tree/VERSION"
mkdir -p "$tree/node/bin" "$tree/workers/browser" "$tree/workers/desktop"
: >"$tree/node/bin/node"
: >"$tree/workers/browser/main.js"
: >"$tree/workers/desktop/main.js"
name="nerdgenie-$bad-$architecture.tar.gz"
tar -czf "$folder/bad/$name" -C "$tree" nerdgenie VERSION node workers
rm -rf "$tree"
(cd "$folder/bad" && sha256sum "$name" >SHA256SUMS)
sum="$(cut -d' ' -f1 "$folder/bad/SHA256SUMS")"
printf '{"version":"%s","date":"%s","architectures":["%s"],"checksums":{"%s":"%s"}}\n' \
	"$bad" "$(date -u +%Y-%m-%d)" "$architecture" "$name" "$sum" >"$folder/bad/manifest.json"

say ""
say "The service must be installed and running for the rollback to be watched"
say "(nerdgenie install, then systemctl --user status nerdgenie.service). Then, in order:"
say ""
say "  nerdgenie update --from $folder/good     # takes $good and stays on it"
say "  nerdgenie update --from $folder/bad      # takes $bad, which never answers /readyz,"
say "                                        # and goes back to $good within sixty seconds"
say ""
say "Watch the terminal screen: the strip says disconnected and then healthy again,"
say "and the log names the version it went back to."
