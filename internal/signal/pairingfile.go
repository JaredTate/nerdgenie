package signal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

const (
	// stateVersion is written into both files, so that a later version of Coeus
	// can tell what it is looking at.
	stateVersion = 1
	// MaxRememberedRequests is how many senders the store remembers having heard
	// from, which is what keeps the ten-minute rule from growing without end.
	MaxRememberedRequests = 100
	// maxStateFileBytes caps how much of either file will be read, so that a
	// file somebody filled up cannot fill memory.
	maxStateFileBytes = 1 << 20
)

// pendingCode is one code waiting to be approved. The code itself is never here:
// only the salt it was mixed with and the hash of the two.
type pendingCode struct {
	// Sender is who the code was offered to.
	Sender string `json:"sender"`
	// Salt is the random bytes mixed into the code, written as hexadecimal.
	Salt string `json:"salt"`
	// Hash is the hash of the salt and the code, written as hexadecimal.
	Hash string `json:"hash"`
	// Created is when the code was drawn, which is when its hour starts.
	Created time.Time `json:"created"`
}

// pairingState is the whole of the codes file.
type pairingState struct {
	// Version says which shape the file is in.
	Version int `json:"version"`
	// Pending are the codes waiting to be approved.
	Pending []pendingCode `json:"pending"`
	// LastRequest is when each sender last asked for a code.
	LastRequest map[string]time.Time `json:"last_request"`
	// WrongTries is how many wrong codes have been typed in a row.
	WrongTries int `json:"wrong_tries"`
	// LockedUntil is when the door opens again, or the zero time when it is not
	// shut.
	LockedUntil time.Time `json:"locked_until"`
}

// approvedSender is one person the agent has been told to listen to.
type approvedSender struct {
	// Sender is their Signal identifier, in the form the daemon reports.
	Sender string `json:"sender"`
	// Approved is when they were paired.
	Approved time.Time `json:"approved"`
}

// approvedState is the whole of the approved file.
type approvedState struct {
	// Version says which shape the file is in.
	Version int `json:"version"`
	// Senders are the people the agent listens to.
	Senders []approvedSender `json:"senders"`
}

// codesFilePath is where the waiting codes are kept.
func codesFilePath(folder string) string { return filepath.Join(folder, "pairing.json") }

// approvedFilePath is where the approved senders are kept.
func approvedFilePath(folder string) string { return filepath.Join(folder, "approved.json") }

// makeFolder makes sure the Signal folder is there, with the mode the layout
// calls for.
func makeFolder(folder string) error {
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		return fmt.Errorf("cannot make the Signal folder %s, so check that the home folder is writable: %w", folder, err)
	}
	return nil
}

// readJSONFile reads one of the two files into the value given. A file that is
// not there leaves the value alone, because an empty store is a valid store.
func readJSONFile(path string, into any) error {
	written, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read the Signal file %s, so check who owns it: %w", path, err)
	}
	if len(written) == 0 {
		return nil
	}
	if len(written) > maxStateFileBytes {
		return fmt.Errorf("the Signal file %s is larger than %d bytes, which no real one is, so move it aside and start again", path, maxStateFileBytes)
	}
	if err := json.Unmarshal(written, into); err != nil {
		return fmt.Errorf("cannot read the Signal file %s, because it is not the JSON this version writes, so move it aside and start again: %w", path, err)
	}
	return nil
}

// writeJSONFile writes one of the two files so that a crash part way through
// leaves the old one in place: the new text goes to a file beside it, is given
// the owner-only mode, and is then moved over the old one.
func writeJSONFile(path string, value any) error {
	written, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cannot turn the Signal state into JSON for %s: %w", path, err)
	}
	beside := path + ".new"
	if err := os.WriteFile(beside, written, contract.SecretFileMode); err != nil {
		return fmt.Errorf("cannot write the Signal file %s, so check that the home folder is writable: %w", beside, err)
	}
	if err := os.Chmod(beside, contract.SecretFileMode); err != nil {
		return fmt.Errorf("cannot set the owner-only mode on the Signal file %s: %w", beside, err)
	}
	if err := os.Rename(beside, path); err != nil {
		return fmt.Errorf("cannot move the Signal file into place at %s: %w", path, err)
	}
	return nil
}
