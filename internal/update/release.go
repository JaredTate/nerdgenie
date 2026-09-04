// Unpacking follows ZeroClaw's updater at
// ~/Code/zeroclaw/src/commands/update.rs, which unpacks a release into a
// staging folder, refuses every link inside the archive so that nothing can be
// written through one, and only then puts the result where it belongs. What is
// added here is that the staging folder is beside the release folder, so the
// last step is a rename on the same filesystem and a half-unpacked release
// never exists under a version's name.

package update

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// KeptReleases is how many installed versions are kept. Three is the running
// one, the one before it to roll back to, and one more, which is enough to get
// out of trouble without filling the disk.
const KeptReleases = 3

// The bounds on one archive. They are variables rather than constants only so
// that a test can lower them and prove what happens at the cap without building
// an archive of hundreds of megabytes; nothing but a test ever changes them.
var (
	// archiveEntryCap is the most files and folders one release may hold.
	archiveEntryCap = 20000
	// unpackedByteCap is the most one release may write to disk.
	unpackedByteCap = int64(512 << 20)
)

// installRelease downloads the archive the manifest names for this machine,
// checks it against the manifest's checksum, and unpacks it into its own folder
// under the home's releases folder. It returns the path of the new binary, and
// leaves nothing behind when anything goes wrong.
func installRelease(ctx context.Context, home contract.Home, source Source, manifest Manifest, architecture string) (string, error) {
	checksum, err := manifest.ChecksumFor(architecture)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(home.ReleasesFolder(), contract.HomeFolderMode); err != nil {
		return "", fmt.Errorf("the releases folder %s could not be made: %w", home.ReleasesFolder(), err)
	}
	archive, err := downloadArchive(ctx, home, source, ArchiveName(manifest.Version, architecture), checksum)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(archive) }()

	staging, err := os.MkdirTemp(home.ReleasesFolder(), "unpacking-")
	if err != nil {
		return "", fmt.Errorf("a folder to unpack the release into could not be made in %s: %w", home.ReleasesFolder(), err)
	}
	defer func() { _ = os.RemoveAll(staging) }()

	if err := unpackArchive(archive, staging); err != nil {
		return "", err
	}
	return putReleaseInPlace(home, manifest.Version, staging)
}

// downloadArchive fetches the archive beside the releases folder and checks it
// against the checksum the manifest gives, so that nothing unverified is ever
// unpacked. It returns the path of the file it wrote.
func downloadArchive(ctx context.Context, home contract.Home, source Source, name string, checksum string) (string, error) {
	file, err := os.CreateTemp(home.ReleasesFolder(), "downloading-")
	if err != nil {
		return "", fmt.Errorf("a file to download the release into could not be made in %s: %w", home.ReleasesFolder(), err)
	}
	path := file.Name()
	sum := sha256.New()

	_, err = source.Fetch(ctx, name, io.MultiWriter(file, sum))
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = matchChecksum(name, checksum, hex.EncodeToString(sum.Sum(nil)))
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// matchChecksum compares what arrived with what the manifest promised.
func matchChecksum(name string, wanted string, found string) error {
	if wanted == found {
		return nil
	}
	return fmt.Errorf("the checksum of %s is %s where the release manifest says %s, so the download is not the release it claims to be and nothing was installed",
		name, found, wanted)
}

// putReleaseInPlace moves the unpacked release under its version's name, after
// making sure it holds the program it is supposed to.
func putReleaseInPlace(home contract.Home, version string, staging string) (string, error) {
	unpacked := filepath.Join(staging, BinaryName)
	if _, err := os.Stat(unpacked); err != nil {
		return "", fmt.Errorf("the release archive holds no %s program, so it is not a Coeus release and nothing was installed", BinaryName)
	}
	if err := os.Chmod(unpacked, 0o755); err != nil {
		return "", fmt.Errorf("the new program %s could not be made runnable: %w", unpacked, err)
	}

	folder := home.ReleaseFolder(version)
	if err := os.RemoveAll(folder); err != nil {
		return "", fmt.Errorf("the old copy of version %s at %s could not be cleared away: %w", version, folder, err)
	}
	if err := os.Rename(staging, folder); err != nil {
		return "", fmt.Errorf("the unpacked release could not be put in place at %s: %w", folder, err)
	}
	return filepath.Join(folder, BinaryName), nil
}

// unpackArchive writes every entry of a gzipped tar into a folder, under a cap
// on how many entries and how many bytes, and refuses anything that is not a
// plain file or a folder inside it.
func unpackArchive(path string, into string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("the downloaded archive %s could not be opened: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	unzipped, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("the download is not a gzipped tar archive, so the address is serving something other than a release: %w", err)
	}
	defer func() { _ = unzipped.Close() }()

	archive := tar.NewReader(unzipped)
	written := int64(0)
	for entries := 0; entries < archiveEntryCap; entries++ {
		header, err := archive.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("the release archive stopped part way through, so try the update again: %w", err)
		}
		if written, err = unpackEntry(archive, header, into, written); err != nil {
			return err
		}
	}
	return fmt.Errorf("the release archive holds more than %d files, so it is not a release Coeus can install", archiveEntryCap)
}

