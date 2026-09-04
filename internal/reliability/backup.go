package reliability

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"filippo.io/age"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// KeptBackups is how many archives are kept. A week of nightly backups is
// enough to notice that something went wrong and to go back to before it did.
const KeptBackups = 7

// BackupSettings is what one backup needs to know.
type BackupSettings struct {
	// Home is the folder being backed up.
	Home contract.Home
	// Clock says what the archive is named after.
	Clock contract.Clock
	// Folder is where the archive is written, and is the home's backups folder
	// when it is empty. The configuration's backup_path is passed here.
	Folder string
	// Keep is how many archives are kept, and is KeptBackups when it is zero.
	Keep int
}

// Backup writes one age-encrypted archive holding the database, the vault, and
// the browser profile, and removes all but the newest few. It returns the path
// of the archive it wrote.
//
// The archive is locked with the vault's own key, so the file at
// Home.VaultKeyFile is what opens it again. Keep a copy of that key somewhere
// else: without it an archive is a wall of random bytes.
func Backup(ctx context.Context, settings BackupSettings) (string, error) {
	if settings.Clock == nil {
		return "", errors.New("a backup needs a clock to name the archive after, so pass clock.System() or the one the test controls")
	}
	folder := settings.Folder
	if folder == "" {
		folder = settings.Home.BackupsFolder()
	}
	identity, err := identityFrom(settings.Home.VaultKeyFile())
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		return "", fmt.Errorf("the backup folder %s could not be made: %w", folder, err)
	}

	archive := filepath.Join(folder, ArchiveName(settings.Clock.Now()))
	beside, err := writeArchiveBeside(ctx, settings.Home, folder, identity.Recipient())
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(beside) }()

	if err := os.Rename(beside, archive); err != nil {
		return "", fmt.Errorf("the archive could not be put in place at %s: %w", archive, err)
	}
	return archive, prune(folder, settings.keep())
}

// keep is how many archives this backup leaves behind it.
func (settings BackupSettings) keep() int {
	if settings.Keep <= 0 {
		return KeptBackups
	}
	return settings.Keep
}

// writeArchiveBeside writes the archive under a temporary name in the same
// folder, so that the name a restore looks for only ever appears on a file that
// is finished, and returns the temporary name.
func writeArchiveBeside(ctx context.Context, home contract.Home, folder string, recipient age.Recipient) (string, error) {
	beside, err := os.CreateTemp(folder, ".writing-backup-*")
	if err != nil {
		return "", fmt.Errorf("the archive could not be written in %s, so check who owns the folder: %w", folder, err)
	}
	besidePath := beside.Name()

	if err := fillArchive(ctx, beside, home, recipient); err != nil {
		_ = beside.Close()
		_ = os.Remove(besidePath)
		return "", err
	}
	if err := beside.Sync(); err != nil {
		_ = beside.Close()
		_ = os.Remove(besidePath)
		return "", fmt.Errorf("the archive %s could not be made durable: %w", besidePath, err)
	}
	if err := beside.Close(); err != nil {
		_ = os.Remove(besidePath)
		return "", fmt.Errorf("the archive %s could not be closed: %w", besidePath, err)
	}
	if err := os.Chmod(besidePath, contract.SecretFileMode); err != nil {
		_ = os.Remove(besidePath)
		return "", fmt.Errorf("the archive %s could not be made readable by nobody else: %w", besidePath, err)
	}
	return besidePath, nil
}

// fillArchive writes the encrypted tar into the file, in one pass, closing the
// encryption last so that everything is flushed.
func fillArchive(ctx context.Context, into io.Writer, home contract.Home, recipient age.Recipient) error {
	locked, err := age.Encrypt(into, recipient)
	if err != nil {
		return fmt.Errorf("the archive could not be locked with the vault key: %w", err)
	}
	archive := tar.NewWriter(locked)

	if err := addDatabase(ctx, archive, home); err != nil {
		return err
	}
	if err := addFile(archive, home.VaultFile(), vaultEntry); err != nil {
		return err
	}
	if err := addBrowserProfile(archive, home); err != nil {
		return err
	}

	if err := archive.Close(); err != nil {
		return fmt.Errorf("the archive could not be finished: %w", err)
	}
	if err := locked.Close(); err != nil {
		return fmt.Errorf("the archive could not be sealed: %w", err)
	}
	return nil
}

// addDatabase puts a copy of the database into the archive that is complete on
// its own, because a plain read of a file the agent is writing catches it in
// the middle. A home with no database yet has nothing to add.
func addDatabase(ctx context.Context, archive *tar.Writer, home contract.Home) error {
	if !databaseIsThere(home.DatabaseFile()) {
		return nil
	}
	folder, err := os.MkdirTemp("", "nerdgenie-database-copy-*")
	if err != nil {
		return fmt.Errorf("a folder for the copy of the database could not be made: %w", err)
	}
	defer func() { _ = os.RemoveAll(folder) }()

	copied := filepath.Join(folder, databaseEntry)
	if err := copyDatabase(ctx, home.DatabaseFile(), copied); err != nil {
		return err
	}
	return addFile(archive, copied, databaseEntry)
}

// addBrowserProfile puts every file under the browser folder into the archive,
// under the same names, so that a restored Chrome profile is still logged in.
func addBrowserProfile(archive *tar.Writer, home contract.Home) error {
	folder := home.BrowserFolder()
	entries := 0
	return filepath.WalkDir(folder, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("the browser folder %s could not be walked: %w", folder, err)
		}
		entries++
		if entries > MaxArchiveEntries {
			return fmt.Errorf("the browser folder %s holds more than %d files, which is more than one archive carries: back it up yourself", folder, MaxArchiveEntries)
		}
		inside, err := filepath.Rel(folder, path)
		if err != nil {
			return fmt.Errorf("the name of %s inside the archive could not be worked out: %w", path, err)
		}
		name := filepath.ToSlash(filepath.Join(browserFolderEntry, inside))
		if entry.IsDir() {
			return addFolder(archive, name)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		return addFile(archive, path, name)
	})
}

// addFolder writes one folder into the archive, so that an empty profile folder
// comes back as a folder.
func addFolder(archive *tar.Writer, name string) error {
	header := &tar.Header{
		Typeflag: tar.TypeDir,
		Name:     name + "/",
		Mode:     int64(contract.HomeFolderMode),
	}
	if err := archive.WriteHeader(header); err != nil {
		return fmt.Errorf("the folder %s could not be written into the archive: %w", name, err)
	}
	return nil
}

// addFile writes one file into the archive under the name given, and says
// nothing when the file is not there, because a home without a vault yet is an
// ordinary home.
func addFile(archive *tar.Writer, path string, name string) error {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("the file %s could not be read into the archive: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("the size of %s could not be read: %w", path, err)
	}
	header := &tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: int64(contract.SecretFileMode), Size: info.Size()}
	if err := archive.WriteHeader(header); err != nil {
		return fmt.Errorf("the file %s could not be written into the archive: %w", path, err)
	}
	if _, err := io.Copy(archive, io.LimitReader(file, info.Size())); err != nil {
		return fmt.Errorf("the contents of %s could not be written into the archive: %w", path, err)
	}
	return nil
}

// prune removes the oldest archives, leaving the newest few.
func prune(folder string, keep int) error {
	archives, err := archivesIn(folder)
	if err != nil {
		return err
	}
	if len(archives) <= keep {
		return nil
	}
	for _, name := range archives[:len(archives)-keep] {
		if err := os.Remove(filepath.Join(folder, name)); err != nil {
			return fmt.Errorf("the old archive %s could not be removed: %w", name, err)
		}
	}
	return nil
}
