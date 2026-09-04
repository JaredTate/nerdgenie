// The key file follows ZeroClaw's secret store, at
// ~/Code/zeroclaw/crates/zeroclaw-config/src/secrets.rs: a key of its own that
// lives beside the encrypted file, owner-only from the moment it is created,
// published without replacing anything, and refused when its mode has been
// loosened. The cipher here is age rather than ZeroClaw's ChaCha20-Poly1305,
// because the same format encrypts the nightly backups.

package vault

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"filippo.io/age"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// maxKeyFileBytes caps how much of a key file is read, because an age identity
// is one short line and anything longer is not one.
const maxKeyFileBytes = 4096

// loadOrCreateIdentity returns the private key that opens the vault, making it
// on first use and refusing it when anyone but the agent's own user account
// could read it.
func loadOrCreateIdentity(path string) (*age.X25519Identity, error) {
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return createIdentity(path)
	} else if err != nil {
		return nil, fmt.Errorf("the vault key file %s could not be looked at: %w", path, err)
	}
	return readIdentity(path)
}

// readIdentity reads the private key out of a key file that is already there,
// after making sure nobody else could have read or written it.
func readIdentity(path string) (*age.X25519Identity, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("the vault key file %s could not be looked at: %w", path, err)
	}
	if err := checkKeyFile(path, info); err != nil {
		return nil, err
	}

	written, err := readShortFile(path)
	if err != nil {
		return nil, fmt.Errorf("the vault key file %s could not be read: %w", path, err)
	}
	identity, err := age.ParseX25519Identity(strings.TrimSpace(string(written)))
	if err != nil {
		return nil, fmt.Errorf("the vault key file %s does not hold an age private key, so move it aside and let Coeus make a new one: %w", path, err)
	}
	return identity, nil
}

// createIdentity makes a new private key and publishes it without replacing an
// existing one, so that two copies of Coeus starting at once cannot leave one
// of them holding a key the file no longer has. The copy that loses the race
// reads the winner's key instead, and does so once rather than trying again.
func createIdentity(path string) (*age.X25519Identity, error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, fmt.Errorf("a new vault key could not be generated: %w", err)
	}
	if err := publishKeyFile(path, identity.String()); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return readIdentity(path)
		}
		return nil, err
	}
	return identity, nil
}

// publishKeyFile writes the key beside its final name with owner-only mode and
// then links it into place, which fails rather than replacing a key that is
// already there.
func publishKeyFile(path string, key string) error {
	folder := filepath.Dir(path)
	beside, err := os.CreateTemp(folder, "vault-key-*")
	if err != nil {
		return fmt.Errorf("the vault key could not be written beside %s: %w", path, err)
	}
	besidePath := beside.Name()
	defer func() { _ = os.Remove(besidePath) }()

	if err := writeAndSync(beside, []byte(key+"\n")); err != nil {
		return fmt.Errorf("the new vault key could not be written to %s: %w", besidePath, err)
	}
	if err := os.Chmod(besidePath, contract.SecretFileMode); err != nil {
		return fmt.Errorf("the new vault key file %s could not be made owner-only: %w", besidePath, err)
	}
	if err := os.Link(besidePath, path); err != nil {
		return fmt.Errorf("the new vault key could not be put in place at %s: %w", path, err)
	}
	return syncFolder(folder)
}

// checkKeyFile refuses a key file that is not a plain file, that others may
// read, or that belongs to somebody else.
func checkKeyFile(path string, info fs.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("the vault key file %s is not a plain file, so move whatever is there aside and let Coeus make a new key", path)
	}
	owner, known := fileOwner(info)
	if !known {
		return fmt.Errorf("the owner of the vault key file %s could not be read, so check the file and its folder", path)
	}
	return checkKeyFileSafety(path, info.Mode().Perm(), owner, os.Getuid())
}

// checkKeyFileSafety holds the two rules a key file must keep: nobody but the
// owner may read it, and the owner is the account Coeus runs as. It takes the
// mode and the owner rather than reading them, so that a test can hand it the
// cases a test cannot create.
func checkKeyFileSafety(path string, mode fs.FileMode, ownerID int, currentUserID int) error {
	if mode&0o077 != 0 {
		return fmt.Errorf("the vault key file %s has mode %#o, which lets other accounts on this machine read it, so run chmod 0600 %s and start Coeus again", path, uint32(mode.Perm()), path)
	}
	if ownerID != currentUserID {
		return fmt.Errorf("the vault key file %s belongs to user %d and Coeus runs as user %d, so move the file aside and let Coeus make a key of its own", path, ownerID, currentUserID)
	}
	return nil
}

// fileOwner returns the user id that owns a file, and false on a system that
// does not report one.
func fileOwner(info fs.FileInfo) (int, bool) {
	details, isUnix := info.Sys().(*syscall.Stat_t)
	if !isUnix {
		return 0, false
	}
	return int(details.Uid), true
}

// readShortFile reads a file that is meant to be small, and refuses one that is
// not, because every buffer in Coeus has a cap.
func readShortFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	written, err := io.ReadAll(io.LimitReader(file, maxKeyFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(written) > maxKeyFileBytes {
		return nil, fmt.Errorf("the file %s is longer than %d bytes, so it does not hold an age private key", path, maxKeyFileBytes)
	}
	return written, nil
}

// writeAndSync writes the whole of the text and waits for the disk to say it is
// there, so that a crash cannot leave a half-written secret behind.
func writeAndSync(file *os.File, written []byte) error {
	if _, err := file.Write(written); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// syncFolder waits for the disk to say a new name in the folder is really
// there, so that a crash cannot lose a file that was just published.
func syncFolder(folder string) error {
	opened, err := os.Open(folder)
	if err != nil {
		return fmt.Errorf("the folder %s could not be opened to make its contents durable: %w", folder, err)
	}
	defer func() { _ = opened.Close() }()
	if err := opened.Sync(); err != nil {
		return fmt.Errorf("the folder %s could not be made durable: %w", folder, err)
	}
	return nil
}
