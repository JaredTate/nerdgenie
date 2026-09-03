package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// aPackedRelease builds the gzipped tar a release ships, with the entries given.
func aPackedRelease(t *testing.T, entries []tar.Header, bodies []string) []byte {
	t.Helper()
	packed := bytes.Buffer{}
	compressor := gzip.NewWriter(&packed)
	archive := tar.NewWriter(compressor)
	for at, header := range entries {
		if err := archive.WriteHeader(&header); err != nil {
			t.Fatalf("writing the header for %s failed: %v", header.Name, err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := archive.Write([]byte(bodies[at])); err != nil {
				t.Fatalf("writing %s into the archive failed: %v", header.Name, err)
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

// aReleaseHolding publishes one archive with the entries given into a folder,
// together with the manifest that names its checksum.
func aReleaseHolding(t *testing.T, folder string, version string, archive []byte) Manifest {
	t.Helper()
	name := ArchiveName(version, runtime.GOARCH)
	if err := os.WriteFile(filepath.Join(folder, name), archive, contract.DataFileMode); err != nil {
		t.Fatalf("publishing the archive failed: %v", err)
	}
	sum := sha256.Sum256(archive)
	return Manifest{
		Version:       version,
		Architectures: []string{runtime.GOARCH},
		Checksums:     map[string]string{name: hex.EncodeToString(sum[:])},
	}
}

// aPlainRelease is the ordinary archive: the binary and one worker bundle.
func aPlainRelease(t *testing.T) []byte {
	t.Helper()
	return aPackedRelease(t,
		[]tar.Header{
			{Name: BinaryName, Mode: 0o755, Size: 20, Typeflag: tar.TypeReg},
			{Name: "workers/", Mode: 0o755, Typeflag: tar.TypeDir},
			{Name: "workers/worker.js", Mode: 0o644, Size: 9, Typeflag: tar.TypeReg},
		},
		[]string{"#!/bin/sh\nexit 0\n     ", "", "a bundle\n"})
}

func TestAReleaseIsUnpackedIntoItsOwnFolder(t *testing.T) {
	home := testkit.NewTempHome(t)
	folder := t.TempDir()
	manifest := aReleaseHolding(t, folder, "0.7.0", aPlainRelease(t))

	binary, err := installRelease(context.Background(), home, Source{Address: folder}, manifest, runtime.GOARCH)

	if err != nil {
		t.Fatalf("installing the release failed: %v", err)
	}
	if binary != filepath.Join(home.ReleaseFolder("0.7.0"), BinaryName) {
		t.Errorf("the binary landed at %s rather than in the release folder", binary)
	}
	about, err := os.Stat(binary)
	if err != nil {
		t.Fatalf("the binary was not unpacked: %v", err)
	}
	if about.Mode().Perm()&0o100 == 0 {
		t.Errorf("the binary is mode %v, which cannot be run", about.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(home.ReleaseFolder("0.7.0"), "workers", "worker.js")); err != nil {
		t.Errorf("the worker bundle was not unpacked: %v", err)
	}
}

func TestAnArchiveWhoseChecksumIsWrongIsRefusedAndNothingIsLeftBehind(t *testing.T) {
	home := testkit.NewTempHome(t)
	folder := t.TempDir()
	manifest := aReleaseHolding(t, folder, "0.7.0", aPlainRelease(t))
	manifest.Checksums[ArchiveName("0.7.0", runtime.GOARCH)] = strings.Repeat("a", ChecksumDigits)

	_, err := installRelease(context.Background(), home, Source{Address: folder}, manifest, runtime.GOARCH)

	if err == nil {
		t.Fatalf("an archive whose checksum did not match was installed")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("the refusal does not say the checksum is wrong: %v", err)
	}
	if _, err := os.Stat(home.ReleaseFolder("0.7.0")); !os.IsNotExist(err) {
		t.Errorf("the refused release left a folder behind in %s", home.ReleasesFolder())
	}
	left, err := os.ReadDir(home.ReleasesFolder())
	if err != nil {
		t.Fatalf("reading the releases folder failed: %v", err)
	}
	if len(left) != 0 {
		t.Errorf("the refused download left %d things behind in the releases folder", len(left))
	}
}

func TestAnArchiveThatIsNotAReleaseIsRefused(t *testing.T) {
	cases := []struct {
		name    string
		archive func(t *testing.T) []byte
		says    string
	}{
		{
			"a name that climbs out of the folder",
			func(t *testing.T) []byte {
				return aPackedRelease(t,
					[]tar.Header{{Name: "../escaped", Mode: 0o644, Size: 4, Typeflag: tar.TypeReg}},
					[]string{"away"})
			},
			"escaped",
		},
		{
			"a link, which a release never holds",
			func(t *testing.T) []byte {
				return aPackedRelease(t,
					[]tar.Header{
						{Name: BinaryName, Mode: 0o755, Size: 2, Typeflag: tar.TypeReg},
						{Name: "elsewhere", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"},
					},
					[]string{"hi", ""})
			},
			"elsewhere",
		},
		{
			"no binary at all",
			func(t *testing.T) []byte {
				return aPackedRelease(t,
					[]tar.Header{{Name: "readme.txt", Mode: 0o644, Size: 2, Typeflag: tar.TypeReg}},
					[]string{"hi"})
			},
			BinaryName,
		},
		{
			"not a gzipped tar at all",
			func(t *testing.T) []byte { return []byte("this is not an archive") },
			"archive",
		},
	}

	for _, one := range cases {
		t.Run(one.name, func(t *testing.T) {
			home := testkit.NewTempHome(t)
			folder := t.TempDir()
			manifest := aReleaseHolding(t, folder, "0.7.0", one.archive(t))

			_, err := installRelease(context.Background(), home, Source{Address: folder}, manifest, runtime.GOARCH)

			if err == nil {
				t.Fatalf("an archive with %s was installed", one.name)
			}
			if !strings.Contains(err.Error(), one.says) {
				t.Errorf("the refusal does not say what is wrong, it says: %v", err)
			}
		})
	}
}

func TestAnArchiveWithTooManyEntriesIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)
	folder := t.TempDir()
	entries := []tar.Header{}
	bodies := []string{}
	for at := range 5 {
		entries = append(entries, tar.Header{Name: filepath.Join("many", string(rune('a'+at))), Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
		bodies = append(bodies, "x")
	}
	manifest := aReleaseHolding(t, folder, "0.7.0", aPackedRelease(t, entries, bodies))

	previous := archiveEntryCap
	archiveEntryCap = 3
	t.Cleanup(func() { archiveEntryCap = previous })

	_, err := installRelease(context.Background(), home, Source{Address: folder}, manifest, runtime.GOARCH)

	if err == nil {
		t.Fatalf("an archive with more entries than the cap was unpacked")
	}
}

func TestAnArchiveLongerUnpackedThanTheCapIsRefused(t *testing.T) {
	home := testkit.NewTempHome(t)
	folder := t.TempDir()
	manifest := aReleaseHolding(t, folder, "0.7.0", aPlainRelease(t))

	previous := unpackedByteCap
	unpackedByteCap = 4
	t.Cleanup(func() { unpackedByteCap = previous })

	_, err := installRelease(context.Background(), home, Source{Address: folder}, manifest, runtime.GOARCH)

	if err == nil {
		t.Fatalf("an archive that unpacks to more than the cap was unpacked")
	}
}

func TestTheNewestReleasesAreKeptAndTheRestGoAway(t *testing.T) {
	home := testkit.NewTempHome(t)
	for _, version := range []string{"0.4.0", "0.5.0", "0.6.0", "0.7.0", "0.8.0"} {
		if err := os.MkdirAll(home.ReleaseFolder(version), contract.HomeFolderMode); err != nil {
			t.Fatalf("making the release folder for %s failed: %v", version, err)
		}
	}
	if err := os.WriteFile(filepath.Join(home.ReleasesFolder(), "notes.txt"), []byte("kept"), contract.DataFileMode); err != nil {
		t.Fatalf("writing a file beside the releases failed: %v", err)
	}

	if err := pruneReleases(home, KeptReleases, "0.4.0"); err != nil {
		t.Fatalf("pruning the releases failed: %v", err)
	}

	for _, version := range []string{"0.4.0", "0.6.0", "0.7.0", "0.8.0"} {
		if _, err := os.Stat(home.ReleaseFolder(version)); err != nil {
			t.Errorf("version %s should have been kept: %v", version, err)
		}
	}
	if _, err := os.Stat(home.ReleaseFolder("0.5.0")); !os.IsNotExist(err) {
		t.Errorf("version 0.5.0 is older than the three kept and was not removed")
	}
	if _, err := os.Stat(filepath.Join(home.ReleasesFolder(), "notes.txt")); err != nil {
		t.Errorf("pruning removed a file that is not a release: %v", err)
	}
}

func TestPruningAHomeWithNoReleasesFolderIsNotAFailure(t *testing.T) {
	home := contract.NewHome(filepath.Join(t.TempDir(), "gone"))

	if err := pruneReleases(home, KeptReleases); err != nil {
		t.Errorf("pruning a home with no releases folder failed: %v", err)
	}
}
