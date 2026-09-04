// The encrypted file follows ZeroClaw's secret store, at
// ~/Code/zeroclaw/crates/zeroclaw-config/src/secrets.rs: the whole store is
// rewritten and published atomically on every change, so a crash leaves either
// the old file or the new one and never half of either.

package vault

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"filippo.io/age"
	"github.com/JaredTate/nerdgenie/internal/contract"
)

// maxVaultFileBytes caps how much plaintext one vault file may hold, because
// every buffer in Nerd Genie has a cap and a real vault is a few kilobytes.
const maxVaultFileBytes = 1 << 20

// Vault is the encrypted secret store: the entries, the key that opens them,
// and the redactor built from their values.
type Vault struct {
	home     contract.Home
	clock    contract.Clock
	guard    sync.Mutex
	identity *age.X25519Identity
	entries  []Entry
	redactor *strings.Replacer
	closed   bool
}

// Open reads the vault under the home folder, making the key file on first use
// and refusing a key file anyone else could read. A vault file that is not
// there yet is an empty vault, written the first time an entry is added.
//
// The clock says when a two-factor code runs out. The running program passes
// clock.System(); a test passes the clock it controls.
func Open(home contract.Home, clock contract.Clock) (*Vault, error) {
	if clock == nil {
		return nil, errors.New("the vault needs a clock to say when a two-factor code runs out, so pass clock.System() or the one the test controls")
	}
	if err := os.MkdirAll(home.Root, contract.HomeFolderMode); err != nil {
		return nil, fmt.Errorf("the home folder %s could not be made: %w", home.Root, err)
	}

	identity, err := loadOrCreateIdentity(home.VaultKeyFile())
	if err != nil {
		return nil, err
	}
	entries, err := readVaultFile(home.VaultFile(), identity)
	if err != nil {
		return nil, err
	}

	opened := &Vault{home: home, clock: clock, identity: identity, entries: entries}
	opened.rebuildRedactor()
	return opened, nil
}

// Close forgets the key and the values it opened, so that a vault the program
// has finished with is no longer in memory. Closing twice does nothing.
func (vault *Vault) Close() error {
	vault.guard.Lock()
	defer vault.guard.Unlock()
	vault.entries = nil
	vault.identity = nil
	vault.closed = true
	vault.rebuildRedactor()
	return nil
}

// Add puts one entry in the vault and writes the file. An entry whose name is
// already there replaces it, which is how a rotated password is entered.
func (vault *Vault) Add(entry Entry) error {
	vault.guard.Lock()
	defer vault.guard.Unlock()
	if err := vault.openForWork(); err != nil {
		return err
	}
	if err := checkEntry(entry); err != nil {
		return err
	}

	kept := slices.Clone(vault.entries)
	at := slices.IndexFunc(kept, func(held Entry) bool { return held.Name == entry.Name })
	if at >= 0 {
		kept[at] = entry
	} else {
		if len(kept) >= MaxEntries {
			return fmt.Errorf("the vault already holds %d entries, which is the limit, so remove one before adding another", MaxEntries)
		}
		kept = append(kept, entry)
	}
	sortEntries(kept)
	return vault.replaceEntries(kept)
}

// Remove takes one entry out of the vault and writes the file.
func (vault *Vault) Remove(name string) error {
	vault.guard.Lock()
	defer vault.guard.Unlock()
	if err := vault.openForWork(); err != nil {
		return err
	}

	at := slices.IndexFunc(vault.entries, func(held Entry) bool { return held.Name == name })
	if at < 0 {
		return fmt.Errorf("the vault holds no entry named %q, so run /vault list to see what it does hold", name)
	}
	kept := slices.Delete(slices.Clone(vault.entries), at, at+1)
	return vault.replaceEntries(kept)
}

// List returns the name and site of every entry, in name order, and never a
// value.
func (vault *Vault) List() []Listing {
	vault.guard.Lock()
	defer vault.guard.Unlock()

	listed := make([]Listing, 0, len(vault.entries))
	for _, entry := range vault.entries {
		listed = append(listed, Listing{Name: entry.Name, Site: entry.Site})
	}
	return listed
}

// openForWork returns the reason a closed vault cannot be used, and nothing
// when it can. The caller already holds the lock.
func (vault *Vault) openForWork() error {
	if vault.closed {
		return errors.New("the vault is closed, so open it again before reading or changing a secret")
	}
	return nil
}

// replaceEntries writes the new list to disk and only then keeps it in memory,
// so that a failed write leaves the vault exactly as it was. The caller already
// holds the lock.
func (vault *Vault) replaceEntries(entries []Entry) error {
	if err := writeVaultFile(vault.home.VaultFile(), vault.identity, entries); err != nil {
		return err
	}
	vault.entries = entries
	vault.rebuildRedactor()
	return nil
}

// readVaultFile decrypts the vault file and reads the entries out of it. A file
// that is not there is an empty vault.
func readVaultFile(path string, identity *age.X25519Identity) ([]Entry, error) {
	written, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("the vault file %s could not be read: %w", path, err)
	}

	plain, err := age.Decrypt(bytes.NewReader(written), identity)
	if err != nil {
		return nil, fmt.Errorf("the vault file %s could not be decrypted with the key in the key file, so restore both from the same backup: %w", path, err)
	}
	plaintext, err := io.ReadAll(io.LimitReader(plain, maxVaultFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("the vault file %s could not be read to the end: %w", path, err)
	}
	if len(plaintext) > maxVaultFileBytes {
		return nil, fmt.Errorf("the vault file %s holds more than %d bytes, so it is damaged and should be restored from a backup", path, maxVaultFileBytes)
	}
	return parseDocument(path, plaintext)
}

// writeVaultFile encrypts the entries and publishes them over the old file in
// one step: write beside it, wait for the disk, then rename.
func writeVaultFile(path string, identity *age.X25519Identity, entries []Entry) error {
	plaintext, err := serializeDocument(entries)
	if err != nil {
		return err
	}
	sealed, err := sealDocument(identity, plaintext)
	if err != nil {
		return err
	}

	folder := filepath.Dir(path)
	beside, err := os.CreateTemp(folder, "vault-*.age")
	if err != nil {
		return fmt.Errorf("the vault could not be written beside %s: %w", path, err)
	}
	besidePath := beside.Name()
	defer func() { _ = os.Remove(besidePath) }()

	if err := writeAndSync(beside, sealed); err != nil {
		return fmt.Errorf("the vault could not be written to %s: %w", besidePath, err)
	}
	if err := os.Chmod(besidePath, contract.SecretFileMode); err != nil {
		return fmt.Errorf("the new vault file %s could not be made owner-only: %w", besidePath, err)
	}
	if err := os.Rename(besidePath, path); err != nil {
		return fmt.Errorf("the new vault could not be put in place at %s: %w", path, err)
	}
	return syncFolder(folder)
}

// sealDocument encrypts the plaintext to the identity's own public key.
func sealDocument(identity *age.X25519Identity, plaintext []byte) ([]byte, error) {
	var sealed bytes.Buffer
	writer, err := age.Encrypt(&sealed, identity.Recipient())
	if err != nil {
		return nil, fmt.Errorf("the vault could not be encrypted: %w", err)
	}
	if _, err := writer.Write(plaintext); err != nil {
		return nil, fmt.Errorf("the vault could not be encrypted: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("the vault could not be encrypted to the end: %w", err)
	}
	return sealed.Bytes(), nil
}
