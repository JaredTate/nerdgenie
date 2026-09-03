// The manifest and its checksums follow ZeroClaw's updater at
// ~/Code/zeroclaw/src/commands/update.rs, which finds the archive for the
// machine it is running on in a release and checks it against a sums file
// published beside it before anything is written. What is different here is that
// the sums live in the manifest itself, so one small document says which
// versions exist, which machines they were built for, and what each archive must
// hash to, and a release is read in one request rather than three.

package update

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MaxManifestBytes is the most of a manifest that is ever read. The document is
// a version, a date, a handful of architectures, and one checksum each, so
// anything past this is not a manifest.
const MaxManifestBytes = 64 << 10

// ChecksumDigits is how many hexadecimal digits a SHA-256 sum is written in.
const ChecksumDigits = 64

// ManifestName is the file in a release that says what the release holds.
const ManifestName = "manifest.json"

// ChecksumsName is the sums file published beside the manifest for people and
// for the installer script of brief 6.2 to read. The updater checks against the
// manifest, which carries the same sums.
const ChecksumsName = "SHA256SUMS"

// BinaryName is the program inside a release archive, and the file the current
// link is made to point at.
const BinaryName = "coeus"

// Manifest is what one release says about itself. A field this version does not
// know about is ignored, so that a manifest written by a later release can still
// be read by this one.
type Manifest struct {
	// Version is the version the release holds, as it is written in the archive
	// names and in the releases folder.
	Version string `json:"version"`
	// Date is the day the release was built, for the person reading the check.
	Date string `json:"date"`
	// Architectures are the machines the release was built for, written the way
	// Go writes them: amd64 and arm64.
	Architectures []string `json:"architectures"`
	// Checksums is the SHA-256 sum of every archive, by the archive's own name.
	Checksums map[string]string `json:"checksums"`
}

// ArchiveName is the file one architecture's release is packed into, which is
// the name brief 6.2 writes into the release and into the sums file.
func ArchiveName(version string, architecture string) string {
	return fmt.Sprintf("coeus-%s-%s.tar.gz", version, architecture)
}

// ParseManifest reads a release manifest and refuses one that does not say
// everything a download needs: a version that can be a folder name, at least one
// architecture, and a checksum for each of them.
func ParseManifest(written []byte) (Manifest, error) {
	if len(written) > MaxManifestBytes {
		return Manifest{}, fmt.Errorf("the release manifest is %d bytes and the limit is %d, so the address is serving something other than a manifest",
			len(written), MaxManifestBytes)
	}
	manifest := Manifest{}
	if err := json.Unmarshal(written, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("the release manifest could not be read as JSON, so check that the address serves %s: %w", ManifestName, err)
	}
	if err := manifest.check(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// check holds the rules a manifest has to keep for the rest of this package to
// trust what it says.
func (manifest Manifest) check() error {
	if !plainName(manifest.Version) {
		return fmt.Errorf("the release manifest gives the version as %q, which cannot be a folder name, so the address is serving something other than a Coeus manifest",
			manifest.Version)
	}
	if len(manifest.Architectures) == 0 {
		return fmt.Errorf("the release manifest for version %s names no architecture, so there is nothing to install", manifest.Version)
	}
	for _, architecture := range manifest.Architectures {
		if !plainName(architecture) {
			return fmt.Errorf("the release manifest names %q as an architecture, which cannot be part of a file name, so the manifest is not one Coeus can use", architecture)
		}
		if _, err := manifest.ChecksumFor(architecture); err != nil {
			return err
		}
	}
	return nil
}

// ChecksumFor is the SHA-256 sum the manifest gives for this machine's
// architecture, and an error naming the architecture when the release was not
// built for it.
func (manifest Manifest) ChecksumFor(architecture string) (string, error) {
	name := ArchiveName(manifest.Version, architecture)
	checksum, found := manifest.Checksums[name]
	if !found {
		return "", fmt.Errorf("the release manifest for version %s has no checksum for %s, so there is no build for the %s architecture to install",
			manifest.Version, name, architecture)
	}
	if !hexadecimalSum(checksum) {
		return "", fmt.Errorf("the release manifest gives the checksum of %s as %q, which is not %d hexadecimal digits, so the manifest cannot be trusted",
			name, checksum, ChecksumDigits)
	}
	return checksum, nil
}

// plainName says whether a word is safe to put in a path: letters, digits, dots,
// hyphens, plus signs and underscores, and nothing that could climb out of a
// folder or start one somewhere else.
func plainName(word string) bool {
	if word == "" || len(word) > 64 || word == "." || word == ".." {
		return false
	}
	for _, letter := range word {
		switch {
		case letter >= 'a' && letter <= 'z', letter >= 'A' && letter <= 'Z',
			letter >= '0' && letter <= '9':
		case letter == '.' || letter == '-' || letter == '+' || letter == '_':
		default:
			return false
		}
	}
	return !strings.Contains(word, "..")
}

// hexadecimalSum says whether the text is a SHA-256 sum written out in lower
// case, which is how every tool that writes one writes it.
func hexadecimalSum(written string) bool {
	if len(written) != ChecksumDigits {
		return false
	}
	for _, letter := range written {
		if (letter < '0' || letter > '9') && (letter < 'a' || letter > 'f') {
			return false
		}
	}
	return true
}
