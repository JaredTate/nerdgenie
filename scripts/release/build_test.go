package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/update"
)

// The version the fixture release is built as. It is passed on the command line
// so that a test never depends on which tags this checkout happens to have.
const builtVersion = "9.9.9-test"

// pinnedNodeVersion is the Node runtime scripts/release/node.sh pins. The test
// reads it out of the script rather than writing it down twice, so that raising
// the pin does not silently leave a stale fixture behind.
func pinnedNodeVersion(t *testing.T) string {
	t.Helper()
	script := readFile(t, filepath.Join(repositoryRoot(t), "scripts", "release", "node.sh"))
	for _, line := range strings.Split(script, "\n") {
		if rest, found := strings.CutPrefix(strings.TrimSpace(line), "node_version="); found {
			return strings.Trim(rest, "\"'")
		}
	}
	t.Fatal("scripts/release/node.sh does not set node_version, so the tests cannot tell which runtime it pins")
	return ""
}

// fixtureCheckout builds a small repository shaped like this one, with a real Go
// program that the release can build for both architectures and worker bundles
// already built, and returns its root. Everything expensive about a real release
// is either already there or seeded into the cache, so the test runs in seconds.
func fixtureCheckout(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module nerdgenie.test\n\ngo 1.27\n")
	writeFile(t, filepath.Join(root, "cmd", "nerdgenie", "main.go"),
		"package main\n\nimport \"fmt\"\n\nvar version = \"unset\"\n\nfunc main() { fmt.Println(version) }\n")

	for _, worker := range []string{"browser", "desktop"} {
		folder := filepath.Join(root, "worker", worker)
		writeFile(t, filepath.Join(folder, "package.json"), `{"name":"nerdgenie-`+worker+`-worker","private":true}`+"\n")
		writeFile(t, filepath.Join(folder, "package-lock.json"), `{"lockfileVersion":3}`+"\n")
		writeFile(t, filepath.Join(folder, "tsconfig.json"), "{}\n")
		writeFile(t, filepath.Join(folder, "src", "main.ts"), "export {};\n")
		writeFile(t, filepath.Join(folder, "dist", "main.js"), "// the "+worker+" worker\n")
		writeFile(t, filepath.Join(folder, "node_modules", ".package-lock.json"), "{}\n")
		writeFile(t, filepath.Join(folder, "node_modules", "a-dependency", "index.js"), "module.exports = 1;\n")
	}

	for _, script := range []string{"build.sh", "node.sh"} {
		copyScript(t, filepath.Join(root, "scripts", "release"), "scripts/release/"+script)
	}
	return root
}

// seedNodeCache puts an already unpacked runtime in the cache for both
// architectures, which is what a second release on the same machine finds.
func seedNodeCache(t *testing.T, root string) string {
	t.Helper()
	cache := filepath.Join(root, ".cache", "release")
	version := pinnedNodeVersion(t)
	for _, architecture := range []string{"amd64", "arm64"} {
		writeExecutable(t, filepath.Join(cache, "node-"+version+"-"+architecture, "bin", "node"),
			"#!/bin/sh\necho v"+version+"\n")
	}
	return cache
}

// fakeNpm writes a stand-in for npm that records what it was asked and, for a
// production install, leaves one dependency behind. A release must not reach the
// npm registry from a test, and what matters here is that build.sh asks for a
// tree without the development tools in it and packs whatever npm leaves.
func fakeNpm(t *testing.T, root string) string {
	t.Helper()
	fakes := filepath.Join(root, "fakes")
	writeExecutable(t, filepath.Join(fakes, "npm"),
		"#!/bin/sh\nprintf 'npm %s (in %s)\\n' \"$*\" \"$PWD\" >> "+filepath.Join(root, "npm.log")+"\n"+
			"case \"$1\" in ci) mkdir -p node_modules/a-dependency && echo 'module.exports = 1;' > node_modules/a-dependency/index.js ;; esac\n"+
			"exit 0\n")
	return fakes
}

// npmWasAsked is everything the fake npm was asked to do.
func npmWasAsked(t *testing.T, root string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, "npm.log"))
	if err != nil {
		return ""
	}
	return string(content)
}

