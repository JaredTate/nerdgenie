#!/usr/bin/env bash
# Generate REPO_MAP.md from the repository tree.
#
# This follows the HomeRecon pattern: the map is generated, never edited by
# hand, and a drift test in `make check` fails when it is stale. The map lists
# every tracked file except dependency folders, build outputs, test artifacts,
# and version-control internals, so a fresh clone regenerates it byte for byte.
#
# Wave 0 replaces this script with the Go version in scripts/repomap/ and adds
# the drift test. Until then, run it by hand: scripts/repo-map.sh > REPO_MAP.md
set -euo pipefail
cd "$(dirname "$0")/.."

echo "# Repository Map"
echo
echo "<!-- generated: scripts/repo-map.sh -->"
echo "<!-- repo-map-contract: v1 -->"
echo
echo "This file is generated from the repository tree. Regenerate it with \`make repo-map\`."
echo "The generator omits dependency folders, build outputs, test artifacts, and version-control internals."
echo
echo "## Roots"
echo
echo "- \`./\` - The three living documents, the license, the third-party notes, the Makefile."
echo "- \`cmd/coeus/\` - The one binary and its subcommands."
echo "- \`internal/\` - The Go packages, one job each; see ARCHITECTURE.md for the list."
echo "- \`worker/\` - The TypeScript browser and desktop workers."
echo "- \`test/\` - Functional tests and fixtures."
echo "- \`scripts/\` - The installer, the repo-map generator, CI helpers."
echo "- \`docs/\` - The design, the work plan, the comparison, the research, the references, the briefs."
echo
echo "## Tree"
echo
echo '```text'
echo "."
git ls-files | grep -vE '^(node_modules|bin|dist|coverage|\.git)/' | grep -vE '/(node_modules|dist|coverage|target)/' | grep -vE '\.(log|tsbuildinfo)$' | sort
echo '```'
