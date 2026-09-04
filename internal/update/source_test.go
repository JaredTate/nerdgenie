package update_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/update"
)

// aReleaseServer serves one release folder over a loopback address, which is
// what stands in for the GitHub release brief 6.2 publishes.
func aReleaseServer(t *testing.T, folder string) string {
	t.Helper()
	server := httptest.NewServer(http.FileServer(http.Dir(folder)))
	t.Cleanup(server.Close)
	return server.URL
}

func TestAManifestIsReadFromAFolderOnThisMachine(t *testing.T) {
	folder := t.TempDir()
	aRelease(t, folder, "0.7.0", aWorkingProgram)

	manifest, err := update.Source{Address: folder}.Manifest(context.Background())

	if err != nil {
		t.Fatalf("reading the manifest from a folder failed: %v", err)
	}
	if manifest.Version != "0.7.0" {
		t.Errorf("the folder offers version %q rather than 0.7.0", manifest.Version)
	}
}

func TestAManifestIsReadFromAWebAddress(t *testing.T) {
	folder := t.TempDir()
	aRelease(t, folder, "0.7.0", aWorkingProgram)
	address := aReleaseServer(t, folder)

	manifest, err := update.Source{Address: address}.Manifest(context.Background())

	if err != nil {
		t.Fatalf("reading the manifest over the network failed: %v", err)
	}
	if manifest.Version != "0.7.0" {
		t.Errorf("the address offers version %q rather than 0.7.0", manifest.Version)
	}
}

func TestAnArchiveIsFetchedFromAWebAddress(t *testing.T) {
	folder := t.TempDir()
	manifest := aRelease(t, folder, "0.7.0", aWorkingProgram)
	address := aReleaseServer(t, folder)
	name := update.ArchiveName("0.7.0", runtime.GOARCH)

	fetched := bytes.Buffer{}
	read, err := update.Source{Address: address}.Fetch(context.Background(), name, &fetched)

	if err != nil {
		t.Fatalf("fetching the archive failed: %v", err)
	}
	if read != int64(fetched.Len()) {
		t.Errorf("the fetch says it read %d bytes and wrote %d", read, fetched.Len())
	}
	onDisk, err := os.ReadFile(filepath.Join(folder, name))
	if err != nil {
		t.Fatalf("reading the archive back failed: %v", err)
	}
	if !bytes.Equal(fetched.Bytes(), onDisk) {
		t.Errorf("the archive that arrived is not the one that was published")
	}
	if _, err := manifest.ChecksumFor(runtime.GOARCH); err != nil {
		t.Errorf("the fixture manifest has no checksum for this machine: %v", err)
	}
}

func TestAnAddressThatServesNoManifestSaysWhereItLooked(t *testing.T) {
	address := aReleaseServer(t, t.TempDir())

	_, err := update.Source{Address: address}.Manifest(context.Background())

	if err == nil {
		t.Fatalf("an address with no manifest on it was accepted")
	}
	if !strings.Contains(err.Error(), update.ManifestName) {
		t.Errorf("the refusal does not say which file was missing: %v", err)
	}
}

func TestAFolderThatServesNoManifestSaysWhereItLooked(t *testing.T) {
	folder := t.TempDir()

	_, err := update.Source{Address: folder}.Manifest(context.Background())

	if err == nil {
		t.Fatalf("a folder with no manifest in it was accepted")
	}
	if !strings.Contains(err.Error(), folder) {
		t.Errorf("the refusal does not say where it looked: %v", err)
	}
}

func TestAnAddressWithAnUnknownSchemeIsRefused(t *testing.T) {
	_, err := update.Source{Address: "ftp://example.invalid/releases"}.Manifest(context.Background())

	if err == nil {
		t.Fatalf("an address Nerd Genie cannot read was accepted")
	}
	if !strings.Contains(err.Error(), "ftp://example.invalid/releases") {
		t.Errorf("the refusal does not name the address: %v", err)
	}
}

func TestAnEmptyAddressIsRefused(t *testing.T) {
	_, err := update.Source{}.Manifest(context.Background())

	if err == nil {
		t.Fatalf("a source with no address was accepted")
	}
}

func TestAFetchStopsAtTheDownloadCap(t *testing.T) {
	folder := t.TempDir()
	huge := bytes.Repeat([]byte("x"), 4096)
	writeReleaseFile(t, filepath.Join(folder, "big.tar.gz"), huge)

	source := update.Source{Address: folder, MaxBytes: 100}
	_, err := source.Fetch(context.Background(), "big.tar.gz", &bytes.Buffer{})

	if err == nil {
		t.Fatalf("a download longer than the cap was read to the end")
	}
	if !strings.Contains(err.Error(), "big.tar.gz") {
		t.Errorf("the refusal does not name the file it was reading: %v", err)
	}
}

func TestAFetchRefusesANameThatClimbsOutOfTheRelease(t *testing.T) {
	folder := t.TempDir()
	writeReleaseFile(t, filepath.Join(folder, "secret"), []byte("not yours"))

	source := update.Source{Address: filepath.Join(folder, "releases")}
	_, err := source.Fetch(context.Background(), "../secret", &bytes.Buffer{})

	if err == nil {
		t.Fatalf("a file name that climbs out of the release folder was fetched")
	}
}

func TestAnAddressThatAnswersWithAnErrorSaysWhatItAnswered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no such release", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	_, err := update.Source{Address: server.URL}.Manifest(context.Background())

	if err == nil {
		t.Fatalf("an address that answered 404 was accepted")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("the refusal does not say what the address answered: %v", err)
	}
}

func TestAFolderSourceReadsTheSumsFilePublishedBesideTheManifest(t *testing.T) {
	folder := t.TempDir()
	aRelease(t, folder, "0.7.0", aWorkingProgram)

	sums := bytes.Buffer{}
	if _, err := (update.Source{Address: folder}).Fetch(context.Background(), update.ChecksumsName, &sums); err != nil {
		t.Fatalf("reading the sums file failed: %v", err)
	}

	if !strings.Contains(sums.String(), update.ArchiveName("0.7.0", runtime.GOARCH)) {
		t.Errorf("the sums file does not name the archive:\n%s", sums.String())
	}
}