// buildTheFixtureRelease runs scripts/release/build.sh over the fixture checkout
// and fails the test when it does not finish cleanly.
func buildTheFixtureRelease(t *testing.T, root string, arguments ...string) string {
	t.Helper()
	cache := seedNodeCache(t, root)
	environment := cleanEnvironment(root, fakeNpm(t, root),
		"NERDGENIE_NODE_CACHE="+cache,
		"GOFLAGS=-mod=mod",
		// One build cache for the whole package, kept outside the fixture, because
		// a fresh cache per test means compiling the standard library for both
		// architectures again on every run.
		"GOCACHE="+filepath.Join(os.TempDir(), "nerdgenie-release-test-build-cache"),
		"GOPATH="+filepath.Join(root, ".gopath"),
		"CGO_ENABLED=0",
	)
	script := filepath.Join(root, "scripts", "release", "build.sh")
	whole := append([]string{"--version", builtVersion}, arguments...)

	printed, code := runScript(t, script, root, environment, whole...)
	if code != 0 {
		t.Fatalf("the release build exited %d, want 0. It printed:\n%s", code, printed)
	}
	return printed
}

func TestTheReleaseWritesAnArchiveForEachArchitecture(t *testing.T) {
	root := fixtureCheckout(t)

	printed := buildTheFixtureRelease(t, root)

	for _, architecture := range []string{"amd64", "arm64"} {
		archive := filepath.Join(root, "dist", "nerdgenie-"+builtVersion+"-"+architecture+".tar.gz")
		if _, err := os.Stat(archive); err != nil {
			t.Fatalf("%s was not written: %v\nThe build printed:\n%s", archive, err, printed)
		}
		names := namesInArchive(t, archive)
		for _, wanted := range []string{
			"nerdgenie", "VERSION", "node/bin/node",
			"workers/browser/main.js", "workers/desktop/main.js",
			"workers/browser/node_modules/a-dependency/index.js",
		} {
			if !slices.Contains(names, wanted) {
				t.Errorf("the %s archive holds no entry %q; it holds %d entries, the first of which is %q",
					architecture, wanted, len(names), firstName(names))
			}
		}
	}

	asked := npmWasAsked(t, root)
	for _, worker := range []string{"browser", "desktop"} {
		if !strings.Contains(asked, "ci --omit=dev") || !strings.Contains(asked, filepath.Join("workers", worker)) {
			t.Errorf("nothing installed the %s worker's run-time dependencies into the release tree. npm was asked:\n%s",
				worker, asked)
		}
	}
	// The desktop worker's driver ships a compiled library for each machine, so
	// the tree in the arm64 archive has to be installed for arm64 and not copied
	// from the one built here.
	for _, processor := range []string{"--cpu=x64", "--cpu=arm64"} {
		if !strings.Contains(asked, processor) {
			t.Errorf("no worker tree was installed with %s, so one archive carries the other machine's libraries. npm was asked:\n%s",
				processor, asked)
		}
	}
}

// firstName is the first path in an archive listing, used only to make a failure
// message say something about what was there instead.
func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func TestTheArchiveIsShapedTheWayTheUpdaterUnpacksOne(t *testing.T) {
	root := fixtureCheckout(t)

	buildTheFixtureRelease(t, root)

	for _, architecture := range []string{"amd64", "arm64"} {
		archive := filepath.Join(root, "dist", update.ArchiveName(builtVersion, architecture))

		// internal/update unpacks into a folder it made and then looks for the
		// program at the top of it, so the archive carries no folder of its own.
		if !slices.Contains(namesInArchive(t, archive), update.BinaryName) {
			t.Errorf("the %s archive has no %q at its top, so internal/update would say it is not a Nerd Genie release",
				architecture, update.BinaryName)
		}
		// And it refuses any entry that is not a plain file or a folder, because a
		// link is a way to write outside the folder being unpacked.
		if links := linksInArchive(t, archive); len(links) > 0 {
			t.Errorf("the %s archive holds %d links, which internal/update refuses; the first is %q",
				architecture, len(links), links[0])
		}
	}
}

func TestTheReleaseWritesAChecksumForEveryArchive(t *testing.T) {
	root := fixtureCheckout(t)

	buildTheFixtureRelease(t, root)

	sums := readFile(t, filepath.Join(root, "dist", "SHA256SUMS"))
	for _, architecture := range []string{"amd64", "arm64"} {
		name := "nerdgenie-" + builtVersion + "-" + architecture + ".tar.gz"
		want := fileChecksum(t, filepath.Join(root, "dist", name))
		if !strings.Contains(sums, want+"  "+name) {
			t.Errorf("SHA256SUMS has no line %q for %s. It says:\n%s", want+"  "+name, name, sums)
		}
	}
	if strings.Contains(sums, "SHA256SUMS") {
		t.Error("SHA256SUMS lists itself, which nothing can verify")
	}
}

