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

// Sentinel is the file that says a life of the program is running.
type Sentinel struct {
	path  string
	clock contract.Clock
}

// sentinelBody is what the file holds, for whoever looks at it after a crash.
type sentinelBody struct {
	// ProcessID is the program that wrote it.
	ProcessID int `json:"processID"`
	// StartedAt is when that life began.
	StartedAt time.Time `json:"startedAt"`
}

// NewSentinel returns the sentinel for one home folder.
func NewSentinel(home contract.Home, clock contract.Clock) *Sentinel {
	return &Sentinel{path: filepath.Join(home.RunFolder(), SentinelFileName), clock: clock}
}

// Start writes the sentinel for this life of the program and says whether the
// last life ended uncleanly, which is true whenever the file was still there. A
// file that cannot be understood counts as unclean too, because something was
// there and no exit path took it away.
func (sentinel *Sentinel) Start() (bool, error) {
	unclean := sentinel.leftBehind()
	written := sentinelBody{ProcessID: os.Getpid(), StartedAt: sentinel.clock.Now()}
	if err := writeStateFile(sentinel.path, written); err != nil {
		return unclean, err
	}
	return unclean, nil
}

// MarkExited takes the sentinel away, which is what makes the next start a
// clean one. It is called on every path out of the program.
func (sentinel *Sentinel) MarkExited() error {
	return removeStateFile(sentinel.path)
}

// leftBehind says whether a sentinel from an earlier life is still there.
func (sentinel *Sentinel) leftBehind() bool {
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
