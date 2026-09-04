package reliability

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The name every archive is given: the prefix, the moment it was written, and
// the suffix. The moment is written so that sorting the names by hand sorts
// them by time, which is what makes the newest archive the last one.
const (
	// ArchivePrefix starts the name of every archive.
	ArchivePrefix = "nerdgenie-backup-"
	// ArchiveSuffix ends it, and says what is inside: a tar file, encrypted with
	// age.
	ArchiveSuffix = ".tar.age"
	// archiveTimeLayout is how the moment is written into the name. It has no
	// colons, because a name with colons in it is awkward to type.
	archiveTimeLayout = "2006-01-02-150405"
)

// The three things a backup holds, named inside the archive so that a restore
// knows where each one goes. Nothing else is ever read out of an archive.
const (
	// databaseEntry is the one SQLite file.
	databaseEntry = "nerdgenie.db"
	// vaultEntry is the encrypted secret store.
	vaultEntry = "vault.age"
	// browserFolderEntry is the folder holding the Chrome profiles.
	browserFolderEntry = "browser"
)

// The bounds on one archive, because a file from outside the program is read
// under a cap like everything else.
const (
	// MaxArchiveBytes is the most a restore writes out of one archive.
	MaxArchiveBytes = 2 << 30
	// MaxArchiveEntries is the most files and folders one archive may hold.
	MaxArchiveEntries = 100000
	// maxKeyFileBytes caps the key file, which holds one short line.
	maxKeyFileBytes = 4096
)

// ArchiveName is the name an archive written at that moment is given.
func ArchiveName(now time.Time) string {
	return ArchivePrefix + now.UTC().Format(archiveTimeLayout) + ArchiveSuffix
}

// LatestArchive is the newest archive in the folder, which is the one a
// recovery reaches for.
func LatestArchive(folder string) (string, error) {
	archives, err := archivesIn(folder)
	if err != nil {
		return "", err
	}
	if len(archives) == 0 {
		return "", fmt.Errorf("there is no backup in %s, so there is nothing to put back: run \"nerdgenie backup\" to make one", folder)
	}
	return filepath.Join(folder, archives[len(archives)-1]), nil
}

// archivesIn lists the archives in a folder, oldest first, which is the order
// their names already sort in.
func archivesIn(folder string) ([]string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, fmt.Errorf("the backup folder %s could not be read: %w", folder, err)
	}
	found := []string{}
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasPrefix(name, ArchivePrefix) && strings.HasSuffix(name, ArchiveSuffix) {
			found = append(found, name)
		}
	}
	slices.Sort(found)
	return found, nil
}

// identityFrom reads the age private key that locks and unlocks an archive.
// It is the key the vault made, because a home has one key and the backup is
// for the same person as the vault.
func identityFrom(path string) (*age.X25519Identity, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("the key file %s could not be read, and without it a backup can be neither locked nor opened: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	written, err := io.ReadAll(io.LimitReader(file, maxKeyFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("the key file %s could not be read: %w", path, err)
	}
	if len(written) > maxKeyFileBytes {
		return nil, fmt.Errorf("the key file %s is longer than %d bytes, so it does not hold an age private key", path, maxKeyFileBytes)
	}
	identity, err := age.ParseX25519Identity(strings.TrimSpace(string(written)))
	if err != nil {
		return nil, fmt.Errorf("the key file %s does not hold an age private key, so point at the vault key of the home the backup came from: %w", path, err)
	}
	return identity, nil
}

// pathInHome turns a name inside an archive into the path it is written to, and
// refuses anything that is not one of the three things a backup holds. This is
// what stops an archive from writing outside the home folder it is restored
// into, whoever wrote the archive.
func pathInHome(home contract.Home, name string) (string, bool) {
	cleaned := path.Clean("/" + strings.ReplaceAll(name, `\`, "/"))[1:]
	if cleaned == "" || cleaned == "." {
		return "", false
	}
	switch {
	case cleaned == databaseEntry:
		return home.DatabaseFile(), true
	case cleaned == vaultEntry:
		return home.VaultFile(), true
	case cleaned == browserFolderEntry:
		return home.BrowserFolder(), true
	case strings.HasPrefix(cleaned, browserFolderEntry+"/"):
		inside := strings.TrimPrefix(cleaned, browserFolderEntry+"/")
		return filepath.Join(home.BrowserFolder(), filepath.FromSlash(inside)), true
	default:
		return "", false
	}
}
