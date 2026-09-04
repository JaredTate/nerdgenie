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

// RestoreSettings is what one restore needs to know.
type RestoreSettings struct {
	// Home is the folder the archive is put back into.
	Home contract.Home
	// Archive is the file to read.
	Archive string
	// KeyFile is the age private key that opens the archive, and is the home's
	// own vault key when it is empty.
	KeyFile string
	// Force writes over a home that is not empty, which is what --force means.
	Force bool
	// OnlyTheDatabase puts back the database and nothing else, which is what a
	// recovery from a damaged database wants: the vault and the browser profile
	// in the home are newer than the ones in the archive.
	OnlyTheDatabase bool
}

// Restore puts the database, the vault, and the browser profile back from an
// archive. It refuses a home that already holds any of them, because a restore
// over a working home loses whatever the agent has learned since the backup;
// Force is the way to say that is what you meant.
func Restore(ctx context.Context, settings RestoreSettings) error {
	file, err := os.Open(settings.Archive)
	if err != nil {
		return fmt.Errorf("the archive %s could not be opened: %w", settings.Archive, err)
	}
	defer func() { _ = file.Close() }()

	keyFile := settings.KeyFile
	if keyFile == "" {
		keyFile = settings.Home.VaultKeyFile()
	}
	identity, err := identityFrom(keyFile)
	if err != nil {
		return err
	}
	if !settings.Force {
		if err := refuseAHomeThatIsNotEmpty(settings.Home, settings.OnlyTheDatabase); err != nil {
			return err
		}
	}

	opened, err := age.Decrypt(file, identity)
	if err != nil {
		return fmt.Errorf("the archive %s could not be opened with the key %s, so check that it is the key of the home the backup came from: %w",
			settings.Archive, keyFile, err)
	}
	return unpack(ctx, tar.NewReader(opened), settings)
}

// unpack writes every entry of the archive into the home folder, under a cap on
// how many entries and how many bytes it will write, and refuses any name that
// is not one of the three things a backup holds.
func unpack(ctx context.Context, archive *tar.Reader, settings RestoreSettings) error {
	name := settings.Archive
	written := int64(0)
	for entries := 0; entries < MaxArchiveEntries; entries++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("putting the archive %s back was stopped part way, so the home folder holds only some of it: %w", name, err)
		}
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("the archive %s could not be read to the end, so it is damaged: %w", name, err)
		}
		path, wanted := pathInHome(settings.Home, header.Name)
		if !wanted {
			return fmt.Errorf("the archive %s holds %q, which is not part of a Nerd Genie backup, so nothing more was put back", name, header.Name)
		}
		if settings.OnlyTheDatabase && path != settings.Home.DatabaseFile() {
			continue
		}
		if written, err = writeEntry(archive, header, path, written); err != nil {
			return fmt.Errorf("the archive %s could not be put back: %w", name, err)
		}
	}
	return fmt.Errorf("the archive %s holds more than %d files, which is more than a Nerd Genie backup ever has", name, MaxArchiveEntries)
}

// writeEntry writes one entry and returns how many bytes have been written out
// of the archive so far, refusing anything that is neither a plain file nor a
// folder and anything that takes the archive past its size cap.
func writeEntry(archive *tar.Reader, header *tar.Header, path string, written int64) (int64, error) {
	switch header.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(path, contract.HomeFolderMode); err != nil {
			return written, fmt.Errorf("the folder %s could not be made: %w", path, err)
		}
		return written, nil
	case tar.TypeReg:
		if header.Size < 0 || written+header.Size > MaxArchiveBytes {
			return written, fmt.Errorf("%q would take the whole restore past %d bytes, which is more than a Nerd Genie backup ever holds", header.Name, int64(MaxArchiveBytes))
		}
		if err := writeRestoredFile(archive, path, header.Size); err != nil {
			return written, err
		}
		return written + header.Size, nil
	default:
		return written, fmt.Errorf("%q is neither a plain file nor a folder, and a Nerd Genie backup holds nothing else", header.Name)
	}
}

// writeRestoredFile writes one file out of the archive, owner-only, because
// everything a backup holds is either a secret or a record of what the agent
// did.
func writeRestoredFile(archive *tar.Reader, path string, size int64) error {
	if err := os.MkdirAll(filepath.Dir(path), contract.HomeFolderMode); err != nil {
		return fmt.Errorf("the folder for %s could not be made: %w", path, err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, contract.SecretFileMode)
	if err != nil {
		return fmt.Errorf("the file %s could not be written: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	copied, err := io.Copy(file, io.LimitReader(archive, size))
	if err != nil {
		return fmt.Errorf("the file %s could not be written out of the archive: %w", path, err)
	}
	if copied != size {
		return fmt.Errorf("the file %s came out of the archive %d bytes long rather than %d, so the archive is damaged", path, copied, size)
	}
	return nil
}

// refuseAHomeThatIsNotEmpty stops a restore that would write over a database, a
// vault, or a browser profile that is already there. A restore of the database
// alone only looks at the database, because it is the only thing it writes.
func refuseAHomeThatIsNotEmpty(home contract.Home, onlyTheDatabase bool) error {
	wanted := []string{home.DatabaseFile(), home.VaultFile()}
	if onlyTheDatabase {
		wanted = wanted[:1]
	}
	for _, path := range wanted {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s is already there, and a restore writes over it, so move the home folder aside first or run the restore again with --force", path)
		}
	}
	if onlyTheDatabase {
		return nil
	}
	entries, err := os.ReadDir(home.BrowserFolder())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("the browser folder %s could not be looked at: %w", home.BrowserFolder(), err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("the browser folder %s already holds a profile, and a restore writes over it, so move the home folder aside first or run the restore again with --force", home.BrowserFolder())
	}
	return nil
}
