package update_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/update"
)

// aChecksum is a plausible SHA-256 sum, which is sixty-four hexadecimal digits.
const aChecksum = "1111111111111111111111111111111111111111111111111111111111111111"

// anotherChecksum is a second one, so that a test can tell two archives apart.
const anotherChecksum = "2222222222222222222222222222222222222222222222222222222222222222"

// aGoodManifest is the manifest brief 6.2 publishes beside the archives: the
// version, the day it was made, the architectures, and one checksum each.
func aGoodManifest() string {
	return `{
		"version": "0.7.0",
		"date": "2026-09-02",
		"architectures": ["amd64", "arm64"],
		"checksums": {
			"nerdgenie-0.7.0-amd64.tar.gz": "` + aChecksum + `",
			"nerdgenie-0.7.0-arm64.tar.gz": "` + anotherChecksum + `"
		}
	}`
}

func TestAManifestSaysWhatIsOnOffer(t *testing.T) {
	manifest, err := update.ParseManifest([]byte(aGoodManifest()))

	if err != nil {
		t.Fatalf("reading a good manifest failed: %v", err)
	}
	if manifest.Version != "0.7.0" {
		t.Errorf("the manifest offers %q rather than 0.7.0", manifest.Version)
	}
	if manifest.Date != "2026-09-02" {
		t.Errorf("the manifest is dated %q rather than 2026-09-02", manifest.Date)
	}
	if len(manifest.Architectures) != 2 {
		t.Errorf("the manifest names %d architectures rather than two", len(manifest.Architectures))
	}
}

func TestTheArchiveIsNamedForItsVersionAndArchitecture(t *testing.T) {
	name := update.ArchiveName("0.7.0", "arm64")

	if name != "nerdgenie-0.7.0-arm64.tar.gz" {
		t.Errorf("the archive is called %q rather than nerdgenie-0.7.0-arm64.tar.gz", name)
	}
}

func TestAManifestHandsBackTheChecksumForOneArchitecture(t *testing.T) {
	manifest, err := update.ParseManifest([]byte(aGoodManifest()))
	if err != nil {
		t.Fatalf("reading a good manifest failed: %v", err)
	}

	checksum, err := manifest.ChecksumFor("arm64")

	if err != nil {
		t.Fatalf("the manifest has no checksum for arm64: %v", err)
	}
	if checksum != anotherChecksum {
		t.Errorf("the checksum for arm64 is %q rather than %q", checksum, anotherChecksum)
	}
}

func TestAManifestSaysSoWhenThereIsNoBuildForThisMachine(t *testing.T) {
	manifest, err := update.ParseManifest([]byte(aGoodManifest()))
	if err != nil {
		t.Fatalf("reading a good manifest failed: %v", err)
	}

	_, err = manifest.ChecksumFor("riscv64")

	if err == nil {
		t.Fatalf("the manifest handed back a checksum for an architecture it does not build")
	}
	if !strings.Contains(err.Error(), "riscv64") {
		t.Errorf("the refusal does not name the architecture that is missing: %v", err)
	}
}

func TestAManifestThatIsWrongIsRefusedWithAReason(t *testing.T) {
	cases := []struct {
		name    string
		written string
		says    string
	}{
		{"not JSON at all", "this is not a manifest", "manifest"},
		{"no version", `{"architectures":["amd64"],"checksums":{}}`, "version"},
		{"a version that is a path", `{"version":"../../etc","architectures":["amd64"],"checksums":{}}`, "version"},
		{"no architectures", `{"version":"0.7.0","architectures":[],"checksums":{}}`, "architecture"},
		{"an architecture with a slash in it", `{"version":"0.7.0","architectures":["a/b"],"checksums":{}}`, "architecture"},
		{"no checksum for an architecture", `{"version":"0.7.0","architectures":["amd64"],"checksums":{}}`, "checksum"},
		{
			"a checksum that is not a sum",
			`{"version":"0.7.0","architectures":["amd64"],"checksums":{"nerdgenie-0.7.0-amd64.tar.gz":"nope"}}`,
			"checksum",
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			_, err := update.ParseManifest([]byte(one.written))

			if err == nil {
				t.Fatalf("a manifest with %s was accepted", one.name)
			}
			if !strings.Contains(err.Error(), one.says) {
				t.Errorf("the refusal does not say what is wrong, it says: %v", err)
			}
		})
	}
}

func TestAChecksumThatIsTheRightLengthButNotHexadecimalIsRefused(t *testing.T) {
	written := `{"version":"0.7.0","architectures":["amd64"],
		"checksums":{"nerdgenie-0.7.0-amd64.tar.gz":"` + strings.Repeat("z", 64) + `"}}`

	if _, err := update.ParseManifest([]byte(written)); err == nil {
		t.Errorf("a checksum of sixty-four letters that are not digits was accepted")
	}
}

func TestAManifestLongerThanTheCapIsRefused(t *testing.T) {
	_, err := update.ParseManifest([]byte(strings.Repeat("x", update.MaxManifestBytes+1)))

	if err == nil {
		t.Fatalf("a manifest longer than the cap was read rather than refused")
	}
}

func TestAManifestMayCarryFieldsThisVersionDoesNotKnow(t *testing.T) {
	written := `{"version":"0.7.0","date":"2026-09-02","architectures":["amd64"],
		"checksums":{"nerdgenie-0.7.0-amd64.tar.gz":"` + aChecksum + `"},"notes":"anything at all"}`

	if _, err := update.ParseManifest([]byte(written)); err != nil {
		t.Fatalf("a manifest with a field from a later release was refused: %v", err)
	}
}
