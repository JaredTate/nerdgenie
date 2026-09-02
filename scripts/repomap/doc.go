// Package main generates REPO_MAP.md from the repository tree, and its drift
// test fails the build when the map on disk no longer matches the tree.
//
// The map lists the files a fresh clone would hold, which is exactly what git
// says is tracked, so nothing lying around on one machine can get into it. The
// design is ported from HomeRecon's generator and its contract test, at
// ~/Code/homerecon/scripts/repo-map/generate-repo-map.mjs and
// ~/Code/homerecon/scripts/repo-map/repo-map.test.cjs, where the same three
// files have kept a much larger codebase legible to agents for months. Nothing
// was copied: that generator is JavaScript and this one is Go.
//
// Run it through make repo-map, which writes REPO_MAP.md. Never edit that file
// by hand.
package main
