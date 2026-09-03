// The lifecycle sentinel follows the lifecycle ledger in Hermes at
// ~/Code/hermes-agent/gateway/lifecycle_ledger.py. A kill signal, an
// out-of-memory kill, or a machine losing power takes the program out before
// any exit path runs, so the next start has no idea the last life ended badly.
// A file written at startup and removed on the way out closes that gap: finding
// it means nothing ran on the way out, which is the moment to look at the
// database before anything is written to it.

package reliability

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// SentinelFileName is the file under the run folder that exists only while the
// program is running.
const SentinelFileName = "lifecycle.json"

// asideTimeLayout is how the time is written into the name of a database moved
// aside. It has no colons, because a name with colons in it is awkward to type
// and awkward to copy.
const asideTimeLayout = "2006-01-02T150405Z"

// sentinelFile is the file that says a life of the program is running.
type sentinelFile struct {
	path  string
	clock contract.Clock
}

// sentinelBody is what the file holds, for whoever looks at it after a crash.
type sentinelBody struct {
	// ProcessID is the program that wrote it.
	ProcessID int `json:"processID"`
	// StartedAt is when that life began.
	StartedAt time.Time `json:"startedAt"`
	// Found is what the recovery saw before the database was opened, kept here
	// until the guard starts, because the recovery runs before there is any
	// channel to tell the user on.
	Found recovery `json:"found,omitzero"`
}

// newSentinel returns the sentinel for one home folder.
func newSentinel(home contract.Home, clock contract.Clock) *sentinelFile {
	return &sentinelFile{path: filepath.Join(home.RunFolder(), SentinelFileName), clock: clock}
}

// startThisLife writes the sentinel for this life of the program, carrying what
// the recovery found so that the guard can tell the user about it.
func (sentinel *sentinelFile) startThisLife(found recovery) error {
	return writeStateFile(sentinel.path, sentinelBody{
		ProcessID: os.Getpid(),
		StartedAt: sentinel.clock.Now(),
		Found:     found,
	})
}

// takeWhatThisLifeFound hands the guard what the recovery saw and clears it from
// the file, so that it is acted on once. It refuses a sentinel this program did
// not write, because that means the database was opened before anything checked
// it, which is the one order that loses everything written afterwards.
func (sentinel *sentinelFile) takeWhatThisLifeFound() (recovery, error) {
	body := sentinelBody{}
	there, err := readStateFile(sentinel.path, &body)
	if err != nil {
		return recovery{}, err
	}
	if !there || body.ProcessID != os.Getpid() {
		return recovery{}, errors.New("the database was not checked before it was opened, so call reliability.PrepareDatabase first and open the database at the path it hands back")
	}
	found := body.Found
	body.Found = recovery{}
	return found, writeStateFile(sentinel.path, body)
}

// markExited takes the sentinel away, which is what makes the next start a
// clean one. It is called on every path out of the program.
func (sentinel *sentinelFile) markExited() error {
	return removeStateFile(sentinel.path)
}

// leftBehind says whether a sentinel from an earlier life is still there.
func (sentinel *sentinelFile) leftBehind() bool {
	_, err := os.Stat(sentinel.path)
	return err == nil
}

// MoveDatabaseAside renames a database that cannot be trusted, putting the time
// in its new name, and returns the name it was given. The write-ahead and
// shared-memory files go with it, because a fresh database beside the old one's
// write-ahead file is a second broken database.
func MoveDatabaseAside(path string, now time.Time) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("the database %s could not be moved aside, because it is not there: %w", path, err)
	}
	movedTo := path + ".broken-" + now.UTC().Format(asideTimeLayout)
	if _, err := os.Stat(movedTo); err == nil {
		return "", fmt.Errorf("the database %s could not be moved to %s, because something is already there: move it away yourself", path, movedTo)
	}
	if err := os.Rename(path, movedTo); err != nil {
		return "", fmt.Errorf("the database %s could not be moved to %s, so check who owns the folder: %w", path, movedTo, err)
	}
	for _, beside := range []string{"-wal", "-shm"} {
		if err := os.Rename(path+beside, movedTo+beside); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("the file %s could not be moved with the database it belongs to: %w", path+beside, err)
		}
	}
	return movedTo, nil
}

// databaseIsThere says whether a database file exists at that path, which is
// what decides whether there is anything to check or to back up.
func databaseIsThere(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