// unpackEntry writes one entry of the archive and returns how much has been
// written in all, so that the caller can stop at the cap.
func unpackEntry(archive *tar.Reader, header *tar.Header, into string, written int64) (int64, error) {
	path, err := pathInside(into, header.Name)
	if err != nil {
		return written, err
	}
	switch header.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(path, contract.HomeFolderMode); err != nil {
			return written, fmt.Errorf("the folder %s from the release could not be made: %w", path, err)
		}
		return written, nil
	case tar.TypeReg:
		if written+header.Size > unpackedByteCap {
			return written, fmt.Errorf("the release archive unpacks to more than the %d byte limit, so it is not a release Coeus can install", unpackedByteCap)
		}
		if err := writeUnpackedFile(archive, path, header); err != nil {
			return written, err
		}
		return written + header.Size, nil
	default:
		return written, fmt.Errorf("the release archive holds %q, which is a link rather than a file, and a release never holds one, so nothing was installed",
			header.Name)
	}
}

// writeUnpackedFile writes one file out of the archive with the mode the archive
// gives it, keeping only the permission bits so that nothing can arrive as a
// program that runs as somebody else.
func writeUnpackedFile(archive *tar.Reader, path string, header *tar.Header) error {
	if err := os.MkdirAll(filepath.Dir(path), contract.HomeFolderMode); err != nil {
		return fmt.Errorf("the folder %s for a file in the release could not be made: %w", filepath.Dir(path), err)
	}
	mode := os.FileMode(header.Mode).Perm()
	if mode == 0 {
		mode = contract.DataFileMode
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("the file %s from the release could not be written: %w", path, err)
	}
	_, err = io.Copy(file, io.LimitReader(archive, header.Size))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("the file %s from the release could not be written in full: %w", path, err)
	}
	return nil
}

// pathInside turns a name from the archive into the path it is written to, and
// refuses any name that would land outside the folder being unpacked into.
func pathInside(folder string, name string) (string, error) {
	cleaned := filepath.Clean(strings.ReplaceAll(name, `\`, "/"))
	path := filepath.Join(folder, cleaned)
	if filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) ||
		!strings.HasPrefix(path, folder+string(filepath.Separator)) {
		return "", fmt.Errorf("the release archive holds %q, which would be written outside the release folder, so nothing was installed", name)
	}
	return path, nil
}

// pruneReleases removes every installed version but the newest few, never
// touching the ones named or anything in the folder that is not a release.
func pruneReleases(home contract.Home, keep int, protected ...string) error {
	versions, err := installedVersions(home)
	if err != nil || len(versions) <= keep {
		return err
	}
	for _, version := range versions[:len(versions)-keep] {
		if contains(protected, version) {
			continue
		}
		if err := os.RemoveAll(home.ReleaseFolder(version)); err != nil {
			return fmt.Errorf("the old release %s could not be removed, so the releases folder is fuller than it should be: %w",
				home.ReleaseFolder(version), err)
		}
	}
	return nil
}

// installedVersions lists the versions under the releases folder, oldest first.
// A folder whose name is not a version, and the current link itself, are not
// releases and are left alone.
func installedVersions(home contract.Home) ([]string, error) {
	entries, err := os.ReadDir(home.ReleasesFolder())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("the releases folder %s could not be read: %w", home.ReleasesFolder(), err)
	}
	versions := []string{}
	for _, entry := range entries {
		if !entry.IsDir() || !plainName(entry.Name()) {
			continue
		}
		if _, _, readable := readVersion(entry.Name()); readable {
			versions = append(versions, entry.Name())
		}
	}
	sort.Slice(versions, func(one int, two int) bool { return Newer(versions[one], versions[two]) })
	return versions, nil
}

// contains says whether the list holds the word.
func contains(words []string, wanted string) bool {
	for _, word := range words {
		if word == wanted {
			return true
		}
	}
	return false
}
