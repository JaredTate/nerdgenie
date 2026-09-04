package update_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/update"
)

// FuzzParseManifest throws arbitrary bytes at the one thing this package parses
// from outside: the release manifest an address serves. Nothing may panic, and a
// manifest that is accepted has to be one the rest of the package can use.
func FuzzParseManifest(f *testing.F) {
	f.Add([]byte(aGoodManifest()))
	f.Add([]byte(`{"version":"0.7.0","architectures":["amd64"],"checksums":{}}`))
	f.Add([]byte(`{"version":"../..","architectures":[],"checksums":null}`))
	f.Add([]byte(`{"version":0,"architectures":"amd64"}`))
	f.Add([]byte("{"))
	f.Add([]byte(""))
	f.Add([]byte(strings.Repeat("{\"version\":\"", 100)))

	f.Fuzz(func(t *testing.T, written []byte) {
		manifest, err := update.ParseManifest(written)
		if err != nil {
			return
		}
		if manifest.Version == "" || strings.ContainsAny(manifest.Version, "/\\") {
			t.Errorf("a manifest with the version %q was accepted, and it cannot be a folder name", manifest.Version)
		}
		if len(manifest.Architectures) == 0 {
			t.Errorf("a manifest naming no architecture was accepted")
		}
		for _, architecture := range manifest.Architectures {
			checksum, err := manifest.ChecksumFor(architecture)
			if err != nil {
				t.Errorf("the manifest was accepted but has no usable checksum for %s: %v", architecture, err)
			}
			if len(checksum) != update.ChecksumDigits {
				t.Errorf("the checksum for %s is %d digits rather than %d", architecture, len(checksum), update.ChecksumDigits)
			}
		}
	})
}

// FuzzNewer throws arbitrary text at the version comparison, which reads version
// strings that came from a manifest and from a folder name. It may not panic,
// and a version can never be newer than itself.
func FuzzNewer(f *testing.F) {
	f.Add("0.7.0", "0.7.1")
	f.Add("v1", "1.0.0-rc1")
	f.Add("dev", "")
	f.Add("....", "9999999999999999999999")
	f.Add("-1", "+2")

	f.Fuzz(func(t *testing.T, running string, candidate string) {
		if update.Newer(running, running) {
			t.Errorf("version %q is reported as newer than itself", running)
		}
		if update.Newer(running, candidate) && update.Newer(candidate, running) {
			t.Errorf("versions %q and %q are each reported as newer than the other", running, candidate)
		}
		update.ArchiveName(candidate, runtime.GOARCH)
	})
}
