package vault

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// SudoEntryName is the name of the one entry that holds the machine password,
// which the shell tool uses when the user approves a command that needs sudo.
const SudoEntryName = "sudo"

// The bounds on what one vault file may hold, so that a damaged or hostile file
// cannot make the agent allocate without end.
const (
	// MaxEntries is the most entries one vault may hold.
	MaxEntries = 500
	// MaxNameLength is the longest an entry name may be.
	MaxNameLength = 64
	// MaxValueLength is the longest any one value in an entry may be, and the
	// longest secret the masked prompt accepts.
	MaxValueLength = 4096
	// MaxDomains is the most hostnames one entry may name.
	MaxDomains = 32
)

// documentVersion is the version this build writes, and the only one it reads.
const documentVersion = 1

// Entry is one login the vault holds: a name to point at it by, the site it is
// for, the hostnames it may be typed into, and the values themselves.
//
// An entry's text form is contract.SecretMarker, and so is its JSON form, so
// that a value cannot leak by being printed, logged, or serialized into a tool
// result. That is the same marker contract.Credential prints as, so nothing on
// the way out of the program shows a value whichever of the two it is holding.
// The harness reads the values through the fields; nothing model-facing ever
// holds one of these.
type Entry struct {
	// Name is what a "secret://name" reference points at, unique in the vault.
	Name string
	// Site is the name the user knows the login by, such as "X".
	Site string
	// Domains are the hostnames the login may be typed into.
	Domains []string
	// Username is the login name.
	Username string
	// Password is the password.
	Password string
	// TOTPSecret is the shared secret the two-factor code comes from, or empty.
	TOTPSecret string
}

// String is the text form of an entry, which says nothing about what it holds.
func (entry Entry) String() string { return contract.SecretMarker }

// MarshalJSON writes an entry as the string "[secret]", so that model-facing
// code cannot serialize a password even by accident. The vault file is written
// through the stored shape below rather than through this method.
func (entry Entry) MarshalJSON() ([]byte, error) {
	return json.Marshal(contract.SecretMarker)
}

// Credential is the entry in the shape the harness fills a login form with.
func (entry Entry) Credential() contract.Credential {
	return contract.Credential{
		Site:       entry.Site,
		Domains:    slices.Clone(entry.Domains),
		Username:   entry.Username,
		Password:   entry.Password,
		TOTPSecret: entry.TOTPSecret,
	}
}

// Listing is one line of the vault listing: what the entry is called and what
// site it is for. A listing never carries a value.
type Listing struct {
	// Name is what a reference points at.
	Name string
	// Site is the name the user knows the login by.
	Site string
}

// storedDocument is the plaintext inside the encrypted file: a version and the
// whole list of entries, rewritten in full on every change.
type storedDocument struct {
	// Version says which shape the file is written in.
	Version int `json:"version"`
	// Entries is the whole list, rewritten in full on every change.
	Entries []storedEntry `json:"entries"`
}

// storedEntry is one entry as it sits inside the encrypted file. Its fields
// match Entry one for one, so that the two convert into each other.
type storedEntry struct {
	// Name is what a reference points at.
	Name string `json:"name"`
	// Site is the name the user knows the login by.
	Site string `json:"site,omitempty"`
	// Domains are the hostnames the login may be typed into.
	Domains []string `json:"domains,omitempty"`
	// Username is the login name.
	Username string `json:"username,omitempty"`
	// Password is the password.
	Password string `json:"password,omitempty"`
	// TOTPSecret is the shared secret the two-factor code comes from.
	TOTPSecret string `json:"totp_secret,omitempty"`
}

// parseDocument reads the plaintext of a vault file into entries. Every error
// names the file, because the user has to know which file to restore.
func parseDocument(path string, plaintext []byte) ([]Entry, error) {
	var document storedDocument
	if err := json.Unmarshal(plaintext, &document); err != nil {
		return nil, fmt.Errorf("the vault file %s does not hold the entry list Coeus writes, so restore it from a backup in ~/.coeus/backups: %w", path, err)
	}
	if document.Version != documentVersion {
		return nil, fmt.Errorf("the vault file %s says it is version %d and this build of Coeus writes version %d, so update Coeus or restore a backup", path, document.Version, documentVersion)
	}
	if len(document.Entries) > MaxEntries {
		return nil, fmt.Errorf("the vault file %s holds %d entries and the limit is %d, so it is damaged and should be restored from a backup", path, len(document.Entries), MaxEntries)
	}

	entries := make([]Entry, 0, len(document.Entries))
	seen := map[string]bool{}
	for _, stored := range document.Entries {
		entry := Entry(stored)
		if err := checkEntry(entry); err != nil {
			return nil, fmt.Errorf("the vault file %s holds an entry Coeus cannot use: %w", path, err)
		}
		if seen[entry.Name] {
			return nil, fmt.Errorf("the vault file %s holds two entries named %q and every name must be its own, so restore it from a backup", path, entry.Name)
		}
		seen[entry.Name] = true
		entries = append(entries, entry)
	}
	sortEntries(entries)
	return entries, nil
}

// serializeDocument turns the entries into the plaintext of a vault file.
func serializeDocument(entries []Entry) ([]byte, error) {
	document := storedDocument{Version: documentVersion, Entries: make([]storedEntry, 0, len(entries))}
	for _, entry := range entries {
		document.Entries = append(document.Entries, storedEntry(entry))
	}
	written, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("the entries could not be written as JSON, which should never happen: %w", err)
	}
	return written, nil
}

// checkEntry says whether an entry can be stored and pointed at by a reference.
func checkEntry(entry Entry) error {
	if _, usable := contract.SecretReferenceName(contract.SecretReferencePrefix + entry.Name); !usable {
		return fmt.Errorf("an entry needs a name with no spaces in it, because a reference is written as %sname", contract.SecretReferencePrefix)
	}
	if len(entry.Name) > MaxNameLength {
		return fmt.Errorf("the entry name %q is %d characters and the limit is %d, so choose a shorter name", entry.Name, len(entry.Name), MaxNameLength)
	}
	if len(entry.Domains) > MaxDomains {
		return fmt.Errorf("the entry %q names %d domains and the limit is %d, so keep only the hostnames the login is really used on", entry.Name, len(entry.Domains), MaxDomains)
	}
	for _, value := range []string{entry.Site, entry.Username, entry.Password, entry.TOTPSecret} {
		if len(value) > MaxValueLength {
			return fmt.Errorf("a value in the entry %q is %d characters and the limit is %d, so it is not a password", entry.Name, len(value), MaxValueLength)
		}
	}
	for _, domain := range entry.Domains {
		if domain == "" || len(domain) > MaxNameLength || strings.ContainsAny(domain, " \t\r\n/") {
			return fmt.Errorf("the entry %q names the domain %q, which is not a hostname, so write it as x.com", entry.Name, domain)
		}
	}
	return nil
}

// sortEntries puts the entries in name order, which is the order the listing
// prints them in and the order the file is written in.
func sortEntries(entries []Entry) {
	slices.SortFunc(entries, func(left, right Entry) int { return strings.Compare(left.Name, right.Name) })
}
