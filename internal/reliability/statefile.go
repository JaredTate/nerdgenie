package reliability

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/JaredTate/coeus/internal/contract"
)

// maxStateFileBytes caps how much of one of these files is read. Every one of
// them holds a handful of times and names, so anything longer is not one of
// ours and is refused rather than loaded.
const maxStateFileBytes = 1 << 20

// readStateFile reads one small JSON file into the value given. It returns
// false when the file is not there, which is the ordinary case on a first
// start, and an error when the file is there but cannot be read or understood.
func readStateFile(path string, into any) (bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("the file %s could not be opened, so check who owns it: %w", path, err)
	}
	defer func() { _ = file.Close() }()

	written, err := io.ReadAll(io.LimitReader(file, maxStateFileBytes+1))
	if err != nil {
		return false, fmt.Errorf("the file %s could not be read: %w", path, err)
	}
	if len(written) > maxStateFileBytes {
		return false, fmt.Errorf("the file %s is longer than %d bytes, so it is not one Coeus wrote: move it aside", path, maxStateFileBytes)
	}
	if err := json.Unmarshal(written, into); err != nil {
		return false, fmt.Errorf("the file %s does not hold the fields Coeus wrote, so move it aside and let Coeus write a new one: %w", path, err)
	}
	return true, nil
}

// writeStateFile writes one small JSON file, making the folder above it first
// and putting the file in place with a rename, so that a crash halfway through
// leaves either the old file or the new one and never half of either.
func writeStateFile(path string, from any) error {
	written, err := json.Marshal(from)
	if err != nil {
		return fmt.Errorf("what was going to be written to %s could not be turned into JSON: %w", path, err)
	}
	folder := filepath.Dir(path)
	if err := os.MkdirAll(folder, contract.HomeFolderMode); err != nil {
		return fmt.Errorf("the folder for %s could not be made, so check who owns %s: %w", path, folder, err)
	}

	beside, err := os.CreateTemp(folder, filepath.Base(path)+"-*")
	if err != nil {
		return fmt.Errorf("the file %s could not be written beside itself, so check who owns %s: %w", path, folder, err)
	}
	besidePath := beside.Name()
	defer func() { _ = os.Remove(besidePath) }()

	if _, err := beside.Write(written); err != nil {
		_ = beside.Close()
		return fmt.Errorf("the file %s could not be written: %w", path, err)
	}
	if err := beside.Close(); err != nil {
		return fmt.Errorf("the file %s could not be closed after writing: %w", path, err)
	}
	if err := os.Rename(besidePath, path); err != nil {
		return fmt.Errorf("the file %s could not be put in place: %w", path, err)
	}
	return nil
}

// removeStateFile takes one of these files away and says nothing when it was
// not there, because both of those mean the same thing to every caller here.
func removeStateFile(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("the file %s could not be removed, so check who owns it: %w", path, err)
	}
	return nil
}