func TestTheReleaseWritesAManifestTheUpdaterCanRead(t *testing.T) {
	root := fixtureCheckout(t)

	buildTheFixtureRelease(t, root)

	// The manifest is read back with the updater's own parser rather than with a
	// struct written out again here, because the whole point of the file is that
	// "nerdgenie update" can read it: a test with its own idea of the shape would go
	// on passing while the two drifted apart.
	content := readFile(t, filepath.Join(root, "dist", "manifest.json"))
	manifest, err := update.ParseManifest([]byte(content))
	if err != nil {
		t.Fatalf("internal/update refuses the manifest make release wrote: %v\nIt says:\n%s", err, content)
	}

	if manifest.Version != builtVersion {
		t.Errorf("the manifest says version %q, want %q", manifest.Version, builtVersion)
	}
	if !strings.HasSuffix(manifest.Date, "Z") || len(manifest.Date) != len("2026-01-02T15:04:05Z") {
		t.Errorf("the manifest date is %q, want a UTC time such as 2026-01-02T15:04:05Z", manifest.Date)
	}
	if len(manifest.Architectures) != 2 {
		t.Fatalf("the manifest names %d architectures, want amd64 and arm64: %s", len(manifest.Architectures), content)
	}
	for _, architecture := range manifest.Architectures {
		said, err := manifest.ChecksumFor(architecture)
		if err != nil {
			t.Errorf("the updater cannot find the %s checksum in the manifest: %v", architecture, err)
			continue
		}
		want := fileChecksum(t, filepath.Join(root, "dist", update.ArchiveName(builtVersion, architecture)))
		if said != want {
			t.Errorf("the manifest says the %s archive hashes to %q, want %q", architecture, said, want)
		}
	}
}

func TestTheManifestSaysWhichNodeRuntimeTheArchivesCarry(t *testing.T) {
	root := fixtureCheckout(t)

	buildTheFixtureRelease(t, root)

	var extra struct {
		NodeVersion string `json:"node_version"`
	}
	content := readFile(t, filepath.Join(root, "dist", "manifest.json"))
	if err := json.Unmarshal([]byte(content), &extra); err != nil {
		t.Fatalf("manifest.json is not readable JSON: %v\nIt says:\n%s", err, content)
	}
	if extra.NodeVersion != pinnedNodeVersion(t) {
		t.Errorf("the manifest says Node %q, want the pinned %q", extra.NodeVersion, pinnedNodeVersion(t))
	}
}

func TestTheReleaseBuildsOnlyTheArchitecturesAskedFor(t *testing.T) {
	root := fixtureCheckout(t)

	buildTheFixtureRelease(t, root, "--arch", "amd64")

	if _, err := os.Stat(filepath.Join(root, "dist", "nerdgenie-"+builtVersion+"-amd64.tar.gz")); err != nil {
		t.Errorf("--arch amd64 did not write the amd64 archive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "dist", "nerdgenie-"+builtVersion+"-arm64.tar.gz")); err == nil {
		t.Error("--arch amd64 wrote an arm64 archive as well")
	}
}

func TestTheReleaseRefusesAnArchitectureItCannotBuild(t *testing.T) {
	root := fixtureCheckout(t)
	cache := seedNodeCache(t, root)
	environment := cleanEnvironment(root, filepath.Join(root, "no-fakes"), "NERDGENIE_NODE_CACHE="+cache)

	printed, code := runScript(t, filepath.Join(root, "scripts", "release", "build.sh"), root, environment,
		"--version", builtVersion, "--arch", "sparc")

	if code == 0 {
		t.Fatalf("an architecture with no pinned Node runtime was accepted. It printed:\n%s", printed)
	}
	requirePrinted(t, printed, "sparc", "the refusal names the architecture it was asked for")
	requirePrinted(t, printed, "amd64", "the refusal names the architectures it does know")
}

// fileChecksum is the SHA-256 of one file, written the way sha256sum writes it.
func fileChecksum(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s to check it: %v", path, err)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
