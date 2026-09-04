package update_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/update"
)

// aWorkingProgram is the shell script a fixture release ships as its binary. It
// answers "update --migrate" the way the real program does, and writing a file
// is how a test sees that it ran.
const aWorkingProgram = "#!/bin/sh\necho \"$@\" >> \"$NERDGENIE_RECORD\"\nexit 0\n"

// aProgramThatFailsItsMigration is a release whose binary comes up but cannot
// bring the database forward, which is the case that has to put the backup back.
const aProgramThatFailsItsMigration = "#!/bin/sh\necho \"$@\" >> \"$NERDGENIE_RECORD\"\n" +
	"case \"$1\" in update) echo 'the migration failed' >&2; exit 1;; esac\nexit 0\n"

// aProgramThatExitsAtOnce is a release that dies the moment it is started, which
// is the deliberately bad release the readiness check has to catch.
const aProgramThatExitsAtOnce = "#!/bin/sh\nexit 1\n"

// aWorkerBundle stands in for the two TypeScript workers that travel with the
// binary, so that a test can prove the whole archive is unpacked.
const aWorkerBundle = "console.log('the browser worker');\n"

// aRelease writes one release into a folder exactly as brief 6.2 publishes it:
// the archive for this machine's architecture, the sums file beside it, and the
// manifest that names both. It returns the manifest it wrote.
func aRelease(t *testing.T, folder string, version string, program string) update.Manifest {
	t.Helper()
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		t.Fatalf("making the release folder %s failed: %v", folder, err)
	}
	archive := releaseArchive(t, program)
	name := update.ArchiveName(version, runtime.GOARCH)
	writeReleaseFile(t, filepath.Join(folder, name), archive)

	sum := sha256.Sum256(archive)
	checksum := hex.EncodeToString(sum[:])
	writeReleaseFile(t, filepath.Join(folder, update.ChecksumsName),
		[]byte(fmt.Sprintf("%s  %s\n", checksum, name)))

	manifest := update.Manifest{
		Version:       version,
		Date:          "2026-09-02",
		Architectures: []string{runtime.GOARCH},
		Checksums:     map[string]string{name: checksum},
	}
	written, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("writing the manifest failed: %v", err)
	}
	writeReleaseFile(t, filepath.Join(folder, update.ManifestName), written)
	return manifest
}

// releaseArchive packs the binary and a worker bundle into the gzipped tar a
// release ships.
func releaseArchive(t *testing.T, program string) []byte {
	t.Helper()
	return packArchive(t, []archiveEntry{
		{name: update.BinaryName, mode: 0o755, body: program},
		{name: "workers/browser/worker.js", mode: 0o644, body: aWorkerBundle},
	})
}

// archiveEntry is one file to put in a fixture archive.
type archiveEntry struct {
	// name is the path inside the archive.
	name string
	// mode is the permission the file is unpacked with.
	mode int64
	// body is what the file holds.
	body string
	// kind is the tar entry type, and is a regular file when it is zero.
	kind byte
	// linkTo is where a link entry points, for the archives a test uses to prove
	// that links are refused.
	linkTo string
}

// packArchive builds a gzipped tar holding the entries, which is what a release
// archive is.
func packArchive(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()
	packed := bytes.Buffer{}
	compressor := gzip.NewWriter(&packed)
	archive := tar.NewWriter(compressor)
	for _, entry := range entries {
		kind := entry.kind
		if kind == 0 {
			kind = tar.TypeReg
		}
		header := &tar.Header{
			Name: entry.name, Mode: entry.mode, Size: int64(len(entry.body)),
			Typeflag: kind, Linkname: entry.linkTo,
		}
		if kind != tar.TypeReg {
			header.Size = 0
		}
		if err := archive.WriteHeader(header); err != nil {
			t.Fatalf("writing the header for %s failed: %v", entry.name, err)
		}
		if kind == tar.TypeReg {
			if _, err := archive.Write([]byte(entry.body)); err != nil {
				t.Fatalf("writing %s into the archive failed: %v", entry.name, err)
			}
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("closing the archive failed: %v", err)
	}
	if err := compressor.Close(); err != nil {
		t.Fatalf("closing the compressor failed: %v", err)
	}
	return packed.Bytes()
}

// writeReleaseFile writes one file of a release.
func writeReleaseFile(t *testing.T, path string, written []byte) {
	t.Helper()
	if err := os.WriteFile(path, written, contract.DataFileMode); err != nil {
		t.Fatalf("writing %s failed: %v", path, err)
	}
}
